package enrollment

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// This is a policy-selection and malformed-front-door test. The complete
// public-entry v3 behavior, including successive workflow GETs, is covered by
// TestBrokerExperimentV3FinalGETFencesCalleeBeforeMintAndHandoff.
func TestBrokerExperimentV3SelectionRejectsMissingStateBeforePipeOwnership(t *testing.T) {
	a := brokerApprovalFixture()
	a.Mode, a.Phase = "controller", "create"
	a.GroupPolicy = groupPolicyFixture()
	a.ControllerHarnessSHA = strings.Repeat("e", 40)
	c := controllerApproval{GroupPolicy: a.GroupPolicy, AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: a.ControllerHarnessSHA, WorkflowSHA: strings.Repeat("b", 40), WorkflowRef: "refs/heads/main", WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create"}}
	raw, _ := json.Marshal(c)
	a.ControllerApprovalSHA256 = handoff.Digest(raw)
	req, err := brokerProvenanceRequest(a, c, workflowProvenanceSource)
	if err != nil {
		t.Fatal(err)
	}
	req.Version, req.Source = 2, handoff.ReusableCalleeSource
	req.Callee = handoff.CalleeIdentity{Repository: req.Repository, WorkflowPath: ".github/workflows/runner.yml", SHA: strings.Repeat("d", 40)}
	grant := handoff.PhaseGrant{Request: req, Mode: a.Mode, OrganizationID: a.OrganizationID, RepositoryID: a.RepositoryID, ExpiresAt: a.ExpiresAt}
	grant.GroupPolicySHA256 = a.GroupPolicy.GrantDigest(req, a.RepositoryID, a.RunnerGroupID, a.ManagementScope)
	p := handoff.PublicPolicy{Version: 3, Role: "broker", Source: handoff.ReusableCalleeSource, WorkflowKeyID: "workflow-v1", WorkflowPublicKey: "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8", IssuerKeyID: "issuer-v1", IssuerPublicKey: "e28a8970753332bd72fef413e6b0b2ef1b4aadda7aa2c141f233712a6876b351", Grants: []handoff.PhaseGrant{grant}}
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("chmod")
	}
	dir, _ = filepath.EvalSymlinks(dir)
	files := BrokerFiles{ApprovalPath: filepath.Join(dir, "broker.json"), ControllerApproval: filepath.Join(dir, "controller.json")}
	aRaw, _ := json.Marshal(a)
	if os.WriteFile(files.ApprovalPath, aRaw, 0600) != nil || os.WriteFile(files.ControllerApproval, raw, 0600) != nil {
		t.Fatal("write")
	}
	old := brokerExperimentPolicyJSON
	defer func() { brokerExperimentPolicyJSON = old }()
	setPolicy := func(p handoff.PublicPolicy) { b, _ := json.Marshal(p); brokerExperimentPolicyJSON = string(b) }
	setPolicy(p)
	selected, err := selectBrokerExperiment(files, time.Now())
	if err != nil || selected.grant.Request != req {
		t.Fatalf("independent selection failed: %v", err)
	}
	var inputs [4]*os.File
	var copies [4]*os.File
	for i := range inputs {
		r, w, e := os.Pipe()
		if e != nil {
			t.Fatal(e)
		}
		fd, e := syscall.Dup(int(r.Fd()))
		if e != nil {
			t.Fatal(e)
		}
		inputs[i], copies[i] = r, os.NewFile(uintptr(fd), "inspection")
		defer copies[i].Close()
		if _, e = w.Write([]byte("unconsumed")); e != nil {
			t.Fatal(e)
		}
		w.Close()
	}
	if _, err = RunBrokerExperimentWithInputs(context.Background(), files, inputs[0], inputs[1], inputs[2], inputs[3]); err == nil {
		t.Fatal("unintegrated v3 reached experiment")
	}
	for _, f := range copies {
		data, e := io.ReadAll(f)
		if e != nil || string(data) != "unconsumed" {
			t.Fatal("v3 rejection consumed input")
		}
	}
	for _, fault := range []string{"callee", "group", "run"} {
		t.Run(fault, func(t *testing.T) {
			q := p
			q.Grants = append([]handoff.PhaseGrant(nil), p.Grants...)
			switch fault {
			case "callee":
				q.Grants[0].Request.Callee.SHA = strings.Repeat("f", 40)
			case "group":
				q.Grants[0].GroupPolicySHA256 = strings.Repeat("f", 64)
			case "run":
				q.Grants[0].Request.WorkflowRunID++
			}
			setPolicy(q)
			if _, err := selectBrokerExperiment(files, time.Now()); err == nil {
				t.Fatal("mismatched policy selected")
			}
		})
	}
}
