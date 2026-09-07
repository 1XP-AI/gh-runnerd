package livecanary

import (
	"reflect"
	"strings"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

const sessionCloseAcknowledged sessionOutstanding = "close-acknowledged204"

// All terminal decisions resolve these actual historical controller records.
// The round reference resolves all four measured slots, including their targets.
type terminalEvidence struct {
	Pair, Acquire, JIT, Handoff, Start, Source, Started, Completed, Round8 controllerRecordRef
}
type baselineTerminalPayload struct {
	Parent   controllerRecordRef         `json:"parent"`
	Previous controllerRecordRef         `json:"previous"`
	Evidence *terminalEvidence           `json:"evidence,omitempty"`
	Set      *baselineSetFacts           `json:"set,omitempty"`
	Roster   *rosterObservation          `json:"roster,omitempty"`
	Deletion *liveworker.DeletionReceipt `json:"deletion,omitempty"`
}
type terminalCollectionFacts struct {
	Measurement                                                                                                                              collectionOutcome       `json:"measurement"`
	Evidence                                                                                                                                 *terminalEvidence       `json:"evidence,omitempty"`
	Outcome                                                                                                                                  baselineTerminalOutcome `json:"outcome"`
	Intent, Result, Decision, SessionCloseIntent, SessionCloseResult, WorkerDeleteResult, SetDeleteIntent, SetDeleteResult, SetAbsenceResult controllerRecordRef
	WorkerDeletion                                                                                                                           *liveworker.DeletionReceipt `json:"worker_deletion,omitempty"`
}

var terminalSteps = [...]string{"terminal-roster", "terminal-set", "terminal-decision", "terminal-session-close", "terminal-worker-delete", "terminal-set-recheck", "terminal-roster-recheck", "terminal-set-delete", "terminal-set-absence", "terminal-roster-final"}

func terminalStage(stage string) bool {
	if stage == "terminal" {
		return true
	}
	for _, x := range terminalSteps {
		if stage == x {
			return true
		}
	}
	return false
}
func terminalShape(r baselineRecord) bool {
	if r.Terminal == nil {
		return false
	}
	shape := baselineRecord{Version: r.Version, Stage: r.Stage, Outcome: r.Outcome, SetID: r.SetID, Creation: r.Creation, SessionID: r.SessionID}
	f := r.Terminal
	payload := baselineTerminalPayload{Parent: f.Parent, Previous: f.Previous}
	response := r.Outcome == "result" || r.Outcome == "unknown"
	if response {
		shape.Intent = r.Intent
	}
	switch r.Stage {
	case "terminal":
		payload.Evidence = f.Evidence
		if f.Evidence == nil {
			return false
		}
	case "terminal-set", "terminal-set-recheck", "terminal-set-absence":
		if response {
			payload.Set = f.Set
			shape.HTTPStatus = r.HTTPStatus
		}
	case "terminal-roster", "terminal-roster-recheck", "terminal-roster-final":
		if response {
			payload.Roster = f.Roster
		}
	case "terminal-session-close", "terminal-set-delete":
		if response {
			shape.HTTPStatus = r.HTTPStatus
		}
	case "terminal-worker-delete":
		if response {
			payload.Deletion = f.Deletion
		}
	case "terminal-decision":
		if r.Outcome != "observed" {
			return false
		}
	default:
		return false
	}
	if r.Stage != "terminal-decision" && r.Outcome == "observed" {
		return false
	}
	shape.Terminal = &payload
	return reflect.DeepEqual(r, shape)
}
func (s *baselineHistory) evidence(lookup func(controllerRecordRef) *Event) *terminalEvidence {
	e := lookup(s.acquireRef)
	if e == nil || e.Baseline == nil {
		return nil
	}
	return &terminalEvidence{s.pairRef, s.acquireRef, s.jitRef, s.handoffRef, s.startRef, e.Baseline.SourceRef, s.startedRef, s.completedRef, s.lastSample}
}
func (s *baselineHistory) terminalSummary() *terminalCollectionFacts {
	m := collectionUnresolved
	if s.terminalEvidence != nil || s.collected() {
		m = collectionCollected
	}
	o := terminalUnresolved
	if refPresent(s.terminalResult) && !s.uncertain {
		o = terminalComplete
	}
	return &terminalCollectionFacts{Measurement: m, Evidence: s.terminalEvidence, Outcome: o, Intent: s.terminalIntent, Result: s.terminalResult, Decision: s.terminalDecision, SessionCloseIntent: s.sessionCloseIntent, SessionCloseResult: s.sessionCloseResult, WorkerDeleteResult: s.workerDeleteRef, WorkerDeletion: s.workerDeletion, SetDeleteIntent: s.setDeleteIntent, SetDeleteResult: s.setDeleteResult, SetAbsenceResult: s.setAbsenceResult}
}
func terminalZeroSet(f *baselineSetFacts, a Approval, id int) bool {
	if !f.eligible(a, id) {
		return false
	}
	st := f.Statistics
	for _, v := range []*int{st.Available, st.Acquired, st.Assigned, st.Running, st.Registered, st.Busy, st.Idle} {
		if v == nil || *v != 0 {
			return false
		}
	}
	return true
}
func terminalLocal(l *liveworker.LocalReceipt) bool {
	if l == nil || l.HTTPStatus != 200 || l.Outcome != liveworker.LocalProfilePresent || l.State == nil || l.State.Status != "exited" || l.State.ExitCode == nil || *l.State.ExitCode != 0 {
		return false
	}
	for _, b := range []*bool{l.State.Running, l.State.Paused, l.State.Restarting, l.State.Dead} {
		if b == nil || *b {
			return false
		}
	}
	return true
}
func (s *baselineHistory) eligibleTerminalRound(lookup func(controllerRecordRef) *Event, a Approval) bool {
	if s.terminalEvidence == nil || s.anchor == nil || s.jit == nil || s.jit.Runner == nil {
		return false
	}
	e := lookup(s.terminalEvidence.Round8)
	if e == nil || e.Baseline == nil || e.Baseline.Outcome != "result" || e.Baseline.Sample == nil {
		return false
	}
	f := e.Baseline.Sample
	if f.Round != 8 || !s.validSampleRepresentation(f, a) || f.SDK == nil || f.SDK.Response.Status != 404 || f.SDK.Response.Outcome != observationNotFound || !f.RESTAddressable || f.REST == nil || f.REST.Response.Status != 404 || f.REST.Response.Outcome != observationNotFound || f.Job == nil || f.Job.Response.Outcome != observationPresent || f.Job.Status != "completed" || f.Job.Conclusion == nil || *f.Job.Conclusion != "success" || !terminalLocal(f.Local) {
		return false
	}
	// Both actual callback records resolve their full source-anchored batches.
	for _, ref := range []controllerRecordRef{s.terminalEvidence.Started, s.terminalEvidence.Completed} {
		e := lookup(ref)
		if e == nil || e.Baseline == nil || e.Baseline.ItemIndex == nil {
			return false
		}
		b := lookup(e.Baseline.BatchRef)
		if b == nil || b.Baseline == nil || b.Baseline.Batch == nil {
			return false
		}
		n := *e.Baseline.ItemIndex
		if n < 0 || n >= len(b.Baseline.Batch.Items) {
			return false
		}
		x := b.Baseline.Batch.Items[n]
		if x.RequestID != s.anchor.RequestID || x.JobID != s.anchor.JobID || x.RunnerID == nil || *x.RunnerID != sdkRunnerID(s.jit.Runner.ID) || x.RunnerName == nil || *x.RunnerName != s.jit.Runner.Name {
			return false
		}
		if e.Baseline.Stage == "completed" && (x.Result == nil || *x.Result != "succeeded") {
			return false
		}
	}
	return true
}
func validTerminalSetRepresentation(f *baselineSetFacts) bool {
	if f == nil {
		return true
	}
	if !baselineText(f.Name, 128) || len(f.Labels) > 8 {
		return false
	}
	for _, l := range f.Labels {
		if !baselineText(l.Name, 128) || !baselineText(l.Type, 32) {
			return false
		}
	}
	return true
}
func (s *baselineHistory) validTerminalResponse(r baselineRecord, a Approval) bool {
	f := r.Terminal
	if r.HTTPStatus != 0 && (r.HTTPStatus < 100 || r.HTTPStatus > 599) {
		return false
	}
	if f.Set != nil && (r.HTTPStatus != 200 || !validTerminalSetRepresentation(f.Set)) {
		return false
	}
	if f.Roster != nil && !validRosterObservation(*f.Roster, a.Organization) {
		return false
	}
	if d := f.Deletion; d != nil {
		c := s.handoff.Container
		if d.PairSHA256 != c.PairSHA256 || d.ContainerID != c.ContainerID || !refPresent(controllerRef(d.Intent)) || !refPresent(controllerRef(d.Result)) || d.Intent.Sequence <= c.CreateResult.Sequence || d.Result.Sequence <= d.Intent.Sequence {
			return false
		}
		if d.AbsenceResult != nil && (!refPresent(controllerRef(*d.AbsenceResult)) || d.AbsenceResult.Sequence <= d.Result.Sequence) {
			return false
		}
	}
	return true
}
func (s *baselineHistory) terminalRecord(r baselineRecord, ref controllerRecordRef, lookup func(controllerRecordRef) *Event, a Approval) error {
	if s.collectionRef.Sequence != 0 || r.SessionID == "" || r.SessionID != s.sessionID {
		return ErrJournal
	}
	f := r.Terminal
	if r.Stage == "terminal" {
		if r.Outcome == "intent" {
			if s.terminalIntent.Sequence != 0 || s.pending != nil || s.child != nil || !s.collected() || !s.messageDone || !refPresent(s.startedRef) || f.Parent != (controllerRecordRef{}) || f.Previous != s.lastSample || !reflect.DeepEqual(f.Evidence, s.evidence(lookup)) {
				return ErrJournal
			}
			s.terminalEvidence = f.Evidence
			s.terminalIntent = ref
			s.terminalLast = ref
			s.pending = &r
			s.pendingRef = ref
			return nil
		}
		if s.pending == nil || s.pending.Stage != "terminal" || s.child != nil || r.Intent != s.terminalIntent {
			return ErrJournal
		}
		p := r
		p.Outcome = "intent"
		p.Intent = controllerRecordRef{}
		if !reflect.DeepEqual(p, *s.pending) || r.Outcome == "result" && (s.uncertain || s.terminalStep != len(terminalSteps)) {
			return ErrJournal
		}
		s.pending = nil
		if r.Outcome == "unknown" {
			s.uncertain = true
		} else {
			s.terminalResult = ref
		}
		return nil
	}
	if s.uncertain || s.pending == nil || s.pending.Stage != "terminal" || s.terminalStep >= len(terminalSteps) || r.Stage != terminalSteps[s.terminalStep] || f.Parent != s.terminalIntent || f.Previous != s.terminalLast {
		return ErrJournal
	}
	if r.Stage == "terminal-decision" {
		prior := lookup(f.Previous)
		roster := lookup(s.pendingTerminalRoster(lookup))
		if r.Outcome != "observed" || s.child != nil || !s.eligibleTerminalRound(lookup, a) || prior == nil || prior.Baseline == nil || !terminalZeroSet(prior.Baseline.Terminal.Set, a, s.setID) || roster == nil || roster.Baseline == nil || !s.rosterMatches(roster.Baseline.Terminal.Roster, lookup) {
			return ErrJournal
		}
		s.terminalDecision = ref
		s.terminalLast = ref
		s.terminalStep++
		return nil
	}
	if r.Outcome == "intent" {
		if s.child != nil || r.Intent != (controllerRecordRef{}) {
			return ErrJournal
		}
		s.child = &r
		s.childRef = ref
		if r.Stage == "terminal-session-close" {
			s.sessionCloseIntent = ref
		}
		if r.Stage == "terminal-set-delete" {
			s.setDeleteIntent = ref
		}
		return nil
	}
	if s.child == nil || r.Intent != s.childRef {
		return ErrJournal
	}
	p := r
	p.Outcome = "intent"
	p.Intent = controllerRecordRef{}
	p.HTTPStatus = 0
	v := *r.Terminal
	v.Set = nil
	v.Roster = nil
	v.Deletion = nil
	p.Terminal = &v
	if !reflect.DeepEqual(p, *s.child) || !s.validTerminalResponse(r, a) {
		return ErrJournal
	}
	s.child = nil
	s.childRef = controllerRecordRef{}
	// Preserve known lower-layer effects even when a separate post-read failed.
	if r.Stage == "terminal-worker-delete" && f.Deletion != nil {
		s.workerDeletion = f.Deletion
		s.workerDeleteRef = ref
	}
	if r.Outcome == "unknown" {
		s.uncertain = true
		return nil
	}
	switch r.Stage {
	case "terminal-roster", "terminal-roster-recheck", "terminal-roster-final":
		if !s.rosterMatches(f.Roster, lookup) {
			return ErrJournal
		}
	case "terminal-set", "terminal-set-recheck":
		if r.HTTPStatus != 200 || !terminalZeroSet(f.Set, a, s.setID) {
			return ErrJournal
		}
	case "terminal-session-close":
		if r.HTTPStatus != 204 {
			return ErrJournal
		}
		s.sessionCloseResult = ref
	case "terminal-worker-delete":
		if f.Deletion == nil || f.Deletion.AbsenceResult == nil {
			return ErrJournal
		}
	case "terminal-set-delete":
		if r.HTTPStatus != 204 {
			return ErrJournal
		}
		s.setDeleteResult = ref
	case "terminal-set-absence":
		if r.HTTPStatus != 404 || f.Set != nil {
			return ErrJournal
		}
		s.setAbsenceResult = ref
	default:
		return ErrJournal
	}
	s.terminalLast = ref
	s.terminalStep++
	return nil
}
func (s *baselineHistory) pendingTerminalRoster(lookup func(controllerRecordRef) *Event) controllerRecordRef {
	e := lookup(s.terminalLast)
	if e == nil || e.Baseline == nil || e.Baseline.Terminal == nil {
		return controllerRecordRef{}
	}
	return e.Baseline.Terminal.Previous
}
func (s *baselineHistory) rosterMatches(o *rosterObservation, lookup func(controllerRecordRef) *Event) bool {
	anchor := lookup(s.rosterRef)
	return o != nil && o.Completeness == rosterComplete && anchor != nil && anchor.Baseline != nil && anchor.Baseline.Roster != nil && reflect.DeepEqual(o, anchor.Baseline.Roster.Observation)
}
func isTerminalRead(stage string) bool {
	return strings.Contains(stage, "roster") || stage == "terminal-set" || stage == "terminal-set-recheck" || stage == "terminal-set-absence"
}
