package livecanary

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
	"github.com/actions/scaleset"
	"github.com/google/uuid"
)

func TestDrainListenerWithdrawsWhilePollResponseIsHeld(t *testing.T) {
	var mu sync.Mutex
	var capacities []string
	var order []string
	polls := 0
	message := map[string]any{
		"messageId":   7,
		"messageType": "RunnerScaleSetJobMessages",
		"statistics": map[string]int{
			"totalAvailableJobs":     1,
			"totalAcquiredJobs":      0,
			"totalAssignedJobs":      1,
			"totalRunningJobs":       0,
			"totalRegisteredRunners": 1,
			"totalBusyRunners":       0,
			"totalIdleRunners":       1,
		},
		"body": "[]",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.Method == http.MethodGet {
			capacities = append(capacities, r.Header.Get(scaleset.HeaderScaleSetMaxCapacity))
			polls++
			if polls == 1 {
				data, _ := json.Marshal(message)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				mu.Unlock()
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1}}`)
			mu.Unlock()
			return
		}
		if r.Method == http.MethodDelete {
			order = append(order, "ack")
			w.WriteHeader(http.StatusNoContent)
			mu.Unlock()
			return
		}
		if r.Method == http.MethodPost {
			order = append(order, "acquire")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"count":1,"value":[11]}`)
			mu.Unlock()
			return
		}
		mu.Unlock()
		http.NotFound(w, r)
	}))
	defer server.Close()

	hook := newDrainPollHook(server.URL)
	client := &http.Client{Transport: hook}
	session := &drainSyntheticSession{
		client: client,
		initial: scaleset.RunnerScaleSetSession{
			SessionID: uuid.New(), OwnerName: "fixture-owner", MessageQueueURL: server.URL,
			Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
		},
		order: &order,
	}

	observation, err := runDrainListener(context.Background(), session, 7, hook)
	if err != nil {
		t.Fatalf("drain listener: %v", err)
	}
	mu.Lock()
	gotCapacities := append([]string(nil), capacities...)
	gotOrder := append([]string(nil), order...)
	mu.Unlock()
	if len(gotCapacities) != 2 || gotCapacities[0] != "1" || gotCapacities[1] != "0" {
		t.Fatalf("poll capacities = %v, want [1 0]", gotCapacities)
	}
	if len(gotOrder) != 2 || gotOrder[0] != "ack" || gotOrder[1] != "acquire" {
		t.Fatalf("side-effect order = %v, want ACK then acquire", gotOrder)
	}
	if observation.Boundary != drainBoundaryRequestWritten || !observation.ResponseHeld || observation.ServerReceipt != drainServerReceiptUnproven {
		t.Fatalf("boundary = %+v, want request-written/held/server-unproven", observation)
	}
	if observation.Poll.ACK != drainResponseSucceeded || observation.Poll.Acquisition != drainResponseSucceeded || observation.NextPoll.Capacity != 0 {
		t.Fatalf("observation = %+v", observation)
	}
}

func TestDrainListenerRejectsMissingRequestWrittenBoundary(t *testing.T) {
	hook := newDrainPollHook("http://fixture.invalid/queue")
	session := &drainSyntheticSession{initial: scaleset.RunnerScaleSetSession{SessionID: uuid.New(), OwnerName: "fixture-owner", MessageQueueURL: "http://fixture.invalid/queue", Statistics: &scaleset.RunnerScaleSetStatistic{}}}
	if _, err := runDrainListener(context.Background(), session, 7, hook); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("missing boundary error = %v, want unresolved", err)
	}
}

func TestDrainListenerMarksResponseBeforeWriteInconclusive(t *testing.T) {
	hook := newDrainPollHook("http://fixture.invalid/queue")
	hook.inner = &drainNoTraceTransport{}
	session := &drainSyntheticSession{
		client: &http.Client{Transport: hook},
		initial: scaleset.RunnerScaleSetSession{
			SessionID: uuid.New(), OwnerName: "fixture-owner", MessageQueueURL: "http://fixture.invalid/queue",
			Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
		},
		order: new([]string),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	observation, err := runDrainListener(ctx, session, 7, hook)
	if !errors.Is(err, ErrNoMessage) || observation.Boundary != drainBoundaryResponseBeforeWrite || observation.Outcome != drainOutcomeInconclusive {
		t.Fatalf("response-before-write = %+v, err=%v; want inconclusive", observation, err)
	}
}

func TestDrainListenerNoMessageIsInconclusive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	hook := newDrainPollHook(server.URL)
	session := &drainSyntheticSession{
		client: http.DefaultClient,
		initial: scaleset.RunnerScaleSetSession{
			SessionID: uuid.New(), OwnerName: "fixture-owner", MessageQueueURL: server.URL,
			Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
		},
	}
	client := *http.DefaultClient
	client.Transport = hook
	session.client = &client
	observation, err := runDrainListener(context.Background(), session, 7, hook)
	if !errors.Is(err, ErrNoMessage) || observation.Outcome != drainOutcomeInconclusive || observation.Poll.Message != drainMessageAbsent || observation.Poll.StatsKnown || observation.NextPoll.StatsKnown {
		t.Fatalf("no-message drain = %+v err=%v, want inconclusive", observation, err)
	}
}

func TestDrainHeldBodyHoldsCloseUntilRelease(t *testing.T) {
	hook := newDrainPollHook("http://fixture.invalid/queue")
	body := &drainHeldBody{source: http.NoBody, release: hook.release}
	done := make(chan error, 1)
	go func() { done <- body.Close() }()
	select {
	case <-done:
		t.Fatal("status-only response close was not held")
	case <-time.After(20 * time.Millisecond):
	}
	hook.releaseResponse()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("held body close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("held body close did not release")
	}
}

func TestDrainObservationIsBoundedAndFailClosed(t *testing.T) {
	observation := validDrainTestObservation()
	if !validDrainObservation(&observation) {
		t.Fatal("valid drain observation rejected")
	}

	for name, mutate := range map[string]func(*drainObservation){
		"negative-counter":         func(o *drainObservation) { o.Poll.Statistics.Available = -1 },
		"negative-unknown-counter": func(o *drainObservation) { o.NextPoll.Statistics.Idle = -1; o.NextPoll.StatsKnown = false },
		"identity-change":          func(o *drainObservation) { o.After.Set.ID++ },
		"runner-change": func(o *drainObservation) {
			runner := *o.After.Runner
			runner.ID++
			o.After.Runner = &runner
		},
		"duplicate-order": func(o *drainObservation) { o.Ordering = append(o.Ordering, "ack") },
		"response-before-write-observed": func(o *drainObservation) {
			o.Boundary = drainBoundaryResponseBeforeWrite
		},
		"server-receipt-claim": func(o *drainObservation) { o.ServerReceipt = "accepted" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := observation
			candidate.Ordering = append([]string(nil), observation.Ordering...)
			mutate(&candidate)
			if validDrainObservation(&candidate) {
				t.Fatal("invalid drain observation accepted")
			}
		})
	}

	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"token", "authorization", "queue-url", "raw-error", "jit"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("bounded observation contains %q: %s", forbidden, raw)
		}
	}

	state := replay([]Event{{Kind: "observation", Operation: "drain", Drain: &observation}})
	if !state.workObserved || state.uncertain {
		t.Fatalf("observed drain replay = %+v, want work retained without uncertainty", state)
	}
	observation.Outcome = drainOutcomeInconclusive
	state = replay([]Event{{Kind: "observation", Operation: "drain", Drain: &observation}})
	if !state.workObserved || !state.uncertain {
		t.Fatalf("inconclusive drain replay = %+v, want retained and quarantined", state)
	}
}

func TestDrainIdlePrerequisiteRejectsAmbiguousState(t *testing.T) {
	observation := validDrainTestObservation()
	before := observation.Before
	if !validDrainIdlePrerequisite(before) {
		t.Fatal("valid idle prerequisite rejected")
	}
	for name, mutate := range map[string]func(*drainSnapshot){
		"missing-runner": func(s *drainSnapshot) { s.Runner = nil },
		"busy":           func(s *drainSnapshot) { s.Statistics.Busy = 1 },
		"assigned":       func(s *drainSnapshot) { s.Statistics.Assigned = 1 },
		"two-runners":    func(s *drainSnapshot) { s.Statistics.Registered = 2 },
		"zero-idle":      func(s *drainSnapshot) { s.Statistics.Idle = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := before
			if before.Runner != nil {
				runner := *before.Runner
				candidate.Runner = &runner
			}
			mutate(&candidate)
			if validDrainIdlePrerequisite(candidate) {
				t.Fatal("ambiguous idle prerequisite accepted")
			}
		})
	}
}

func TestDrainRequiresVerificationAuthority(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	c := credentials(a)
	c.VerificationToken = ""
	if _, err := NewSDKAPI(a, c); !errors.Is(err, ErrApproval) {
		t.Fatalf("drain without verification authority = %v, want approval rejection", err)
	}
}

func TestDrainPhaseAuthorityIsAccepted(t *testing.T) {
	a := approval()
	a.Phases = []string{"create", "drain", "inspect", "cleanup"}
	if err := a.Validate(time.Now()); err != nil {
		t.Fatalf("issue-71 drain authority rejected: %v", err)
	}
}

func TestDrainPhaseStartRetainsCrashUncertainty(t *testing.T) {
	state := replay([]Event{{Kind: "phase", Operation: "drain"}})
	if !state.uncertain {
		t.Fatal("drain phase without final observation did not retain uncertainty")
	}
}

func TestDrainPollJournalsReservationBeforeACK(t *testing.T) {
	a := approval()
	j := &memoryJournal{}
	d := Driver{Approval: a, Journal: j, API: &fakeAPI{}}
	inner := &drainGuardSession{
		initial: scaleset.RunnerScaleSetSession{SessionID: uuid.New(), OwnerName: a.setName(), Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1}},
		message: &scaleset.RunnerScaleSetMessage{
			MessageID:            17,
			Statistics:           &scaleset.RunnerScaleSetStatistic{TotalAvailableJobs: 1, TotalAssignedJobs: 1, TotalRegisteredRunners: 1, TotalIdleRunners: 1},
			JobAvailableMessages: []*scaleset.JobAvailable{{JobMessageBase: scaleset.JobMessageBase{RunnerRequestID: 41, WorkflowRunID: a.WorkflowRunID, OwnerName: a.Organization, RepositoryName: a.Repository}}},
		},
	}
	client := &journaledDrainClient{d: &d, inner: inner, sessionID: inner.initial.SessionID.String()}
	if _, err := client.GetMessage(context.Background(), 0, drainInitialCapacity); err != nil {
		t.Fatalf("journaled poll: %v", err)
	}
	var poll Event
	for _, event := range j.Events() {
		if event.Kind == "result" && event.Operation == "observe-poll" {
			poll = event
		}
	}
	if len(poll.RequestIDs) != 1 || poll.RequestIDs[0] != 41 || poll.Work != workDemand {
		t.Fatalf("poll reservation = %+v, want request 41/demand", poll)
	}
	state := replay(j.Events())
	if !state.reserved || !state.workObserved || !state.observedJobs[41] {
		t.Fatalf("poll reservation replay = %+v, want durable quarantine fence", state)
	}
}

func TestDrainRejectedIdlePrerequisiteRetainsFence(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	j := &memoryJournal{events: []Event{
		{Kind: "phase", Operation: "create"},
		{Kind: "intent", Operation: "create"},
		{Kind: "result", Operation: "create", ID: 7},
	}}
	api := &drainDriverAPI{fakeAPI: &fakeAPI{}, approval: a, snapshotStats: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalBusyRunners: 1}}
	d := Driver{Approval: a, Journal: j, API: api}
	if err := d.Run(context.Background(), "drain"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("busy idle prerequisite = %v, want quarantine", err)
	}
	state := replay(j.Events())
	if !state.uncertain {
		t.Fatalf("rejected prerequisite replay = %+v, want durable uncertainty", state)
	}
	for _, event := range j.Events() {
		if event.Kind == "observation" && event.Operation == "drain-marker" && event.DrainMarker == drainMarkerPrerequisiteFailed {
			return
		}
	}
	t.Fatal("rejected prerequisite did not persist a bounded marker")
}

func TestDrainRejectsEmbeddedSessionStatisticsMismatch(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	j := &memoryJournal{events: []Event{
		{Kind: "phase", Operation: "create"},
		{Kind: "intent", Operation: "create"},
		{Kind: "result", Operation: "create", ID: 7},
	}}
	api := &drainDriverAPI{fakeAPI: &fakeAPI{}, approval: a, sessionStats: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1, TotalAssignedJobs: 1}}
	d := Driver{Approval: a, Journal: j, API: api}
	if err := d.Run(context.Background(), "drain"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("embedded session mismatch = %v, want quarantine", err)
	}
	if replay(j.Events()).uncertain == false {
		t.Fatal("embedded session mismatch did not retain uncertainty")
	}
}

func TestDrainRejectsEffectsAfterCancellationAndRecordsMarker(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	j := &memoryJournal{events: []Event{
		{Kind: "phase", Operation: "create"},
		{Kind: "intent", Operation: "create"},
		{Kind: "result", Operation: "create", ID: 7},
	}}
	api := &drainDriverAPI{fakeAPI: &fakeAPI{}, approval: a, blocking: true, opened: make(chan struct{})}
	d := Driver{Approval: a, Journal: j, API: api}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, "drain") }()
	select {
	case <-api.opened:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("drain session did not open")
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrQuarantine) {
			t.Fatalf("canceled drain = %v, want quarantine", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled drain did not join listener")
	}
	for _, event := range j.Events() {
		if event.Kind == "observation" && event.Operation == "drain-marker" && event.DrainMarker == drainMarkerCancelled {
			return
		}
	}
	t.Fatal("canceled drain did not persist cancellation marker")
}

func TestDrainRejectsDuplicateOrWrongEffectsBeforeInnerCall(t *testing.T) {
	inner := &drainGuardSession{
		initial: scaleset.RunnerScaleSetSession{SessionID: uuid.New(), OwnerName: "fixture", Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1}},
		message: &scaleset.RunnerScaleSetMessage{
			MessageID:            7,
			Statistics:           &scaleset.RunnerScaleSetStatistic{TotalAvailableJobs: 1, TotalAssignedJobs: 1, TotalRegisteredRunners: 1, TotalIdleRunners: 1},
			JobAvailableMessages: []*scaleset.JobAvailable{{JobMessageBase: scaleset.JobMessageBase{RunnerRequestID: 11}}},
		},
	}
	obs := drainObservation{Poll: drainPollObservation{Message: drainMessageUnknown}, NextPoll: drainPollObservation{Message: drainMessageUnknown}}
	client := &drainClient{inner: inner, obs: &obs, phaseCtx: context.Background()}
	if _, err := client.GetMessage(context.Background(), 0, drainInitialCapacity); err != nil {
		t.Fatalf("guard poll: %v", err)
	}
	if err := client.DeleteMessage(context.Background(), 8); !errors.Is(err, ErrQuarantine) || inner.ackCalls != 0 {
		t.Fatalf("wrong ACK = %v calls=%d, want pre-effect quarantine", err, inner.ackCalls)
	}
	if err := client.DeleteMessage(context.Background(), 7); err != nil || inner.ackCalls != 1 {
		t.Fatalf("valid ACK = %v calls=%d", err, inner.ackCalls)
	}
	if err := client.DeleteMessage(context.Background(), 7); !errors.Is(err, ErrQuarantine) || inner.ackCalls != 1 {
		t.Fatalf("duplicate ACK = %v calls=%d, want no second call", err, inner.ackCalls)
	}
	if _, err := client.AcquireJobs(context.Background(), []int64{12}); !errors.Is(err, ErrQuarantine) || inner.acquireCalls != 0 {
		t.Fatalf("wrong acquire = %v calls=%d, want pre-effect quarantine", err, inner.acquireCalls)
	}
	if _, err := client.AcquireJobs(context.Background(), []int64{11}); err != nil || inner.acquireCalls != 1 {
		t.Fatalf("valid acquire = %v calls=%d", err, inner.acquireCalls)
	}
	if _, err := client.AcquireJobs(context.Background(), []int64{11}); !errors.Is(err, ErrQuarantine) || inner.acquireCalls != 1 {
		t.Fatalf("duplicate acquire = %v calls=%d, want no second call", err, inner.acquireCalls)
	}
}

func TestDrainRequiresKnownConsistentStatisticsAndControlledMessage(t *testing.T) {
	observation := validDrainTestObservation()
	unknown := observation
	unknown.Poll.StatsKnown = false
	unknown.Poll.Statistics = drainStatistics{}
	if validDrainObservation(&unknown) {
		t.Fatal("observed drain with unknown poll statistics accepted")
	}
	contradictory := observation
	contradictory.Poll.Statistics.Registered = 1
	contradictory.Poll.Statistics.Busy = 1
	contradictory.Poll.Statistics.Idle = 1
	if validDrainObservation(&contradictory) {
		t.Fatal("contradictory runner partition accepted")
	}
	if _, err := newDrainStatistics(&scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalBusyRunners: 1, TotalIdleRunners: 1}); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("contradictory source statistics = %v, want quarantine", err)
	}
	noMessage := observation
	noMessage.Poll.Message = drainMessageAbsent
	noMessage.Poll.Statistics = drainStatistics{}
	noMessage.Poll.StatsKnown = false
	noMessage.Poll.ACK = drainResponseNotAttempted
	noMessage.Poll.Acquisition = drainResponseNotAttempted
	noMessage.Ordering = []string{"poll-old", "poll-zero"}
	if validDrainObservation(&noMessage) {
		t.Fatal("observed drain with no controlled old message accepted")
	}
	unknownNext := observation
	unknownNext.NextPoll.StatsKnown = false
	unknownNext.NextPoll.Statistics = drainStatistics{}
	if validDrainObservation(&unknownNext) {
		t.Fatal("observed drain with unknown next-poll statistics accepted")
	}
	emptyStatistics := observation
	emptyStatistics.Poll.Statistics = drainStatistics{}
	if validDrainObservation(&emptyStatistics) {
		t.Fatal("observed drain with empty controlled-poll statistics accepted")
	}
}

type drainCancelOnIntentJournal struct {
	*memoryJournal
	cancel context.CancelFunc
}

func (j *drainCancelOnIntentJournal) Append(event Event) error {
	if event.Kind == "intent" && event.Operation == "ack" {
		j.cancel()
	}
	return j.memoryJournal.Append(event)
}

func TestDrainCancellationAfterIntentRejectsEffect(t *testing.T) {
	a := approval()
	ctx, cancel := context.WithCancel(context.Background())
	j := &drainCancelOnIntentJournal{memoryJournal: &memoryJournal{}, cancel: cancel}
	f := &fakeAPI{session: &fakeSession{session: scaleset.RunnerScaleSetSession{SessionID: uuid.New(), Statistics: &scaleset.RunnerScaleSetStatistic{}}}}
	d := &Driver{Approval: a, Journal: j, API: f}
	c := &journaledDrainClient{d: d, inner: f.session, sessionID: "session"}
	c.bindDrainContext(ctx)
	err := c.DeleteMessage(context.WithoutCancel(ctx), 7)
	if !errors.Is(err, ErrQuarantine) || f.session.ack != 0 {
		t.Fatalf("cancellation race effect = err %v ack %d; want quarantine and zero calls", err, f.session.ack)
	}
}

type drainCancelBeforeSnapshotAPI struct {
	*drainDriverAPI
	cancel context.CancelFunc
}

func (a *drainCancelBeforeSnapshotAPI) Preflight(context.Context, Approval) error {
	a.cancel()
	return nil
}

func (*drainCancelBeforeSnapshotAPI) GetScaleSet(context.Context, int) (*scaleset.RunnerScaleSet, error) {
	return nil, context.Canceled
}

func TestDrainCancellationBeforeSnapshotRecordsMarker(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	j := &memoryJournal{events: []Event{
		{Kind: "phase", Operation: "create"},
		{Kind: "intent", Operation: "create"},
		{Kind: "result", Operation: "create", ID: 7},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	api := &drainCancelBeforeSnapshotAPI{
		drainDriverAPI: &drainDriverAPI{fakeAPI: &fakeAPI{}, approval: a},
		cancel:         cancel,
	}
	d := &Driver{Approval: a, Journal: j, API: api}
	if err := d.Run(ctx, "drain"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("before-snapshot cancellation = %v, want quarantine", err)
	}
	for _, event := range j.Events() {
		if event.Kind == "observation" && event.Operation == "drain-marker" && event.DrainMarker == drainMarkerCancelled {
			return
		}
	}
	t.Fatal("before-snapshot cancellation did not persist a cancellation marker")
}

func TestDrainPollHookPreservesStatisticsFieldPresence(t *testing.T) {
	full := `{"messageId":7,"messageType":"RunnerScaleSetJobMessages","statistics":{"totalAvailableJobs":1,"totalAcquiredJobs":0,"totalAssignedJobs":1,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1},"body":"[]"}`
	empty := `{"messageId":7,"messageType":"RunnerScaleSetJobMessages","statistics":{},"body":"[]"}`
	missing := `{"messageId":7,"messageType":"RunnerScaleSetJobMessages","body":"[]"}`
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{name: "all fields", body: full, want: true},
		{name: "empty object", body: empty, want: false},
		{name: "missing object", body: missing, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			hook := newDrainPollHook("http://fixture.invalid/queue")
			hook.inner = drainRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})
			req, err := http.NewRequest(http.MethodGet, "http://fixture.invalid/queue", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := hook.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			hook.releaseResponse()
			if _, err := io.ReadAll(response.Body); err != nil {
				t.Fatal(err)
			}
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			_, got := hook.pollStatistics(1)
			if got != test.want {
				t.Fatalf("statistics field presence = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDriverRoutesDrainBeforeNoWorkerStatisticsQuarantine(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	j := &memoryJournal{events: []Event{
		{Kind: "phase", Operation: "create"},
		{Kind: "intent", Operation: "create"},
		{Kind: "result", Operation: "create", ID: 7},
	}}
	api := &drainDriverAPI{fakeAPI: &fakeAPI{}, approval: a}
	d := Driver{Approval: a, Journal: j, API: api}
	err := d.Run(context.Background(), "drain")
	if !errors.Is(err, ErrNoMessage) {
		t.Fatalf("drain result = %v, want bounded unresolved result", err)
	}
	if api.getScaleSetCalls != 2 {
		t.Fatalf("owned set reads = %d, want before/after drain reads", api.getScaleSetCalls)
	}
	if api.closeCalls != 0 {
		t.Fatalf("ambiguous drain closed session %d times", api.closeCalls)
	}
	var observed bool
	for _, event := range j.events {
		if event.Kind == "observation" && event.Operation == "drain" {
			observed = true
		}
	}
	if !observed {
		t.Fatal("drain did not retain its bounded observation")
	}
}

func TestValidatePairedApprovalsAcceptsDrainVerification(t *testing.T) {
	a := approval()
	a.Phases = []string{"create", "drain", "inspect", "cleanup"}
	worker := liveworker.Approval{
		RunnerUpdatesDisabled: true, HarnessSHA: a.HarnessSHA, WorkflowSHA: a.WorkflowSHA,
		OwnerNonce: a.OwnerNonce, Controller: a.Controller, Endpoint: "/fixture/docker.sock",
		DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("a", 64), Image: liveworker.ImageReference,
		ExpiresAt: a.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"},
	}
	if err := ValidatePairedApprovals(a, worker); err != nil {
		t.Fatalf("paired approval with drain verification rejected: %v", err)
	}
}

func validDrainTestObservation() drainObservation {
	set := drainSetIdentity{ID: 7, Name: "g01-test-set", RunnerGroupID: 3, Label: "g01-test-set"}
	runner := &drainRunnerIdentity{ID: 19, Name: "g01-test-worker-1", ScaleSetID: 7}
	beforeStats := drainStatistics{Registered: 1, Idle: 1}
	return drainObservation{
		Version: drainObservationVersion, Outcome: drainOutcomeObserved,
		InitialCapacity: drainInitialCapacity, WithdrawnCapacity: drainWithdrawnCapacity,
		Boundary: drainBoundaryRequestWritten, ServerReceipt: drainServerReceiptUnproven,
		ResponseHeld: true,
		Poll:         drainPollObservation{Capacity: drainInitialCapacity, Message: drainMessagePresent, Statistics: drainStatistics{Available: 1, Assigned: 1, Registered: 1, Idle: 1}, StatsKnown: true, ACK: drainResponseSucceeded, Acquisition: drainResponseSucceeded},
		NextPoll:     drainPollObservation{Capacity: drainWithdrawnCapacity, Message: drainMessageAbsent, Statistics: beforeStats, StatsKnown: true, ACK: drainResponseNotAttempted, Acquisition: drainResponseNotAttempted},
		Before:       drainSnapshot{Set: set, Statistics: beforeStats, StatsKnown: true, Runner: runner},
		After:        drainSnapshot{Set: set, Statistics: beforeStats, StatsKnown: true, Runner: runner},
		Ordering:     []string{"poll-old", "ack", "acquire", "poll-zero"}, Sequence: 1, ObservedAt: time.Unix(1, 0).UTC(),
	}
}

type drainDriverAPI struct {
	*fakeAPI
	approval         Approval
	getScaleSetCalls int
	snapshotStats    *scaleset.RunnerScaleSetStatistic
	blocking         bool
	opened           chan struct{}
	closeCalls       int
	sessionStats     *scaleset.RunnerScaleSetStatistic
}

type drainRoundTripper func(*http.Request) (*http.Response, error)

func (f drainRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func (a *drainDriverAPI) GetScaleSet(context.Context, int) (*scaleset.RunnerScaleSet, error) {
	a.getScaleSetCalls++
	stats := a.snapshotStats
	if stats == nil {
		stats = &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1}
	}
	return &scaleset.RunnerScaleSet{
		ID: 7, Name: a.approval.setName(), RunnerGroupID: a.approval.RunnerGroupID,
		Labels:        []scaleset.Label{{Name: a.approval.setName()}},
		RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		Statistics:    stats,
	}, nil
}

func (a *drainDriverAPI) FindRunner(context.Context, string) (*scaleset.RunnerReference, error) {
	return &scaleset.RunnerReference{ID: 19, Name: a.approval.workerName(), RunnerScaleSetID: 7}, nil
}

func (*drainDriverAPI) VerifyRun(context.Context, Approval, int64) error { return nil }

func (a *drainDriverAPI) OpenDrainSession(_ context.Context, _ int, _ string, hook *drainPollHook) (Session, error) {
	if a.blocking {
		close(a.opened)
		hook.mu.Lock()
		hook.target = "http://fixture.invalid/queue"
		hook.mu.Unlock()
		return &drainBlockingSession{initial: scaleset.RunnerScaleSetSession{SessionID: uuid.New(), OwnerName: a.approval.setName(), MessageQueueURL: hook.target, RunnerScaleSet: &scaleset.RunnerScaleSet{ID: 7, Name: a.approval.setName(), RunnerGroupID: a.approval.RunnerGroupID, Labels: []scaleset.Label{{Name: a.approval.setName()}}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true}, Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1}}, Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1}}}, nil
	}
	hook.mu.Lock()
	hook.target = "http://fixture.invalid/queue"
	hook.mu.Unlock()
	stats := a.sessionStats
	if stats == nil {
		stats = &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1}
	}
	return &drainSyntheticSession{initial: scaleset.RunnerScaleSetSession{SessionID: uuid.New(), OwnerName: a.approval.setName(), MessageQueueURL: "http://fixture.invalid/queue", RunnerScaleSet: &scaleset.RunnerScaleSet{ID: 7, Name: a.approval.setName(), RunnerGroupID: a.approval.RunnerGroupID, Labels: []scaleset.Label{{Name: a.approval.setName()}}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true}, Statistics: stats}, Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1}}, closeCalls: &a.closeCalls}, nil
}

type drainGuardSession struct {
	initial      scaleset.RunnerScaleSetSession
	message      *scaleset.RunnerScaleSetMessage
	ackCalls     int
	acquireCalls int
}

func (s *drainGuardSession) Session() scaleset.RunnerScaleSetSession { return s.initial }
func (s *drainGuardSession) Close(context.Context) error             { return nil }
func (s *drainGuardSession) GetMessage(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error) {
	return s.message, nil
}
func (s *drainGuardSession) DeleteMessage(context.Context, int) error {
	s.ackCalls++
	return nil
}
func (s *drainGuardSession) AcquireJobs(_ context.Context, ids []int64) ([]int64, error) {
	s.acquireCalls++
	return append([]int64(nil), ids...), nil
}

type drainBlockingSession struct {
	initial scaleset.RunnerScaleSetSession
}

func (s *drainBlockingSession) Session() scaleset.RunnerScaleSetSession { return s.initial }
func (s *drainBlockingSession) Close(ctx context.Context) error         { return ctx.Err() }
func (s *drainBlockingSession) GetMessage(ctx context.Context, _, _ int) (*scaleset.RunnerScaleSetMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (s *drainBlockingSession) DeleteMessage(context.Context, int) error {
	return errors.New("unexpected ACK")
}
func (s *drainBlockingSession) AcquireJobs(context.Context, []int64) ([]int64, error) {
	return nil, errors.New("unexpected acquisition")
}

type drainNoTraceTransport struct {
	gets int
}

func (t *drainNoTraceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response := &http.Response{Header: make(http.Header), Request: req}
	switch req.Method {
	case http.MethodGet:
		t.gets++
		if t.gets == 1 {
			response.StatusCode = http.StatusOK
			response.Status = http.StatusText(http.StatusOK)
			response.Body = io.NopCloser(strings.NewReader(`{"messageId":7,"messageType":"RunnerScaleSetJobMessages","statistics":{"totalRegisteredRunners":1,"totalIdleRunners":1},"body":"[]"}`))
		} else {
			response.StatusCode = http.StatusAccepted
			response.Status = http.StatusText(http.StatusAccepted)
			response.Body = http.NoBody
		}
	case http.MethodDelete:
		response.StatusCode = http.StatusNoContent
		response.Status = http.StatusText(http.StatusNoContent)
		response.Body = http.NoBody
	case http.MethodPost:
		response.StatusCode = http.StatusOK
		response.Status = http.StatusText(http.StatusOK)
		response.Body = io.NopCloser(strings.NewReader(`{"count":1,"value":[11]}`))
	default:
		response.StatusCode = http.StatusNotFound
		response.Status = http.StatusText(http.StatusNotFound)
		response.Body = http.NoBody
	}
	return response, nil
}

type drainSyntheticSession struct {
	client     *http.Client
	initial    scaleset.RunnerScaleSetSession
	order      *[]string
	closeCalls *int
}

func (s *drainSyntheticSession) Session() scaleset.RunnerScaleSetSession { return s.initial }
func (s *drainSyntheticSession) Close(context.Context) error {
	if s.closeCalls != nil {
		(*s.closeCalls)++
	}
	return nil
}
func (s *drainSyntheticSession) GetMessage(ctx context.Context, _, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	if s.client == nil {
		return nil, errors.New("synthetic transport unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.initial.MessageQueueURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(scaleset.HeaderScaleSetMaxCapacity, strconv.Itoa(capacity))
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("synthetic poll failed")
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		return nil, err
	}
	return &scaleset.RunnerScaleSetMessage{
		MessageID:            7,
		Statistics:           &scaleset.RunnerScaleSetStatistic{TotalAvailableJobs: 1, TotalAssignedJobs: 1, TotalRegisteredRunners: 1, TotalIdleRunners: 1},
		JobAvailableMessages: []*scaleset.JobAvailable{{JobMessageBase: scaleset.JobMessageBase{RunnerRequestID: 11}}},
	}, nil
}
func (s *drainSyntheticSession) DeleteMessage(ctx context.Context, id int) error {
	if s.order == nil {
		return errors.New("missing order sink")
	}
	if err := appendSyntheticMethod(ctx, s.client, http.MethodDelete, s.initial.MessageQueueURL+"/"+strconv.Itoa(id), s.order); err != nil {
		return err
	}
	return nil
}
func (s *drainSyntheticSession) AcquireJobs(ctx context.Context, ids []int64) ([]int64, error) {
	if len(ids) != 1 {
		return nil, errors.New("unexpected request count")
	}
	if err := appendSyntheticMethod(ctx, s.client, http.MethodPost, s.initial.MessageQueueURL+"/acquirejobs", s.order); err != nil {
		return nil, err
	}
	return append([]int64(nil), ids...), nil
}

func appendSyntheticMethod(ctx context.Context, client *http.Client, method, target string, order *[]string) error {
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errors.New("synthetic side effect failed")
	}
	return nil
}
