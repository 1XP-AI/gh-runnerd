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
