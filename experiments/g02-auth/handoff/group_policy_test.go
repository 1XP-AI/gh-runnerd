package handoff

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGroupPolicyRequiresSeparateGrantContract(t *testing.T) {
	p := phasePolicyFixture(t)
	p.Grants[0].GroupPolicySHA256 = strings.Repeat("a", 64)
	b, _ := json.Marshal(p)
	if _, _, _, e := DecodePublicPolicy(b, "broker", time.Now()); e == nil {
		t.Fatal("v1 policy accepted v2 group authority")
	}
	p.Version = 2
	b, _ = json.Marshal(p)
	decoded, _, _, e := DecodePublicPolicy(b, "broker", time.Now())
	if e != nil {
		t.Fatal("explicit v2 group policy rejected", e)
	}
	if _, e = decoded.Select("controller", p.Grants[0].Request, time.Now()); e == nil {
		t.Fatal("v2 selected through legacy selector")
	}
}

func TestGroupPolicyStrictLiteralAndDigest(t *testing.T) {
	p := GroupPolicy{Version: 2, OrganizationID: 101, Workflow: "fixture-org/canary/.github/workflows/runner.yml@" + strings.Repeat("d", 40)}
	r := phasePolicyFixture(t).Grants[0].Request
	if !p.Valid(r.Repository, r.WorkflowPath) {
		t.Fatal("synthetic literal rejected")
	}
	d := p.GrantDigest(r, 42, 3, "")
	if len(d) != 64 {
		t.Fatal("missing group authority digest")
	}
	for _, fault := range []string{"workflow", "org-id", "group", "repository-id", "scope", "caller-bytes", "request-phase"} {
		t.Run(fault, func(t *testing.T) {
			x, q, repo, group, scope := p, r, int64(42), int64(3), ""
			switch fault {
			case "workflow":
				x.Workflow = strings.Replace(x.Workflow, "dddd", "eeee", 1)
			case "org-id":
				x.OrganizationID++
			case "group":
				group++
			case "repository-id":
				repo++
			case "scope":
				scope = "organization-only"
			case "caller-bytes":
				q.ControllerApprovalSHA256 = strings.Repeat("f", 64)
			case "request-phase":
				q.Phase = "inspect"
			}
			if x.GrantDigest(q, repo, group, scope) == d {
				t.Fatal("group digest omitted approved field")
			}
		})
	}
	for _, literal := range []string{"", "fixture-org/canary/.github/workflows/runner.yml@main", "fixture-org/canary/.github/workflows/runner.yml@refs/tags/v1", "other/canary/.github/workflows/runner.yml@" + strings.Repeat("d", 40), "fixture-org/canary/../runner.yml@" + strings.Repeat("d", 40), r.Repository + "/" + r.WorkflowPath + "@" + r.WorkflowSHA} {
		x := p
		x.Workflow = literal
		if x.Valid(r.Repository, r.WorkflowPath) {
			t.Fatal("invalid or caller literal accepted")
		}
	}
	b, _ := json.Marshal(p)
	for _, raw := range []string{"null", "{}", strings.Replace(string(b), `"version":2`, `"version":1`, 1), strings.Replace(string(b), `"organization_id":101`, `"organization_id":null`, 1), strings.Replace(string(b), `"version":2`, `"Version":2`, 1), strings.Replace(string(b), `"version":2`, `"version":2,"VERSION":2`, 1)} {
		var x GroupPolicy
		if json.Unmarshal([]byte(raw), &x) == nil {
			t.Fatal("ambiguous or missing group approval accepted")
		}
	}
}
