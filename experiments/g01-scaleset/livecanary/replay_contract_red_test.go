package livecanary

import "testing"

func appendReplayContractSnapshot(t *testing.T, j *FileJournal, snapshot drainSnapshot, stage string) {
	t.Helper()
	for _, event := range []Event{
		{Kind: "intent", Operation: "observe-owned"},
		{Kind: "result", Operation: "observe-owned", ID: snapshot.Set.ID},
		{Kind: "intent", Operation: "observe-runner"},
		{Kind: "result", Operation: "observe-runner", ID: snapshot.Runner.ID, DrainSnapshot: &snapshot, DrainSnapshotStage: stage},
	} {
		if err := j.Append(event); err != nil {
			t.Fatalf("append snapshot event: %v", err)
		}
	}
}

func openReplayContractJournal(t *testing.T) (*FileJournal, Approval, Event) {
	t.Helper()
	a := approval()
	a.Phases = append(a.Phases, "drain")
	j, err := openTestJournal(t, privateDir(t), a)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range drainReplayPrefix() {
		if err := j.Append(event); err != nil {
			_ = j.Close()
			t.Fatalf("append drain prefix: %v", err)
		}
	}
	events := j.Events()
	return j, a, events[len(events)-1]
}

func reopenReplayContractJournal(t *testing.T, j *FileJournal, a Approval) *FileJournal {
	t.Helper()
	directory := j.directory
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	return reopened
}

func appendReplayContractObservation(t *testing.T, j *FileJournal, observation drainObservation, phase Event) {
	t.Helper()
	observation.Sequence = phase.Sequence
	if err := j.Append(Event{Kind: "observation", Operation: "drain", Drain: &observation}); err != nil {
		t.Fatalf("append drain observation: %v", err)
	}
}

func TestReplayContractRejectsMissingDrainSnapshotStagesAfterFileJournalReopen(t *testing.T) {
	for _, tc := range []struct {
		name         string
		appendBefore bool
		appendAfter  bool
	}{
		{name: "missing both"},
		{name: "missing before", appendAfter: true},
		{name: "missing after", appendBefore: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j, a, phase := openReplayContractJournal(t)
			observation := drainObservationForApproval(a)
			if tc.appendBefore {
				appendReplayContractSnapshot(t, j, observation.Before, "before")
			}
			if tc.appendAfter {
				appendReplayContractSnapshot(t, j, observation.After, "after")
			}
			appendReplayContractObservation(t, j, observation, phase)
			reopened := reopenReplayContractJournal(t, j, a)
			defer reopened.Close()
			if state := replayWithApproval(reopened.Events(), &a); !state.uncertain {
				t.Fatalf("accepted %s drain history without exactly two snapshots: %+v", tc.name, state)
			}
		})
	}
}

func TestReplayContractRejectsExtraDrainSnapshotAfterCompletedPhase(t *testing.T) {
	j, a, phase := openReplayContractJournal(t)
	observation := drainObservationForApproval(a)
	appendReplayContractSnapshot(t, j, observation.Before, "before")
	appendReplayContractSnapshot(t, j, observation.After, "after")
	appendReplayContractObservation(t, j, observation, phase)
	appendReplayContractSnapshot(t, j, observation.After, "after")
	reopened := reopenReplayContractJournal(t, j, a)
	defer reopened.Close()
	if state := replayWithApproval(reopened.Events(), &a); !state.uncertain {
		t.Fatalf("accepted extra post-completion drain snapshot: %+v", state)
	}
}

func TestReplayContractRejectsFinalDrainSnapshotMismatch(t *testing.T) {
	j, a, phase := openReplayContractJournal(t)
	observed := drainObservationForApproval(a)
	appendReplayContractSnapshot(t, j, observed.Before, "before")
	appendReplayContractSnapshot(t, j, observed.After, "after")
	observed.Before.Runner = &drainRunnerIdentity{ID: 20, Name: "g01-test-worker-2", ScaleSetID: 7}
	observed.After.Runner = &drainRunnerIdentity{ID: 20, Name: "g01-test-worker-2", ScaleSetID: 7}
	appendReplayContractObservation(t, j, observed, phase)
	reopened := reopenReplayContractJournal(t, j, a)
	defer reopened.Close()
	if state := replayWithApproval(reopened.Events(), &a); !state.uncertain {
		t.Fatalf("accepted final observation whose runner differs from durable snapshots: %+v", state)
	}
}

func TestReplayContractRejectsForeignMetadataWithMatchingSetID(t *testing.T) {
	j, a, phase := openReplayContractJournal(t)
	foreign := drainObservationForApproval(a).Before
	foreign.Set.Name = "foreign-set"
	foreign.Set.RunnerGroupID = 99
	foreign.Set.Label = "foreign-set"
	foreign.Runner.Name = "foreign-worker"
	appendReplayContractSnapshot(t, j, foreign, "before")
	appendReplayContractSnapshot(t, j, foreign, "after")
	observation := drainObservationForApproval(a)
	observation.Before = foreign
	observation.After = foreign
	appendReplayContractObservation(t, j, observation, phase)
	reopened := reopenReplayContractJournal(t, j, a)
	defer reopened.Close()
	if state := replayWithApproval(reopened.Events(), &a); !state.uncertain {
		t.Fatalf("accepted foreign metadata with matching phase SetID: %+v", state)
	}
}

func TestReplayContractRequiresCompleteOrderedHistory(t *testing.T) {
	a := approval()
	observation := drainObservationForApproval(a)
	observation.Sequence = 4
	events := drainReplayPrefix()
	events = appendReplayContractSnapshotEvents(events, observation.Before, "before")
	events = appendReplayContractSnapshotEvents(events, observation.After, "after")
	events = append(events, Event{Kind: "observation", Operation: "drain", Sequence: 13, Drain: &observation})
	for prefix := len(drainReplayPrefix()); prefix < len(events); prefix++ {
		if state := replayWithApproval(events[:prefix], &a); !state.uncertain {
			t.Fatalf("crash prefix %d discharged drain fence: %+v", prefix, state)
		}
	}
	if state := replayWithApproval(events, &a); state.uncertain {
		t.Fatalf("complete ordered drain history remained fenced: %+v", state)
	}
}

func appendReplayContractSnapshotEvents(events []Event, snapshot drainSnapshot, stage string) []Event {
	return append(events,
		Event{Kind: "intent", Operation: "observe-owned"},
		Event{Kind: "result", Operation: "observe-owned", ID: snapshot.Set.ID},
		Event{Kind: "intent", Operation: "observe-runner"},
		Event{Kind: "result", Operation: "observe-runner", ID: snapshot.Runner.ID, DrainSnapshot: &snapshot, DrainSnapshotStage: stage},
	)
}

func TestReplayContractRejectsWrongSnapshotStageOrder(t *testing.T) {
	a := approval()
	observation := drainObservationForApproval(a)
	cases := []struct {
		name  string
		build func([]Event) []Event
	}{
		{
			name: "after before before",
			build: func(events []Event) []Event {
				events = appendReplayContractSnapshotEvents(events, observation.After, "after")
				events = appendReplayContractSnapshotEvents(events, observation.Before, "before")
				return append(events, Event{Kind: "observation", Operation: "drain", Sequence: 13, Drain: &observation})
			},
		},
		{
			name: "final before after",
			build: func(events []Event) []Event {
				events = append(events, Event{Kind: "observation", Operation: "drain", Sequence: 5, Drain: &observation})
				events = appendReplayContractSnapshotEvents(events, observation.Before, "before")
				events = appendReplayContractSnapshotEvents(events, observation.After, "after")
				return events
			},
		},
		{
			name: "duplicate before",
			build: func(events []Event) []Event {
				events = appendReplayContractSnapshotEvents(events, observation.Before, "before")
				events = appendReplayContractSnapshotEvents(events, observation.Before, "before")
				events = appendReplayContractSnapshotEvents(events, observation.After, "after")
				return append(events, Event{Kind: "observation", Operation: "drain", Sequence: 17, Drain: &observation})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if state := replayWithApproval(tc.build(drainReplayPrefix()), &a); !state.uncertain {
				t.Fatalf("accepted malformed ordered drain history: %+v", state)
			}
		})
	}
}

func TestReplayContractRejectsSnapshotOutsideDrainPhase(t *testing.T) {
	a := approval()
	observation := drainObservationForApproval(a)
	observation.Sequence = 4
	events := appendReplayContractSnapshotEvents(nil, observation.Before, "before")
	events = append(events, drainReplayPrefix()...)
	events = appendReplayContractSnapshotEvents(events, observation.After, "after")
	events = append(events, Event{Kind: "observation", Operation: "drain", Sequence: 17, Drain: &observation})
	if state := replayWithApproval(events, &a); !state.uncertain {
		t.Fatalf("snapshot outside active drain phase discharged fence: %+v", state)
	}
}

func TestReplayWithApprovalBindsDrainIdentityToApprovedSet(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	snapshot := drainObservationForApproval(a).Before
	observation := drainObservationForApproval(a)
	observation.Sequence = 4
	events := []Event{
		{Kind: "phase", Operation: "create", Sequence: 1},
		{Kind: "intent", Operation: "create", Sequence: 2},
		{Kind: "result", Operation: "create", Sequence: 3, ID: 7},
		{Kind: "phase", Operation: "drain", Sequence: 4, ID: 7},
	}
	events = appendReplayContractSnapshotEvents(events, snapshot, "before")
	events = appendReplayContractSnapshotEvents(events, snapshot, "after")
	events = append(events, Event{Kind: "observation", Operation: "drain", Sequence: 13, Drain: &observation})
	if state := replayWithApproval(events, &a); state.uncertain {
		t.Fatalf("approved drain identity was not accepted: %+v", state)
	}
	foreign := snapshot
	foreign.Set.Name = "foreign-set"
	foreign.Set.Label = "foreign-set"
	foreign.Set.RunnerGroupID = 99
	foreignRunnerName := "foreign-worker"
	for i := 4; i < len(events); i++ {
		if events[i].DrainSnapshot != nil {
			snapshot := *events[i].DrainSnapshot
			snapshot.Set = foreign.Set
			runner := *snapshot.Runner
			runner.Name = foreignRunnerName
			snapshot.Runner = &runner
			events[i].DrainSnapshot = &snapshot
		}
	}
	if state := replayWithApproval(events, &a); !state.uncertain {
		t.Fatalf("foreign snapshot identity discharged approved drain fence: %+v", state)
	}
}
