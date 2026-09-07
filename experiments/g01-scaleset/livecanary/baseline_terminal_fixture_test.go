//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type terminalFixture struct {
	controllerResponse func(string, any) any
	*pairedIntegrationFixture
	sessionDeletes, workerDeletes, setDeletes, setAbsences, workerAbsences, rosters atomic.Int32
	deletedSet, deletedWorker                                                       atomic.Bool
}

func newTerminalFixture(t *testing.T, cleanup bool) *terminalFixture {
	phases := []string{"create", "start", "inspect"}
	if cleanup {
		phases = append(phases, "cleanup")
	}
	return newTerminalFixtureWithPhases(t, phases)
}
func newTerminalFixtureWithPhases(t *testing.T, phases []string) *terminalFixture {
	return newTerminalFixtureWithControllerPhases(t, phases, nil)
}
func newTerminalFixtureWithControllerPhases(t *testing.T, phases, controllerPhases []string) *terminalFixture {
	t.Helper()
	f := &terminalFixture{}
	config := &pairedFixtureConfiguration{controllerResponse: func(stage string, value any) any {
		if f.controllerResponse != nil {
			value = f.controllerResponse(stage, value)
		}
		if stage == "set" && f.deletedSet.Load() {
			if _, ok := value.(baselineReply); ok {
				return value
			}
			f.setAbsences.Add(1)
			return baselineReply{status: 404}
		}
		return value
	}}
	config.workerPhases = phases
	config.controllerPhases = controllerPhases
	f.pairedIntegrationFixture = newPairedIntegrationFixtureConfigured(t, config)
	f.githubBefore = func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == "DELETE" && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7/sessions/00000000-0000-4000-8000-000000000001"):
			f.sessionDeletes.Add(1)
			w.WriteHeader(204)
			return true
		case r.Method == "DELETE" && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
			f.setDeletes.Add(1)
			f.deletedSet.Store(true)
			w.WriteHeader(204)
			return true
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/actions/runners"):
			f.rosters.Add(1)
		case r.Method == "GET" && f.c.polls.Load() >= 2 && strings.HasSuffix(r.URL.Path, "/agents/81"):
			f.sdkReads.Add(1)
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"typeName":"AgentNotFoundException","message":"synthetic missing runner"}`))
			return true
		case r.Method == "GET" && f.c.polls.Load() >= 2 && strings.HasSuffix(r.URL.Path, "/actions/runners/9001"):
			f.restReads.Add(1)
			w.WriteHeader(404)
			return true
		}
		return false
	}
	f.dockerBefore = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == "DELETE" && r.URL.Path == "/v1.45/containers/"+strings.Repeat("c", 64) {
			if r.URL.Query().Get("force") != "false" || r.URL.Query().Get("v") != "false" {
				t.Error("force or volume removal requested")
				w.WriteHeader(403)
				return true
			}
			f.workerDeletes.Add(1)
			f.deletedWorker.Store(true)
			w.WriteHeader(204)
			return true
		}
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/json") {
			if f.deletedWorker.Load() {
				f.workerAbsences.Add(1)
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"message":"synthetic no such container"}`))
				return true
			}
			if f.c.polls.Load() >= 2 {
				f.container["State"] = map[string]any{"Status": "exited", "Running": false, "Paused": false, "Restarting": false, "Dead": false, "ExitCode": 0}
			}
		}
		return false
	}
	return f
}
func (f *terminalFixture) run() (pairedBaselineTerminalResult, error) {
	return runPairedTerminalWithCadence(context.Background(), &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, fastPairCadence())
}
func TestPairedTerminalActualJournalsFinalize(t *testing.T) {
	f := newTerminalFixture(t, true)
	if len(f.c.j.Events()) != 3 || f.wf.Journal == nil {
		t.Fatal("actual journal fixture not prepared")
	}
	out, err := f.run()
	if err != nil || out.Terminal != terminalComplete || out.Collection.Outcome != collectionCollected || out.Collection.Rounds != 8 || out.Collection.Result.Sequence == 0 {
		t.Fatalf("terminal feature unavailable: outcome=%s measurement=%s rounds=%d error=%v", out.Terminal, out.Collection.Outcome, out.Collection.Rounds, err)
	}
	if f.c.acquires.Load() != 1 || f.jit.Load() != 1 || f.creates.Load() != 1 || f.starts.Load() != 1 || f.sessionDeletes.Load() != 1 || f.workerDeletes.Load() != 1 || f.setDeletes.Load() != 1 || f.workerAbsences.Load() != 1 || f.setAbsences.Load() != 1 || f.rosters.Load() != 4 || f.cleanup.Load() != 0 || f.c.forbidden.Load() != 0 {
		t.Fatalf("terminal sequence: acquire=%d JIT=%d create=%d start=%d session=%d worker=%d set=%d worker404=%d set404=%d roster=%d forbidden=%d", f.c.acquires.Load(), f.jit.Load(), f.creates.Load(), f.starts.Load(), f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load(), f.workerAbsences.Load(), f.setAbsences.Load(), f.rosters.Load(), f.c.forbidden.Load())
	}
}
func TestPairedTerminalMissingCleanupRefusesBeforePrefix(t *testing.T) {
	f := newTerminalFixture(t, false)
	_, err := f.run()
	if err == nil || f.c.requests.Load() != 0 || f.dockerReads.Load() != 0 || len(f.c.j.Events()) != 3 {
		t.Fatal("missing cleanup authority reached the prefix")
	}
}
