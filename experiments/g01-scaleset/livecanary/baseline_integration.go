package livecanary

import (
	"context"
	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

type pairedBaselineCollection struct {
	Result             controllerRecordRef `json:"result"`
	Outcome            string              `json:"outcome"`
	Rounds             int                 `json:"rounds"`
	OutstandingSession string              `json:"outstanding_session"`
}

func runPairedBaseline(context.Context, *Driver, *liveworker.Driver) (pairedBaselineCollection, error) {
	return pairedBaselineCollection{Outcome: "unresolved", OutstandingSession: "none"}, ErrQuarantine
}
