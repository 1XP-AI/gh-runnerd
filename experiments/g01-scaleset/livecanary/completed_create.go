package livecanary

import "reflect"

// completedCreateHistory describes stored protocol evidence, never authority.
// Callers must independently hold current journal/approval/credential authority.
// In particular, consumption hashes cannot reconstruct a receipt or create proof.
type completedCreateHistory struct {
	state               state
	inventory, creation Event
}

// parseCompletedCreateHistory accepts the writer's empty-work create protocol,
// preserving physical sequences and event bytes for controllerEventRef. Authority
// records are structural here: the journal opener owns their renewal-chain policy.
// Legacy six-record histories remain evidence under the existing trusted-journal
// model; neither form authenticates historic receipt provenance.
func parseCompletedCreateHistory(events []Event) (completedCreateHistory, error) {
	var h completedCreateHistory
	step := 0
	for index, e := range events {
		if e.Sequence != index+1 {
			return completedCreateHistory{}, ErrQuarantine
		}
		if e.Kind == "authority" {
			if e.Authority == nil || !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Kind: "authority", Authority: e.Authority}) {
				return completedCreateHistory{}, ErrQuarantine
			}
			continue
		}
		if index == 0 && e.Kind == "controller-handoff-consumed" {
			if !validControllerHandoffEvent(e) || e.ControllerHandoff.Phase != "create" {
				return completedCreateHistory{}, ErrQuarantine
			}
			continue
		}
		expected := Event{Sequence: e.Sequence}
		switch step {
		case 0:
			expected.Kind, expected.Operation = "phase", "create"
		case 1:
			if !validHandoffSHA256(e.Digest) {
				return completedCreateHistory{}, ErrQuarantine
			}
			expected.Kind, expected.Digest = "inventory", e.Digest
			h.inventory = e
		case 2:
			expected.Kind, expected.Operation = "intent", "observe-discovery"
		case 3:
			expected.Kind, expected.Operation = "result", "observe-discovery"
		case 4:
			expected.Kind, expected.Operation = "intent", "create"
		case 5:
			if e.ID <= 0 {
				return completedCreateHistory{}, ErrQuarantine
			}
			expected.Kind, expected.Operation, expected.ID = "result", "create", e.ID
			h.creation = e
		default:
			return completedCreateHistory{}, ErrQuarantine
		}
		if !reflect.DeepEqual(e, expected) {
			return completedCreateHistory{}, ErrQuarantine
		}
		step++
	}
	if step != 6 {
		return completedCreateHistory{}, ErrQuarantine
	}
	h.state = replay(events)
	if h.state.uncertain || h.state.deleted || h.state.reserved || h.state.workObserved || len(h.state.observedJobs) != 0 || h.state.setID <= 0 || h.state.inventory == "" {
		return completedCreateHistory{}, ErrQuarantine
	}
	return h, nil
}
