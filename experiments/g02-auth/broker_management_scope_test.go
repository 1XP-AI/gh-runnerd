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

func organizationScopeFixture(t *testing.T, change func(map[string]any)) (BrokerApproval, Candidate, *brokerAPI, *brokerHTTPFixture, string) {
	t.Helper()
	a, candidate, api, fixture, root := newBrokerFixture(t)
	a.ManagementScope = "organization-only"
	api.client.Transport = brokerTransport{transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/installation/repositories" || r.URL.Path == "/repos/org-a/canary" {
			t.Error("organization-only management token reached a repository-access endpoint")
			return response(http.StatusForbidden, "{}"), nil
		}
		if r.URL.Path != "/app/installations/201/access_tokens" {
			return fixture.RoundTrip(r)
		}
		fixture.calls = append(fixture.calls, r.Method+" "+r.URL.Path)
		fixture.tokenCalls++
		data, err := io.ReadAll(r.Body)
		var request map[string]any
		want := map[string]any{"permissions": map[string]any{"organization_self_hosted_runners": "write"}}
		if err != nil || json.Unmarshal(data, &request) != nil || !reflect.DeepEqual(request, want) {
			t.Error("organization-only mint requested repository selectors or extra permissions")
		}
		issued := map[string]any{"token": fixture.token, "expires_at": time.Now().Add(time.Hour), "permissions": map[string]any{"organization_self_hosted_runners": "write"}}
		if change != nil {
			change(issued)
		}
		data, err = json.Marshal(issued)
		if err != nil {
			t.Fatal(err)
		}
		return response(http.StatusCreated, string(data)), nil
	})}
	return a, candidate, api, fixture, root
}

func TestBrokerOrganizationOnlyMintUsesExactPermission(t *testing.T) {
	for _, fault := range []string{"", "empty-repositories", "null-repositories", "selection-all", "selection-selected", "metadata", "contents", "missing-permissions", "read-only", "returned-repository", "unknown-selection", "expired", "long-lived", "short-token"} {
		t.Run(fault, func(t *testing.T) {
			a, candidate, api, fixture, _ := organizationScopeFixture(t, func(issued map[string]any) {
				switch fault {
				case "empty-repositories":
					issued["repositories"] = []any{}
				case "null-repositories":
					issued["repositories"] = nil
				case "selection-all":
					issued["repository_selection"] = "all"
				case "selection-selected":
					issued["repository_selection"] = "selected"
				case "metadata", "contents":
					issued["permissions"].(map[string]any)[fault] = "read"
				case "missing-permissions":
					delete(issued, "permissions")
				case "read-only":
					issued["permissions"] = map[string]any{"organization_self_hosted_runners": "read"}
				case "returned-repository":
					issued["repositories"] = []any{map[string]any{"id": 501}}
				case "unknown-selection":
					issued["repository_selection"] = "unrecognized"
				case "expired":
					issued["expires_at"] = time.Now()
				case "long-lived":
					issued["expires_at"] = time.Now().Add(2 * time.Hour)
				case "short-token":
					issued["token"] = "short"
				}
			})
			credential, err := parseCredential(candidate)
			if err != nil {
				t.Fatal(err)
			}
			_, err = api.mint(context.Background(), a, credential)
			accept := fault == "" || fault == "empty-repositories" || fault == "null-repositories" || strings.HasPrefix(fault, "selection-")
			if (err == nil) != accept || fixture.tokenCalls != 1 {
				t.Fatalf("mint accepted=%v want=%v requests=%d", err == nil, accept, fixture.tokenCalls)
			}
		})
	}
}

func TestBrokerOrganizationOnlyDiscoveryKeepsGroupBoundary(t *testing.T) {
	for _, badGroup := range []bool{false, true} {
		t.Run(map[bool]string{false: "selected-private-group", true: "public-group"}[badGroup], func(t *testing.T) {
			a, candidate, api, fixture, root := organizationScopeFixture(t, nil)
			fixture.badGroup = badGroup
			result, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM)}, root, api, nil)
			if (err == nil) == badGroup || (!badGroup && result.Status != "actions_host_observed") {
				t.Fatal("organization-only discovery did not enforce group eligibility")
			}
			want := []string{"GET /app", "GET /orgs/org-a/installation", "POST /app/installations/201/access_tokens", "GET /orgs/org-a/actions/runner-groups/3"}
			if !badGroup {
				want = append(want, "GET /orgs/org-a/actions/runner-groups/3/repositories", "POST /orgs/org-a/actions/runners/registration-token", "POST /actions/runner-registration")
			}
			if !reflect.DeepEqual(fixture.calls, want) || fixture.tokenCalls != 1 {
				t.Fatalf("wrong management operation order: %v", fixture.calls)
			}
			assertNoSecretFiles(t, root, string(candidate.PEM), fixture.token, "synthetic-private-registration-token", "synthetic-private-admin-token")
		})
	}
}

func TestBrokerManagementScopeIsExplicitAndMatchesController(t *testing.T) {
	a, _, _, _, root := newBrokerFixture(t)
	a.Mode, a.Phase = "controller", "create"
	plan := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { return nil })
	defer plan.close()
	for _, scope := range []string{"", "organization-only", "all", "Organization-only", "organization-only "} {
		candidate := a
		candidate.ManagementScope = scope
		if (candidate.validate(time.Now()) == nil) != (scope == "" || scope == "organization-only") {
			t.Fatalf("unexpected approval scope validity: %q", scope)
		}
	}
	for _, brokerScope := range []string{"", "organization-only"} {
		for _, controllerScope := range []string{"", "organization-only"} {
			a.ManagementScope = brokerScope
			controller := plan.controller
			controller.ManagementScope = controllerScope
			if (controller.validate(a, time.Now()) == nil) != (brokerScope == controllerScope) {
				t.Fatal("controller accepted a different management scope")
			}
		}
	}
}

func TestBrokerManagementScopeCannotReuseConsumedState(t *testing.T) {
	for _, initial := range []string{"", "organization-only"} {
		t.Run(map[string]string{"": "legacy", "organization-only": "organization-only"}[initial], func(t *testing.T) {
			factory := newBrokerFixture
			if initial != "" {
				factory = func(t *testing.T) (BrokerApproval, Candidate, *brokerAPI, *brokerHTTPFixture, string) {
					return organizationScopeFixture(t, nil)
				}
			}
			a, candidate, api, fixture, root := factory(t)
			if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM)}, root, api, nil); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(fixture.admissionRoot, "broker-admission.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			a.ManagementScope = map[string]string{"": "organization-only", "organization-only": ""}[initial]
			if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM)}, root, api, nil); err == nil || fixture.tokenCalls != 1 {
				t.Fatal("changed scope reused an already consumed experiment")
			}
			after, err := os.ReadFile(filepath.Join(fixture.admissionRoot, "broker-admission.jsonl"))
			if err != nil || string(before) != string(after) {
				t.Fatal("scope refusal changed durable admission state")
			}
		})
	}
}

func TestBrokerOrganizationOnlyControllerHandoffPreservesScope(t *testing.T) {
	a, candidate, api, fixture, root := organizationScopeFixture(t, nil)
	a.Mode, a.Phase = "controller", "create"
	launched := 0
	plan := brokerTestPlan(t, &a, filepath.Dir(root), func(_ context.Context, data []byte, _ string) error {
		launched++
		var payload map[string]any
		if json.Unmarshal(data, &payload) != nil || payload["management_scope"] != "organization-only" || payload["metadata"] != "" || payload["installation_token"] != fixture.token || payload["organization_self_hosted_runners"] != "write" {
			t.Fatal("organization-only scope or permission changed at handoff")
		}
		return nil
	})
	result, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM)}, root, api, plan)
	if err != nil || result.Status != "controller_completed" || launched != 1 || fixture.tokenCalls != 1 {
		t.Fatalf("synthetic controller handoff failed: launches=%d mints=%d err=%v", launched, fixture.tokenCalls, err)
	}
	assertNoSecretFiles(t, root, string(candidate.PEM), fixture.token)
}

func TestBrokerOrganizationOnlyPreflightRejectsMissingOrChangedPolicy(t *testing.T) {
	faults := []string{"", "fork", "missing-fork", "null-fork", "public", "id", "full-name", "owner-id", "owner-login", "count", "extra-repository", "group-name", "group-id", "visibility"}
	for _, flag := range []string{"default", "inherited", "allows_public_repositories"} {
		for _, value := range []string{"true", "null", "missing"} {
			faults = append(faults, flag+"/"+value)
		}
	}
	for _, fault := range faults {
		t.Run(fault, func(t *testing.T) {
			a, _, api, fixture, _ := organizationScopeFixture(t, nil)
			inner := api.client.Transport
			api.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer "+fixture.token {
					t.Error("wrong preflight method or management authority")
				}
				res, err := inner.RoundTrip(r)
				if err != nil {
					return nil, err
				}
				defer res.Body.Close()
				var body map[string]any
				if json.NewDecoder(res.Body).Decode(&body) != nil {
					t.Fatal("policy fixture JSON")
				}
				if strings.HasSuffix(r.URL.Path, "/repositories") {
					repo := body["repositories"].([]any)[0].(map[string]any)
					switch fault {
					case "fork":
						repo["fork"] = true
					case "missing-fork":
						delete(repo, "fork")
					case "null-fork":
						repo["fork"] = nil
					case "public":
						repo["private"] = false
					case "id":
						repo["id"] = 999
					case "full-name":
						repo["full_name"] = "other/target"
					case "owner-id":
						repo["owner"].(map[string]any)["id"] = 999
					case "owner-login":
						repo["owner"].(map[string]any)["login"] = "other"
					case "count":
						body["total_count"] = 2
					case "extra-repository":
						body["repositories"] = []any{repo, repo}
					}
				} else {
					switch fault {
					case "group-name":
						body["name"] = "other-group"
					case "group-id":
						body["id"] = 999
					case "visibility":
						body["visibility"] = "all"
					}
					if parts := strings.Split(fault, "/"); len(parts) == 2 {
						switch parts[1] {
						case "true":
							body[parts[0]] = true
						case "null":
							body[parts[0]] = nil
						case "missing":
							delete(body, parts[0])
						}
					}
				}
				data, _ := json.Marshal(body)
				return response(res.StatusCode, string(data)), nil
			})
			err := api.preflight(context.Background(), a, fixture.token)
			if (err == nil) != (fault == "") || (fault == "" && len(fixture.calls) != 2) {
				t.Fatal("organization-only group preflight accepted incomplete/changed policy")
			}
		})
	}
}

func TestBrokerOrganizationOnlyAmbiguousMintNeverRetries(t *testing.T) {
	a, candidate, api, fixture, root := organizationScopeFixture(t, nil)
	inner := api.client.Transport
	api.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/app/installations/201/access_tokens" {
			fixture.tokenCalls++
			return response(http.StatusBadGateway, "synthetic-private-ambiguous-issuance"), nil
		}
		return inner.RoundTrip(r)
	})
	for attempt := 0; attempt < 2; attempt++ {
		_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM)}, root, api, nil)
		if err == nil || strings.Contains(err.Error(), "synthetic-private") {
			t.Fatal("ambiguous mint accepted or exposed")
		}
	}
	if fixture.tokenCalls != 1 {
		t.Fatal("ambiguous organization-only mint retried")
	}
	assertNoSecretFiles(t, root, string(candidate.PEM), "synthetic-private-ambiguous-issuance")
}
