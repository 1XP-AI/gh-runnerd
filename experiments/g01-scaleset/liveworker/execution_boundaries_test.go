package liveworker

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func bindAndCreate(t *testing.T, w *PairedWorker, a Approval) (HandoffReceipt, ContainerReceipt) {
	t.Helper()
	pair, err := w.Bind(fixtureRef(2))
	if err != nil {
		t.Fatal("fixture bind failed")
	}
	h := fixtureHandoff(pair.PairSHA256, a)
	created, err := w.Create(h, syntheticJIT)
	if err != nil {
		t.Fatal("fixture paired create failed")
	}
	return h, created
}

func TestPairedCopiedHandoffAndHistoricalConflictsDoNotExtendAuthority(t *testing.T) {
	d, j, runtime, in := pairedFixture(t)
	checks := 0
	err := d.WithPairedExecution(context.Background(), in, func(c ControllerCheck) error {
		if c.Stage == CheckCreate {
			checks++
			if c.Handoff == nil || c.Handoff.RequestID != 101 {
				return ErrApproval
			}
			c.Handoff.RequestID = 999
			c.Handoff.Runner.Name = "synthetic-private-mutation"
		} else if c.Handoff != nil {
			t.Error("handoff supplied at another stage")
		}
		return nil
	}, func(w *PairedWorker) error {
		h, created := bindAndCreate(t, w, d.Approval)
		before, preflight := len(j.Events()), runtime.preflights.Load()
		otherJIT := base64.StdEncoding.EncodeToString([]byte("different synthetic JIT value"))
		if _, err := w.Create(h, otherJIT); err == nil {
			t.Fatal("changed JIT reused historical receipt")
		}
		bad := h
		bad.RequestID++
		if _, err := w.Create(bad, syntheticJIT); err == nil {
			t.Fatal("changed handoff reused historical receipt")
		}
		if len(j.Events()) != before || runtime.creates.Load() != 1 || runtime.preflights.Load() != preflight {
			t.Fatal("conflicting historical request caused append/preflight/effect")
		}
		events := j.Events()
		events[1].Paired.Create.Handoff.RequestID = 800
		events[1].Paired.Create.EnvDigest = strings.Repeat("0", 64)
		if _, err := w.Start(created, fixtureRef(7)); err != nil {
			t.Fatal("returned event mutation changed actual replay")
		}
		if j.Events()[1].Paired.Create.Handoff.RequestID != 101 {
			t.Fatal("controller-check copy mutated durable handoff")
		}
		return nil
	})
	if err != nil || checks < 2 {
		t.Fatal("immutable handoff lifecycle failed")
	}
}

func TestPairedWorkerPhaseGuardsPrecedeRuntime(t *testing.T) {
	for _, missing := range []string{"create", "start", "inspect", "cleanup"} {
		t.Run(missing, func(t *testing.T) {
			a := approval()
			a.Phases = slices.DeleteFunc(a.Phases, func(p string) bool { return p == missing })
			j, err := openTestJournal(t, privateDir(t), a)
			if err != nil {
				t.Fatal("phase fixture failed")
			}
			defer j.Close()
			runtime := &pairedFakeRuntime{fakeRuntime: &fakeRuntime{}}
			d := Driver{a, j, runtime}
			err = d.WithPairedExecution(context.Background(), fixturePairInput(a), func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
				var created ContainerReceipt
				if missing != "create" {
					_, created = bindAndCreate(t, w, a)
				}
				calls := runtime.preflights.Load()
				switch missing {
				case "create":
					if _, err := w.Bind(fixtureRef(2)); err == nil {
						t.Fatal("inspect/start/cleanup authority bound worker")
					}
					if _, err := w.Create(fixtureHandoff(fixturePairHash(w.Binding()), a), syntheticJIT); err == nil {
						t.Fatal("create phase bypassed")
					}
				case "start":
					if _, err := w.Start(created, fixtureRef(7)); err == nil {
						t.Fatal("start phase bypassed")
					}
				case "inspect":
					if _, err := w.Observe(); err == nil {
						t.Fatal("inspect phase bypassed")
					}
				case "cleanup":
					if _, err := w.DeleteTerminal(TerminalDecisionRef{created.PairSHA256, created.ContainerID, created.CreateResult, fixtureRef(8)}); err == nil {
						t.Fatal("cleanup phase bypassed")
					}
				}
				if runtime.preflights.Load() != calls || runtime.starts.Load()+runtime.deletes.Load() != 0 {
					t.Fatal("phase mismatch reached runtime")
				}
				return nil
			})
			if err != nil {
				t.Fatal("phase scope failed")
			}
		})
	}
}

func TestPairedFreshControllerRejectionAndWrongWorkerInputs(t *testing.T) {
	for _, fault := range []string{"old-controller-intent", "nonce", "head", "harness", "attempt", "source", "set", "oversize-binding", "memory-journal", "unsupported-runtime"} {
		t.Run(fault, func(t *testing.T) {
			d, _, runtime, in := pairedFixture(t)
			check := func(ControllerCheck) error { return nil }
			switch fault {
			case "old-controller-intent":
				check = func(c ControllerCheck) error {
					if c.Stage == CheckScope {
						return errors.New("old controller-only intent")
					}
					return nil
				}
			case "nonce":
				in.OwnerNonce = strings.Repeat("a", 32)
			case "head":
				in.Source.HeadSHA = strings.Repeat("a", 40)
			case "harness":
				in.HarnessSHA = strings.Repeat("a", 40)
			case "attempt":
				in.Source.Attempt = 2
			case "source":
				in.Source.RepositoryID = 0
			case "set":
				in.ScaleSetName = "foreign-set"
			case "oversize-binding":
				in.Source.WorkflowPath = ".github/workflows/" + strings.Repeat("a", maxPairBinding) + ".yml"
			case "memory-journal":
				d.Journal = &memoryJournal{}
			case "unsupported-runtime":
				d.Runtime = &fakeRuntime{}
			}
			entered := false
			if err := d.WithPairedExecution(context.Background(), in, check, func(*PairedWorker) error { entered = true; return nil }); err == nil || entered || runtime.preflights.Load() != 0 {
				t.Fatal("invalid entry reached callback/runtime")
			}
		})
	}
}

func TestPairedDriverMutationAndReentrantCallUseCapturedInputs(t *testing.T) {
	d, j, runtime, in := pairedFixture(t)
	a := d.Approval
	var worker *PairedWorker
	reentered := false
	err := d.WithPairedExecution(context.Background(), in, func(c ControllerCheck) error {
		if worker != nil && c.Stage == CheckCreate && !reentered {
			reentered = true
			if _, err := worker.Observe(); err == nil {
				t.Error("checker reentrancy took operation lock")
			}
		}
		return nil
	}, func(w *PairedWorker) error {
		worker = w
		d.Approval.Phases[0] = "inspect"
		d.Approval.OwnerNonce = strings.Repeat("a", 32)
		d.Journal = &memoryJournal{}
		d.Runtime = &fakeRuntime{}
		_, created := bindAndCreate(t, w, a)
		if _, err := w.Start(created, fixtureRef(7)); err != nil {
			t.Fatal("Driver mutation changed captured operation authority")
		}
		return nil
	})
	if err != nil || !reentered || runtime.creates.Load() != 1 || runtime.starts.Load() != 1 || len(j.Events()) == 0 {
		t.Fatal("captured-input lifecycle failed")
	}
}

func TestPairedCurrentIdentityAuthorityAndCancellationFenceEveryOperation(t *testing.T) {
	for _, fault := range []string{"journal", "directory", "claim", "admission-directory", "authority", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			d, j, runtime, in := pairedFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := d.WithPairedExecution(ctx, in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
				_, created := bindAndCreate(t, w, d.Approval)
				before := runtime.preflights.Load()
				switch fault {
				case "journal":
					path := filepath.Join(j.directory, "journal.jsonl")
					if os.Rename(path, path+".old") != nil || os.WriteFile(path, []byte("replacement"), 0600) != nil {
						t.Fatal("fixture replacement failed")
					}
				case "directory":
					if os.Rename(j.directory, j.directory+".old") != nil || os.Mkdir(j.directory, 0700) != nil {
						t.Fatal("fixture replacement failed")
					}
				case "claim":
					path := filepath.Join(j.claim.directory, "admission.json")
					if os.Rename(path, path+".old") != nil || os.WriteFile(path, []byte("replacement"), 0600) != nil {
						t.Fatal("fixture replacement failed")
					}
				case "admission-directory":
					if os.Rename(j.claim.directory, j.claim.directory+".old") != nil || os.Mkdir(j.claim.directory, 0700) != nil {
						t.Fatal("fixture replacement failed")
					}
				case "authority":
					j.mu.Lock()
					j.authority.Digest = strings.Repeat("a", 64)
					j.mu.Unlock()
				case "cancel":
					cancel()
				}
				if _, err := w.Start(created, fixtureRef(7)); err == nil || runtime.starts.Load() != 0 || runtime.preflights.Load() != before {
					t.Fatal("changed current authority reached runtime")
				}
				return nil
			})
			if fault != "cancel" && err != nil {
				t.Fatal("fixture scope return failed")
			}
		})
	}
}

type gatedPairRuntime struct {
	*pairedFakeRuntime
	started chan struct{}
	release chan struct{}
}

func (r *gatedPairRuntime) Create(ctx context.Context, name string, payload map[string]any) (string, bool, error) {
	id, warnings, err := r.fakeRuntime.Create(ctx, name, payload)
	close(r.started)
	<-r.release
	return id, warnings, err
}

func TestPairedScopeRevokesCopiesAndDrainsKnownResultBeforeClose(t *testing.T) {
	d, j, runtime, in := pairedFixture(t)
	gated := &gatedPairRuntime{runtime, make(chan struct{}), make(chan struct{})}
	d.Runtime = gated
	var retained PairedWorker
	created := make(chan error, 1)
	done := make(chan error, 1)
	go func() {
		done <- d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
			retained = *w
			pair, err := w.Bind(fixtureRef(2))
			if err != nil {
				return err
			}
			go func() { _, err := w.Create(fixtureHandoff(pair.PairSHA256, d.Approval), syntheticJIT); created <- err }()
			<-gated.started
			if _, err := w.Observe(); err == nil {
				return errors.New("concurrent method did not refuse")
			}
			return nil
		})
	}()
	<-gated.started
	closed := make(chan struct{})
	go func() { _ = j.Close(); close(closed) }()
	select {
	case <-done:
		t.Fatal("scope returned before result drain")
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case <-closed:
		t.Fatal("Close released claim during scope")
	default:
	}
	close(gated.release)
	if err := <-done; err != nil {
		t.Fatal("drained scope failed")
	}
	if err := <-created; err != nil {
		t.Fatal("known completed create result was discarded")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not resume")
	}
	if _, err := retained.Bind(fixtureRef(2)); err == nil || runtime.creates.Load() != 1 {
		t.Fatal("copied receiver retained authority")
	}
	if !validRef(j.pairState().created.CreateResult) {
		t.Fatal("result missing before lease release")
	}
}

func TestPairedPanicRevokesAndZeroReceiverRefuses(t *testing.T) {
	d, j, _, in := pairedFixture(t)
	var retained PairedWorker
	func() {
		defer func() {
			if recover() == nil {
				t.Error("fixture panic did not propagate")
			}
		}()
		_ = d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error { retained = *w; panic("synthetic panic") })
	}()
	if _, err := retained.Bind(fixtureRef(2)); err == nil {
		t.Fatal("panic left scope active")
	}
	release, err := j.authorize(d.Approval)
	if err != nil {
		t.Fatal("panic retained execution lock")
	}
	release()
	var zero PairedWorker
	if zero.Binding() != (PairBinding{}) {
		t.Fatal("zero binding was fabricated")
	}
	if _, err := zero.Bind(fixtureRef(2)); err == nil {
		t.Fatal("zero receiver accepted operation")
	}
}

// The stored authority and scope start with the same valid real deadline.
func TestPairedRealApprovalDeadlineFencesNextRequest(t *testing.T) {
	a := approval()
	a.ExpiresAt = time.Now().Add(500 * time.Millisecond)
	j, err := openTestJournal(t, privateDir(t), a)
	if err != nil {
		t.Fatal("short valid approval fixture failed")
	}
	defer j.Close()
	runtime := &pairedFakeRuntime{fakeRuntime: &fakeRuntime{image: ImageProfile{}}}
	d := Driver{a, j, runtime}
	entered := false
	err = d.WithPairedExecution(context.Background(), fixturePairInput(a), func(ControllerCheck) error { return nil }, func(w *PairedWorker) error {
		entered = true
		_, created := bindAndCreate(t, w, a)
		before := runtime.preflights.Load()
		select {
		case <-w.scope.ctx.Done():
		case <-time.After(2 * time.Second):
			t.Fatal("real approval deadline did not fire")
		}
		if time.Now().Before(a.ExpiresAt) || approvalDigest(a) != j.authority.Digest {
			t.Fatal("fixture did not cross matching authority deadline")
		}
		if _, err := w.Start(created, fixtureRef(7)); err == nil || runtime.preflights.Load() != before || runtime.starts.Load() != 0 {
			t.Fatal("expired authority reached runtime")
		}
		return nil
	})
	if !entered || err == nil {
		t.Fatal("real expiry was not observed")
	}
}

func TestPairedTypedNilRuntimeRefusesBeforeCallbackOrBinding(t *testing.T) {
	d, j, _, in := pairedFixture(t)
	var docker *Docker
	d.Runtime = docker
	entered := false
	err := d.WithPairedExecution(context.Background(), in, func(ControllerCheck) error { return nil }, func(w *PairedWorker) error { entered = true; _, err := w.Bind(fixtureRef(2)); return err })
	if err == nil || entered || len(j.Events()) != 0 {
		t.Fatal("typed-nil runtime entered scope or recorded binding")
	}
}
