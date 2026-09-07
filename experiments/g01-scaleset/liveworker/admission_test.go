package liveworker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func workerAdmissionState(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal("private fixture directory")
	}
	return path
}

func TestWorkerAdmissionCapsIndependentDirectories(t *testing.T) {
	parent := privateDir(t)
	capRoot := workerAdmissionState(t, parent, "admission")
	states := []string{workerAdmissionState(t, parent, "first"), workerAdmissionState(t, parent, "second")}
	runtimes := []*fakeRuntime{{}, {}}
	a := approval()
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := range states {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			candidate := a
			if i == 1 {
				candidate.OwnerNonce = strings.Repeat("5", 32)
			}
			j, err := openJournalAtAdmission(states[i], candidate, capRoot, func(f *os.File) error { return f.Sync() })
			if err != nil {
				return
			}
			defer j.Close()
			d := Driver{candidate, j, runtimes[i]}
			if err := d.Run(context.Background(), "create", syntheticJIT); err != nil {
				t.Errorf("admitted positive control: %v", err)
			}
		}(i)
	}
	close(gate)
	wg.Wait()
	if count := runtimes[0].creates.Load() + runtimes[1].creates.Load(); count != 1 {
		t.Fatalf("independent directories admitted %d creates, want one", count)
	}
}

func TestWorkerAdmissionRetainsSlotAfterOutcomeAndClose(t *testing.T) {
	for _, outcome := range []string{"created", "deleted", "unknown"} {
		t.Run(outcome, func(t *testing.T) {
			parent := privateDir(t)
			capRoot := workerAdmissionState(t, parent, "admission")
			first := workerAdmissionState(t, parent, "first")
			a := approval()
			j, err := openJournalAtAdmission(first, a, capRoot, func(f *os.File) error { return f.Sync() })
			if err != nil {
				t.Fatal("first admission")
			}
			runtime := &fakeRuntime{}
			if outcome == "unknown" {
				runtime.createErr = errors.New("synthetic lost create response")
			}
			d := Driver{a, j, runtime}
			err = d.Run(context.Background(), "create", syntheticJIT)
			if (outcome == "unknown") != (err != nil) {
				t.Fatal("initial effect fixture")
			}
			if outcome == "deleted" && d.Run(context.Background(), "cleanup", "") != nil {
				t.Fatal("empty owned cleanup control")
			}
			if j.Close() != nil {
				t.Fatal("fixture close")
			}
			second := workerAdmissionState(t, parent, "second")
			other, err := openJournalAtAdmission(second, a, capRoot, func(f *os.File) error { return f.Sync() })
			if err == nil {
				other.Close()
				t.Error("close or terminal/unknown outcome released the experiment slot")
			}
			reopened, err := openJournalAtAdmission(first, a, capRoot, func(f *os.File) error { return f.Sync() })
			if err != nil {
				t.Fatal("original owned journal could not reopen")
			}
			defer reopened.Close()
			d.Journal = reopened
			if d.Run(context.Background(), "create", syntheticJIT) == nil || runtime.creates.Load() != 1 {
				t.Fatal("restart retried original worker creation")
			}
		})
	}
}
