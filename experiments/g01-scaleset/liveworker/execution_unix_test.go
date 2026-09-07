package liveworker

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureHasIntent(events []Event, operation string) bool {
	if len(events) == 0 {
		return false
	}
	e := events[len(events)-1]
	if e.Kind == "intent" && e.Operation == operation {
		return true
	}
	if e.Kind != "paired" || e.Paired == nil {
		return false
	}
	switch operation {
	case "create":
		return e.Paired.Create != nil && e.Paired.Create.Kind == "intent"
	case "start":
		return e.Paired.Start != nil && e.Paired.Start.Kind == "intent"
	}
	return false
}

func pairedUnixFixture(t *testing.T) (*dockerFixture, *FileJournal, PairInput) {
	t.Helper()
	f := unixFixture(t)
	j, err := openTestJournal(t, privateDir(t), f.approval)
	if err != nil {
		t.Fatal("actual Unix/journal fixture failed")
	}
	t.Cleanup(func() { _ = j.Close() })
	f.driver.Journal = j
	return f, j, fixturePairInput(f.approval)
}

func pairedTerminalBody(t *testing.T, f *dockerFixture) []byte {
	t.Helper()
	data, err := json.Marshal(f.runtime.container)
	if err != nil {
		t.Fatal("fixture marshal failed")
	}
	return pairedChangeState(t, data, func(s map[string]any) {
		s["Status"] = "exited"
		s["Running"] = false
		s["Paused"] = false
		s["Restarting"] = false
		s["Dead"] = false
		s["ExitCode"] = 0
	})
}

func serveTerminalThenAbsence(f *dockerFixture, data []byte) {
	f.inspectResponse.Store(&inspectFixtureResponse{serve: func(w http.ResponseWriter, r *http.Request) {
		if f.runtime.deletes.Load() > 0 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"synthetic not found"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}})
}

func TestPairedActualUnixLifecycleUsesDurableReceiptsAndSeparateAbsence(t *testing.T) {
	f, j, in := pairedUnixFixture(t)
	err := f.driver.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
		pair, created := bindAndCreate(t, w, f.approval)
		if _, err := w.Start(created, fixtureRef(7)); err != nil {
			t.Fatal("actual Unix start failed")
		}
		serveTerminalThenAbsence(f, pairedTerminalBody(t, f))
		local, err := w.Observe()
		if err != nil || !terminalFacts(local) {
			t.Fatal("typed terminal profile unavailable")
		}
		*local.State.ExitCode = 12
		*local.State.Running = true
		if !terminalFacts(*j.pairState().lastLocal) {
			t.Fatal("returned local facts alias journal")
		}
		decision := TerminalDecisionRef{pair.PairSHA256, created.ContainerID, created.CreateResult, fixtureRef(8)}
		deleted, err := w.DeleteTerminal(decision)
		if err != nil || deleted.AbsenceResult == nil || deleted.AbsenceResult.Sequence != deleted.Result.Sequence+2 {
			t.Fatal("DELETE and separate exact 404 receipts missing")
		}
		originalAbsence := *deleted.AbsenceResult
		later, err := w.Observe()
		if err != nil || later.Outcome != LocalNotFoundReported || later.Result == originalAbsence {
			t.Fatal("new observation did not produce its own receipt")
		}
		*deleted.AbsenceResult = fixtureRef(999)
		before := f.requests.Load()
		again, err := w.DeleteTerminal(decision)
		if err != nil || again.AbsenceResult == nil || *again.AbsenceResult != originalAbsence || f.requests.Load() != before {
			t.Fatal("deletion copy/cache boundary failed")
		}
		return nil
	})
	if err != nil || f.runtime.creates.Load() != 1 || f.runtime.starts.Load() != 1 || f.runtime.deletes.Load() != 1 {
		t.Fatal("actual Unix lifecycle failed")
	}
	data, err := os.ReadFile(filepath.Join(j.directory, "journal.jsonl"))
	if err != nil || strings.Contains(string(data), syntheticJIT) || strings.Contains(string(data), "JITCONFIG") || strings.Contains(string(data), "synthetic not found") {
		t.Fatal("journal retained raw runtime data")
	}
}

func TestPairedActualUnixTerminalRefusalsNeverDelete(t *testing.T) {
	for _, fault := range []string{"created", "running", "nonzero", "negative", "missing-exit", "null-exit", "running-true", "paused-null", "restarting-true", "dead-true", "profile", "wrong-id", "not-found", "controller", "delete-race"} {
		t.Run(fault, func(t *testing.T) {
			f, j, in := pairedUnixFixture(t)
			check := func(c ControllerCheck) error {
				if fault == "controller" && c.Stage == CheckDelete {
					return ErrState
				}
				return nil
			}
			err := f.driver.WithPairedExecution(context.Background(), in, check, func(w *PairedWorker) error {
				pair, created := bindAndCreate(t, w, f.approval)
				body := pairedTerminalBody(t, f)
				body = pairedChangeState(t, body, func(s map[string]any) {
					switch fault {
					case "created":
						s["Status"] = "created"
					case "running":
						s["Status"] = "running"
					case "nonzero":
						s["ExitCode"] = 23
					case "negative":
						s["ExitCode"] = -1
					case "missing-exit":
						delete(s, "ExitCode")
					case "null-exit":
						s["ExitCode"] = nil
					case "running-true":
						s["Running"] = true
					case "paused-null":
						s["Paused"] = nil
					case "restarting-true":
						s["Restarting"] = true
					case "dead-true":
						s["Dead"] = true
					}
				})
				if fault == "profile" || fault == "wrong-id" {
					var doc map[string]any
					_ = json.Unmarshal(body, &doc)
					if fault == "profile" {
						doc["Name"] = "/unowned"
					} else {
						doc["Id"] = strings.Repeat("d", 64)
					}
					body, _ = json.Marshal(doc)
				}
				f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusOK, body: body})
				if fault == "not-found" {
					f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusNotFound, body: []byte(`{"message":"synthetic-private-response"}`)})
				}
				if fault == "delete-race" {
					f.fault = fault
				}
				before := f.requests.Load()
				decision := TerminalDecisionRef{pair.PairSHA256, created.ContainerID, created.CreateResult, fixtureRef(8)}
				if _, err := w.DeleteTerminal(decision); err == nil {
					t.Fatal("invalid terminal evidence allowed deletion")
				}
				expected := int64(0)
				if fault == "delete-race" {
					expected = 1
				}
				if f.runtime.deletes.Load() != expected {
					t.Fatal("unexpected deletion request")
				}
				if fault == "controller" && f.requests.Load() != before {
					t.Fatal("controller rejection reached runtime")
				}
				if fault == "delete-race" {
					before = f.requests.Load()
					if _, err := w.DeleteTerminal(decision); err == nil || f.requests.Load() != before || !j.pairState().uncertain {
						t.Fatal("failed delete repeated")
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal("refusal fixture scope failed")
			}
		})
	}
}

func TestPairedActualUnixKnownResponsesSurviveEOFCancellation(t *testing.T) {
	for _, operation := range []string{"create", "start", "delete"} {
		t.Run(operation, func(t *testing.T) {
			f, j, in := pairedUnixFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var known bool
			err := f.driver.WithPairedExecution(ctx, in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
				pair, err := w.Bind(fixtureRef(2))
				if err != nil {
					return err
				}
				docker := f.driver.Runtime.(*Docker)
				install := func(path string) {
					docker.client.Transport = cancelAtEOFTransport{docker.client.Transport, path, cancel}
				}
				if operation == "create" {
					install("/v1.45/containers/create")
				}
				created, err := w.Create(fixtureHandoff(pair.PairSHA256, f.approval), syntheticJIT)
				if err != nil {
					t.Fatal("known create response discarded")
				}
				if operation == "create" {
					known = validRef(created.CreateResult)
					return nil
				}
				if operation == "start" {
					install("/v1.45/containers/" + created.ContainerID + "/start")
					ref, err := w.Start(created, fixtureRef(7))
					known = err == nil && validRef(ref)
					return nil
				}
				serveTerminalThenAbsence(f, pairedTerminalBody(t, f))
				install("/v1.45/containers/" + created.ContainerID)
				result, err := w.DeleteTerminal(TerminalDecisionRef{pair.PairSHA256, created.ContainerID, created.CreateResult, fixtureRef(8)})
				known = err != nil && validRef(result.Result) && result.AbsenceResult == nil
				return nil
			})
			if !known || err == nil || ctx.Err() == nil || j.pairState().uncertain {
				t.Fatal("known EOF response lost or cancellation not fenced")
			}
		})
	}
}

func pairedChangeState(t *testing.T, data []byte, change func(map[string]any)) []byte {
	t.Helper()
	var document map[string]any
	if json.Unmarshal(data, &document) != nil {
		t.Fatal("invalid fixture document")
	}
	state, ok := document["State"].(map[string]any)
	if !ok {
		t.Fatal("fixture state absent")
	}
	change(state)
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal("fixture state marshal failed")
	}
	return data
}

func TestPairedActualUnixCleanupOnlyRenewalCanDeleteKnownTerminal(t *testing.T) {
	f, j, in := pairedUnixFixture(t)
	var binding PairBinding
	var created ContainerReceipt
	if err := f.driver.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
		binding = w.Binding()
		_, created = bindAndCreate(t, w, f.approval)
		_, err := w.Start(created, fixtureRef(7))
		return err
	}); err != nil {
		t.Fatal("completed history fixture failed")
	}
	if j.Close() != nil {
		t.Fatal("close failed")
	}
	renewed := f.approval
	renewed.ExpiresAt = renewed.ExpiresAt.Add(time.Minute)
	renewed.Phases = []string{"cleanup"}
	reopened, err := openTestJournal(t, j.directory, renewed)
	if err != nil {
		t.Fatal("cleanup-only paired renewal failed")
	}
	defer reopened.Close()
	f.driver.Journal = reopened
	f.driver.Approval = renewed
	renewedRuntime, err := NewDocker(renewed)
	if err != nil {
		t.Fatal("renewed runtime fixture failed")
	}
	f.driver.Runtime = renewedRuntime
	serveTerminalThenAbsence(f, pairedTerminalBody(t, f))
	if err = f.driver.WithPairedExecution(context.Background(), in, func(c ControllerCheck) error {
		if c.PairSHA256 != fixturePairHash(binding) {
			return ErrState
		}
		if c.Stage == CheckDelete && (c.ControllerRecord != fixtureRef(8) || c.WorkerRecord != created.CreateResult) {
			return ErrState
		}
		return nil
	}, func(w *PairedWorker) error {
		if w.Binding() != binding {
			t.Fatal("recovery changed stable binding")
		}
		before := f.requests.Load()
		if _, err := w.Observe(); err == nil || f.requests.Load() != before {
			t.Fatal("cleanup authority granted separate inspect phase")
		}
		result, err := w.DeleteTerminal(TerminalDecisionRef{created.PairSHA256, created.ContainerID, created.CreateResult, fixtureRef(8)})
		if err != nil || result.AbsenceResult == nil || result.AbsenceResult.Sequence != result.Result.Sequence+2 {
			t.Fatal("eligible cleanup-only recovery did not complete separate receipts")
		}
		return nil
	}); err != nil || f.runtime.deletes.Load() != 1 || f.runtime.creates.Load() != 1 || f.runtime.starts.Load() != 1 {
		t.Fatal("eligible recovered cleanup failed")
	}
}
