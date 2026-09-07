// Package contract is an offline protocol experiment, not a production adapter.
package contract

import (
	"context"

	"github.com/actions/scaleset"
)

type recoveryState struct {
	Desired int
	Workers map[string]string
}

// recoverState is initially a callback-only baseline: it has no independent
// reconciliation. The first red run demonstrates the lost callback counterexample.
func recoverState(ctx context.Context, client *scaleset.Client, state recoveryState) (recoveryState, error) {
	return state, nil
}
