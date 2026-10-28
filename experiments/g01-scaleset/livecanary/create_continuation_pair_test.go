//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"os"
	"testing"
)

func TestCreateContinuationReceiptBackedPairedCollection(t *testing.T) {
	var fake *fakeAPI
	f := newPairedIntegrationFixtureConfigured(t, &pairedFixtureConfiguration{controllerCreate: func(j *FileJournal, a Approval) { _, fake = completedHandoffCreate(t, j, a) }})
	id, err := f.c.j.controllerIdentity()
	if err != nil {
		t.Fatal(err)
	}
	before := f.c.j.Events()
	inventory, creation := controllerEventRef(id, before[2]), controllerEventRef(id, before[6])
	out, err := runFastPair(f)
	if err != nil || out.Outcome != collectionCollected || out.Rounds != 8 {
		t.Fatalf("receipt-backed pair failed: %v", err)
	}
	continuations := 0
	for _, e := range f.c.j.Events() {
		r := e.Baseline
		if r == nil {
			continue
		}
		if r.Creation != creation {
			t.Fatal("paired creation physical ref changed")
		}
		if r.Stage == "continuation" && r.Outcome == "result" {
			continuations++
		}
		if r.Roster != nil && r.Roster.Inventory != inventory {
			t.Fatal("paired inventory physical ref changed")
		}
	}
	if inventory.Sequence != 3 || creation.Sequence != 7 || continuations != 1 || fake.createCalls != 1 || f.c.acks.Load() != 2 || f.c.acquires.Load() != 1 || f.jit.Load() != 1 || f.creates.Load() != 1 || f.starts.Load() != 1 || f.c.forbidden.Load() != 0 || f.cleanup.Load() != 0 {
		t.Fatal("paired singleton counts or references changed")
	}
	dir, admission := f.c.j.directory, f.c.j.claim.directory
	if err = f.c.j.Close(); err != nil {
		t.Fatal(err)
	}
	f.c.j, err = openJournalAtAdmission(dir, f.c.a, admission, func(file *os.File) error { return file.Sync() })
	if err != nil {
		t.Fatal(err)
	}
	h, err := replayBaseline(f.c.j.Events(), id, f.c.a)
	if err != nil || !h.complete || !h.continued || h.creation != creation {
		t.Fatal("paired replay lost physical refs", err)
	}
	requests := f.c.requests.Load()
	docker := f.dockerReads.Load()
	if _, err = runFastPair(f); err == nil || f.c.requests.Load() != requests || f.dockerReads.Load() != docker || fake.createCalls != 1 {
		t.Fatal("paired reopen resumed effects")
	}
}
