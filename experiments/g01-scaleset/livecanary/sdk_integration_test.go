package livecanary

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
)

type fixtureSDK struct{ *SDKAPI }

func (f fixtureSDK) Preflight(context.Context, Approval) error        { return nil } // Authority HTTP tested separately.
func (f fixtureSDK) Inventory(context.Context) (string, error)        { return strings.Repeat("0", 64), nil }
func (f fixtureSDK) VerifyRun(context.Context, Approval, int64) error { return nil }

// Exercise the new live driver through the actual pinned SDK. This synthetic
// transport is permanently fenced to its own loopback server, like the original
// offline spike; no endpoint flag or environment can repoint it to GitHub.
func TestDriverBarriersThroughPinnedSDK(t *testing.T) {
	for _, phase := range []string{"after-ack", "acquire-loss", "jit-loss"} {
		t.Run(phase, func(t *testing.T) {
			a := approval()
			j := &memoryJournal{}
			acks, acquires, jits, closes := 0, 0, 0, 0
			set := &scaleset.RunnerScaleSet{ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID, Labels: []scaleset.Label{{Name: a.setName()}}, Statistics: &scaleset.RunnerScaleSetStatistic{}}
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body any
				switch {
				case strings.HasSuffix(r.URL.Path, "/runners/registration-token"):
					w.WriteHeader(http.StatusCreated)
					body = map[string]string{"token": "synthetic-registration"}
				case strings.HasSuffix(r.URL.Path, "/actions/runner-registration"):
					claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
					w.WriteHeader(http.StatusCreated)
					body = map[string]string{"url": server.URL + "/tenant/", "token": "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."}
				case strings.HasSuffix(r.URL.Path, "/sessions"):
					body = scaleset.RunnerScaleSetSession{SessionID: uuid.MustParse("00000000-0000-4000-8000-000000000001"), OwnerName: a.setName(), MessageQueueURL: server.URL + "/queue", MessageQueueAccessToken: "synthetic-queue", Statistics: &scaleset.RunnerScaleSetStatistic{}}
				case strings.Contains(r.URL.Path, "/sessions/") && r.Method == http.MethodDelete:
					closes++
					w.WriteHeader(http.StatusNoContent)
					return
				case r.URL.Path == "/queue":
					if r.Header.Get(scaleset.HeaderScaleSetMaxCapacity) != "1" {
						t.Error("capacity not bounded")
					}
					jobs, _ := json.Marshal([]any{map[string]any{"messageType": "JobAvailable", "runnerRequestId": 42, "workflowRunId": a.WorkflowRunID, "ownerName": a.Organization, "repositoryName": a.Repository}})
					body = map[string]any{"messageId": 9, "messageType": "RunnerScaleSetJobMessages", "body": string(jobs), "statistics": map[string]int{"totalAssignedJobs": 1}}
				case r.URL.Path == "/queue/9":
					acks++
					w.WriteHeader(http.StatusNoContent)
					return
				case strings.HasSuffix(r.URL.Path, "/acquirejobs"):
					acquires++
					body = map[string]any{"count": 1, "value": []int64{42}}
				case strings.HasSuffix(r.URL.Path, "/generatejitconfig"):
					jits++
					body = scaleset.RunnerScaleSetJitRunnerConfig{EncodedJITConfig: "synthetic-secret-jit", Runner: &scaleset.RunnerReference{ID: 8, Name: a.workerName(), RunnerScaleSetID: 7}}
				case strings.HasSuffix(r.URL.Path, "/runners"):
					body = map[string]any{"count": 0, "value": []any{}}
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/runnerscalesets"):
					body = map[string]any{"count": 0, "value": []any{}}
				default:
					body = set
				}
				_ = json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			retry := retryablehttp.NewClient()
			retry.RetryMax = 0
			retry.Logger = nil
			retry.HTTPClient.Timeout = time.Second
			transport := retry.HTTPClient.Transport.(*http.Transport)
			transport.Proxy = nil
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != server.Listener.Addr().String() {
					return nil, errors.New("non-fixture address denied")
				}
				return (&net.Dialer{}).DialContext(ctx, network, address)
			}
			defer transport.CloseIdleConnections()
			options := []scaleset.HTTPOption{scaleset.WithRetryableHTTPClint(retry)}
			client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: server.URL + "/fixture-org", PersonalAccessToken: "synthetic-installation"}, options...)
			if err != nil {
				t.Fatal("SDK fixture construction failed")
			}
			api := fixtureSDK{&SDKAPI{client: client, options: options}}
			d := Driver{a, j, api}
			if d.Run(context.Background(), "create") != nil {
				t.Fatal("SDK create failed")
			}
			err = d.Run(context.Background(), phase)
			switch phase {
			case "after-ack":
				if err != nil || acks != 1 || acquires != 0 || closes != 1 {
					t.Fatal("ACK barrier failed through SDK")
				}
			case "acquire-loss":
				if !errors.Is(err, ErrQuarantine) || acks != 1 || acquires != 1 || closes != 0 {
					t.Fatal("acquisition barrier failed through SDK")
				}
			case "jit-loss":
				if !errors.Is(err, ErrQuarantine) || jits != 1 {
					t.Fatal("JIT barrier failed through SDK")
				}
			}
			data, _ := json.Marshal(j.Events())
			if strings.Contains(string(data), "synthetic-secret") {
				t.Fatal("SDK response secret persisted")
			}
		})
	}
}
