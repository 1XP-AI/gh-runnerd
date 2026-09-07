//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
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

func fastPairCadence() pairedBaselineCadence {
	now := time.Now()
	return pairedBaselineCadence{now: func() time.Time { return now }, wait: func(ctx context.Context, d time.Duration) error {
		if ctx.Err() != nil {
			return ErrQuarantine
		}
		now = now.Add(d)
		return nil
	}}
}
func runFastPair(f *pairedIntegrationFixture) (pairedBaselineCollection, error) {
	return runPairedBaselineWithCadence(context.Background(), &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, fastPairCadence())
}

func TestPairedDistinctIDsAndOriginalCadence(t *testing.T) {
	f := newPairedIntegrationFixture(t)
	clock := fastPairCadence()
	wait := clock.wait
	var delays []time.Duration
	clock.wait = func(ctx context.Context, d time.Duration) error { delays = append(delays, d); return wait(ctx, d) }
	out, err := runPairedBaselineWithCadence(context.Background(), &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, clock)
	if err != nil || out.Outcome != collectionCollected || len(delays) != 7 || f.sdkReads.Load() != 8 || f.restReads.Load() != 8 || f.jobLists.Load() != 8 || f.jobDetails.Load() != 8 || f.containerReads.Load() != 9 {
		t.Fatalf("bounded distinct-domain collection failed: rounds=%d sdk=%d rest=%d local=%d", out.Rounds, f.sdkReads.Load(), f.restReads.Load(), f.containerReads.Load())
	}
	for _, d := range delays {
		if d < 5*time.Second {
			t.Fatal("shortened cadence")
		}
	}
	if f.c.forbidden.Load() != 0 || f.cleanup.Load() != 0 {
		t.Fatal("unexpected target or cleanup")
	}
}

func TestPairedActualJournalSyncFailureMatrix(t *testing.T) {
	// Every integration stage's intent/result, plus listener parent, session and
	// final summary. A written-but-unsynced record never authorizes a fresh run.
	for _, stage := range []string{"pair", "host-preflight", "roster-anchor", "set-observe", "session-open", "acquire", "continuation", "jit", "handoff", "worker-start", "identity-sample", "collection"} {
		outcomes := []string{"intent", "result"}
		if stage == "collection" {
			outcomes = []string{"observed"}
		}
		for _, outcome := range outcomes {
			t.Run(stage+"/"+outcome, func(t *testing.T) {
				f := newPairedIntegrationFixture(t)
				fired := false
				f.c.j.syncFile = func(file *os.File) error {
					raw, err := os.ReadFile(file.Name())
					if err != nil {
						return err
					}
					lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
					var e Event
					if json.Unmarshal(lines[len(lines)-1], &e) == nil && e.Baseline != nil && e.Baseline.Stage == stage && e.Baseline.Outcome == outcome && !fired {
						fired = true
						return ErrJournal
					}
					return file.Sync()
				}
				result, err := runFastPair(f)
				if !fired {
					t.Fatal("selected actual write not reached")
				}
				if err == nil || result.Outcome == collectionCollected {
					t.Fatal("failed persistence returned successful collection")
				}
				before := [4]int32{f.c.requests.Load(), f.jit.Load(), f.creates.Load(), f.starts.Load()}
				dir, admission := f.c.j.directory, f.c.j.claim.directory
				_ = f.c.j.Close()
				reopened, reopenErr := openJournalAtAdmission(dir, f.c.a, admission, func(file *os.File) error { return file.Sync() })
				if reopenErr == nil {
					f.c.j = reopened
					t.Cleanup(func() { _ = reopened.Close() })
					if err := f.wf.Reopen(f.w.Approval); err == nil {
						f.w.Journal = f.wf.Journal
					}
					again, retryErr := runFastPair(f)
					if retryErr == nil || again.Outcome == collectionCollected {
						t.Fatal("reopened prefix resumed")
					}
				}
				after := [4]int32{f.c.requests.Load(), f.jit.Load(), f.creates.Load(), f.starts.Load()}
				if before != after || f.cleanup.Load() != 0 {
					t.Fatal("reopen repeated effects")
				}
			})
		}
	}
	for n := 1; n <= 7; n++ {
		t.Run("worker/"+strconv.Itoa(n), func(t *testing.T) {
			f := newPairedIntegrationFixture(t)
			f.wf.FailRecordSyncForTest(n)
			out, err := runFastPair(f)
			if err == nil || out.Outcome == collectionCollected {
				t.Fatal("worker sync failure accepted")
			}
			before := [3]int32{f.jit.Load(), f.creates.Load(), f.starts.Load()}
			if err := f.wf.Reopen(f.w.Approval); err == nil {
				f.w.Journal = f.wf.Journal
				_, _ = runFastPair(f)
			}
			if before != ([3]int32{f.jit.Load(), f.creates.Load(), f.starts.Load()}) || f.cleanup.Load() != 0 {
				t.Fatal("worker reopen repeated effects")
			}
		})
	}
}

func TestPairedJITCanceledAfterObservedResponseRetainsTuple(t *testing.T) {
	f := newPairedIntegrationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.c.afterResponse = func(req *http.Request, resp *http.Response) {
		if strings.HasSuffix(req.URL.Path, "/generatejitconfig") && resp.StatusCode == 200 {
			cancel()
		}
	}
	out, err := runPairedBaselineWithCadence(ctx, &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, fastPairCadence())
	if err == nil || out.Outcome != collectionUnresolved || f.jit.Load() != 1 || f.creates.Load() != 0 {
		t.Fatal("canceled JIT effect boundary")
	}
	var found bool
	for _, e := range f.c.j.Events() {
		if r := e.Baseline; r != nil && r.Stage == "jit" && r.Outcome == "unknown" {
			found = r.HTTPStatus == 200 && r.JIT.Runner != nil && r.JIT.Runner.ID == 81 && r.JIT.Runner.Name == f.c.a.workerName() && r.JIT.Runner.ScaleSetID == 7
		}
	}
	if !found {
		t.Fatal("bounded observed JIT runner tuple lost after cancellation")
	}
}
