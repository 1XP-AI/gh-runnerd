// Package liveworker is a single-use, explicitly authorized Docker experiment.
// It does not dispatch jobs, obtain JIT, pull images or implement a provider.
package liveworker

import (
	"context"
	"errors"
	"slices"
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
	RunnerUpdatesDisabled bool      `json:"runner_updates_disabled"`
	HarnessSHA            string    `json:"harness_sha"`
	WorkflowSHA           string    `json:"workflow_sha"`
	OwnerNonce            string    `json:"owner_nonce"`
	Controller            string    `json:"controller"`
	Endpoint              string    `json:"endpoint"`
	DaemonID              string    `json:"daemon_id"`
	ImageID               string    `json:"image_id"`
	Image                 string    `json:"image"`
	ExpiresAt             time.Time `json:"expires_at"`
	Phases                []string  `json:"phases"`
}

type Event struct {
	Status       string `json:"status,omitempty"`
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
	Path       string         `json:"Path"`
	Args       []string       `json:"Args"`
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

type state struct {
	id, envDigest, labelsDigest               string
	created, startAttempt, uncertain, deleted bool
}

func replay(events []Event) state {
	s := state{}
	pending := ""
	for _, e := range events {
		switch e.Kind {
		case "intent":
			if pending != "" {
				s.uncertain = true
			}
			pending = e.Operation
			if e.Operation == "create" {
				s.created = true
				s.envDigest = e.EnvDigest
				s.labelsDigest = e.LabelsDigest
			}
			if e.Operation == "start" {
				s.startAttempt = true
			}
		case "result":
			if pending != e.Operation {
				s.uncertain = true
			}
			pending = ""
			if e.Operation == "create" {
				s.id = e.ID
			}
			if e.Operation == "delete" {
				s.deleted = true
			}
		case "unknown":
			if e.Operation == "create" && id.MatchString(e.ID) {
				s.id = e.ID
			}
			s.uncertain = true
			pending = ""
		}
	}
	s.uncertain = s.uncertain || pending != ""
	return s
}
func (d *Driver) record(e Event) error {
	if d.Journal.Append(e) != nil {
		return ErrState
	}
	return nil
}
func (d *Driver) effect(ctx context.Context, e Event, call func(context.Context) (Event, error)) error {
	e.Kind = "intent"
	if err := d.record(e); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := call(bounded)
	if err != nil {
		knownID := ""
		if id.MatchString(result.ID) {
			knownID = result.ID
		}
		if d.record(Event{Kind: "unknown", Operation: e.Operation, ID: knownID}) != nil {
			return ErrState
		}
		return ErrUncertain
	}
	result.Kind = "result"
	result.Operation = e.Operation
	return d.record(result)
}
func bounded[T any](ctx context.Context, call func(context.Context) (T, error)) (T, error) {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	value, err := call(c)
	if err != nil {
		var zero T
		return zero, ErrRemote
	}
	return value, nil
}

// Run performs one explicitly requested phase. Unknown effects retain the slot;
// local deletion never releases a GitHub reservation or permits another create.
func (d *Driver) Run(ctx context.Context, phase, jit string) error {
	if d.Approval.Validate(time.Now()) != nil || !slices.Contains(d.Approval.Phases, phase) {
		return ErrApproval
	}
	s := replay(d.Journal.Events())
	if phase != "inspect" && (s.uncertain || s.deleted) {
		return ErrUncertain
	}
	if phase == "create" && s.created {
		return ErrUncertain
	}
	if phase != "create" && jit != "" {
		return ErrApproval
	}
	deadline := minTime(time.Now().Add(10*time.Minute), d.Approval.ExpiresAt)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	image, err := bounded(ctx, func(c context.Context) (ImageProfile, error) { return d.Runtime.Preflight(c, d.Approval) })
	if err != nil {
		return ErrApproval
	}
	if phase == "create" {
		payload, e, err := d.Approval.creation(image, jit)
		if err != nil {
			return err
		}
		e.Operation = "create"
		return d.effect(ctx, e, func(c context.Context) (Event, error) {
			containerID, warnings, err := d.Runtime.Create(c, d.Approval.name(), payload)
			if err != nil || !id.MatchString(containerID) {
				return Event{}, ErrUncertain
			}
			if warnings {
				return Event{ID: containerID}, ErrUncertain
			}
			return Event{ID: containerID}, nil
		})
	}
	if !id.MatchString(s.id) {
		return ErrUncertain
	}
	container, err := bounded(ctx, func(c context.Context) (*Container, error) { return d.Runtime.Inspect(c, s.id) })
	if err != nil {
		return ErrUncertain
	}
	if d.Approval.verify(container, s) != nil {
		return ErrUncertain
	}
	if phase == "inspect" {
		status := container.State.Status
		if !slices.Contains([]string{"created", "running", "paused", "restarting", "removing", "exited", "dead"}, status) {
			status = "unknown"
		}
		return d.record(Event{Kind: "observation", Operation: "inspect", ID: s.id, Status: status})
	}
	if container.State.Running || container.State.Paused || container.State.Restarting || container.State.Dead {
		return ErrUncertain
	}
	if phase == "start" {
		if s.startAttempt || container.State.Status != "created" {
			return ErrUncertain
		}
		return d.effect(ctx, Event{Operation: "start", ID: s.id}, func(c context.Context) (Event, error) { return Event{ID: s.id}, d.Runtime.Start(c, s.id) })
	}
	if container.State.Status != "created" && container.State.Status != "exited" {
		return ErrUncertain
	}
	return d.effect(ctx, Event{Operation: "delete", ID: s.id}, func(c context.Context) (Event, error) { return Event{ID: s.id}, d.Runtime.Delete(c, s.id) })
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
