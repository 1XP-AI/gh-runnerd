package enrollment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func TestBrokerExperimentRejectsLegacyZeroGroupPolicy(t *testing.T) {
	a := brokerApprovalFixture()
	a.Mode = "controller"
	a.Phase = "create"
	a.GroupPolicy = handoff.GroupPolicy{}
	a.ControllerHarnessSHA = strings.Repeat("e", 40)
	c := controllerApproval{GroupPolicy: a.GroupPolicy, AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: a.ControllerHarnessSHA, WorkflowSHA: strings.Repeat("b", 40), WorkflowRef: "refs/heads/main", WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create"}}
	raw, _ := json.Marshal(c)
	a.ControllerApprovalSHA256 = handoff.Digest(raw)
	req, err := brokerProvenanceRequest(a, c, workflowProvenanceSource)
	if err != nil {
		t.Fatal(err)
	}
	p := handoff.PublicPolicy{Version: 1, Role: "broker", Source: workflowProvenanceSource, WorkflowKeyID: "workflow-v1", WorkflowPublicKey: "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8", IssuerKeyID: "issuer-v1", IssuerPublicKey: "e28a8970753332bd72fef413e6b0b2ef1b4aadda7aa2c141f233712a6876b351", Grants: []handoff.PhaseGrant{{Request: req, Mode: "controller", OrganizationID: a.OrganizationID, RepositoryID: a.RepositoryID, ExpiresAt: a.ExpiresAt}}}
	policy, _ := json.Marshal(p)
	old := brokerExperimentPolicyJSON
	brokerExperimentPolicyJSON = string(policy)
	t.Cleanup(func() { brokerExperimentPolicyJSON = old })

	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal("private fixture")
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := BrokerFiles{ApprovalPath: filepath.Join(dir, "broker.json"), ControllerApproval: filepath.Join(dir, "controller.json")}
	approval, _ := json.Marshal(a)
	if err := os.WriteFile(files.ApprovalPath, approval, 0600); err != nil {
		t.Fatal("private broker approval fixture")
	}
	if err := os.WriteFile(files.ControllerApproval, raw, 0600); err != nil {
		t.Fatal("private controller approval fixture")
	}

	if _, err := selectBrokerExperiment(files, time.Now()); err == nil {
		t.Fatal("legacy v1 grant with omitted group policy selected for credential issuance")
	}
}
