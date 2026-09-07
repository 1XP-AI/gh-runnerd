package livecanary

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestObserveRESTRunnerRejectsIncompleteOrAmbiguousFields(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		good       bool
	}{
		{"false-present", `{"id":8,"name":"fixture-runner","status":"online","busy":false,"future_field":1}`, true},
		{"busy", `{"id":8,"name":"fixture-runner","status":"online","busy":true}`, true},
		{"offline", `{"id":8,"name":"fixture-runner","status":"offline","busy":false}`, true},
		{"missing-busy", `{"id":8,"name":"fixture-runner","status":"online"}`, false},
		{"null-busy", `{"id":8,"name":"fixture-runner","status":"online","busy":null}`, false},
		{"null", `null`, false},
		{"unknown-status", `{"id":8,"name":"fixture-runner","status":"synthetic-secret-state","busy":false}`, false},
		{"missing-status", `{"id":8,"name":"fixture-runner","busy":false}`, false},
		{"missing-id", `{"name":"fixture-runner","status":"online","busy":false}`, false},
		{"missing-name", `{"id":8,"status":"online","busy":false}`, false},
		{"wrong-name", `{"id":8,"name":"synthetic-secret-name","status":"online","busy":false}`, false},
		{"negative-id", `{"id":-8,"name":"fixture-runner","status":"online","busy":false}`, false},
		{"duplicate-id", `{"id":7,"id":8,"name":"fixture-runner","status":"online","busy":false}`, false},
		{"folded-id", `{"id":7,"ID":8,"name":"fixture-runner","status":"online","busy":false}`, false},
		{"trailing", `{"id":8,"name":"fixture-runner","status":"online","busy":false} {}`, false},
		{"malformed", `{"id":8`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) })
			got, err := api.observeRESTRunner(context.Background(), 8, "fixture-runner")
			if (err == nil) != tc.good {
				t.Fatalf("field validation mismatch: %v", err)
			}
			if tc.good && (got.Busy == nil || got.Response.Outcome != observationPresent) {
				t.Fatal("presence lost")
			}
			assertObservationSecretSafe(t, got, err)
		})
	}
}

func TestObserveRESTJobRejectsSourceAndAttemptAmbiguity(t *testing.T) {
	for _, fault := range []string{"queued", "progress", "empty", "missing-jobs", "null-jobs", "terminal-flip", "status-regression", "two", "count", "missing-count", "next-page", "list-null", "wrong-id", "wrong-run", "wrong-head", "changed-attempt", "changed-source", "run-duplicate", "run-folded", "nested-folded", "run-null", "run-malformed", "run-trailing", "run-missing-private", "run-missing-fork", "detail-duplicate", "runner-drift", "runner-group", "missing-association", "unknown-status", "unknown-conclusion", "missing-conclusion", "previous-id"} {
		t.Run(fault, func(t *testing.T) {
			a := approval()
			var calls atomic.Int32
			api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer "+credentials(a).VerificationToken {
					t.Error("wrong Actions authority")
				}
				job := observationJob(a)
				if fault == "queued" {
					job["status"] = "queued"
					job["conclusion"] = nil
					delete(job, "runner_id")
					delete(job, "runner_name")
					delete(job, "runner_group_id")
				}
				switch r.URL.Path {
				case "/repos/fixture-org/canary/actions/runs/5":
					run := observationRun(a)
					if fault == "changed-attempt" {
						run["run_attempt"] = 2
					}
					if fault == "changed-source" {
						run["event"] = "pull_request"
					}
					if fault == "run-missing-private" {
						delete(run["head_repository"].(map[string]any), "private")
					}
					if fault == "run-missing-fork" {
						delete(run["head_repository"].(map[string]any), "fork")
					}
					data, _ := json.Marshal(run)
					switch fault {
					case "run-duplicate":
						data = []byte(`{"id":99,` + string(data[1:]))
					case "run-folded":
						data = []byte(`{"ID":99,` + string(data[1:]))
					case "nested-folded":
						data = []byte(strings.Replace(string(data), `"head_repository":{`, `"head_repository":{"ID":99,`, 1))
					case "run-null":
						data = []byte(`null`)
					case "run-malformed":
						data = []byte(`{"id":`)
					case "run-trailing":
						data = append(data, []byte(` {}`)...)
					}
					_, _ = w.Write(data)
				case "/repos/fixture-org/canary/actions/runs/5/attempts/1/jobs":
					count := 1
					if fault == "terminal-flip" {
						job["conclusion"] = "failure"
					}
					if fault == "progress" {
						job["status"] = "queued"
						job["conclusion"] = nil
						delete(job, "runner_id")
						delete(job, "runner_name")
						delete(job, "runner_group_id")
					}
					jobs := []any{job}
					if fault == "empty" || fault == "missing-jobs" || fault == "null-jobs" {
						count = 0
						jobs = []any{}
					}
					if fault == "two" {
						count = 2
						jobs = append(jobs, job)
					}
					if fault == "count" {
						count = 2
					}
					if fault == "next-page" {
						w.Header().Set("Link", `<https://evil.example/>; rel="next"`)
					}
					if fault == "list-null" {
						_, _ = w.Write([]byte(`null`))
						return
					}
					list := map[string]any{"total_count": count, "jobs": jobs}
					if fault == "missing-jobs" {
						delete(list, "jobs")
					}
					if fault == "null-jobs" {
						list["jobs"] = nil
					}
					if fault == "missing-count" {
						delete(list, "total_count")
					}
					_ = json.NewEncoder(w).Encode(list)
				case "/repos/fixture-org/canary/actions/jobs/9007199254740995":
					switch fault {
					case "status-regression":
						job["status"] = "in_progress"
						job["conclusion"] = nil
					case "wrong-id":
						job["id"] = int64(9007199254740996)
					case "wrong-run":
						job["run_id"] = 6
					case "wrong-head":
						job["head_sha"] = strings.Repeat("f", 40)
					case "runner-drift":
						job["runner_id"] = int64(9007199254740997)
					case "runner-group":
						job["runner_group_id"] = 99
					case "missing-association":
						delete(job, "runner_id")
					case "unknown-status":
						job["status"] = "synthetic-secret-state"
					case "unknown-conclusion":
						job["conclusion"] = "synthetic-secret-conclusion"
					case "missing-conclusion":
						delete(job, "conclusion")
					}
					data, _ := json.Marshal(job)
					if fault == "detail-duplicate" {
						data = []byte(`{"ID":1,` + string(data[1:]))
					}
					_, _ = w.Write(data)
				default:
					t.Error("unexpected REST endpoint")
					w.WriteHeader(500)
				}
			})
			var previous restJobID
			if fault == "previous-id" {
				previous = 9007199254740996
			}
			got, err := api.observeRESTJob(context.Background(), 1, previous)
			good := fault == "queued" || fault == "empty" || fault == "progress"
			if (err == nil) != good {
				t.Fatalf("source/job validation mismatch: %v", err)
			}
			if good && fault != "progress" && got.Response.Outcome != observationPending {
				t.Fatal("pending observation became conclusive")
			}
			if strings.HasPrefix(fault, "run-") || fault == "nested-folded" || fault == "changed-attempt" || fault == "changed-source" {
				if calls.Load() != 1 {
					t.Fatal("invalid source permitted downstream reads")
				}
			}
			assertObservationSecretSafe(t, got, err)
		})
	}
}

func TestObserveInvalidAuthorityAndInputsNeverReachNetwork(t *testing.T) {
	for _, fault := range []string{"sdk-id", "sdk-set", "sdk-name", "rest-id", "rest-name", "attempt", "previous", "run-zero", "missing-verifier", "same-verifier", "verifier-newline", "expired-approval", "expired-token", "wrong-app", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			var calls atomic.Int32
			api := observationFixture(t, func(http.ResponseWriter, *http.Request) {}, func(http.ResponseWriter, *http.Request) bool { calls.Add(1); return false })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sdkID, setID, name, restID, attempt, previous := sdkRunnerID(8), 7, "fixture-runner", restRunnerID(8), 1, restJobID(0)
			switch fault {
			case "sdk-id":
				sdkID = 0
			case "sdk-set":
				setID = 0
			case "sdk-name":
				name = "bad\nname"
			case "rest-id":
				restID = -1
			case "rest-name":
				name = ""
			case "attempt":
				attempt = 2
			case "previous":
				previous = -1
			case "run-zero":
				api.approval.Phases = []string{"inspect"}
				api.approval.WorkflowRunID = 0
			case "missing-verifier":
				api.approval.Phases = []string{"inspect"}
				api.credentials.VerificationToken = ""
			case "same-verifier":
				api.approval.Phases = []string{"inspect"}
				api.credentials.VerificationToken = api.credentials.InstallationToken
			case "verifier-newline":
				api.approval.Phases = []string{"inspect"}
				api.credentials.VerificationToken += "\n"
			case "expired-approval":
				api.approval.ExpiresAt = time.Now().Add(-time.Second)
			case "expired-token":
				api.credentials.ExpiresAt = time.Now().Add(-time.Second)
			case "wrong-app":
				api.credentials.AppID++
			case "cancelled":
				cancel()
			}
			var err error
			if strings.HasPrefix(fault, "sdk-") {
				_, err = api.observeSDKRunner(ctx, sdkID, name, setID)
			} else if strings.HasPrefix(fault, "rest-") {
				_, err = api.observeRESTRunner(ctx, restID, name)
			} else {
				_, err = api.observeRESTJob(ctx, attempt, previous)
			}
			if err == nil || calls.Load() != 0 {
				t.Fatal("invalid invocation accepted or reached endpoint")
			}
		})
	}
}

func TestObserveStatusAndResponseBudget(t *testing.T) {
	for _, kind := range []string{"sdk", "rest"} {
		for _, mode := range []string{"404", "401", "403", "429", "500", "redirect", "fixed", "chunked", "gzip"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				var calls atomic.Int32
				api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					if status, e := strconv.Atoi(mode); e == nil {
						w.WriteHeader(status)
						_, _ = w.Write([]byte(`{"message":"synthetic-secret-body","typeName":"AgentNotFoundException"}`))
						return
					}
					if mode == "redirect" {
						w.Header().Set("Location", "https://evil.example/secret")
						w.WriteHeader(302)
						return
					}
					body := `{"extra":"` + strings.Repeat("s", int(responseBodyLimit)) + `"}`
					switch mode {
					case "fixed":
						w.Header().Set("Content-Length", strconv.Itoa(len(body)))
					case "chunked":
						w.(http.Flusher).Flush()
					case "gzip":
						w.Header().Set("Content-Encoding", "gzip")
						zip := gzip.NewWriter(w)
						_, _ = zip.Write([]byte(body))
						_ = zip.Close()
						return
					}
					_, _ = w.Write([]byte(body))
				})
				var result any
				var err error
				var response observationResponse
				if kind == "sdk" {
					got, e := api.observeSDKRunner(context.Background(), 8, "fixture-runner", 7)
					result, err, response = got, e, got.Response
				} else {
					got, e := api.observeRESTRunner(context.Background(), 8, "fixture-runner")
					result, err, response = got, e, got.Response
				}
				if mode == "404" {
					if err != nil || response.Outcome != observationNotFound {
						t.Fatal("scoped not-found status lost")
					}
				} else if err == nil || response.Outcome != observationUnresolved {
					t.Fatal("unknown or oversized response accepted")
				}
				if calls.Load() != 1 {
					t.Fatal("observation retried or followed redirect")
				}
				assertObservationSecretSafe(t, result, err)
			})
		}
	}
}

type observationRoundTripper func(*http.Request) (*http.Response, error)

func (f observationRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestObserveSampleKeepsOneDeadlineAcrossSourceListDetail(t *testing.T) {
	a := approval()
	c := credentials(a)
	api := &SDKAPI{approval: a, credentials: c, baseURL: "https://api.github.com"}
	var first time.Time
	calls := 0
	api.rest = &http.Client{Transport: observationRoundTripper(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Error("network request has no deadline")
		}
		if calls == 0 {
			first = deadline
		} else if !deadline.Equal(first) {
			t.Error("sample reset deadline between reads")
		}
		calls++
		var body any
		switch calls {
		case 1:
			body = observationRun(a)
		case 2:
			body = map[string]any{"total_count": 1, "jobs": []any{observationJob(a)}}
		case 3:
			body = observationJob(a)
		default:
			t.Error("extra sample request")
		}
		data, _ := json.Marshal(body)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})}
	started := time.Now()
	got, err := api.observeRESTJob(context.Background(), 1, 0)
	if err != nil || got.ID == 0 || calls != 3 || first.After(started.Add(operationTimeout+time.Second)) {
		t.Fatal("sample deadline or request scope wrong")
	}
}

func TestObserveCancellationAndApprovalExpiry(t *testing.T) {
	for _, mode := range []string{"cancel", "approval-expiry"} {
		t.Run(mode, func(t *testing.T) {
			entered := make(chan struct{})
			api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "approval-expiry" {
				api.approval.ExpiresAt = time.Now().Add(80 * time.Millisecond)
			}
			done := make(chan error, 1)
			go func() { _, err := api.observeRESTRunner(ctx, 8, "fixture-runner"); done <- err }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("fixture request never began")
			}
			if mode == "cancel" {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancelled observation accepted")
				}
			case <-time.After(time.Second):
				t.Fatal("observation ignored deadline")
			}
		})
	}
}

func TestObservationResponseCaptureIsLocalAndRejectsOtherOperations(t *testing.T) {
	first := &runnerResponseCapture{suffix: "/_apis/distributedtask/pools/0/agents/8"}
	second := &runnerResponseCapture{suffix: "/_apis/distributedtask/pools/0/agents/9"}
	var wg sync.WaitGroup
	for _, item := range []struct {
		capture    *runnerResponseCapture
		id, status int
	}{{first, 8, 404}, {second, 9, 200}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.WithValue(context.Background(), runnerResponseKey{}, item.capture)
			for _, path := range []string{"/orgs/fixture-org/actions/runners/registration-token", "/actions/runner-registration"} {
				req, _ := http.NewRequestWithContext(ctx, "POST", "https://fixture.actions.githubusercontent.com"+path, nil)
				captureRunnerResponse(req, 404)
			}
			req, _ := http.NewRequestWithContext(ctx, "GET", "https://fixture.actions.githubusercontent.com/tenant/_apis/distributedtask/pools/0/agents/"+strconv.Itoa(item.id)+"?api-version=6.0-preview", nil)
			captureRunnerResponse(req, item.status)
		}()
	}
	wg.Wait()
	if status, one := first.result(); status != 404 || !one {
		t.Fatal("first endpoint provenance lost")
	}
	if status, one := second.result(); status != 200 || !one {
		t.Fatal("concurrent provenance crossed")
	}
	req, _ := http.NewRequestWithContext(context.WithValue(context.Background(), runnerResponseKey{}, first), "GET", "https://fixture.actions.githubusercontent.com/tenant/_apis/distributedtask/pools/0/agents/8?api-version=6.0-preview", nil)
	captureRunnerResponse(req, 200)
	if _, one := first.result(); one {
		t.Fatal("multiple target responses accepted as one")
	}
}

func TestObserveSDKBootstrapFailureCannotReportRunnerAbsent(t *testing.T) {
	for _, status := range []int{404, 403, 429, 500} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var all, targets atomic.Int32
			api := observationFixture(t, func(http.ResponseWriter, *http.Request) { targets.Add(1) }, func(w http.ResponseWriter, r *http.Request) bool {
				all.Add(1)
				if !strings.HasSuffix(r.URL.Path, "/runners/registration-token") {
					t.Error("bootstrap failure did not stop request sequence")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"typeName":"AgentNotFoundException","message":"synthetic-secret-bootstrap"}`))
				return true
			})
			got, err := api.observeSDKRunner(context.Background(), 8, "fixture-runner", 7)
			if err == nil || got.Response.Outcome != observationUnresolved || got.Response.Status != 0 || targets.Load() != 0 || all.Load() != 1 {
				t.Fatal("bootstrap outcome became target absence or was retried")
			}
			assertObservationSecretSafe(t, got, err)
		})
	}
}

func TestObserveCancellationReachesSDKBootstrapAndRESTDetail(t *testing.T) {
	for _, mode := range []string{"sdk-bootstrap", "rest-detail"} {
		t.Run(mode, func(t *testing.T) {
			entered := make(chan struct{})
			var later atomic.Int32
			api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if mode == "sdk-bootstrap" {
					later.Add(1)
					return
				}
				a := approval()
				if strings.HasSuffix(r.URL.Path, "/runs/5") {
					_ = json.NewEncoder(w).Encode(observationRun(a))
					return
				}
				if strings.HasSuffix(r.URL.Path, "/attempts/1/jobs") {
					_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "jobs": []any{observationJob(a)}})
					return
				}
				close(entered)
				<-r.Context().Done()
			}, func(w http.ResponseWriter, r *http.Request) bool {
				if mode == "sdk-bootstrap" {
					close(entered)
					<-r.Context().Done()
					return true
				}
				return false
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				var err error
				if mode == "sdk-bootstrap" {
					_, err = api.observeSDKRunner(ctx, 8, "fixture-runner", 7)
				} else {
					_, err = api.observeRESTJob(ctx, 1, 0)
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("observation did not reach barrier")
			}
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancelled observation succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("original context cancellation lost")
			}
			if later.Load() != 0 {
				t.Fatal("SDK continued after cancelled bootstrap")
			}
		})
	}
}

func TestObserveJobStatusProgressionWithinOneSample(t *testing.T) {
	for _, from := range []string{"queued", "in_progress", "completed"} {
		for _, to := range []string{"queued", "in_progress", "completed"} {
			t.Run(from+"/"+to, func(t *testing.T) {
				a := approval()
				api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/runs/5") {
						_ = json.NewEncoder(w).Encode(observationRun(a))
						return
					}
					job := observationJob(a)
					status := to
					list := strings.HasSuffix(r.URL.Path, "/attempts/1/jobs")
					if list {
						status = from
					}
					job["status"] = status
					if status != "completed" {
						job["conclusion"] = nil
					}
					if list {
						_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "jobs": []any{job}})
					} else {
						_ = json.NewEncoder(w).Encode(job)
					}
				})
				rank := map[string]int{"queued": 0, "in_progress": 1, "completed": 2}
				_, err := api.observeRESTJob(context.Background(), 1, 0)
				if (err == nil) != (rank[to] >= rank[from]) {
					t.Fatal("same-sample status regression accepted or forward progress refused")
				}
			})
		}
	}
}
