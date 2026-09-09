package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func unresolvedResponse(endpoint string) observationResponse {
	return observationResponse{Endpoint: endpoint, Outcome: observationUnresolved}
}

func (a *SDKAPI) observationContext(ctx context.Context, verification bool) (context.Context, context.CancelFunc, error) {
	now := time.Now()
	if a.approval.Validate(now) != nil || a.credentials.validate(a.approval, now) != nil || ctx.Err() != nil {
		return nil, nil, ErrApproval
	}
	if verification && (len(a.credentials.VerificationToken) < 20 || len(a.credentials.VerificationToken) > 1024 || a.credentials.VerificationToken == a.credentials.InstallationToken || strings.ContainsAny(a.credentials.VerificationToken, "\r\n\x00")) {
		return nil, nil, ErrApproval
	}
	deadline := now.Add(operationTimeout)
	if a.approval.ExpiresAt.Before(deadline) {
		deadline = a.approval.ExpiresAt
	}
	if a.credentials.ExpiresAt.Before(deadline) {
		deadline = a.credentials.ExpiresAt
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	return bounded, cancel, nil
}

// Unlike the existing fault harness get(), this new reader rejects ambiguous
// JSON keys while accepting unrelated forward-compatible GitHub fields.
func (a *SDKAPI) observationGET(ctx context.Context, path, token, endpoint string, target any) (observationResponse, error) {
	out := unresolvedResponse(endpoint)
	if a.rest == nil || ctx.Err() != nil || !a.credentials.ExpiresAt.After(time.Now()) {
		return out, ErrRemote
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return out, ErrRemote
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := a.rest.Do(req)
	if err != nil {
		return out, ErrRemote
	}
	defer resp.Body.Close()
	out.Status = resp.StatusCode
	data, err := io.ReadAll(io.LimitReader(resp.Body, responseBodyLimit+1))
	if err != nil || int64(len(data)) > responseBodyLimit || ctx.Err() != nil {
		return out, ErrRemote
	}
	if resp.StatusCode == http.StatusNotFound {
		out.Outcome = observationNotFound
		return out, nil
	}
	if resp.StatusCode != http.StatusOK || len(resp.Header.Values("Link")) != 0 {
		return out, ErrRemote
	}
	if !uniqueKeys(json.NewDecoder(bytes.NewReader(data))) || json.Unmarshal(data, target) != nil {
		return out, ErrRemote
	}
	return out, nil
}

type observedRepository struct {
	ID      int64 `json:"id"`
	Private *bool `json:"private"`
	Fork    *bool `json:"fork"`
}

func (r observedRepository) policyValue() repository {
	return repository{ID: r.ID, Private: r.Private != nil && *r.Private, Fork: r.Fork != nil && *r.Fork}
}
func (a *SDKAPI) observeApprovedRun(ctx context.Context) (observationResponse, error) {
	return a.observeApprovedRunFor(ctx, a.approval, a.approval.WorkflowRunID)
}

func (a *SDKAPI) observeApprovedRunFor(ctx context.Context, approval Approval, id int64) (observationResponse, error) {
	var wire struct {
		ID             int64              `json:"id"`
		HeadSHA        string             `json:"head_sha"`
		Event          string             `json:"event"`
		Path           string             `json:"path"`
		RunAttempt     int                `json:"run_attempt"`
		Repository     observedRepository `json:"repository"`
		HeadRepository observedRepository `json:"head_repository"`
	}
	out, err := a.observationGET(ctx, "/repos/"+approval.Organization+"/"+approval.Repository+"/actions/runs/"+strconv.FormatInt(id, 10), a.credentials.VerificationToken, "rest_run", &wire)
	if err != nil || out.Outcome == observationNotFound {
		return out, err
	}
	run := workflowRun{ID: wire.ID, HeadSHA: wire.HeadSHA, Event: wire.Event, Path: wire.Path, RunAttempt: wire.RunAttempt, Repository: wire.Repository.policyValue(), HeadRepository: wire.HeadRepository.policyValue()}
	if wire.Repository.Private == nil || wire.Repository.Fork == nil || wire.HeadRepository.Private == nil || wire.HeadRepository.Fork == nil || !matchesApprovedRun(approval, id, run) {
		return out, ErrRemote
	}
	return out, nil
}

// A context-local collector sees only this exact SDK target GET, not its token
// bootstrap or another invocation. It retains no URL, body or header values.
type runnerResponseKey struct{}
type runnerResponseCapture struct {
	suffix        string
	mu            sync.Mutex
	count, status int
}

func captureRunnerResponse(req *http.Request, status int) {
	c, _ := req.Context().Value(runnerResponseKey{}).(*runnerResponseCapture)
	if c == nil || req.Method != http.MethodGet || !strings.HasSuffix(req.URL.Path, c.suffix) || req.URL.EscapedPath() != req.URL.Path || req.URL.Fragment != "" {
		return
	}
	q := req.URL.Query()
	if len(q) != 1 || len(q["api-version"]) != 1 || q.Get("api-version") != "6.0-preview" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	c.status = status
}
func (c *runnerResponseCapture) result() (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status, c.count == 1
}
