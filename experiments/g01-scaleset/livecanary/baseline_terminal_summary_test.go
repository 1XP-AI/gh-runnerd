//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestPairedTerminalCollectionOnlyRejectsInventedTerminal(t *testing.T) {
	f := newTerminalFixture(t, true)
	out, err := runFastPair(f.pairedIntegrationFixture)
	if err != nil || out.Outcome != collectionCollected || out.Terminal != nil {
		t.Fatal("collection-only positive failed")
	}
	events := f.c.j.Events()
	id, err := f.c.j.controllerIdentity()
	if err != nil {
		t.Fatal(err)
	}
	state, err := replayBaseline(events[:len(events)-1], id, f.c.a)
	if err != nil || state.terminalIntent.Sequence != 0 || state.measurementEvidence == nil {
		t.Fatal("actual collection history missing")
	}
	last := cloneEvent(events[len(events)-1])
	if last.Baseline == nil || last.Baseline.Stage != "collection" || last.Baseline.Collection.Terminal != nil {
		t.Fatal("actual last collection missing")
	}
	last.Baseline.Collection.Terminal = state.terminalSummary()
	path, dir, admission := f.c.j.file.Name(), f.c.j.directory, f.c.j.claim.directory
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.c.j.Close(); err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte{'\n'}), []byte{'\n'})
	lines[len(lines)-1], err = json.Marshal(last)
	if err != nil {
		t.Fatal(err)
	}
	data := append(bytes.Join(lines, []byte{'\n'}), '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = file.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	before := f.c.requests.Load()
	j, reopenErr := openJournalAtAdmission(dir, f.c.a, admission, func(file *os.File) error { return file.Sync() })
	if j != nil {
		_ = j.Close()
	}
	t.Logf("actual_collection_only=true terminal_parent=false invented_terminal_accepted=%t C_requests_delta=%d", reopenErr == nil, f.c.requests.Load()-before)
	if reopenErr == nil {
		t.Error("same-inode collection-only history accepted synthesized terminal object")
	}
	if f.c.requests.Load() != before {
		t.Fatal("replay issued request")
	}
}

func TestPairedTerminalPreParentFailureRetainsReturnedOnlyTerminal(t *testing.T) {
	f := newTerminalFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fired := false
	f.c.j.syncFile = func(file *os.File) error {
		if err := file.Sync(); err != nil {
			return err
		}
		e := terminalLastDiskEvent(file)
		if e.Baseline != nil && e.Baseline.Stage == "identity-sample" && e.Baseline.Outcome == "result" && e.Baseline.Sample.Round == 8 {
			fired = true
			cancel()
		}
		return nil
	}
	out, err := terminalRunContext(f, ctx)
	if !fired || err == nil || out.Terminal != terminalUnresolved || out.Collection.Rounds != 8 || out.Collection.Terminal == nil || out.Collection.Terminal.Measurement != collectionCollected || out.Collection.Terminal.Evidence == nil || out.Collection.Result.Sequence == 0 || f.sessionDeletes.Load() != 0 || f.workerDeletes.Load() != 0 || f.setDeletes.Load() != 0 {
		t.Fatal("actual pre-parent failure evidence missing")
	}
	var stored *baselineCollectionFacts
	var storedEvent Event
	for _, e := range f.c.j.Events() {
		if e.Baseline == nil {
			continue
		}
		if e.Baseline.Stage == "terminal" {
			t.Fatal("unexpected terminal parent")
		}
		if e.Baseline.Stage == "collection" {
			stored = e.Baseline.Collection
			storedEvent = e
		}
	}
	if stored == nil {
		t.Fatal("actual C summary missing")
	}
	identity, identityErr := f.c.j.controllerIdentity()
	if identityErr != nil || out.Collection.Result != controllerEventRef(identity, storedEvent) || out.Collection.Terminal.Intent.Sequence != 0 || out.Collection.Terminal.Result.Sequence != 0 {
		t.Fatal("returned projection invented or changed actual C/terminal reference")
	}
	t.Logf("round8_durable=true canceled_before_parent=true returned_evidence=true summary_ref=%d persisted_terminal=%t", out.Collection.Result.Sequence, stored.Terminal != nil)
	if stored.Terminal != nil {
		t.Error("pre-parent returned terminal evidence persisted without lifecycle intent")
	}
}
