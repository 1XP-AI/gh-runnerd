//go:build g01_pair_fixture

package livecanary

// This concrete, one-shot rendezvous belongs only to the refused TEST build.
// Its filesystem identities correlate approved fixture processes; they do not
// isolate hostile processes running as the same user.
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
	"syscall"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

const originalContextCompanion = "original-context-fixture.json"

type originalContextConfig struct {
	Case           string           `json:"case"`
	Phase          string           `json:"phase"`
	Target         string           `json:"target"`
	Mode           string           `json:"mode"`
	Socket         string           `json:"socket"`
	Deadline       time.Time        `json:"deadline"`
	Directory      PreparedIdentity `json:"directory"`
	SocketIdentity PreparedIdentity `json:"socket_identity"`
	State          PreparedIdentity `json:"state"`
	Journal        PreparedIdentity `json:"journal"`
	Admission      PreparedIdentity `json:"admission"`
	Claim          PreparedIdentity `json:"claim"`
	ApprovalSHA256 string           `json:"approval_sha256"`
	HelperSHA256   string           `json:"helper_sha256"`
	EndpointSHA256 string           `json:"endpoint_sha256"`
	ClaimSHA256    string           `json:"claim_sha256"`
	PrefixSHA256   string           `json:"prefix_sha256"`
}

type originalContextFrame struct {
	Kind     string `json:"kind"`
	Case     string `json:"case"`
	Phase    string `json:"phase"`
	Target   string `json:"target"`
	Sequence int    `json:"sequence"`
	Digest   string `json:"digest"`
	Outcome  string `json:"outcome"`
}

// Reject null at every depth as well as duplicate, unknown and trailing data.
func originalContextDecode(raw []byte, dst any) error {
	var value any
	if DecodeStrict(raw, &value) != nil {
		return ErrApproval
	}
	var nonnull func(any) bool
	nonnull = func(v any) bool {
		switch x := v.(type) {
		case nil:
			return false
		case map[string]any:
			for _, e := range x {
				if !nonnull(e) {
					return false
				}
			}
		case []any:
			for _, e := range x {
				if !nonnull(e) {
					return false
				}
			}
		}
		return true
	}
	if !nonnull(value) || DecodeStrict(raw, dst) != nil {
		return ErrApproval
	}
	typ := reflect.TypeOf(dst).Elem()
	if typ.Kind() == reflect.Struct {
		obj, ok := value.(map[string]any)
		if !ok || len(obj) != typ.NumField() {
			return ErrApproval
		}
		for n := 0; n < typ.NumField(); n++ {
			key := strings.Split(typ.Field(n).Tag.Get("json"), ",")[0]
			if _, ok := obj[key]; !ok {
				return ErrApproval
			}
		}
	}
	return nil
}

func originalContextReadPrivate(path string, limit int64) ([]byte, os.FileInfo, error) {
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, nil, ErrApproval
	}
	defer f.Close()
	i, e := f.Stat()
	if e != nil || !privateFile(i, 0600) || i.Size() <= 0 || i.Size() > limit {
		return nil, nil, ErrApproval
	}
	raw, e := io.ReadAll(io.LimitReader(f, limit+1))
	after, ae := os.Lstat(path)
	if e != nil || ae != nil || !os.SameFile(i, after) || int64(len(raw)) != i.Size() {
		return nil, nil, ErrApproval
	}
	return raw, i, nil
}

func originalContextDirectory(path string, id PreparedIdentity) bool {
	real, e := filepath.EvalSymlinks(path)
	i, se := os.Lstat(path)
	return e == nil && se == nil && real == path && privateStateDirectory(path) && preparedIdentity(i) == id
}

func originalContextSocket(path string, id PreparedIdentity) bool {
	i, e := os.Lstat(path)
	if e != nil || i.Mode()&os.ModeSocket == 0 || i.Mode().Perm() != 0600 {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && int(s.Uid) == os.Geteuid() && s.Nlink == 1 && preparedIdentity(i) == id
}

func (c originalContextConfig) valid() bool {
	phaseTarget := c.Phase == "after-ack" && c.Target == "ack-result" || c.Phase == "acquire-loss" && c.Target == "acquire-intent"
	return phaseTarget && (c.Mode == "cancel" || c.Mode == "continue") && c.Case == c.Phase+"/original-"+c.Mode+"-after-"+c.Target && filepath.IsAbs(c.Socket) && filepath.Clean(c.Socket) == c.Socket && filepath.Base(c.Socket) == "r" && len(c.Socket) < 100 && c.Deadline.After(time.Now()) && !c.Deadline.After(time.Now().Add(25*time.Second)) && originalContextDirectory(filepath.Dir(c.Socket), c.Directory) && originalContextSocket(c.Socket, c.SocketIdentity)
}

func (c originalContextConfig) frame(kind, digest, outcome string) originalContextFrame {
	n := 18
	if c.Target == "acquire-intent" {
		n = 19
	}
	return originalContextFrame{kind, c.Case, c.Phase, c.Target, n, digest, outcome}
}

func originalContextSend(conn net.Conn, f originalContextFrame) error {
	raw, e := json.Marshal(f)
	if e != nil || len(raw)+1 > 1024 {
		return ErrApproval
	}
	n, e := conn.Write(append(raw, '\n'))
	if e != nil || n != len(raw)+1 {
		return ErrApproval
	}
	return nil
}

func originalContextRead(reader *bufio.Reader) (originalContextFrame, error) {
	var f originalContextFrame
	raw, e := reader.ReadSlice('\n')
	if e == io.EOF && len(raw) == 0 {
		return f, io.EOF
	}
	if e != nil || len(raw) > 1024 || originalContextDecode(raw, &f) != nil {
		return f, ErrApproval
	}
	return f, nil
}

type originalContextReadResult struct {
	frame originalContextFrame
	err   error
}
type originalContextFinish struct {
	outcome string
	reply   chan error
}

// OriginalContextCommandFixture owns the one C0, one socket reader and the
// concrete journal opener. Neither cancellation nor protocol messages confer
// authority or replace the broker's receipt, binding, payload or claim.
type OriginalContextCommandFixture struct {
	ctx        context.Context
	cancel     context.CancelFunc
	config     originalContextConfig
	admission  string
	approval   Approval
	prefix     []byte
	seed       []Event
	conn       net.Conn
	previous   func(string, Approval) (*FileJournal, error)
	target     chan string
	release    chan error
	finish     chan originalContextFinish
	done       chan struct{}
	readerDone chan struct{}
	stop       chan struct{}
	once       sync.Once
	err        error // published by done
	armed      bool  // journal append mutex owns this field
}

// BeginOriginalContextCommandForFixture selects only a present, strictly valid
// companion. Absence retains the ordinary dispatcher; malformed presence fails.
// The existing concrete opener is captured, never supplied by the caller.
func BeginOriginalContextCommandForFixture(state, approvalPath, mode, phase string) (context.Context, *OriginalContextCommandFixture, error) {
	path := filepath.Join(state, originalContextCompanion)
	if _, e := os.Lstat(path); os.IsNotExist(e) {
		return nil, nil, nil
	}
	raw, _, e := originalContextReadPrivate(path, 4096)
	var c originalContextConfig
	if e != nil || originalContextDecode(raw, &c) != nil || !c.valid() || c.Phase != phase || mode != "controller" || controllerExperimentFixtureOpen == nil || !originalContextDirectory(state, c.State) {
		return nil, nil, ErrApproval
	}
	a, e := ReadApproval(approvalPath)
	approvalRaw, _, ae := originalContextReadPrivate(approvalPath, 16384)
	endpoint, _, ee := originalContextReadPrivate(filepath.Join(state, "paired-fixture.json"), 8192)
	var endpointConfig struct {
		WorkerAdmissionDirectory string `json:"worker_admission_directory,omitempty"`
		ExperimentalPolicy       string `json:"experimental_policy,omitempty"`
		HarnessSHA               string `json:"harness_sha,omitempty"`
		BaseURL                  string `json:"base_url"`
		CAPEM                    string `json:"ca_pem"`
		AdmissionDirectory       string `json:"admission_directory"`
	}
	executable, xe := os.Executable()
	helper, he := os.ReadFile(executable)
	prefix, ji, je := originalContextReadPrivate(filepath.Join(state, "journal.jsonl"), 1<<20)
	if e != nil || ae != nil || ee != nil || xe != nil || he != nil || je != nil || handoff.Digest(approvalRaw) != c.ApprovalSHA256 || handoff.Digest(helper) != c.HelperSHA256 || handoff.Digest(endpoint) != c.EndpointSHA256 || preparedIdentity(ji) != c.Journal || handoff.Digest(prefix) != c.PrefixSHA256 || DecodeStrict(endpoint, &endpointConfig) != nil || !originalContextDirectory(endpointConfig.AdmissionDirectory, c.Admission) {
		return nil, nil, ErrApproval
	}
	// The existing state claim is private and immutable; the separate broker
	// ledger gains its actual receipt provenance only after real issuance.
	seed, e := originalContextEvents(prefix, a)
	if e != nil || len(seed) != 7 || seed[0].ControllerHandoff == nil || seed[0].ControllerHandoff.Phase != "create" || !reflect.DeepEqual(seed[6], Event{Sequence: 7, Kind: "result", Operation: "create", ID: 7}) {
		return nil, nil, ErrApproval
	}
	ctx, cancel := context.WithCancel(context.Background())
	conn, e := (&net.Dialer{Deadline: c.Deadline}).DialContext(context.Background(), "unix", c.Socket)
	if e != nil {
		cancel()
		return nil, nil, ErrApproval
	}
	if !originalContextSocket(c.Socket, c.SocketIdentity) || conn.SetDeadline(c.Deadline) != nil {
		conn.Close()
		cancel()
		return nil, nil, ErrApproval
	}
	f := &OriginalContextCommandFixture{ctx: ctx, cancel: cancel, config: c, admission: endpointConfig.AdmissionDirectory, approval: a, prefix: prefix, seed: seed, conn: conn, previous: controllerExperimentFixtureOpen, target: make(chan string, 1), release: make(chan error, 1), finish: make(chan originalContextFinish, 1), done: make(chan struct{}), readerDone: make(chan struct{}), stop: make(chan struct{})}
	controllerExperimentFixtureOpen = func(path string, approval Approval) (*FileJournal, error) {
		if path != state || approvalDigest(approval) != approvalDigest(a) {
			return nil, ErrApproval
		}
		j, e := f.previous(path, approval)
		if e != nil {
			return nil, e
		}
		claim, ce := os.ReadFile(j.claim.file.Name())
		if j.syncFile != nil || preparedIdentity(j.fileInfo) != c.Journal || preparedIdentity(j.claim.fileInfo) != c.Claim || ce != nil || handoff.Digest(claim) != c.ClaimSHA256 {
			j.Close()
			return nil, ErrApproval
		}
		j.syncFile = f.sync
		return j, nil
	}
	go f.coordinate()
	return ctx, f, nil
}

// Inspect only descriptor bytes. Calling Events/authorize/Close here deadlocks:
// append holds j.mu and has not yet published this successfully synced line.
func (f *OriginalContextCommandFixture) sync(file *os.File) error {
	if file.Sync() != nil {
		return ErrJournal
	}
	i, e := file.Stat()
	if e != nil || !privateFile(i, 0600) || preparedIdentity(i) != f.config.Journal || i.Size() > 1<<20 {
		return ErrJournal
	}
	raw, e := io.ReadAll(io.NewSectionReader(file, 0, i.Size()))
	if e != nil || !bytes.HasPrefix(raw, f.prefix) {
		return ErrJournal
	}
	events, e := originalContextEvents(raw, f.approval)
	if e != nil || len(events) < 8 || !originalContextBrokerClaim(f.admission, events[7].ControllerHandoff) {
		return ErrJournal
	}
	n := f.config.frame("", "", "").Sequence
	// Later records are asserted by the independent parent and postexit reopen.
	if len(events) > n {
		return nil
	}
	want, e := originalContextPrefix(events, f.seed, f.config.Phase, f.config.ApprovalSHA256)
	if e != nil || len(events) > len(want) || !reflect.DeepEqual(events, want[:len(events)]) {
		return ErrJournal
	}
	if len(events) != n {
		return nil
	}
	if f.armed {
		return ErrJournal
	}
	f.armed = true
	select {
	case f.target <- handoff.Digest(raw):
	case <-f.done:
		return ErrJournal
	}
	select {
	case e := <-f.release:
		return e
	case <-f.done:
		return ErrJournal
	}
}

func originalContextEvents(raw []byte, a Approval) ([]Event, error) {
	if len(raw) == 0 || len(raw) > 1<<20 || raw[len(raw)-1] != '\n' {
		return nil, ErrJournal
	}
	lines := bytes.Split(raw[:len(raw)-1], []byte{'\n'})
	var h journalHeader
	if DecodeStrict(lines[0], &h) != nil || h.Version != 1 || h.Ownership != ownershipDigest(a) || h.Authority.Digest != approvalDigest(a) || !h.Authority.ExpiresAt.Equal(a.ExpiresAt) || !reflect.DeepEqual(h.Authority.Phases, a.Phases) {
		return nil, ErrJournal
	}
	var events []Event
	for _, line := range lines[1:] {
		var event Event
		if DecodeStrict(line, &event) != nil || event.Sequence != len(events)+1 || !validEvent(event) || event.Kind == "authority" {
			return nil, ErrJournal
		}
		events = append(events, event)
	}
	if validateControllerHandoffReplay(events) != nil {
		return nil, ErrJournal
	}
	return events, nil
}

// Consumption comes from the actual synced event, then must match the actual
// broker claim. No receipt or nonce is pre-minted to fill the companion.
func originalContextPrefix(events, seed []Event, phase, rawDigest string) ([]Event, error) {
	if len(events) < 8 || events[7].ControllerHandoff == nil {
		return nil, ErrJournal
	}
	c := events[7].ControllerHandoff
	if c.Phase != phase || c.ApprovalSHA256 != rawDigest || c.KeyID != "fixture-ed25519-v1" || len(c.ReceiptSHA256) != 64 || len(c.ReceiptNonce) != 32 {
		return nil, ErrJournal
	}
	sid := "00000000-0000-4000-8000-000000000001"
	want := append([]Event(nil), seed...)
	add := func(e Event) { e.Sequence = len(want) + 1; want = append(want, e) }
	for _, e := range []Event{{Kind: "controller-handoff-consumed", ControllerHandoff: c}, {Kind: "phase", Operation: phase}, {Kind: "intent", Operation: "observe-owned"}, {Kind: "result", Operation: "observe-owned"}, {Kind: "intent", Operation: "session-open"}, {Kind: "result", Operation: "session-open", SessionID: sid}, {Kind: "intent", Operation: "observe-poll"}, {Kind: "result", Operation: "observe-poll", Work: workDemand}, {Kind: "observation", Operation: "poll", ID: 9, SessionID: sid, RequestIDs: []int64{42}}, {Kind: "intent", Operation: "ack"}, {Kind: "result", Operation: "ack", ID: 9, SessionID: sid}} {
		add(e)
	}
	if phase == "acquire-loss" {
		add(Event{Kind: "intent", Operation: "acquire", RequestIDs: []int64{42}})
	}
	return want, nil
}

func (f *OriginalContextCommandFixture) coordinate() {
	defer close(f.done)
	defer f.conn.Close()
	defer f.cancel()
	reads := make(chan originalContextReadResult, 1)
	go func() {
		defer close(f.readerDone)
		reader := bufio.NewReaderSize(f.conn, 1025)
		for n := 0; n < 4; n++ {
			frame, e := originalContextRead(reader)
			select {
			case reads <- originalContextReadResult{frame, e}:
			case <-f.stop:
				return
			}
			if e != nil {
				return
			}
		}
	}()
	defer func() { close(f.stop); f.conn.Close(); <-f.readerDone }()
	timer := time.NewTimer(time.Until(f.config.Deadline))
	defer timer.Stop()
	fail := func() {
		f.err = ErrApproval
		select {
		case f.release <- ErrJournal:
		default:
		}
	}
	var digest string
	select {
	case digest = <-f.target:
	case <-reads:
		fail()
		return
	case <-f.finish:
		fail()
		return
	case <-timer.C:
		fail()
		return
	}
	base := func(kind, outcome string) originalContextFrame { return f.config.frame(kind, digest, outcome) }
	if originalContextSend(f.conn, base("TARGET_REACHED", "")) != nil {
		fail()
		return
	}
	receive := func(want originalContextFrame) bool {
		select {
		case r := <-reads:
			return r.err == nil && r.frame == want
		case <-timer.C:
			return false
		case <-f.finish:
			return false
		}
	}
	action, ack := "CONTINUE", "ORIGINAL_ACTIVE"
	if f.config.Mode == "cancel" {
		action, ack = "CANCEL_ORIGINAL", "ORIGINAL_CANCELED"
	}
	if !receive(base(action, "")) || f.ctx.Err() != nil {
		fail()
		return
	}
	if f.config.Mode == "cancel" {
		f.cancel()
		if f.ctx.Err() != context.Canceled {
			fail()
			return
		}
		select {
		case <-f.ctx.Done():
		default:
			fail()
			return
		}
	}
	if originalContextSend(f.conn, base(ack, "")) != nil || !receive(base("RELEASE", "")) {
		fail()
		return
	}
	f.release <- nil // C0 cancellation must never abort a valid Sync/release.
	var finish originalContextFinish
	select {
	case finish = <-f.finish:
	case <-reads:
		fail()
		return
	case <-timer.C:
		fail()
		return
	}
	if originalContextSend(f.conn, base("COMMAND_RETURNED", finish.outcome)) != nil || !receive(base("FINISHED", finish.outcome)) {
		fail()
		finish.reply <- ErrApproval
		return
	}
	// The parent closes only its write half after FINISHED. Consume exact EOF
	// to reject trailing frames before acknowledging the joined reader.
	select {
	case r := <-reads:
		if r.err != io.EOF {
			fail()
			finish.reply <- ErrApproval
			return
		}
	case <-timer.C:
		fail()
		finish.reply <- ErrApproval
		return
	}
	<-f.readerDone
	if originalContextSend(f.conn, base("JOINED", finish.outcome)) != nil {
		fail()
		finish.reply <- ErrApproval
		return
	}
	finish.reply <- nil
}

// Finish reports the actual sanitized category, even when a deliberate dispatch
// detachment succeeds. Physical/durable assertions in the parent expose it.
func (f *OriginalContextCommandFixture) Finish(code int, output []byte) error {
	outcome := ""
	if code == 0 && string(output) == "experimental phase completed; inspect private evidence\n" {
		outcome = "completed"
	}
	if code == 1 && string(output) == "canary refused; approval, authority or private state requires review\n" {
		outcome = "refused"
	}
	if outcome == "" || len(output) > 256 {
		f.Close()
		return ErrApproval
	}
	reply := make(chan error, 1)
	select {
	case f.finish <- originalContextFinish{outcome, reply}:
	case <-f.done:
		f.Close()
		return ErrApproval
	}
	select {
	case <-reply:
	case <-f.done:
	}
	<-f.done
	f.Close()
	return f.err
}

func (f *OriginalContextCommandFixture) Close() {
	f.once.Do(func() { f.conn.Close(); f.cancel(); <-f.done; controllerExperimentFixtureOpen = f.previous })
}

// Avoid accidental raw output retention if a future command writes more than
// the fixed outcome. MultiWriter still preserves the ordinary stdout path.
type OriginalContextFixtureOutput struct {
	data     []byte
	overflow bool
}

func (o *OriginalContextFixtureOutput) Write(p []byte) (int, error) {
	if len(o.data)+len(p) <= 256 {
		o.data = append(o.data, p...)
	} else {
		o.overflow = true
	}
	return len(p), nil
}
func (o *OriginalContextFixtureOutput) Bytes() []byte {
	if o.overflow {
		return nil
	}
	return o.data
}

func originalContextBrokerClaim(admission string, c *controllerHandoffConsumption) bool {
	if c == nil {
		return false
	}
	raw, _, e := originalContextReadPrivate(filepath.Join(admission, "broker-admission.jsonl"), 512<<10)
	if e != nil || raw[len(raw)-1] != '\n' {
		return false
	}
	count := 0
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		var event map[string]json.RawMessage
		if originalContextDecode(line, &event) != nil {
			return false
		}
		var kind, slot string
		if json.Unmarshal(event["kind"], &kind) != nil {
			continue
		}
		_ = json.Unmarshal(event["slot"], &slot)
		if kind != "claim" || slot != c.Phase {
			continue
		}
		var provenance struct {
			Digest string `json:"digest"`
			Nonce  string `json:"receipt_nonce"`
			Source string `json:"source"`
		}
		if originalContextDecode(event["provenance"], &provenance) != nil || provenance.Digest != c.ReceiptSHA256 || provenance.Nonce != c.ReceiptNonce || provenance.Source != "github-rest-workflow-v1" {
			return false
		}
		count++
	}
	return count == 1
}
