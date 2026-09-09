// Package livecanary is an isolated, explicitly authorized controller experiment.
// Driver.Run remains a controller-only fault harness. The private paired
// experiment connects reviewed worker execution; neither path dispatches workflows.
package livecanary

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
)

var (
	ErrApproval   = errors.New("approval rejected")
	ErrJournal    = errors.New("journal unavailable; stop and retain resources")
	ErrQuarantine = errors.New("state uncertain; quarantine and inspect only")
	ErrRemote     = errors.New("remote operation failed; inspect private state")
	ErrBarrier    = errors.New("planned barrier reached")
	ErrNoMessage  = errors.New("no controlled message observed; phase unresolved")
)

type Approval struct {
	AppID          int64     `json:"app_id"`
	InstallationID int64     `json:"installation_id"`
	Organization   string    `json:"organization"`
	Repository     string    `json:"repository"`
	RepositoryID   int64     `json:"repository_id"`
	RunnerGroupID  int       `json:"runner_group_id"`
	OwnerNonce     string    `json:"owner_nonce"`
	HarnessSHA     string    `json:"harness_sha"`
	WorkflowSHA    string    `json:"workflow_sha"`
	WorkflowPath   string    `json:"workflow_path"`
	WorkflowRunID  int64     `json:"workflow_run_id"`
	Controller     string    `json:"controller"`
	ExpiresAt      time.Time `json:"expires_at"`
	ActionsHosts   []string  `json:"actions_hosts"`
	Phases         []string  `json:"phases"`
}

type Event struct {
	Baseline      *baselineRecord   `json:"baseline,omitempty"`
	Drain         *drainObservation `json:"drain,omitempty"`
	DrainSnapshot *drainSnapshot    `json:"drain_snapshot,omitempty"`
	DrainMarker   string            `json:"drain_marker,omitempty"`
	Authority     *phaseAuthority   `json:"authority,omitempty"`
	Sequence      int               `json:"sequence"`
	Kind          string            `json:"kind"`
	Operation     string            `json:"operation,omitempty"`
	ID            int               `json:"id,omitempty"`
	SessionID     string            `json:"session_id,omitempty"`
	RequestIDs    []int64           `json:"request_ids,omitempty"`
	Count         int               `json:"count,omitempty"`
	Digest        string            `json:"digest,omitempty"`
	Succeeded     bool              `json:"succeeded,omitempty"`
	Work          string            `json:"work,omitempty"`
}

type Journal interface {
	authorize(Approval) (func(), error)
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

type Driver struct {
	Approval Approval
	Journal  Journal
	API      API
}

const operationTimeout = 30 * time.Second

type state struct {
	setID                        int
	sessionID                    string
	reserved, uncertain, deleted bool
	phaseSeen                    map[string]bool
	observedJobs                 map[int64]bool
	workObserved                 bool
	inventory                    string
}

func replay(events []Event) state {
	s := state{phaseSeen: make(map[string]bool), observedJobs: make(map[int64]bool)}
	pending := ""
	for _, e := range events {
		switch e.Kind {
		case "baseline":
			// This library slice never grants a legacy fault/cleanup phase.
			s.reserved, s.uncertain, s.workObserved = true, true, true
		case "phase":
			s.phaseSeen[e.Operation] = true
			if e.Operation == "drain" {
				// The phase record is durable intent to observe a live session.
				// A crash before its final observation must not reopen cleanup.
				s.uncertain = true
			}
		case "inventory":
			s.inventory = e.Digest
		case "observation":
			if e.Operation == "poll" || e.Operation == "drain" {
				for _, id := range e.RequestIDs {
					s.observedJobs[id] = true
				}
			}
			if len(e.RequestIDs) > 0 {
				s.reserved = true
				s.workObserved = true
			}
			if e.Operation == "drain" {
				s.workObserved = true
				if e.Drain == nil || e.Drain.Outcome != drainOutcomeObserved {
					s.uncertain = true
				}
			}
			if e.Operation == "drain-marker" {
				s.uncertain = true
			}
			if e.DrainSnapshot != nil && !validDrainIdlePrerequisite(*e.DrainSnapshot) {
				s.uncertain = true
			}
		case "intent":
			if pending != "" {
				s.uncertain = true
			}
			pending = e.Operation
			if e.Operation == "acquire" || e.Operation == "jit" {
				s.reserved = true
			}
		case "result":
			if pending != e.Operation {
				s.uncertain = true
			}
			pending = ""
			if e.Work != "" {
				s.workObserved = true
				s.uncertain = s.uncertain || e.Work == workUnresolved
			}
			if len(e.RequestIDs) > 0 {
				s.reserved = true
				s.workObserved = true
				for _, id := range e.RequestIDs {
					s.observedJobs[id] = true
				}
			}
			if e.DrainSnapshot != nil && !validDrainIdlePrerequisite(*e.DrainSnapshot) {
				s.uncertain = true
			}
			switch e.Operation {
			case "create":
				s.setID = e.ID
			case "session-open":
				s.sessionID = e.SessionID
			case "session-close":
				s.sessionID = ""
			case "delete":
				s.deleted = true
			}
		case "unknown":
			s.uncertain = true
			pending = ""
		}
	}
	s.uncertain = s.uncertain || pending != "" || s.sessionID != ""
	return s
}

func (d *Driver) record(e Event) error {
	if d.Journal.Append(e) != nil {
		return ErrJournal
	}
	return nil
}

// effect persists intent before an effect or work-bearing observation. A read
// result can reveal work that must survive restart, so it has the same ordering.
// A valid create/session identity and its work category share one result record.
func (d *Driver) effect(ctx context.Context, op string, ids []int64, call func(context.Context) (Event, error)) error {
	if err := d.record(Event{Kind: "intent", Operation: op, RequestIDs: ids}); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	e, err := call(bounded)
	if err != nil {
		if d.record(Event{Kind: "unknown", Operation: op}) != nil {
			return ErrJournal
		}
		return ErrQuarantine // Never expose SDK URL, response body or error text.
	}
	e.Kind = "result"
	e.Operation = op
	if err := d.record(e); err != nil {
		return err
	}
	if e.Work == workUnresolved {
		return ErrQuarantine
	}
	return nil
}

func boundedRead[T any](ctx context.Context, read func(context.Context) (T, error)) (T, error) {
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	result, err := read(bounded)
	if err != nil {
		var zero T
		return zero, ErrRemote
	}
	return result, nil
}

func (d *Driver) owned(ctx context.Context, id int) (*scaleset.RunnerScaleSet, error) {
	var s *scaleset.RunnerScaleSet
	err := d.effect(ctx, "observe-owned", nil, func(c context.Context) (Event, error) {
		var err error
		s, err = d.API.GetScaleSet(c, id)
		if err != nil {
			return Event{}, err
		}
		// Ownership validation is part of the durable observation result.
		// A later matching zero must not erase an earlier loss of this proof.
		if s == nil || s.ID != id || s.Name != d.Approval.setName() || s.RunnerGroupID != d.Approval.RunnerGroupID || !slices.ContainsFunc(s.Labels, func(l scaleset.Label) bool { return l.Name == d.Approval.setName() }) {
			return Event{Work: workUnresolved}, nil
		}
		return Event{Work: statisticsWork(s.Statistics)}, nil
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Run executes one reviewed controller phase. All errors are fixed categories.
// A second process uses the durable journal to retain unknown reservations;
// session credentials cannot be rehydrated using the supported SDK API.
func (d *Driver) Run(ctx context.Context, phase string) error {
	s, release, err := authorizePhase(d.Approval, d.Journal, phase)
	if err != nil {
		return err
	}
	defer release()
	deadline := time.Now().Add(10 * time.Minute)
	if d.Approval.ExpiresAt.Before(deadline) {
		deadline = d.Approval.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	_, err = boundedRead(ctx, func(c context.Context) (bool, error) { return true, d.API.Preflight(c, d.Approval) })
	if err != nil {
		return ErrApproval
	}
	if phase == "inspect" {
		return d.inspect(ctx, s)
	}
	if err := d.record(Event{Kind: "phase", Operation: phase}); err != nil {
		return err
	}
	if phase == "create" {
		if s.setID != 0 || len(d.Journal.Events()) > 1 {
			return ErrQuarantine
		}
		inventory, err := boundedRead(ctx, d.API.Inventory)
		if err != nil {
			return err
		}
		if err = d.record(Event{Kind: "inventory", Digest: inventory}); err != nil {
			return err
		}
		existing, err := d.discover(ctx)
		if err != nil {
			return err
		}
		if existing != nil {
			return ErrQuarantine
		}
		return d.effect(ctx, "create", nil, func(c context.Context) (Event, error) {
			set, err := d.API.CreateScaleSet(c, &scaleset.RunnerScaleSet{Name: d.Approval.setName(), RunnerGroupID: d.Approval.RunnerGroupID, Labels: []scaleset.Label{{Name: d.Approval.setName(), Type: "System"}}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true}})
			if err != nil || set == nil || set.ID <= 0 || set.Name != d.Approval.setName() || set.RunnerGroupID != d.Approval.RunnerGroupID || !set.RunnerSetting.DisableUpdate {
				return Event{}, ErrRemote
			}
			return Event{ID: set.ID, Work: statisticsWork(set.Statistics)}, nil
		})
	}
	if s.setID <= 0 || s.reserved {
		return ErrQuarantine
	}
	if phase == "drain" {
		if s.workObserved || len(s.observedJobs) != 0 {
			return ErrQuarantine
		}
		return d.drain(ctx, s.setID)
	}
	set, err := d.owned(ctx, s.setID)
	if err != nil {
		return err
	}
	if phase == "cleanup" {
		s = replay(d.Journal.Events()) // Includes the just-completed owned read.
		// Aggregate zero alone never authorizes deletion. This scope has never
		// issued JIT/acquired a job, has no unresolved session, and must match
		// its original runner inventory as well as its immutable create receipt.
		// No terminal-job reconciliation exists in this bounded harness.
		// A closed session or aggregate zero never clears observed requests.
		if s.uncertain || s.workObserved || len(s.observedJobs) != 0 || set.Statistics == nil || *set.Statistics != (scaleset.RunnerScaleSetStatistic{}) {
			return ErrQuarantine
		}
		inventory, err := boundedRead(ctx, d.API.Inventory)
		if err != nil {
			return err
		}
		if s.inventory == "" || inventory != s.inventory {
			return ErrQuarantine
		}
		if err := d.effect(ctx, "delete", nil, func(c context.Context) (Event, error) { return Event{}, d.API.DeleteScaleSet(c, s.setID) }); err != nil {
			return err
		}
		after, err := boundedRead(ctx, d.API.Inventory)
		if err != nil {
			return err
		}
		if after != s.inventory {
			return ErrQuarantine
		}
		return d.record(Event{Kind: "observation", Operation: "inventory", Digest: after})
	}
	// A pinned container does not pin the listener if GitHub can request an
	// update. Recheck server state before session/JIT effects; observation and
	// otherwise verified empty cleanup remain possible after setting drift.
	if !set.RunnerSetting.DisableUpdate {
		return ErrQuarantine
	}
	if phase == "jit-loss" {
		ref, err := d.runner(ctx, s.setID)
		if err != nil {
			return err
		}
		if ref != nil {
			return ErrQuarantine
		}
		return d.effect(ctx, "jit", nil, func(c context.Context) (Event, error) {
			result, err := d.API.GenerateJIT(c, s.setID, d.Approval.workerName())
			// Deliberately suppress the response, including JIT and runner ID.
			// Both remote failure and response suppression retain one reservation.
			if result != nil {
				result.EncodedJITConfig = ""
			}
			if d.record(Event{Kind: "response", Operation: "jit", Succeeded: err == nil}) != nil {
				return Event{}, ErrJournal
			}
			return Event{}, ErrBarrier
		})
	}
	return d.probe(ctx, phase, s.setID)
}

func (d *Driver) inspect(ctx context.Context, s state) error {
	if s.setID == 0 { // Discovery is evidence only; never adopt an ambiguous create.
		_, err := d.discover(ctx)
		return err
	}
	set, err := d.owned(ctx, s.setID)
	if err != nil {
		return err
	}
	ref, err := d.runner(ctx, s.setID)
	if err != nil {
		return err
	}
	if ref != nil && (ref.Name != d.Approval.workerName() || ref.RunnerScaleSetID != s.setID || ref.ID <= 0) {
		return ErrQuarantine
	}
	e := Event{Kind: "observation", Operation: "inspect"}
	if set.Statistics != nil {
		e.Count = set.Statistics.TotalAssignedJobs
	}
	if ref != nil {
		e.ID = ref.ID
	}
	return d.record(e) // Observation never releases the reservation or uncertainty.
}
