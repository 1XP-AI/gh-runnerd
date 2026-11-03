package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

type brokerV3RunFixture struct {
	files          BrokerFiles
	candidate      Candidate
	fixture        *brokerHTTPFixture
	workflowGET    *int
	changeCalleeAt *int
}

func newBrokerV3RunFixture(t *testing.T, phase string, changedCallee bool, rootMismatch ...bool) brokerV3RunFixture {
	t.Helper()
	a, candidate, _, fixture, attempt := newBrokerFixture(t)
	a.Mode, a.Phase = "controller", phase
	a.GroupPolicy = groupPolicyFixture()
	fixture.requireReadTokenLifecycle = true
	fixture.requireWorkflowGETBeforeMint = true
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
	controllerStateName := "controller-state"
	if len(rootMismatch) == 1 && rootMismatch[0] {
		controllerStateName = "root-mismatch-controller-state"
	}
	controllerState := filepath.Join(parent, controllerStateName)
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
	changeCalleeAt := new(int)
	if changedCallee {
		*changeCalleeAt = 2
	}
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == workflowRunPath(request.Repository, request.WorkflowRunID) {
			*workflowGET = *workflowGET + 1
			fixture.calls = append(fixture.calls, "GET "+r.URL.Path)
			fixture.events = append(fixture.events, "GET "+r.URL.Path)
			if fixture.loseWorkflowResponse && *workflowGET == 1 {
				return nil, errors.New("synthetic accepted workflow response lost")
			}
			if fixture.timeoutWorkflowResponse && *workflowGET == 1 {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			body := goodRun
			if *changeCalleeAt != 0 && *workflowGET == *changeCalleeAt {
				body = badRun
			}
			return response(http.StatusOK, body), nil
		}
		if r.URL.Path == "/orgs/org-a/actions/runner-groups/3" {
			fixture.events = append(fixture.events, "GET "+r.URL.Path)
			selectedWorkflows := []string{a.GroupPolicy.Workflow}
			if fixture.badGroupPolicy {
				selectedWorkflows = []string{"org-a/canary/.github/workflows/other.yml@" + strings.Repeat("e", 40)}
			}
			groupJSON, err := json.Marshal(map[string]any{
				"id":                         3,
				"name":                       "synthetic-group",
				"visibility":                 "selected",
				"default":                    false,
				"inherited":                  false,
				"allows_public_repositories": false,
				"restricted_to_workflows":    true,
				"selected_workflows":         selectedWorkflows,
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
	return brokerV3RunFixture{files: BrokerFiles{ApprovalPath: approvalPath, StateDirectory: attempt, ControllerBinary: binary.path, ControllerApproval: controllerPath, ControllerStateDirectory: controllerState}, candidate: candidate, fixture: fixture, workflowGET: workflowGET, changeCalleeAt: changeCalleeAt}
}

func brokerJournalSyncFailureContext(ctx context.Context, failedPhases ...string) context.Context {
	failed := make(map[string]bool, len(failedPhases))
	for _, phase := range failedPhases {
		failed[phase] = true
	}
	return context.WithValue(ctx, brokerJournalSyncFailureHookKey{}, brokerJournalSyncFailureHook(func(journal *brokerJournal, phase string) error {
		if failed[phase] {
			return errors.New("synthetic fsync failure")
		}
		return journal.file.Sync()
	}))
}

func brokerFixtureCallCount(calls []string, want string) int {
	count := 0
	for _, call := range calls {
		if call == want {
			count++
		}
	}
	return count
}

func TestBrokerExperimentV3ReadOnlyPreflightRejectsBeforeWriteMint(t *testing.T) {
	fixture := newBrokerV3RunFixture(t, "create", false)
	fixture.fixture.badGroupPolicy = true
	management, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
	if err != nil {
		t.Fatal(err)
	}
	input := brokerV3Pipe(t, management)
	observation := brokerV3Pipe(t, []byte("synthetic-private-workflow-token"))
	workflowSeed := brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize))
	issuerSeed := brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize))
	result, runErr := RunBrokerExperimentWithInputs(context.Background(), fixture.files, input, observation, workflowSeed, issuerSeed)
	if runErr == nil || result.Status != "" || fixture.fixture.tokenCalls != 1 || *fixture.workflowGET != 1 {
		t.Fatalf("ineligible group crossed read-only preflight: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, fixture.fixture.calls)
	}
	wantPermissions := map[string]string{"metadata": "read", "organization_self_hosted_runners": "read"}
	if len(fixture.fixture.mintRequests) != 1 || !reflect.DeepEqual(fixture.fixture.mintRequests[0].RepositoryIDs, []int64{501}) || !reflect.DeepEqual(fixture.fixture.mintRequests[0].Permissions, wantPermissions) {
		t.Fatalf("ineligible group did not stop after an exact read-only token: requests=%+v", fixture.fixture.mintRequests)
	}
	if !reflect.DeepEqual(fixture.fixture.calls[len(fixture.fixture.calls)-1:], []string{"DELETE /installation/token"}) {
		t.Fatalf("read-only token was not revoked after rejected provider preflight: %v", fixture.fixture.calls)
	}
	if got := brokerFixtureCallCount(fixture.fixture.calls, "DELETE /installation/token"); got != 1 {
		t.Fatalf("read-only token DELETE requests=%d, want exactly one: %v", got, fixture.fixture.calls)
	}
	journal, err := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(journal, []byte("management_token_request_started")) || bytes.Contains(journal, []byte("controller_handoff_started")) {
		t.Fatalf("ineligible group crossed write-token or handoff journal boundary: %s", journal)
	}
}

func TestBrokerExperimentV3StopsWhenReadOnlyTokenRevocationIsUncertain(t *testing.T) {
	fixture := newBrokerV3RunFixture(t, "create", false)
	fixture.fixture.revokeError = true
	management, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
	if err != nil {
		t.Fatal(err)
	}
	input := brokerV3Pipe(t, management)
	observation := brokerV3Pipe(t, []byte("synthetic-private-workflow-token"))
	workflowSeed := brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize))
	issuerSeed := brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize))
	result, runErr := RunBrokerExperimentWithInputs(context.Background(), fixture.files, input, observation, workflowSeed, issuerSeed)
	if runErr == nil || result.Status != "" || fixture.fixture.tokenCalls != 1 || *fixture.workflowGET != 1 {
		t.Fatalf("uncertain read-only-token revocation crossed the write boundary: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, fixture.fixture.calls)
	}
	if !reflect.DeepEqual(fixture.fixture.calls[len(fixture.fixture.calls)-1:], []string{"DELETE /installation/token"}) {
		t.Fatalf("read-only installation token was not revoked as the final API operation: %v", fixture.fixture.calls)
	}
	if got := brokerFixtureCallCount(fixture.fixture.calls, "DELETE /installation/token"); got != 1 {
		t.Fatalf("read-only token DELETE requests=%d, want exactly one: %v", got, fixture.fixture.calls)
	}
	journal, err := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(journal, []byte("read_only_token_revocation_uncertain")) || bytes.Contains(journal, []byte("management_token_request_started")) || bytes.Contains(journal, []byte("controller_handoff_started")) {
		t.Fatalf("uncertain revocation was not durably fenced before write mint: %s", journal)
	}
	if bytes.Contains(journal, []byte("synthetic-private-read-installation-token")) {
		t.Fatal("read-only installation token was persisted in the broker journal")
	}
}

func TestBrokerExperimentV3RevokedRecordSyncFailureStopsBeforeWriteMint(t *testing.T) {
	fixture := newBrokerV3RunFixture(t, "create", false)
	management, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := brokerJournalSyncFailureContext(context.Background(), "read_only_token_revoked")
	result, runErr := RunBrokerExperimentWithInputs(
		ctx, fixture.files,
		brokerV3Pipe(t, management),
		brokerV3Pipe(t, []byte("synthetic-private-workflow-token")),
		brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize)),
		brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize)),
	)
	if runErr == nil || result.Status != "" || fixture.fixture.tokenCalls != 1 || *fixture.workflowGET != 1 {
		t.Fatalf("revoked-record sync failure crossed the write boundary: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, fixture.fixture.calls)
	}
	if got := brokerFixtureCallCount(fixture.fixture.calls, "DELETE /installation/token"); got != 1 {
		t.Fatalf("read-only token DELETE requests=%d, want exactly one: %v", got, fixture.fixture.calls)
	}
	if len(fixture.fixture.calls) == 0 || fixture.fixture.calls[len(fixture.fixture.calls)-1] != "DELETE /installation/token" {
		t.Fatalf("revocation was not the last provider operation: %v", fixture.fixture.calls)
	}
	journal, err := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(journal)
	started := strings.Index(text, "read_only_token_revocation_started")
	revoked := strings.Index(text, "read_only_token_revoked")
	uncertain := strings.Index(text, "read_only_token_revocation_uncertain")
	if started < 0 || revoked <= started || uncertain <= revoked || strings.Contains(text, "management_token_request_started") || strings.Contains(text, "controller_handoff_started") {
		t.Fatalf("sync failure was not fenced by a terminal uncertainty record before writes: %s", text)
	}
	if bytes.Contains(journal, []byte("synthetic-private-read-installation-token")) {
		t.Fatal("read-only installation token was persisted in the broker journal")
	}
}

func TestBrokerExperimentV3DoubleRevocationJournalSyncFailureBlocksLaterInspect(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	_, runErr := executeBrokerV3Fixture(t,
		brokerJournalSyncFailureContext(context.Background(), "read_only_token_revoked", "read_only_token_revocation_uncertain"),
		create,
	)
	if runErr == nil || create.fixture.tokenCalls != 1 || brokerFixtureCallCount(create.fixture.calls, "DELETE /installation/token") != 1 {
		t.Fatalf("compound journal fault did not stop after one read-token DELETE: err=%v mints=%d calls=%v", runErr, create.fixture.tokenCalls, create.fixture.calls)
	}
	admission, err := os.ReadFile(filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !bytes.Contains(admission, []byte(`"kind":"read_token_lifecycle_started"`)) || bytes.Contains(admission, []byte(`"kind":"read_token_lifecycle_confirmed"`)) {
		t.Fatalf("compound journal fault did not leave an unresolved account-level lifecycle intent: err=%v", err)
	}

	inspect := newBrokerV3RunFixture(t, "inspect", false)
	inspect.fixture.admissionRoot = create.fixture.admissionRoot
	inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
	result, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
	if inspectErr == nil || result.Status != "" || inspect.fixture.tokenCalls != 0 || brokerFixtureCallCount(inspect.fixture.calls, "DELETE /installation/token") != 0 {
		t.Fatalf("later inspect crossed an unresolved read-token revocation: result=%+v err=%v mints=%d calls=%v", result, inspectErr, inspect.fixture.tokenCalls, inspect.fixture.calls)
	}
	for _, call := range inspect.fixture.calls {
		if strings.HasPrefix(call, "POST /app/installations/") {
			t.Fatalf("later inspect minted a token after unresolved revocation: %v", inspect.fixture.calls)
		}
	}
}

func TestBrokerExperimentV3LostReadTokenMintResponseBlocksLaterInspect(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	create.fixture.loseReadTokenResponse = true
	_, createErr := executeBrokerV3Fixture(t, context.Background(), create)
	if createErr == nil || create.fixture.tokenCalls != 1 || !create.fixture.readTokenAccepted || brokerFixtureCallCount(create.fixture.calls, "DELETE /installation/token") != 0 {
		t.Fatalf("fixture did not model an accepted read-token mint with a lost response: err=%v mints=%d accepted=%t calls=%v", createErr, create.fixture.tokenCalls, create.fixture.readTokenAccepted, create.fixture.calls)
	}
	admission, err := os.ReadFile(filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl"))
	started := bytes.Contains(admission, []byte(`"kind":"read_token_lifecycle_started"`))
	confirmed := bytes.Contains(admission, []byte(`"kind":"read_token_lifecycle_confirmed"`))
	if err != nil || !started || confirmed {
		t.Errorf("lost response account-level lifecycle state: err=%v started=%t confirmed=%t", err, started, confirmed)
	}

	inspect := newBrokerV3RunFixture(t, "inspect", false)
	inspect.fixture.admissionRoot = create.fixture.admissionRoot
	inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
	result, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
	if inspectErr == nil || result.Status != "" || inspect.fixture.tokenCalls != 0 || len(inspect.fixture.calls) != 0 {
		t.Fatalf("later inspect crossed an unresolved read-token issuance: result=%+v err=%v mints=%d calls=%v", result, inspectErr, inspect.fixture.tokenCalls, inspect.fixture.calls)
	}
}

func TestBrokerExperimentV3LostWorkflowProvenanceResponseBlocksLaterInspect(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	create.fixture.loseWorkflowResponse = true
	_, createErr := executeBrokerV3Fixture(t, context.Background(), create)
	if createErr == nil || create.fixture.tokenCalls != 0 || *create.workflowGET != 1 {
		t.Fatalf("fixture did not model a lost workflow provenance response before mint: err=%v mints=%d workflow_gets=%d calls=%v", createErr, create.fixture.tokenCalls, *create.workflowGET, create.fixture.calls)
	}
	if len(create.fixture.events) == 0 || create.fixture.events[0] != "GET /repos/org-a/canary/actions/runs/7" {
		t.Fatalf("workflow provenance GET was not causally observed: events=%v", create.fixture.events)
	}
	admission, err := os.ReadFile(filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !bytes.Contains(admission, []byte(`"kind":"workflow_provenance_get_started"`)) || bytes.Contains(admission, []byte(`"kind":"workflow_provenance_get_confirmed"`)) {
		t.Fatalf("lost response did not leave an unresolved workflow provenance intent: err=%v ledger=%s", err, admission)
	}

	inspect := newBrokerV3RunFixture(t, "inspect", false)
	inspect.fixture.admissionRoot = create.fixture.admissionRoot
	inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
	result, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
	if inspectErr == nil || result.Status != "" || inspect.fixture.tokenCalls != 0 || len(inspect.fixture.calls) != 0 {
		t.Fatalf("later inspect crossed unresolved workflow provenance: result=%+v err=%v mints=%d calls=%v", result, inspectErr, inspect.fixture.tokenCalls, inspect.fixture.calls)
	}
}

func TestBrokerExperimentV3WorkflowProvenanceTimeoutBlocksLaterInspect(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	create.fixture.timeoutWorkflowResponse = true
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, createErr := executeBrokerV3Fixture(t, ctx, create)
	if createErr == nil || create.fixture.tokenCalls != 0 || *create.workflowGET != 1 {
		t.Fatalf("workflow provenance timeout crossed the mint fence: err=%v mints=%d workflow_gets=%d calls=%v", createErr, create.fixture.tokenCalls, *create.workflowGET, create.fixture.calls)
	}
	admission, err := os.ReadFile(filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !bytes.Contains(admission, []byte(`"kind":"workflow_provenance_get_started"`)) || bytes.Contains(admission, []byte(`"kind":"workflow_provenance_get_confirmed"`)) {
		t.Fatalf("timeout did not leave an unresolved workflow provenance intent: err=%v ledger=%s", err, admission)
	}

	inspect := newBrokerV3RunFixture(t, "inspect", false)
	inspect.fixture.admissionRoot = create.fixture.admissionRoot
	inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
	result, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
	if inspectErr == nil || result.Status != "" || inspect.fixture.tokenCalls != 0 || len(inspect.fixture.calls) != 0 {
		t.Fatalf("later inspect crossed timeout-ambiguous workflow provenance: result=%+v err=%v mints=%d calls=%v", result, inspectErr, inspect.fixture.tokenCalls, inspect.fixture.calls)
	}
}

func TestBrokerExperimentV3WorkflowProvenancePreRequestFailureRecordsNoDispatchForLaterInspect(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	_, createErr := executeBrokerV3Fixture(t, brokerJournalSyncFailureContext(context.Background(), "workflow_provenance_get_started"), create)
	if createErr == nil || create.fixture.tokenCalls != 0 || *create.workflowGET != 0 || len(create.fixture.calls) != 0 {
		t.Fatalf("workflow provenance journal failure crossed the HTTP boundary: err=%v mints=%d workflow_gets=%d calls=%v", createErr, create.fixture.tokenCalls, *create.workflowGET, create.fixture.calls)
	}
	admission, err := os.ReadFile(filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !bytes.Contains(admission, []byte(`"kind":"workflow_provenance_get_not_dispatched"`)) || bytes.Contains(admission, []byte(`"kind":"workflow_provenance_get_started"`)) {
		t.Fatalf("pre-request failure lacks an explicit no-dispatch outcome: err=%v ledger=%s", err, admission)
	}

	inspect := newBrokerV3RunFixture(t, "inspect", false)
	inspect.fixture.admissionRoot = create.fixture.admissionRoot
	inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
	result, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
	if inspectErr != nil || result.Status != "controller_completed" || inspect.fixture.tokenCalls != 2 || *inspect.workflowGET != 3 {
		t.Fatalf("explicit no-dispatch proof did not permit later inspect: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", result, inspectErr, inspect.fixture.tokenCalls, *inspect.workflowGET, inspect.fixture.calls)
	}
}

func TestBrokerExperimentV3HistoricalWorkflowProvenanceClaimReplayBinding(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	if _, err := executeBrokerV3Fixture(t, context.Background(), create); err != nil {
		t.Fatalf("complete synthetic create: %v", err)
	}
	ledgerPath := filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl")
	ledger, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read admission ledger: %v", err)
	}
	mutateLedger := func(mutate func(*brokerWorkflowProvenanceEvent)) []byte {
		t.Helper()
		lines := strings.Split(strings.TrimSuffix(string(ledger), "\n"), "\n")
		mutated := 0
		for i, line := range lines {
			var kind struct {
				Kind string `json:"kind"`
			}
			if decodeBrokerJSON([]byte(line), &kind, false) != nil {
				t.Fatal("decode admission event kind")
			}
			if kind.Kind != "workflow_provenance_get_started" && kind.Kind != "workflow_provenance_get_confirmed" {
				continue
			}
			var event brokerWorkflowProvenanceEvent
			if decodeBrokerJSON([]byte(line), &event, true) != nil {
				t.Fatal("decode workflow provenance event")
			}
			mutate(&event)
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("encode changed workflow provenance event: %v", err)
			}
			lines[i] = string(encoded)
			mutated++
		}
		if mutated != 2 {
			t.Fatalf("mutated %d workflow provenance lifecycle events, want start and confirmation", mutated)
		}
		return []byte(strings.Join(lines, "\n") + "\n")
	}

	cases := []struct {
		name   string
		mutate func(*brokerWorkflowProvenanceEvent)
	}{
		{name: "slot", mutate: func(event *brokerWorkflowProvenanceEvent) { event.Slot = "cleanup" }},
		{name: "attempt", mutate: func(event *brokerWorkflowProvenanceEvent) { event.Attempt.Inode++ }},
		{name: "journal", mutate: func(event *brokerWorkflowProvenanceEvent) { event.Journal.Inode++ }},
		{name: "controller-approval-digest", mutate: func(event *brokerWorkflowProvenanceEvent) { event.ControllerApprovalSHA256 = strings.Repeat("e", 64) }},
		{name: "source", mutate: func(event *brokerWorkflowProvenanceEvent) { event.Source = "other-source" }},
		{name: "workflow-run-id", mutate: func(event *brokerWorkflowProvenanceEvent) { event.WorkflowRunID++ }},
		{name: "workflow-ref", mutate: func(event *brokerWorkflowProvenanceEvent) { event.WorkflowRef = "refs/heads/release" }},
		{name: "workflow-sha", mutate: func(event *brokerWorkflowProvenanceEvent) { event.WorkflowSHA = strings.Repeat("f", 40) }},
		{name: "workflow-path", mutate: func(event *brokerWorkflowProvenanceEvent) { event.WorkflowPath = ".github/workflows/other.yml" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(ledgerPath, mutateLedger(tc.mutate), 0600); err != nil {
				t.Fatalf("write mismatched workflow provenance history: %v", err)
			}
			inspect := newBrokerV3RunFixture(t, "inspect", false)
			inspect.fixture.admissionRoot = create.fixture.admissionRoot
			inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
			result, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
			if inspectErr == nil || result.Status != "" || inspect.fixture.tokenCalls != 0 || *inspect.workflowGET != 0 || len(inspect.fixture.calls) != 0 {
				t.Fatalf("inspect crossed mismatched historical %s: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", tc.name, result, inspectErr, inspect.fixture.tokenCalls, *inspect.workflowGET, inspect.fixture.calls)
			}
		})
	}

	if err := os.WriteFile(ledgerPath, ledger, 0600); err != nil {
		t.Fatalf("restore matching workflow provenance history: %v", err)
	}
	t.Run("matching-history", func(t *testing.T) {
		inspect := newBrokerV3RunFixture(t, "inspect", false)
		inspect.fixture.admissionRoot = create.fixture.admissionRoot
		inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
		result, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
		if inspectErr != nil || result.Status != "controller_completed" || inspect.fixture.tokenCalls != 2 || *inspect.workflowGET != 3 {
			t.Fatalf("inspect rejected matching historical workflow identity: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", result, inspectErr, inspect.fixture.tokenCalls, *inspect.workflowGET, inspect.fixture.calls)
		}
	})
}

func TestBrokerExperimentV3LegacyIncompleteClaimWithoutLifecycleBlocksLaterInspect(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	if _, err := executeBrokerV3Fixture(t, context.Background(), create); err != nil {
		t.Fatalf("complete synthetic v3 create: %v", err)
	}
	ledgerPath := filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl")
	ledger, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read synthetic admission ledger: %v", err)
	}
	var legacyLines []string
	for _, line := range strings.Split(strings.TrimSuffix(string(ledger), "\n"), "\n") {
		if strings.Contains(line, `"kind":"read_token_lifecycle_`) || strings.Contains(line, `"kind":"workflow_provenance_get_`) || strings.Contains(line, `"kind":"complete"`) {
			continue
		}
		legacyLines = append(legacyLines, line)
	}
	if len(legacyLines) != 2 {
		t.Fatalf("expected a legacy header plus one incomplete claim, got %d ledger lines", len(legacyLines))
	}
	if err := os.WriteFile(ledgerPath, []byte(strings.Join(legacyLines, "\n")+"\n"), 0600); err != nil {
		t.Fatalf("write synthetic legacy incomplete ledger: %v", err)
	}

	inspect := newBrokerV3RunFixture(t, "inspect", false)
	inspect.fixture.admissionRoot = create.fixture.admissionRoot
	inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
	result, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
	if inspectErr == nil || result.Status != "" || inspect.fixture.tokenCalls != 0 || len(inspect.fixture.calls) != 0 {
		t.Fatalf("legacy incomplete reusable-callee claim without lifecycle proof was recovered: result=%+v err=%v mints=%d calls=%v", result, inspectErr, inspect.fixture.tokenCalls, inspect.fixture.calls)
	}
}

func TestBrokerExperimentV3PreRequestFailureRecordsNoDispatchForLaterInspect(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	_, createErr := executeBrokerV3Fixture(t, brokerJournalSyncFailureContext(context.Background(), "token_request_started"), create)
	if createErr == nil || create.fixture.tokenCalls != 0 || brokerFixtureCallCount(create.fixture.calls, "POST /app/installations/201/access_tokens") != 0 {
		t.Fatalf("token-request journal failure reached issuance: err=%v mints=%d calls=%v", createErr, create.fixture.tokenCalls, create.fixture.calls)
	}
	admission, err := os.ReadFile(filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !bytes.Contains(admission, []byte(`"kind":"read_token_request_not_dispatched"`)) {
		t.Fatalf("pre-request failure lacks durable no-dispatch proof: err=%v", err)
	}

	inspect := newBrokerV3RunFixture(t, "inspect", false)
	inspect.fixture.admissionRoot = create.fixture.admissionRoot
	inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
	result, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
	if inspectErr != nil || result.Status != "controller_completed" || inspect.fixture.tokenCalls != 2 {
		t.Fatalf("durable no-dispatch proof failed to permit later inspect: result=%+v err=%v mints=%d calls=%v", result, inspectErr, inspect.fixture.tokenCalls, inspect.fixture.calls)
	}
}

func executeBrokerV3Fixture(t *testing.T, ctx context.Context, fixture brokerV3RunFixture) (BrokerResult, error) {
	t.Helper()
	management, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
	if err != nil {
		t.Fatal(err)
	}
	return RunBrokerExperimentWithInputs(
		ctx, fixture.files,
		brokerV3Pipe(t, management),
		brokerV3Pipe(t, []byte("synthetic-private-workflow-token")),
		brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize)),
		brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize)),
	)
}

func TestBrokerExperimentV3RevokesTokenFromRejectedReadIssuance(t *testing.T) {
	for _, fault := range []string{"write-scope", "expired"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newBrokerV3RunFixture(t, "create", false)
			fixture.fixture.readTokenFault = fault
			management, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
			if err != nil {
				t.Fatal(err)
			}
			input := brokerV3Pipe(t, management)
			observation := brokerV3Pipe(t, []byte("synthetic-private-workflow-token"))
			workflowSeed := brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize))
			issuerSeed := brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize))
			result, runErr := RunBrokerExperimentWithInputs(context.Background(), fixture.files, input, observation, workflowSeed, issuerSeed)
			if runErr == nil || result.Status != "" || fixture.fixture.tokenCalls != 1 || *fixture.workflowGET != 1 {
				t.Fatalf("rejected read-token issuance crossed its initial workflow gate: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, fixture.fixture.calls)
			}
			if len(fixture.fixture.calls) == 0 || fixture.fixture.calls[len(fixture.fixture.calls)-1] != "DELETE /installation/token" {
				t.Fatalf("usable rejected token was not revoked: calls=%v", fixture.fixture.calls)
			}
			if got := brokerFixtureCallCount(fixture.fixture.calls, "DELETE /installation/token"); got != 1 {
				t.Fatalf("read-only token DELETE requests=%d, want exactly one: %v", got, fixture.fixture.calls)
			}
			for _, call := range fixture.fixture.calls {
				if call == "GET /installation/repositories" || strings.HasPrefix(call, "GET /orgs/org-a/actions/runner-groups/") {
					t.Fatalf("rejected token was used for provider preflight: calls=%v", fixture.fixture.calls)
				}
			}
			journal, err := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(journal, []byte("read_only_token_revocation_started")) || !bytes.Contains(journal, []byte("read_only_token_revoked")) || bytes.Contains(journal, []byte("read_only_token_verified")) || bytes.Contains(journal, []byte("management_token_request_started")) || bytes.Contains(journal, []byte("controller_handoff_started")) {
				t.Fatalf("rejected issuance cleanup crossed an invalid journal boundary: %s", journal)
			}
			if bytes.Contains(journal, []byte("synthetic-private-read-installation-token")) {
				t.Fatal("rejected read-token candidate was persisted in the broker journal")
			}
		})
	}
}

func TestBrokerExperimentV3DoesNotRevokeVerificationTokenCollision(t *testing.T) {
	fixture := newBrokerV3RunFixture(t, "create", false)
	fixture.fixture.readTokenToken = "synthetic-private-workflow-token"
	management, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
	if err != nil {
		t.Fatal(err)
	}
	input := brokerV3Pipe(t, management)
	observation := brokerV3Pipe(t, []byte("synthetic-private-workflow-token"))
	workflowSeed := brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize))
	issuerSeed := brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize))
	result, runErr := RunBrokerExperimentWithInputs(context.Background(), fixture.files, input, observation, workflowSeed, issuerSeed)
	if runErr == nil || result.Status != "" || fixture.fixture.tokenCalls != 1 || *fixture.workflowGET != 1 {
		t.Fatalf("verification-token collision was not rejected before use: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, fixture.fixture.calls)
	}
	for _, call := range fixture.fixture.calls {
		if call == "DELETE /installation/token" || call == "GET /installation/repositories" || strings.HasPrefix(call, "GET /orgs/org-a/actions/runner-groups/") {
			t.Fatalf("caller verification token was used or revoked: calls=%v", fixture.fixture.calls)
		}
	}
	journal, err := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(journal, []byte("read_only_token_revocation_skipped")) || !bytes.Contains(journal, []byte("verification_token_collision")) || bytes.Contains(journal, []byte("read_only_token_revocation_started")) {
		t.Fatalf("verification-token collision outcome was not durably recorded without a revoke attempt: %s", journal)
	}
	if bytes.Contains(journal, []byte("synthetic-private-workflow-token")) {
		t.Fatal("caller verification token was persisted in the broker journal")
	}
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

func brokerV3PipeWithCopy(t *testing.T, data []byte) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Dup(int(r.Fd()))
	if err != nil {
		r.Close()
		w.Close()
		t.Fatal(err)
	}
	copy := os.NewFile(uintptr(fd), "broker-v3-input-probe")
	if copy == nil {
		r.Close()
		w.Close()
		syscall.Close(fd)
		t.Fatal("duplicate input pipe")
	}
	if _, err := w.Write(data); err != nil {
		r.Close()
		copy.Close()
		w.Close()
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		r.Close()
		copy.Close()
		t.Fatal(err)
	}
	return r, copy
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
					if runErr == nil || fixture.fixture.tokenCalls != 1 || *fixture.workflowGET != 2 || result.Status != "" || !bytes.Contains(journal, []byte("token_request_started")) || !bytes.Contains(journal, []byte("read_only_token_verified")) || bytes.Contains(journal, []byte("management_token_request_started")) || bytes.Contains(journal, []byte("controller_handoff_started")) {
						t.Fatalf("changed callee crossed final fence: result=%+v err=%v mints=%d workflow_gets=%d journal=%s", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, journal)
					}
					return
				}
				if runErr != nil || result.Status != "controller_completed" || fixture.fixture.tokenCalls != 2 || *fixture.workflowGET != 3 {
					t.Fatalf("unchanged callee did not complete one handoff: result=%+v err=%v mints=%d workflow_gets=%d calls=%v", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, fixture.fixture.calls)
				}
				wantRead := map[string]string{"metadata": "read", "organization_self_hosted_runners": "read"}
				wantWrite := map[string]string{"metadata": "read", "organization_self_hosted_runners": "write"}
				if len(fixture.fixture.mintRequests) != 2 || !reflect.DeepEqual(fixture.fixture.mintRequests[0].Permissions, wantRead) || !reflect.DeepEqual(fixture.fixture.mintRequests[1].Permissions, wantWrite) {
					t.Fatalf("v3 token sequence did not minimize authority before final write: %+v", fixture.fixture.mintRequests)
				}
				wantEvents := []string{
					"POST /app/installations/201/access_tokens",
					"GET /installation/repositories",
					"GET /repos/org-a/canary",
					"GET /orgs/org-a/actions/runner-groups/3",
					"GET /orgs/org-a/actions/runner-groups/3/repositories",
					"DELETE /installation/token",
					"GET /repos/org-a/canary/actions/runs/7",
					"POST /app/installations/201/access_tokens",
					"GET /installation/repositories",
					"GET /repos/org-a/canary",
					"GET /orgs/org-a/actions/runner-groups/3",
					"GET /orgs/org-a/actions/runner-groups/3/repositories",
					"GET /repos/org-a/canary/actions/runs/7",
				}
				at := 0
				for _, event := range fixture.fixture.events {
					if at < len(wantEvents) && event == wantEvents[at] {
						at++
					}
				}
				if at != len(wantEvents) {
					t.Fatalf("expected ordered read preflight, final workflow fence, write mint and write preflight; events=%v", fixture.fixture.events)
				}
			})
		}
	}
}

func TestBrokerExperimentV3RepeatsCalleeFenceAfterWritePreflight(t *testing.T) {
	fixture := newBrokerV3RunFixture(t, "create", false)
	*fixture.changeCalleeAt = 3
	management, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
	if err != nil {
		t.Fatal(err)
	}
	observation := brokerV3Pipe(t, []byte("synthetic-private-workflow-token"))
	workflowSeed := brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize))
	issuerSeed := brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize))
	input := brokerV3Pipe(t, management)
	result, runErr := RunBrokerExperimentWithInputs(context.Background(), fixture.files, input, observation, workflowSeed, issuerSeed)
	journal, readErr := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if runErr == nil || result.Status != "" || fixture.fixture.tokenCalls != 2 || *fixture.workflowGET != 3 || bytes.Contains(journal, []byte("controller_handoff_started")) {
		t.Fatalf("callee change after write preflight crossed handoff fence: result=%+v err=%v mints=%d workflow_gets=%d journal=%s", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET, journal)
	}
}

func TestBrokerExperimentV3ControllerRootMismatchRefusesBeforeManagementInputAndMint(t *testing.T) {
	fixture := newBrokerV3RunFixture(t, "create", false, true)
	management, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
	if err != nil {
		t.Fatal(err)
	}
	observation := brokerV3Pipe(t, []byte("synthetic-private-workflow-token"))
	workflowSeed := brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize))
	issuerSeed := brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize))
	input := brokerV3Pipe(t, management)
	result, runErr := RunBrokerExperimentWithInputs(context.Background(), fixture.files, input, observation, workflowSeed, issuerSeed)
	journal, readErr := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if runErr == nil || fixture.fixture.tokenCalls != 0 || result.Status != "" || bytes.Contains(journal, []byte("token_request_started")) || bytes.Contains(journal, []byte("controller_handoff_started")) {
		t.Fatalf("root mismatch crossed management fence: result=%+v err=%v mints=%d journal=%s", result, runErr, fixture.fixture.tokenCalls, journal)
	}
}

func TestBrokerExperimentV3CredentialInputFailureConsumesAttempt(t *testing.T) {
	cases := []struct {
		name  string
		input func(t *testing.T, candidate Candidate) []byte
	}{
		{
			name: "malformed-json",
			input: func(*testing.T, Candidate) []byte {
				return []byte(`{"pem":`)
			},
		},
		{
			name: "oversized-pem",
			input: func(t *testing.T, candidate Candidate) []byte {
				data, err := json.Marshal(brokerInput{PEM: strings.Repeat("x", 32769), VerificationToken: "synthetic-private-workflow-token"})
				if err != nil {
					t.Fatal(err)
				}
				return data
			},
		},
		{
			name: "malformed-pem",
			input: func(t *testing.T, _ Candidate) []byte {
				data, err := json.Marshal(brokerInput{PEM: "not a private key", VerificationToken: "synthetic-private-workflow-token"})
				if err != nil {
					t.Fatal(err)
				}
				return data
			},
		},
		{
			name: "missing-verification-token",
			input: func(t *testing.T, candidate Candidate) []byte {
				data, err := json.Marshal(brokerInput{PEM: string(candidate.PEM)})
				if err != nil {
					t.Fatal(err)
				}
				return data
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newBrokerV3RunFixture(t, "create", false)
			observation := brokerV3Pipe(t, []byte("synthetic-private-workflow-token"))
			workflowSeed := brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize))
			issuerSeed := brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize))
			input := brokerV3Pipe(t, tc.input(t, fixture.candidate))
			result, runErr := RunBrokerExperimentWithInputs(context.Background(), fixture.files, input, observation, workflowSeed, issuerSeed)
			journalPath := filepath.Join(fixture.files.StateDirectory, "broker.jsonl")
			journal, readErr := os.ReadFile(journalPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if runErr == nil || result.Status != "" || fixture.fixture.tokenCalls != 0 || *fixture.workflowGET != 1 {
				t.Fatalf("invalid v3 management input crossed effects: result=%+v err=%v mints=%d workflow_gets=%d", result, runErr, fixture.fixture.tokenCalls, *fixture.workflowGET)
			}
			for _, phase := range [][]byte{
				[]byte(`"controller_preparation_started"`),
				[]byte(`"controller_state_prepared"`),
				[]byte(`"management_input_rejected"`),
			} {
				if !bytes.Contains(journal, phase) {
					t.Fatalf("failed attempt journal lacks %s: %s", phase, journal)
				}
			}
			if bytes.Contains(journal, []byte(`"token_request_started"`)) || bytes.Contains(journal, []byte(`"controller_handoff_started"`)) {
				t.Fatalf("invalid v3 management input crossed mint/handoff: %s", journal)
			}
			ledger, readErr := os.ReadFile(filepath.Join(fixture.fixture.admissionRoot, "broker-admission.jsonl"))
			if readErr != nil || !bytes.Contains(ledger, []byte(`"kind":"claim"`)) {
				t.Fatalf("trusted input failure did not consume the durable phase slot: err=%v ledger=%s", readErr, ledger)
			}

			validInput, err := json.Marshal(brokerInput{PEM: string(fixture.candidate.PEM), VerificationToken: "synthetic-private-workflow-token"})
			if err != nil {
				t.Fatal(err)
			}
			retryInput, retryInputCopy := brokerV3PipeWithCopy(t, validInput)
			retryObservation := brokerV3Pipe(t, []byte("synthetic-private-workflow-token"))
			retryWorkflowSeed := brokerV3Pipe(t, bytes.Repeat([]byte{1}, ed25519.SeedSize))
			retryIssuerSeed := brokerV3Pipe(t, bytes.Repeat([]byte{71}, ed25519.SeedSize))
			retryResult, retryErr := RunBrokerExperimentWithInputs(context.Background(), fixture.files, retryInput, retryObservation, retryWorkflowSeed, retryIssuerSeed)
			defer retryInputCopy.Close()
			journalAfterRetry, readErr := os.ReadFile(journalPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			unconsumed, readErr := io.ReadAll(retryInputCopy)
			if retryErr == nil || retryResult.Status != "" || fixture.fixture.tokenCalls != 0 || *fixture.workflowGET != 1 || !bytes.Equal(journal, journalAfterRetry) || readErr != nil || !bytes.Equal(unconsumed, validInput) {
				t.Fatalf("consumed v3 attempt was retried: result=%+v err=%v mints=%d workflow_gets=%d input_read_err=%v unconsumed=%d journal_changed=%t", retryResult, retryErr, fixture.fixture.tokenCalls, *fixture.workflowGET, readErr, len(unconsumed), !bytes.Equal(journal, journalAfterRetry))
			}
		})
	}
}
