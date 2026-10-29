package livecanary

import "context"

func controllerPhaseConsumed(events []Event, phase string) bool {
	for _, e := range events {
		if e.ControllerHandoff != nil && e.ControllerHandoff.Phase == phase || e.Kind == "phase" && e.Operation == phase {
			return true
		}
	}
	return false
}

// Only the exact current paired invocation can follow a completed create.
// The historical parser remains strict and physical sequence numbers survive.
func parsePairedCreatePrelude(events []Event) (completedCreateHistory, error) {
	if len(events) > 0 {
		last := events[len(events)-1]
		if last.ControllerHandoff != nil && last.ControllerHandoff.Phase == pairedPreparationPhase {
			if !validControllerHandoffEvent(last) || last.Sequence != len(events) {
				return completedCreateHistory{}, ErrQuarantine
			}
			return parseCompletedCreateHistory(events[:len(events)-1])
		}
	}
	return parseCompletedCreateHistory(events)
}

type phaseProofContextKey struct{}

func pairedProofFromContext(ctx context.Context) *controllerHandoffProof {
	p, _ := ctx.Value(phaseProofContextKey{}).(*controllerHandoffProof)
	return p
}

type experimentalPhaseAdmissionKey struct{}

func phaseHandoffEligibleHistory(a Approval, phase string, events []Event) bool {
	if phase == replacementPhase || jitBoundaryPhase(phase) {
		return len(events) == 7
	}
	if controllerPhaseConsumed(events, phase) {
		return false
	}
	if phase == pairedPreparationPhase {
		_, err := parseCompletedCreateHistory(events)
		return err == nil
	}
	if phase == "cleanup" {
		return false
	}
	s := replayWithApproval(events, &a)
	if s.phaseSeen[phase] || phase != "inspect" && (s.uncertain || s.deleted) {
		return false
	}
	return phase == "inspect" || s.setID > 0 && !s.reserved
}

func controllerHandoffMode(phase string) string {
	if phase == pairedPreparationPhase {
		return pairedPreparationPhase
	}
	return "controller"
}
