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
	for _, scenario := range []string{"after-ack", "acquire-loss", "jit-loss", "ack-refresh", "create-oversize", "create-error-oversize"} {
		t.Run(scenario, func(t *testing.T) {
			phase := scenario
			if scenario == "ack-refresh" {
				phase = "after-ack"
			}
			a := approval()
			j := &memoryJournal{}
			acks, acquires, jits, closes := 0, 0, 0, 0
			creates, refreshes, replacementACKs := 0, 0, 0
			set := &scaleset.RunnerScaleSet{ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID, Labels: []scaleset.Label{{Name: a.setName()}}, Statistics: &scaleset.RunnerScaleSetStatistic{}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true}}
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
				case strings.Contains(r.URL.Path, "/sessions/") && r.Method == http.MethodPatch:
					refreshes++
					body = scaleset.RunnerScaleSetSession{SessionID: uuid.MustParse("00000000-0000-4000-8000-000000000002"), OwnerName: a.setName(), MessageQueueURL: server.URL + "/replacement", MessageQueueAccessToken: "synthetic-replacement", Statistics: &scaleset.RunnerScaleSetStatistic{}}
				case r.URL.Path == "/replacement/9":
					replacementACKs++
					w.WriteHeader(http.StatusNoContent)
					return
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
					if scenario == "ack-refresh" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
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
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/runnerscalesets"):
					creates++
					var request struct {
						RunnerSetting struct {
							DisableUpdate bool `json:"disableUpdate"`
						} `json:"RunnerSetting"`
					}
					if json.NewDecoder(r.Body).Decode(&request) != nil || !request.RunnerSetting.DisableUpdate {
						t.Error("SDK create request did not disable runner updates")
					}
					body = set
					if strings.HasPrefix(scenario, "create-") {
						if scenario == "create-error-oversize" {
							w.WriteHeader(http.StatusForbidden)
						}
						body = map[string]any{"id": set.ID, "name": set.Name, "runnerGroupId": set.RunnerGroupID, "extra": strings.Repeat("synthetic-secret-body", int(responseBodyLimit)/8)}
					}
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
			retry.HTTPClient.Transport = withResponseBudget(transport)
			options := []scaleset.HTTPOption{scaleset.WithRetryableHTTPClint(retry)}
			client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: server.URL + "/fixture-org", PersonalAccessToken: "synthetic-installation"}, options...)
			if err != nil {
				t.Fatal("SDK fixture construction failed")
			}
			api := fixtureSDK{&SDKAPI{client: client, options: options}}
			d := Driver{a, j, api}
			createErr := d.Run(context.Background(), "create")
			if strings.HasPrefix(scenario, "create-") {
				if !errors.Is(createErr, ErrQuarantine) || creates != 1 || !replay(j.Events()).uncertain {
					t.Fatal("oversize SDK effect did not quarantine")
				}
				if d.Run(context.Background(), "create") == nil || creates != 1 {
					t.Fatal("oversize response caused side effect retry")
				}
				data, _ := json.Marshal(j.Events())
				if strings.Contains(string(data), "synthetic-secret") {
					t.Fatal("oversize SDK body leaked")
				}
				return
			}
			if createErr != nil {
				t.Fatal("SDK create failed")
			}
			err = d.Run(context.Background(), phase)
			if scenario == "ack-refresh" {
				if !errors.Is(err, ErrQuarantine) || acks != 1 || refreshes != 0 || replacementACKs != 0 || closes != 0 || !replay(j.Events()).uncertain {
					t.Fatal("SDK refreshed and acknowledged through a replaced session")
				}
				return
			}
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

func TestDriverDrainThroughPinnedSDKAndPollHook(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	set := &scaleset.RunnerScaleSet{
		ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID,
		Labels:        []scaleset.Label{{Name: a.setName()}},
		RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		Statistics:    &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
	}
	var polls, acks, acquires, closes int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/actions/runners/registration-token") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "fixture-admin"})
		case strings.HasSuffix(r.URL.Path, "/actions/runner-registration") && r.Method == http.MethodPost:
			claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"url": server.URL, "token": "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."})
		case strings.HasSuffix(r.URL.Path, "/sessions") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(scaleset.RunnerScaleSetSession{
				SessionID: uuid.MustParse("00000000-0000-4000-8000-000000000011"), OwnerName: a.setName(),
				MessageQueueURL: server.URL + "/queue", MessageQueueAccessToken: "fixture-queue",
				RunnerScaleSet: set, Statistics: set.Statistics,
			})
		case r.URL.Path == "/queue" && r.Method == http.MethodGet:
			if polls == 0 {
				if r.Header.Get(scaleset.HeaderScaleSetMaxCapacity) != "1" {
					t.Errorf("first poll capacity = %q, want 1", r.Header.Get(scaleset.HeaderScaleSetMaxCapacity))
				}
				polls++
				jobs, _ := json.Marshal([]any{map[string]any{"messageType": "JobAvailable", "runnerRequestId": 41, "workflowRunId": a.WorkflowRunID, "ownerName": a.Organization, "repositoryName": a.Repository}})
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"messageId": 17, "messageType": "RunnerScaleSetJobMessages", "body": string(jobs),
					"statistics": map[string]int{"totalAvailableJobs": 1, "totalAcquiredJobs": 0, "totalAssignedJobs": 1, "totalRunningJobs": 0, "totalRegisteredRunners": 1, "totalBusyRunners": 0, "totalIdleRunners": 1},
				})
				return
			}
			if r.Header.Get(scaleset.HeaderScaleSetMaxCapacity) != "0" {
				t.Errorf("next poll capacity = %q, want 0", r.Header.Get(scaleset.HeaderScaleSetMaxCapacity))
			}
			polls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statistics": map[string]int{"totalAvailableJobs": 0, "totalAcquiredJobs": 0, "totalAssignedJobs": 0, "totalRunningJobs": 0, "totalRegisteredRunners": 1, "totalBusyRunners": 0, "totalIdleRunners": 1},
			})
		case r.URL.Path == "/queue/17" && r.Method == http.MethodDelete:
			acks++
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/acquirejobs") && r.Method == http.MethodPost:
			acquires++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "value": []int64{41}})
		case strings.Contains(r.URL.Path, "/sessions/") && r.Method == http.MethodDelete:
			closes++
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/runnerscalesets/7") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(set)
		case strings.HasSuffix(r.URL.Path, "/agents") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "value": []any{map[string]any{"id": 19, "name": a.workerName(), "runnerScaleSetId": 7}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	buildAPI := func(hook *drainPollHook) (*SDKAPI, error) {
		retry := retryablehttp.NewClient()
		retry.RetryMax = 0
		retry.Logger = nil
		retry.HTTPClient.Timeout = time.Second
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != server.Listener.Addr().String() {
				return nil, errors.New("non-fixture address denied")
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
		wrappers := []func(http.RoundTripper) http.RoundTripper(nil)
		if hook != nil {
			wrappers = append(wrappers, func(inner http.RoundTripper) http.RoundTripper {
				hook.inner = inner
				return hook
			})
		}
		retry.HTTPClient.Transport = withResponseBudget(transport, wrappers...)
		options := []scaleset.HTTPOption{scaleset.WithRetryableHTTPClint(retry), scaleset.WithLogger(slog.New(slog.DiscardHandler))}
		client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: server.URL + "/fixture-org", PersonalAccessToken: "synthetic-installation"}, options...)
		if err != nil {
			return nil, err
		}
		return &SDKAPI{client: client, rest: retry.HTTPClient, baseURL: server.URL + "/api/v3", approval: a, options: options}, nil
	}
	base, err := buildAPI(nil)
	if err != nil {
		t.Fatal(err)
	}
	base.drainClientFactory = buildAPI
	api := fixtureSDK{base}
	j := &memoryJournal{events: []Event{{Kind: "phase", Operation: "create"}, {Kind: "intent", Operation: "create"}, {Kind: "result", Operation: "create", ID: 7}}}
	d := Driver{Approval: a, Journal: j, API: api}
	if err := d.Run(context.Background(), "drain"); err != nil {
		t.Fatalf("pinned SDK drain: %v", err)
	}
	if polls != 2 || acks != 1 || acquires != 1 || closes != 1 {
		t.Fatalf("pinned SDK effects polls=%d ack=%d acquire=%d close=%d", polls, acks, acquires, closes)
	}
	var observed bool
	var drainPhaseSequence, drainPhaseSetID, observationSequence int
	for _, event := range j.Events() {
		if event.Kind == "phase" && event.Operation == "drain" {
			drainPhaseSequence = event.Sequence
			drainPhaseSetID = event.ID
		}
		if event.Kind == "observation" && event.Operation == "drain" && event.Drain != nil && event.Drain.Outcome == drainOutcomeObserved {
			observed = true
			observationSequence = event.Drain.Sequence
			if !sameDrainRunnerPartition(event.Drain.NextPoll.Statistics, event.Drain.Before.Statistics) {
				t.Fatal("pinned SDK drain accepted a withdrawn-poll runner partition mismatch")
			}
		}
	}
	if !observed {
		t.Fatal("pinned SDK drain did not retain observed outcome")
	}
	if drainPhaseSequence <= 0 || drainPhaseSetID != set.ID || observationSequence != drainPhaseSequence {
		t.Fatalf("pinned SDK drain binding = phase sequence %d SetID %d, observation %d; want exact phase sequence and SetID %d", drainPhaseSequence, drainPhaseSetID, observationSequence, set.ID)
	}
}
