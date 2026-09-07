//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestPairedStopsAtFirstContradictoryReader(t *testing.T) {
	f := newPairedIntegrationFixture(t)
	f.remote = func(r *http.Request, value any) any {
		if strings.HasSuffix(r.URL.Path, "/attempts/1/jobs") {
			value.(map[string]any)["jobs"].([]any)[0].(map[string]any)["runner_name"] = "another-runner"
		}
		if strings.HasSuffix(r.URL.Path, "/actions/jobs/701") {
			value.(map[string]any)["runner_name"] = "another-runner"
		}
		return value
	}
	out, err := runPairedBaseline(context.Background(), &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w)
	if err == nil || out.Outcome != collectionUnresolved {
		t.Fatal("contradictory job accepted")
	}
	// Start verifies the original created container once. An invalid REST job
	// must stop this round before either later REST-runner or worker observation.
	if f.restReads.Load() != 0 || f.containerReads.Load() != 1 {
		t.Fatalf("later reader ran after contradiction: REST=%d worker=%d", f.restReads.Load(), f.containerReads.Load())
	}
	events := f.c.j.Events()
	var child, parent bool
	for _, e := range events {
		if r := e.Baseline; r != nil && r.Outcome == "unknown" {
			child = child || r.Stage == "identity-sample"
			parent = parent || r.Stage == "continuation"
		}
	}
	if !child || !parent || out.OutstandingSession != sessionKnownOpen {
		t.Fatal("child and parent uncertainty not retained")
	}
}
