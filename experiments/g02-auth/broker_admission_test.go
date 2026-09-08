package enrollment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Fixtures inject only this canonical newly-owned root. No production input,
// approval, environment variable or flag selects the admission directory.
func brokerTestPlan(t *testing.T, a *BrokerApproval, parent string, launch func(context.Context, []byte, string) error) *brokerControllerPlan {
	t.Helper()
	a.ControllerHarnessSHA = strings.Repeat("c", 40)
	a.ControllerBinarySHA256 = strings.Repeat("d", 64)
	state := filepath.Join(parent, "controller-state")
	if e := os.Mkdir(state, 0700); e != nil && !os.IsExist(e) {
		t.Fatal("fixture state")
	}
	root, e := openBrokerPrivateDirectory(state)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { root.Close() })
	c := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: a.ControllerHarnessSHA, WorkflowSHA: strings.Repeat("b", 40), WorkflowPath: ".github/workflows/canary.yml", Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "jit-loss", "inspect", "cleanup"}}
	raw, _ := json.Marshal(c)
	a.ControllerApprovalSHA256 = brokerBytesDigest(raw)
	p, e := newBrokerControllerPlan(*a, c, raw, root, state, func() error { return nil }, launch)
	if e != nil {
		t.Fatal(e)
	}
	p.localPrepare = func(context.Context, string) (brokerPreparationReceipt, error) {
		return brokerSyntheticPreparation(t, p, filepath.Join(parent, "admission"))
	}
	return p
}
func TestBrokerFinitePhasesAndUnknownRetention(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "unknown"}[unknown], func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			parent := filepath.Dir(root)
			if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil); e != nil {
				t.Fatal("discovery positive")
			}
			a.Mode = "controller"
			for _, phase := range []string{"create", "jit-loss", "inspect", "cleanup"} {
				a.Phase = phase
				f.root = filepath.Join(parent, phase)
				plan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error {
					if unknown && phase == "create" {
						return errBroker
					}
					return nil
				})
				_, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, f.root, api, plan)
				refused := unknown && (phase == "create" || phase == "jit-loss")
				if (e != nil) != refused {
					t.Fatalf("phase=%s refused=%t want=%t", phase, e != nil, refused)
				}
			}
			want := 5
			if unknown {
				want = 4
			}
			if f.tokenCalls != want {
				t.Fatalf("mint=%d want=%d", f.tokenCalls, want)
			}
			for _, phase := range []string{"inspect", "cleanup", "create"} {
				a.Phase = phase
				a.ExpiresAt = a.ExpiresAt.Add(time.Minute)
				f.root = filepath.Join(parent, phase+"-retry")
				plan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error { return nil })
				if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, f.root, api, plan); e == nil {
					t.Fatal("consumed phase retried")
				}
			}
			if f.tokenCalls != want {
				t.Fatal("retry consumed token")
			}
			data, e := os.ReadFile(filepath.Join(f.admissionRoot, "broker-admission.jsonl"))
			if e != nil {
				t.Fatal("ledger")
			}
			if unknown {
				var claim, complete int
				for _, line := range strings.Split(string(data), "\n") {
					var event brokerClaimEvent
					if json.Unmarshal([]byte(line), &event) == nil && event.Slot == "create" {
						if event.Kind == "claim" {
							claim++
						}
						if event.Kind == "complete" {
							complete++
						}
					}
				}
				if claim != 1 || complete != 0 {
					t.Fatal("earlier unknown erased by recovery")
				}
			}
		})
	}
}

func TestBrokerPairedAdmissionAcceptsHistoricalControllerClaim(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	parent := filepath.Dir(root)
	a.Mode, a.Phase, a.AllowVerificationAuthority = "controller", "create", true
	controllerPlan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error { return nil })
	controllerPlan.controller.Phases = []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "inspect", "cleanup"}
	controllerPlan.controller.WorkflowRunID = 7
	controllerPlan.raw, _ = json.Marshal(controllerPlan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(controllerPlan.raw)
	controllerPlan.approval = a
	if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM), VerificationToken: "synthetic-private-workflow-token"}, root, api, controllerPlan); err != nil {
		t.Fatalf("controller prerequisite claim: %v", err)
	}

	// The paired attempt reuses the same controller identity and durable ledger,
	// as the real controller-create -> paired-terminal sequence does.
	a.Mode, a.Phase = "paired-terminal", "paired-terminal"
	pairedRoot := filepath.Join(parent, "paired-attempt")
	pairedPlan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error { return nil })
	pairedPlan.controller.Phases = append([]string(nil), controllerPlan.controller.Phases...)
	pairedPlan.controller.WorkflowRunID = controllerPlan.controller.WorkflowRunID
	pairedPlan.raw, _ = json.Marshal(pairedPlan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(pairedPlan.raw)
	pairedPlan.approval = a
	workerState := filepath.Join(parent, "paired-worker-state")
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal("worker state")
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: pairedPlan.controller.HarnessSHA, WorkflowSHA: pairedPlan.controller.WorkflowSHA, OwnerNonce: pairedPlan.controller.OwnerNonce, Controller: pairedPlan.controller.Controller, Endpoint: "/tmp/g01-paired-admission.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: pairedPlan.controller.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal("worker approval")
	}
	workerPath := filepath.Join(parent, "paired-worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal("worker approval file")
	}
	pairedPlan.worker, err = openBrokerWorkerPlan(workerPath, workerState, pairedPlan.statePath, a, pairedPlan.controller)
	if err != nil {
		t.Fatalf("paired worker plan: %v", err)
	}
	brokerAttachSyntheticWorkerPreparation(t, pairedPlan, filepath.Join(parent, "paired-worker-admission"))
	defer pairedPlan.worker.close()
	if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM), VerificationToken: "synthetic-private-workflow-token"}, pairedRoot, api, pairedPlan); err != nil {
		t.Fatalf("paired attempt rejected historical controller claim: %v", err)
	}
	if f.tokenCalls != 2 {
		t.Fatalf("historical controller claim blocked current paired issuance: mints=%d", f.tokenCalls)
	}
	ledger, err := os.ReadFile(filepath.Join(f.admissionRoot, "broker-admission.jsonl"))
	if err != nil {
		t.Fatal("paired ledger")
	}
	lines := strings.Split(strings.TrimSpace(string(ledger)), "\n")
	var pairedEvent brokerClaimEvent
	for _, line := range lines {
		var candidate brokerClaimEvent
		if json.Unmarshal([]byte(line), &candidate) == nil && candidate.Slot == "paired-terminal" && candidate.Kind == "claim" {
			pairedEvent = candidate
			break
		}
	}
	controllerView := a
	controllerView.Mode, controllerView.Phase = "controller", "inspect"
	if pairedEvent.Slot == "" || !validBrokerClaimEvent(controllerView, pairedEvent) {
		t.Fatal("historical paired claim was rejected under the reciprocal controller mode")
	}
}
func TestBrokerAdmissionFailureBeforeAPIAndResync(t *testing.T) {
	for _, kind := range []string{"missing", "symlink", "sync", "existing-sync"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			switch kind {
			case "missing":
				api.admissionDirectory = func() (string, error) { return filepath.Join(f.admissionRoot, "absent"), nil }
			case "symlink":
				link := filepath.Join(filepath.Dir(f.admissionRoot), "alias")
				if os.Symlink(f.admissionRoot, link) != nil {
					t.Fatal("fixture link")
				}
				api.admissionDirectory = func() (string, error) { return link, nil }
			case "sync", "existing-sync":
				api.syncDirectory = func(*os.Root) error { return errBroker }
			}
			attempts := 1
			if kind == "existing-sync" {
				attempts = 2
			}
			for i := 0; i < attempts; i++ {
				if i > 0 {
					root = filepath.Join(filepath.Dir(root), "again")
					f.root = root
				}
				if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil); e == nil {
					t.Fatal("invalid admission accepted")
				}
			}
			if len(f.calls) != 0 {
				t.Fatal("admission failure contacted API")
			}
		})
	}
}
func TestBrokerSnapshotReservedAndRecheckedBeforeHandoff(t *testing.T) {
	for _, kind := range []string{"regular", "symlink", "changed", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			called := false
			p := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { called = true; return nil })
			if kind == "regular" || kind == "symlink" {
				if os.Mkdir(root, 0700) != nil {
					t.Fatal("fixture")
				}
				name := filepath.Join(root, "controller-approval.json")
				if kind == "regular" {
					os.WriteFile(name, []byte("existing"), 0600)
				} else {
					os.Symlink(filepath.Join(filepath.Dir(root), "absent"), name)
				}
			} else {
				old := p.binaryCheck
				p.binaryCheck = func() error {
					if p.snapshot != nil && f.tokenCalls > 0 {
						name := filepath.Join(root, "controller-approval.json")
						if kind == "replaced" {
							os.Rename(name, name+".retained")
						}
						os.WriteFile(name, []byte("altered"), 0600)
					}
					return old()
				}
			}
			_, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, p)
			if e == nil || called {
				t.Fatal("unsafe snapshot reached handoff")
			}
			want := 0
			if kind == "changed" || kind == "replaced" {
				want = 1
			}
			if f.tokenCalls != want {
				t.Fatalf("mint=%d want=%d", f.tokenCalls, want)
			}
		})
	}
}

func TestBrokerMalformedLedgerCannotBecomePartialAuthority(t *testing.T) {
	for _, kind := range []string{"missing controller", "missing authority", "bad snapshot", "bad ownership", "discovery controller", "invalid renewal", "duplicate complete", "missing newline", "work after unknown", "late completion"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			parent := filepath.Dir(root)
			p := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error { return nil })
			if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, p); e != nil {
				t.Fatal("initial phase")
			}
			path := filepath.Join(f.admissionRoot, "broker-admission.jsonl")
			data, _ := os.ReadFile(path)
			lines := strings.Split(string(data), "\n")
			var event brokerClaimEvent
			if json.Unmarshal([]byte(lines[1]), &event) != nil {
				t.Fatal("event")
			}
			switch kind {
			case "missing controller":
				event.Controller = nil
			case "missing authority":
				event.Authority = nil
			case "bad snapshot":
				event.Snapshot = brokerInode{}
			case "bad ownership":
				event.Controller.Ownership = strings.Repeat("e", 64)
			case "discovery controller":
				event.Slot = "discover-actions-host"
			case "invalid renewal":
				event.Attempt.Inode++
				event.Journal.Inode++
				event.Snapshot.Inode++
				event.Slot = "jit-loss"
				event.Authority.Approval.ExpiresAt = event.Authority.Approval.ExpiresAt.Add(time.Minute)
				event.Authority.Digest = brokerDigest(event.Authority.Approval)
				b, _ := json.Marshal(event)
				lines = append(lines[:len(lines)-1], string(b), "")
			case "work after unknown", "late completion":
				originalComplete := lines[2]
				event.Attempt.Inode++
				event.Journal.Inode++
				event.Snapshot.Inode++
				event.Slot = "inspect"
				if kind == "work after unknown" {
					event.Slot = "jit-loss"
				}
				b, _ := json.Marshal(event)
				lines = []string{lines[0], lines[1], string(b), ""}
				if kind == "late completion" {
					lines = []string{lines[0], lines[1], string(b), originalComplete, ""}
				}
			case "duplicate complete":
				lines = append(lines[:len(lines)-1], lines[2], "")
			}
			if kind != "invalid renewal" && kind != "duplicate complete" && kind != "work after unknown" && kind != "late completion" {
				b, _ := json.Marshal(event)
				lines[1] = string(b)
			}
			altered := strings.Join(lines, "\n")
			if kind == "missing newline" {
				altered = strings.TrimSuffix(altered, "\n")
			}
			if os.WriteFile(path, []byte(altered), 0600) != nil {
				t.Fatal("fixture write")
			}
			a.Phase = "inspect"
			f.root = filepath.Join(parent, "inspect")
			p = brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error { return nil })
			before := len(f.calls)
			if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, f.root, api, p); e == nil || f.tokenCalls != 1 || len(f.calls) != before {
				t.Fatal("corrupt ledger permitted authenticated effects")
			}
		})
	}
}
func TestBrokerControllerRenewalOnlyRecoveryAndSameOwnership(t *testing.T) {
	for _, kind := range []string{"recovery", "expiry rollback", "new work", "workflow", "state", "binary", "nonce"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			parent := filepath.Dir(root)
			p := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error { return nil })
			if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, p); e != nil {
				t.Fatal("initial controller")
			}
			a.Phase = "inspect"
			a.ExpiresAt = a.ExpiresAt.Add(time.Minute)
			f.root = filepath.Join(parent, "inspect")
			p = brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error { return nil })
			p.controller.Phases = []string{"inspect", "cleanup"}
			switch kind {
			case "expiry rollback":
				p.controller.ExpiresAt = p.controller.ExpiresAt.Add(-2 * time.Minute)
				a.ExpiresAt = p.controller.ExpiresAt
			case "new work":
				p.controller.Phases = []string{"inspect", "create"}
			case "workflow":
				p.controller.WorkflowSHA = strings.Repeat("e", 40)
			case "state":
				os.Rename(p.statePath, p.statePath+"-retained")
				os.Mkdir(p.statePath, 0700)
			case "binary":
				a.ControllerBinarySHA256 = strings.Repeat("e", 64)
			case "nonce":
				a.OwnerNonce = strings.Repeat("e", 32)
				p.controller.OwnerNonce = a.OwnerNonce
			}
			p.raw, _ = json.Marshal(p.controller)
			a.ControllerApprovalSHA256 = brokerBytesDigest(p.raw)
			p.approval = a
			_, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, f.root, api, p)
			if kind == "recovery" {
				if e != nil || f.tokenCalls != 2 {
					t.Fatal("authorized stable recovery refused")
				}
			} else if e == nil || f.tokenCalls != 1 {
				t.Fatal("ownership or authority rebound before mint")
			}
		})
	}
}
func TestBrokerPreexistingControllerClaimRefusesBeforeMint(t *testing.T) {
	for _, kind := range []string{"conflict", "empty", "symlink", "partial journal", "matching"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			p := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { return nil })
			path := filepath.Join(f.admissionRoot, "admission.json")
			stateJournal := filepath.Join(p.statePath, "journal.jsonl")
			if os.WriteFile(stateJournal, []byte("synthetic owned journal\n"), 0600) != nil {
				t.Fatal("fixture journal")
			}
			ji, _ := os.Stat(stateJournal)
			jid := brokerFileIdentity(ji)
			binding, _ := p.binding()
			record := map[string]any{"version": 1, "ownership": binding.Ownership, "state_device": binding.State.Device, "state_inode": binding.State.Inode, "journal_device": jid.Device, "journal_inode": jid.Inode}
			if kind == "conflict" {
				record["ownership"] = strings.Repeat("e", 64)
			}
			data, _ := json.Marshal(record)
			switch kind {
			case "empty":
				data = nil
			case "symlink":
				os.Symlink(stateJournal, path)
			}
			if kind != "symlink" && kind != "partial journal" {
				if os.WriteFile(path, data, 0600) != nil {
					t.Fatal("fixture claim")
				}
			}
			_, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, p)
			if kind == "matching" {
				if e != nil || f.tokenCalls != 1 {
					t.Fatal("compatible claim refused")
				}
			} else if e == nil || f.tokenCalls != 0 {
				t.Fatal("conflicting/ambiguous inventory consumed token")
			}
		})
	}
}

func TestBrokerAdmissionResyncFailureAndRootReplacement(t *testing.T) {
	for _, kind := range []string{"existing root sync", "claim root sync", "root replacement", "parent replacement", "lock replacement", "nonempty lock"} {
		t.Run(kind, func(t *testing.T) {
			a, _, api, f, root := newBrokerFixture(t)
			j, e := openBrokerJournal(root, a)
			if e != nil {
				t.Fatal(e)
			}
			defer j.close()
			calls := 0
			syncFn := func(r *os.Root) error {
				calls++
				if kind == "claim root sync" && calls >= 6 {
					return errBroker
				}
				return syncDirectory(r)
			}
			claim, e := openBrokerAdmission(f.admissionRoot, a, j, nil, syncFn)
			if kind == "claim root sync" {
				if e == nil {
					claim.close()
					t.Fatal("failed claim sync accepted")
				}
				api.syncDirectory = func(*os.Root) error { return errBroker }
				j2, e := openBrokerJournal(filepath.Join(filepath.Dir(root), "retry"), a)
				if e != nil {
					t.Fatal(e)
				}
				defer j2.close()
				if c, e := openBrokerAdmission(f.admissionRoot, a, j2, nil, api.syncDirectory); e == nil {
					c.close()
					t.Fatal("existing failed record skipped resync")
				}
				return
			}
			if e != nil {
				t.Fatal("claim positive")
			}
			defer claim.close()
			switch kind {
			case "lock replacement":
				name := filepath.Join(f.admissionRoot, "broker-admission.lock")
				os.Rename(name, name+"-retained")
				os.WriteFile(name, nil, 0600)
			case "nonempty lock":
				os.WriteFile(filepath.Join(f.admissionRoot, "broker-admission.lock"), []byte("unknown"), 0600)
			case "existing root sync":
				claim.sync = func(*os.Root) error { return errBroker }
			case "root replacement":
				os.Rename(f.admissionRoot, f.admissionRoot+"-retained")
				os.Mkdir(f.admissionRoot, 0700)
			case "parent replacement":
				parent := filepath.Dir(f.admissionRoot)
				os.Rename(parent, parent+"-retained")
				t.Cleanup(func() { os.RemoveAll(parent); os.Rename(parent+"-retained", parent) })
				os.Mkdir(parent, 0700)
				os.Mkdir(f.admissionRoot, 0700)
			}
			if claim.check() == nil {
				t.Fatal("lost durability/identity still authorized")
			}
		})
	}
}

func TestBrokerActualG01ClaimSerializationCompatibility(t *testing.T) {
	directory := os.Getenv("G01_BROKER_BRIDGE_FIXTURE")
	if directory == "" {
		t.Skip("requires separately generated G01 local journal fixture")
	}
	raw, e := os.ReadFile(filepath.Join(directory, "controller.json"))
	var c controllerApproval
	if e != nil || decodeBrokerJSON(raw, &c, true) != nil {
		t.Fatal("G01 typed approval fixture")
	}
	a := brokerApprovalFixture()
	a.Mode = "controller"
	a.Phase = "create"
	a.ExpiresAt = c.ExpiresAt
	a.ControllerHarnessSHA = c.HarnessSHA
	a.ControllerBinarySHA256 = strings.Repeat("d", 64)
	a.ControllerApprovalSHA256 = brokerBytesDigest(raw)
	state, e := openBrokerPrivateDirectory(filepath.Join(directory, "state"))
	if e != nil {
		t.Fatal("G01 state fixture")
	}
	defer state.Close()
	root, e := openBrokerPrivateDirectory(filepath.Join(directory, "admission"))
	if e != nil {
		t.Fatal("G01 claim fixture")
	}
	defer root.Close()
	p, e := newBrokerControllerPlan(a, c, raw, state, filepath.Join(directory, "state"), func() error { return nil }, func(context.Context, []byte, string) error { return nil })
	if e != nil || p.compatibleControllerClaim(root) != nil {
		t.Fatal("actual G01 typed serialization/admission claim incompatible")
	}
}

func TestBrokerAllFiniteSlotsUseSameControllerAuthority(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	parent := filepath.Dir(root)
	if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil); e != nil {
		t.Fatal("discovery")
	}
	phases := []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "jit-loss", "inspect", "cleanup"}
	a.Mode = "controller"
	a.AllowVerificationAuthority = true
	for _, phase := range phases {
		a.Phase = phase
		f.root = filepath.Join(parent, phase)
		p := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error { return nil })
		p.controller.Phases = phases
		p.controller.WorkflowRunID = 7
		p.raw, _ = json.Marshal(p.controller)
		a.ControllerApprovalSHA256 = brokerBytesDigest(p.raw)
		p.approval = a
		if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM), VerificationToken: "synthetic-distinct-workflow-authority"}, f.root, api, p); e != nil {
			t.Fatalf("approved slot %s refused", phase)
		}
	}
	if f.tokenCalls != 9 {
		t.Fatalf("finite issuance count=%d", f.tokenCalls)
	}
}

func TestBrokerControllerClaimReadUsesInitializationLease(t *testing.T) {
	a, _, _, f, root := newBrokerFixture(t)
	a.Mode = "controller"
	a.Phase = "create"
	p := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { return nil })
	admission, e := openBrokerPrivateDirectory(f.admissionRoot)
	if e != nil {
		t.Fatal("fixture")
	}
	defer admission.Close()
	directory, e := admission.Open(".")
	if e != nil {
		t.Fatal("fixture directory")
	}
	defer directory.Close()
	if syscall.Flock(int(directory.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		t.Fatal("fixture initialization lease")
	}
	if p.compatibleControllerClaim(admission) == nil {
		t.Error("controller claim checked while another initializer held directory")
	}
	if _, e := admission.Lstat("admission.json"); !os.IsNotExist(e) {
		t.Fatal("compatibility created/adopted a claim")
	}
	if syscall.Flock(int(directory.Fd()), syscall.LOCK_UN) != nil {
		t.Fatal("fixture unlock")
	}
	if p.compatibleControllerClaim(admission) != nil {
		t.Fatal("empty unchanged inventory refused after lease released")
	}
}

func TestBrokerLockReplacementRefusesOnReopen(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil); e != nil {
		t.Fatal("initial discovery")
	}
	lock := filepath.Join(f.admissionRoot, "broker-admission.lock")
	if os.Rename(lock, lock+"-retained") != nil || os.WriteFile(lock, nil, 0600) != nil {
		t.Fatal("owned replacement fixture")
	}
	// An unused slot must not rebind the replacement serialization identity.
	a.Mode = "controller"
	a.Phase = "inspect"
	f.root = filepath.Join(filepath.Dir(root), "inspect")
	p := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { return nil })
	before := len(f.calls)
	if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, f.root, api, p); e == nil || len(f.calls) != before || f.tokenCalls != 1 {
		t.Fatal("replacement lock rebound ledger on reopen")
	}
}

func TestBrokerInvalidCanonicalJournalRefusesBeforeMint(t *testing.T) {
	for _, kind := range []string{"locked", "malformed", "oversized", "mismatched authority", "phase ineligible"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			launched := false
			p := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { launched = true; return errBroker })
			// Sequencing fixture: the actual canonical parser/phase matrix is exercised
			// by G01 tests and the optional cross-module executable test below.
			p.localPrepare = func(context.Context, string) (brokerPreparationReceipt, error) {
				return brokerPreparationReceipt{}, errBroker
			}

			binding, _ := p.binding()
			header := map[string]any{"version": 1, "ownership": binding.Ownership, "authority": map[string]any{"digest": brokerDigest(p.controller), "expires_at": p.controller.ExpiresAt, "phases": p.controller.Phases}}
			if kind == "mismatched authority" {
				header["authority"].(map[string]any)["digest"] = strings.Repeat("f", 64)
			}
			raw, _ := json.Marshal(header)
			raw = append(raw, '\n')
			switch kind {
			case "malformed":
				raw = append(raw, []byte("{invalid-event}\n")...)
			case "oversized":
				raw = []byte(strings.Repeat("x", (1<<20)+1))
			case "phase ineligible":
				raw = append(raw, []byte(`{"sequence":1,"kind":"phase","operation":"create"}`+"\n")...)
			}
			path := filepath.Join(p.statePath, "journal.jsonl")
			if os.WriteFile(path, raw, 0600) != nil {
				t.Fatal("fixture journal")
			}
			journal, e := os.OpenFile(path, os.O_RDWR, 0)
			if e != nil {
				t.Fatal("fixture open")
			}
			defer journal.Close()
			if kind == "locked" && syscall.Flock(int(journal.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
				t.Fatal("fixture lease")
			}
			ji, _ := journal.Stat()
			id := brokerFileIdentity(ji)
			claim := map[string]any{"version": 1, "ownership": binding.Ownership, "state_device": binding.State.Device, "state_inode": binding.State.Inode, "journal_device": id.Device, "journal_inode": id.Inode}
			encoded, _ := json.Marshal(claim)
			if os.WriteFile(filepath.Join(f.admissionRoot, "admission.json"), encoded, 0600) != nil {
				t.Fatal("fixture claim")
			}
			_, e = brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, p)
			if e == nil || f.tokenCalls != 0 || launched {
				t.Fatalf("known-invalid controller journal reached issuance/handoff: mints=%d launched=%t", f.tokenCalls, launched)
			}
		})
	}
}
