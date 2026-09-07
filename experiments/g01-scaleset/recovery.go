// Package contract is an offline protocol experiment, not a production adapter.
package contract

import (
	"context"
	"errors"

	"github.com/actions/scaleset"
)

type recoveryState struct {
	Desired         int
	Workers         map[string]string
	References      map[string]int
	AdmissionPaused bool
}

// recoverState demonstrates independent reads through supported SDK methods.
// Its input represents surviving non-secret intent, not a durable store. It
// neither creates, retries, deletes nor releases workers. Production persistence,
// freshness fencing, caps and provider ownership validation belong to later gates.
func recoverState(ctx context.Context, client *scaleset.Client, state recoveryState) (recoveryState, error) {
	state.AdmissionPaused = true
	workers := make(map[string]string, len(state.Workers))
	for name := range state.Workers {
		workers[name] = "quarantined"
	}
	state.Workers = workers
	state.References = make(map[string]int)
	set, err := client.GetRunnerScaleSetByID(ctx, 7)
	if err != nil || set == nil || set.ID != 7 || set.Statistics == nil || set.Statistics.TotalAssignedJobs < 0 {
		return state, errors.New("recovery statistics unavailable")
	}
	state.Desired = set.Statistics.TotalAssignedJobs
	for name := range workers {
		ref, err := client.GetRunnerByName(ctx, name)
		if err != nil {
			return state, errors.New("recovery inventory unavailable")
		}
		if ref == nil {
			continue
		} // Absence does not prove the local process exited.
		if ref.Name != name || ref.RunnerScaleSetID != 7 || ref.ID <= 0 {
			return state, errors.New("recovery ownership mismatch")
		}
		state.References[name] = ref.ID
	}
	// A read returning aggregate demand does not establish current provider state
	// or a server drain barrier. Leave admission paused and reservations held.
	return state, nil
}
