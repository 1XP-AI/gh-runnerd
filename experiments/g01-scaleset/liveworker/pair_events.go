package liveworker

import (
	"encoding/json"
	"net/http"
	"slices"
)

// Each union branch stores predecessors and facts, never its own event hash.
type pairedEvent struct {
	Bound  *pairBoundEvent  `json:"bound,omitempty"`
	Create *pairCreateEvent `json:"create,omitempty"`
	Start  *pairStartEvent  `json:"start,omitempty"`
	Local  *pairLocalEvent  `json:"local,omitempty"`
	Delete *pairDeleteEvent `json:"delete,omitempty"`
}

type pairBoundEvent struct {
	Binding          PairBinding `json:"binding"`
	ControllerIntent RecordRef   `json:"controller_intent"`
}
type pairCreateEvent struct {
	Kind         string          `json:"kind"`
	Handoff      *HandoffReceipt `json:"handoff,omitempty"`
	Intent       RecordRef       `json:"intent"`
	ContainerID  string          `json:"container_id,omitempty"`
	EnvDigest    string          `json:"env_digest,omitempty"`
	LabelsDigest string          `json:"labels_digest,omitempty"`
}
type pairStartEvent struct {
	Kind             string    `json:"kind"`
	CreateResult     RecordRef `json:"create_result"`
	ControllerResult RecordRef `json:"controller_result"`
	Intent           RecordRef `json:"intent"`
}
type pairLocalEvent struct {
	Kind        string            `json:"kind"`
	ContainerID string            `json:"container_id"`
	Intent      RecordRef         `json:"intent"`
	Method      string            `json:"method,omitempty"`
	Path        string            `json:"path,omitempty"`
	HTTPStatus  int               `json:"http_status,omitempty"`
	Outcome     LocalOutcome      `json:"outcome,omitempty"`
	State       *DockerStateFacts `json:"state,omitempty"`
}
type pairDeleteEvent struct {
	Kind        string               `json:"kind"`
	Decision    *TerminalDecisionRef `json:"decision,omitempty"`
	Observation RecordRef            `json:"observation"`
	Intent      RecordRef            `json:"intent"`
}

type pairedState struct {
	binding     *PairBinding
	pair        PairReceipt
	legacy      bool
	uncertain   bool
	pending     string
	pendingRef  RecordRef
	createInput *pairCreateEvent
	created     *ContainerReceipt
	knownID     string
	startInput  *pairStartEvent
	started     RecordRef
	lastLocal   *LocalReceipt
	deleteInput *pairDeleteEvent
	deleted     *DeletionReceipt
}

func cloneEvent(e Event) Event {
	if e.Authority != nil {
		a := *e.Authority
		a.Phases = slices.Clone(a.Phases)
		e.Authority = &a
	}
	if e.Paired == nil {
		return e
	}
	p := *e.Paired
	e.Paired = &p
	if p.Bound != nil {
		v := *p.Bound
		p.Bound = &v
	}
	if p.Create != nil {
		v := *p.Create
		p.Create = &v
		if v.Handoff != nil {
			h := *v.Handoff
			v.Handoff = &h
		}
	}
	if p.Start != nil {
		v := *p.Start
		p.Start = &v
	}
	if p.Local != nil {
		v := *p.Local
		v.State = cloneFacts(v.State)
		p.Local = &v
	}
	if p.Delete != nil {
		v := *p.Delete
		p.Delete = &v
		if v.Decision != nil {
			decision := *v.Decision
			v.Decision = &decision
		}
	}
	return e
}

func validPairedShape(e Event) bool {
	if e.Sequence <= 0 || e.Kind != "paired" || e.Paired == nil || e.Authority != nil || e.Status != "" || e.Operation != "" || e.ID != "" || e.Digest != "" || e.EnvDigest != "" || e.LabelsDigest != "" {
		return false
	}
	p, branches := e.Paired, 0
	for _, present := range []bool{p.Bound != nil, p.Create != nil, p.Start != nil, p.Local != nil, p.Delete != nil} {
		if present {
			branches++
		}
	}
	data, err := json.Marshal(e)
	return branches == 1 && err == nil && len(data)+1 <= maxPairRecord
}

func validLocalFacts(e pairLocalEvent) bool {
	if e.Method != http.MethodGet || e.Path != "/v"+apiVersion+"/containers/"+e.ContainerID+"/json" {
		return false
	}
	switch e.Outcome {
	case LocalProfilePresent:
		if e.HTTPStatus != http.StatusOK {
			return false
		}
		return e.State == nil || slices.Contains([]ContainerStatus{ContainerStatusUnknown, ContainerStatusCreated, ContainerStatusRunning, ContainerStatusPaused, ContainerStatusRestarting, ContainerStatusRemoving, ContainerStatusExited, ContainerStatusDead}, e.State.Status)
	case LocalNotFoundReported:
		return e.HTTPStatus == http.StatusNotFound && e.State == nil
	case LocalUnknown:
		return (e.HTTPStatus == 0 || e.HTTPStatus >= 100 && e.HTTPStatus <= 599) && e.State == nil
	}
	return false
}

// step is shared by append and reopen. Apply to a value copy; no existing
// payload is mutated, so a rejected event cannot partially update replay.
func (s *pairedState) step(e Event) bool {
	if e.Paired == nil {
		if e.Kind == "paired" {
			return false
		}
		if e.Kind != "authority" {
			if s.binding != nil {
				return e.Kind == "observation" && e.Operation == "inspect"
			}
			s.legacy = true
		}
		return validEvent(e)
	}
	if !validPairedShape(e) {
		return false
	}
	p := e.Paired
	if p.Bound != nil {
		b := p.Bound
		if s.binding != nil || s.legacy || !validBinding(b.Binding) || !validRef(b.ControllerIntent) || b.ControllerIntent.Sequence <= b.Binding.Input.SetCreation.Sequence {
			return false
		}
		copy := b.Binding
		s.binding = &copy
		s.pair = PairReceipt{pairHash(copy), b.ControllerIntent, workerEventRef(copy.Worker, e)}
		return true
	}
	if s.binding == nil {
		return false
	}
	ref := workerEventRef(s.binding.Worker, e)
	if c := p.Create; c != nil {
		if c.Kind == "intent" {
			if s.pending != "" || s.uncertain || s.createInput != nil || c.Intent != (RecordRef{}) || c.ContainerID != "" || c.Handoff == nil || !validHandoff(*c.Handoff, *s.binding, s.pair) || !id.MatchString(c.EnvDigest) || !id.MatchString(c.LabelsDigest) {
				return false
			}
			copy := *c
			s.createInput, s.pending, s.pendingRef = &copy, "create", ref
			return true
		}
		if s.pending != "create" || c.Intent != s.pendingRef || c.Handoff != nil || c.EnvDigest != "" || c.LabelsDigest != "" || (c.Kind != "result" && c.Kind != "unknown") || (c.ContainerID != "" && !id.MatchString(c.ContainerID)) || (c.Kind == "result" && c.ContainerID == "") {
			return false
		}
		s.knownID = c.ContainerID
		if c.Kind == "result" {
			s.created = &ContainerReceipt{s.pair.PairSHA256, s.createInput.Handoff.HandoffIntent, s.pendingRef, ref, c.ContainerID, s.createInput.EnvDigest, s.createInput.LabelsDigest}
		} else {
			s.uncertain = true
		}
		s.pending, s.pendingRef = "", RecordRef{}
		return true
	}
	if start := p.Start; start != nil {
		if start.Kind == "intent" {
			if s.pending != "" || s.uncertain || s.created == nil || s.startInput != nil || s.deleted != nil || start.Intent != (RecordRef{}) || start.CreateResult != s.created.CreateResult || !validRef(start.ControllerResult) || start.ControllerResult.Sequence <= s.created.HandoffIntent.Sequence || s.lastLocal == nil || s.lastLocal.Result.Sequence != e.Sequence-1 || !startFacts(*s.lastLocal) {
				return false
			}
			copy := *start
			s.startInput, s.pending, s.pendingRef = &copy, "start", ref
			return true
		}
		if s.pending != "start" || start.Intent != s.pendingRef || start.CreateResult != (RecordRef{}) || start.ControllerResult != (RecordRef{}) || (start.Kind != "result" && start.Kind != "unknown") {
			return false
		}
		if start.Kind == "result" {
			s.started = ref
		} else {
			s.uncertain = true
		}
		s.pending, s.pendingRef = "", RecordRef{}
		return true
	}
	if local := p.Local; local != nil {
		if !id.MatchString(s.knownID) || local.ContainerID != s.knownID {
			return false
		}
		if local.Kind == "intent" {
			if local.Intent != (RecordRef{}) || local.Method != "" || local.Path != "" || local.HTTPStatus != 0 || local.Outcome != "" || local.State != nil {
				return false
			}
			// A fresh report can follow an interrupted request after reopen. Its
			// new intent never resolves or clears that earlier uncertainty.
			if s.pending != "" {
				s.uncertain = true
			}
			s.pending, s.pendingRef = "local", ref
			return true
		}
		if local.Kind != "result" || s.pending != "local" || local.Intent != s.pendingRef || !validLocalFacts(*local) {
			return false
		}
		s.lastLocal = &LocalReceipt{s.pair.PairSHA256, s.pendingRef, ref, local.ContainerID, local.Method, local.Path, local.HTTPStatus, local.Outcome, cloneFacts(local.State)}
		if local.Outcome == LocalUnknown {
			s.uncertain = true
		}
		if local.Outcome == LocalNotFoundReported && s.deleted != nil && s.deleted.AbsenceResult == nil {
			copy := cloneDeletion(*s.deleted)
			copy.AbsenceResult = &ref
			s.deleted = &copy
		}
		s.pending, s.pendingRef = "", RecordRef{}
		return true
	}
	if deletion := p.Delete; deletion != nil {
		if deletion.Kind == "intent" {
			if s.pending != "" || s.uncertain || s.created == nil || s.deleteInput != nil || deletion.Intent != (RecordRef{}) || deletion.Decision == nil || !s.matchesDecision(*deletion.Decision) || s.lastLocal == nil || deletion.Observation != s.lastLocal.Result || s.lastLocal.Result.Sequence != e.Sequence-1 || !terminalFacts(*s.lastLocal) {
				return false
			}
			copy := *deletion
			s.deleteInput, s.pending, s.pendingRef = &copy, "delete", ref
			return true
		}
		if s.pending != "delete" || deletion.Intent != s.pendingRef || deletion.Decision != nil || deletion.Observation != (RecordRef{}) || (deletion.Kind != "result" && deletion.Kind != "unknown") {
			return false
		}
		if deletion.Kind == "result" {
			s.deleted = &DeletionReceipt{s.pair.PairSHA256, s.knownID, s.pendingRef, ref, nil}
		} else {
			s.uncertain = true
		}
		s.pending, s.pendingRef = "", RecordRef{}
		return true
	}
	return false
}

func (s pairedState) matchesDecision(d TerminalDecisionRef) bool {
	return s.created != nil && d.PairSHA256 == s.pair.PairSHA256 && d.ContainerID == s.created.ContainerID && d.CreateResult == s.created.CreateResult && validRef(d.ControllerDecision) && d.ControllerDecision.Sequence > s.created.HandoffIntent.Sequence
}

func terminalFacts(r LocalReceipt) bool {
	s := r.State
	return r.Outcome == LocalProfilePresent && s != nil && s.Status == ContainerStatusExited && s.ExitCode != nil && *s.ExitCode == 0 && s.Running != nil && !*s.Running && s.Paused != nil && !*s.Paused && s.Restarting != nil && !*s.Restarting && s.Dead != nil && !*s.Dead
}

func startFacts(r LocalReceipt) bool {
	s := r.State
	return r.Outcome == LocalProfilePresent && s != nil && s.Status == ContainerStatusCreated && s.Running != nil && !*s.Running && s.Paused != nil && !*s.Paused && s.Restarting != nil && !*s.Restarting && s.Dead != nil && !*s.Dead
}
