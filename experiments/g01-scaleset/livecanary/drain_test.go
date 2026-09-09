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
			w.WriteHeader(http.StatusAccepted)
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
		"duplicate-order":          func(o *drainObservation) { o.Ordering = append(o.Ordering, "ack") },
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
		Before:       drainSnapshot{Set: set, Statistics: beforeStats, Runner: runner},
		After:        drainSnapshot{Set: set, Statistics: beforeStats, Runner: runner},
		Ordering:     []string{"poll-old", "ack", "acquire", "poll-zero"}, Sequence: 1, ObservedAt: time.Unix(1, 0).UTC(),
	}
}

type drainDriverAPI struct {
	*fakeAPI
	approval         Approval
	getScaleSetCalls int
}

func (a *drainDriverAPI) GetScaleSet(context.Context, int) (*scaleset.RunnerScaleSet, error) {
	a.getScaleSetCalls++
	return &scaleset.RunnerScaleSet{
		ID: 7, Name: a.approval.setName(), RunnerGroupID: a.approval.RunnerGroupID,
		Labels:        []scaleset.Label{{Name: a.approval.setName()}},
		RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		Statistics:    &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
	}, nil
}

func (a *drainDriverAPI) FindRunner(context.Context, string) (*scaleset.RunnerReference, error) {
	return &scaleset.RunnerReference{ID: 19, Name: a.approval.workerName(), RunnerScaleSetID: 7}, nil
}

func (*drainDriverAPI) VerifyRun(context.Context, Approval, int64) error { return nil }

func (a *drainDriverAPI) OpenDrainSession(_ context.Context, _ int, _ string, hook *drainPollHook) (Session, error) {
	hook.mu.Lock()
	hook.target = "http://fixture.invalid/queue"
	hook.mu.Unlock()
	return &drainSyntheticSession{initial: scaleset.RunnerScaleSetSession{SessionID: uuid.New(), OwnerName: a.approval.setName(), MessageQueueURL: "http://fixture.invalid/queue", Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1}}}, nil
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
	client  *http.Client
	initial scaleset.RunnerScaleSetSession
	order   *[]string
}

func (s *drainSyntheticSession) Session() scaleset.RunnerScaleSetSession { return s.initial }
func (s *drainSyntheticSession) Close(context.Context) error             { return nil }
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
