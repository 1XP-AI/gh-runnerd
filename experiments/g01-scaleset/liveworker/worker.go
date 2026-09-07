// Package liveworker is a single-use, explicitly authorized Docker experiment.
// It does not dispatch jobs, obtain JIT, pull images or implement a provider.
package liveworker

import (
	"context"
	"errors"
	"time"
)

var (
	ErrApproval  = errors.New("worker approval rejected")
	ErrState     = errors.New("private worker state unavailable; retain reservation")
	ErrUncertain = errors.New("worker state uncertain; quarantine and inspect only")
	ErrRemote    = errors.New("worker runtime operation failed")
)

const ImageReference = "ghcr.io/actions/actions-runner@sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d"

type Approval struct {
	HarnessSHA  string    `json:"harness_sha"`
	WorkflowSHA string    `json:"workflow_sha"`
	OwnerNonce  string    `json:"owner_nonce"`
	Controller  string    `json:"controller"`
	Endpoint    string    `json:"endpoint"`
	DaemonID    string    `json:"daemon_id"`
	ImageID     string    `json:"image_id"`
	Image       string    `json:"image"`
	ExpiresAt   time.Time `json:"expires_at"`
	Phases      []string  `json:"phases"`
}

type Event struct {
	Sequence     int    `json:"sequence"`
	Kind         string `json:"kind"`
	Operation    string `json:"operation,omitempty"`
	ID           string `json:"id,omitempty"`
	Digest       string `json:"digest,omitempty"`
	EnvDigest    string `json:"env_digest,omitempty"`
	LabelsDigest string `json:"labels_digest,omitempty"`
}
type Journal interface {
	Events() []Event
	Append(Event) error
}

type ImageProfile struct {
	Env    []string
	Labels map[string]string
}
type Container struct {
	ID         string         `json:"Id"`
	Name       string         `json:"Name"`
	ImageID    string         `json:"Image"`
	Config     map[string]any `json:"Config"`
	HostConfig map[string]any `json:"HostConfig"`
	Mounts     []any          `json:"Mounts"`
	State      struct {
		Status                            string
		Running, Paused, Restarting, Dead bool
	} `json:"State"`
	NetworkSettings struct{ Networks map[string]any } `json:"NetworkSettings"`
}

type Runtime interface {
	Preflight(context.Context, Approval) (ImageProfile, error)
	Create(context.Context, string, map[string]any) (string, bool, error)
	Inspect(context.Context, string) (*Container, error)
	Start(context.Context, string) error
	Delete(context.Context, string) error
}

type Driver struct {
	Approval Approval
	Journal  Journal
	Runtime  Runtime
}

// Deliberately unsafe compiling red baseline, replaced before any real client.
func (d *Driver) Run(ctx context.Context, phase, jit string) error {
	_, _, err := d.Runtime.Create(ctx, "", nil)
	return err
}
