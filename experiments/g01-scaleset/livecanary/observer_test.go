package livecanary

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/hashicorp/go-retryablehttp"
)

// The only dialable address comes from this private loopback fixture. No flag,
// environment variable or credential can repoint it to a live service.
func observationFixture(t *testing.T, handler http.HandlerFunc, intercept ...func(http.ResponseWriter, *http.Request) bool) *SDKAPI {
	t.Helper()
	a := approval()
	c := credentials(a)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(intercept) != 0 && intercept[0](w, r) {
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/runners/registration-token"):
			if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer "+c.InstallationToken {
				t.Error("SDK bootstrap authority mismatch")
			}
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"token":"synthetic-registration"}`))
		case strings.HasSuffix(r.URL.Path, "/actions/runner-registration"):
			if r.Method != "POST" || r.Header.Get("Authorization") != "RemoteAuth synthetic-registration" {
				t.Error("SDK admin authority mismatch")
			}
			claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
			_ = json.NewEncoder(w).Encode(map[string]string{"url": server.URL + "/tenant/", "token": "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."})
		default:
			handler(w, r)
		}
	}))
	t.Cleanup(server.Close)
	transport := http.DefaultTransport.(*http.Transport).Clone()
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
	httpClient := &http.Client{Transport: withResponseBudget(transport), Timeout: operationTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	retry := retryablehttp.NewClient()
	retry.RetryMax = 0
	retry.Logger = nil
	retry.HTTPClient = httpClient
	options := []scaleset.HTTPOption{scaleset.WithRetryableHTTPClint(retry), scaleset.WithLogger(slog.New(slog.DiscardHandler))}
	client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: server.URL + "/" + a.Organization, PersonalAccessToken: c.InstallationToken}, options...)
	if err != nil {
		t.Fatal("fixture SDK construction failed")
	}
	return &SDKAPI{client: client, rest: httpClient, baseURL: server.URL, approval: a, credentials: c, options: options}
}

func TestObserveSDKRunnerIdentityAndProvenance(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		outcome    observationOutcome
		good       bool
	}{
		{"exact", `{"id":8,"name":"fixture-runner","runnerScaleSetId":7}`, 200, observationPresent, true},
		{"not-found", `{"typeName":"AgentNotFoundException","message":"synthetic-secret"}`, 404, observationNotFound, true},
		{"forbidden-category", `{"typeName":"AgentNotFoundException","message":"synthetic-secret"}`, 403, observationUnresolved, false},
		{"server-category", `{"typeName":"AgentNotFoundException","message":"synthetic-secret"}`, 500, observationUnresolved, false},
		{"generic404", `{"message":"AgentNotFoundException synthetic-secret"}`, 404, observationUnresolved, false},
		{"nil", `null`, 200, observationUnresolved, false},
		{"wrong-id", `{"id":9,"name":"fixture-runner","runnerScaleSetId":7}`, 200, observationUnresolved, false},
		{"wrong-name", `{"id":8,"name":"other","runnerScaleSetId":7}`, 200, observationUnresolved, false},
		{"wrong-set", `{"id":8,"name":"fixture-runner","runnerScaleSetId":9}`, 200, observationUnresolved, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/tenant/_apis/distributedtask/pools/0/agents/8" || r.URL.Query().Get("api-version") != "6.0-preview" {
					t.Error("wrong SDK target or name fallback")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			got, err := api.observeSDKRunner(context.Background(), 8, "fixture-runner", 7)
			if (err == nil) != tc.good || got.Response.Outcome != tc.outcome || got.Response.Status != tc.status {
				t.Fatalf("observation mismatch: outcome=%q status=%d error=%v", got.Response.Outcome, got.Response.Status, err)
			}
			if tc.name == "exact" && (got.ID != 8 || got.Name != "fixture-runner" || got.ScaleSetID != 7) {
				t.Fatal("SDK facts lost")
			}
			assertObservationSecretSafe(t, got, err)
		})
	}
}
func assertObservationSecretSafe(t *testing.T, got any, err error) {
	t.Helper()
	data, _ := json.Marshal(got)
	if strings.Contains(string(data), "synthetic-secret") || (err != nil && strings.Contains(err.Error(), "synthetic-secret")) {
		t.Fatal("raw remote data escaped")
	}
}
func observationRun(a Approval) map[string]any {
	repo := map[string]any{"id": a.RepositoryID, "private": true, "fork": false}
	head := map[string]any{"id": a.RepositoryID, "private": true, "fork": false}
	return map[string]any{"id": a.WorkflowRunID, "head_sha": a.WorkflowSHA, "event": "workflow_dispatch", "path": a.WorkflowPath, "run_attempt": 1, "repository": repo, "head_repository": head}
}
func observationJob(a Approval) map[string]any {
	return map[string]any{"id": int64(9007199254740995), "run_id": a.WorkflowRunID, "run_attempt": 1, "head_sha": a.WorkflowSHA, "status": "completed", "conclusion": "success", "runner_id": int64(9007199254740993), "runner_name": "fixture-runner", "runner_group_id": a.RunnerGroupID}
}
func TestObserveRESTExactFacts(t *testing.T) {
	a := approval()
	api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("REST observation wrote")
		}
		var body any
		token := credentials(a).VerificationToken
		switch r.URL.Path {
		case "/orgs/fixture-org/actions/runners/9007199254740993":
			token = credentials(a).InstallationToken
			body = map[string]any{"id": int64(9007199254740993), "name": "fixture-runner", "status": "online", "busy": false}
		case "/repos/fixture-org/canary/actions/runs/5":
			body = observationRun(a)
		case "/repos/fixture-org/canary/actions/runs/5/attempts/1/jobs":
			if r.URL.Query().Get("per_page") != "2" || r.URL.Query().Get("page") != "1" {
				t.Error("unbounded attempt lookup")
			}
			body = map[string]any{"total_count": 1, "jobs": []any{observationJob(a)}}
		case "/repos/fixture-org/canary/actions/jobs/9007199254740995":
			body = observationJob(a)
		default:
			t.Error("unexpected REST endpoint")
			w.WriteHeader(500)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Error("REST authority/version mismatch")
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	runner, err := api.observeRESTRunner(context.Background(), 9007199254740993, "fixture-runner")
	if err != nil || runner.ID != 9007199254740993 || runner.Busy == nil || *runner.Busy || runner.Response.Outcome != observationPresent {
		t.Error("REST runner facts unavailable")
	}
	job, err := api.observeRESTJob(context.Background(), 1, 0)
	if err != nil || job.ID != 9007199254740995 || job.RunnerID == nil || *job.RunnerID != 9007199254740993 || job.Attempt != 1 || job.Conclusion == nil || *job.Conclusion != "success" {
		t.Error("exact attempt/job facts unavailable or conflated")
	}
}
