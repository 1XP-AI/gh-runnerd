//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func jitBoundaryReopen(t *testing.T, f *jitBoundaryFixture) *FileJournal {
	t.Helper()
	dir, admission := f.base.j.directory, f.base.j.claim.directory
	f.base.j.Close()
	j, err := openJournalAtAdmission(dir, f.base.a, admission, func(file *os.File) error { return file.Sync() })
	if err != nil {
		t.Fatal("historical prefix reopen", err)
	}
	t.Cleanup(func() { j.Close() })
	f.base.j = j
	return j
}
func TestJITBoundaryPreparationRequiresOriginalFreshCreate(t *testing.T) {
	for _, phase := range []string{beforeJITPhase, "jit-loss"} {
		for _, fault := range []string{"valid", "raw-different", "legacy-six", "extra", "interleaved", "renewal", "consumed", "phase-only", "other-phase", "generic"} {
			t.Run(phase+"/"+fault, func(t *testing.T) {
				f := newJITBoundaryFixture(t, phase, "")
				j := f.base.j
				dir, admission := j.directory, j.claim.directory
				events := j.Events()
				path := filepath.Join(dir, "journal.jsonl")
				header := bytes.Clone(j.headerBytes)
				if fault == "raw-different" {
					raw, _ := json.Marshal(f.base.a)
					if os.WriteFile(f.path, raw, 0600) != nil {
						t.Fatal("raw rewrite")
					}
				}
				switch fault {
				case "legacy-six":
					events = events[1:]
				case "extra":
					events = append(events, Event{Kind: "observation", Operation: "inspect", ID: 7})
				case "interleaved":
					events = append(events[:3], append([]Event{{Kind: "observation", Operation: "inspect", ID: 7}}, events[3:]...)...)
				case "renewal":
					authority := j.authority
					authority.ExpiresAt = authority.ExpiresAt.Add(time.Minute)
					events = append(events, Event{Kind: "authority", Authority: &authority})
				case "consumed", "other-phase":
					h := f.handoff
					consumed := controllerHandoffConsumption{ReceiptNonce: h.receipt.ReceiptNonce, Phase: phase, ApprovalSHA256: handoff.Digest(h.snapshot), ReceiptSHA256: handoff.Digest(h.receiptBytes), KeyID: h.root.KeyID()}
					if fault == "other-phase" {
						consumed.Phase = "inspect"
					}
					events = append(events, Event{Kind: "controller-handoff-consumed", ControllerHandoff: &consumed})
				case "phase-only":
					events = append(events, Event{Kind: "phase", Operation: phase})
				}
				j.Close()
				var data bytes.Buffer
				data.Write(header)
				for i, e := range events {
					e.Sequence = i + 1
					raw, _ := json.Marshal(e)
					data.Write(raw)
					data.WriteByte('\n')
				}
				if os.WriteFile(path, data.Bytes(), 0600) != nil {
					t.Fatal("fixture history")
				}
				open := func(dir string, a Approval) (*FileJournal, error) {
					return openJournalAtAdmission(dir, a, admission, func(file *os.File) error { return file.Sync() })
				}
				var receipt PreparationReceipt
				var err error
				if fault == "generic" {
					receipt, err = prepareJournal(dir, f.base.a, phase, open)
				} else {
					receipt, err = prepareJITBoundaryJournal(dir, f.base.a, f.path, phase, open)
				}
				if fault == "valid" {
					if err != nil || receipt.Phase != phase || receipt.JournalDigest == "" {
						t.Fatal("valid fresh raw preparation rejected", err)
					}
				} else if err == nil || !reflect.DeepEqual(receipt, PreparationReceipt{}) {
					t.Fatal("nonfresh/raw-drift preparation minted receipt", fault)
				}
				if f.base.requests.Load() != 0 {
					t.Fatal("preparation contacted API")
				}
			})
		}
	}
}

func TestJITBoundaryAtomicPrefixBeforeCredentialInput(t *testing.T) {
	for _, phase := range []string{beforeJITPhase, "jit-loss"} {
		t.Run(phase, func(t *testing.T) {
			f := newJITBoundaryFixture(t, phase, "")
			h := f.handoff
			calls, reads := 0, 0
			// The fifth clock call is consumeControllerHandoffAt's freshly sampled
			// appendNow under mu, after the receiver's independent preliminary check.
			now := func() time.Time {
				calls++
				if calls == 5 {
					j := f.base.j
					e := Event{Sequence: 8, Kind: "phase", Operation: "inspect"}
					if _, err := j.appendStored(e); err != nil {
						t.Fatal(err)
					}
				}
				return time.Now()
			}
			err := runControllerHandoffPhase(context.Background(), f.base.j, h.snapshot, h.receiptBytes, phase, h.request.Source, h.root, now, func(context.Context) ([]byte, error) { reads++; return []byte("invalid"), nil }, func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("prefix race built API")
				return nil, nil
			}, nil)
			if err == nil || calls < 5 || reads != 0 || f.base.requests.Load() != 0 || len(f.base.j.Events()) != 8 {
				t.Fatal("atomic input fence accepted changed prefix", calls, reads)
			}
		})
	}
}
func TestJITBoundaryConcurrentConsumesOnce(t *testing.T) {
	f := newJITBoundaryFixture(t, beforeJITPhase, "")
	h := f.handoff
	var reads atomic.Int32
	start := make(chan struct{})
	done := make(chan error, 2)
	for n := 0; n < 2; n++ {
		receipt := h.receipt
		receipt.ReceiptNonce = strings.Repeat(string(rune('e'+n)), 32)
		encoded := h.encodeReceipt(t, receipt, true)
		go func() {
			<-start
			done <- runControllerHandoffPhase(context.Background(), f.base.j, h.snapshot, encoded, f.phase, h.request.Source, h.root, time.Now, func(context.Context) ([]byte, error) { reads.Add(1); return []byte("invalid"), nil }, func(context.Context, Approval, Credentials) (API, error) { return nil, ErrApproval }, nil)
		}()
	}
	close(start)
	for n := 0; n < 2; n++ {
		if err := <-done; err == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
	if reads.Load() != 1 || len(f.base.j.Events()) != 8 || !replay(f.base.j.Events()).reserved || f.base.requests.Load() != 0 {
		t.Fatal("duplicate consumer read credentials/effected", reads.Load())
	}
}
func TestJITBoundaryHistoryCannotResumeOrRenew(t *testing.T) {
	for _, phase := range []string{beforeJITPhase, "jit-loss"} {
		for _, prefix := range []string{"consume-only", "phase-only", "request-validated", "send-or-stop", "complete"} {
			t.Run(phase+"/"+prefix, func(t *testing.T) {
				f := newJITBoundaryFixture(t, phase, "")
				if err := f.run(t, context.Background()); err != nil {
					t.Fatal("complete setup", err)
				}
				j := f.base.j
				events := j.Events()
				data, _ := os.ReadFile(j.file.Name())
				lines := bytes.Split(data, []byte{'\n'})
				keep := len(events)
				switch prefix {
				case "consume-only":
					keep = 8
				case "phase-only":
					keep = 9
				case "request-validated":
					keep = 18
				case "send-or-stop":
					keep = 19
				}
				dir, admission := j.directory, j.claim.directory
				j.Close()
				os.WriteFile(filepath.Join(dir, "journal.jsonl"), append(bytes.Join(lines[:keep+1], []byte{'\n'}), '\n'), 0600)
				j, err := openJournalAtAdmission(dir, f.base.a, admission, func(file *os.File) error { return file.Sync() })
				if err != nil {
					t.Fatal("partial replay rejected historical truth", err)
				}
				defer j.Close()
				f.base.j = j
				s := replay(j.Events())
				if !s.reserved || !s.uncertain || !s.workObserved {
					t.Fatal("partial replay cleared quarantine")
				}
				before := f.base.requests.Load()
				f.handoff.receipt.ReceiptNonce = strings.Repeat("f", 32)
				f.handoff.receiptBytes = f.handoff.encodeReceipt(t, f.handoff.receipt, true)
				if f.run(t, context.Background()) == nil || (&Driver{f.base.a, j, f.base.api}).Run(context.Background(), beforeJITPhase) == nil || (&Driver{f.base.a, j, f.base.api}).Run(context.Background(), "jit-loss") == nil || f.base.requests.Load() != before {
					t.Fatal("reopen/fresh nonce/alternate mode restored JIT authority")
				}
				if keep >= 10 && keep < len(events) {
					next := events[keep]
					if j.Append(Event{Kind: "jit-boundary", JITBoundary: next.JITBoundary}) == nil {
						t.Fatal("partial typed history resumed through generic Append")
					}
				}
				if keep >= 10 || phase == beforeJITPhase {
					renew := f.base.a
					renew.ExpiresAt = renew.ExpiresAt.Add(time.Minute)
					j.Close()
					if opened, e := openJournalAtAdmission(dir, renew, admission, func(file *os.File) error { return file.Sync() }); e == nil {
						opened.Close()
						t.Fatal("renewal restored boundary history authority")
					}
				}
			})
		}
	}
}

func TestJITBoundaryStrictJournalSchemaAndRawReferences(t *testing.T) {
	for _, fault := range []string{"version", "mode", "stage", "missing-zero", "null-zero", "false-required", "missing-false", "null-retained", "unknown", "union-zero", "union-null", "nested-extra", "nested-null", "nested-missing", "foreign-ref", "raw-prior", "order", "typed-after-old", "trailing-old"} {
		t.Run(fault, func(t *testing.T) {
			f := newJITBoundaryFixture(t, beforeJITPhase, "")
			if f.run(t, context.Background()) != nil {
				t.Fatal("setup")
			}
			j := f.base.j
			dir, admission := j.directory, j.claim.directory
			data, _ := os.ReadFile(j.file.Name())
			j.Close()
			lines := bytes.Split(data, []byte{'\n'})
			index := 19
			var e map[string]any
			json.Unmarshal(lines[index], &e)
			r := e["jit_boundary"].(map[string]any)
			switch fault {
			case "version":
				r["version"] = 2
			case "mode":
				r["phase"] = "jit-loss"
			case "stage":
				r["stage"] = "complete"
			case "missing-zero":
				delete(r, "observed_forward_count")
			case "null-zero":
				r["observed_forward_count"] = nil
			case "false-required":
				r["request_validated"] = false
			case "missing-false", "null-retained":
				index = 22
				json.Unmarshal(lines[index], &e)
				r = e["jit_boundary"].(map[string]any)
				summary := r["summary"].(map[string]any)
				if fault == "missing-false" {
					delete(summary, "jit_retry_allowed")
				} else {
					summary["set_retained"] = nil
				}
			case "unknown":
				r["secret"] = "forbidden"
			case "union-zero":
				e["id"] = 0
			case "union-null":
				e["baseline"] = nil
			case "nested-extra":
				r["identity"].(map[string]any)["unknown"] = 0
			case "nested-null":
				r["identity"].(map[string]any)["claim"] = nil
			case "nested-missing":
				delete(r["identity"].(map[string]any)["claim"].(map[string]any), "device")
			case "foreign-ref":
				r["previous"].(map[string]any)["event_sha256"] = strings.Repeat("f", 64)
			case "raw-prior":
				lines[7] = append([]byte{' '}, lines[7]...)
			case "order":
				r["previous"], r["creation"] = r["creation"], r["previous"]
			case "typed-after-old":
				e["sequence"] = 23
				lines = append(lines[:23], append([][]byte{lines[19]}, lines[23:]...)...)
			case "trailing-old":
				old, _ := json.Marshal(Event{Sequence: 23, Kind: "response", Operation: "jit"})
				lines = append(lines[:23], append([][]byte{old}, lines[23:]...)...)
			}
			if fault != "raw-prior" && fault != "trailing-old" {
				lines[index], _ = json.Marshal(e)
			}
			os.WriteFile(filepath.Join(dir, "journal.jsonl"), bytes.Join(lines, []byte{'\n'}), 0600)
			if opened, err := openJournalAtAdmission(dir, f.base.a, admission, func(file *os.File) error { return file.Sync() }); err == nil {
				opened.Close()
				t.Fatal("schema/physical-reference mutation replayed", fault)
			}
		})
	}
}

func TestJITBoundaryProofCannotCrossOpen(t *testing.T) {
	f := newJITBoundaryFixture(t, beforeJITPhase, "")
	h := f.handoff
	ctx := context.WithValue(context.Background(), jitBoundarySnapshotKey{}, handoff.Digest(h.snapshot))
	ctx = context.WithValue(ctx, jitBoundaryRawKey{}, h.snapshot)
	data, proof, err := consumeControllerHandoffWithProof(ctx, f.base.j, h.snapshot, f.phase, h.request.Source, newControllerHandoffMemorySource(h.receiptBytes), h.root, time.Now, func(context.Context) ([]byte, error) { return handoffCredentialBytes(t, h.approval), nil })
	clear(data)
	if err != nil {
		t.Fatal("proof setup", err)
	}
	jitBoundaryReopen(t, f)
	if (&Driver{f.base.a, f.base.j, f.base.api}).runWithHandoff(ctx, f.phase, &proof) == nil || f.base.requests.Load() != 0 || len(f.base.j.Events()) != 8 {
		t.Fatal("old per-open proof entered new open")
	}
}
func TestJITBoundaryDirectRunAndCredentialShapes(t *testing.T) {
	a := approval()
	a.Phases = append(a.Phases, beforeJITPhase)
	j := &memoryJournal{}
	api := &fakeAPI{}
	if (&Driver{a, j, api}).Run(context.Background(), beforeJITPhase) == nil || len(j.Events()) != 0 {
		t.Fatal("direct before-jit borrowed legacy harness authority")
	}
	for _, phase := range []string{beforeJITPhase, "jit-loss"} {
		a := approval()
		a.Phases = []string{"create", phase}
		c := credentials(a)
		c.VerificationToken = ""
		valid := c.validate(a, time.Now()) == nil
		if valid != (phase == "jit-loss") {
			t.Fatal("JIT verification credential shape changed", phase)
		}
	}
	if len(phases) != 11 {
		t.Fatal("controller finite phase schema widened")
	}
}
func TestJITBoundaryCloneOwnsNestedPayload(t *testing.T) {
	f := newJITBoundaryFixture(t, beforeJITPhase, "")
	if f.run(t, context.Background()) != nil {
		t.Fatal("setup")
	}
	before := f.base.j.Events()
	copy := f.base.j.Events()
	*copy[9].JITBoundary.Retained = false
	copy[13].JITBoundary.Set.Labels[0].Name = "foreign"
	*copy[13].JITBoundary.Set.Statistics.Assigned = 1
	if !reflect.DeepEqual(before, f.base.j.Events()) {
		t.Fatal("clone aliases journal payload")
	}
}
