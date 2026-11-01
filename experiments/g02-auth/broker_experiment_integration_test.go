package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func TestBrokerOfflineReusableCalleeReceiptReachesHandoff(t *testing.T) {
	a, candidate, _, fixture, attempt := newBrokerFixture(t)
	a.Mode, a.Phase = "controller", "inspect"
	a.GroupPolicy = groupPolicyFixture()
	plan := brokerTestPlan(t, &a, filepath.Dir(attempt), func(context.Context, []byte, string) error {
		return nil
	})
	plan.controller.WorkflowRunID = 7
	plan.controller.WorkflowRef = "refs/heads/main"
	plan.raw, _ = json.Marshal(plan.controller)
	a.ControllerApprovalSHA256 = handoff.Digest(plan.raw)
	plan.approval = a

	parent := filepath.Dir(attempt)
	approvalPath := filepath.Join(parent, "experiment-broker-approval.json")
	controllerPath := filepath.Join(parent, "experiment-controller-approval.json")
	approvalData, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(approvalPath, approvalData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controllerPath, plan.raw, 0600); err != nil {
		t.Fatal(err)
	}

	workflowKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	issuerKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{71}, ed25519.SeedSize))
	workflowRoot, err := handoff.NewTrustRoot("workflow-v2", workflowKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	issuerRoot, err := handoff.NewIssuerRoot("issuer-v2", issuerKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	request, err := brokerProvenanceRequest(a, plan.controller, workflowProvenanceSource)
	if err != nil {
		t.Fatal(err)
	}
	request.Version = 2
	request.Source = handoff.ReusableCalleeSource
	request.Callee = handoff.CalleeIdentity{Repository: request.Repository, WorkflowPath: ".github/workflows/runner.yml", SHA: strings.Repeat("d", 40)}
	grant := handoff.PhaseGrant{Request: request, Mode: a.Mode, OrganizationID: a.OrganizationID, RepositoryID: a.RepositoryID, ExpiresAt: a.ExpiresAt}
	grant.GroupPolicySHA256 = a.GroupPolicy.GrantDigest(request, a.RepositoryID, a.RunnerGroupID, a.ManagementScope)
	policy := handoff.PublicPolicy{Version: 3, Role: "broker", Source: handoff.ReusableCalleeSource, WorkflowKeyID: workflowRoot.KeyID(), WorkflowPublicKey: hexPublicKey(workflowKey.Public().(ed25519.PublicKey)), IssuerKeyID: issuerRoot.KeyID(), IssuerPublicKey: hexPublicKey(issuerKey.Public().(ed25519.PublicKey)), Grants: []handoff.PhaseGrant{grant}}
	policyData, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	oldPolicy := brokerExperimentPolicyJSON
	brokerExperimentPolicyJSON = string(policyData)
	t.Cleanup(func() { brokerExperimentPolicyJSON = oldPolicy })
	selected, err := selectBrokerExperiment(BrokerFiles{ApprovalPath: approvalPath, ControllerApproval: controllerPath}, time.Now())
	if err != nil || selected.grant.Request != request {
		t.Fatalf("independent broker grant selection failed: %v", err)
	}

	reviewed := reviewedWorkflowGrant{Request: selected.grant.Request, OrganizationID: selected.grant.OrganizationID, RepositoryID: selected.grant.RepositoryID, ExpiresAt: selected.grant.ExpiresAt}
	runJSON := workflowRunJSON(t, reviewed)
	runJSON = strings.TrimSuffix(runJSON, "}") + `,"referenced_workflows":[{"path":"org-a/canary/.github/workflows/runner.yml@` + request.Callee.SHA + `","sha":"` + request.Callee.SHA + `"}]}`
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == workflowRunPath(request.Repository, request.WorkflowRunID) {
			return response(http.StatusOK, runJSON), nil
		}
		if r.URL.Path == "/orgs/org-a/actions/runner-groups/3" {
			groupJSON, err := json.Marshal(map[string]any{
				"id":                         3,
				"name":                       "synthetic-group",
				"visibility":                 "selected",
				"default":                    false,
				"inherited":                  false,
				"allows_public_repositories": false,
				"restricted_to_workflows":    true,
				"selected_workflows":         []string{a.GroupPolicy.Workflow},
			})
			if err != nil {
				return nil, errBroker
			}
			return response(http.StatusOK, string(groupJSON)), nil
		}
		return fixture.RoundTrip(r)
	})
	api := newBrokerAPIWithProvenanceRoot(time.Now, transport, selected.workflow)
	api.admissionDirectory = func() (string, error) { return fixture.admissionRoot, nil }
	api.provenanceFactory = func(check func() error) (BrokerProvenanceAdapter, error) {
		return newWorkflowProvenanceAdapter(reviewed, newWorkflowRunObserver(transport), func(context.Context) (string, error) {
			return "synthetic-private-workflow-token", nil
		}, workflowSignerFunc(func(ctx context.Context, data []byte) ([]byte, error) {
			if ctx.Err() != nil {
				return nil, errBroker
			}
			return ed25519.Sign(workflowKey, data), nil
		}), selected.workflow.KeyID(), time.Now, nil, check)
	}

	journal, err := openBrokerJournal(attempt, a)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.close()
	provenancePointer, err := brokerWorkflowReceiptForRequest(context.Background(), api, a, plan.controller, a.ControllerApprovalSHA256, &request, journal)
	if err != nil || provenancePointer == nil || selected.workflow.Verify(selected.grant.Request, *provenancePointer, time.Now()) != nil {
		t.Fatalf("authenticated v2 receipt failed: %v", err)
	}
	provenance := *provenancePointer
	receiptData, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	launches := 0
	var handoffBytes []byte
	plan.receiptRaw = receiptData
	plan.handoff = &brokerHandoffAssembly{
		experimental: true,
		signer: workflowSignerFunc(func(ctx context.Context, data []byte) ([]byte, error) {
			if ctx.Err() != nil {
				return nil, errBroker
			}
			return ed25519.Sign(issuerKey, data), nil
		}),
		issuer:   selected.issuer,
		workflow: selected.workflow,
		request:  selected.grant.Request,
		grant:    reviewedWorkflowGrant{Request: selected.grant.Request, OrganizationID: selected.grant.OrganizationID, RepositoryID: selected.grant.RepositoryID, ExpiresAt: selected.grant.ExpiresAt},
		source:   selected.grant.Request.Source,
		launch: func(_ context.Context, h *issuedControllerHandoff, _ string) error {
			launches++
			handoffBytes = bytes.Clone(h.envelope)
			return nil
		},
	}
	if got, err := brokerHandoffRequest(a, plan, provenance.Source); err != nil || got != request {
		t.Fatalf("selected request rejected before execution: got=%+v err=%v want=%+v", got, err, request)
	}
	if !plan.handoff.valid(api.provenanceRoot) {
		t.Fatal("selected handoff assembly invalid before execution")
	}
	result, err := brokerExecuteWithProvenance(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-workflow-token"}, attempt, api, plan, &provenance, journal)
	if err != nil || result.Status != "controller_completed" || launches != 1 {
		t.Fatalf("offline v2 broker handoff failed: result=%+v err=%v launches=%d token=%d calls=%v", result, err, launches, fixture.tokenCalls, fixture.calls)
	}
	phase, err := handoff.DecodeStrictPhaseBinding(handoffBytes)
	if err != nil || phase.Request != request || phase.Request.Callee != request.Callee || phase.Version != 2 {
		t.Fatalf("handoff lost selected callee grant: phase=%+v err=%v", phase, err)
	}
	if fixture.tokenCalls != 1 {
		t.Fatalf("unexpected management token count: %d", fixture.tokenCalls)
	}
}

func hexPublicKey(key ed25519.PublicKey) string {
	return hex.EncodeToString(key)
}
