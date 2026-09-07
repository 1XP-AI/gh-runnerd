package livecanary

import (
	"context"
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

// This private, fixed entry is distinct from collection-only execution.
// The compiled feature scaffold refuses before any new effect.
func runPairedTerminalWithCadence(ctx context.Context, d *Driver, w *liveworker.Driver, cadence pairedBaselineCadence) (pairedBaselineTerminalResult, error) {
	return pairedBaselineTerminalResult{Collection: pairedBaselineCollection{Outcome: collectionUnresolved, OutstandingSession: sessionNone}, Terminal: terminalUnresolved}, ErrQuarantine
}
