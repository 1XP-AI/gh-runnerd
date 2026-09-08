package enrollment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pairedPlanInputs(t *testing.T) (BrokerApproval, controllerApproval, string, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	controllerState := filepath.Join(root, "controller-state")
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Hour)
	nonce := strings.Repeat("a", 32)
	harness := strings.Repeat("b", 40)
	workflow := strings.Repeat("c", 40)
	a := brokerApprovalFixture()
	a.Mode, a.Phase = "paired-terminal", "paired-terminal"
	a.ExpiresAt = now
	a.ControllerHarnessSHA = harness
	c := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: nonce, HarnessSHA: harness, WorkflowSHA: workflow, WorkflowPath: ".github/workflows/canary.yml", Controller: "trusted-controller", ExpiresAt: now, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "inspect", "cleanup", "jit-loss"}}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: harness, WorkflowSHA: workflow, OwnerNonce: nonce, Controller: c.Controller, Endpoint: "/tmp/g01-paired-docker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: now, Phases: []string{"create", "start", "inspect", "cleanup"}}
	data, err := json.Marshal(worker)
	if err != nil {
		t.Fatal(err)
	}
	workerPath := filepath.Join(root, "worker.json")
	if err := os.WriteFile(workerPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	return a, c, controllerState, workerState, workerPath
}

func TestPairedWorkerBindingRetainsApprovalAndStateIdentity(t *testing.T) {
	a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
	plan, err := openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c)
	if err != nil {
		t.Fatal("valid paired worker plan refused")
	}
	defer plan.close()
	if _, err := plan.binding(); err != nil {
		t.Fatal("valid paired worker binding unavailable")
	}
	if err := os.Rename(workerPath, workerPath+".retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, plan.raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.binding(); err == nil {
		t.Fatal("replaced worker approval retained authority")
	}
}

func TestPairedWorkerApprovalMismatchRefusesBeforeBinding(t *testing.T) {
	a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
	raw, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatal(err)
	}
	var worker pairedWorkerApproval
	if err := decodeBrokerJSON(raw, &worker, true); err != nil {
		t.Fatal(err)
	}
	worker.WorkflowSHA = strings.Repeat("e", 40)
	raw, err = json.Marshal(worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c); err == nil {
		t.Fatal("mismatched worker identity accepted")
	}
}

func TestPairedWorkerDaemonIDMatchesCanonicalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		daemon string
		valid  bool
	}{
		{name: "colon and 128 bytes", daemon: "a:" + strings.Repeat("d", 126), valid: true},
		{name: "129 bytes", daemon: "a" + strings.Repeat("d", 128), valid: false},
		{name: "invalid slash", daemon: "a/b", valid: false},
		{name: "invalid leading punctuation", daemon: ":daemon", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
			data, err := os.ReadFile(workerPath)
			if err != nil {
				t.Fatal(err)
			}
			var worker pairedWorkerApproval
			if err := decodeBrokerJSON(data, &worker, true); err != nil {
				t.Fatal(err)
			}
			worker.DaemonID = tc.daemon
			data, err = json.Marshal(worker)
			if err != nil || os.WriteFile(workerPath, data, 0600) != nil {
				t.Fatal("worker approval rewrite")
			}
			_, err = openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c)
			if (err == nil) != tc.valid {
				t.Fatalf("daemon ID validity=%v want=%v: %v", err == nil, tc.valid, err)
			}
		})
	}
}

func TestPairedApprovalRejectsInsufficientTerminalAuthority(t *testing.T) {
	a := brokerApprovalFixture()
	a.Mode, a.Phase = "paired-terminal", "paired-terminal"
	a.ExpiresAt = time.Now().Add(90 * time.Second)
	if err := a.validate(time.Now()); err == nil {
		t.Fatal("paired approval accepted less than the bounded terminal completion budget")
	}
}

func TestPairedBrokerBindsWorkerBeforeWorkflowVerifiedHandoff(t *testing.T) {
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
	parent := filepath.Dir(attempt)
	launches := 0
	plan := brokerTestPlan(t, &a, parent, func(_ context.Context, data []byte, _ string) error {
		launches++
		verified := false
		for _, call := range fixture.calls {
			if strings.HasPrefix(call, "GET /repos/org-a/canary/actions/runs/") {
				verified = true
				break
			}
		}
		if !verified {
			t.Fatal("paired handoff launched before workflow verification")
		}
		var payload map[string]any
		if json.Unmarshal(data, &payload) != nil || payload["installation_token"] != fixture.token || payload["verification_token"] != "synthetic-private-verification-token" {
			t.Fatal("paired handoff lost broker-issued credentials")
		}
		if _, found := payload["pem"]; found {
			t.Fatal("PEM crossed the broker boundary")
		}
		return nil
	})

	// The paired mode requires the same reviewed workflow verification authority
	// as the original terminal fixture before the child is launched.
	plan.controller.Phases = []string{"create", "before-ack", "inspect", "cleanup"}
	plan.controller.WorkflowRunID = 7
	plan.raw, _ = json.Marshal(plan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(plan.raw)
	plan.approval = a

	workerState := filepath.Join(parent, "worker-state")
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: plan.controller.HarnessSHA, WorkflowSHA: plan.controller.WorkflowSHA, OwnerNonce: plan.controller.OwnerNonce, Controller: plan.controller.Controller, Endpoint: "/tmp/g01-paired-broker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: plan.controller.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal(err)
	}
	workerPath := filepath.Join(parent, "worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal(err)
	}
	workerPlan, err := openBrokerWorkerPlan(workerPath, workerState, plan.statePath, a, plan.controller)
	if err != nil {
		t.Fatal("valid paired worker plan refused")
	}
	plan.worker = workerPlan

	result, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"}, attempt, api, plan)
	if err != nil || result.Status != "paired_terminal_completed" || fixture.tokenCalls != 1 || launches != 1 {
		t.Fatalf("paired broker handoff incomplete: result=%+v err=%v mints=%d launches=%d", result, err, fixture.tokenCalls, launches)
	}
	verified := false
	for _, call := range fixture.calls {
		if strings.HasPrefix(call, "GET /repos/org-a/canary/actions/runs/") {
			verified = true
		}
	}
	if !verified {
		t.Fatal("workflow identity was not verified")
	}
	if fixture.tokenCalls != 1 {
		t.Fatal("paired handoff retried issuance")
	}
	ledger, err := os.ReadFile(filepath.Join(fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !strings.Contains(string(ledger), `"slot":"paired-terminal"`) || !strings.Contains(string(ledger), `"worker"`) || strings.Contains(string(ledger), fixture.token) || strings.Contains(string(ledger), string(candidate.PEM)) {
		t.Fatal("paired admission did not retain bounded worker binding")
	}
}

func TestPairedFailureAllowsAuthorizedInspectWithoutPairedRetry(t *testing.T) {
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
	parent := filepath.Dir(attempt)
	pairedLaunches := 0
	pairedPlan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error {
		pairedLaunches++
		return errBroker
	})
	pairedPlan.controller.Phases = []string{"create", "before-ack", "after-ack", "before-acquire", "inspect", "cleanup"}
	pairedPlan.controller.WorkflowRunID = 7
	pairedPlan.raw, _ = json.Marshal(pairedPlan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(pairedPlan.raw)
	pairedPlan.approval = a
	workerState := filepath.Join(parent, "failed-paired-worker-state")
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal("worker state")
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: pairedPlan.controller.HarnessSHA, WorkflowSHA: pairedPlan.controller.WorkflowSHA, OwnerNonce: pairedPlan.controller.OwnerNonce, Controller: pairedPlan.controller.Controller, Endpoint: "/tmp/g01-paired-failure.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: pairedPlan.controller.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal("worker approval")
	}
	workerPath := filepath.Join(parent, "failed-paired-worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal("worker approval file")
	}
	pairedPlan.worker, err = openBrokerWorkerPlan(workerPath, workerState, pairedPlan.statePath, a, pairedPlan.controller)
	if err != nil {
		t.Fatal("paired worker plan")
	}
	if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-workflow-token"}, attempt, api, pairedPlan); err == nil || fixture.tokenCalls != 1 || pairedLaunches != 1 {
		t.Fatalf("failed paired attempt was accepted or retried: err=%v mints=%d launches=%d", err, fixture.tokenCalls, pairedLaunches)
	}

	// Inspect is a distinct, explicitly authorized controller slot. It may
	// collect its own authenticated evidence, but it must not replay the
	// incomplete paired claim or invoke the worker handoff again.
	a.Mode, a.Phase = "controller", "inspect"
	inspectLaunches := 0
	inspectPlan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error {
		inspectLaunches++
		return nil
	})
	inspectPlan.controller.Phases = append([]string(nil), pairedPlan.controller.Phases...)
	inspectPlan.controller.WorkflowRunID = pairedPlan.controller.WorkflowRunID
	inspectPlan.raw, _ = json.Marshal(inspectPlan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(inspectPlan.raw)
	inspectPlan.approval = a
	_ = inspectPlan.state.Close()
	inspectPlan.state, err = openBrokerPrivateDirectory(pairedPlan.statePath)
	if err != nil {
		t.Fatal("reopen controller state")
	}
	t.Cleanup(func() { _ = inspectPlan.state.Close() })
	inspectPlan.statePath = pairedPlan.statePath
	inspectPlan.stateInfo, err = inspectPlan.state.Stat(".")
	if err != nil {
		t.Fatal("controller state identity")
	}
	result, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-workflow-token"}, filepath.Join(parent, "inspect-attempt"), api, inspectPlan)
	if err != nil || result.Status != "controller_completed" || fixture.tokenCalls != 2 || pairedLaunches != 1 || inspectLaunches != 1 {
		t.Fatalf("authorized inspect did not remain distinct from failed pair: result=%+v err=%v mints=%d paired=%d inspect=%d", result, err, fixture.tokenCalls, pairedLaunches, inspectLaunches)
	}
	// The inspect slot is also one-shot; reopening it cannot mint or launch.
	retryPlan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error {
		t.Fatal("inspect retry launched")
		return nil
	})
	retryPlan.controller.Phases = append([]string(nil), pairedPlan.controller.Phases...)
	retryPlan.controller.WorkflowRunID = pairedPlan.controller.WorkflowRunID
	retryPlan.raw, _ = json.Marshal(retryPlan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(retryPlan.raw)
	retryPlan.approval = a
	_ = retryPlan.state.Close()
	retryPlan.state, err = openBrokerPrivateDirectory(pairedPlan.statePath)
	if err != nil {
		t.Fatal("reopen controller state for retry")
	}
	t.Cleanup(func() { _ = retryPlan.state.Close() })
	retryPlan.statePath = pairedPlan.statePath
	retryPlan.stateInfo, err = retryPlan.state.Stat(".")
	if err != nil {
		t.Fatal("controller state retry identity")
	}
	if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-workflow-token"}, filepath.Join(parent, "inspect-retry"), api, retryPlan); err == nil || fixture.tokenCalls != 2 || pairedLaunches != 1 || inspectLaunches != 1 {
		t.Fatalf("inspect retry replayed paired or inspect effects: err=%v mints=%d paired=%d inspect=%d", err, fixture.tokenCalls, pairedLaunches, inspectLaunches)
	}
}

// This is intentionally an entrypoint-level fixture. It uses the real
// BrokerFiles loader, paired preparation process and brokerExecute closure, so
// the canonical preparation receipt and fixed child handoff are both exercised.
func TestPairedBrokerRealEntrypointUsesPairedPreparationClosure(t *testing.T) {
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	parent := filepath.Dir(attempt)
	controllerState := filepath.Join(parent, "paired-controller-state")
	workerState := filepath.Join(parent, "paired-worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal("controller state")
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal("worker state")
	}
	now := time.Now().Add(time.Hour)
	harness := strings.Repeat("c", 40)
	workflow := strings.Repeat("b", 40)
	a.Mode, a.Phase, a.AllowVerificationAuthority, a.ExpiresAt, a.ControllerHarnessSHA = "paired-terminal", "paired-terminal", true, now, harness
	controller := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: harness, WorkflowSHA: workflow, WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: now, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "inspect", "cleanup"}}
	controllerData, err := json.Marshal(controller)
	if err != nil {
		t.Fatal("controller approval")
	}
	controllerPath := filepath.Join(parent, "controller-approval.json")
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal("controller approval file")
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: harness, WorkflowSHA: workflow, OwnerNonce: a.OwnerNonce, Controller: controller.Controller, Endpoint: "/tmp/g01-paired-entry.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: now, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal("worker approval")
	}
	workerPath := filepath.Join(parent, "worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal("worker approval file")
	}
	binary := testBrokerBinary(t)
	a.ControllerBinarySHA256 = binary.digest
	a.ControllerApprovalSHA256 = brokerBytesDigest(controllerData)
	approvalPath := filepath.Join(parent, "broker-approval.json")
	approvalData, err := json.Marshal(a)
	if err != nil {
		t.Fatal("broker approval")
	}
	if err := os.WriteFile(approvalPath, approvalData, 0600); err != nil {
		t.Fatal("broker approval file")
	}
	inputPath := filepath.Join(parent, "broker-input.json")
	inputData, err := json.Marshal(brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"})
	if err != nil {
		t.Fatal("broker input")
	}
	if err := os.WriteFile(inputPath, inputData, 0600); err != nil {
		t.Fatal("broker input file")
	}
	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatal("broker input open")
	}
	defer input.Close()
	oldOpener := brokerBinaryOpener
	brokerBinaryOpener = func(string, BrokerApproval) (*verifiedBrokerBinary, error) { return binary, nil }
	defer func() { brokerBinaryOpener = oldOpener }()
	result, err := runBrokerWithAPI(context.Background(), BrokerFiles{ApprovalPath: approvalPath, StateDirectory: attempt, ControllerBinary: binary.path, ControllerApproval: controllerPath, ControllerStateDirectory: controllerState, WorkerApproval: workerPath, WorkerStateDirectory: workerState}, input, api)
	if err != nil || result.Status != "paired_terminal_completed" || fixture.tokenCalls != 1 {
		t.Fatalf("real paired entrypoint did not complete one handoff: result=%+v err=%v mints=%d calls=%v", result, err, fixture.tokenCalls, fixture.calls)
	}
	retryPath := filepath.Join(parent, "broker-input-retry.json")
	if err := os.WriteFile(retryPath, inputData, 0600); err != nil {
		t.Fatal("retry input file")
	}
	retryInput, err := os.Open(retryPath)
	if err != nil {
		t.Fatal("retry input open")
	}
	defer retryInput.Close()
	_, retryErr := runBrokerWithAPI(context.Background(), BrokerFiles{ApprovalPath: approvalPath, StateDirectory: attempt, ControllerBinary: binary.path, ControllerApproval: controllerPath, ControllerStateDirectory: controllerState, WorkerApproval: workerPath, WorkerStateDirectory: workerState}, retryInput, api)
	if retryErr == nil || fixture.tokenCalls != 1 {
		t.Fatalf("paired handoff replayed after a completed attempt: err=%v mints=%d", retryErr, fixture.tokenCalls)
	}
}
