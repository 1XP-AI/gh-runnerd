//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestPairedTerminalFinalResultCapacity(t *testing.T) {
	for _, fault := range []string{"actual-result-consumes-reserve", "exactly-two-records", "less-than-two-records", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			f := newTerminalFixture(t, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			intentSeen, resultSeen := false, false
			var remaining int64
			f.c.j.syncFile = func(file *os.File) error {
				e := terminalLastDiskEvent(file)
				if err := file.Sync(); err != nil {
					return err
				}
				if e.Baseline == nil || e.Baseline.Stage != "terminal-roster-final" {
					return nil
				}
				if e.Baseline.Outcome == "intent" {
					intentSeen = true
					// The unchanged pre-read gate has all four maximum records.
					terminalPadValidLines(t, file, baselineJournalLimit-4*baselineRecordLimit)
				}
				if e.Baseline.Outcome == "result" {
					resultSeen = true
					switch fault {
					case "exactly-two-records":
						terminalPadValidLines(t, file, baselineJournalLimit-2*baselineRecordLimit)
					case "less-than-two-records":
						terminalPadValidLines(t, file, baselineJournalLimit-2*baselineRecordLimit+1)
					case "canceled":
						cancel()
					}
					info, err := file.Stat()
					if err != nil {
						return err
					}
					remaining = baselineJournalLimit - info.Size()
				}
				return nil
			}
			out, err := terminalRunContext(f, ctx)
			if !intentSeen || !resultSeen || f.rosters.Load() != 4 || f.sessionDeletes.Load() != 1 || f.workerDeletes.Load() != 1 || f.setDeletes.Load() != 1 || f.setAbsences.Load() != 1 {
				t.Fatal("capacity fixture did not execute the actual final read and cleanup")
			}
			if fault == "actual-result-consumes-reserve" && (remaining < 2*baselineRecordLimit || remaining >= 4*baselineRecordLimit) {
				t.Fatal("actual final result did not cross only the obsolete reserve")
			}
			wantComplete := fault == "actual-result-consumes-reserve" || fault == "exactly-two-records"
			if wantComplete {
				if err != nil || out.Terminal != terminalComplete || out.Collection.Outcome != collectionCollected || out.Collection.Result.Sequence == 0 {
					t.Errorf("complete cleanup rejected with remaining=%d: terminal=%s error=%v", remaining, out.Terminal, err)
				}
			} else if err == nil || out.Terminal != terminalUnresolved || out.Collection.Outcome != collectionUnresolved {
				t.Error("insufficient capacity or canceled authority permitted completion")
			}
			facts := out.Collection.Terminal
			if facts == nil || facts.Measurement != collectionCollected || facts.WorkerDeletion == nil || facts.WorkerDeletion.AbsenceResult == nil || !refPresent(facts.SetDeleteResult) || !refPresent(facts.SetAbsenceResult) {
				t.Error("capacity boundary erased known cleanup/measurement evidence")
			}
			// Padding must remain a valid journal, not a corrupt-tail refusal.
			dir, admission := f.c.j.directory, f.c.j.claim.directory
			_ = f.c.j.Close()
			j, reopenErr := openJournalAtAdmission(dir, f.c.a, admission, func(file *os.File) error { return file.Sync() })
			if reopenErr != nil {
				t.Fatal("near-limit journal could not reopen/replay")
			}
			f.c.j = j
			t.Cleanup(func() { _ = j.Close() })
			before := [2]int32{f.c.requests.Load(), f.dockerReads.Load()}
			if _, replayErr := f.run(); replayErr == nil || before != [2]int32{f.c.requests.Load(), f.dockerReads.Load()} {
				t.Fatal("reopened terminal history resumed effects")
			}
		})
	}
}

// Pad existing complete JSON lines, preserving decoded events, their references,
// the pinned inode, and both the per-line and cumulative journal limits.
func terminalPadValidLines(t *testing.T, file *os.File, size int) {
	t.Helper()
	data, err := os.ReadFile(file.Name())
	if err != nil || len(data) > size || size > baselineJournalLimit || !bytes.HasSuffix(data, []byte{'\n'}) {
		t.Fatal("invalid capacity fixture size")
	}
	left := size - len(data)
	var padded bytes.Buffer
	for _, line := range bytes.Split(data[:len(data)-1], []byte{'\n'}) {
		if len(line)+1 > baselineRecordLimit {
			t.Fatal("capacity fixture line already too long")
		}
		n := min(left, baselineRecordLimit-len(line)-1)
		padded.Write(line)
		padded.Write(bytes.Repeat([]byte{' '}, n))
		padded.WriteByte('\n')
		left -= n
	}
	if left != 0 || file.Truncate(0) != nil {
		t.Fatal("insufficient legal line padding for capacity fixture")
	}
	if n, err := file.Write(padded.Bytes()); err != nil || n != size || file.Sync() != nil {
		t.Fatal("persist capacity fixture")
	}
}
