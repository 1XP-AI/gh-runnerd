package livecanary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagementScopeLegacyControllerGolden(t *testing.T) {
	// Matches the broker's original controller schema, including field order.
	legacy := `{"app_id":71,"installation_id":201,"organization":"org-a","repository":"canary","repository_id":501,"runner_group_id":3,"owner_nonce":"` + strings.Repeat("a", 32) + `","harness_sha":"` + strings.Repeat("b", 40) + `","workflow_sha":"` + strings.Repeat("c", 40) + `","workflow_ref":"refs/heads/main","workflow_path":".github/workflows/canary.yml","workflow_run_id":7,"controller":"trusted-controller","expires_at":"2026-10-25T00:00:00Z","actions_hosts":["fixture.actions.githubusercontent.com"],"phases":["create"]}`
	var a Approval
	if DecodeStrict([]byte(legacy), &a) != nil {
		t.Fatal("legacy controller approval no longer decodes")
	}
	data, err := json.Marshal(a)
	digest := sha256.Sum256([]byte(legacy))
	if err != nil || string(data) != legacy || approvalDigest(a) != hex.EncodeToString(digest[:]) {
		t.Fatal("empty scope changed legacy controller bytes or digest")
	}
	owner := strings.Replace(legacy, `"expires_at":"2026-10-25T00:00:00Z"`, `"expires_at":"0001-01-01T00:00:00Z"`, 1)
	owner = strings.Replace(owner, `"phases":["create"]`, `"phases":null`, 1)
	ownerDigest := sha256.Sum256([]byte(owner))
	if ownershipDigest(a) != hex.EncodeToString(ownerDigest[:]) {
		t.Fatal("empty scope changed legacy controller ownership")
	}
	a.ManagementScope = "organization-only"
	if approvalDigest(a) == hex.EncodeToString(digest[:]) || ownershipDigest(a) == hex.EncodeToString(ownerDigest[:]) {
		t.Fatal("scope omitted from controller authority or ownership binding")
	}
}

func TestOrganizationOnlyPreflightRequiresCompletePrivateGroupPolicy(t *testing.T) {
	faults := []string{"", "group-id", "all-repositories", "repository-id", "full-name", "owner", "missing-owner", "public", "fork", "missing-fork", "null-fork", "count", "extra-repository", "empty-repositories", "duplicate-policy", "folded-policy", "duplicate-owner"}
	for _, flag := range []string{"default", "inherited", "allows_public_repositories"} {
		for _, state := range []string{"true", "missing", "null"} {
			faults = append(faults, flag+"/"+state)
		}
	}
	for _, fault := range faults {
		t.Run(fault, func(t *testing.T) {
			a := approval()
			a.ManagementScope = "organization-only"
			a.WorkflowRunID = 7
			c := credentials(a)
			c.ManagementScope, c.Metadata = a.ManagementScope, ""
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads++
				workflow := strings.Contains(r.URL.Path, "/actions/runs/")
				wantToken := c.InstallationToken
				if workflow {
					wantToken = c.VerificationToken
				}
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer "+wantToken {
					t.Error("management authority crossed its endpoint boundary")
				}
				prefix := "/orgs/" + a.Organization + "/actions/runner-groups/"
				if !strings.HasPrefix(r.URL.Path, prefix) && !workflow {
					t.Error("organization-only management token attempted repository access")
					w.WriteHeader(http.StatusForbidden)
					return
				}
				group := map[string]any{"id": a.RunnerGroupID, "visibility": "selected", "default": false, "inherited": false, "allows_public_repositories": false}
				repo := map[string]any{"id": a.RepositoryID, "full_name": a.Organization + "/" + a.Repository, "private": true, "fork": false, "owner": map[string]any{"login": a.Organization}}
				count, repos := 1, []any{repo}
				switch fault {
				case "group-id":
					group["id"] = a.RunnerGroupID + 1
				case "all-repositories":
					group["visibility"] = "all"
				case "repository-id":
					repo["id"] = a.RepositoryID + 1
				case "full-name":
					repo["full_name"] = "other/target"
				case "owner":
					repo["owner"] = map[string]any{"login": "other"}
				case "missing-owner":
					delete(repo, "owner")
				case "public":
					repo["private"] = false
				case "fork":
					repo["fork"] = true
				case "missing-fork":
					delete(repo, "fork")
				case "null-fork":
					repo["fork"] = nil
				case "count":
					count = 2
				case "extra-repository":
					repos = append(repos, repo)
				case "empty-repositories":
					repos = nil
				}
				if parts := strings.Split(fault, "/"); len(parts) == 2 {
					switch parts[1] {
					case "true":
						group[parts[0]] = true
					case "missing":
						delete(group, parts[0])
					case "null":
						group[parts[0]] = nil
					}
				}
				var body any = group
				if strings.HasSuffix(r.URL.Path, "/repositories") {
					body = map[string]any{"total_count": count, "repositories": repos}
				} else if workflow {
					body = map[string]any{"id": a.WorkflowRunID, "head_sha": a.WorkflowSHA, "path": a.WorkflowPath, "event": "workflow_dispatch", "run_attempt": 1, "repository": repo, "head_repository": repo}
				}
				data, _ := json.Marshal(body)
				if !workflow && !strings.HasSuffix(r.URL.Path, "/repositories") {
					if fault == "duplicate-policy" {
						data = []byte(strings.Replace(string(data), `"default":false`, `"default":true,"default":false`, 1))
					} else if fault == "folded-policy" {
						data = []byte(strings.Replace(string(data), `"default":false`, `"DEFAULT":true,"default":false`, 1))
					}
				} else if !workflow && fault == "duplicate-owner" {
					data = []byte(strings.Replace(string(data), `"login":"`+a.Organization+`"`, `"LOGIN":"foreign","login":"`+a.Organization+`"`, 1))
				}
				_, _ = w.Write(data)
			}))
			defer server.Close()
			api := &SDKAPI{rest: server.Client(), baseURL: server.URL, approval: a, credentials: c}
			err := api.Preflight(context.Background(), a)
			if (err == nil) != (fault == "") || (fault == "" && reads != 2) {
				t.Fatalf("policy accepted=%v want=%v reads=%d", err == nil, fault == "", reads)
			}
			if fault == "" && (api.VerifyRun(context.Background(), a, a.WorkflowRunID) != nil || reads != 3) {
				t.Fatal("organization-only management changed separate workflow verification")
			}
		})
	}
}

func TestManagementScopeMustMatchCredentialPermission(t *testing.T) {
	for _, test := range []struct {
		name, approved, handedOff, metadata string
		accept                              bool
	}{
		{"legacy", "", "", "read", true},
		{"organization-only", "organization-only", "organization-only", "", true},
		{"metadata in organization scope", "organization-only", "organization-only", "read", false},
		{"missing handoff scope", "organization-only", "", "", false},
		{"missing approved scope", "", "organization-only", "read", false},
		{"unknown scope", "all", "all", "read", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := approval()
			a.ManagementScope = test.approved
			c := credentials(a)
			c.ManagementScope, c.Metadata = test.handedOff, test.metadata
			if (c.validate(a, time.Now()) == nil) != test.accept {
				t.Fatal("credential scope or permission accepted incorrectly")
			}
			if (a.Validate(time.Now()) == nil) != (test.approved == "" || test.approved == "organization-only") {
				t.Fatal("unknown approval management scope accepted")
			}
		})
	}
}

func TestManagementScopeCannotRenewOrReuseControllerState(t *testing.T) {
	for _, initial := range []string{"", "organization-only"} {
		t.Run(map[string]string{"": "legacy", "organization-only": "organization-only"}[initial], func(t *testing.T) {
			parent := privateDir(t)
			directory := admissionState(t, parent, "state")
			capRoot := testAdmissionDirectory(t, directory)
			a := approval()
			a.ManagementScope = initial
			open := func(a Approval) (*FileJournal, error) {
				return openJournalAtAdmission(directory, a, capRoot, func(f *os.File) error { return f.Sync() })
			}
			j, err := open(a)
			if err != nil {
				t.Fatal(err)
			}
			if err := j.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(directory, "journal.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			a.ManagementScope = map[string]string{"": "organization-only", "organization-only": ""}[initial]
			a.ExpiresAt = a.ExpiresAt.Add(time.Minute)
			a.Phases = []string{"inspect", "cleanup"}
			if changed, err := open(a); err == nil {
				_ = changed.Close()
				t.Fatal("scope-only change renewed existing controller authority")
			}
			after, err := os.ReadFile(filepath.Join(directory, "journal.jsonl"))
			if err != nil || string(before) != string(after) {
				t.Fatal("scope refusal changed controller history")
			}
		})
	}
}
