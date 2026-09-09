package livecanary

import (
	"errors"
	"testing"
)

func setObservedRunnerPartition(o *drainObservation, registered, busy, idle int) {
	for _, poll := range []*drainPollObservation{&o.Poll, &o.NextPoll} {
		poll.Statistics.Registered = registered
		poll.Statistics.Busy = busy
		poll.Statistics.Idle = idle
	}
	for _, snapshot := range []*drainSnapshot{&o.Before, &o.After} {
		snapshot.Statistics.Registered = registered
		snapshot.Statistics.Busy = busy
		snapshot.Statistics.Idle = idle
	}
}

func TestSecurityReviewDrainRequiresOwnedIdleBeforeProof(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*drainObservation)
	}{
		{
			name: "two-runners",
			mutate: func(o *drainObservation) {
				setObservedRunnerPartition(o, 2, 0, 2)
			},
		},
		{
			name: "busy",
			mutate: func(o *drainObservation) {
				setObservedRunnerPartition(o, 1, 1, 0)
			},
		},
		{
			name: "missing-owned-identity",
			mutate: func(o *drainObservation) {
				o.Before.Runner = nil
				o.After.Runner = nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observation := validDrainTestObservation()
			tc.mutate(&observation)
			if validDrainObservation(&observation) {
				t.Fatalf("observed drain with %s prerequisite was accepted", tc.name)
			}
		})
	}

	a := approval()
	a.Phases = append(a.Phases, "drain")
	directory := privateDir(t)
	j, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range drainReplayPrefix() {
		if err := j.Append(event); err != nil {
			j.Close()
			t.Fatalf("append drain prefix: %v", err)
		}
	}
	observation := validDrainTestObservation()
	setObservedRunnerPartition(&observation, 2, 0, 2)
	phase := j.Events()[len(j.Events())-1]
	observation.Sequence = phase.Sequence
	appendErr := j.Append(Event{Kind: "observation", Operation: "drain", Drain: &observation})
	if appendErr == nil {
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := openTestJournal(t, directory, a)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if state := replay(reopened.Events()); !state.uncertain {
			t.Fatalf("real FileJournal accepted non-owned idle proof and discharged fence: %+v", state)
		}
		return
	}
	if !errors.Is(appendErr, ErrJournal) {
		j.Close()
		t.Fatalf("non-owned idle observation append = %v, want ErrJournal", appendErr)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if events := reopened.Events(); len(events) != len(drainReplayPrefix()) {
		t.Fatalf("reopened journal after rejected non-owned idle proof = %d events, want %d", len(events), len(drainReplayPrefix()))
	}
}

func TestSecurityReviewMatchingDrainPhaseDischargesAfterFileJournalReopen(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, "drain")
	directory := privateDir(t)
	j, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range drainReplayPrefix() {
		if err := j.Append(event); err != nil {
			j.Close()
			t.Fatalf("append drain prefix: %v", err)
		}
	}
	phase := j.Events()[len(j.Events())-1]
	observation := validDrainTestObservation()
	observation.Sequence = phase.Sequence
	if err := j.Append(Event{Kind: "observation", Operation: "drain", Drain: &observation}); err != nil {
		j.Close()
		t.Fatalf("append valid drain observation: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if state := replay(reopened.Events()); state.uncertain {
		t.Fatalf("matching one-idle drain phase remained fenced after reopen: %+v", state)
	}
}

func TestSecurityReviewValidOneIdleRunnerDrainObservation(t *testing.T) {
	if observation := validDrainTestObservation(); !validDrainObservation(&observation) {
		t.Fatal("valid one-idle runner drain observation rejected")
	}
}

func TestSecurityReviewInconclusiveDrainMayHaveMissingOwnedIdentity(t *testing.T) {
	observation := validDrainTestObservation()
	observation.Outcome = drainOutcomeInconclusive
	observation.Before.Runner = nil
	observation.After.Runner = nil
	if !validDrainObservation(&observation) {
		t.Fatal("inconclusive drain with missing runner identity was rejected")
	}
}

func TestSecurityReviewObservedDrainAllowsLegitimateJobCounterChanges(t *testing.T) {
	observation := validDrainTestObservation()
	for _, poll := range []*drainPollObservation{&observation.Poll, &observation.NextPoll} {
		poll.Statistics.Available = 4
		poll.Statistics.Acquired = 3
		poll.Statistics.Assigned = 2
		poll.Statistics.Running = 1
	}
	observation.After.Statistics.Available = 4
	observation.After.Statistics.Acquired = 3
	observation.After.Statistics.Assigned = 2
	observation.After.Statistics.Running = 1
	if !validDrainObservation(&observation) {
		t.Fatal("legitimate job-counter changes were rejected despite stable owned idle prerequisite")
	}
}
