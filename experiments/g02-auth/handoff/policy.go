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
	Request        Request   `json:"request"`
	Mode           string    `json:"mode"`
	OrganizationID int64     `json:"organization_id"`
	RepositoryID   int64     `json:"repository_id"`
	ExpiresAt      time.Time `json:"expires_at"`
}

func DecodePublicPolicy(data []byte, role string, now time.Time) (PublicPolicy, TrustRoot, IssuerRoot, error) {
	var p PublicPolicy
	fail := func() (PublicPolicy, TrustRoot, IssuerRoot, error) {
		return PublicPolicy{}, TrustRoot{}, IssuerRoot{}, ErrInvalid
	}
	if decodeExactObject(data, &p, 32768) != nil || p.Version != 1 || p.Role != role || (role != "broker" && role != "controller") || p.Source != "github-rest-workflow-v1" || len(p.Grants) < 1 || len(p.Grants) > 16 {
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
		if decodeExactObject(raw, &g, 32768) != nil {
			return fail()
		}
		var f map[string]json.RawMessage
		_ = json.Unmarshal(raw, &f)
		var req Request
		if decodeExactObject(f["request"], &req, 4096) != nil {
			return fail()
		}
	}
	seen := map[Request]bool{}
	for _, g := range p.Grants {
		// Strict nested fields and duplicate keys are checked independently too.
		if !g.Request.Valid() || g.Request.Source != p.Source || !strings.HasPrefix(g.Request.WorkflowRef, "refs/heads/") || g.OrganizationID < 1 || g.RepositoryID < 1 || !g.ExpiresAt.After(now) || g.ExpiresAt.After(now.Add(24*time.Hour)) || seen[g.Request] {
			return fail()
		}
		if !(g.Mode == "controller" && g.Request.Phase == "create") && PhaseAudience(g.Mode, g.Request.Phase) == "" {
			return fail()
		}
		seen[g.Request] = true
	}
	return p, w, i, nil
}
func (p PublicPolicy) Select(mode string, request Request, now time.Time) (PhaseGrant, error) {
	for _, g := range p.Grants {
		if g.Mode == mode && g.Request == request && g.ExpiresAt.After(now) {
			return g, nil
		}
	}
	return PhaseGrant{}, ErrInvalid
}
