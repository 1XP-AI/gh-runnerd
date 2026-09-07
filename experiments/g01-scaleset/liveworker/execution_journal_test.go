package liveworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPairedAssignedReferencesFollowActualFsyncAndDeepCopies(t *testing.T) {
	d, j, _, in := pairedFixture(t)
	synced := 0
	j.recordSync = func(f *os.File) error {
		if err := f.Sync(); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(j.directory, "journal.jsonl"))
		if err != nil {
			return err
		}
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		var e Event
		if json.Unmarshal([]byte(lines[len(lines)-1]), &e) != nil || e.Sequence != len(j.events)+1 {
			return errors.New("assignment missing at real fsync")
		}
		synced++
		return nil
	}
	err := d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
		_, created := bindAndCreate(t, w, d.Approval)
		if synced != 3 {
			t.Fatal("receipt returned before expected sync boundaries")
		}
		identity, _ := json.Marshal(w.Binding().Worker)
		for _, ref := range []RecordRef{j.pairState().pair.WorkerBound, created.CreateIntent, created.CreateResult} {
			event := j.Events()[ref.Sequence-1]
			data, _ := json.Marshal(event)
			hash := sha256.Sum256(append(append(append([]byte("gh-runnerd/g01-pair/worker-event/v1\x00"), identity...), 0), data...))
			if ref.EventSHA256 != hex.EncodeToString(hash[:]) {
				t.Fatal("assigned receipt has wrong identity/event domain")
			}
		}
		events := j.Events()
		events[0].Paired.Bound.Binding.Input.Source.RepositoryID = 999
		events[1].Paired.Create.Handoff.Runner.Name = "mutated"
		if w.Binding().Input.Source.RepositoryID != 42 || j.Events()[1].Paired.Create.Handoff.Runner.Name != d.Approval.name() {
			t.Fatal("event snapshots alias authority")
		}
		return nil
	})
	if err != nil {
		t.Fatal("durable reference fixture failed")
	}
}

func TestPairedAppendAndReopenRejectInvalidShapeOrPredecessors(t *testing.T) {
	for _, fault := range []string{"duplicate-bind", "two-branches", "mixed-legacy", "wrong-predecessor", "start-without-read", "delete-without-read", "wrong-local-target", "legacy-effect", "oversize-binding", "oversize-event"} {
		t.Run(fault, func(t *testing.T) {
			d, j, _, in := pairedFixture(t)
			var created ContainerReceipt
			if err := d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error { _, created = bindAndCreate(t, w, d.Approval); return nil }); err != nil {
				t.Fatal("fixture setup failed")
			}
			e := cloneEvent(j.Events()[0])
			switch fault {
			case "two-branches":
				e.Paired.Local = &pairLocalEvent{}
			case "mixed-legacy":
				e.Status = "hidden"
			case "wrong-predecessor":
				e = Event{Kind: "paired", Paired: &pairedEvent{Create: &pairCreateEvent{Kind: "result", Intent: fixtureRef(2), ContainerID: created.ContainerID}}}
			case "start-without-read":
				e = Event{Kind: "paired", Paired: &pairedEvent{Start: &pairStartEvent{Kind: "intent", CreateResult: created.CreateResult, ControllerResult: fixtureRef(7)}}}
			case "delete-without-read":
				e = Event{Kind: "paired", Paired: &pairedEvent{Delete: &pairDeleteEvent{Kind: "intent", Decision: &TerminalDecisionRef{created.PairSHA256, created.ContainerID, created.CreateResult, fixtureRef(8)}}}}
			case "wrong-local-target":
				e = Event{Kind: "paired", Paired: &pairedEvent{Local: &pairLocalEvent{Kind: "intent", ContainerID: strings.Repeat("d", 64)}}}
			case "legacy-effect":
				e = Event{Kind: "intent", Operation: "delete"}
			case "oversize-binding":
				e.Paired.Bound.Binding.Input.Source.WorkflowPath = ".github/workflows/" + strings.Repeat("a", maxPairBinding) + ".yml"
			case "oversize-event":
				e = Event{Kind: "paired", Paired: &pairedEvent{Local: &pairLocalEvent{Kind: "intent", ContainerID: created.ContainerID, Path: strings.Repeat("a", maxPairRecord)}}}
			}
			before := j.bytes
			if j.Append(e) == nil || j.bytes != before {
				t.Fatal("invalid append changed durable state")
			}
			e.Sequence = len(j.Events()) + 1
			if j.Close() != nil {
				t.Fatal("close failed")
			}
			f, err := os.OpenFile(filepath.Join(j.directory, "journal.jsonl"), os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				t.Fatal("fixture append failed")
			}
			data, _ := json.Marshal(e)
			_, err = f.Write(append(data, '\n'))
			_ = f.Close()
			if err != nil {
				t.Fatal("fixture write failed")
			}
			if reopened, err := openTestJournal(t, j.directory, d.Approval); err == nil {
				_ = reopened.Close()
				t.Fatal("invalid persisted pair replayed")
			}
		})
	}
}

func padPairedJournal(t *testing.T, j *FileJournal, remaining int64) {
	t.Helper()
	e := Event{Sequence: len(j.Events()) + 1, Kind: "observation", Operation: "inspect"}
	encoded, _ := json.Marshal(e)
	// Status introduces its own field overhead; compute it before exact padding.
	e.Status = "x"
	encoded, _ = json.Marshal(e)
	padding := maxJournal - j.bytes - remaining - int64(len(encoded)+1)
	if padding < 0 {
		t.Fatal("insufficient fixture padding")
	}
	e.Status += strings.Repeat("x", int(padding))
	if j.Append(e) != nil || j.bytes != maxJournal-remaining {
		t.Fatal("bounded journal fixture failed")
	}
}

func TestPairedCapacityRefusesBeforeFirstRuntimeRequest(t *testing.T) {
	for _, operation := range []string{"create", "start", "observe", "delete"} {
		t.Run(operation, func(t *testing.T) {
			d, j, runtime, in := pairedFixture(t)
			err := d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
				pair, err := w.Bind(fixtureRef(2))
				if err != nil {
					return err
				}
				var created ContainerReceipt
				if operation != "create" {
					created, err = w.Create(fixtureHandoff(pair.PairSHA256, d.Approval), syntheticJIT)
					if err != nil {
						return err
					}
				}
				room := int64(pairCallRoom)
				if operation == "start" {
					room *= 2
				}
				if operation == "delete" {
					room = pairDeleteRoom
					runtime.container.State.Status = "exited"
				}
				padPairedJournal(t, j, room-1)
				before, reads, events := runtime.preflights.Load(), runtime.reads.Load(), len(j.Events())
				switch operation {
				case "create":
					_, err = w.Create(fixtureHandoff(pair.PairSHA256, d.Approval), syntheticJIT)
				case "start":
					_, err = w.Start(created, fixtureRef(7))
				case "observe":
					_, err = w.Observe()
				case "delete":
					_, err = w.DeleteTerminal(TerminalDecisionRef{pair.PairSHA256, created.ContainerID, created.CreateResult, fixtureRef(8)})
				}
				if err == nil || runtime.preflights.Load() != before || runtime.reads.Load() != reads || len(j.Events()) != events || runtime.starts.Load() != 0 || runtime.deletes.Load() != 0 {
					t.Fatal("capacity check followed a request or write")
				}
				return nil
			})
			if err != nil {
				t.Fatal("capacity fixture failed")
			}
		})
	}
}

func TestPairedGlobalAndPhysicalRecordLimits(t *testing.T) {
	t.Run("global", func(t *testing.T) {
		d, j, _, in := pairedFixture(t)
		_ = d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error { _, err := w.Bind(fixtureRef(2)); return err })
		padPairedJournal(t, j, 1)
		before := j.bytes
		if j.Append(Event{Kind: "observation", Operation: "inspect"}) == nil || j.bytes != before {
			t.Fatal("cumulative limit exceeded")
		}
	})
	for _, fault := range []string{"padded-pair", "duplicate-key", "unknown-field", "torn-tail"} {
		t.Run(fault, func(t *testing.T) {
			d, j, _, in := pairedFixture(t)
			_ = d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error { _, err := w.Bind(fixtureRef(2)); return err })
			_ = j.Close()
			path := filepath.Join(j.directory, "journal.jsonl")
			data, _ := os.ReadFile(path)
			lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			switch fault {
			case "padded-pair":
				lines[1] += strings.Repeat(" ", maxPairRecord)
			case "duplicate-key":
				lines[1] = strings.Replace(lines[1], `"kind":"paired"`, `"kind":"paired","Kind":"paired"`, 1)
			case "unknown-field":
				lines[1] = strings.TrimSuffix(lines[1], "}") + `,"foreign":true}`
			case "torn-tail":
				lines[1] = strings.TrimSuffix(lines[1], "}")
			}
			if os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600) != nil {
				t.Fatal("fixture rewrite failed")
			}
			if reopened, err := openTestJournal(t, j.directory, d.Approval); err == nil {
				_ = reopened.Close()
				t.Fatal("invalid physical record accepted")
			}
		})
	}
}

func TestPairedFailedFsyncPoisonsScopeAndReopenNeverRestoresEffects(t *testing.T) {
	for _, failure := range []string{"intent", "result"} {
		t.Run(failure, func(t *testing.T) {
			d, j, runtime, in := pairedFixture(t)
			var h HandoffReceipt
			err := d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
				pair, err := w.Bind(fixtureRef(2))
				if err != nil {
					return err
				}
				h = fixtureHandoff(pair.PairSHA256, d.Approval)
				writes := 0
				j.recordSync = func(f *os.File) error {
					writes++
					if writes == 1 && failure == "intent" || writes == 2 && failure == "result" {
						return errors.New("synthetic fsync failure")
					}
					return f.Sync()
				}
				if result, err := w.Create(h, syntheticJIT); err == nil || result != (ContainerReceipt{}) {
					t.Fatal("failed sync returned assigned receipt")
				}
				before := runtime.preflights.Load()
				if _, err := w.Create(h, syntheticJIT); err == nil || runtime.preflights.Load() != before || !j.poisoned {
					t.Fatal("failed sync scope retained authority")
				}
				return nil
			})
			expected := int64(0)
			if failure == "result" {
				expected = 1
			}
			if err == nil || runtime.creates.Load() != expected {
				t.Fatal("fsync failure boundary failed")
			}
			_ = j.Close()
			// A failed sync may leave complete bytes. Their presence never grants a new
			// scope create/start authority, even if the completed result can be replayed.
			reopened, err := openTestJournal(t, j.directory, d.Approval)
			if err != nil {
				t.Fatal("complete fixture bytes should remain reportable")
			}
			defer reopened.Close()
			d.Journal = reopened
			if err = d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
				before := runtime.preflights.Load()
				if _, err := w.Create(h, syntheticJIT); err == nil {
					t.Fatal("reopen restored creation")
				}
				c := ContainerReceipt{}
				if reopened.pairState().created != nil {
					c = *reopened.pairState().created
				}
				if _, err := w.Start(c, fixtureRef(7)); err == nil || runtime.preflights.Load() != before {
					t.Fatal("reopen restored start")
				}
				return nil
			}); err != nil {
				t.Fatal("reopened report scope failed")
			}
		})
	}
}

func TestPairedRecoveryRenewalAndInterruptedStartRemainReportOnly(t *testing.T) {
	d, j, runtime, in := pairedFixture(t)
	var binding PairBinding
	var created ContainerReceipt
	if err := d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
		binding = w.Binding()
		_, created = bindAndCreate(t, w, d.Approval)
		if _, err := w.Observe(); err != nil {
			return err
		}
		return j.Append(Event{Kind: "paired", Paired: &pairedEvent{Start: &pairStartEvent{Kind: "intent", CreateResult: created.CreateResult, ControllerResult: fixtureRef(7)}}})
	}); err != nil {
		t.Fatal("pending actual-file fixture failed")
	}
	_ = j.Close()
	renewed := d.Approval
	renewed.ExpiresAt = renewed.ExpiresAt.Add(time.Minute)
	renewed.Phases = []string{"inspect", "cleanup"}
	reopened, err := openTestJournal(t, j.directory, renewed)
	if err != nil {
		t.Fatal("pre-claim renewal of paired file failed")
	}
	defer reopened.Close()
	d.Approval = renewed
	d.Journal = reopened
	events := reopened.Events()
	last := events[len(events)-1]
	if last.Kind != "authority" {
		t.Fatal("renewal receipt missing")
	}
	last.Authority.Phases[0] = "create"
	if reopened.Events()[len(events)-1].Authority.Phases[0] != "inspect" {
		t.Fatal("authority snapshot aliases private state")
	}
	if err = d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
		if w.Binding() != binding {
			t.Fatal("authority renewal changed stable pair")
		}
		if _, err := w.Observe(); err != nil {
			return err
		}
		if !reopened.pairState().uncertain {
			t.Fatal("fresh observation cleared interrupted start")
		}
		before := runtime.preflights.Load()
		if _, err := w.Start(created, fixtureRef(7)); err == nil {
			t.Fatal("recovery restored start")
		}
		runtime.container.State.Status = "exited"
		if _, err := w.DeleteTerminal(TerminalDecisionRef{created.PairSHA256, created.ContainerID, created.CreateResult, fixtureRef(8)}); err == nil || runtime.preflights.Load() != before {
			t.Fatal("interrupted start restored delete")
		}
		return nil
	}); err != nil {
		t.Fatal("recovery reporting failed")
	}
	if runtime.starts.Load() != 0 || runtime.deletes.Load() != 0 {
		t.Fatal("report-only recovery caused effect")
	}
}

func TestPairedLateCapacityLossRefusesBeforeEffect(t *testing.T) {
	for _, boundary := range []string{"before-intent", "after-intent"} {
		t.Run(boundary, func(t *testing.T) {
			d, j, runtime, in := pairedFixture(t)
			calls := 0
			check := func(c ControllerCheck) error {
				if c.Stage == CheckCreate {
					calls++
					target := 3
					if boundary == "after-intent" {
						target = 4
					}
					if calls == target {
						remaining := int64(pairCallRoom - 1)
						if boundary == "after-intent" {
							remaining = maxPairRecord - 1
						}
						padPairedJournal(t, j, remaining)
					}
				}
				return nil
			}
			if err := d.WithPairedExecution(context.Background(), in, check, func(w *PairedWorker) error {
				pair, err := w.Bind(fixtureRef(2))
				if err != nil {
					return err
				}
				if _, err := w.Create(fixtureHandoff(pair.PairSHA256, d.Approval), syntheticJIT); err == nil || runtime.creates.Load() != 0 {
					t.Fatal("late capacity loss reached effect")
				}
				return nil
			}); err != nil {
				t.Fatal("late capacity fixture failed")
			}
		})
	}
}
