//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
)

func terminalLastDiskEvent(file *os.File) Event {
	data, _ := os.ReadFile(file.Name())
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	var e Event
	if len(lines) > 0 {
		_ = json.Unmarshal(lines[len(lines)-1], &e)
	}
	return e
}
func terminalRunContext(f *terminalFixture, ctx context.Context) (pairedBaselineTerminalResult, error) {
	return runPairedTerminalWithCadence(ctx, &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, fastPairCadence())
}
func terminalNoReplay(t *testing.T, f *terminalFixture) {
	t.Helper()
	before := [6]int32{f.c.requests.Load(), f.dockerReads.Load(), f.creates.Load(), f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load()}
	dir, admission := f.c.j.directory, f.c.j.claim.directory
	_ = f.c.j.Close()
	j, err := openJournalAtAdmission(dir, f.c.a, admission, func(file *os.File) error { return file.Sync() })
	if err == nil {
		f.c.j = j
		t.Cleanup(func() { _ = j.Close() })
		if f.wf.Reopen(f.w.Approval) == nil {
			f.w.Journal = f.wf.Journal
		}
		if _, err = f.run(); err == nil {
			t.Fatal("reopened terminal run accepted")
		}
	}
	after := [6]int32{f.c.requests.Load(), f.dockerReads.Load(), f.creates.Load(), f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load()}
	if before != after {
		t.Fatal("reopened terminal history issued another request")
	}
}
func TestPairedTerminalEligibilityUsesFreshExactFacts(t *testing.T) {
	for _, fault := range []string{"sdk-empty404", "rest-still-present", "job-failure", "exit-nonzero", "exit-missing", "running", "set-assigned", "set-running", "set-missing-stats", "set-wrong-owner", "set-update", "roster-drift", "post-worker-set-drift", "post-worker-roster-drift"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			f := newTerminalFixture(t, true)
			original := f.githubBefore
			f.githubBefore = func(w http.ResponseWriter, r *http.Request) bool {
				if f.c.polls.Load() >= 2 && strings.HasSuffix(r.URL.Path, "/agents/81") && fault == "sdk-empty404" {
					f.sdkReads.Add(1)
					w.WriteHeader(404)
					return true
				}
				if f.c.polls.Load() >= 2 && strings.HasSuffix(r.URL.Path, "/actions/runners/9001") && fault == "rest-still-present" {
					return false
				}
				return original(w, r)
			}
			f.remote = func(r *http.Request, v any) any {
				if f.c.polls.Load() >= 2 && fault == "job-failure" {
					if strings.HasSuffix(r.URL.Path, "/attempts/1/jobs") {
						v.(map[string]any)["jobs"].([]any)[0].(map[string]any)["conclusion"] = "failure"
					}
					if strings.HasSuffix(r.URL.Path, "/actions/jobs/701") {
						v.(map[string]any)["conclusion"] = "failure"
					}
				}
				if strings.HasSuffix(r.URL.Path, "/actions/runners") && (fault == "roster-drift" && f.rosters.Load() >= 2 || fault == "post-worker-roster-drift" && f.rosters.Load() >= 3) {
					return map[string]any{"total_count": 1, "runners": []any{map[string]any{"id": 999, "name": "another-runner", "status": "offline", "busy": false}}}
				}
				return v
			}
			f.dockerResponse = func(r *http.Request, v any) any {
				if f.c.polls.Load() >= 2 && strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
					st := v.(map[string]any)["State"].(map[string]any)
					switch fault {
					case "exit-nonzero":
						st["ExitCode"] = 23
					case "exit-missing":
						delete(st, "ExitCode")
					case "running":
						st["Status"] = "running"
						st["Running"] = true
					}
				}
				return v
			}
			f.controllerResponse = func(stage string, v any) any {
				if stage != "set" || f.c.sets.Load() < 2 {
					return v
				}
				set := v.(scaleset.RunnerScaleSet)
				switch fault {
				case "set-assigned":
					set.Statistics.TotalAssignedJobs = 1
				case "set-running":
					set.Statistics.TotalRunningJobs = 1
				case "set-missing-stats":
					set.Statistics = nil
				case "set-wrong-owner":
					set.Labels = nil
				case "set-update":
					set.RunnerSetting.DisableUpdate = false
				case "post-worker-set-drift":
					if f.c.sets.Load() >= 3 {
						set.Name = "another-set"
					}
				}
				return set
			}
			out, err := f.run()
			if err == nil || out.Terminal == terminalComplete || out.Collection.Outcome != collectionUnresolved {
				t.Fatal("ineligible terminal completed")
			}
			later := strings.HasPrefix(fault, "post-worker-")
			want := int32(0)
			if later {
				want = 1
			}
			if f.sessionDeletes.Load() != want || f.workerDeletes.Load() != want || f.setDeletes.Load() != 0 {
				t.Fatalf("effect crossed eligibility: close%d worker%d set%d", f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load())
			}
			terminalNoReplay(t, f)
		})
	}
}
func TestPairedTerminalCapturedAcknowledgementCancellation(t *testing.T) {
	for _, stage := range []string{"session", "set"} {
		t.Run(stage, func(t *testing.T) {
			f := newTerminalFixture(t, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fired := false
			f.c.afterResponse = func(r *http.Request, response *http.Response) {
				match := stage == "session" && strings.Contains(r.URL.Path, "/sessions/") || stage == "set" && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7")
				if r.Method == "DELETE" && response.StatusCode == 204 && match {
					fired = true
					cancel()
				}
			}
			out, err := terminalRunContext(f, ctx)
			facts := out.Collection.Terminal
			if !fired || err == nil || out.Terminal != terminalUnresolved || facts == nil || facts.Measurement != collectionCollected || facts.Evidence == nil || out.Collection.Rounds != 8 || !refPresent(facts.SessionCloseResult) || out.Collection.OutstandingSession != sessionCloseAcknowledged {
				t.Fatal("captured close acknowledgement/history lost")
			}
			if stage == "session" {
				if f.workerDeletes.Load() != 0 || f.setDeletes.Load() != 0 {
					t.Fatal("effect after canceled close")
				}
			} else {
				if !refPresent(facts.SetDeleteResult) || f.setAbsences.Load() != 0 || f.rosters.Load() != 3 {
					t.Fatal("set acknowledgement lost or postcheck after cancel")
				}
			}
			terminalNoReplay(t, f)
		})
	}
}
func TestPairedTerminalMissingAcknowledgementsAndPostchecks(t *testing.T) {
	for _, fault := range []string{"session404", "session-lost", "worker409", "worker-post500", "worker-post-empty404", "set-lost", "set-post200", "set-post500", "set-post-lost", "final-roster-drift"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			f := newTerminalFixture(t, true)
			github := f.githubBefore
			f.githubBefore = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/sessions/") {
					if fault == "session404" {
						f.sessionDeletes.Add(1)
						w.WriteHeader(404)
						return true
					}
					if fault == "session-lost" {
						f.sessionDeletes.Add(1)
						terminalLose(w)
						return true
					}
				}
				if fault == "set-lost" && r.Method == "DELETE" && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7") {
					f.setDeletes.Add(1)
					terminalLose(w)
					return true
				}
				return github(w, r)
			}
			docker := f.dockerBefore
			f.dockerBefore = func(w http.ResponseWriter, r *http.Request) bool {
				if fault == "worker409" && r.Method == "DELETE" {
					f.workerDeletes.Add(1)
					w.WriteHeader(409)
					return true
				}
				if f.deletedWorker.Load() && r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/json") {
					if fault == "worker-post500" || fault == "worker-post-empty404" {
						f.workerAbsences.Add(1)
						status := 500
						if fault == "worker-post-empty404" {
							status = 404
						}
						w.WriteHeader(status)
						return true
					}
				}
				return docker(w, r)
			}
			f.controllerResponse = func(stage string, v any) any {
				if stage == "set" && f.deletedSet.Load() {
					switch fault {
					case "set-post200":
						return baselineReply{status: 200, body: v}
					case "set-post500":
						return baselineReply{status: 500}
					case "set-post-lost":
						return baselineReply{lost: true}
					}
				}
				return v
			}
			f.remote = func(r *http.Request, v any) any {
				if fault == "final-roster-drift" && f.deletedSet.Load() && strings.HasSuffix(r.URL.Path, "/actions/runners") {
					return map[string]any{"total_count": 1, "runners": []any{map[string]any{"id": 91, "name": "extra", "status": "offline", "busy": false}}}
				}
				return v
			}
			out, err := f.run()
			facts := out.Collection.Terminal
			if err == nil || out.Terminal != terminalUnresolved || out.Collection.Outcome != collectionUnresolved || facts == nil || facts.Measurement != collectionCollected {
				t.Fatal("incomplete cleanup reported success/history lost")
			}
			if strings.HasPrefix(fault, "session") && (f.workerDeletes.Load() != 0 || f.setDeletes.Load() != 0 || refPresent(facts.SessionCloseResult)) {
				t.Fatal("missing close acknowledged")
			}
			if strings.HasPrefix(fault, "worker") && f.setDeletes.Load() != 0 {
				t.Fatal("set delete without W absence")
			}
			if strings.HasPrefix(fault, "worker-post") && (facts.WorkerDeletion == nil || facts.WorkerDeletion.AbsenceResult != nil) {
				t.Fatal("known worker delete receipt lost")
			}
			if strings.HasPrefix(fault, "set-post") && (!refPresent(facts.SetDeleteResult) || refPresent(facts.SetAbsenceResult) || f.rosters.Load() != 3) {
				t.Fatal("missing set absence accepted or deletion lost")
			}
			terminalNoReplay(t, f)
		})
	}
}
func terminalLose(w http.ResponseWriter) {
	if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
		_ = conn.Close()
	}
}

func TestPairedTerminalCompletionCadenceAndReceiptSeparation(t *testing.T) {
	f := newTerminalFixture(t, true)
	clock := fastPairCadence()
	wait := clock.wait
	var waits []time.Duration
	clock.wait = func(ctx context.Context, d time.Duration) error { waits = append(waits, d); return wait(ctx, d) }
	out, err := runPairedTerminalWithCadence(context.Background(), &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, clock)
	facts := out.Collection.Terminal
	if err != nil || facts == nil || len(waits) != 7 || facts.Outcome != terminalComplete || facts.Measurement != collectionCollected || facts.Evidence == nil || !refPresent(facts.Evidence.Started) || !refPresent(facts.SessionCloseResult) || facts.WorkerDeletion == nil || facts.WorkerDeletion.AbsenceResult == nil || !refPresent(facts.SetDeleteResult) || !refPresent(facts.SetAbsenceResult) || facts.SetAbsenceResult == facts.SetDeleteResult || out.Collection.OutstandingSession != sessionCloseAcknowledged {
		t.Fatal("terminal receipts not separate")
	}
	for _, d := range waits {
		if d < 5*time.Second {
			t.Fatal("terminal cadence shortened")
		}
	}
	if f.sdkReads.Load() != 8 || f.restReads.Load() != 8 || f.jobLists.Load() != 8 || f.jobDetails.Load() != 8 || len(f.wf.Journal.Events()) != 29 {
		t.Fatal("rounds/worker cached Bind writes changed")
	}
	terminalNoReplay(t, f)
}

func TestPairedTerminalEveryOriginalWorkerPhaseRequired(t *testing.T) {
	for _, missing := range []string{"create", "start", "inspect", "cleanup"} {
		t.Run(missing, func(t *testing.T) {
			var phases []string
			for _, p := range []string{"create", "start", "inspect", "cleanup"} {
				if p != missing {
					phases = append(phases, p)
				}
			}
			f := newTerminalFixtureWithPhases(t, phases)
			if _, err := f.run(); err == nil || f.c.requests.Load() != 0 || f.dockerReads.Load() != 0 || len(f.c.j.Events()) != 3 || len(f.wf.Journal.Events()) != 0 {
				t.Fatal("missing original phase reached prefix")
			}
		})
	}
}

func TestPairedTerminalOriginalDeadlineAtClose(t *testing.T) {
	f := newTerminalFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reached := false
	f.c.j.syncFile = func(file *os.File) error {
		e := terminalLastDiskEvent(file)
		if e.Baseline != nil && e.Baseline.Stage == "terminal-session-close" && e.Baseline.Outcome == "intent" && !reached {
			reached = true
			<-ctx.Done()
		}
		return file.Sync()
	}
	out, err := terminalRunContext(f, ctx)
	if !reached || ctx.Err() != context.DeadlineExceeded || err == nil || out.Terminal != terminalUnresolved || f.sessionDeletes.Load() != 0 || out.Collection.Terminal == nil || out.Collection.Terminal.Measurement != collectionCollected {
		t.Fatal("original deadline did not bound terminal effect")
	}
}
func TestPairedTerminalCollectionOnlyRemainsEffectFree(t *testing.T) {
	f := newTerminalFixture(t, true)
	out, err := runFastPair(f.pairedIntegrationFixture)
	if err != nil || out.Outcome != collectionCollected || out.Terminal != nil || out.OutstandingSession != sessionKnownOpen || f.sessionDeletes.Load() != 0 || f.workerDeletes.Load() != 0 || f.setDeletes.Load() != 0 || f.rosters.Load() != 1 {
		t.Fatal("collection-only path enabled terminal cleanup")
	}
	for _, e := range f.c.j.Events() {
		if e.Baseline != nil && e.Baseline.Terminal != nil {
			t.Fatal("collection-only terminal record")
		}
	}
}
