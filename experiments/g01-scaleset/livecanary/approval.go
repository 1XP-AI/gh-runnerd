package livecanary

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

var component = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,99}$`)
var nonce = regexp.MustCompile(`^[a-f0-9]{32}$`)
var sha = regexp.MustCompile(`^[a-f0-9]{40}$`)
var actionsHost = regexp.MustCompile(`^[a-z0-9-]+(?:\.[a-z0-9-]+)*\.actions\.githubusercontent\.com$`)
var workflowPath = regexp.MustCompile(`^\.github/workflows/[a-zA-Z0-9_-]+\.ya?ml$`)
var phases = []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "jit-loss", "inspect", "cleanup"}

func (a Approval) setName() string    { return "g01-" + a.OwnerNonce }
func (a Approval) workerName() string { return a.setName() + "-worker-1" }

func (a Approval) Validate(now time.Time) error {
	if a.AppID <= 0 || a.InstallationID <= 0 {
		return ErrApproval
	}
	if a.Organization == ".." {
		return ErrApproval
	}
	if !component.MatchString(a.Organization) || !component.MatchString(a.Repository) || !component.MatchString(a.Controller) || a.Organization == "." || a.Repository == "." || a.Repository == ".." || a.RepositoryID <= 0 || a.RunnerGroupID <= 0 || !nonce.MatchString(a.OwnerNonce) || !sha.MatchString(a.HarnessSHA) || !sha.MatchString(a.WorkflowSHA) || !workflowPath.MatchString(a.WorkflowPath) || !a.ExpiresAt.After(now) || a.ExpiresAt.After(now.Add(24*time.Hour)) || len(a.ActionsHosts) == 0 || len(a.ActionsHosts) > 8 || len(a.Phases) == 0 {
		return ErrApproval
	}
	seen := map[string]bool{}
	for _, h := range a.ActionsHosts {
		if !actionsHost.MatchString(h) || strings.Contains(h, "..") || seen[h] {
			return ErrApproval
		}
		seen[h] = true
	}
	seen = map[string]bool{}
	for _, p := range a.Phases {
		if !slices.Contains(phases, p) || seen[p] {
			return ErrApproval
		}
		seen[p] = true
		if p != "create" && p != "inspect" && p != "cleanup" && p != "jit-loss" && a.WorkflowRunID <= 0 {
			return ErrApproval
		}
	}
	return nil
}
