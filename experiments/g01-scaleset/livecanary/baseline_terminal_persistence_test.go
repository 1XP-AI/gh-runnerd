//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"context"
	"os"
	"testing"
)

func TestPairedTerminalActualControllerSyncFailures(t *testing.T) {
	for _, stage := range append([]string{"terminal"}, append(append([]string(nil), terminalSteps[:]...), "collection")...) {
		outcomes := []string{"intent", "result"}
		if stage == "terminal-decision" || stage == "collection" {
			outcomes = []string{"observed"}
		}
		for _, outcome := range outcomes {
			t.Run(stage+"/"+outcome, func(t *testing.T) {
				t.Parallel()
				f := newTerminalFixture(t, true)
				fired := false
				f.c.j.syncFile = func(file *os.File) error {
					e := terminalLastDiskEvent(file)
					if !fired && e.Baseline != nil && e.Baseline.Stage == stage && e.Baseline.Outcome == outcome {
						fired = true
						return ErrJournal
					}
					return file.Sync()
				}
				out, err := f.run()
				if !fired || err == nil || out.Terminal == terminalComplete || out.Collection.Outcome != collectionUnresolved || out.Collection.Result.Sequence != 0 {
					t.Fatal("failed terminal durability returned success")
				}
				if out.Collection.Terminal == nil || out.Collection.Terminal.Measurement != collectionCollected || out.Collection.Terminal.Evidence == nil {
					t.Fatal("completed measurement references lost on terminal persistence failure")
				}
				if stage == "collection" {
					facts := out.Collection.Terminal
					if facts == nil || facts.Measurement != collectionCollected || out.Collection.Rounds != 8 || facts.WorkerDeletion == nil || facts.WorkerDeletion.AbsenceResult == nil || !refPresent(facts.SetDeleteResult) || !refPresent(facts.SetAbsenceResult) || !refPresent(facts.SessionCloseResult) {
						t.Fatal("final summary failure erased earlier durable evidence")
					}
				}
				if stage == "terminal-session-close" && outcome == "intent" && f.sessionDeletes.Load() != 0 {
					t.Fatal("close before durable intent")
				}
				if stage == "terminal-worker-delete" && outcome == "intent" && f.workerDeletes.Load() != 0 {
					t.Fatal("worker delete before durable C intent")
				}
				if stage == "terminal-set-delete" && outcome == "intent" && f.setDeletes.Load() != 0 {
					t.Fatal("set delete before durable intent")
				}
				terminalNoReplay(t, f)
			})
		}
	}
}
func TestPairedTerminalActualWorkerSyncFailures(t *testing.T) {
	// The ordinary execution contributes 23 records. DeleteTerminal appends a
	// fresh inspect pair, delete pair, and distinct post-delete inspect pair.
	for n := 24; n <= 29; n++ {
		t.Run(string(rune('a'+n-24)), func(t *testing.T) {
			t.Parallel()
			f := newTerminalFixture(t, true)
			f.wf.FailRecordSyncForTest(n)
			out, err := f.run()
			if err == nil || out.Terminal != terminalUnresolved || f.setDeletes.Load() != 0 || out.Collection.Terminal == nil || out.Collection.Terminal.Measurement != collectionCollected || !refPresent(out.Collection.Terminal.SessionCloseResult) {
				t.Fatal("worker persistence failure crossed set deletion boundary")
			}
			if n <= 26 && f.workerDeletes.Load() != 0 {
				t.Fatal("worker delete before durable worker intent")
			}
			if n >= 28 && (out.Collection.Terminal.WorkerDeletion == nil || out.Collection.Terminal.WorkerDeletion.AbsenceResult != nil) {
				t.Fatal("known worker deletion lost on post-read sync failure")
			}
			terminalNoReplay(t, f)
		})
	}
}
func TestPairedTerminalPostIntentAuthorityBoundaries(t *testing.T) {
	for _, stage := range []string{"terminal-session-close", "terminal-worker-delete", "terminal-set-delete", "terminal-set-absence"} {
		for _, fault := range []string{"cancel", "capacity"} {
			t.Run(stage+"/"+fault, func(t *testing.T) {
				t.Parallel()
				f := newTerminalFixture(t, true)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				fired := false
				f.c.j.syncFile = func(file *os.File) error {
					e := terminalLastDiskEvent(file)
					if err := file.Sync(); err != nil {
						return err
					}
					if !fired && e.Baseline != nil && e.Baseline.Stage == stage && e.Baseline.Outcome == "intent" {
						fired = true
						if fault == "cancel" {
							cancel()
						} else {
							info, _ := file.Stat()
							padding := make([]byte, baselineJournalLimit-info.Size()-baselineRecordLimit)
							for i := range padding {
								padding[i] = ' '
							}
							_, _ = file.Write(padding)
							_ = file.Sync()
						}
					}
					return nil
				}
				out, err := terminalRunContext(f, ctx)
				if !fired || err == nil || out.Terminal != terminalUnresolved {
					t.Fatal("post-intent boundary allowed success")
				}
				switch stage {
				case "terminal-session-close":
					if f.sessionDeletes.Load() != 0 {
						t.Fatal("close after authority loss")
					}
				case "terminal-worker-delete":
					if f.workerDeletes.Load() != 0 {
						t.Fatal("worker delete after boundary")
					}
				case "terminal-set-delete":
					if f.setDeletes.Load() != 0 {
						t.Fatal("set delete after boundary")
					}
				case "terminal-set-absence":
					if f.setAbsences.Load() != 0 {
						t.Fatal("post-read after boundary")
					}
				}
				terminalNoReplay(t, f)
			})
		}
	}
}

func TestPairedTerminalWorkerReceiptSurvivesControllerWriteFailure(t *testing.T) {
	f := newTerminalFixture(t, true)
	fired := false
	f.c.j.syncFile = func(file *os.File) error {
		e := terminalLastDiskEvent(file)
		if !fired && e.Baseline != nil && e.Baseline.Stage == "terminal-worker-delete" && e.Baseline.Outcome == "result" {
			fired = true
			return ErrJournal
		}
		return file.Sync()
	}
	out, err := f.run()
	facts := out.Collection.Terminal
	events := f.wf.Journal.Events()
	if !fired || err == nil || out.Terminal != terminalUnresolved || facts == nil || facts.WorkerDeleteResult.Sequence != 0 || out.Collection.Result.Sequence != 0 || f.setDeletes.Load() != 0 {
		t.Fatal("failed bridge persistence returned C authority")
	}
	if len(events) != 29 || events[26].Paired.Delete == nil || events[26].Paired.Delete.Kind != "result" || events[28].Paired.Local == nil || events[28].Paired.Local.HTTPStatus != 404 {
		t.Fatal("actual worker receipt fixture did not persist")
	}
	receipt := facts.WorkerDeletion
	if receipt == nil || receipt.Result.Sequence != 27 || receipt.AbsenceResult == nil || receipt.AbsenceResult.Sequence != 29 {
		t.Fatal("independently durable worker deletion/absence receipt lost")
	}
	terminalNoReplay(t, f)
}
