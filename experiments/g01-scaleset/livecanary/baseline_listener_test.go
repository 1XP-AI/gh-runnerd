package livecanary

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
)

type baselineRoundTrip func(*http.Request) (*http.Response, error)

func (f baselineRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type baselineReply struct {
	status int
	body   any
	lost   bool
}

type baselineFixture struct {
	a                                               Approval
	api                                             *SDKAPI
	j                                               *FileJournal
	release                                         func()
	acks, acquires, continuations, polls, forbidden atomic.Int32
	sessions, sources, sets, requests               atomic.Int32
	change                                          func(string, any) any
	afterResponse                                   func(*http.Request, *http.Response)
	extra                                           func(http.ResponseWriter, *http.Request) bool
}

func baselineFixtureItem(a Approval, kind string) map[string]any {
	j := map[string]any{"messageType": kind, "runnerRequestId": int64(42), "jobId": "opaque-job-not-a-REST-id"}
	if kind == "JobAvailable" {
		j["ownerName"], j["repositoryName"], j["workflowRunId"] = a.Organization, a.Repository, a.WorkflowRunID
		j["eventName"], j["jobWorkflowRef"] = "workflow_dispatch", "opaque-observed-workflow-reference"
		j["acquireJobUrl"], j["jobDisplayName"] = "https://invalid.example/synthetic-secret-acquire", "synthetic-secret-display"
	}
	if kind == "JobStarted" || kind == "JobCompleted" {
		j["runnerId"], j["runnerName"] = 81, "fixture-runner"
	}
	if kind == "JobCompleted" {
		j["result"] = "succeeded"
	}
	return j
}
func newBaselineFixture(t *testing.T, change func(string, any) any) *baselineFixture {
	return newBaselineFixtureWithInventory(t, change, "")
}

func newBaselineFixtureWithInventory(t *testing.T, change func(string, any) any, inventory string) *baselineFixture {
	return newBaselineFixtureWithApproval(t, change, inventory, approval())
}

func newBaselineFixtureWithApproval(t *testing.T, change func(string, any) any, inventory string, a Approval) *baselineFixture {
	t.Helper()
	f := &baselineFixture{a: a, change: change}
	c := credentials(f.a)
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
		_ = r.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		stage := ""
		var body any
		switch {
		case strings.HasSuffix(r.URL.Path, "/runners/registration-token"):
			w.WriteHeader(201)
			body = map[string]string{"token": "synthetic-registration"}
		case strings.HasSuffix(r.URL.Path, "/actions/runner-registration"):
			claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
			body = map[string]string{"url": server.URL + "/tenant/", "token": "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."}
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
			stage = "set"
			f.sets.Add(1)
			body = scaleset.RunnerScaleSet{ID: 7, Name: f.a.setName(), RunnerGroupID: f.a.RunnerGroupID, Labels: []scaleset.Label{{Name: f.a.setName()}}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true}, Statistics: &scaleset.RunnerScaleSetStatistic{}}
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/sessions"):
			stage = "session"
			f.sessions.Add(1)
			body = scaleset.RunnerScaleSetSession{SessionID: uuid.MustParse("00000000-0000-4000-8000-000000000001"), OwnerName: f.a.setName(), MessageQueueURL: server.URL + "/queue", MessageQueueAccessToken: "synthetic-secret-queue", Statistics: &scaleset.RunnerScaleSetStatistic{}}
		case r.URL.Path == "/queue" && r.Method == "GET":
			n := f.polls.Add(1)
			stage = "poll" + strconv.Itoa(int(n))

			wantCursor := ""
			for _, e := range f.j.Events() {
				if e.Baseline != nil && e.Baseline.Stage == "ack" && e.Baseline.Outcome == "result" {
					wantCursor = strconv.Itoa(e.Baseline.MessageID)
				}
			}
			if r.URL.Query().Get("lastMessageId") != wantCursor {
				t.Error("SDK cursor did not equal the last durable ACK")
			}
			items := []any{baselineFixtureItem(f.a, "JobAvailable")}
			if n == 2 {
				items = []any{baselineFixtureItem(f.a, "JobAssigned"), baselineFixtureItem(f.a, "JobCompleted"), baselineFixtureItem(f.a, "JobStarted")}
			}
			if change != nil {
				items = change(stage+"-items", items).([]any)
			}
			data, _ := json.Marshal(items)
			body = map[string]any{"messageId": 8 + n, "messageType": "RunnerScaleSetJobMessages", "body": string(data), "statistics": scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}}
		case strings.HasPrefix(r.URL.Path, "/queue/") && r.Method == "DELETE":
			f.acks.Add(1)
			stage = "ack"
			body = baselineReply{status: 204}
		case strings.HasSuffix(r.URL.Path, "/acquirejobs") && r.Method == "POST":
			f.acquires.Add(1)
			stage = "acquire"
			body = map[string]any{"count": 1, "value": []int64{42}}
		case r.URL.Path == "/repos/"+f.a.Organization+"/"+f.a.Repository+"/actions/runs/"+strconv.FormatInt(f.a.WorkflowRunID, 10):
			if r.Header.Get("Authorization") != "Bearer "+c.VerificationToken {
				t.Error("wrong verification authority")
			}
			stage = "source"
			f.sources.Add(1)
			body = observationRun(f.a)
		default:
			if f.extra != nil && f.extra(w, r) {
				return
			}
			f.forbidden.Add(1)
			w.WriteHeader(403)
			return
		}
		if change != nil {
			body = change(stage, body)
		}
		if reply, ok := body.(baselineReply); ok {
			if reply.lost {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
				return
			}
			w.WriteHeader(reply.status)
			if reply.body == nil {
				return
			}
			body = reply.body
		}
		if raw, ok := body.(json.RawMessage); ok {
			_, _ = w.Write(raw)
		} else {
			_ = json.NewEncoder(w).Encode(body)
		}
	}))
	t.Cleanup(server.Close)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != server.Listener.Addr().String() {
			return nil, errors.New("non-fixture address denied")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	t.Cleanup(transport.CloseIdleConnections)
	outer := transport.Clone()
	outer.RegisterProtocol("https", baselineRoundTrip(func(r *http.Request) (*http.Response, error) {
		response, err := (responseBudgetTransport{inner: transport}).RoundTrip(r)
		if err == nil && f.afterResponse != nil {
			f.afterResponse(r, response)
		}
		return response, err
	}))
	httpClient := &http.Client{Transport: outer, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	retry := retryablehttp.NewClient()
	retry.RetryMax = 0
	retry.Logger = nil
	retry.HTTPClient = httpClient
	options := []scaleset.HTTPOption{scaleset.WithRetryableHTTPClint(retry), scaleset.WithLogger(slog.New(slog.DiscardHandler))}
	client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: server.URL + "/" + f.a.Organization, PersonalAccessToken: c.InstallationToken}, options...)
	if err != nil {
		t.Fatal("SDK fixture construction failed")
	}
	f.api = &SDKAPI{client: client, rest: httpClient, baseURL: server.URL, approval: f.a, credentials: c, options: options}
	f.j, err = openTestJournal(t, privateDir(t), f.a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if f.release != nil {
			f.release()
			f.release = nil
		}
		_ = f.j.Close()
	})
	if inventory != "" && f.j.Append(Event{Kind: "inventory", Digest: inventory}) != nil {
		t.Fatal("initial inventory fixture")
	}
	if f.j.Append(Event{Kind: "intent", Operation: "create"}) != nil || f.j.Append(Event{Kind: "result", Operation: "create", ID: 7}) != nil {
		t.Fatal("owned set receipt fixture failed")
	}
	f.release, err = f.j.authorize(f.a)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *baselineFixture) run(t *testing.T) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := newBaselineListenerHeld(ctx, f.a, f.j, f.api, 7)
	if err != nil {
		return err
	}
	return b.run(func(context.Context, baselineAcquisition) error { f.continuations.Add(1); return nil })
}

func TestBaselinePinnedSDKAdmission(t *testing.T) {
	for _, name := range []string{"valid", "unknown-sibling", "started-before-acquire", "wrong-acquire-count"} {
		t.Run(name, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, body any) any {
				if stage == "poll1-items" && name == "unknown-sibling" {
					return append(body.([]any), map[string]any{"messageType": "FutureJob", "runnerRequestId": 99})
				}
				if stage == "poll1-items" && name == "started-before-acquire" {
					return append(body.([]any), baselineFixtureItem(approval(), "JobStarted"))
				}
				if stage == "acquire" && name == "wrong-acquire-count" {
					return map[string]any{"count": 2, "value": []int64{42}}
				}
				return body
			})
			err := f.run(t)
			if name == "valid" {
				if err != nil || f.acks.Load() != 2 || f.acquires.Load() != 1 || f.continuations.Load() != 1 || f.forbidden.Load() != 0 {
					t.Fatalf("valid flow: err=%v ack=%d acquire=%d continuation=%d forbidden=%d", err, f.acks.Load(), f.acquires.Load(), f.continuations.Load(), f.forbidden.Load())
				}
			} else {
				wantACK, wantAcquire := int32(0), int32(0)
				if name == "wrong-acquire-count" {
					wantACK, wantAcquire = 1, 1
				}
				if err == nil || f.acks.Load() != wantACK || f.acquires.Load() != wantAcquire || f.continuations.Load() != 0 {
					t.Fatalf("unsafe admission: err=%v ack=%d acquire=%d continuation=%d", err, f.acks.Load(), f.acquires.Load(), f.continuations.Load())
				}
			}
		})
	}
}
