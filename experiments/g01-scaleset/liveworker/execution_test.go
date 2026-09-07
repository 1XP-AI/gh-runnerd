package liveworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureRef(sequence int) RecordRef {
	return RecordRef{sequence, strings.Repeat("a", 64)}
}

func fixturePairInput(a Approval) PairInput {
	return PairInput{
		Controller:    JournalIdentity{strings.Repeat("b", 64), FileIdentity{1, 2}, FileIdentity{1, 3}, FileIdentity{1, 4}, FileIdentity{1, 5}},
		Source:        PairSource{"fixture-org", "fixture-repo", 42, 91, ".github/workflows/canary.yml", a.WorkflowSHA, 1},
		RunnerGroupID: 3, ScaleSetID: 7, ScaleSetName: "g01-" + a.OwnerNonce,
		SetCreation: fixtureRef(1), OwnerNonce: a.OwnerNonce, ControllerName: a.Controller, HarnessSHA: a.HarnessSHA,
	}
}

func fixturePairHash(binding PairBinding) string {
	data, _ := json.Marshal(binding)
	sum := sha256.Sum256(append([]byte("gh-runnerd/g01-pair/binding/v1\x00"), data...))
	return hex.EncodeToString(sum[:])
}

func fixtureHandoff(pair string, a Approval) HandoffReceipt {
	return HandoffReceipt{pair, fixtureRef(3), fixtureRef(4), fixtureRef(5), fixtureRef(6), 101, SDKRunnerIdentity{9, a.name(), 7}}
}

type pairedFakeRuntime struct {
	*fakeRuntime
	reads int
}

func (f *pairedFakeRuntime) InspectExact(ctx context.Context, target string) (DockerInspectObservation, error) {
	f.reads++
	falseValue, exit := false, int64(0)
	result := DockerInspectObservation{TargetID: target, Method: http.MethodGet, Path: "/v1.45/containers/" + target + "/json", HTTPStatus: http.StatusOK, Outcome: DockerInspectPresent, Container: f.container}
	if f.deletes.Load() != 0 {
		result.HTTPStatus, result.Outcome, result.Container = http.StatusNotFound, DockerInspectNotFoundReported, nil
	} else {
		result.State = &DockerStateFacts{ContainerStatus(f.container.State.Status), &f.container.State.Running, &falseValue, &falseValue, &falseValue, &exit}
	}
	return result, nil
}

func pairedFixture(t *testing.T) (*Driver, *FileJournal, *pairedFakeRuntime, PairInput) {
	t.Helper()
	a := approval()
	j, err := openTestJournal(t, privateDir(t), a)
	if err != nil {
		t.Fatal("actual private worker journal fixture failed")
	}
	t.Cleanup(func() { _ = j.Close() })
	runtime := &pairedFakeRuntime{fakeRuntime: &fakeRuntime{image: ImageProfile{Env: []string{"PATH=/usr/bin"}}}}
	return &Driver{a, j, runtime}, j, runtime, fixturePairInput(a)
}

func TestPairedActualFileLifecycleAndHistoricalReceipts(t *testing.T) {
	d, j, runtime, in := pairedFixture(t)
	entered, checks := false, 0
	var binding PairBinding
	check := func(c ControllerCheck) error {
		checks++
		if c.Stage == CheckBind && c.PairSHA256 != fixturePairHash(binding) {
			t.Error("controller bind check did not receive full computed binding hash")
		}
		return nil // Synthetic controller checks prove worker behavior only.
	}
	err := d.WithPairedExecution(context.Background(), in, check, func(w *PairedWorker) error {
		entered = true
		binding = w.Binding()
		if binding.Version != 1 || binding.Input != in || binding.Worker.OwnershipSHA256 != ownershipDigest(d.Approval) {
			t.Fatal("scope did not capture exact stable binding")
		}
		if release, err := j.authorize(d.Approval); err == nil {
			release()
			t.Fatal("paired callback did not hold the real execution lease")
		}
		pair, err := w.Bind(fixtureRef(2))
		if err != nil || pair.WorkerBound.Sequence != 1 {
			t.Fatal("worker binding was not the first assigned durable event")
		}
		h := fixtureHandoff(pair.PairSHA256, d.Approval)
		created, err := w.Create(h, syntheticJIT)
		if err != nil || created.CreateIntent.Sequence != 2 || created.CreateResult.Sequence != 3 {
			t.Fatal("durable create boundaries missing")
		}
		again, err := w.Create(h, syntheticJIT)
		if err != nil || again != created || runtime.creates.Load() != 1 {
			t.Fatal("same-scope historical create reissued an effect")
		}
		started, err := w.Start(created, fixtureRef(7))
		if err != nil {
			t.Fatal("paired start failed")
		}
		againStart, err := w.Start(created, fixtureRef(7))
		if err != nil || againStart != started || runtime.starts.Load() != 1 {
			t.Fatal("same-scope historical start reissued an effect")
		}
		runtime.container.State.Status, runtime.container.State.Running = "exited", false
		observed, err := w.Observe()
		if err != nil || observed.Outcome != LocalProfilePresent || observed.State == nil {
			t.Fatal("paired local facts missing")
		}
		decision := TerminalDecisionRef{pair.PairSHA256, created.ContainerID, created.CreateResult, fixtureRef(8)}
		deleted, err := w.DeleteTerminal(decision)
		if err != nil || deleted.AbsenceResult == nil || runtime.deletes.Load() != 1 {
			t.Fatal("terminal deletion and separate absence result missing")
		}
		cached, err := w.DeleteTerminal(decision)
		if err != nil || cached.Result != deleted.Result || runtime.deletes.Load() != 1 {
			t.Fatal("historical deletion reissued an effect")
		}
		return nil
	})
	if err != nil || !entered || checks == 0 {
		t.Fatalf("usable actual-file paired scope unavailable: entered=%t error=%v", entered, err)
	}
	data, err := os.ReadFile(filepath.Join(j.directory, "journal.jsonl"))
	if err != nil || strings.Contains(string(data), syntheticJIT) || strings.Contains(string(data), "ACTIONS_RUNNER_INPUT_JITCONFIG") {
		t.Fatal("paired journal leaked JIT")
	}
}

func TestPairedActualFileBoundStateCannotBeAdoptedAfterReturn(t *testing.T) {
	d, j, runtime, in := pairedFixture(t)
	var retained PairedWorker
	var handoff HandoffReceipt
	if err := d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
		retained = *w
		pair, err := w.Bind(fixtureRef(2))
		handoff = fixtureHandoff(pair.PairSHA256, d.Approval)
		return err
	}); err != nil {
		t.Fatalf("real worker binding unavailable: %v", err)
	}
	if _, err := retained.Create(handoff, syntheticJIT); err == nil || runtime.creates.Load() != 0 {
		t.Fatal("copied receiver outlived its released lease")
	}
	if d.Run(context.Background(), "create", syntheticJIT) == nil || runtime.creates.Load() != 0 {
		t.Fatal("standalone create bypassed the recorded pair")
	}
	if err := j.Close(); err != nil {
		t.Fatal("fixture close failed")
	}
	reopened, err := openTestJournal(t, j.directory, d.Approval)
	if err != nil {
		t.Fatal("bound journal could not be reopened for reporting")
	}
	defer reopened.Close()
	d.Journal = reopened
	err = d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
		if _, err := w.Create(handoff, syntheticJIT); err == nil {
			t.Fatal("reopened pair restored create")
		}
		return nil
	})
	if err != nil || runtime.creates.Load() != 0 {
		t.Fatal("reopened scope did not preserve the partial pair reservation")
	}
}
