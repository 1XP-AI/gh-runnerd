//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// Four serial children of the two existing routed parents, not another costly
// root route. Histories are original-context CANCELED evidence only.
type originalContextCommandCase struct{ phase, mode string }

func (c *originalContextCommandCase) acquireAllowed() bool {
	return c.phase == "acquire-loss" && c.mode == "continue"
}
func (c *originalContextCommandCase) closeAllowed() bool {
	return c.phase == "after-ack" && c.mode == "continue"
}
func (c *originalContextCommandCase) expected(h18 []Event) []Event {
	events := append([]Event(nil), h18...)
	add := func(e Event) { e.Sequence = len(events) + 1; events = append(events, e) }
	if c.phase == "after-ack" {
		add(Event{Kind: "barrier", Operation: c.phase})
		if c.mode == "continue" {
			add(Event{Kind: "intent", Operation: "session-close"})
			add(Event{Kind: "result", Operation: "session-close", SessionID: "00000000-0000-4000-8000-000000000001"})
		} else {
			add(Event{Kind: "unknown", Operation: "session-close"})
		}
	} else {
		add(Event{Kind: "intent", Operation: "acquire", RequestIDs: []int64{42}})
		if c.mode == "continue" {
			add(Event{Kind: "response", Operation: "acquire", Succeeded: true})
		}
		add(Event{Kind: "unknown", Operation: "acquire"})
		add(Event{Kind: "unknown", Operation: "probe"})
	}
	return events
}

type originalContextCommandTrace struct {
	err                                      error
	target, acknowledged, released, returned bool
	outcome                                  string
	elapsed                                  time.Duration
	digest                                   string
}

type originalContextParent struct {
	f             *ackCommandFixture
	config        originalContextConfig
	listener      *net.UnixListener
	done          chan originalContextCommandTrace
	joined        chan struct{}
	companionInfo os.FileInfo
	companionRaw  []byte
	started       time.Time
}

func newOriginalContextParent(f *ackCommandFixture) *originalContextParent {
	t := f.t
	t.Helper()
	directory, e := os.MkdirTemp("/tmp", "g01oc-")
	if e != nil {
		t.Fatal("original context owned IPC directory setup failed")
	}
	directory, e = filepath.EvalSymlinks(directory)
	if e != nil {
		t.Fatal("original context canonical IPC directory unavailable")
	}
	identity := func(path string) PreparedIdentity {
		info, e := os.Lstat(path)
		if e != nil {
			t.Fatal("original context owned identity unavailable")
		}
		return preparedIdentity(info)
	}
	dirID := identity(directory)
	socket := filepath.Join(directory, "r")
	listener, e := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if e != nil {
		os.Remove(directory)
		t.Fatal("original context owned Unix listener setup failed")
	}
	listener.SetUnlinkOnClose(false) // an unexpectedly replaced socket is never removed
	if os.Chmod(socket, 0600) != nil {
		listener.Close()
		t.Fatal("original context socket permissions failed")
	}
	socketID := identity(socket)
	target := "ack-result"
	if f.phase == "acquire-loss" {
		target = "acquire-intent"
	}
	c := originalContextConfig{Case: f.phase + "/original-" + f.originalContext.mode + "-after-" + target, Phase: f.phase, Target: target, Mode: f.originalContext.mode, Socket: socket, Directory: dirID, SocketIdentity: socketID, State: identity(f.base.j.directory), Journal: identity(filepath.Join(f.base.j.directory, "journal.jsonl")), Admission: identity(f.admission), Claim: identity(f.base.j.claim.file.Name()), ApprovalSHA256: handoff.Digest(f.raw), HelperSHA256: phaseHelperFileDigest(t, f.controller), EndpointSHA256: handoff.Digest(f.read(filepath.Join(f.base.j.directory, "paired-fixture.json"))), ClaimSHA256: handoff.Digest(f.read(f.base.j.claim.file.Name())), PrefixSHA256: handoff.Digest(f.original)}
	p := &originalContextParent{f: f, config: c, listener: listener, done: make(chan originalContextCommandTrace, 1), joined: make(chan struct{})}
	t.Cleanup(func() {
		listener.Close()
		if originalContextSocket(socket, socketID) {
			if os.Remove(socket) != nil {
				t.Error("original context owned socket cleanup failed")
			}
		} else if _, e := os.Lstat(socket); !os.IsNotExist(e) {
			t.Error("original context socket replaced; preserved")
		}
		if originalContextDirectory(directory, dirID) {
			if os.Remove(directory) != nil {
				t.Error("original context owned IPC directory cleanup failed")
			}
		}
	})
	return p
}

func (p *originalContextParent) start() {
	f, t := p.f, p.f.t
	p.started = time.Now()
	p.config.Deadline = p.started.Add(25 * time.Second)
	raw, e := json.Marshal(p.config)
	if e != nil || len(raw) > 4096 || !p.config.valid() {
		t.Fatal("original context fixed companion configuration invalid")
	}
	path := filepath.Join(f.base.j.directory, originalContextCompanion)
	file, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal("original context companion allocation failed")
	}
	_, e = file.Write(raw)
	if e != nil || file.Sync() != nil {
		file.Close()
		t.Fatal("original context companion persistence failed")
	}
	p.companionInfo, e = file.Stat()
	file.Close()
	if e != nil {
		t.Fatal("original context companion identity unavailable")
	}
	p.companionRaw = raw
	t.Cleanup(p.disarm)
	if p.listener.SetDeadline(p.config.Deadline) != nil {
		t.Fatal("original context listener supervision setup failed")
	}
	go p.coordinate()
	t.Cleanup(func() { p.listener.Close(); <-p.joined })
}

func (p *originalContextParent) coordinate() {
	trace := originalContextCommandTrace{err: ErrApproval}
	defer func() { trace.elapsed = time.Since(p.started); p.done <- trace; close(p.joined) }()
	conn, e := p.listener.AcceptUnix()
	p.listener.Close()
	if e != nil {
		return
	}
	defer conn.Close()
	if conn.SetDeadline(p.config.Deadline) != nil || !originalContextSocket(p.config.Socket, p.config.SocketIdentity) {
		return
	}
	reader := bufio.NewReaderSize(conn, 1025)
	target, e := originalContextRead(reader)
	if e != nil || target.Kind != "TARGET_REACHED" || len(target.Digest) != 64 || target != p.config.frame("TARGET_REACHED", target.Digest, "") {
		return
	}
	f := p.f
	f.mu.Lock()
	f.checkpoint("original-context-target", target.Sequence)
	raw := f.read(filepath.Join(f.base.j.directory, "journal.jsonl"))
	if handoff.Digest(raw) != target.Digest || !originalContextBrokerClaim(f.admission, f.consumption()) || f.counts["DELETE /queue/9"] != 1 || f.counts["POST /tenant/_apis/runtime/runnerscalesets/7/acquirejobs"] != 0 || f.counts["DELETE /tenant/_apis/runtime/runnerscalesets/7/sessions/"+f.sid] != 0 {
		f.t.Error("original context target full-prefix/physical prerequisite differs")
		f.mu.Unlock()
		return
	}
	f.mu.Unlock()
	trace.target = true
	trace.digest = target.Digest
	action, ack := "CONTINUE", "ORIGINAL_ACTIVE"
	if p.config.Mode == "cancel" {
		action, ack = "CANCEL_ORIGINAL", "ORIGINAL_CANCELED"
	}
	if originalContextSend(conn, p.config.frame(action, target.Digest, "")) != nil {
		return
	}
	frame, e := originalContextRead(reader)
	if e != nil || frame != p.config.frame(ack, target.Digest, "") {
		return
	}
	trace.acknowledged = true
	// Independently verify the same durable bytes still held before RELEASE.
	if !bytes.Equal(raw, f.read(filepath.Join(f.base.j.directory, "journal.jsonl"))) {
		return
	}
	if originalContextSend(conn, p.config.frame("RELEASE", target.Digest, "")) != nil {
		return
	}
	trace.released = true
	frame, e = originalContextRead(reader)
	if e != nil || (frame.Outcome != "completed" && frame.Outcome != "refused") || frame != p.config.frame("COMMAND_RETURNED", target.Digest, frame.Outcome) {
		return
	}
	trace.returned = true
	trace.outcome = frame.Outcome
	if originalContextSend(conn, p.config.frame("FINISHED", target.Digest, frame.Outcome)) != nil {
		return
	}
	if conn.CloseWrite() != nil {
		return
	}
	frame, e = originalContextRead(reader)
	if e != nil || frame != p.config.frame("JOINED", target.Digest, trace.outcome) {
		return
	}
	if _, e = originalContextRead(reader); e != io.EOF {
		return
	}
	trace.err = nil
}

// This runs after primary Wait/input writers/IPC/active handlers and assertions,
// before ordinary valid signed guards. Only our captured opt-in is removed.
func (p *originalContextParent) disarm() {
	if removeOriginalContextCompanion(p.f.base.j.directory, p.companionInfo, p.companionRaw) != nil {
		p.f.t.Error("original context captured companion changed; preserved")
	}
}
func removeOriginalContextCompanion(state string, captured os.FileInfo, raw []byte) error {
	path := filepath.Join(state, originalContextCompanion)
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil || captured == nil || !privateFile(info, 0600) || !os.SameFile(info, captured) {
		return ErrApproval
	}
	actual, _, e := originalContextReadPrivate(path, 4096)
	if e != nil || !bytes.Equal(actual, raw) || os.Remove(path) != nil {
		return ErrApproval
	}
	return nil
}

func (f *ackCommandFixture) joinOriginalContextHandlers() {
	f.entryMu.Lock()
	f.closing = true
	f.entryMu.Unlock()
	f.handlers.Wait()
	f.entryMu.Lock()
	f.closing = false
	f.entryMu.Unlock()
}

func (f *ackCommandFixture) assertOriginalContextSettled() []Event {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	runtime := "/tenant/_apis/runtime/runnerscalesets/7"
	acquire, close := 0, 0
	if f.originalContext.acquireAllowed() {
		acquire = 1
	}
	if f.originalContext.closeAllowed() {
		close = 1
	}
	// First report physical mismatches, then exact durable suffix, before output
	// categories or negative guards can mask deliberate propagation mutations.
	if n := f.counts["POST "+runtime+"/acquirejobs"]; n != acquire {
		f.t.Errorf("original context physical acquire witness: actual=%d want=%d", n, acquire)
	}
	if n := f.counts["DELETE "+runtime+"/sessions/"+f.sid]; n != close {
		f.t.Errorf("original context physical close witness: actual=%d want=%d", n, close)
	}
	events := f.base.readEvents()
	want := f.expected()
	if !reflect.DeepEqual(events, want) {
		for i := 0; i < len(events) && i < len(want); i++ {
			if !reflect.DeepEqual(events[i], want[i]) {
				f.t.Errorf("original context durable witness sequence=%d kind=%s operation=%s succeeded=%t wantKind=%s wantOperation=%s", i+1, events[i].Kind, events[i].Operation, events[i].Succeeded, want[i].Kind, want[i].Operation)
				break
			}
		}
		f.t.Errorf("original context full settled events count=%d want=%d", len(events), len(want))
	}
	counts := map[string]int{"GET /installation/repositories": 1, "GET /repos/fixture-org/canary": 1, "GET /orgs/fixture-org/actions/runner-groups/3": 1, "GET /orgs/fixture-org/actions/runner-groups/3/repositories": 1, "POST /api/v3/orgs/fixture-org/actions/runners/registration-token": 1, "POST /api/v3/actions/runner-registration": 1, "GET " + runtime: 1, "POST " + runtime + "/sessions": 1, "GET /queue": 1, "GET " + workflowRunFixturePath(f.transport): 1, "DELETE /queue/9": 1}
	steps := []string{"GET /installation/repositories", "GET /repos/fixture-org/canary", "GET /orgs/fixture-org/actions/runner-groups/3", "GET /orgs/fixture-org/actions/runner-groups/3/repositories", "POST /api/v3/orgs/fixture-org/actions/runners/registration-token", "POST /api/v3/actions/runner-registration", "GET " + runtime, "POST " + runtime + "/sessions", "GET /queue", "GET " + workflowRunFixturePath(f.transport), "DELETE /queue/9"}
	if acquire == 1 {
		key := "POST " + runtime + "/acquirejobs"
		counts[key] = 1
		steps = append(steps, key)
	}
	if close == 1 {
		key := "DELETE " + runtime + "/sessions/" + f.sid
		counts[key] = 1
		steps = append(steps, key)
	}
	if !reflect.DeepEqual(f.counts, counts) || !reflect.DeepEqual(f.steps, steps) {
		f.t.Error("original context complete physical counts/order differ")
	}
	j, e := OpenJournalForPairedFixtureAt(f.base.j.directory, f.base.a, f.admission)
	if e != nil {
		f.t.Error("original context concrete original-approval reopen failed")
		return events
	}
	if !reflect.DeepEqual(j.Events(), events) {
		f.t.Error("original context descriptor/concrete reopen events differ")
	}
	s := replayWithApproval(j.Events(), &f.base.a)
	if j.Close() != nil {
		f.t.Error("original context concrete reopen close failed")
	}
	sid := f.sid
	if close == 1 {
		sid = ""
	}
	if s.setID != 7 || s.deleted || !reflect.DeepEqual(s.phaseSeen, map[string]bool{"create": true, f.phase: true}) || !reflect.DeepEqual(s.observedJobs, map[int64]bool{42: true}) || !s.workObserved || !s.reserved || s.uncertain != (close == 0) || s.sessionID != sid {
		f.t.Error("original context concrete retained reservation/session/replay differs")
	}
	f.checkpoint("original-context-settled", len(want))
	return events
}

func runOriginalContextCommandCase(t *testing.T, controller, broker, phase, mode string) {
	f := newACKCommandFixture(t, controller, broker, phase, "job9")
	f.originalContext = &originalContextCommandCase{phase, mode}
	journal := filepath.Join(f.base.j.directory, "journal.jsonl")
	f.base.readEvents = func() []Event {
		raw := f.read(journal)
		events, e := originalContextEvents(raw, f.base.a)
		if e != nil {
			t.Error("original context strict original-authority bytes differ")
		}
		return events
	}
	p := newOriginalContextParent(f)
	p.start()
	var events []Event
	f.afterCommand = func() {
		trace := <-p.done
		f.joinOriginalContextHandlers()
		f.scanCommandJournals()
		if trace.err != nil || !trace.target || !trace.acknowledged || !trace.released || !trace.returned {
			t.Error("original context SETUP: target/ack/release/finish/join missing")
			return
		}
		events = f.assertSettled()
		f.assertBroker()
		want := "refused"
		if f.originalContext.closeAllowed() {
			want = "completed"
		}
		if trace.outcome != want {
			t.Error("original context actual receiver category differs")
		}
		t.Logf("original context target=%s ack=%s release=true command-return=%s joined=true prefix=%s elapsed=%s", p.config.Target, mode, trace.outcome, trace.digest, trace.elapsed)
	}
	f.runOutcome(f.broker, f.brokerArgs(f.attempt), f.originalContext.closeAllowed(), f.management, []byte("synthetic-observation-credential"), f.workflowSeed, f.issuerSeed)
	f.afterCommand = nil
	f.scanCommandJournals()
	if t.Failed() {
		return
	}
	p.disarm()
	if _, e := os.Lstat(filepath.Join(f.base.j.directory, originalContextCompanion)); !os.IsNotExist(e) {
		t.Fatal("original context guard opt-in absence unproven")
	}
	f.guardControlsForPhase(events, phase)
	f.alternatePreparation()
	f.joinOriginalContextHandlers()
	f.assertSettled()
	f.scanCommandJournals()
}

// Finite protocol/setup negatives live under the existing parents' fixture
// helper checks rather than another routed actual-command parent.
func originalContextProtocolNegatives(t *testing.T) {
	valid, _ := json.Marshal(originalContextFrame{"CONTINUE", "after-ack/original-continue-after-ack-result", "after-ack", "ack-result", 18, strings.Repeat("a", 64), ""})
	for _, raw := range [][]byte{append(bytes.Clone(valid), []byte(" {}")...), []byte(`{"kind":"CONTINUE","kind":"RELEASE"}`), []byte(`{"kind":null}`), []byte(`{"unknown":true}`), []byte("null"), []byte(strings.Repeat("x", 1025) + "\n"), []byte("{}")} {
		var frame originalContextFrame
		if len(raw) <= 1024 && originalContextDecode(raw, &frame) == nil {
			t.Error("original context malformed frame accepted")
		}
		if _, e := originalContextRead(bufio.NewReaderSize(bytes.NewReader(append(bytes.Clone(raw), '\n')), 1025)); e == nil {
			t.Error("original context malformed framed input accepted")
		}
	}
}

// These are harness refusal/cleanup checks, never behavioral RED specimens.
func originalContextSetupNegatives(t *testing.T) {
	t.Run("companion-and-captured-cleanup", func(t *testing.T) {
		state := t.TempDir()
		path := filepath.Join(state, originalContextCompanion)
		if os.WriteFile(path, []byte("{}"), 0600) != nil {
			t.Fatal("setup file")
		}
		captured, _ := os.Lstat(path)
		if os.WriteFile(path, []byte("changed"), 0600) != nil {
			t.Fatal("setup write")
		}
		if removeOriginalContextCompanion(state, captured, []byte("{}")) == nil {
			t.Error("changed companion removed")
		}
		if _, e := os.Lstat(path); e != nil {
			t.Fatal("changed companion not preserved")
		}
		old := path + ".old"
		if os.Rename(path, old) != nil || os.WriteFile(path, []byte("{}"), 0600) != nil {
			t.Fatal("setup replacement")
		}
		if removeOriginalContextCompanion(state, captured, []byte("{}")) == nil {
			t.Error("replaced companion removed")
		}
		for _, mutation := range []string{"symlink", "hardlink", "mode", "oversize", "malformed", "stale"} {
			t.Run(mutation, func(t *testing.T) {
				os.Remove(path)
				switch mutation {
				case "symlink":
					if os.Symlink(old, path) != nil {
						t.Fatal("setup symlink")
					}
				case "hardlink":
					if os.Link(old, path) != nil {
						t.Fatal("setup hardlink")
					}
				case "mode":
					os.WriteFile(path, []byte("{}"), 0644)
				case "oversize":
					os.WriteFile(path, bytes.Repeat([]byte("x"), 4097), 0600)
				case "malformed":
					os.WriteFile(path, []byte(`{"case":null}`), 0600)
				case "stale":
					os.WriteFile(path, []byte(`{}`), 0600)
				}
				if _, owner, e := BeginOriginalContextCommandForFixture(state, "unread", "controller", "after-ack"); e == nil || owner != nil {
					t.Error("present invalid companion silently accepted")
				}
			})
		}
		os.Remove(path)
		if ctx, owner, e := BeginOriginalContextCommandForFixture(state, "unread", "controller", "after-ack"); e != nil || ctx != nil || owner != nil {
			t.Error("absent opt-in changed ordinary dispatch")
		}
		if os.WriteFile(path, []byte("{}"), 0600) != nil {
			t.Fatal("setup valid file")
		}
		captured, _ = os.Lstat(path)
		if removeOriginalContextCompanion(state, captured, []byte("{}")) != nil {
			t.Error("captured owned cleanup refused")
		}
	})
	t.Run("socket-replacement-preserved", func(t *testing.T) {
		directory, e := os.MkdirTemp("/tmp", "g01oc-")
		if e != nil {
			t.Fatal("setup directory")
		}
		directory, _ = filepath.EvalSymlinks(directory)
		defer os.Remove(directory)
		path := filepath.Join(directory, "r")
		old, e := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if e != nil {
			t.Fatal("setup socket")
		}
		old.SetUnlinkOnClose(false)
		os.Chmod(path, 0600)
		info, _ := os.Lstat(path)
		identity := preparedIdentity(info)
		if !originalContextSocket(path, identity) {
			t.Error("owned socket rejected")
		}
		dirInfo, _ := os.Lstat(directory)
		good := originalContextConfig{Case: "after-ack/original-continue-after-ack-result", Phase: "after-ack", Target: "ack-result", Mode: "continue", Socket: path, Directory: preparedIdentity(dirInfo), SocketIdentity: identity, Deadline: time.Now().Add(10 * time.Second)}
		if !good.valid() {
			t.Fatal("valid fixed selection rejected")
		}
		for _, change := range []string{"case", "phase", "target", "mode", "past", "unbounded"} {
			bad := good
			switch change {
			case "case":
				bad.Case = "foreign"
			case "phase":
				bad.Phase = "inspect"
			case "target":
				bad.Target = "close-intent"
			case "mode":
				bad.Mode = "retry"
			case "past":
				bad.Deadline = time.Now().Add(-time.Second)
			case "unbounded":
				bad.Deadline = time.Now().Add(time.Minute)
			}
			if bad.valid() {
				t.Error("invalid selection accepted")
			}
		}
		os.Chmod(path, 0666)
		if originalContextSocket(path, identity) {
			t.Error("permissive socket accepted")
		}
		os.Chmod(path, 0600)
		os.Chmod(directory, 0770)
		if good.valid() {
			t.Error("permissive IPC directory accepted")
		}
		os.Chmod(directory, 0700)

		if os.Remove(path) != nil {
			t.Fatal("setup socket replace")
		}
		replacement, e := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if e != nil {
			t.Fatal("setup replacement socket")
		}
		replacement.SetUnlinkOnClose(false)
		defer replacement.Close()
		defer os.Remove(path)
		os.Chmod(path, 0600)
		if originalContextSocket(path, identity) {
			t.Error("replaced socket identity accepted")
		}
		old.Close()
		if _, e := os.Lstat(path); e != nil {
			t.Error("closing old listener unlinked replacement")
		}
	})
	for _, failure := range []string{"EOF", "target-missing", "early-finish", "wrong-action", "duplicate-action", "bad-release", "trailing-finish", "continue-canceled"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			receiver, peer := net.Pipe()
			defer peer.Close()
			deadline := time.Now().Add(time.Second)
			if failure == "target-missing" {
				deadline = time.Now().Add(10 * time.Millisecond)
			}
			receiver.SetDeadline(deadline)
			peer.SetDeadline(deadline)
			c := originalContextConfig{Case: "after-ack/original-continue-after-ack-result", Phase: "after-ack", Target: "ack-result", Mode: "continue", Deadline: deadline}
			owner := &OriginalContextCommandFixture{ctx: ctx, cancel: cancel, config: c, conn: receiver, previous: controllerExperimentFixtureOpen, target: make(chan string, 1), release: make(chan error, 1), finish: make(chan originalContextFinish, 1), done: make(chan struct{}), readerDone: make(chan struct{}), stop: make(chan struct{}), once: sync.Once{}}
			go owner.coordinate()
			if failure == "EOF" {
				peer.Close()
			} else if failure == "early-finish" {
				owner.finish <- originalContextFinish{"refused", make(chan error, 1)}
			} else if failure != "target-missing" {
				digest := strings.Repeat("a", 64)
				owner.target <- digest
				reader := bufio.NewReaderSize(peer, 1025)
				if _, e := originalContextRead(reader); e != nil {
					t.Fatal("setup target read")
				}
				action := "CONTINUE"
				if failure == "wrong-action" {
					action = "CANCEL_ORIGINAL"
				}
				if failure == "continue-canceled" {
					cancel()
				}
				_ = originalContextSend(peer, c.frame(action, digest, ""))
				if failure != "wrong-action" && failure != "continue-canceled" {
					if _, e := originalContextRead(reader); e != nil {
						t.Fatal("setup active acknowledgment")
					}
					release := "RELEASE"
					if failure == "duplicate-action" {
						release = "CONTINUE"
					}
					if failure == "bad-release" {
						release = "FINISHED"
					}
					_ = originalContextSend(peer, c.frame(release, digest, ""))
					if failure == "trailing-finish" {
						if e := <-owner.release; e != nil {
							t.Fatal("setup release")
						}
						owner.finish <- originalContextFinish{"completed", make(chan error, 1)}
						if _, e := originalContextRead(reader); e != nil {
							t.Fatal("setup finish read")
						}
						_ = originalContextSend(peer, c.frame("FINISHED", digest, "completed"))
						_ = originalContextSend(peer, c.frame("CONTINUE", digest, ""))
					}
				}
			}
			<-owner.done
			if owner.err == nil || ctx.Err() != context.Canceled {
				t.Error("protocol failure accepted as specimen outcome")
			}
			owner.Close()
		})
	}
	t.Run("real-sync-failure", func(t *testing.T) {
		file, e := os.CreateTemp(t.TempDir(), "sync")
		if e != nil {
			t.Fatal("setup file")
		}
		file.Close()
		owner := OriginalContextCommandFixture{}
		if owner.sync(file) == nil {
			t.Error("failed real Sync accepted")
		}
	})
}
