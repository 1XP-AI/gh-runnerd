package livecanary

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func controllerGroupPolicy() handoff.GroupPolicy {
	return handoff.GroupPolicy{Version: 2, OrganizationID: 101, Workflow: "org-a/canary/.github/workflows/runner.yml@" + strings.Repeat("d", 40)}
}

func TestControllerGroupPolicyCannotFallBackToDirectDriver(t *testing.T) {
	a := approval()
	a.GroupPolicy = controllerGroupPolicy()
	a.GroupPolicy.Workflow = a.Organization + "/" + a.Repository + "/.github/workflows/runner.yml@" + strings.Repeat("d", 40)
	j, e := openTestJournal(t, privateDir(t), a)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	api := &fakeAPI{}
	d := Driver{a, j, api}
	if e := d.Run(context.Background(), "create"); e == nil || api.createCalls != 0 || len(j.Events()) != 0 {
		t.Fatal("v2 group approval fell back to direct driver", e, api.createCalls)
	}
}

func TestControllerGroupPolicySelectionAndSignedApprovalBinding(t *testing.T) {
	a := controllerHandoffApprovalAt(time.Now())
	a.Phases = []string{"create"}
	a.GroupPolicy = controllerGroupPolicy()
	a.GroupPolicy.Workflow = a.Organization + "/" + a.Repository + "/.github/workflows/runner.yml@" + strings.Repeat("d", 40)
	raw, _ := json.Marshal(a)
	_, request, e := controllerHandoffRequest(raw, "create", "github-rest-workflow-v1", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	p := handoff.PublicPolicy{Version: 2, Role: "controller", Source: request.Source, WorkflowKeyID: "workflow-v1", WorkflowPublicKey: "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8", IssuerKeyID: "issuer-v1", IssuerPublicKey: "e28a8970753332bd72fef413e6b0b2ef1b4aadda7aa2c141f233712a6876b351", Grants: []handoff.PhaseGrant{{Request: request, Mode: "controller", OrganizationID: 101, RepositoryID: a.RepositoryID, ExpiresAt: a.ExpiresAt, GroupPolicySHA256: a.GroupPolicy.GrantDigest(request, a.RepositoryID, int64(a.RunnerGroupID), a.ManagementScope)}}}
	dir := privateDir(t)
	files := PairedTerminalFiles{ControllerApprovalPath: filepath.Join(dir, "approval.json")}
	old := controllerExperimentPolicyJSON
	defer func() { controllerExperimentPolicyJSON = old }()
	for _, fault := range []string{"", "v1", "missing-grant", "wrong-grant", "group", "scope", "literal", "organization-id", "raw-bytes", "default"} {
		t.Run(fault, func(t *testing.T) {
			x, q := a, p
			q.Grants = append([]handoff.PhaseGrant(nil), p.Grants...)
			switch fault {
			case "v1":
				q.Version = 1
				q.Grants[0].GroupPolicySHA256 = ""
			case "missing-grant":
				q.Grants[0].GroupPolicySHA256 = ""
			case "wrong-grant":
				q.Grants[0].GroupPolicySHA256 = strings.Repeat("f", 64)
			case "group":
				x.RunnerGroupID++
			case "scope":
				x.ManagementScope = "organization-only"
			case "literal":
				x.GroupPolicy.Workflow = strings.Replace(x.GroupPolicy.Workflow, "dddd", "eeee", 1)
			case "organization-id":
				x.GroupPolicy.OrganizationID++
			}
			b, _ := json.Marshal(x)
			if fault == "raw-bytes" {
				b = append(b, '\n')
			}
			qraw, _ := json.Marshal(q)
			controllerExperimentPolicyJSON = string(qraw)
			if fault == "default" {
				controllerExperimentPolicyJSON = ""
			}
			if os.WriteFile(files.ControllerApprovalPath, b, 0600) != nil {
				t.Fatal("fixture approval")
			}
			_, e := selectControllerExperiment(files, "controller", "create", time.Now())
			if (e == nil) != (fault == "" || fault == "legacy") {
				t.Fatal("incorrect group authority selection", fault, e)
			}
		})
	}
	// The unchanged v1 receipt signs the exact raw approval hash, which includes
	// the selected v2 policy. Prove both actual signature and reader boundaries.
	f := controllerHandoffFixtureForApproval(t, a, "create", time.Now())
	if f.root.Verify(f.request, f.receipt, time.Now()) != nil {
		t.Fatal("signed group approval rejected")
	}
	x := a
	x.GroupPolicy.Workflow = strings.Replace(x.GroupPolicy.Workflow, "dddd", "eeee", 1)
	xraw, _ := json.Marshal(x)
	_, changed, e := controllerHandoffRequest(xraw, "create", "signed-fixture-v1", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if f.root.Verify(changed, f.receipt, time.Now()) == nil {
		t.Fatal("signature accepted another group approval")
	}
	t.Run("reader", func(t *testing.T) {
		j, e := openTestJournal(t, privateDir(t), x)
		if e != nil {
			t.Fatal(e)
		}
		defer j.Close()
		reads := 0
		_, e = consumeControllerHandoff(context.Background(), j, xraw, "create", "signed-fixture-v1", newControllerHandoffMemorySource(f.receiptBytes), f.root, time.Now, func(context.Context) ([]byte, error) { reads++; return []byte("synthetic-credential"), nil })
		if e == nil || reads != 0 {
			t.Fatal("changed group approval crossed credential reader")
		}
	})
	t.Run("ownership", func(t *testing.T) {
		j, e := openTestJournal(t, privateDir(t), a)
		if e != nil {
			t.Fatal(e)
		}
		defer j.Close()
		if release, e := j.authorize(x); e == nil {
			release()
			t.Fatal("group change retained journal authority")
		}
	})
}

func TestControllerExactGroupPolicyPreflight(t *testing.T) {
	for _, scope := range []string{"", "organization-only"} {
		for _, fault := range []string{"", "legacy", "legacy-flags", "missing", "null", "false", "empty", "null-list", "missing-list", "extra", "wrong", "caller", "branch", "tag", "group", "403", "default-missing", "default-null", "inherited-missing", "inherited-null", "public-missing", "public-null"} {
			t.Run(scope+"/"+fault, func(t *testing.T) {
				a := approval()
				a.ManagementScope = scope
				a.GroupPolicy = controllerGroupPolicy()
				// This fixture deliberately uses the existing target, never a live policy.
				a.GroupPolicy.Workflow = a.Organization + "/" + a.Repository + "/.github/workflows/runner.yml@" + strings.Repeat("d", 40)
				if fault == "legacy" || fault == "legacy-flags" {
					a.GroupPolicy = handoff.GroupPolicy{}
				}
				c := credentials(a)
				c.ManagementScope = scope
				if scope != "" {
					c.Metadata = ""
				}
				calls := 0
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Method != "GET" {
						t.Error("preflight effect")
						w.WriteHeader(403)
						return
					}
					group := map[string]any{"id": a.RunnerGroupID, "visibility": "selected", "default": false, "inherited": false, "allows_public_repositories": false, "restricted_to_workflows": true, "selected_workflows": []string{a.GroupPolicy.Workflow}}
					repo := map[string]any{"id": a.RepositoryID, "full_name": a.Organization + "/" + a.Repository, "private": true, "fork": false, "owner": map[string]any{"login": a.Organization}}
					if strings.HasSuffix(r.URL.Path, "/repositories") {
						_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "repositories": []any{repo}})
						return
					}
					if strings.HasPrefix(r.URL.Path, "/repos/") {
						_ = json.NewEncoder(w).Encode(repo)
						return
					}
					switch fault {
					case "legacy-flags":
						delete(group, "default")
						group["inherited"] = nil
						delete(group, "allows_public_repositories")
					case "default-missing":
						delete(group, "default")
					case "default-null":
						group["default"] = nil
					case "inherited-missing":
						delete(group, "inherited")
					case "inherited-null":
						group["inherited"] = nil
					case "public-missing":
						delete(group, "allows_public_repositories")
					case "public-null":
						group["allows_public_repositories"] = nil
					case "legacy":
						group["restricted_to_workflows"] = "ignored-v1"
						group["selected_workflows"] = 123
					case "missing":
						delete(group, "restricted_to_workflows")
					case "null":
						group["restricted_to_workflows"] = nil
					case "false":
						group["restricted_to_workflows"] = false
					case "empty":
						group["selected_workflows"] = []string{}
					case "null-list":
						group["selected_workflows"] = nil
					case "missing-list":
						delete(group, "selected_workflows")
					case "extra":
						group["selected_workflows"] = []string{a.GroupPolicy.Workflow, a.GroupPolicy.Workflow}
					case "wrong":
						group["selected_workflows"] = []string{strings.Replace(a.GroupPolicy.Workflow, "dddd", "eeee", 1)}
					case "caller":
						group["selected_workflows"] = []string{a.Organization + "/" + a.Repository + "/" + a.WorkflowPath + "@" + a.WorkflowSHA}
					case "branch":
						group["selected_workflows"] = []string{strings.Split(a.GroupPolicy.Workflow, "@")[0] + "@refs/heads/main"}
					case "tag":
						group["selected_workflows"] = []string{strings.Split(a.GroupPolicy.Workflow, "@")[0] + "@v1"}
					case "group":
						group["id"] = a.RunnerGroupID + 1
					case "403":
						w.WriteHeader(403)
						return
					}
					_ = json.NewEncoder(w).Encode(group)
				}))
				defer s.Close()
				api := &SDKAPI{rest: s.Client(), baseURL: s.URL, approval: a, credentials: c}
				err := api.Preflight(context.Background(), a)
				if (err == nil) != (fault == "" || fault == "legacy" || (fault == "legacy-flags" && scope == "")) {
					t.Fatalf("policy accepted=%v fault=%s calls=%d", err == nil, fault, calls)
				}
			})
		}
	}
}
