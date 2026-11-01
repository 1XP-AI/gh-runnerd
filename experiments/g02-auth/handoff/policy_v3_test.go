package handoff

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPolicyV3IndependentCalleeSelection(t *testing.T) {
	now := time.Now().UTC()
	p := phasePolicyFixture(t)
	p.Version, p.Source = 3, ReusableCalleeSource
	g := &p.Grants[0]
	g.Request.Version, g.Request.Source = 2, ReusableCalleeSource
	g.Request.Callee = CalleeIdentity{Repository: g.Request.Repository, WorkflowPath: ".github/workflows/callee.yml", SHA: strings.Repeat("a", 40)}
	gp := GroupPolicy{Version: 2, OrganizationID: g.OrganizationID, Workflow: g.Request.Repository + "/" + g.Request.Callee.WorkflowPath + "@" + g.Request.Callee.SHA}
	g.GroupPolicySHA256 = gp.GrantDigest(g.Request, g.RepositoryID, 7, "organization-only")
	raw, _ := json.Marshal(p)
	decoded, _, _, err := DecodePublicPolicy(raw, "broker", now)
	if err != nil {
		t.Fatalf("valid v3 rejected: %v", err)
	}
	caller := g.Request
	caller.Callee = CalleeIdentity{}
	selected, err := decoded.SelectCallee(g.Mode, caller, g.OrganizationID, g.RepositoryID, 7, "organization-only", gp, now)
	if err != nil || selected.Request != g.Request || !selected.ExpiresAt.Equal(g.ExpiresAt) {
		t.Fatalf("selection: %+v %v", selected, err)
	}
	for name, change := range map[string]func(*Request){
		"approval":    func(r *Request) { r.ControllerApprovalSHA256 = strings.Repeat("c", 64) },
		"run":         func(r *Request) { r.WorkflowRunID++ },
		"repository":  func(r *Request) { r.Repository = "other/repo" },
		"caller path": func(r *Request) { r.WorkflowPath = ".github/workflows/other.yml" },
		"caller sha":  func(r *Request) { r.WorkflowSHA = strings.Repeat("e", 40) }, "phase": func(r *Request) { r.Phase = "inspect" },
		"version": func(r *Request) { r.Version = 0 }, "source": func(r *Request) { r.Source = "github-rest-workflow-v1" },
		"injected callee": func(r *Request) { r.Callee = g.Request.Callee },
	} {
		t.Run(name, func(t *testing.T) {
			x := caller
			change(&x)
			if _, e := decoded.SelectCallee(g.Mode, x, g.OrganizationID, g.RepositoryID, 7, "organization-only", gp, now); e == nil {
				t.Fatal("accepted altered caller")
			}
		})
	}
	for _, tc := range []struct {
		org, repo int64
		at        time.Time
	}{{g.OrganizationID + 1, g.RepositoryID, now}, {g.OrganizationID, g.RepositoryID + 1, now}, {g.OrganizationID, g.RepositoryID, g.ExpiresAt}} {
		if _, e := decoded.SelectCallee(g.Mode, caller, tc.org, tc.repo, 7, "organization-only", gp, tc.at); e == nil {
			t.Fatal("accepted altered grant input")
		}
	}
	if _, e := decoded.SelectCallee("paired-terminal", caller, g.OrganizationID, g.RepositoryID, 7, "organization-only", gp, now); e == nil {
		t.Fatal("accepted wrong mode")
	}
	if _, e := decoded.SelectGroup(g.Mode, g.Request, g.GroupPolicySHA256, now); e == nil {
		t.Fatal("v3 accepted legacy selection")
	}
	for name, mutate := range map[string]func(*PublicPolicy){
		"cross repository":      func(x *PublicPolicy) { x.Grants[0].Request.Callee.Repository = "other/repo" },
		"missing callee":        func(x *PublicPolicy) { x.Grants[0].Request.Callee = CalleeIdentity{} },
		"wrong callee path":     func(x *PublicPolicy) { x.Grants[0].Request.Callee.WorkflowPath = "../wrong.yml" },
		"wrong callee sha":      func(x *PublicPolicy) { x.Grants[0].Request.Callee.SHA = strings.Repeat("A", 40) },
		"wrong source":          func(x *PublicPolicy) { x.Source = "github-rest-workflow-v1" },
		"wrong request version": func(x *PublicPolicy) { x.Grants[0].Request.Version = 0 },
		"policy downgrade":      func(x *PublicPolicy) { x.Version = 2 },
		"missing digest":        func(x *PublicPolicy) { x.Grants[0].GroupPolicySHA256 = "" },
		"ambiguous": func(x *PublicPolicy) {
			y := x.Grants[0]
			y.Request.Callee.SHA = strings.Repeat("d", 40)
			x.Grants = append(x.Grants, y)
		},
	} {
		t.Run(name, func(t *testing.T) {
			x := p
			x.Grants = append([]PhaseGrant(nil), p.Grants...)
			mutate(&x)
			b, _ := json.Marshal(x)
			if _, _, _, e := DecodePublicPolicy(b, "broker", now); e == nil {
				t.Fatal("accepted invalid policy")
			}
		})
	}
	for _, s := range []string{strings.Replace(string(raw), `"callee":{`, `"callee":null,"extra":{`, 1), strings.Replace(string(raw), `"sha":"`+g.Request.Callee.SHA+`"`, `"sha":null`, 1), strings.Replace(string(raw), `"sha":"`+g.Request.Callee.SHA+`"`, `"SHA":"`+g.Request.Callee.SHA+`"`, 1), strings.Replace(string(raw), `"sha":"`+g.Request.Callee.SHA+`"`, `"sha":"`+g.Request.Callee.SHA+`","extra":1`, 1), strings.Replace(string(raw), `"version":3`, `"Version":3`, 1), strings.Replace(string(raw), `"version":3`, `"version":3,"version":3`, 1), strings.Replace(string(raw), `"group_policy_sha256":"`+g.GroupPolicySHA256+`"`, `"group_policy_sha256":null`, 1)} {
		if _, _, _, e := DecodePublicPolicy([]byte(s), "broker", now); e == nil {
			t.Fatal("accepted malformed policy")
		}
	}
}

func TestPolicyLegacyWireAndDigestGolden(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	p := phasePolicyFixture(t)
	p.Grants[0].ExpiresAt = now.Add(time.Hour)
	for _, version := range []int{1, 2} {
		p.Version = version
		if version == 2 {
			p.Grants[0].GroupPolicySHA256 = strings.Repeat("a", 64)
		}
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		want := map[int]string{1: "7d0f76cc6ef2dbfa3ed60efb65460015e3a4b1de236e5784018f2b7ec4a012ba", 2: "347d0bf08e17b1da3e6d5936430ecf7d3a518ead6a0749fdb9abeb17151203b8"}[version]
		if Digest(b) != want {
			t.Fatalf("v%d policy wire changed: %s", version, Digest(b))
		}
		decoded, _, _, err := DecodePublicPolicy(b, "broker", now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = decoded.SelectGroup("controller", p.Grants[0].Request, p.Grants[0].GroupPolicySHA256, now); err != nil {
			t.Fatal(err)
		}
	}
	gp := GroupPolicy{Version: 2, OrganizationID: 101, Workflow: "fixture-org/canary/.github/workflows/runner.yml@" + strings.Repeat("d", 40)}
	if got := gp.GrantDigest(p.Grants[0].Request, 42, 3, ""); got != "3fb7a5ac1cfadccc3b1e8e23a8ecec343eb2c225984ffe59a3a7c30e6590510c" {
		t.Fatalf("group digest changed: %s", got)
	}
}

func TestPolicyV3RejectsDigestCommittedWorkflowMismatch(t *testing.T) {
	now := time.Now().UTC()
	p := phasePolicyFixture(t)
	p.Version, p.Source = 3, ReusableCalleeSource
	g := &p.Grants[0]
	g.Request.Version, g.Request.Source = 2, ReusableCalleeSource
	g.Request.Callee = CalleeIdentity{Repository: g.Request.Repository, WorkflowPath: ".github/workflows/callee.yml", SHA: strings.Repeat("a", 40)}
	gp := GroupPolicy{Version: 2, OrganizationID: g.OrganizationID, Workflow: g.Request.Repository + "/.github/workflows/other.yml@" + strings.Repeat("b", 40)}
	g.GroupPolicySHA256 = gp.GrantDigest(g.Request, g.RepositoryID, 7, "organization-only")
	if g.GroupPolicySHA256 == "" {
		t.Fatal("fixture digest invalid")
	}
	raw, _ := json.Marshal(p)
	decoded, _, _, err := DecodePublicPolicy(raw, "broker", now)
	if err != nil {
		t.Fatal(err)
	}
	caller := g.Request
	caller.Callee = CalleeIdentity{}
	if _, err := decoded.SelectCallee(g.Mode, caller, g.OrganizationID, g.RepositoryID, 7, "organization-only", gp, now); err == nil {
		t.Fatal("accepted digest-committed workflow mismatch")
	}
}

func TestPolicyV3GroupApprovalControls(t *testing.T) {
	now := time.Now().UTC()
	p := phasePolicyFixture(t)
	p.Version, p.Source = 3, ReusableCalleeSource
	g := &p.Grants[0]
	g.Request.Version, g.Request.Source = 2, ReusableCalleeSource
	g.Request.Callee = CalleeIdentity{Repository: g.Request.Repository, WorkflowPath: ".github/workflows/callee.yml", SHA: strings.Repeat("a", 40)}
	gp := GroupPolicy{Version: 2, OrganizationID: g.OrganizationID, Workflow: g.Request.Repository + "/" + g.Request.Callee.WorkflowPath + "@" + g.Request.Callee.SHA}
	g.GroupPolicySHA256 = gp.GrantDigest(g.Request, g.RepositoryID, 7, "organization-only")
	caller := g.Request
	caller.Callee = CalleeIdentity{}
	for name, change := range map[string]func(*PublicPolicy, *GroupPolicy, *int64, *int64, *string){
		"workflow path matching digest": func(p *PublicPolicy, gp *GroupPolicy, repo, group *int64, scope *string) {
			gp.Workflow = p.Grants[0].Request.Repository + "/.github/workflows/other.yml@" + strings.Repeat("a", 40)
			p.Grants[0].GroupPolicySHA256 = gp.GrantDigest(p.Grants[0].Request, *repo, *group, *scope)
		},
		"workflow sha matching digest": func(p *PublicPolicy, gp *GroupPolicy, repo, group *int64, scope *string) {
			gp.Workflow = p.Grants[0].Request.Repository + "/.github/workflows/callee.yml@" + strings.Repeat("b", 40)
			p.Grants[0].GroupPolicySHA256 = gp.GrantDigest(p.Grants[0].Request, *repo, *group, *scope)
		},
		"workflow repository": func(_ *PublicPolicy, gp *GroupPolicy, _, _ *int64, _ *string) {
			gp.Workflow = "other/repo/.github/workflows/callee.yml@" + strings.Repeat("a", 40)
		},
		"organization matching digest": func(p *PublicPolicy, gp *GroupPolicy, repo, group *int64, scope *string) {
			gp.OrganizationID++
			p.Grants[0].GroupPolicySHA256 = gp.GrantDigest(p.Grants[0].Request, *repo, *group, *scope)
		},
		"repository id":    func(_ *PublicPolicy, _ *GroupPolicy, repo, _ *int64, _ *string) { *repo++ },
		"group id":         func(_ *PublicPolicy, _ *GroupPolicy, _, group *int64, _ *string) { *group++ },
		"management scope": func(_ *PublicPolicy, _ *GroupPolicy, _, _ *int64, scope *string) { *scope = "" },
		"invalid scope":    func(_ *PublicPolicy, _ *GroupPolicy, _, _ *int64, scope *string) { *scope = "invalid" },
		"wrong digest": func(p *PublicPolicy, _ *GroupPolicy, _, _ *int64, _ *string) {
			p.Grants[0].GroupPolicySHA256 = strings.Repeat("c", 64)
		},
		"ambiguous grant": func(p *PublicPolicy, _ *GroupPolicy, _, _ *int64, _ *string) {
			p.Grants = append(p.Grants, p.Grants[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			x := p
			x.Grants = append([]PhaseGrant(nil), p.Grants...)
			approval := gp
			org, repo, group, scope := g.OrganizationID, g.RepositoryID, int64(7), "organization-only"
			change(&x, &approval, &repo, &group, &scope)
			if _, err := x.SelectCallee(g.Mode, caller, org, repo, group, scope, approval, now); err == nil {
				t.Fatal("accepted altered approval")
			}
		})
	}
}
