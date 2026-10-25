package livecanary

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// An organization-only token has no repository permission to introspect. Its
// approved runner group supplies target eligibility; it does not narrow the
// organization-wide management permission or replace workflow verification.
func (a *SDKAPI) preflightOrganizationOnly(ctx context.Context, approval Approval) error {
	if approval.Validate(time.Now()) != nil {
		return ErrApproval
	}
	var group struct {
		ID           int    `json:"id"`
		Visibility   string `json:"visibility"`
		Default      *bool  `json:"default"`
		AllowsPublic *bool  `json:"allows_public_repositories"`
		Inherited    *bool  `json:"inherited"`
	}
	prefix := "/orgs/" + approval.Organization + "/actions/runner-groups/" + strconv.Itoa(approval.RunnerGroupID)
	observed, err := a.observationGET(ctx, prefix, a.credentials.InstallationToken, "rest_group_policy", &group)
	if err != nil || observed.Status != http.StatusOK || group.ID != approval.RunnerGroupID || group.Visibility != "selected" || group.Default == nil || *group.Default || group.Inherited == nil || *group.Inherited || group.AllowsPublic == nil || *group.AllowsPublic {
		return ErrApproval
	}
	var allowed struct {
		TotalCount   int `json:"total_count"`
		Repositories []struct {
			ID       int64  `json:"id"`
			FullName string `json:"full_name"`
			Private  bool   `json:"private"`
			Fork     *bool  `json:"fork"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repositories"`
	}
	observed, err = a.observationGET(ctx, prefix+"/repositories?per_page=100", a.credentials.InstallationToken, "rest_group_repositories", &allowed)
	if err != nil || observed.Status != http.StatusOK || allowed.TotalCount != 1 || len(allowed.Repositories) != 1 {
		return ErrApproval
	}
	repo := allowed.Repositories[0]
	if repo.ID != approval.RepositoryID || repo.FullName != approval.Organization+"/"+approval.Repository || !repo.Private || repo.Fork == nil || *repo.Fork || repo.Owner.Login != approval.Organization {
		return ErrApproval
	}
	return nil
}
