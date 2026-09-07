// Package livecanary is an isolated, explicitly authorized controller experiment.
// It does not launch workers or dispatch workflows.
package livecanary

import (
	"context"
	"errors"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
)

var (
	ErrApproval = errors.New("approval rejected")
	ErrJournal = errors.New("journal unavailable; stop and retain resources")
	ErrQuarantine = errors.New("state uncertain; quarantine and inspect only")
	ErrRemote = errors.New("remote operation failed; inspect private state")
	ErrBarrier = errors.New("planned barrier reached")
)

type Approval struct {
	Organization string `json:"organization"`
	Repository string `json:"repository"`
	RepositoryID int64 `json:"repository_id"`
	RunnerGroupID int `json:"runner_group_id"`
	OwnerNonce string `json:"owner_nonce"`
	HarnessSHA string `json:"harness_sha"`
	WorkflowSHA string `json:"workflow_sha"`
	WorkflowPath string `json:"workflow_path"`
	WorkflowRunID int64 `json:"workflow_run_id"`
	Controller string `json:"controller"`
	ExpiresAt time.Time `json:"expires_at"`
	ActionsHosts []string `json:"actions_hosts"`
	Phases []string `json:"phases"`
}

type Event struct {
	Sequence int `json:"sequence"`
	Kind string `json:"kind"`
	Operation string `json:"operation,omitempty"`
	ID int `json:"id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	RequestIDs []int64 `json:"request_ids,omitempty"`
	Count int `json:"count,omitempty"`
	Digest string `json:"digest,omitempty"`
}

type Journal interface {
	Events() []Event
	Append(Event) error
}

type Session interface {
	listener.Client
	Close(context.Context) error
}

type API interface {
	Preflight(context.Context, Approval) error
	Inventory(context.Context) (string, error)
	FindScaleSet(context.Context, string, int) (*scaleset.RunnerScaleSet, error)
	GetScaleSet(context.Context, int) (*scaleset.RunnerScaleSet, error)
	CreateScaleSet(context.Context, *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error)
	DeleteScaleSet(context.Context, int) error
	OpenSession(context.Context, int, string) (Session, error)
	FindRunner(context.Context, string) (*scaleset.RunnerReference, error)
	GenerateJIT(context.Context, int, string) (*scaleset.RunnerScaleSetJitRunnerConfig, error)
	VerifyRun(context.Context, Approval, int64) error
}

type Driver struct { Approval Approval; Journal Journal; API API }

// Run is the deliberately unsafe compiling red baseline; it is replaced before
// any live command is introduced. No real API implementation exists at this step.
func (d *Driver) Run(ctx context.Context, phase string) error {
	_, err := d.API.CreateScaleSet(ctx, &scaleset.RunnerScaleSet{})
	return err
}
