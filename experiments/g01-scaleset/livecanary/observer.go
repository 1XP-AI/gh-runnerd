package livecanary

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/actions/scaleset"
)

// Private facts for a future authorized caller, never ownership or permission.
type sdkRunnerID int
type restRunnerID int64
type restJobID int64
type observationOutcome string

const (
	observationPresent    observationOutcome = "present"
	observationPending    observationOutcome = "pending"
	observationNotFound   observationOutcome = "not_found_reported"
	observationUnresolved observationOutcome = "unresolved"
)

type observationResponse struct {
	Endpoint string
	Status   int
	Outcome  observationOutcome
}
type sdkRunnerObservation struct {
	Response   observationResponse
	ID         sdkRunnerID
	Name       string
	ScaleSetID int
}
type restRunnerObservation struct {
	Response observationResponse
	ID       restRunnerID
	Name     string
	Status   string
	Busy     *bool
}
type restJobObservation struct {
	Response      observationResponse
	ID            restJobID
	RunID         int64
	Attempt       int
	HeadSHA       string
	Status        string
	Conclusion    *string
	RunnerID      *restRunnerID
	RunnerName    *string
	RunnerGroupID *int64
}

// The SDK client is used serially. Its internal mutex is not cancellable; the
// shared context bounds network work, not arbitrary concurrent SDK lock waits.
func (a *SDKAPI) observeSDKRunner(ctx context.Context, id sdkRunnerID, name string, setID int) (sdkRunnerObservation, error) {
	out := sdkRunnerObservation{Response: unresolvedResponse("sdk_runner")}
	if id <= 0 || setID <= 0 || !observationName(name) || a.client == nil {
		return out, ErrApproval
	}
	ctx, cancel, err := a.observationContext(ctx, false)
	if err != nil {
		return out, err
	}
	defer cancel()
	capture := &runnerResponseCapture{suffix: "/_apis/distributedtask/pools/0/agents/" + strconv.Itoa(int(id))}
	ctx = context.WithValue(ctx, runnerResponseKey{}, capture)
	ref, callErr := a.client.GetRunner(ctx, int(id))
	status, single := capture.result()
	out.Response.Status = status
	if !single || ctx.Err() != nil {
		return out, ErrRemote
	}
	if status == http.StatusNotFound && errors.Is(callErr, scaleset.RunnerNotFoundError) {
		out.Response.Outcome = observationNotFound
		return out, nil
	}
	if callErr != nil || status != http.StatusOK || ref == nil || ref.ID != int(id) || ref.Name != name || ref.RunnerScaleSetID != setID {
		return out, ErrRemote
	}
	out.Response.Outcome = observationPresent
	out.ID, out.Name, out.ScaleSetID = id, ref.Name, ref.RunnerScaleSetID
	return out, nil
}

func (a *SDKAPI) observeRESTRunner(ctx context.Context, id restRunnerID, name string) (restRunnerObservation, error) {
	out := restRunnerObservation{Response: unresolvedResponse("rest_runner")}
	if id <= 0 || !observationName(name) {
		return out, ErrApproval
	}
	ctx, cancel, err := a.observationContext(ctx, false)
	if err != nil {
		return out, err
	}
	defer cancel()
	var wire struct {
		ID     restRunnerID `json:"id"`
		Name   string       `json:"name"`
		Status string       `json:"status"`
		Busy   *bool        `json:"busy"`
	}
	path := "/orgs/" + a.approval.Organization + "/actions/runners/" + strconv.FormatInt(int64(id), 10)
	out.Response, err = a.observationGET(ctx, path, a.credentials.InstallationToken, "rest_runner", &wire)
	if err != nil || out.Response.Outcome == observationNotFound {
		return out, err
	}
	if wire.ID != id || wire.Name != name || wire.Busy == nil || (wire.Status != "online" && wire.Status != "offline") {
		return out, ErrRemote
	}
	out.ID, out.Name, out.Status, out.Busy = wire.ID, wire.Name, wire.Status, wire.Busy
	out.Response.Outcome = observationPresent
	return out, nil
}

type observedJobWire struct {
	ID            restJobID     `json:"id"`
	RunID         int64         `json:"run_id"`
	RunAttempt    *int          `json:"run_attempt"`
	HeadSHA       string        `json:"head_sha"`
	Status        string        `json:"status"`
	Conclusion    *string       `json:"conclusion"`
	RunnerID      *restRunnerID `json:"runner_id"`
	RunnerName    *string       `json:"runner_name"`
	RunnerGroupID *int64        `json:"runner_group_id"`
}

func (a *SDKAPI) observeRESTJob(ctx context.Context, attempt int, previous restJobID) (restJobObservation, error) {
	out := restJobObservation{Response: unresolvedResponse("rest_run")}
	if attempt != 1 || previous < 0 || a.approval.WorkflowRunID <= 0 {
		return out, ErrApproval
	}
	ctx, cancel, err := a.observationContext(ctx, true)
	if err != nil {
		return out, err
	}
	defer cancel()
	out.Response, err = a.observeApprovedRun(ctx)
	if err != nil || out.Response.Outcome == observationNotFound {
		return out, err
	}
	prefix := "/repos/" + a.approval.Organization + "/" + a.approval.Repository + "/actions/"
	var list struct {
		Count *int               `json:"total_count"`
		Jobs  *[]observedJobWire `json:"jobs"`
	}
	out.Response, err = a.observationGET(ctx, prefix+"runs/"+strconv.FormatInt(a.approval.WorkflowRunID, 10)+"/attempts/1/jobs?per_page=2&page=1", a.credentials.VerificationToken, "rest_attempt_jobs", &list)
	if err != nil || out.Response.Outcome == observationNotFound {
		return out, err
	}
	if list.Count == nil || list.Jobs == nil || *list.Count != len(*list.Jobs) || len(*list.Jobs) > 1 {
		return out, ErrRemote
	}
	if len(*list.Jobs) == 0 {
		out.Response.Outcome = observationPending
		return out, nil
	}
	candidate := (*list.Jobs)[0]
	if !candidate.valid(a.approval) || (previous != 0 && candidate.ID != previous) {
		return out, ErrRemote
	}
	var detail observedJobWire
	out.Response, err = a.observationGET(ctx, prefix+"jobs/"+strconv.FormatInt(int64(candidate.ID), 10), a.credentials.VerificationToken, "rest_job", &detail)
	if err != nil || out.Response.Outcome == observationNotFound {
		return out, err
	}
	// The list's path fixes its attempt, but this unscoped exact GET must
	// explicitly corroborate it. The optional wire field is required by this
	// experiment's evidence policy, not guaranteed to be present by GitHub.
	if detail.RunAttempt == nil || !detail.valid(a.approval) || detail.ID != candidate.ID || !samePositiveAssociation(candidate, detail) || !jobProgresses(candidate, detail) {
		return out, ErrRemote
	}
	out.ID, out.RunID, out.Attempt, out.HeadSHA = detail.ID, detail.RunID, *detail.RunAttempt, detail.HeadSHA
	out.Status, out.Conclusion = detail.Status, detail.Conclusion
	out.RunnerID, out.RunnerName, out.RunnerGroupID = detail.RunnerID, detail.RunnerName, detail.RunnerGroupID
	out.Response.Outcome = observationPending
	if detail.RunnerID != nil && *detail.RunnerID > 0 && detail.RunnerName != nil && *detail.RunnerName != "" && detail.RunnerGroupID != nil && *detail.RunnerGroupID > 0 {
		out.Response.Outcome = observationPresent
	}
	return out, nil
}

func (j observedJobWire) valid(a Approval) bool {
	if j.ID <= 0 || j.RunID != a.WorkflowRunID || j.HeadSHA != a.WorkflowSHA || (j.RunAttempt != nil && *j.RunAttempt != 1) {
		return false
	}
	switch j.Status {
	case "queued", "in_progress", "completed":
	default:
		return false
	}
	if j.Conclusion != nil {
		switch *j.Conclusion {
		case "success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required", "stale", "startup_failure":
		default:
			return false
		}
	}
	if (j.Status == "completed") != (j.Conclusion != nil) {
		return false
	}
	if j.RunnerID != nil && *j.RunnerID < 0 {
		return false
	}
	if j.RunnerName != nil && *j.RunnerName != "" && !observationName(*j.RunnerName) {
		return false
	}
	return j.RunnerGroupID == nil || *j.RunnerGroupID == 0 || *j.RunnerGroupID == int64(a.RunnerGroupID)
}

// Within this one sample, positive list associations cannot be contradicted or
// retracted by the exact GET. No history is merged between invocations.
func samePositiveAssociation(first, second observedJobWire) bool {
	if first.RunnerID != nil && *first.RunnerID > 0 && (second.RunnerID == nil || *first.RunnerID != *second.RunnerID) {
		return false
	}
	if first.RunnerName != nil && *first.RunnerName != "" && (second.RunnerName == nil || *first.RunnerName != *second.RunnerName) {
		return false
	}
	return first.RunnerGroupID == nil || *first.RunnerGroupID == 0 || (second.RunnerGroupID != nil && *first.RunnerGroupID == *second.RunnerGroupID)
}
func observationName(name string) bool {
	return len(name) > 0 && len(name) <= 256 && utf8.ValidString(name) && !strings.ContainsFunc(name, unicode.IsControl)
}

// One list/detail sample may progress, but cannot forget a terminal outcome or
// regress to an earlier state. This does not retain history across samples.
func jobProgresses(first, second observedJobWire) bool {
	rank := func(status string) int {
		switch status {
		case "queued":
			return 0
		case "in_progress":
			return 1
		case "completed":
			return 2
		}
		return -1
	}
	if rank(second.Status) < rank(first.Status) {
		return false
	}
	return first.Status != "completed" || (first.Conclusion != nil && second.Conclusion != nil && *first.Conclusion == *second.Conclusion)
}
