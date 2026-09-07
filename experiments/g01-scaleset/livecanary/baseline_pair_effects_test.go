//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPairedJITWireRefusals(t *testing.T) {
	for _, mode := range []string{"nil-runner", "id-zero", "wrong-name", "wrong-set", "empty-secret", "invalid-base64", "duplicate", "null"} {
		t.Run(mode, func(t *testing.T) {
			f := newPairedIntegrationFixture(t)
			f.remote = func(req *http.Request, value any) any {
				if !strings.HasSuffix(req.URL.Path, "/generatejitconfig") {
					return value
				}
				m := value.(map[string]any)
				switch mode {
				case "nil-runner":
					m["runner"] = nil
				case "id-zero":
					m["runner"].(map[string]any)["id"] = 0
				case "wrong-name":
					m["runner"].(map[string]any)["name"] = "another-runner"
				case "wrong-set":
					m["runner"].(map[string]any)["runnerScaleSetId"] = 8
				case "empty-secret":
					m["encodedJITConfig"] = ""
				case "invalid-base64":
					m["encodedJITConfig"] = strings.Repeat("!", 32)
				case "duplicate":
					return json.RawMessage(`{"runner":{},"runner":{},"encodedJITConfig":"invalid"}`)
				case "null":
					return nil
				}
				return m
			}
			out, err := runFastPair(f)
			if err == nil || out.Outcome != collectionUnresolved || f.c.acquires.Load() != 1 || f.jit.Load() != 1 || f.creates.Load() != 0 || f.starts.Load() != 0 {
				t.Fatal("invalid JIT supplied execution authority")
			}
			var child, parent bool
			for _, e := range f.c.j.Events() {
				if r := e.Baseline; r != nil && r.Outcome == "unknown" {
					child = child || r.Stage == "jit"
					parent = parent || r.Stage == "continuation"
				}
			}
			if !child || !parent {
				t.Fatal("invalid JIT uncertainty layers lost")
			}
		})
	}
}

func TestPairedLostEffectsNeverRepeat(t *testing.T) {
	for _, mode := range []string{"jit", "create", "start"} {
		t.Run(mode, func(t *testing.T) {
			f := newPairedIntegrationFixture(t)
			drop := func(w http.ResponseWriter) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
			}
			if mode == "jit" {
				f.githubBefore = func(w http.ResponseWriter, r *http.Request) bool {
					if strings.HasSuffix(r.URL.Path, "/generatejitconfig") {
						f.jit.Add(1)
						drop(w)
						return true
					}
					return false
				}
			} else {
				f.dockerBefore = func(w http.ResponseWriter, r *http.Request) bool {
					if mode == "create" && r.Method == "POST" && r.URL.Path == "/v1.45/containers/create" {
						f.creates.Add(1)
						drop(w)
						return true
					}
					if mode == "start" && r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/start") {
						f.starts.Add(1)
						drop(w)
						return true
					}
					return false
				}
			}
			out, err := runFastPair(f)
			if err == nil || out.Outcome != collectionUnresolved || out.OutstandingSession != sessionKnownOpen || f.jit.Load() != 1 || f.creates.Load() > 1 || f.starts.Load() > 1 || f.cleanup.Load() != 0 {
				t.Fatal("lost effect was retried or forgotten")
			}
			before := [3]int32{f.jit.Load(), f.creates.Load(), f.starts.Load()}
			_, _ = runFastPair(f)
			if before != ([3]int32{f.jit.Load(), f.creates.Load(), f.starts.Load()}) {
				t.Fatal("second invocation retried unknown effect")
			}
		})
	}
}

func TestPairedKnownCreateThenCanceledRetainsReceipt(t *testing.T) {
	f := newPairedIntegrationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seen := false
	f.c.j.syncFile = func(file *os.File) error {
		if err := file.Sync(); err != nil {
			return err
		}
		raw, _ := os.ReadFile(file.Name())
		lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
		var e Event
		if json.Unmarshal(lines[len(lines)-1], &e) == nil && e.Baseline != nil && e.Baseline.Stage == "handoff" && e.Baseline.Outcome == "result" {
			seen = true
			cancel()
		}
		return nil
	}
	out, err := runPairedBaselineWithCadence(ctx, &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, fastPairCadence())
	if !seen || err == nil || out.Outcome != collectionUnresolved || f.creates.Load() != 1 || f.starts.Load() != 0 {
		t.Fatal("canceled known-create boundary")
	}
	var receipt bool
	for _, e := range f.c.j.Events() {
		if r := e.Baseline; r != nil && r.Stage == "handoff" && r.Outcome == "result" {
			receipt = r.Handoff.Container != nil && r.Handoff.Container.CreateResult.Sequence > 0
		}
	}
	if !receipt {
		t.Fatal("known worker receipt lost")
	}
}

func TestPairedCloseDrainsBothActualJournals(t *testing.T) {
	f := newPairedIntegrationFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.dockerBefore = func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == "POST" && r.URL.Path == "/v1.45/containers/create" {
			once.Do(func() { close(entered) })
			<-release
		}
		return false
	}
	done := make(chan error, 1)
	go func() { _, err := runFastPair(f); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("create not entered")
	}
	cClosed, wClosed := make(chan struct{}), make(chan struct{})
	go func() { _ = f.c.j.Close(); close(cClosed) }()
	go func() { _ = f.wf.Journal.Close(); close(wClosed) }()
	select {
	case <-cClosed:
		t.Fatal("controller lease released during worker effect")
	case <-wClosed:
		t.Fatal("worker lease released during worker effect")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("drained execution failed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("scope failed to drain")
	}
	select {
	case <-cClosed:
	case <-time.After(time.Second):
		t.Fatal("controller close blocked")
	}
	select {
	case <-wClosed:
	case <-time.After(time.Second):
		t.Fatal("worker close blocked")
	}
}
