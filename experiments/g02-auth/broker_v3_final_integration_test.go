package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

type brokerV3RunFixture struct {
	files       BrokerFiles
	candidate   Candidate
	fixture     *brokerHTTPFixture
	workflowGET *int
}

func newBrokerV3RunFixture(t *testing.T, phase string, changedCallee bool) brokerV3RunFixture {
	t.Helper()
	a, candidate, _, fixture, attempt := newBrokerFixture(t)
	a.Mode, a.Phase = "controller", phase
	a.GroupPolicy = groupPolicyFixture()
	a.AllowVerificationAuthority = phase == "after-ack"
	now := time.Now()
	a.ExpiresAt = now.Add(time.Hour)
	workflowSHA := strings.Repeat("b", 40)
	harnessSHA := strings.Repeat("c", 40)
	c := controllerApproval{GroupPolicy: a.GroupPolicy, AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: harnessSHA, WorkflowSHA: workflowSHA, WorkflowRef: "refs/heads/main", WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{phase}}
	a.ControllerHarnessSHA = harnessSHA
	binary := testBrokerBinary(t)
	a.ControllerBinarySHA256 = binary.digest
	controllerRaw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	a.ControllerApprovalSHA256 = handoff.Digest(controllerRaw)
	aRaw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(attempt)
	if err := os.Mkdir(attempt, 0700); err != nil {
		t.Fatal(err)
	}
	controllerState := filepath.Join(parent, "controller-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	approvalPath := filepath.Join(parent, "broker-approval.json")
	controllerPath := filepath.Join(parent, "controller-approval.json")
	if err := os.WriteFile(approvalPath, aRaw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controllerPath, controllerRaw, 0600); err != nil {
		t.Fatal(err)
	}

	workflowKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	issuerKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{71}, ed25519.SeedSize))
	workflowRoot, err := handoff.NewTrustRoot("workflow-v3-test", workflowKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	issuerRoot, err := handoff.NewIssuerRoot("issuer-v3-test", issuerKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	request, err := brokerProvenanceRequest(a, c, workflowProvenanceSource)
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

	reviewed := reviewedWorkflowGrant{Request: request, OrganizationID: a.OrganizationID, RepositoryID: a.RepositoryID, ExpiresAt: a.ExpiresAt}
	goodRun := strings.TrimSuffix(workflowRunJSON(t, reviewed), "}") + `,"referenced_workflows":[{"path":"` + request.Callee.Repository + "/" + request.Callee.WorkflowPath + "@" + request.Callee.SHA + `","sha":"` + request.Callee.SHA + `"}]}`
	badCallee := handoff.CalleeIdentity{Repository: request.Callee.Repository, WorkflowPath: ".github/workflows/changed.yml", SHA: strings.Repeat("e", 40)}
	badRun := strings.Replace(goodRun, request.Callee.Repository+"/"+request.Callee.WorkflowPath+"@"+request.Callee.SHA, badCallee.Repository+"/"+badCallee.WorkflowPath+"@"+badCallee.SHA, 1)
	badRun = strings.Replace(badRun, `"sha":"`+request.Callee.SHA+`"`, `"sha":"`+badCallee.SHA+`"`, 1)
	workflowGET := new(int)
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == workflowRunPath(request.Repository, request.WorkflowRunID) {
			*workflowGET = *workflowGET + 1
			body := goodRun
			if changedCallee && *workflowGET == 2 {
				body = badRun
			}
			return response(http.StatusOK, body), nil
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
	oldAPI := brokerExperimentFixtureAPI
	brokerExperimentFixtureAPI = func(root handoff.TrustRoot) *brokerAPI {
		api := newBrokerAPIWithProvenanceRoot(time.Now, transport, root)
		api.admissionDirectory = func() (string, error) { return fixture.admissionRoot, nil }
		return api
	}
	t.Cleanup(func() { brokerExperimentFixtureAPI = oldAPI })
	oldOpener := brokerBinaryOpener
	brokerBinaryOpener = func(string, BrokerApproval) (*verifiedBrokerBinary, error) {
		file, err := os.Open(binary.path)
		if err != nil {
			return nil, errBroker
		}
		return &verifiedBrokerBinary{path: binary.path, file: file, digest: binary.digest}, nil
	}
	t.Cleanup(func() { brokerBinaryOpener = oldOpener })
	return brokerV3RunFixture{files: BrokerFiles{ApprovalPath: approvalPath, StateDirectory: attempt, ControllerBinary: binary.path, ControllerApproval: controllerPath, ControllerStateDirectory: controllerState}, candidate: candidate, fixture: fixture, workflowGET: workflowGET}
}

func brokerV3Pipe(t *testing.T, data []byte) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		r.Close()
		w.Close()
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		r.Close()
		t.Fatal(err)
	}
	return r
}

func TestBrokerExperimentV3FinalGETFencesCalleeBeforeMintAndHandoff(t *testing.T) {
	for _, phase := range []string{"create", "inspect", "after-ack"} {
		for _, changedCallee := range []bool{true, false} {
			name := phase + "/"
			if changedCallee {
				name += "changed-callee"
			} else {
				name += "unchanged-callee"
			}
			t.Run(name, func(t *testing.T) {
				fixture := newBrokerV3RunFixture(t, phase, changedCallee)
				management, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
				if err != nil {
					t.Fatal(err)
				}
				observation := brokerV3Pipe(t, []byte("synthetic-private-workflow-token"))
				workflowSeed := brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize))
				issuerSeed := brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize))
				input := brokerV3Pipe(t, management)
				result, runErr := RunBrokerExperimentWithInputs(context.Background(), fixture.files, input, observation, workflowSeed, issuerSeed)
				if changedCallee {
					journal, readErr := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
					if readErr != nil {
						t.Fatal(readErr)
					}
					if runErr == nil || fixture.fixture.tokenCalls != 0 || *fixture.workflowGET != 2 || result.Status != "" || bytes.Contains(journal, []byte("token_request_started")) || bytes.Contains(journal, []byte("controller_handoff_started")) {
						t.Fatalf("changed callee crossed final fence: result=%+v err=%v mints=%d workflow_gets=%d journal=%s", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, journal)
					}
					return
				}
				if runErr != nil || result.Status != "controller_completed" || fixture.fixture.tokenCalls != 1 || *fixture.workflowGET != 2 {
					t.Fatalf("unchanged callee did not complete one handoff: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, fixture.fixture.calls)
				}
			})
		}
	}
}
