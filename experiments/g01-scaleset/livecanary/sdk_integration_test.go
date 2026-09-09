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
	"strings"
	"sync"
	"sync/atomic"
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

type countingFixtureSDK struct {
	fixtureSDK
	verifyCalls atomic.Int32
}

func (f *countingFixtureSDK) VerifyRun(context.Context, Approval, int64) error {
	f.verifyCalls.Add(1)
	return nil
}

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
	var polls, acks, acquires, closes, snapshotReads int
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
			snapshotReads++
			snapshot := *set
			if snapshotReads == 2 {
				stats := *set.Statistics
				stats.TotalAcquiredJobs = 1
				snapshot.Statistics = &stats
			}
			_ = json.NewEncoder(w).Encode(&snapshot)
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
	directory := privateDir(t)
	j, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range drainReplayPrefix()[:3] {
		if err := j.Append(event); err != nil {
			_ = j.Close()
			t.Fatalf("append create prefix: %v", err)
		}
	}
	d := Driver{Approval: a, Journal: j, API: api}
	if err := d.Run(context.Background(), "drain"); err != nil {
		t.Fatalf("pinned SDK drain: %v", err)
	}
	if polls != 2 || acks != 1 || acquires != 1 || closes != 1 || snapshotReads != 2 {
		t.Fatalf("pinned SDK effects polls=%d ack=%d acquire=%d close=%d snapshots=%d", polls, acks, acquires, closes, snapshotReads)
	}
	var observed bool
	var drainPhaseSequence, drainPhaseSetID, observationSequence int
	var snapshotStages []string
	for _, event := range j.Events() {
		if event.Kind == "phase" && event.Operation == "drain" {
			drainPhaseSequence = event.Sequence
			drainPhaseSetID = event.ID
		}
		if event.DrainSnapshot != nil {
			snapshotStages = append(snapshotStages, event.DrainSnapshotStage)
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
	if len(snapshotStages) != 2 || snapshotStages[0] != "before" || snapshotStages[1] != "after" {
		t.Fatalf("pinned SDK drain snapshot stages = %v, want before/after", snapshotStages)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if state := replayWithApproval(reopened.Events(), &a); state.uncertain {
		t.Fatalf("legitimate after job-counter change retained replay uncertainty: %+v", state)
	}
}

func TestPinnedSDKDrainRejectsAmbiguousEmbeddedJobIdentityBeforeEffects(t *testing.T) {
	a := approval()
	for _, test := range []struct {
		name       string
		body       string
		wantReject bool
	}{
		// The pinned SDK's ordinary decoder accepts the case-folded duplicate
		// and keeps the later value. The adapter must reject that ambiguous wire
		// before the listener can ACK or acquire the message.
		{name: "casefolded identity", body: `[{"messageType":"JobAvailable","runnerRequestId":41,"workflowRunId":5,"ownerName":"foreign-owner","OwnerName":"fixture-org","repositoryName":"canary"}]`, wantReject: true},
		{name: "duplicate identity", body: `[{"messageType":"JobAvailable","runnerRequestId":41,"workflowRunId":5,"ownerName":"fixture-org","ownerName":"fixture-org","repositoryName":"canary"}]`, wantReject: true},
		{name: "malformed body", body: `[{"messageType":"JobAvailable","runnerRequestId":41`, wantReject: true},
		{name: "legitimate SDK body", body: pinnedDrainJobBody(a)},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, _, session, hook := newPinnedDrainSession(t, a, test.body)
			observation, err := runDrainListener(context.Background(), session, 7, hook)
			if test.wantReject {
				if !errors.Is(err, ErrQuarantine) || observation.Outcome == drainOutcomeObserved {
					t.Fatalf("ambiguous embedded job identity was accepted: observation=%+v err=%v", observation, err)
				}
				if fixture.acks.Load() != 0 || fixture.acquires.Load() != 0 {
					t.Fatalf("ambiguous embedded job identity reached effects: polls=%d acks=%d acquires=%d", fixture.polls.Load(), fixture.acks.Load(), fixture.acquires.Load())
				}
				return
			}
			if err != nil || observation.Outcome != drainOutcomeObserved || fixture.acks.Load() != 1 || fixture.acquires.Load() != 1 {
				t.Fatalf("legitimate embedded job path failed: observation=%+v err=%v polls=%d acks=%d acquires=%d", observation, err, fixture.polls.Load(), fixture.acks.Load(), fixture.acquires.Load())
			}
		})
	}
}

func TestPinnedSDKDrainBindsPollCursorBeforeInnerCall(t *testing.T) {
	t.Run("first poll requires zero cursor", func(t *testing.T) {
		a := approval()
		fixture, _, session, hook := newPinnedDrainSession(t, a, pinnedDrainJobBody(a))
		c := newPinnedDrainClient(session, hook)
		hook.releaseResponse()

		if _, err := c.GetMessage(context.Background(), 9, drainInitialCapacity); !errors.Is(err, ErrQuarantine) {
			t.Fatalf("wrong first cursor = %v, want quarantine", err)
		}
		if fixture.polls.Load() != 0 {
			t.Fatalf("wrong first cursor reached inner SDK: polls=%d", fixture.polls.Load())
		}
	})

	t.Run("second poll requires acknowledged message cursor", func(t *testing.T) {
		a := approval()
		fixture, _, session, hook := newPinnedDrainSession(t, a, pinnedDrainJobBody(a))
		c := newPinnedDrainClient(session, hook)
		hook.releaseResponse()

		if _, err := c.GetMessage(context.Background(), 0, drainInitialCapacity); err != nil {
			t.Fatalf("valid first poll: %v", err)
		}
		if _, err := c.GetMessage(context.Background(), 0, drainWithdrawnCapacity); !errors.Is(err, ErrQuarantine) {
			t.Fatalf("wrong second cursor = %v, want quarantine", err)
		}
		if fixture.polls.Load() != 1 {
			t.Fatalf("wrong second cursor reached inner SDK: polls=%d cursors=%v", fixture.polls.Load(), fixture.cursorsSnapshot())
		}
	})

	t.Run("legitimate SDK cursor path remains bounded", func(t *testing.T) {
		a := approval()
		fixture, _, session, hook := newPinnedDrainSession(t, a, pinnedDrainJobBody(a))
		c := newPinnedDrainClient(session, hook)
		hook.releaseResponse()

		message, err := c.GetMessage(context.Background(), 0, drainInitialCapacity)
		if err != nil || message == nil {
			t.Fatalf("valid first poll = message %v err %v", message, err)
		}
		if err := c.DeleteMessage(context.Background(), message.MessageID); err != nil {
			t.Fatalf("valid ACK: %v", err)
		}
		if _, err := c.AcquireJobs(context.Background(), []int64{41}); err != nil {
			t.Fatalf("valid acquisition: %v", err)
		}
		if message, err = c.GetMessage(context.Background(), 17, drainWithdrawnCapacity); err != nil || message != nil {
			t.Fatalf("valid second poll = message %v err %v", message, err)
		}
		if fixture.polls.Load() != 2 || fixture.acks.Load() != 1 || fixture.acquires.Load() != 1 {
			t.Fatalf("legitimate SDK effects polls=%d acks=%d acquires=%d cursors=%v capacities=%v", fixture.polls.Load(), fixture.acks.Load(), fixture.acquires.Load(), fixture.cursorsSnapshot(), fixture.capacitiesSnapshot())
		}
	})
}

func TestPinnedSDKDrainMatchesWireBeforeVerifyRun(t *testing.T) {
	a := approval()
	body := `[{"messageType":"JobAvailable","runnerRequestId":41,"workflowRunId":5,"ownerName":"foreign-owner","OwnerName":"fixture-org","repositoryName":"canary"}]`
	fixture, base, session, hook := newPinnedDrainSession(t, a, body)
	api := &countingFixtureSDK{fixtureSDK: fixtureSDK{base}}
	d := &Driver{Approval: a, Journal: &memoryJournal{}, API: api}
	c := &journaledDrainClient{d: d, inner: session, sessionID: session.Session().SessionID.String(), hook: hook}
	hook.releaseResponse()

	if _, err := c.GetMessage(context.Background(), 0, drainInitialCapacity); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("ambiguous embedded job identity = %v, want quarantine", err)
	}
	if api.verifyCalls.Load() != 0 {
		t.Fatalf("wire mismatch reached VerifyRun: calls=%d", api.verifyCalls.Load())
	}
	if fixture.acks.Load() != 0 || fixture.acquires.Load() != 0 {
		t.Fatalf("wire mismatch reached effects: acks=%d acquires=%d", fixture.acks.Load(), fixture.acquires.Load())
	}
}

func TestPinnedSDKDrainRejectsNonEOFPollReadError(t *testing.T) {
	a := approval()
	fixture, _, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{pollReadError: io.ErrUnexpectedEOF})
	hook.releaseResponse()
	if _, err := session.GetMessage(context.Background(), 0, drainInitialCapacity); err == nil {
		t.Fatal("non-EOF poll read error was accepted by the pinned SDK")
	}
	if _, known := hook.pollStatistics(1); known {
		t.Fatalf("non-EOF poll read error became known wire facts: fixture polls=%d", fixture.polls.Load())
	}
	if fixture.acks.Load() != 0 || fixture.acquires.Load() != 0 {
		t.Fatalf("non-EOF poll read error reached effects: ack=%d acquire=%d", fixture.acks.Load(), fixture.acquires.Load())
	}
}

func TestPinnedSDKDrainBindsACKToPhysicalDelete(t *testing.T) {
	a := approval()
	fixture, base, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{
		acceptAnyACK: true,
		mutateRequest: func(req *http.Request) {
			if req.Method == http.MethodDelete && strings.HasPrefix(req.URL.Path, "/queue/") {
				req.URL.Path = "/queue/999"
				req.URL.RawPath = ""
			}
		},
	})
	hook.releaseResponse()
	j := &memoryJournal{}
	api := &countingFixtureSDK{fixtureSDK: fixtureSDK{base}}
	d := &Driver{Approval: a, Journal: j, API: api}
	client := &journaledDrainClient{d: d, inner: session, sessionID: session.Session().SessionID.String(), setID: 7, hook: hook}
	message, err := client.GetMessage(context.Background(), 0, drainInitialCapacity)
	if err != nil || message == nil {
		t.Fatalf("pinned SDK setup poll = message %v err %v", message, err)
	}
	if err := client.DeleteMessage(context.Background(), message.MessageID); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("wrong physical ACK = %v, want quarantine", err)
	}
	if fixture.acks.Load() != 1 {
		t.Fatalf("wrong physical ACK did not reach successful fixture endpoint: ack=%d", fixture.acks.Load())
	}
}

func TestPinnedSDKDrainRejectsAmbiguousRunnerSnapshot(t *testing.T) {
	a := approval()
	runnerBody := `{"count":1,"value":[{"id":19,"name":"` + a.workerName() + `","runnerScaleSetId":7,"RunnerScaleSetId":7}]}`
	fixture, base, _, _ := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{runnerBody: runnerBody})
	j := &memoryJournal{}
	d := &Driver{Approval: a, Journal: j, API: fixtureSDK{base}}
	if _, err := d.drainSnapshot(context.Background(), 7, "before"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("ambiguous runner snapshot = %v, want quarantine (fixture polls=%d)", err, fixture.polls.Load())
	}
}

func TestPinnedSDKDrainRejectsAmbiguousSessionResponse(t *testing.T) {
	a := approval()
	name := a.setName()
	sessionBody := `{"sessionId":"00000000-0000-4000-8000-000000000021","SessionID":"00000000-0000-4000-8000-000000000021","ownerName":"` + name + `","runnerScaleSet":{"id":7,"name":"` + name + `","runnerGroupId":3,"labels":[{"name":"` + name + `","type":"System"}],"RunnerSetting":{"disableUpdate":true},"statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1}},"messageQueueUrl":"QUEUE_URL","MessageQueueURL":"QUEUE_URL","messageQueueAccessToken":"fixture-queue","statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1}}`
	fixture, _, session, _ := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{sessionBody: sessionBody, allowOpenError: true})
	if fixture.openErr == nil || session != nil {
		t.Fatalf("ambiguous session response accepted: err=%v session=%v", fixture.openErr, session)
	}
}

func pinnedDrainJobBody(a Approval) string {
	data, _ := json.Marshal([]map[string]any{{
		"messageType":     "JobAvailable",
		"runnerRequestId": int64(41),
		"workflowRunId":   a.WorkflowRunID,
		"ownerName":       a.Organization,
		"repositoryName":  a.Repository,
	}})
	return string(data)
}

type pinnedDrainOptions struct {
	firstPollBody  string
	withdrawnBody  string
	acquireBody    string
	verifyBody     string
	sessionBody    string
	runnerBody     string
	allowOpenError bool
	acceptAnyACK   bool
	pollReadError  error
	snapshotBodies []string
	mutateRequest  func(*http.Request)
}

type pinnedDrainFixture struct {
	server         *httptest.Server
	body           string
	firstPollBody  string
	withdrawnBody  string
	acquireBody    string
	verifyBody     string
	runnerBody     string
	openErr        error
	snapshotBodies []string
	polls          atomic.Int32
	acks           atomic.Int32
	acquires       atomic.Int32
	verifyCalls    atomic.Int32
	snapshotReads  atomic.Int32
	mu             sync.Mutex
	cursors        []string
	capacity       []string
}

func newPinnedDrainSession(t *testing.T, a Approval, body string) (*pinnedDrainFixture, *SDKAPI, Session, *drainPollHook) {
	return newPinnedDrainSessionOptions(t, a, body, pinnedDrainOptions{})
}

func newPinnedDrainSessionOptions(t *testing.T, a Approval, body string, options pinnedDrainOptions) (*pinnedDrainFixture, *SDKAPI, Session, *drainPollHook) {
	t.Helper()
	fixture := &pinnedDrainFixture{
		body: body, firstPollBody: options.firstPollBody, withdrawnBody: options.withdrawnBody,
		acquireBody: options.acquireBody, verifyBody: options.verifyBody,
		runnerBody:     options.runnerBody,
		snapshotBodies: append([]string(nil), options.snapshotBodies...),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/actions/runners/registration-token") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "fixture-registration"})
		case strings.HasSuffix(r.URL.Path, "/actions/runner-registration") && r.Method == http.MethodPost:
			claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"url":   fixture.server.URL,
				"token": "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + ".",
			})
		case strings.HasSuffix(r.URL.Path, "/sessions") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			if options.sessionBody != "" {
				_, _ = io.WriteString(w, strings.ReplaceAll(options.sessionBody, "QUEUE_URL", fixture.server.URL+"/queue"))
				return
			}
			_ = json.NewEncoder(w).Encode(scaleset.RunnerScaleSetSession{
				SessionID: uuid.MustParse("00000000-0000-4000-8000-000000000021"), OwnerName: a.setName(),
				MessageQueueURL: fixture.server.URL + "/queue", MessageQueueAccessToken: "fixture-queue",
				RunnerScaleSet: pinnedDrainScaleSet(a),
				Statistics:     &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
			})
		case strings.HasSuffix(r.URL.Path, "/actions/runs/5") && r.Method == http.MethodGet:
			fixture.verifyCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if fixture.verifyBody != "" {
				_, _ = io.WriteString(w, fixture.verifyBody)
				return
			}
			_ = json.NewEncoder(w).Encode(pinnedDrainRun(a))
		case r.URL.Path == "/queue" && r.Method == http.MethodGet:
			fixture.polls.Add(1)
			fixture.mu.Lock()
			fixture.cursors = append(fixture.cursors, r.URL.Query().Get("lastMessageId"))
			fixture.capacity = append(fixture.capacity, r.Header.Get(scaleset.HeaderScaleSetMaxCapacity))
			fixture.mu.Unlock()
			if fixture.polls.Load() == 1 {
				if fixture.firstPollBody != "" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, fixture.firstPollBody)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"messageId": 17, "messageType": "RunnerScaleSetJobMessages", "body": fixture.body,
					"statistics": pinnedDrainStatistics(1, 1),
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			if fixture.withdrawnBody != "" {
				_, _ = io.WriteString(w, fixture.withdrawnBody)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"statistics": pinnedDrainStatistics(0, 0)})
		case options.acceptAnyACK && strings.HasPrefix(r.URL.Path, "/queue/") && r.Method == http.MethodDelete:
			fixture.acks.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/queue/17" && r.Method == http.MethodDelete:
			fixture.acks.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/acquirejobs") && r.Method == http.MethodPost:
			fixture.acquires.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if fixture.acquireBody != "" {
				_, _ = io.WriteString(w, fixture.acquireBody)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "value": []int64{41}})
		case strings.Contains(r.URL.Path, "/sessions/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/runnerscalesets/7") && r.Method == http.MethodGet:
			read := int(fixture.snapshotReads.Add(1))
			if read <= len(fixture.snapshotBodies) && fixture.snapshotBodies[read-1] != "" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, fixture.snapshotBodies[read-1])
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(pinnedDrainSnapshot(a))
		case strings.HasSuffix(r.URL.Path, "/agents") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if fixture.runnerBody != "" {
				_, _ = io.WriteString(w, fixture.runnerBody)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "value": []any{map[string]any{"id": 19, "name": a.workerName(), "runnerScaleSetId": 7}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fixture.server.Close)

	buildAPI := func(hook *drainPollHook) (*SDKAPI, error) {
		retry := retryablehttp.NewClient()
		retry.RetryMax = 0
		retry.Logger = nil
		retry.HTTPClient.Timeout = time.Second
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != fixture.server.Listener.Addr().String() {
				return nil, errors.New("non-fixture address denied")
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
		wrappers := []func(http.RoundTripper) http.RoundTripper(nil)
		if hook != nil {
			if options.pollReadError != nil {
				wrappers = append(wrappers, func(inner http.RoundTripper) http.RoundTripper {
					return sdkResponseBodyFaultRoundTripper{inner: inner, path: "/queue", err: options.pollReadError}
				})
			}
			wrappers = append(wrappers, func(inner http.RoundTripper) http.RoundTripper {
				hook.inner = inner
				return hook
			})
		}
		if options.mutateRequest != nil {
			wrappers = append(wrappers, func(inner http.RoundTripper) http.RoundTripper {
				return sdkRequestMutationRoundTripper{inner: inner, mutate: options.mutateRequest}
			})
		}
		retry.HTTPClient.Transport = withResponseBudget(transport, wrappers...)
		options := []scaleset.HTTPOption{scaleset.WithRetryableHTTPClint(retry), scaleset.WithLogger(slog.New(slog.DiscardHandler))}
		client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: fixture.server.URL + "/fixture-org", PersonalAccessToken: "synthetic-installation"}, options...)
		if err != nil {
			return nil, err
		}
		return &SDKAPI{client: client, rest: retry.HTTPClient, baseURL: fixture.server.URL + "/api/v3", approval: a, credentials: credentials(a), options: options}, nil
	}
	base, err := buildAPI(nil)
	if err != nil {
		t.Fatal(err)
	}
	base.drainClientFactory = buildAPI
	hook := newDrainPollHook("")
	session, err := base.OpenDrainSession(context.Background(), 7, a.setName(), hook)
	if err != nil {
		fixture.openErr = err
		if !options.allowOpenError {
			t.Fatal(err)
		}
		return fixture, base, nil, hook
	}
	return fixture, base, session, hook
}

type sdkResponseBodyFaultRoundTripper struct {
	inner http.RoundTripper
	path  string
	err   error
}

func (t sdkResponseBodyFaultRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.inner.RoundTrip(req)
	if err != nil || response == nil || req.URL == nil || req.URL.Path != t.path || response.Body == nil {
		return response, err
	}
	response.Body = &sdkResponseBodyFault{source: response.Body, err: t.err}
	return response, nil
}

type sdkResponseBodyFault struct {
	source   io.ReadCloser
	err      error
	injected bool
	pending  bool
}

func (b *sdkResponseBodyFault) Read(p []byte) (int, error) {
	if b.pending {
		b.pending = false
		return 0, b.err
	}
	n, err := b.source.Read(p)
	if err == io.EOF && !b.injected {
		b.injected = true
		if n > 0 {
			b.pending = true
			return n, nil
		}
		return 0, b.err
	}
	return n, err
}

func (b *sdkResponseBodyFault) Close() error { return b.source.Close() }

type sdkRequestMutationRoundTripper struct {
	inner  http.RoundTripper
	mutate func(*http.Request)
}

func (t sdkRequestMutationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.mutate != nil {
		t.mutate(req)
	}
	return t.inner.RoundTrip(req)
}

func pinnedDrainRun(a Approval) map[string]any {
	repository := map[string]any{"id": a.RepositoryID, "private": true, "fork": false}
	return map[string]any{
		"id": a.WorkflowRunID, "head_sha": a.WorkflowSHA, "event": "workflow_dispatch",
		"path": a.WorkflowPath, "run_attempt": 1, "repository": repository,
		"head_repository": repository,
	}
}

func newPinnedDrainClient(session Session, hook *drainPollHook) *drainClient {
	initial := session.Session()
	stats, _ := newDrainStatistics(initial.Statistics)
	return &drainClient{
		inner: session, hook: hook, phaseCtx: context.Background(),
		obs: &drainObservation{Poll: drainPollObservation{Message: drainMessageUnknown}, NextPoll: drainPollObservation{Message: drainMessageUnknown}}, ownedRunnerStats: stats, ownedRunnerStatsKnown: true,
	}
}

func pinnedDrainStatistics(available, assigned int) map[string]int {
	return map[string]int{
		"totalAvailableJobs": available, "totalAcquiredJobs": 0, "totalAssignedJobs": assigned,
		"totalRunningJobs": 0, "totalRegisteredRunners": 1, "totalBusyRunners": 0, "totalIdleRunners": 1,
	}
}

func pinnedDrainScaleSet(a Approval) *scaleset.RunnerScaleSet {
	return &scaleset.RunnerScaleSet{
		ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID,
		Labels:        []scaleset.Label{{Name: a.setName(), Type: "System"}},
		RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		Statistics:    &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
	}
}

func pinnedDrainSnapshot(a Approval) map[string]any {
	return map[string]any{
		"id": 7, "name": a.setName(), "runnerGroupId": a.RunnerGroupID,
		"labels":        []map[string]string{{"name": a.setName(), "type": "System"}},
		"RunnerSetting": map[string]any{"disableUpdate": true},
		"statistics":    pinnedDrainStatistics(0, 0),
	}
}

func (f *pinnedDrainFixture) cursorsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cursors...)
}

func (f *pinnedDrainFixture) capacitiesSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.capacity...)
}
