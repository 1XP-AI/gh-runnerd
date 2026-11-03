package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func brokerApprovalFixture() BrokerApproval {
	return BrokerApproval{OwnerNonce: strings.Repeat("a", 32), Mode: "discover-actions-host", AppID: 71, AppName: "synthetic-app", AppOwner: "org-a", AppOwnerID: 101, InstallationID: 201, Organization: "org-a", OrganizationID: 101, Repository: "canary", RepositoryID: 501, RunnerGroupID: 3, RunnerGroupName: "synthetic-group", ExpiresAt: time.Now().Add(time.Hour)}
}

func TestPairedTerminalBrokerApprovalUsesDedicatedMode(t *testing.T) {
	a := brokerApprovalFixture()
	a.Mode = "paired-terminal"
	a.Phase = "paired-terminal"
	if err := a.validate(time.Now()); err != nil {
		t.Fatalf("paired terminal approval was refused: %v", err)
	}
}

type brokerHTTPFixture struct {
	t              *testing.T
	calls          []string
	events         []string
	tokenCalls     int
	mintRequests   []brokerMintRequestFixture
	badGroup       bool
	badGroupPolicy bool
	mintError      bool
	revokeError    bool
	readTokenFault string
	readTokenToken string
	token          string
	root           string
	admissionRoot  string
}

type brokerMintRequestFixture struct {
	RepositoryIDs []int64
	Permissions   map[string]string
}

func (f *brokerHTTPFixture) RoundTrip(r *http.Request) (*http.Response, error) {
	call := r.Method + " " + r.URL.Path
	f.calls = append(f.calls, call)
	f.events = append(f.events, call)
	if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" {
		f.t.Fatal("unexpected live destination")
	}
	status := 200
	var payload any
	repo := map[string]any{"id": 501, "full_name": "org-a/canary", "private": true, "fork": false, "owner": map[string]any{"id": 101, "login": "org-a"}}
	switch r.URL.Path {
	case "/app":
		payload = map[string]any{"id": 71, "slug": "synthetic-app", "owner": map[string]any{"id": 101, "login": "org-a", "type": "Organization"}}
	case "/orgs/org-a/installation":
		payload = map[string]any{"id": 201, "app_id": 71, "target_id": 101, "target_type": "Organization", "suspended_at": nil, "account": map[string]any{"id": 101, "login": "org-a", "type": "Organization"}, "permissions": map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"}}
	case "/app/installations/201/access_tokens":
		f.tokenCalls++
		data, _ := os.ReadFile(filepath.Join(f.root, "broker.jsonl"))
		if !strings.Contains(string(data), "token_request_started") {
			f.t.Error("token request preceded durable intent")
		}
		body, _ := io.ReadAll(r.Body)
		var request struct {
			RepositoryIDs []int64           `json:"repository_ids"`
			Permissions   map[string]string `json:"permissions"`
		}
		if json.Unmarshal(body, &request) != nil || !reflect.DeepEqual(request.RepositoryIDs, []int64{501}) || len(request.Permissions) != 2 || request.Permissions["metadata"] != "read" || (request.Permissions["organization_self_hosted_runners"] != "read" && request.Permissions["organization_self_hosted_runners"] != "write") {
			f.t.Error("token request not narrowly scoped")
		}
		if request.Permissions["organization_self_hosted_runners"] == "write" && strings.Contains(string(data), "read_only_token_verified") && !strings.Contains(string(data), "management_token_request_started") {
			f.t.Error("management token request preceded its durable intent")
		}
		permissions := make(map[string]string, len(request.Permissions))
		for name, value := range request.Permissions {
			permissions[name] = value
		}
		f.mintRequests = append(f.mintRequests, brokerMintRequestFixture{RepositoryIDs: append([]int64(nil), request.RepositoryIDs...), Permissions: permissions})
		if f.mintError {
			return response(502, "synthetic-private-api-error"), nil
		}
		status = 201
		token := f.token
		expiresAt := time.Now().Add(time.Hour)
		responsePermissions := request.Permissions
		if request.Permissions["organization_self_hosted_runners"] == "read" {
			token = "synthetic-private-read-installation-token"
			if f.readTokenToken != "" {
				token = f.readTokenToken
			}
			switch f.readTokenFault {
			case "write-scope":
				responsePermissions = map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"}
			case "expired":
				expiresAt = time.Now().Add(-time.Minute)
			}
		}
		payload = map[string]any{"token": token, "expires_at": expiresAt, "permissions": responsePermissions, "repository_selection": "selected", "repositories": []any{repo}}
	case "/installation/token":
		if r.Method != http.MethodDelete || r.Header.Get("Authorization") != "Bearer synthetic-private-read-installation-token" {
			f.t.Error("installation-token revocation used the wrong method or authority")
		}
		data, err := os.ReadFile(filepath.Join(f.root, "broker.jsonl"))
		if err != nil || !strings.Contains(string(data), "read_only_token_revocation_started") {
			f.t.Error("installation-token revocation preceded its durable journal intent")
		}
		if f.revokeError {
			status = http.StatusBadGateway
			payload = map[string]string{"message": "synthetic revocation failure"}
		} else {
			status = http.StatusNoContent
		}
	case "/installation/repositories":
		payload = map[string]any{"total_count": 1, "repositories": []any{repo}}
	case "/repos/org-a/canary":
		payload = repo
	case "/orgs/org-a/actions/runner-groups/3":
		payload = map[string]any{"id": 3, "name": "synthetic-group", "visibility": "selected", "default": false, "inherited": false, "allows_public_repositories": f.badGroup}
	case "/orgs/org-a/actions/runner-groups/3/repositories":
		payload = map[string]any{"total_count": 1, "repositories": []any{repo}}
	case "/repos/org-a/canary/actions/runs/7":
		payload = map[string]any{"id": 7, "head_branch": "main", "head_sha": strings.Repeat("b", 40), "path": ".github/workflows/canary.yml", "event": "workflow_dispatch", "run_attempt": 1, "repository": repo, "head_repository": repo}
	case "/orgs/org-a/actions/runners/registration-token":
		status = 201
		payload = map[string]any{"token": "synthetic-private-registration-token", "expires_at": time.Now().Add(time.Hour)}
	case "/actions/runner-registration":
		if r.Header.Get("Authorization") != "RemoteAuth synthetic-private-registration-token" {
			f.t.Error("wrong admin exchange authority")
		}
		payload = map[string]any{"url": "https://fixture.actions.githubusercontent.com/tenant/synthetic/", "token": "synthetic-private-admin-token"}
	default:
		f.t.Fatal("unapproved API call")
	}
	if status == http.StatusNoContent {
		return response(status, ""), nil
	}
	data, _ := json.Marshal(payload)
	return response(status, string(data)), nil
}

func TestBrokerWorkflowVerificationUsesHeadBranchForAttestedBranchRef(t *testing.T) {
	f := &brokerHTTPFixture{t: t}
	api := newBrokerAPI(time.Now, f)
	a := brokerApprovalFixture()
	controller := controllerApproval{WorkflowRunID: 7, WorkflowSHA: strings.Repeat("b", 40), WorkflowRef: "refs/heads/main", WorkflowPath: ".github/workflows/canary.yml"}
	provenance := &BrokerProvenanceReceipt{WorkflowRef: controller.WorkflowRef}
	if err := api.verifyWorkflowWithReceipt(context.Background(), a, controller, "synthetic-private-workflow-token", provenance); err != nil {
		t.Fatalf("workflow run with the API's head_branch field was refused: %v", err)
	}
}

func TestBrokerReadOnlyRevocationRecordsUncertainWhenRevokedSyncFails(t *testing.T) {
	a, _, fixtureAPI, fixture, root := newBrokerFixture(t)
	journal, err := openBrokerJournal(root, a)
	if err != nil {
		t.Fatalf("open attempt journal: %v", err)
	}
	t.Cleanup(journal.close)
	claim, err := openBrokerAdmission(fixture.admissionRoot, a, journal, nil, fixtureAPI.syncDirectory)
	if err != nil {
		t.Fatalf("open admission claim: %v", err)
	}
	t.Cleanup(claim.close)
	journal.syncFile = func(phase string) error {
		if phase == "read_only_token_revoked" {
			return errors.New("synthetic fsync failure")
		}
		return journal.file.Sync()
	}
	deleteCalls := 0
	api := newBrokerAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
		deleteCalls++
		if r.Method != http.MethodDelete || r.Header.Get("Authorization") != "Bearer synthetic-private-read-installation-token" {
			t.Errorf("revocation used the wrong method or authority: %s %s", r.Method, r.URL.Path)
		}
		return response(http.StatusNoContent, ""), nil
	}))
	if err := revokeBrokerReadOnlyToken(context.Background(), api, journal, claim, "synthetic-private-read-installation-token", "synthetic-private-workflow-token"); err == nil {
		t.Fatal("revocation succeeded despite a failed durable success record")
	}
	if deleteCalls != 1 {
		t.Fatalf("revocation requests=%d, want exactly one", deleteCalls)
	}
	data, err := os.ReadFile(filepath.Join(root, "broker.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	started := strings.Index(string(data), "read_only_token_revocation_started")
	revoked := strings.Index(string(data), "read_only_token_revoked")
	uncertain := strings.Index(string(data), "read_only_token_revocation_uncertain")
	if started < 0 || revoked <= started || uncertain <= revoked || !strings.Contains(string(data), "revocation_result_persistence_failed") {
		t.Fatalf("failed revoked sync was not followed by a terminal uncertainty record: %s", data)
	}
	if strings.Contains(string(data), "synthetic-private-read-installation-token") {
		t.Fatal("read-only installation token was persisted in the broker journal")
	}
}

func TestBrokerWorkflowRunHeadBranchMatchesOnlySupportedRefs(t *testing.T) {
	for _, tc := range []struct {
		name, headBranch, workflowRef string
		want                          bool
	}{
		{name: "branch", headBranch: "main", workflowRef: "refs/heads/main", want: true},
		{name: "nested branch", headBranch: "release/v1", workflowRef: "refs/heads/release/v1", want: true},
		{name: "tag", headBranch: "v1.2.3", workflowRef: "refs/tags/v1.2.3", want: false},
		{name: "branch tag collision", headBranch: "main", workflowRef: "refs/tags/main", want: false},
		{name: "mismatch", headBranch: "main", workflowRef: "refs/heads/release", want: false},
		{name: "missing branch", headBranch: "", workflowRef: "refs/heads/main", want: false},
		{name: "pull head", headBranch: "17", workflowRef: "refs/pull/17/head", want: false},
		{name: "pull merge", headBranch: "17", workflowRef: "refs/pull/17/merge", want: false},
		{name: "unsupported ref", headBranch: "main", workflowRef: "refs/remotes/origin/main", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := brokerWorkflowRunHeadBranchMatchesRef(tc.headBranch, tc.workflowRef); got != tc.want {
				t.Fatalf("head_branch=%q workflow_ref=%q matched=%t want=%t", tc.headBranch, tc.workflowRef, got, tc.want)
			}
		})
	}
}

func newBrokerFixture(t *testing.T) (BrokerApproval, Candidate, *brokerAPI, *brokerHTTPFixture, string) {
	t.Helper()
	parent := t.TempDir()
	if os.Chmod(parent, 0700) != nil {
		t.Fatal("private fixture directory")
	}
	root := filepath.Join(parent, "attempt")
	f := &brokerHTTPFixture{t: t, token: "synthetic-private-installation-token", root: root}
	admission := filepath.Join(parent, "admission")
	if os.Mkdir(admission, 0700) != nil {
		t.Fatal("fixture admission")
	}
	admission, _ = filepath.EvalSymlinks(admission)
	f.admissionRoot = admission
	adapter := newSignedBrokerFixtureAdapter(t)
	api := newBrokerAPIWithProvenanceRoot(time.Now, f, adapter.root)
	api.admissionDirectory = func() (string, error) { return admission, nil }
	// The test-only adapter is the explicit signed fixture source. Production
	// newBrokerAPI has no provenance adapter and therefore quarantines controller
	// and paired execution before reading credentials.
	api.provenance = adapter
	return brokerApprovalFixture(), syntheticCandidate(t), api, f, root
}
func TestBrokerDiscoveryUsesOneRestrictedTokenThenScopeBeforeAuth(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	result, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil)
	if err != nil || result.Status != "actions_host_observed" || result.ActionsHost != "fixture.actions.githubusercontent.com" {
		t.Fatalf("discovery incomplete: %+v %v", result, err)
	}
	if f.tokenCalls != 1 {
		t.Fatal("not exactly one token request")
	}
	want := []string{"GET /app", "GET /orgs/org-a/installation", "POST /app/installations/201/access_tokens", "GET /installation/repositories", "GET /repos/org-a/canary", "GET /orgs/org-a/actions/runner-groups/3", "GET /orgs/org-a/actions/runner-groups/3/repositories", "POST /orgs/org-a/actions/runners/registration-token", "POST /actions/runner-registration"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("unsafe operation order: %v", f.calls)
	}
	assertNoSecretFiles(t, root, string(c.PEM), f.token, "synthetic-private-registration-token", "synthetic-private-admin-token")
}

func TestBrokerMalformedPEMRefusesBeforePersistentAdmission(t *testing.T) {
	a, _, api, f, root := newBrokerFixture(t)
	admissionCalls := 0
	originalAdmissionDirectory := api.admissionDirectory
	api.admissionDirectory = func() (string, error) {
		admissionCalls++
		return originalAdmissionDirectory()
	}
	if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: "not a PEM credential"}, root, api, nil); err == nil {
		t.Fatal("malformed PEM reached broker effects")
	}
	if admissionCalls != 0 {
		t.Fatalf("malformed PEM entered persistent admission: directory lookups=%d", admissionCalls)
	}
	if f.tokenCalls != 0 || len(f.calls) != 0 {
		t.Fatalf("malformed PEM reached authenticated effects: tokenCalls=%d calls=%v", f.tokenCalls, f.calls)
	}
	if _, err := os.Stat(filepath.Join(f.admissionRoot, "broker-admission.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("malformed PEM created admission ledger: err=%v", err)
	}
}

func TestBrokerFailedGroupNeverObtainsAuthOrHandsOff(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	f.badGroup = true
	_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil)
	if err == nil {
		t.Fatal("unsafe group handed off")
	}
	for _, call := range f.calls {
		if strings.Contains(call, "registration-token") || strings.Contains(call, "runner-registration") {
			t.Fatal("auth after failed preflight")
		}
	}
	if f.tokenCalls != 1 {
		t.Fatal("scope failure retried issuance")
	}
}
func TestBrokerAmbiguousMintSurvivesRestartWithoutRetry(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	f.mintError = true
	for i := 0; i < 2; i++ {
		_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil)
		if err == nil || strings.Contains(err.Error(), "synthetic-private") {
			t.Fatal("mint ambiguity accepted or disclosed")
		}
	}
	if f.tokenCalls != 1 {
		t.Fatal("ambiguous token mint retried")
	}
	assertNoSecretFiles(t, root, string(c.PEM), "synthetic-private-api-error")
}
func TestBrokerControllerPayloadUsesActualIssuanceAndPrivateHandoff(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	a.Mode = "controller"
	a.Phase = "create"
	calls := 0
	plan := brokerTestPlan(t, &a, filepath.Dir(root), func(_ context.Context, data []byte, _ string) error {
		calls++
		var payload map[string]any
		if json.Unmarshal(data, &payload) != nil {
			t.Fatal("invalid controller input")
		}
		if payload["installation_token"] != f.token || payload["app_id"] != float64(71) || payload["installation_id"] != float64(201) || payload["organization"] != "org-a" || payload["organization_self_hosted_runners"] != "write" || payload["metadata"] != "read" {
			t.Fatal("issuance facts lost")
		}
		if _, ok := payload["pem"]; ok {
			t.Fatal("PEM handed to controller")
		}
		return nil
	})
	result, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, plan)
	if err != nil || result.Status != "controller_completed" || calls != 1 {
		t.Fatal("private controller handoff incomplete")
	}
	assertNoSecretFiles(t, root, string(c.PEM), f.token)
}
