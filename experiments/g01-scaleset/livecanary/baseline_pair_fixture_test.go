//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

const pairFixtureJIT = "c3ludGhldGljLXByaXZhdGUtaml0LWNvbmZpZw=="

type pairedIntegrationFixture struct {
	githubBefore, dockerBefore                 func(http.ResponseWriter, *http.Request) bool
	dockerResponse                             func(*http.Request, any) any
	containerReads                             atomic.Int32
	c                                          *baselineFixture
	w                                          *liveworker.Driver
	wf                                         *liveworker.PairFileFixture
	jit, creates, starts, dockerReads, cleanup atomic.Int32
	mu                                         sync.Mutex
	container                                  map[string]any
	sdkReads, restReads, jobLists, jobDetails  atomic.Int32
	remote                                     func(*http.Request, any) any
}

type pairedFixtureConfiguration struct {
	workerPhases       []string
	controllerPhases   []string
	controllerResponse func(string, any) any
}

func newPairedIntegrationFixture(t *testing.T) *pairedIntegrationFixture {
	return newPairedIntegrationFixtureConfigured(t, nil)
}
func newPairedIntegrationFixtureConfigured(t *testing.T, config *pairedFixtureConfiguration) *pairedIntegrationFixture {
	t.Helper()
	f := &pairedIntegrationFixture{}
	a := approval()
	if config != nil && config.controllerPhases != nil {
		a.Phases = append([]string(nil), config.controllerPhases...)
	}
	empty := sha256.Sum256([]byte("null"))
	f.c = newBaselineFixtureWithApproval(t, func(stage string, value any) any {
		if strings.HasSuffix(stage, "-items") {
			for _, item := range value.([]any) {
				m := item.(map[string]any)
				if _, ok := m["runnerId"]; ok {
					m["runnerName"] = a.workerName()
				}
			}
		}
		if config != nil && config.controllerResponse != nil {
			return config.controllerResponse(stage, value)
		}
		return value
	}, hex.EncodeToString(empty[:]), a)
	// The listener fixture normally owns C life; integration owns both leases.
	f.c.release()
	f.c.release = nil
	f.c.extra = func(w http.ResponseWriter, r *http.Request) bool {
		if f.githubBefore != nil && f.githubBefore(w, r) {
			return true
		}
		var value any
		switch {
		case r.Method == "GET" && r.URL.Path == "/orgs/"+a.Organization+"/actions/runners":
			value = map[string]any{"total_count": 0, "runners": []any{}}
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/generatejitconfig"):
			f.jit.Add(1)
			value = map[string]any{"runner": map[string]any{"id": 81, "name": a.workerName(), "runnerScaleSetId": 7}, "encodedJITConfig": pairFixtureJIT}
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/agents/81"):
			f.sdkReads.Add(1)
			value = map[string]any{"id": 81, "name": a.workerName(), "runnerScaleSetId": 7}
		case r.Method == "GET" && r.URL.Path == "/orgs/"+a.Organization+"/actions/runners/9001":
			f.restReads.Add(1)
			value = map[string]any{"id": 9001, "name": a.workerName(), "status": "online", "busy": true}
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/attempts/1/jobs"):
			f.jobLists.Add(1)
			value = map[string]any{"total_count": 1, "jobs": []any{f.job()}}
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/actions/jobs/701"):
			f.jobDetails.Add(1)
			value = f.job()
		default:
			if r.Method == "DELETE" {
				f.cleanup.Add(1)
			}
			return false
		}
		if f.remote != nil {
			value = f.remote(r, value)
		}
		_ = json.NewEncoder(w).Encode(value)
		return true
	}
	dir, err := os.MkdirTemp("", "g01bp-")
	if err != nil {
		t.Fatal("private socket root")
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	wa := liveworker.Approval{RunnerUpdatesDisabled: true, HarnessSHA: a.HarnessSHA, WorkflowSHA: a.WorkflowSHA, OwnerNonce: a.OwnerNonce, Controller: a.Controller, Endpoint: filepath.Join(dir, "api.sock"), DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("a", 64), Image: liveworker.ImageReference, ExpiresAt: a.ExpiresAt, Phases: []string{"create", "start", "inspect"}}
	if config != nil && config.workerPhases != nil {
		wa.Phases = append([]string(nil), config.workerPhases...)
	}
	listener, err := net.Listen("unix", wa.Endpoint)
	if err != nil {
		t.Fatal("private Unix listener")
	}
	if os.Chmod(wa.Endpoint, 0600) != nil {
		t.Fatal("private socket mode")
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.dockerBefore != nil && f.dockerBefore(w, r) {
			return
		}
		var value any
		switch {
		case r.Method == "GET" && r.URL.Path == "/version":
			f.dockerReads.Add(1)
			value = map[string]any{"ApiVersion": "1.51", "MinAPIVersion": "1.24"}
		case r.Method == "GET" && r.URL.Path == "/v1.45/info":
			f.dockerReads.Add(1)
			value = map[string]any{"ID": wa.DaemonID, "OSType": "linux", "Architecture": "aarch64", "NCPU": 4, "MemTotal": int64(4 << 30), "MemoryLimit": true, "SwapLimit": true, "CpuCfsQuota": true, "PidsLimit": true, "Warnings": []string{}}
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1.45/images/"):
			f.dockerReads.Add(1)
			value = map[string]any{"Id": wa.ImageID, "Os": "linux", "Architecture": "arm64", "RepoDigests": []string{wa.Image}, "Config": map[string]any{"Env": []string{"PATH=/usr/bin"}, "Labels": map[string]string{}, "Volumes": map[string]any{}, "ExposedPorts": map[string]any{}}}
		case r.Method == "POST" && r.URL.Path == "/v1.45/containers/create":
			var payload map[string]any
			if json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&payload) != nil {
				t.Error("invalid fixture creation")
				w.WriteHeader(400)
				return
			}
			host := payload["HostConfig"]
			delete(payload, "HostConfig")
			payload["Env"] = append([]any{"PATH=/usr/bin"}, payload["Env"].([]any)...)
			f.container = map[string]any{"Id": strings.Repeat("c", 64), "Name": "/" + r.URL.Query().Get("name"), "Image": wa.ImageID, "Path": "/home/runner/bin/Runner.Listener", "Args": []string{"run", "--once"}, "Config": payload, "HostConfig": host, "Mounts": []any{}, "State": map[string]any{"Status": "created", "Running": false, "Paused": false, "Restarting": false, "Dead": false, "ExitCode": 0}, "NetworkSettings": map[string]any{"Networks": map[string]any{"bridge": map[string]any{}}}}
			f.creates.Add(1)
			w.WriteHeader(201)
			value = map[string]any{"Id": strings.Repeat("c", 64), "Warnings": []string{}}
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/start"):
			f.starts.Add(1)
			f.container["State"] = map[string]any{"Status": "running", "Running": true, "Paused": false, "Restarting": false, "Dead": false, "ExitCode": 0}
			w.WriteHeader(204)
			return
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/json"):
			f.dockerReads.Add(1)
			f.containerReads.Add(1)
			value = f.container
		default:
			f.cleanup.Add(1)
			w.WriteHeader(403)
			return
		}
		if f.dockerResponse != nil {
			value = f.dockerResponse(r, value)
		}
		_ = json.NewEncoder(w).Encode(value)
	})}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close(); _ = listener.Close() })
	docker, err := liveworker.NewDocker(wa)
	if err != nil {
		t.Fatal("concrete Docker fixture")
	}
	f.wf = liveworker.NewPairFixtureForTest(t, wa)
	f.w = &liveworker.Driver{Approval: wa, Journal: f.wf.Journal, Runtime: docker}
	return f
}

func (f *pairedIntegrationFixture) job() map[string]any {
	status := "in_progress"
	var conclusion any
	if f.c.polls.Load() >= 2 {
		status = "completed"
		conclusion = "success"
	}
	return map[string]any{"id": 701, "run_id": f.c.a.WorkflowRunID, "run_attempt": 1, "head_sha": f.c.a.WorkflowSHA, "status": status, "conclusion": conclusion, "runner_id": 9001, "runner_name": f.c.a.workerName(), "runner_group_id": f.c.a.RunnerGroupID}
}

func TestPairedBaselineActualJournalsExecuteAndCollect(t *testing.T) {
	f := newPairedIntegrationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := runPairedBaseline(ctx, &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w)
	if err != nil || result.Outcome != "collected" || result.Rounds != 8 || result.OutstandingSession != "known-open" || result.Result.Sequence == 0 {
		t.Fatal("actual paired execution/collection unavailable")
	}
	if f.jit.Load() != 1 || f.creates.Load() != 1 || f.starts.Load() != 1 || f.c.acquires.Load() != 1 || f.c.acks.Load() != 2 || f.cleanup.Load() != 0 || f.c.forbidden.Load() != 0 {
		t.Fatal("singleton or zero-cleanup contract")
	}
	for _, events := range []any{f.c.j.Events(), f.wf.Journal.Events()} {
		raw, _ := json.Marshal(events)
		if strings.Contains(string(raw), pairFixtureJIT) || strings.Contains(string(raw), f.c.api.credentials.InstallationToken) || strings.Contains(string(raw), f.c.api.credentials.VerificationToken) {
			t.Fatal("private handoff secret persisted")
		}
	}
	if decoded, err := base64.StdEncoding.DecodeString(pairFixtureJIT); err != nil || len(decoded) == 0 {
		t.Fatal("fixture JIT")
	}
}
