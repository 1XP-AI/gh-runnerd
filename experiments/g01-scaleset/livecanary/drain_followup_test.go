package livecanary

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"
	"testing"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
)

func drainReplayPrefix() []Event {
	return []Event{
		{Kind: "phase", Operation: "create", Sequence: 1},
		{Kind: "intent", Operation: "create", Sequence: 2},
		{Kind: "result", Operation: "create", Sequence: 3, ID: 7},
		{Kind: "phase", Operation: "drain", Sequence: 4, ID: 7},
	}
}

func TestReplayDrainRequiresOneMatchingPhaseIdentityAndSequence(t *testing.T) {
	base := validDrainTestObservation()
	base.Sequence = 4
	foreign := base
	foreign.Before.Set.ID = 99
	foreign.After.Set.ID = 99
	foreign.Before.Runner = &drainRunnerIdentity{ID: 19, Name: "g01-test-worker-1", ScaleSetID: 99}
	foreign.After.Runner = &drainRunnerIdentity{ID: 19, Name: "g01-test-worker-1", ScaleSetID: 99}

	cases := []struct {
		name      string
		events    func(drainObservation) []Event
		uncertain bool
	}{
		{
			name: "one matching phase",
			events: func(observation drainObservation) []Event {
				events := drainReplayPrefix()
				events = append(events, Event{Kind: "observation", Operation: "drain", Sequence: 5, Drain: &observation})
				return events
			},
			uncertain: false,
		},
		{
			name: "missing phase",
			events: func(observation drainObservation) []Event {
				events := drainReplayPrefix()[:3]
				return append(events, Event{Kind: "observation", Operation: "drain", Sequence: 4, Drain: &observation})
			},
			uncertain: true,
		},
		{
			name: "repeated phase",
			events: func(observation drainObservation) []Event {
				events := drainReplayPrefix()
				events = append(events, Event{Kind: "phase", Operation: "drain", Sequence: 5, ID: 7})
				return append(events, Event{Kind: "observation", Operation: "drain", Sequence: 6, Drain: &observation})
			},
			uncertain: true,
		},
		{
			name: "interrupted phase",
			events: func(observation drainObservation) []Event {
				events := drainReplayPrefix()
				events = append(events, Event{Kind: "phase", Operation: "inspect", Sequence: 5})
				return append(events, Event{Kind: "observation", Operation: "drain", Sequence: 6, Drain: &observation})
			},
			uncertain: true,
		},
		{
			name: "mismatched sequence",
			events: func(observation drainObservation) []Event {
				observation.Sequence = 999
				events := drainReplayPrefix()
				return append(events, Event{Kind: "observation", Operation: "drain", Sequence: 5, Drain: &observation})
			},
			uncertain: true,
		},
		{
			name: "mismatched phase identity",
			events: func(observation drainObservation) []Event {
				events := drainReplayPrefix()
				events[3].ID = 99
				return append(events, Event{Kind: "observation", Operation: "drain", Sequence: 5, Drain: &observation})
			},
			uncertain: true,
		},
		{
			name: "mismatched created identity",
			events: func(observation drainObservation) []Event {
				events := drainReplayPrefix()
				return append(events, Event{Kind: "observation", Operation: "drain", Sequence: 5, Drain: &observation})
			},
			uncertain: true,
		},
		{
			name: "duplicate observation",
			events: func(observation drainObservation) []Event {
				events := drainReplayPrefix()
				events = append(events, Event{Kind: "observation", Operation: "drain", Sequence: 5, Drain: &observation})
				return append(events, Event{Kind: "observation", Operation: "drain", Sequence: 6, Drain: &observation})
			},
			uncertain: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observation := base
			if tc.name == "mismatched created identity" {
				observation = foreign
			}
			if got := replay(tc.events(observation)).uncertain; got != tc.uncertain {
				t.Fatalf("uncertain=%v, want %v; state=%+v", got, tc.uncertain, replay(tc.events(observation)))
			}
		})
	}
}

func TestFileJournalDrainReplayRetainsMismatchedSequenceAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*drainObservation)
	}{
		{name: "sequence", mutate: func(observation *drainObservation) { observation.Sequence = 999 }},
		{name: "created identity", mutate: func(observation *drainObservation) {
			observation.Before.Set.ID = 99
			observation.After.Set.ID = 99
			observation.Before.Runner = &drainRunnerIdentity{ID: 19, Name: "g01-test-worker-1", ScaleSetID: 99}
			observation.After.Runner = &drainRunnerIdentity{ID: 19, Name: "g01-test-worker-1", ScaleSetID: 99}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := approval()
			a.Phases = append(a.Phases, "drain")
			directory := t.TempDir()
			if err := os.Chmod(directory, 0700); err != nil {
				t.Fatal(err)
			}
			j, err := openTestJournal(t, directory, a)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range drainReplayPrefix() {
				if err := j.Append(event); err != nil {
					j.Close()
					t.Fatalf("append prefix: %v", err)
				}
			}
			observation := validDrainTestObservation()
			if tc.name == "sequence" {
				observation.Sequence = 999
			} else {
				phase := j.Events()[len(j.Events())-1]
				observation.Sequence = phase.Sequence
			}
			tc.mutate(&observation)
			if err := j.Append(Event{Kind: "observation", Operation: "drain", Drain: &observation}); err != nil {
				j.Close()
				t.Fatalf("append drain observation: %v", err)
			}
			if err := j.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := openTestJournal(t, directory, a)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if state := replay(reopened.Events()); !state.uncertain {
				t.Fatalf("reopened mismatched drain history discharged fence: %+v", state)
			}
		})
	}
}

func TestReplayDrainPhaseRequiresPositiveSetID(t *testing.T) {
	base := validDrainTestObservation()
	base.Sequence = 4
	for _, tc := range []struct {
		name string
		id   int
	}{
		{name: "missing phase ID", id: 0},
		{name: "zero phase ID", id: 0},
		{name: "negative phase ID", id: -1},
		{name: "foreign phase ID", id: 99},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := drainReplayPrefix()
			events[3].ID = tc.id
			events = append(events, Event{Kind: "observation", Operation: "drain", Sequence: 5, Drain: &base})
			if state := replay(events); !state.uncertain {
				t.Fatalf("drain phase with %d SetID discharged its fence: %+v", tc.id, state)
			}
		})
	}
}

func TestFileJournalRejectsNonPositiveDrainPhaseSetID(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   int
	}{
		{name: "missing phase ID", id: 0},
		{name: "zero phase ID", id: 0},
		{name: "negative phase ID", id: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := approval()
			a.Phases = append(a.Phases, "drain")
			directory := privateDir(t)
			j, err := openTestJournal(t, directory, a)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range drainReplayPrefix()[:3] {
				if err := j.Append(event); err != nil {
					j.Close()
					t.Fatalf("append create prefix: %v", err)
				}
			}
			phase := drainReplayPrefix()[3]
			phase.ID = tc.id
			if err := j.Append(phase); err == nil {
				j.Close()
				t.Fatalf("accepted drain phase with non-positive SetID %d", tc.id)
			}
			if err := j.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := openTestJournal(t, directory, a)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if events := reopened.Events(); len(events) != 3 || replay(events).setID != 7 {
				t.Fatalf("reopened journal after rejected drain phase = %+v", events)
			}
		})
	}
}

func TestFileJournalForeignDrainPhaseSetIDRetainsFenceAfterReopen(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	directory := privateDir(t)
	j, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range drainReplayPrefix()[:3] {
		if err := j.Append(event); err != nil {
			j.Close()
			t.Fatalf("append create prefix: %v", err)
		}
	}
	phase := drainReplayPrefix()[3]
	phase.ID = 99
	if err := j.Append(phase); err != nil {
		j.Close()
		t.Fatalf("append foreign drain phase: %v", err)
	}
	observation := validDrainTestObservation()
	observation.Sequence = phase.Sequence
	if err := j.Append(Event{Kind: "observation", Operation: "drain", Drain: &observation}); err != nil {
		j.Close()
		t.Fatalf("append drain observation: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if state := replay(reopened.Events()); !state.uncertain {
		t.Fatalf("reopened foreign drain phase discharged its fence: %+v", state)
	}
}

func TestDrainObservedAllowsNextPollJobCounterChanges(t *testing.T) {
	observation := validDrainTestObservation()
	observation.NextPoll.Statistics.Available = 4
	observation.NextPoll.Statistics.Acquired = 3
	observation.NextPoll.Statistics.Assigned = 2
	observation.NextPoll.Statistics.Running = 1
	if !validDrainObservation(&observation) {
		t.Fatal("job counters in the withdrawn poll were incorrectly frozen")
	}
}

type contradictoryNextPollTransport struct{ polls int }

func (t *contradictoryNextPollTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		status := http.StatusNoContent
		body := io.ReadCloser(http.NoBody)
		if req.Method == http.MethodPost {
			status = http.StatusOK
			body = io.NopCloser(strings.NewReader(`{"count":1,"value":[11]}`))
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Request: req, Body: body}, nil
	}
	t.polls++
	trace := httptrace.ContextClientTrace(req.Context())
	if trace == nil || trace.WroteRequest == nil {
		return nil, errors.New("missing client trace")
	}
	trace.WroteRequest(httptrace.WroteRequestInfo{})
	body := `{"statistics":{"totalAvailableJobs":1,"totalAcquiredJobs":0,"totalAssignedJobs":1,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1}}`
	status := http.StatusOK
	if t.polls == 1 {
		body = `{"messageId":7,"messageType":"RunnerScaleSetJobMessages","statistics":{"totalAvailableJobs":1,"totalAcquiredJobs":0,"totalAssignedJobs":1,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1},"body":"[]"}`
	} else {
		status = http.StatusAccepted
		body = `{"statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":0,"totalBusyRunners":0,"totalIdleRunners":0}}`
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Request: req, Body: io.NopCloser(bytes.NewBufferString(body))}, nil
}

func TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition(t *testing.T) {
	const target = "http://fixture.invalid/queue"
	hook := newDrainPollHook(target)
	hook.inner = &contradictoryNextPollTransport{}
	session := &drainSyntheticSession{
		client: &http.Client{Transport: hook},
		initial: scaleset.RunnerScaleSetSession{
			SessionID: uuid.New(), OwnerName: "fixture-owner", MessageQueueURL: target,
			Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
		},
		order: new([]string),
	}
	observation, err := runDrainListener(context.Background(), session, 7, hook)
	if err == nil || observation.Outcome == drainOutcomeObserved {
		t.Fatalf("contradictory withdrawn-poll runner partition was observed: observation=%+v err=%v", observation, err)
	}
}
