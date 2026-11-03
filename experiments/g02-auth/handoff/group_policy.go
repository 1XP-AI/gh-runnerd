package handoff

import (
	"encoding/json"
	"strings"
)

// GroupPolicy is independently reviewed configuration, never provider provenance.
// A nonempty v2 policy requires PublicPolicy v3 to bind the invoked reusable
// callee; the legacy v1 receipt schema remains caller-only.
type GroupPolicy struct {
	Version        int    `json:"version"`
	OrganizationID int64  `json:"organization_id"`
	Workflow       string `json:"workflow"`
}

func (p *GroupPolicy) UnmarshalJSON(data []byte) error {
	type plain GroupPolicy
	var decoded plain
	if decodeExactObject(data, &decoded, 1024) != nil || decoded.Version != 2 || decoded.OrganizationID < 1 {
		return ErrInvalid
	}
	*p = GroupPolicy(decoded)
	return nil
}

func (p GroupPolicy) Valid(repository, callerPath string) bool {
	if p == (GroupPolicy{}) {
		return true
	}
	if p.Version != 2 || p.OrganizationID < 1 || !repositoryFullName.MatchString(repository) {
		return false
	}
	parts := strings.Split(p.Workflow, "@")
	if len(parts) != 2 || !sha40Hex.MatchString(parts[1]) || !strings.HasPrefix(parts[0], repository+"/") {
		return false
	}
	path := strings.TrimPrefix(parts[0], repository+"/")
	return workflowPath.MatchString(path) && path != callerPath
}

// Decode these previously unknown response fields only for the selected v2
// contract; legacy callers retain their original unknown-field behavior.
func (p GroupPolicy) MatchesObserved(restriction, selected json.RawMessage) bool {
	if p == (GroupPolicy{}) {
		return true
	}
	var restricted bool
	var workflows []string
	return p.Version == 2 && json.Unmarshal(restriction, &restricted) == nil && restricted && json.Unmarshal(selected, &workflows) == nil && len(workflows) == 1 && workflows[0] == p.Workflow
}

// GrantDigest commits the policy to the whole caller request (including raw
// approval digest), target and management scope. Role/mode are selected by the
// independently compiled PublicPolicy and exact PhaseGrant.
func (p GroupPolicy) GrantDigest(r Request, repositoryID, groupID int64, managementScope string) string {
	if p == (GroupPolicy{}) {
		return ""
	}
	if !p.Valid(r.Repository, r.WorkflowPath) || !r.Valid() || repositoryID < 1 || groupID < 1 || (managementScope != "" && managementScope != "organization-only") {
		return ""
	}
	data, _ := json.Marshal(struct {
		Policy          GroupPolicy `json:"policy"`
		Request         Request     `json:"request"`
		RepositoryID    int64       `json:"repository_id"`
		GroupID         int64       `json:"runner_group_id"`
		ManagementScope string      `json:"management_scope"`
	}{p, r, repositoryID, groupID, managementScope})
	return Digest(append([]byte("gh-runnerd/exact-group-policy/v2\x00"), data...))
}
