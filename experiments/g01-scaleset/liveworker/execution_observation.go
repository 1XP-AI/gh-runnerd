package liveworker

import (
	"context"
	"net/http"
	"time"
)

func (w *PairedWorker) Observe() (LocalReceipt, error) {
	s, err := w.enter("inspect")
	if err != nil {
		return LocalReceipt{}, err
	}
	defer s.op.Unlock()
	state := s.journal.pairState()
	if state.binding == nil || !id.MatchString(state.knownID) {
		return LocalReceipt{}, ErrUncertain
	}
	check := ControllerCheck{Stage: CheckScope, PairSHA256: s.pairSHA, ControllerRecord: s.binding.Input.SetCreation, WorkerRecord: state.pair.WorkerBound}
	if _, err := s.preflight(check, pairCallRoom); err != nil {
		return LocalReceipt{}, err
	}
	receipt, _, err := s.observe(check)
	return receipt, err
}

// observe runs within the method's operation lock. It records bounded facts
// before returning them; raw Config/Env is used only by the profile verifier.
func (s *pairedScope) observe(check ControllerCheck) (LocalReceipt, *Container, error) {
	state := s.journal.pairState()
	if !id.MatchString(state.knownID) || state.createInput == nil || !s.journal.hasRoom(pairCallRoom) {
		return LocalReceipt{}, nil, ErrState
	}
	if err := s.guard(check); err != nil {
		return LocalReceipt{}, nil, err
	}
	if !s.journal.hasRoom(pairCallRoom) {
		return LocalReceipt{}, nil, ErrState
	}
	intent, err := s.store(pairedEvent{Local: &pairLocalEvent{Kind: "intent", ContainerID: state.knownID}})
	if err != nil {
		return LocalReceipt{}, nil, err
	}
	if err := s.guard(check); err != nil {
		return LocalReceipt{}, nil, err
	}
	if !s.journal.hasRoom(maxPairRecord) {
		return LocalReceipt{}, nil, ErrState
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	observation, callErr := s.runtime.InspectExact(ctx, state.knownID)
	cancel()
	result := pairLocalEvent{Kind: "result", ContainerID: state.knownID, Intent: intent, Method: http.MethodGet, Path: "/v" + apiVersion + "/containers/" + state.knownID + "/json", Outcome: LocalUnknown}
	var container *Container
	if observation.TargetID == state.knownID && observation.Method == result.Method && observation.Path == result.Path {
		if observation.HTTPStatus >= 100 && observation.HTTPStatus <= 599 {
			result.HTTPStatus = observation.HTTPStatus
		}
		if callErr == nil {
			switch observation.Outcome {
			case DockerInspectPresent:
				profile := stateForProfile(state)
				if observation.HTTPStatus == http.StatusOK && s.approval.verify(observation.Container, profile) == nil && compatibleFacts(observation) {
					result.Outcome, result.State, container = LocalProfilePresent, cloneFacts(observation.State), observation.Container
				}
			case DockerInspectNotFoundReported:
				if observation.HTTPStatus == http.StatusNotFound && observation.State == nil && observation.Container == nil {
					result.Outcome = LocalNotFoundReported
				}
			}
		}
	}
	if _, err := s.store(pairedEvent{Local: &result}); err != nil {
		return LocalReceipt{}, nil, err
	}
	receipt := cloneLocal(*s.journal.pairState().lastLocal)
	if result.Outcome == LocalUnknown {
		return receipt, nil, ErrUncertain
	}
	return receipt, container, nil
}

func stateForProfile(s pairedState) state {
	return state{id: s.knownID, envDigest: s.createInput.EnvDigest, labelsDigest: s.createInput.LabelsDigest}
}

func compatibleFacts(o DockerInspectObservation) bool {
	if o.Container == nil {
		return false
	}
	if o.State == nil {
		return o.Container.State.Status == string(ContainerStatusUnknown)
	}
	facts, raw := o.State, o.Container.State
	if string(facts.Status) != raw.Status {
		return false
	}
	for _, value := range []struct {
		fact *bool
		raw  bool
	}{{facts.Running, raw.Running}, {facts.Paused, raw.Paused}, {facts.Restarting, raw.Restarting}, {facts.Dead, raw.Dead}} {
		if value.fact != nil && *value.fact != value.raw {
			return false
		}
	}
	return validLocalFacts(pairLocalEvent{ContainerID: o.TargetID, Method: o.Method, Path: o.Path, HTTPStatus: o.HTTPStatus, Outcome: LocalProfilePresent, State: facts})
}

func (w *PairedWorker) DeleteTerminal(decision TerminalDecisionRef) (DeletionReceipt, error) {
	s, err := w.enter("cleanup")
	if err != nil {
		return DeletionReceipt{}, err
	}
	defer s.op.Unlock()
	state := s.journal.pairState()
	if !state.matchesDecision(decision) {
		return DeletionReceipt{}, ErrUncertain
	}
	check := ControllerCheck{Stage: CheckDelete, PairSHA256: s.pairSHA, ControllerRecord: decision.ControllerDecision, WorkerRecord: decision.CreateResult}
	if err := s.guard(check); err != nil {
		return DeletionReceipt{}, err
	}
	if state.deleted != nil {
		if !s.original || state.deleteInput == nil || *state.deleteInput.Decision != decision {
			return DeletionReceipt{}, ErrUncertain
		}
		receipt := cloneDeletion(*state.deleted)
		if receipt.AbsenceResult == nil {
			return receipt, ErrUncertain
		}
		return receipt, nil
	}
	if state.pending != "" || state.uncertain || state.deleteInput != nil {
		return DeletionReceipt{}, ErrUncertain
	}
	if _, err := s.preflight(check, pairDeleteRoom); err != nil {
		return DeletionReceipt{}, err
	}
	local, _, err := s.observe(check)
	if err != nil {
		return DeletionReceipt{}, err
	}
	if !terminalFacts(local) {
		return DeletionReceipt{}, ErrUncertain
	}
	if err := s.guard(check); err != nil {
		return DeletionReceipt{}, err
	}
	if !s.journal.hasRoom(2 * pairCallRoom) {
		return DeletionReceipt{}, ErrState
	}
	intent, err := s.store(pairedEvent{Delete: &pairDeleteEvent{Kind: "intent", Decision: &decision, Observation: local.Result}})
	if err != nil {
		return DeletionReceipt{}, err
	}
	if err := s.guard(check); err != nil {
		return DeletionReceipt{}, err
	}
	if !s.journal.hasRoom(maxPairRecord + pairCallRoom) {
		return DeletionReceipt{}, ErrState
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	callErr := s.runtime.Delete(ctx, decision.ContainerID)
	cancel()
	result := pairDeleteEvent{Kind: "result", Intent: intent}
	if callErr != nil {
		result.Kind = "unknown"
	}
	if _, err := s.store(pairedEvent{Delete: &result}); err != nil {
		return DeletionReceipt{}, err
	}
	if callErr != nil {
		return DeletionReceipt{}, ErrUncertain
	}
	receipt := cloneDeletion(*s.journal.pairState().deleted)
	// A completed DELETE remains known when cancellation prevents its post-read.
	post, _, err := s.observe(check)
	if err != nil {
		return receipt, err
	}
	receipt = cloneDeletion(*s.journal.pairState().deleted)
	if post.Outcome != LocalNotFoundReported {
		return receipt, ErrUncertain
	}
	return receipt, nil
}
