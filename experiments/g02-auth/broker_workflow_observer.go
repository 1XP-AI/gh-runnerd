package enrollment

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

// The observer has only one fixed-origin, read-only operation. It holds neither
// management credentials nor APIs for issuing them.
type workflowRunObserver struct{ client *http.Client }

func newWorkflowRunObserver(fixture http.RoundTripper) *workflowRunObserver {
	return &workflowRunObserver{client: newBrokerHTTPClient(fixture)}
}

type workflowRunObservation struct {
	ID             int64            `json:"id"`
	HeadSHA        string           `json:"head_sha"`
	HeadBranch     string           `json:"head_branch"`
	Event          string           `json:"event"`
	Path           string           `json:"path"`
	RunAttempt     int              `json:"run_attempt"`
	Repository     brokerRepository `json:"repository"`
	HeadRepository brokerRepository `json:"head_repository"`
}

func workflowRunPath(repository string, runID int64) string {
	return "/repos/" + repository + "/actions/runs/" + strconv.FormatInt(runID, 10)
}

// An empty ref retains the pre-existing unattested verification behavior.
// Observation and attested corroboration always supply an exact branch ref.
func (r workflowRunObservation) matches(a BrokerApproval, runID int64, sha, path, ref string) bool {
	return r.ID == runID && r.HeadSHA == sha && r.Path == path && r.Event == "workflow_dispatch" && r.RunAttempt == 1 && r.Repository.matches(a) && r.HeadRepository.matches(a) && (ref == "" || brokerWorkflowRunHeadBranchMatchesRef(r.HeadBranch, ref))
}

func (o *workflowRunObserver) observe(ctx context.Context, grant reviewedWorkflowGrant, token string) (workflowRunObservation, error) {
	var run workflowRunObservation
	if ctx == nil || ctx.Err() != nil || o == nil || o.client == nil || !grant.valid() || !validBrokerToken(token) || len(token) > 1024 {
		return run, errBroker
	}
	parts := strings.Split(grant.Request.Repository, "/")
	a := BrokerApproval{Organization: parts[0], Repository: parts[1], OrganizationID: grant.OrganizationID, RepositoryID: grant.RepositoryID}
	api := brokerAPI{client: o.client}
	if api.call(ctx, "GET", workflowRunPath(grant.Request.Repository, grant.Request.WorkflowRunID), "Bearer "+token, nil, 200, &run) != nil || ctx.Err() != nil || !run.matches(a, grant.Request.WorkflowRunID, grant.Request.WorkflowSHA, grant.Request.WorkflowPath, grant.Request.WorkflowRef) {
		return workflowRunObservation{}, errBroker
	}
	return run, nil
}
