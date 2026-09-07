package enrollment

import (
	"context"
	"encoding/json"
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

type brokerHTTPFixture struct {
	t             *testing.T
	calls         []string
	tokenCalls    int
	badGroup      bool
	mintError     bool
	token         string
	root          string
	admissionRoot string
}

func (f *brokerHTTPFixture) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
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
		if json.Unmarshal(body, &request) != nil || !reflect.DeepEqual(request.RepositoryIDs, []int64{501}) || len(request.Permissions) != 2 || request.Permissions["metadata"] != "read" || request.Permissions["organization_self_hosted_runners"] != "write" {
			f.t.Error("token request not narrowly scoped")
		}
		if f.mintError {
			return response(502, "synthetic-private-api-error"), nil
		}
		status = 201
		payload = map[string]any{"token": f.token, "expires_at": time.Now().Add(time.Hour), "permissions": request.Permissions, "repository_selection": "selected", "repositories": []any{repo}}
	case "/installation/repositories":
		payload = map[string]any{"total_count": 1, "repositories": []any{repo}}
	case "/repos/org-a/canary":
		payload = repo
	case "/orgs/org-a/actions/runner-groups/3":
		payload = map[string]any{"id": 3, "name": "synthetic-group", "visibility": "selected", "default": false, "inherited": false, "allows_public_repositories": f.badGroup}
	case "/orgs/org-a/actions/runner-groups/3/repositories":
		payload = map[string]any{"total_count": 1, "repositories": []any{repo}}
	case "/repos/org-a/canary/actions/runs/7":
		payload = map[string]any{"id": 7, "head_sha": strings.Repeat("b", 40), "path": ".github/workflows/canary.yml", "event": "workflow_dispatch", "run_attempt": 1, "repository": repo, "head_repository": repo}
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
	data, _ := json.Marshal(payload)
	return response(status, string(data)), nil
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
	api := newBrokerAPI(time.Now, f)
	api.admissionDirectory = func() (string, error) { return admission, nil }
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
