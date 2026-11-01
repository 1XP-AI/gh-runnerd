package handoff

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

// PublicPolicy describes reviewed build inputs. Parsing one cannot select the
// policy of either executable: each sealed entry reads its own compiled string.
type PublicPolicy struct {
	Version           int          `json:"version"`
	Role              string       `json:"role"`
	Source            string       `json:"source"`
	WorkflowKeyID     string       `json:"workflow_key_id"`
	WorkflowPublicKey string       `json:"workflow_public_key"`
	IssuerKeyID       string       `json:"issuer_key_id"`
	IssuerPublicKey   string       `json:"issuer_public_key"`
	Grants            []PhaseGrant `json:"grants"`
}
type PhaseGrant struct {
	GroupPolicySHA256 string    `json:"group_policy_sha256,omitempty"`
	Request           Request   `json:"request"`
	Mode              string    `json:"mode"`
	OrganizationID    int64     `json:"organization_id"`
	RepositoryID      int64     `json:"repository_id"`
	ExpiresAt         time.Time `json:"expires_at"`
}

func DecodePublicPolicy(data []byte, role string, now time.Time) (PublicPolicy, TrustRoot, IssuerRoot, error) {
	var p PublicPolicy
	fail := func() (PublicPolicy, TrustRoot, IssuerRoot, error) {
		return PublicPolicy{}, TrustRoot{}, IssuerRoot{}, ErrInvalid
	}
	if decodeExactObject(data, &p, 32768) != nil || (p.Version != 1 && p.Version != 2 && p.Version != 3) || p.Role != role || (role != "broker" && role != "controller") || ((p.Version != 3 && p.Source != "github-rest-workflow-v1") || (p.Version == 3 && p.Source != ReusableCalleeSource)) || len(p.Grants) < 1 || len(p.Grants) > 16 {
		return fail()
	}
	wk, e := hex.DecodeString(p.WorkflowPublicKey)
	ik, e2 := hex.DecodeString(p.IssuerPublicKey)
	if e != nil || e2 != nil || hex.EncodeToString(wk) != p.WorkflowPublicKey || hex.EncodeToString(ik) != p.IssuerPublicKey {
		return fail()
	}
	w, e := NewTrustRoot(p.WorkflowKeyID, wk)
	i, e2 := NewIssuerRoot(p.IssuerKeyID, ik)
	if e != nil || e2 != nil || !i.Independent(w) {
		return fail()
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	var grants []json.RawMessage
	_ = json.Unmarshal(fields["grants"], &grants)
	for _, raw := range grants {
		var g PhaseGrant
		if p.Version >= 2 {
			g.GroupPolicySHA256 = strings.Repeat("0", 64)
		}
		if p.Version == 3 {
			g.Request.Version = 2
			g.Request.Callee = CalleeIdentity{Repository: "placeholder"}
		}
		if decodeExactObject(raw, &g, 32768) != nil {
			return fail()
		}
		var f map[string]json.RawMessage
		_ = json.Unmarshal(raw, &f)
		var req Request
		if p.Version == 3 {
			req.Version = 2
			req.Callee = CalleeIdentity{Repository: "placeholder"}
		}
		if decodeExactObject(f["request"], &req, 4096) != nil {
			return fail()
		}
		if p.Version == 3 {
			var requestFields map[string]json.RawMessage
			_ = json.Unmarshal(f["request"], &requestFields)
			var callee CalleeIdentity
			if decodeExactObject(requestFields["callee"], &callee, 4096) != nil {
				return fail()
			}
		}
	}
	seen := map[Request]bool{}
	for _, g := range p.Grants {
		// Strict nested fields and duplicate keys are checked independently too.
		if (p.Version == 1 && g.GroupPolicySHA256 != "") || (p.Version >= 2 && !sha256Hex.MatchString(g.GroupPolicySHA256)) || (p.Version == 3 && g.Request.Version != 2) || !g.Request.Valid() || g.Request.Source != p.Source || !strings.HasPrefix(g.Request.WorkflowRef, "refs/heads/") || g.OrganizationID < 1 || g.RepositoryID < 1 || !g.ExpiresAt.After(now) || g.ExpiresAt.After(now.Add(24*time.Hour)) || seen[g.Request] {
			return fail()
		}
		if !(g.Mode == "controller" && g.Request.Phase == "create") && PhaseAudience(g.Mode, g.Request.Phase) == "" {
			return fail()
		}
		key := g.Request
		if p.Version == 3 {
			key.Callee = CalleeIdentity{}
			if seen[key] {
				return fail()
			}
		}
		seen[key] = true
	}
	return p, w, i, nil
}
func (p PublicPolicy) Select(mode string, request Request, now time.Time) (PhaseGrant, error) {
	return p.SelectGroup(mode, request, "", now)
}

// SelectGroup has no legacy fallback: a nonempty digest requires v2, and v2
// requires the independently granted digest of the exact group approval.
func (p PublicPolicy) SelectGroup(mode string, request Request, digest string, now time.Time) (PhaseGrant, error) {
	if (p.Version != 1 || digest != "") && (p.Version != 2 || !sha256Hex.MatchString(digest)) {
		return PhaseGrant{}, ErrInvalid
	}
	for _, g := range p.Grants {
		if g.GroupPolicySHA256 == digest && g.Mode == mode && g.Request == request && g.ExpiresAt.After(now) {
			return g, nil
		}
	}
	return PhaseGrant{}, ErrInvalid
}

// SelectCallee selects a v3 grant using independently supplied caller fields.
// The caller must leave Callee empty; the selected compiled grant supplies it.
// groupPolicy and target inputs must come from independent approval, never a
// provider response or receipt. The digest binds those inputs to the full grant.
func (p PublicPolicy) SelectCallee(mode string, caller Request, organizationID, repositoryID, groupID int64, managementScope string, groupPolicy GroupPolicy, now time.Time) (PhaseGrant, error) {
	if p.Version != 3 || p.Source != ReusableCalleeSource || caller.Version != 2 || caller.Source != ReusableCalleeSource || caller.Callee != (CalleeIdentity{}) || organizationID < 1 || repositoryID < 1 || groupID < 1 || groupPolicy == (GroupPolicy{}) || groupPolicy.OrganizationID != organizationID {
		return PhaseGrant{}, ErrInvalid
	}
	var result PhaseGrant
	found := false
	for _, g := range p.Grants {
		key := g.Request
		key.Callee = CalleeIdentity{}
		if key == caller && g.Mode == mode && g.OrganizationID == organizationID && g.RepositoryID == repositoryID && g.ExpiresAt.After(now) && g.Request.Valid() && groupPolicy.Valid(g.Request.Repository, g.Request.WorkflowPath) && groupPolicy.Workflow == g.Request.Callee.Repository+"/"+g.Request.Callee.WorkflowPath+"@"+g.Request.Callee.SHA && g.GroupPolicySHA256 != "" && g.GroupPolicySHA256 == groupPolicy.GrantDigest(g.Request, repositoryID, groupID, managementScope) {
			if found {
				return PhaseGrant{}, ErrInvalid
			}
			result, found = g, true
		}
	}
	if !found {
		return PhaseGrant{}, ErrInvalid
	}
	return result, nil
}
