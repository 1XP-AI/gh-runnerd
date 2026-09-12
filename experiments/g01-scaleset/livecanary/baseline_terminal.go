package livecanary

import (
	"context"
	"strings"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

type baselineTerminalOutcome string

const (
	terminalUnresolved baselineTerminalOutcome = "unresolved"
	terminalComplete   baselineTerminalOutcome = "complete"
)

type pairedBaselineTerminalResult struct {
	Collection pairedBaselineCollection `json:"collection"`
	Terminal   baselineTerminalOutcome  `json:"terminal"`
}

// Both private entries fix their behavior; neither is exposed by a live phase.
func runPairedTerminal(ctx context.Context, d *Driver, w *liveworker.Driver) (pairedBaselineTerminalResult, error) {
	return runPairedTerminalWithCadence(ctx, d, w, realBaselineCadence())
}
func runPairedTerminalWithCadence(ctx context.Context, d *Driver, w *liveworker.Driver, cadence pairedBaselineCadence) (pairedBaselineTerminalResult, error) {
	return runPairedTerminalWithBinding(ctx, d, w, cadence, nil)
}
func runPairedTerminalWithBinding(ctx context.Context, d *Driver, w *liveworker.Driver, cadence pairedBaselineCadence, bindingCheck func() error) (pairedBaselineTerminalResult, error) {
	out, err := runPairedBaselineModeWithBinding(ctx, d, w, cadence, true, bindingCheck)
	result := pairedBaselineTerminalResult{Collection: out, Terminal: terminalUnresolved}
	if out.Terminal != nil && err == nil {
		result.Terminal = out.Terminal.Outcome
	}
	return result, err
}

// Reserve only records still needed by the actual replayed terminal stage.
// Every caller retains the current pair/authority check before this capacity check.
func (s *pairedBaselineScope) terminalCapacity() error {
	state, err := s.state()
	if err != nil {
		return err
	}
	records := 4 // Next child intent/result, parent closure and collection summary.
	if state.pending != nil && state.pending.Stage == "terminal" {
		if state.child != nil {
			records = 3 // The actual child intent is already durable.
		} else if state.terminalStep == len(terminalSteps) {
			records = 2 // All children finished; only parent and summary remain.
		}
	}
	s.journal.mu.Lock()
	defer s.journal.mu.Unlock()
	f, err := s.journal.file.Stat()
	if err != nil || s.journal.writeFailed || f.Size()+int64(records*baselineRecordLimit) > baselineJournalLimit {
		return ErrJournal
	}
	return nil
}
func (s *pairedBaselineScope) finalizeTerminal() (err error) {
	if !s.terminalEnabled || s.listener == nil || !s.listener.finalizing {
		return ErrQuarantine
	}
	if err = s.sampleRounds(5, 8); err != nil {
		return err
	}
	state, err := s.state()
	if err != nil {
		return err
	}
	if err = s.boundary(); err != nil {
		return err
	}
	parent := baselineRecord{Stage: "terminal", Outcome: "intent", SessionID: state.sessionID, Terminal: &baselineTerminalPayload{Previous: state.lastSample, Evidence: state.evidence(func(r controllerRecordRef) *Event { return s.event(workerRef(r)) })}}
	parent.Intent, err = s.store(parent)
	if err != nil {
		return err
	}
	defer func() {
		// Unknown child closure is allowed only for this actual pending child. A
		// poisoned journal remains pending; no fabricated persistence is reported.
		current, replayErr := s.state()
		if replayErr == nil && current.child != nil && current.pending != nil && current.pending.Stage == "terminal" {
			child := *current.child
			child.Intent = current.childRef
			_, _ = s.finish(child, false)
		}
		current, replayErr = s.state()
		if replayErr == nil && current.pending != nil && current.pending.Stage == "terminal" && current.child == nil {
			_, storeErr := s.finish(parent, err == nil)
			if storeErr != nil {
				err = ErrJournal
			}
		}
	}()
	for _, stage := range terminalSteps {
		if err = s.terminalStepCall(stage); err != nil {
			return err
		}
	}
	return nil
}
func (s *pairedBaselineScope) terminalStepCall(stage string) error {
	if err := s.boundary(); err != nil {
		return err
	}
	state, err := s.state()
	if err != nil {
		return err
	}
	r := baselineRecord{Stage: stage, SessionID: state.sessionID, Terminal: &baselineTerminalPayload{Parent: state.terminalIntent, Previous: state.terminalLast}}
	if stage == "terminal-decision" {
		// Replay resolves the full exact round and both fresh prerequisite reads.
		r.Outcome = "observed"
		if _, err = s.store(r); err != nil {
			return err
		}
		return s.boundary()
	}
	r, err = s.begin(r)
	if err != nil {
		return err
	}
	known := false
	var callErr error
	switch {
	case strings.Contains(stage, "roster"):
		observation, e := s.captured.observeRoster(s.ctx)
		callErr = e
		r.Terminal.Roster = &observation
		known = e == nil && state.rosterMatches(&observation, func(ref controllerRecordRef) *Event { return s.event(workerRef(ref)) })
	case stage == "terminal-set" || stage == "terminal-set-recheck" || stage == "terminal-set-absence":
		capture := &baselineWireCapture{stage: stage, setID: s.setID, allowedHosts: baselineWireAllowedHosts(s.approval, s.captured.drainEndpointHost())}
		ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
		set, e := s.captured.GetScaleSet(capture.context(ctx), s.setID)
		callErr = e
		cancel()
		_, _, _, r.HTTPStatus = capture.facts()
		r.Terminal.Set = capture.set
		if stage == "terminal-set-absence" {
			known = capture.observed() && r.HTTPStatus == 404
		} else {
			known = e == nil && capture.observed() && r.HTTPStatus == 200 && terminalZeroSet(capture.set, s.approval, s.setID) && set != nil && set.ID == capture.set.ID && set.Name == capture.set.Name && set.RunnerGroupID == capture.set.GroupID && set.RunnerSetting.DisableUpdate && capture.set.Statistics.matches(set.Statistics)
		}
		// A strict reported404 is expected to be returned by the SDK as an error.
		if stage == "terminal-set-absence" && known {
			callErr = nil
		}
	case stage == "terminal-session-close":
		if s.listener.session == nil || s.listener.session.Session().SessionID.String() != state.sessionID {
			return ErrQuarantine
		}
		origin := s.listener.capturedOrigin()
		if origin == "" {
			return ErrQuarantine
		}
		capture := &baselineWireCapture{stage: stage, setID: s.setID, sessionID: state.sessionID, origin: origin}
		ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
		callErr = s.listener.session.Close(capture.context(ctx))
		cancel()
		_, _, _, r.HTTPStatus = capture.facts()
		known = capture.observed() && r.HTTPStatus == 204
	case stage == "terminal-worker-delete":
		c := state.handoff.Container
		receipt, e := s.worker.DeleteTerminal(liveworker.TerminalDecisionRef{PairSHA256: c.PairSHA256, ContainerID: c.ContainerID, CreateResult: c.CreateResult, ControllerDecision: workerRef(state.terminalDecision)})
		callErr = e
		if receipt != (liveworker.DeletionReceipt{}) {
			r.Terminal.Deletion = &receipt
			s.observedWorkerDeletion = &receipt
		}
		known = e == nil && r.Terminal.Deletion != nil && receipt.AbsenceResult != nil
	case stage == "terminal-set-delete":
		capture := &baselineWireCapture{stage: stage, setID: s.setID, allowedHosts: baselineWireAllowedHosts(s.approval, s.captured.drainEndpointHost())}
		ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
		callErr = s.captured.DeleteScaleSet(capture.context(ctx), s.setID)
		cancel()
		_, _, _, r.HTTPStatus = capture.facts()
		known = capture.observed() && r.HTTPStatus == 204
	default:
		return ErrQuarantine
	}
	_, err = s.finish(r, known)
	if err != nil {
		return err
	}
	if callErr != nil {
		return ErrQuarantine
	}
	return s.boundary()
}
