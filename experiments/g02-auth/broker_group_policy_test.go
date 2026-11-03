package enrollment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func groupPolicyFixture() handoff.GroupPolicy {
	return handoff.GroupPolicy{Version: 2, OrganizationID: 101, Workflow: "org-a/canary/.github/workflows/runner.yml@" + strings.Repeat("d", 40)}
}

func TestBrokerGroupApprovalRequiresReusableCalleeBinding(t *testing.T) {
	a := brokerApprovalFixture()
	a.Mode = "controller"
	a.Phase = "create"
	a.GroupPolicy = groupPolicyFixture()
	a.ControllerHarnessSHA = strings.Repeat("e", 40)
	c := controllerApproval{GroupPolicy: a.GroupPolicy, AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: a.ControllerHarnessSHA, WorkflowSHA: strings.Repeat("b", 40), WorkflowRef: "refs/heads/main", WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create"}}
	raw, _ := json.Marshal(c)
	a.ControllerApprovalSHA256 = handoff.Digest(raw)
	req, e := brokerProvenanceRequest(a, c, workflowProvenanceSource)
	if e != nil {
		t.Fatal(e)
	}
	p := handoff.PublicPolicy{Version: 1, Role: "broker", Source: workflowProvenanceSource, WorkflowKeyID: "workflow-v1", WorkflowPublicKey: "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8", IssuerKeyID: "issuer-v1", IssuerPublicKey: "e28a8970753332bd72fef413e6b0b2ef1b4aadda7aa2c141f233712a6876b351", Grants: []handoff.PhaseGrant{{Request: req, Mode: "controller", OrganizationID: 101, RepositoryID: 501, ExpiresAt: a.ExpiresAt}}}
	policy, _ := json.Marshal(p)
	old := brokerExperimentPolicyJSON
	brokerExperimentPolicyJSON = string(policy)
	defer func() { brokerExperimentPolicyJSON = old }()
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private fixture")
	}
	dir, _ = filepath.EvalSymlinks(dir)
	files := BrokerFiles{ApprovalPath: filepath.Join(dir, "broker.json"), ControllerApproval: filepath.Join(dir, "controller.json")}
	b, _ := json.Marshal(a)
	if os.WriteFile(files.ApprovalPath, b, 0600) != nil || os.WriteFile(files.ControllerApproval, raw, 0600) != nil {
		t.Fatal("private fixture")
	}
	if _, e = selectBrokerExperiment(files, time.Now()); e == nil {
		t.Fatal("new group approval selected by v1 policy")
	}
	p.Version = 2
	p.Grants[0].GroupPolicySHA256 = a.GroupPolicy.GrantDigest(req, a.RepositoryID, a.RunnerGroupID, a.ManagementScope)
	policy, _ = json.Marshal(p)
	brokerExperimentPolicyJSON = string(policy)
	if _, e = selectBrokerExperiment(files, time.Now()); e == nil {
		t.Fatal("caller-only v2 policy selected a reusable-workflow group")
	}
	oldAPI := brokerExperimentFixtureAPI
	var apiBuilds, httpCalls int
	brokerExperimentFixtureAPI = func(handoff.TrustRoot) *brokerAPI {
		apiBuilds++
		return newBrokerAPI(time.Now, transportFunc(func(*http.Request) (*http.Response, error) {
			httpCalls++
			return nil, errBroker
		}))
	}
	defer func() { brokerExperimentFixtureAPI = oldAPI }()
	var inputs [4]*os.File
	var copies [4]*os.File
	for i := range inputs {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		fd, err := syscall.Dup(int(r.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		inputs[i], copies[i] = r, os.NewFile(uintptr(fd), "caller-only-policy-inspection")
		defer copies[i].Close()
		if _, err = w.Write([]byte("unconsumed")); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	result, err := RunBrokerExperimentWithInputs(context.Background(), files, inputs[0], inputs[1], inputs[2], inputs[3])
	if err == nil || result.Status != "" {
		t.Fatalf("caller-only v2 entrypoint result=%+v error=%v, want refusal", result, err)
	}
	if apiBuilds != 0 || httpCalls != 0 {
		t.Fatalf("caller-only v2 crossed broker pre-effect boundary: API builds=%d HTTP calls=%d", apiBuilds, httpCalls)
	}
	for _, file := range copies {
		data, err := io.ReadAll(file)
		if err != nil || string(data) != "unconsumed" {
			t.Fatal("caller-only v2 consumed inherited input")
		}
	}
	for _, fault := range []string{"literal", "group", "organization-id", "scope", "raw-bytes", "missing", "version", "grant", "default"} {
		t.Run(fault, func(t *testing.T) {
			x, y, q := a, c, p
			q.Grants = append([]handoff.PhaseGrant(nil), p.Grants...)
			switch fault {
			case "literal":
				x.GroupPolicy.Workflow = strings.Replace(x.GroupPolicy.Workflow, "dddd", "eeee", 1)
				y.GroupPolicy = x.GroupPolicy
			case "group":
				x.RunnerGroupID++
				y.RunnerGroupID++
			case "organization-id":
				x.OrganizationID++
				x.GroupPolicy.OrganizationID++
				y.GroupPolicy = x.GroupPolicy
			case "scope":
				x.ManagementScope = "organization-only"
				y.ManagementScope = x.ManagementScope
			case "missing":
				x.GroupPolicy = handoff.GroupPolicy{}
				y.GroupPolicy = x.GroupPolicy
			case "version":
				q.Version = 1
			case "grant":
				q.Grants[0].GroupPolicySHA256 = strings.Repeat("f", 64)
			}
			yraw, _ := json.Marshal(y)
			if fault == "raw-bytes" {
				yraw = append(yraw, '\n')
			}
			x.ControllerApprovalSHA256 = handoff.Digest(yraw)
			xraw, _ := json.Marshal(x)
			qraw, _ := json.Marshal(q)
			brokerExperimentPolicyJSON = string(qraw)
			if fault == "default" {
				brokerExperimentPolicyJSON = ""
			}
			if os.WriteFile(files.ApprovalPath, xraw, 0600) != nil || os.WriteFile(files.ControllerApproval, yraw, 0600) != nil {
				t.Fatal("fixture rewrite")
			}
			if _, e := selectBrokerExperiment(files, time.Now()); e == nil {
				t.Fatal("changed approval selected by stale group authority")
			}
		})
	}
}

func groupPolicyResponse(fault string, p handoff.GroupPolicy) (int, string) {
	g := map[string]any{"id": 3, "name": "synthetic-group", "visibility": "selected", "default": false, "inherited": false, "allows_public_repositories": false, "restricted_to_workflows": true, "selected_workflows": []string{p.Workflow}}
	switch fault {
	case "legacy":
		g["restricted_to_workflows"] = "ignored-v1"
		g["selected_workflows"] = 123
	case "missing":
		delete(g, "restricted_to_workflows")
	case "null":
		g["restricted_to_workflows"] = nil
	case "false":
		g["restricted_to_workflows"] = false
	case "empty":
		g["selected_workflows"] = []string{}
	case "null-list":
		g["selected_workflows"] = nil
	case "missing-list":
		delete(g, "selected_workflows")
	case "extra":
		g["selected_workflows"] = []string{p.Workflow, p.Workflow}
	case "wrong":
		g["selected_workflows"] = []string{strings.Replace(p.Workflow, "dddd", "eeee", 1)}
	case "caller":
		g["selected_workflows"] = []string{"org-a/canary/.github/workflows/canary.yml@" + strings.Repeat("b", 40)}
	case "branch":
		g["selected_workflows"] = []string{"org-a/canary/.github/workflows/runner.yml@refs/heads/main"}
	case "tag":
		g["selected_workflows"] = []string{"org-a/canary/.github/workflows/runner.yml@v1"}
	case "group":
		g["id"] = 4
	case "403":
		return 403, "{}"
	}
	b, _ := json.Marshal(g)
	return 200, string(b)
}

func TestBrokerExactGroupPolicyPreflight(t *testing.T) {
	for _, scope := range []string{"", "organization-only"} {
		for _, fault := range []string{"", "missing", "null", "false", "empty", "null-list", "missing-list", "extra", "wrong", "caller", "branch", "tag", "group", "403"} {
			t.Run(scope+"/"+fault, func(t *testing.T) {
				a, candidate, api, f, root := newBrokerFixture(t)
				if scope != "" {
					a, candidate, api, f, root = organizationScopeFixture(t, nil)
				}
				a.GroupPolicy = groupPolicyFixture()
				a.Mode = "controller"
				a.Phase = "create"
				launches := 0
				plan := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { launches++; return nil })
				base := api.client.Transport
				api.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.Path == "/orgs/org-a/actions/runner-groups/3" {
						f.calls = append(f.calls, r.Method+" "+r.URL.Path)
						status, body := groupPolicyResponse(fault, a.GroupPolicy)
						return response(status, body), nil
					}
					return base.RoundTrip(r)
				})
				_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM)}, root, api, plan)
				if (err == nil) != (fault == "") {
					t.Fatalf("group policy accepted=%v fault=%s", err == nil, fault)
				}
				if fault != "" && launches != 0 {
					t.Fatal("child launch after rejected group")
				}
				if f.tokenCalls != 1 {
					t.Fatalf("mint calls=%d; group preflight is post-mint", f.tokenCalls)
				}
				for _, call := range f.calls {
					if fault != "" && (strings.Contains(call, "registration-token") || strings.Contains(call, "runner-registration")) {
						t.Fatal("registration after rejected policy")
					}
				}
				if fault != "" {
					_, err = brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM)}, root, api, plan)
					if err == nil || f.tokenCalls != 1 {
						t.Fatal("failed policy claim retried mint")
					}
				}
			})
		}
	}
}
