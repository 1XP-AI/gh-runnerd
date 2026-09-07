//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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
	for _, stage := range []string{"pair", "host-preflight", "roster-anchor", "set-observe", "session-open", "poll", "source", "ack", "acquire", "continuation", "jit", "handoff", "worker-start", "identity-sample", "collection"} {
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

func TestPairedCurrentIdentityAfterIntent(t *testing.T) {
	for _, point := range []string{"pair", "session-open", "acquire", "jit"} {
		for _, change := range []string{"controller-file", "worker-file", "controller-claim", "worker-claim", "dependency", "cancel"} {
			t.Run(point+"/"+change, func(t *testing.T) {
				f := newPairedIntegrationFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				fired := false
				f.c.j.syncFile = func(file *os.File) error {
					if err := file.Sync(); err != nil {
						return err
					}
					raw, _ := os.ReadFile(file.Name())
					lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
					var e Event
					if json.Unmarshal(lines[len(lines)-1], &e) != nil || e.Baseline == nil || e.Baseline.Stage != point || e.Baseline.Outcome != "intent" || fired {
						return nil
					}
					fired = true
					path := ""
					switch change {
					case "controller-file":
						path = file.Name()
					case "worker-file":
						path = filepath.Join(f.wf.StateDirectory(), "journal.jsonl")
					case "controller-claim":
						path = filepath.Join(f.c.j.claim.directory, "admission.json")
					case "worker-claim":
						path = filepath.Join(f.wf.AdmissionDirectory(), "admission.json")
					case "dependency":
						f.c.api.baseURL += "/substituted"
					case "cancel":
						cancel()
					}
					if path != "" {
						old, err := os.ReadFile(path)
						if err != nil {
							return err
						}
						if err = os.Rename(path, path+".retained"); err != nil {
							return err
						}
						return os.WriteFile(path, old, 0600)
					}
					return nil
				}
				out, err := runPairedBaselineWithCadence(ctx, &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, fastPairCadence())
				if !fired || err == nil || out.Outcome != collectionUnresolved || f.creates.Load() != 0 || f.starts.Load() != 0 || f.cleanup.Load() != 0 {
					t.Fatal("current identity gate failed")
				}
				if point == "pair" || point == "session-open" {
					if f.c.sessions.Load() != 0 || f.c.acquires.Load() != 0 || f.jit.Load() != 0 {
						t.Fatal("effect after lost prefix authority")
					}
				}
				if point == "acquire" && f.c.acquires.Load() != 0 || point == "jit" && f.jit.Load() != 0 {
					t.Fatal("effect after current authority loss")
				}
			})
		}
	}
}

func TestPairedRosterAndHostStopBeforeSession(t *testing.T) {
	for _, mode := range []string{"roster-null", "roster-drift", "host-image"} {
		t.Run(mode, func(t *testing.T) {
			f := newPairedIntegrationFixture(t)
			if mode == "host-image" {
				f.dockerResponse = func(req *http.Request, value any) any {
					if strings.HasPrefix(req.URL.Path, "/v1.45/images/") {
						value.(map[string]any)["Id"] = "sha256:" + strings.Repeat("b", 64)
					}
					return value
				}
			} else {
				f.remote = func(req *http.Request, value any) any {
					if req.URL.Path == "/orgs/"+f.c.a.Organization+"/actions/runners" {
						if mode == "roster-null" {
							return map[string]any{"total_count": 0, "runners": nil}
						}
						return map[string]any{"total_count": 1, "runners": []any{map[string]any{"id": 999}}}
					}
					return value
				}
			}
			out, err := runFastPair(f)
			if err == nil || out.Outcome != collectionUnresolved || f.c.sessions.Load() != 0 || f.c.acquires.Load() != 0 || f.jit.Load() != 0 || f.creates.Load() != 0 {
				t.Fatal("unverified prefix admitted listener")
			}
		})
	}
}

func TestPairedCrossRoundEvidenceAndEarlyContradictions(t *testing.T) {
	for _, mode := range []string{"rest-id", "rest-name", "status-regression", "terminal-flip", "missing-later"} {
		t.Run(mode, func(t *testing.T) {
			f := newPairedIntegrationFixture(t)
			f.remote = func(req *http.Request, value any) any {
				second := f.jobLists.Load() >= 2
				var job map[string]any
				if strings.HasSuffix(req.URL.Path, "/attempts/1/jobs") {
					if mode == "missing-later" && second {
						return map[string]any{"total_count": 0, "jobs": []any{}}
					}
					job = value.(map[string]any)["jobs"].([]any)[0].(map[string]any)
				} else if strings.HasSuffix(req.URL.Path, "/actions/jobs/701") {
					job = value.(map[string]any)
				}
				if job != nil {
					if mode == "terminal-flip" {
						job["status"] = "completed"
						job["conclusion"] = "failure"
						if second {
							job["conclusion"] = "success"
						}
					}
					if second {
						switch mode {
						case "rest-id":
							job["runner_id"] = 9002
						case "rest-name":
							job["runner_name"] = "other"
						case "status-regression":
							job["status"] = "queued"
							job["conclusion"] = nil
						}
					}
				}
				return value
			}
			out, err := runFastPair(f)
			if mode == "missing-later" {
				if err != nil || out.Outcome != collectionCollected || out.Rounds != 8 || f.restReads.Load() != 8 || f.jobDetails.Load() != 1 {
					t.Fatal("later pending discarded previous positive identity")
				}
				return
			}
			if err == nil || out.Outcome != collectionUnresolved || out.Rounds != 1 || f.restReads.Load() != 1 || f.containerReads.Load() != 2 {
				t.Fatalf("contradiction did not fence exact next reader: rounds=%d rest=%d local=%d", out.Rounds, f.restReads.Load(), f.containerReads.Load())
			}
		})
	}
}

func TestPairedCrossStagePayloadsRefused(t *testing.T) {
	for _, inject := range []func(*baselineRecord){func(r *baselineRecord) { r.Pair = &baselinePair{} }, func(r *baselineRecord) { r.Host = &baselineHost{} }, func(r *baselineRecord) { r.Roster = &baselineRoster{} }, func(r *baselineRecord) { r.JIT = &baselineJIT{} }, func(r *baselineRecord) { r.Handoff = &baselineHandoff{} }, func(r *baselineRecord) { r.Start = &baselineStart{} }, func(r *baselineRecord) { r.Sample = &baselineSample{} }, func(r *baselineRecord) { r.Collection = &baselineCollectionFacts{} }} {
		for _, stage := range []string{"set-observe", "session-open", "poll", "source", "ack", "acquire", "continuation", "started", "completed", "desired"} {
			r := baselineRecord{Version: 1, Stage: stage, SetID: 7, Outcome: "intent"}
			inject(&r)
			if validBaselineShape(Event{Kind: "baseline", Baseline: &r}) {
				t.Fatal("new payload accepted on legacy baseline stage")
			}
		}
	}
}

func TestPairedCadenceWaitsAfterSlowRoundResult(t *testing.T) {
	f := newPairedIntegrationFixture(t)
	base := time.Now()
	var elapsed atomic.Int64
	var waits []time.Duration
	clock := pairedBaselineCadence{now: func() time.Time { return base.Add(time.Duration(elapsed.Load())) }, wait: func(ctx context.Context, d time.Duration) error {
		if ctx.Err() != nil {
			return ErrQuarantine
		}
		waits = append(waits, d)
		elapsed.Add(int64(d))
		return nil
	}}
	// Real transport response, synthetic elapsed reader time. Operation and
	// approval contexts keep their real deadlines throughout this fixture.
	f.c.afterResponse = func(req *http.Request, _ *http.Response) {
		if strings.HasSuffix(req.URL.Path, "/agents/81") {
			elapsed.Add(int64(10 * time.Second))
		}
	}
	out, err := runPairedBaselineWithCadence(context.Background(), &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, clock)
	if err != nil || out.Outcome != collectionCollected {
		t.Fatal("slow round fixture")
	}
	if len(waits) != 7 {
		t.Fatalf("missing completion-to-next-round gaps: %d", len(waits))
	}
	for _, d := range waits {
		if d < 5*time.Second {
			t.Fatal("round latency consumed the required gap")
		}
	}
}
