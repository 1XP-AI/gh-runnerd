package livecanary

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBaselineCapacityAndWriteFailureBeforeEffect(t *testing.T) {
	for _, name := range []string{"capacity", "write"} {
		t.Run(name, func(t *testing.T) {
			f := newBaselineFixture(t, nil)
			if name == "capacity" {
				data, err := os.ReadFile(f.j.file.Name())
				if err != nil {
					t.Fatal(err)
				}
				data = append(data[:len(data)-1], []byte(strings.Repeat(" ", baselineJournalLimit-2*baselineRecordLimit-len(data)+2))...)
				data = append(data, '\n')
				if f.j.file.Truncate(0) != nil {
					t.Fatal("fixture size")
				}
				if _, err = f.j.file.Write(data); err != nil || f.j.file.Sync() != nil {
					t.Fatal("fixture padding")
				}
			} else {
				old := f.j.file
				readOnly, err := os.Open(old.Name())
				if err != nil {
					t.Fatal(err)
				}
				f.j.file = readOnly
				t.Cleanup(func() { _ = old.Close() })
			}
			if err := f.run(t); err == nil || f.requests.Load() != 0 {
				t.Fatal("capacity/write failure reached server")
			}
		})
	}
}
func TestBaselineReplayRefPayloadAndOrdering(t *testing.T) {
	for _, name := range []string{"corrupt-ref", "unknown-field", "interleaved-legacy", "skip-ACK", "skip-callback", "oversize-record", "torn-tail"} {
		t.Run(name, func(t *testing.T) {
			f := newBaselineFixture(t, nil)
			if err := f.run(t); err != nil {
				t.Fatal(err)
			}
			events := f.j.Events()
			dir := f.j.directory
			path := f.j.file.Name()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f.release()
			f.release = nil
			if f.j.Close() != nil {
				t.Fatal("close fixture")
			}
			lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			switch name {
			case "corrupt-ref":
				for i, e := range events {
					if e.Baseline != nil && e.Baseline.Stage == "acquire" && e.Baseline.Outcome == "result" {
						e.Baseline.Intent.EventSHA256 = strings.Repeat("0", 64)
						encoded, _ := json.Marshal(e)
						lines[i+1] = string(encoded)
						break
					}
				}
			case "unknown-field":
				lines[len(lines)-1] = strings.TrimSuffix(lines[len(lines)-1], "}") + `,"unexpected":true}`
			case "interleaved-legacy":
				e := Event{Sequence: len(events) + 1, Kind: "intent", Operation: "delete", ID: 7}
				encoded, _ := json.Marshal(e)
				lines = append(lines, string(encoded))
			case "skip-ACK", "skip-callback":
				for i, e := range events {
					r := e.Baseline
					if r != nil && ((name == "skip-ACK" && r.Stage == "ack" && r.Outcome == "intent") || (name == "skip-callback" && r.Stage == "started")) {
						n := 1
						e.Baseline = &baselineRecord{Version: 1, Stage: "desired", Outcome: "observed", SetID: 7, SessionID: r.SessionID, Creation: r.Creation, BatchRef: r.BatchRef, Desired: &n}
						encoded, _ := json.Marshal(e)
						lines = append(lines[:i+1], string(encoded))
						break
					}
				}
			case "oversize-record":
				last := events[len(events)-1]
				last.Baseline.SessionID = strings.Repeat("x", baselineRecordLimit)
				encoded, _ := json.Marshal(last)
				lines[len(lines)-1] = string(encoded)
			case "torn-tail":
				lines[len(lines)-1] = strings.TrimSuffix(lines[len(lines)-1], "}")
			}
			if os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600) != nil {
				t.Fatal("fixture rewrite")
			}
			if reopened, err := openTestJournal(t, dir, f.a); err == nil {
				_ = reopened.Close()
				t.Fatal("invalid history accepted")
			}
		})
	}
}
func TestBaselineClaimlessRenewalAndDomain(t *testing.T) {
	f := newBaselineFixture(t, nil)
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	oldEvents := f.j.Events()
	id, err := f.j.controllerIdentity()
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(id)
	if !strings.HasPrefix(string(encoded), `{"ownership_sha256":`) || !strings.Contains(string(encoded), `,"state":{"device":`) || !strings.Contains(string(encoded), `,"journal":{"device":`) || !strings.Contains(string(encoded), `,"admission_directory":{"device":`) || !strings.Contains(string(encoded), `,"claim":{"device":`) {
		t.Fatal("stable pair identity contract changed")
	}
	dir := f.j.directory
	f.release()
	f.release = nil
	if f.j.Close() != nil {
		t.Fatal("close")
	}
	renewed := f.a
	renewed.ExpiresAt = renewed.ExpiresAt.Add(time.Minute)
	renewed.Phases = []string{"inspect", "cleanup"}
	j, err := openTestJournal(t, dir, renewed)
	if err != nil {
		t.Fatal("claimless renewal rejected", err)
	}
	f.j = j
	idAfter, err := j.controllerIdentity()
	if err != nil || idAfter != id {
		t.Fatal("renewal altered stable identity")
	}
	for _, e := range oldEvents {
		if controllerEventRef(id, e) != controllerEventRef(idAfter, j.Events()[e.Sequence-1]) {
			t.Fatal("renewal changed assigned reference")
		}
	}
	if j.Events()[len(oldEvents)].Kind != "authority" {
		t.Fatal("renewal not stored before claim open")
	}
	release, err := j.authorize(renewed)
	if err != nil {
		t.Fatal(err)
	}
	f.release = release
	if _, err = newBaselineListenerHeld(context.Background(), renewed, j, f.api, 7); err == nil {
		t.Fatal("renewal granted baseline restart")
	}
}
func TestBaselinePollIdentityAndOperationReuse(t *testing.T) {
	for _, name := range []string{"repeated-message", "different-request", "extra-available"} {
		t.Run(name, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, v any) any {
				if name == "repeated-message" && stage == "poll2" {
					v.(map[string]any)["messageId"] = 9
				}
				if stage == "poll2-items" && name != "repeated-message" {
					items := v.([]any)
					if name == "extra-available" {
						return append(items, baselineFixtureItem(approval(), "JobAvailable"))
					}
					items[0].(map[string]any)["runnerRequestId"] = 43
				}
				return v
			})
			baselineRefuses(t, f, 1, 1, 1)
		})
	}

	for _, op := range []string{"outstanding-poll", "wrong-ACK", "before-ACK", "desired-before-ACK", "duplicate-ACK", "duplicate-request"} {
		t.Run(op, func(t *testing.T) {
			f := newBaselineFixture(t, nil)
			b := baselineStepped(t, f)
			m, err := b.GetMessage(context.Background(), 0, 1)
			if err != nil {
				t.Fatal(err)
			}
			if op == "duplicate-ACK" || op == "duplicate-request" {
				if err = b.DeleteMessage(context.Background(), m.MessageID); err != nil {
					t.Fatal(err)
				}
			}
			before := f.requests.Load()
			switch op {
			case "outstanding-poll":
				_, err = b.GetMessage(context.Background(), 0, 1)
			case "wrong-ACK":
				err = b.DeleteMessage(context.Background(), m.MessageID+1)
			case "before-ACK":
				_, err = b.AcquireJobs(context.Background(), []int64{42})
			case "desired-before-ACK":
				_, err = b.HandleDesiredRunnerCount(context.Background(), 1)
			case "duplicate-ACK":
				err = b.DeleteMessage(context.Background(), m.MessageID)
			case "duplicate-request":
				_, err = b.AcquireJobs(context.Background(), []int64{42, 42})
			}
			if err == nil || f.requests.Load() != before {
				t.Fatal("invalid/replayed operation reached server")
			}
		})
	}

}
func TestBaselineIntentDeadlineAfterFsync(t *testing.T) {
	f := newBaselineFixture(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	b, err := newBaselineListenerHeld(ctx, f.a, f.j, f.api, 7)
	if err != nil {
		t.Fatal(err)
	}
	f.j.syncFile = func(file *os.File) error {
		if err := file.Sync(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	}
	if err = b.run(func(context.Context, baselineAcquisition) error { return nil }); err == nil || f.requests.Load() != 0 {
		t.Fatal("expired intent performed a request")
	}
	if baselineLast(t, f).Outcome != "intent" {
		t.Fatal("deadline erased pending intent")
	}
}
func TestBaselineConstructorRefusesChangedFiles(t *testing.T) {
	for _, name := range []string{"state", "claim"} {
		t.Run(name, func(t *testing.T) {
			f := newBaselineFixture(t, nil)
			if name == "state" {
				old := f.j.directory
				if err := os.Rename(old, old+".old"); err != nil {
					t.Fatal(err)
				}
				if os.Mkdir(old, 0700) != nil {
					t.Fatal("new state")
				}
				data, _ := os.ReadFile(filepath.Join(old+".old", "journal.jsonl"))
				if os.WriteFile(filepath.Join(old, "journal.jsonl"), data, 0600) != nil {
					t.Fatal("new journal")
				}
			} else {
				path := f.j.claim.file.Name()
				data, _ := os.ReadFile(path)
				if os.Rename(path, path+".old") != nil || os.WriteFile(path, data, 0600) != nil {
					t.Fatal("claim replacement")
				}
			}
			if _, err := newBaselineListenerHeld(context.Background(), f.a, f.j, f.api, 7); err == nil || f.requests.Load() != 0 {
				t.Fatal("changed file ownership accepted")
			}
		})
	}
}

func TestBaselineControllerReferenceWireContract(t *testing.T) {
	id := controllerJournalIdentity{OwnershipSHA256: strings.Repeat("a", 64), State: controllerFileIdentity{1, 2}, Journal: controllerFileIdentity{3, 4}, AdmissionDirectory: controllerFileIdentity{5, 6}, Claim: controllerFileIdentity{7, 8}}
	ref := controllerEventRef(id, Event{Sequence: 2, Kind: "result", Operation: "create", ID: 7})
	if ref.Sequence != 2 || ref.EventSHA256 != "64f21b46249bcc991e04c1ab5875cfcc29aaa856ea5f9b4e458b96a5988f1007" {
		t.Fatal("assigned controller domain/field-order contract changed")
	}
}
