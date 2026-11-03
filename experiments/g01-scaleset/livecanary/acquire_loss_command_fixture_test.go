//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
	"github.com/actions/scaleset"
)

const acquireLossPhase = "acquire-loss"

func workflowProvenancePairMatchesClaim(startedRaw, confirmedRaw, claimRaw []byte) bool {
	type inode struct {
		Device uint64 `json:"device"`
		Inode  uint64 `json:"inode"`
	}
	type workflowEvent struct {
		Kind                     string `json:"kind"`
		Slot                     string `json:"slot"`
		Attempt                  inode  `json:"attempt"`
		Journal                  inode  `json:"journal"`
		ControllerApprovalSHA256 string `json:"controller_approval_sha256"`
		Source                   string `json:"source"`
		WorkflowRunID            int64  `json:"workflow_run_id"`
		WorkflowRef              string `json:"workflow_ref"`
		WorkflowSHA              string `json:"workflow_sha"`
		WorkflowPath             string `json:"workflow_path"`
	}
	type claimEvent struct {
		Kind           string `json:"kind"`
		Slot           string `json:"slot"`
		Attempt        inode  `json:"attempt"`
		Journal        inode  `json:"journal"`
		SnapshotDigest string `json:"snapshot_digest"`
		Provenance     struct {
			Source string `json:"source"`
		} `json:"provenance"`
		Authority struct {
			Approval struct {
				WorkflowRunID int64  `json:"workflow_run_id"`
				WorkflowRef   string `json:"workflow_ref"`
				WorkflowSHA   string `json:"workflow_sha"`
				WorkflowPath  string `json:"workflow_path"`
			} `json:"approval"`
		} `json:"authority"`
	}
	decode := func(raw []byte, target any) bool {
		var fields map[string]json.RawMessage
		return DecodeStrict(raw, &fields) == nil && json.Unmarshal(raw, target) == nil
	}
	var started, confirmed workflowEvent
	var claim claimEvent
	if !decode(startedRaw, &started) || !decode(confirmedRaw, &confirmed) || !decode(claimRaw, &claim) ||
		started.Kind != "workflow_provenance_get_started" || confirmed.Kind != "workflow_provenance_get_confirmed" || claim.Kind != "claim" ||
		claim.Slot == "" || claim.Attempt.Device == 0 || claim.Attempt.Inode == 0 || claim.Journal.Device == 0 || claim.Journal.Inode == 0 ||
		claim.SnapshotDigest == "" || claim.Provenance.Source == "" || claim.Authority.Approval.WorkflowRunID <= 0 ||
		claim.Authority.Approval.WorkflowRef == "" || claim.Authority.Approval.WorkflowSHA == "" || claim.Authority.Approval.WorkflowPath == "" {
		return false
	}
	confirmed.Kind = ""
	started.Kind = ""
	return started == confirmed && started.Slot == claim.Slot && started.Attempt == claim.Attempt && started.Journal == claim.Journal &&
		started.ControllerApprovalSHA256 == claim.SnapshotDigest && started.Source == claim.Provenance.Source &&
		started.WorkflowRunID == claim.Authority.Approval.WorkflowRunID && started.WorkflowRef == claim.Authority.Approval.WorkflowRef &&
		started.WorkflowSHA == claim.Authority.Approval.WorkflowSHA && started.WorkflowPath == claim.Authority.Approval.WorkflowPath
}

func TestWorkflowProvenancePairMatchesExactClaim(t *testing.T) {
	event := func(kind string) map[string]any {
		return map[string]any{
			"kind": kind, "slot": "after-ack",
			"attempt":                    map[string]any{"device": uint64(1), "inode": uint64(2)},
			"journal":                    map[string]any{"device": uint64(3), "inode": uint64(4)},
			"controller_approval_sha256": strings.Repeat("a", 64),
			"source":                     "signed-fixture-v1",
			"workflow_run_id":            int64(7),
			"workflow_ref":               "refs/heads/main",
			"workflow_sha":               strings.Repeat("b", 40),
			"workflow_path":              ".github/workflows/canary.yml",
		}
	}
	claim := map[string]any{
		"kind": "claim", "slot": "after-ack",
		"attempt":         map[string]any{"device": uint64(1), "inode": uint64(2)},
		"journal":         map[string]any{"device": uint64(3), "inode": uint64(4)},
		"snapshot_digest": strings.Repeat("a", 64),
		"provenance":      map[string]any{"source": "signed-fixture-v1"},
		"authority": map[string]any{"approval": map[string]any{
			"workflow_run_id": int64(7), "workflow_ref": "refs/heads/main",
			"workflow_sha": strings.Repeat("b", 40), "workflow_path": ".github/workflows/canary.yml",
		}},
	}
	encode := func(value any) []byte {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal("fixture JSON encoding failed")
		}
		return raw
	}
	if !workflowProvenancePairMatchesClaim(encode(event("workflow_provenance_get_started")), encode(event("workflow_provenance_get_confirmed")), encode(claim)) {
		t.Fatal("matching workflow provenance pair rejected")
	}
	mutations := []struct {
		name  string
		field string
		value any
	}{
		{"controller approval", "controller_approval_sha256", strings.Repeat("c", 64)},
		{"workflow run", "workflow_run_id", int64(8)},
		{"workflow ref", "workflow_ref", "refs/heads/other"},
		{"workflow SHA", "workflow_sha", strings.Repeat("d", 40)},
		{"workflow path", "workflow_path", ".github/workflows/other.yml"},
		{"attempt", "attempt", map[string]any{"device": uint64(5), "inode": uint64(6)}},
		{"journal", "journal", map[string]any{"device": uint64(7), "inode": uint64(8)}},
		{"source", "source", "other-source"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			started, confirmed := event("workflow_provenance_get_started"), event("workflow_provenance_get_confirmed")
			started[mutation.field], confirmed[mutation.field] = mutation.value, mutation.value
			if workflowProvenancePairMatchesClaim(encode(started), encode(confirmed), encode(claim)) {
				t.Error("mismatched workflow provenance identity accepted")
			}
		})
	}
	t.Run("start-and-confirmation-pair", func(t *testing.T) {
		started, confirmed := event("workflow_provenance_get_started"), event("workflow_provenance_get_confirmed")
		confirmed["workflow_ref"] = "refs/heads/other"
		if workflowProvenancePairMatchesClaim(encode(started), encode(confirmed), encode(claim)) {
			t.Error("mismatched start and confirmation accepted")
		}
	})
}

// Qualification requires both actual, current-source commands. Missing/stale
// helpers are setup failures, never optional coverage or an in-process fallback.
func acquireLossHelpers(t *testing.T) (string, string) {
	t.Helper()
	dir, controller, broker := os.Getenv("G01_PHASE_HELPER_DIR"), os.Getenv("G01_PHASE_CONTROLLER_TEST_BINARY"), os.Getenv("G01_PHASE_BROKER_TEST_BINARY")
	if dir == "" || controller != filepath.Join(dir, "controller.test") || broker != filepath.Join(dir, "broker.test") || !privateStateDirectory(dir) {
		t.Fatal("acquire-loss qualification requires owned current-source controller and broker TEST helpers")
	}
	tool := os.Getenv("G01_PHASE_GO")
	if tool == "" {
		tool = "go"
	}
	tool, e := exec.LookPath(tool)
	if e != nil {
		t.Fatal("acquire-loss helper Go tool unavailable")
	}
	var p phaseHelperProvenance
	raw, e := os.ReadFile(filepath.Join(dir, "provenance.json"))
	if e != nil || DecodeStrict(raw, &p) != nil || p.SourceSHA256 != phaseHelperSourceFingerprint(t) || p.GoToolSHA256 != phaseHelperFileDigest(t, tool) || p.ControllerSHA256 != phaseHelperFileDigest(t, controller) || p.BrokerSHA256 != phaseHelperFileDigest(t, broker) {
		t.Fatal("acquire-loss TEST helpers lack matching source/tool/binary provenance")
	}
	t.Logf("helper provenance source=%s tool=%s controller=%s broker=%s", p.SourceSHA256, p.GoToolSHA256, p.ControllerSHA256, p.BrokerSHA256)
	return controller, broker
}

// Read bytes while the child owns the exclusive FileJournal lock. Apply the
// same strict header, sequence, schema and consumption checks without opening
// another writer. Reopen the concrete journal only after command.Wait.
func acquireLossReadEvents(t *testing.T, path string, a Approval) []Event {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil || len(raw) == 0 || len(raw) > 1<<20 || raw[len(raw)-1] != '\n' {
		t.Error("acquire-loss durable journal unavailable or torn")
		return nil
	}
	lines := bytes.Split(raw[:len(raw)-1], []byte{'\n'})
	var header journalHeader
	if DecodeStrict(lines[0], &header) != nil || header.Version != 1 || header.Ownership != ownershipDigest(a) || !header.Authority.matchesOwnership(a) {
		t.Error("acquire-loss strict durable header differs")
		return nil
	}
	authority := header.Authority
	var events []Event
	for _, line := range lines[1:] {
		var event Event
		if DecodeStrict(line, &event) != nil || event.Sequence != len(events)+1 || !validEvent(event) {
			t.Error("acquire-loss strict durable event/sequence differs")
			return nil
		}
		if event.Kind == "authority" {
			if !event.Authority.matchesOwnership(a) || !event.Authority.renews(authority) {
				t.Error("acquire-loss durable authority differs")
				return nil
			}
			authority = *event.Authority
		}
		events = append(events, event)
	}
	if validateControllerHandoffReplay(events) != nil {
		t.Error("acquire-loss strict consumption replay differs")
		return nil
	}
	return events
}

type acquireLossTrace struct {
	mu                            sync.Mutex
	steps                         []string
	close, forbidden, entered     int
	committed, disconnected       bool // fixture causality only, never controller input
	queueAuthorization, sessionID string
}

func acquireLossDurableEntry(t *testing.T, events []Event, a Approval, sid, stage string) {
	t.Helper()
	consumed, phase, opened, observed, ackIntent, ackResult, acquireIntent := 0, 0, 0, 0, 0, 0, 0
	for _, e := range events {
		switch {
		case e.ControllerHandoff != nil && e.ControllerHandoff.Phase == acquireLossPhase:
			consumed++
			if phase != 0 {
				t.Error("consumption must precede phase")
			}
		case e.Kind == "phase" && e.Operation == acquireLossPhase:
			phase++
			if consumed != 1 {
				t.Error("phase must follow exactly one consumption")
			}
		case e.Kind == "result" && e.Operation == "session-open":
			opened++
			if e.SessionID != sid || phase != 1 {
				t.Error("open identity/phase differs")
			}
		case e.Kind == "observation" && e.Operation == "poll":
			observed++
			if e.ID != 9 || e.SessionID != sid || !reflect.DeepEqual(e.RequestIDs, []int64{42}) || opened != 1 {
				t.Error("validated durable poll identity/order differs")
			}
		case e.Kind == "intent" && e.Operation == "ack":
			ackIntent++
			if observed != 1 {
				t.Error("ACK intent preceded validated observation")
			}
		case e.Kind == "result" && e.Operation == "ack":
			ackResult++
			if e.ID != 9 || e.SessionID != sid || ackIntent != 1 {
				t.Error("durable ACK result identity/order differs")
			}
		case e.Kind == "intent" && e.Operation == "acquire":
			acquireIntent++
			if ackResult != 1 || !reflect.DeepEqual(e.RequestIDs, []int64{42}) {
				t.Error("acquire intent preceded exact durable ACK or request42")
			}
		}
	}
	if consumed != 1 || phase != 1 || opened != 1 || observed != 1 || ackIntent != 1 || (stage == "ack" && (ackResult != 0 || acquireIntent != 0)) || (stage == "acquire" && (ackResult != 1 || acquireIntent != 1)) {
		t.Errorf("durable %s entry evidence incomplete: consumed=%d phase=%d open=%d observation=%d ackIntent=%d ackResult=%d acquireIntent=%d", stage, consumed, phase, opened, observed, ackIntent, ackResult, acquireIntent)
	}
}

func acquireLossPreflight(w http.ResponseWriter, r *http.Request, a Approval) bool {
	repo := map[string]any{"id": a.RepositoryID, "full_name": a.Organization + "/" + a.Repository, "private": true, "fork": false}
	var body any
	switch r.URL.Path {
	case "/installation/repositories", "/orgs/fixture-org/actions/runner-groups/3/repositories":
		body = map[string]any{"total_count": 1, "repositories": []any{repo}}
	case "/repos/fixture-org/canary":
		body = repo
	case "/orgs/fixture-org/actions/runner-groups/3":
		body = map[string]any{"id": 3, "visibility": "selected", "default": false, "allows_public_repositories": false, "inherited": false}
		if a.GroupPolicy.Version == 2 {
			body.(map[string]any)["restricted_to_workflows"] = true
			body.(map[string]any)["selected_workflows"] = []string{a.GroupPolicy.Workflow}
		}
	default:
		return false
	}
	if r.Method != http.MethodGet {
		return false
	}
	json.NewEncoder(w).Encode(body)
	return true
}

type acquireLossCommandFixture struct {
	t                                                                        *testing.T
	base                                                                     *baselineFixture
	trace                                                                    *acquireLossTrace
	controller, broker, approvalPath, brokerApprovalPath, attempt, admission string
	raw, management, workflowSeed, issuerSeed                                []byte
	transport                                                                *processBrokerHTTP
	brokerMu                                                                 sync.Mutex
	secretMu                                                                 sync.Mutex
	additionalSecrets                                                        [][]byte
	brokerBeforeRequest                                                      func(*http.Request)
	afterCommand                                                             func()
}

func newAcquireLossCommandFixture(t *testing.T, controller, broker, variant string) *acquireLossCommandFixture {
	return newCommandQualificationFixture(t, controller, broker, variant, acquireLossPhase)
}

// Share signed command setup without weakening the acquisition assertions.
func newCommandQualificationFixture(t *testing.T, controller, broker, variant, phase string) *acquireLossCommandFixture {
	t.Helper()
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	// Actual command qualification uses the reusable-callee v3 contract.
	// Keep its independent group-policy authority aligned with the policy and
	// approval it writes; the legacy zero-value policy is rejected before any
	// provider request or issuance.
	a.GroupPolicy = handoff.GroupPolicy{Version: 2, OrganizationID: 101, Workflow: a.Organization + "/" + a.Repository + "/.github/workflows/runner.yml@" + strings.Repeat("d", 40)}
	if phase == "drain" {
		a.Phases = append(append([]string(nil), a.Phases...), phase)
	}
	// The original approval is tighter than the ten-minute phase cap, so the
	// actual broker and negative envelopes share this exact original deadline.
	a.ExpiresAt = time.Now().Add(5 * time.Minute)
	f := &acquireLossCommandFixture{t: t, controller: controller, broker: broker, trace: &acquireLossTrace{}}
	f.base = newBaselineFixtureWithCreate(t, func(stage string, value any) any {
		if stage == "session" {
			s := value.(scaleset.RunnerScaleSetSession)
			f.trace.mu.Lock()
			f.trace.sessionID, f.trace.queueAuthorization = s.SessionID.String(), "Bearer "+s.MessageQueueAccessToken
			f.trace.mu.Unlock()
		}
		return value
	}, "", a, func(j *FileJournal) {
		original, _ := completedHandoffCreate(t, j, a)
		f.raw = bytes.Clone(original.snapshot)
	})
	f.base.release()
	f.base.release = nil
	f.admission = f.base.j.claim.directory
	parent := filepath.Dir(f.base.j.directory)
	f.approvalPath, f.brokerApprovalPath, f.attempt = filepath.Join(parent, "controller-approval.json"), filepath.Join(parent, "broker-"+phase+".json"), filepath.Join(parent, "attempt-"+phase)
	if os.WriteFile(f.approvalPath, f.raw, 0600) != nil {
		t.Fatal("original raw approval write")
	}
	if handoff.Digest(f.raw) == approvalDigest(a) || len(f.base.j.Events()) != 7 {
		t.Fatal("synthetic signed completed-create prefix lost")
	}
	journalPath := filepath.Join(f.base.j.directory, "journal.jsonl")
	originalPrefix, e := os.ReadFile(journalPath)
	if e != nil {
		t.Fatal("original durable prefix read")
	}
	f.base.readEvents = func() []Event {
		current, e := os.ReadFile(journalPath)
		if e != nil || !bytes.HasPrefix(current, originalPrefix) {
			t.Error("original signed completed-create bytes/sequence changed")
		}
		return acquireLossReadEvents(t, journalPath, a)
	}
	// Count attempts before baseline dispatch/body draining, including forbidden
	// requests. The real SDK's acquire POST has no session UUID wire field.
	f.base.beforeRequest = func(w http.ResponseWriter, r *http.Request) bool {
		x := f.trace
		x.mu.Lock()
		defer x.mu.Unlock()
		path := r.URL.Path
		runtime := "/tenant/_apis/runtime/runnerscalesets/7"
		switch {
		case r.Method == "POST" && path == runtime+"/sessions":
			x.steps = append(x.steps, "open")
			if r.URL.RawQuery != "api-version=6.0-preview" {
				t.Error("physical open query differs")
			}
		case r.Method == "GET" && path == "/queue":
			x.steps = append(x.steps, "poll")
			if r.URL.RawQuery != "" || r.Header.Get("Authorization") != x.queueAuthorization {
				t.Error("physical poll cursor/session differs")
			}
		case r.Method == "DELETE" && path == "/queue/9":
			x.steps = append(x.steps, "ack")
			if r.URL.RawQuery != "" || r.Header.Get("Authorization") != x.queueAuthorization || f.base.sources.Load() != 1 {
				t.Error("physical ACK session/source validation differs")
			}
			acquireLossDurableEntry(t, f.base.readEvents(), a, x.sessionID, "ack")
		case r.Method == "POST" && path == runtime+"/acquirejobs":
			x.steps = append(x.steps, "acquire")
			x.entered++
			f.base.acquires.Add(1)
			body, e := io.ReadAll(io.LimitReader(r.Body, 65))
			r.Body.Close()
			if e != nil || !bytes.Equal(body, []byte("[42]")) || r.URL.RawQuery != "api-version=6.0-preview" || r.Header.Get("Authorization") != x.queueAuthorization || x.queueAuthorization == "" || !reflect.DeepEqual(x.steps, []string{"open", "poll", "ack", "acquire"}) {
				t.Error("physical acquire method/path/query/body/session authorization/order differs")
			}
			acquireLossDurableEntry(t, f.base.readEvents(), a, x.sessionID, "acquire")
			if variant == "response-discarded" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"count": 1, "value": []int64{42}})
				return true
			}
			conn, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error("acquire disconnect hijack setup failed")
				return true
			}
			if variant == "disconnect-after-commit" {
				x.committed = true
			}
			if conn.Close() != nil {
				t.Error("acquire disconnect close failed")
				return true
			}
			x.disconnected = true
			return true
		case r.Method == "DELETE" && path == runtime+"/sessions/"+x.sessionID:
			x.close++
			x.steps = append(x.steps, "close")
			w.WriteHeader(http.StatusNoContent) // a valid close cannot hide an attempt
			return true
		case r.Method == "GET" && (path == runtime || path == "/installation/repositories" || path == "/repos/fixture-org/canary" || path == "/orgs/fixture-org/actions/runner-groups/3" || path == "/orgs/fixture-org/actions/runner-groups/3/repositories" || path == workflowRunFixturePath(&processBrokerHTTP{approval: &a})):
			if x.entered > 0 {
				x.forbidden++
				t.Error("post-acquire discovery attempted")
			}
		case r.Method == "POST" && (strings.HasSuffix(path, "/runners/registration-token") || strings.HasSuffix(path, "/actions/runner-registration")):
			if x.entered > 0 {
				x.forbidden++
				t.Error("post-acquire bootstrap attempted")
			}
		default:
			x.forbidden++
			w.WriteHeader(http.StatusForbidden)
			return true
		}
		return false
	}
	f.base.extra = func(w http.ResponseWriter, r *http.Request) bool { return acquireLossPreflight(w, r, a) }
	if f.base.j.Close() != nil {
		t.Fatal("release completed-create writer")
	}
	writePhaseFixtureConfig(t, f.base.j.directory, f.admission, experimentalPolicy(t, a, f.raw, "controller", phase, "controller"), a.HarnessSHA, f.base.api, f.base.caPEM)
	// Broker binary admission requires an owned single-link executable in the
	// original parent. Copy exactly the manifest-checked controller bytes.
	data, e := os.ReadFile(controller)
	if e != nil {
		t.Fatal("controller TEST helper read")
	}
	f.controller = filepath.Join(parent, "experimental-controller-test")
	if os.WriteFile(f.controller, data, 0500) != nil {
		t.Fatal("controller TEST helper copy")
	}
	ba := enrollment.BrokerApproval{GroupPolicy: a.GroupPolicy, ManagementScope: a.ManagementScope, OwnerNonce: a.OwnerNonce, Mode: "controller", Phase: phase, AppID: a.AppID, AppName: "fixture-app", AppOwner: a.Organization, AppOwnerID: 101, InstallationID: a.InstallationID, Organization: a.Organization, OrganizationID: 101, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: int64(a.RunnerGroupID), RunnerGroupName: "fixture-group", ExpiresAt: a.ExpiresAt, ControllerBinarySHA256: handoff.Digest(data), ControllerApprovalSHA256: handoff.Digest(f.raw), ControllerHarnessSHA: a.HarnessSHA, AllowVerificationAuthority: true}
	raw, _ := json.Marshal(ba)
	if os.WriteFile(f.brokerApprovalPath, raw, 0600) != nil {
		t.Fatal("broker approval write")
	}
	f.transport = &processBrokerHTTP{t: t, attempt: f.attempt, approval: &f.base.a}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.brokerMu.Lock()
		defer f.brokerMu.Unlock()
		if f.brokerBeforeRequest != nil {
			f.brokerBeforeRequest(r)
		}
		if r.URL.Path == workflowRunFixturePath(f.transport) && r.Header.Get("Authorization") != "Bearer synthetic-observation-credential" && r.Header.Get("Authorization") != "Bearer "+f.base.api.credentials.VerificationToken {
			t.Error("broker observation authority differs")
		}
		response, e := f.transport.RoundTrip(r)
		if e != nil {
			w.WriteHeader(500)
			return
		}
		defer response.Body.Close()
		w.WriteHeader(response.StatusCode)
		io.Copy(w, response.Body)
	}))
	t.Cleanup(server.Close)
	config, _ := json.Marshal(map[string]string{"policy": experimentalPolicy(t, a, f.raw, "controller", phase, "broker"), "base_url": server.URL, "ca_pem": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})), "admission": f.admission, "worker_admission": ""})
	if os.WriteFile(filepath.Join(f.base.j.directory, "broker-experiment-fixture.json"), config, 0600) != nil {
		t.Fatal("broker fixture write")
	}
	f.workflowSeed, _ = hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	f.issuerSeed = bytes.Repeat([]byte{71}, ed25519.SeedSize)
	f.management = experimentalManagement(t, f.base.api.credentials.VerificationToken)
	t.Cleanup(func() { clear(f.management); clear(f.workflowSeed); clear(f.issuerSeed) })
	return f
}

func (f *acquireLossCommandFixture) noSecrets(data []byte) {
	f.t.Helper()
	secrets := [][]byte{f.management, f.workflowSeed, f.issuerSeed, []byte("PRIVATE KEY"), []byte("synthetic-process-installation-token"), []byte("synthetic-installation-token"), []byte("synthetic-observation-credential"), []byte("synthetic-secret-queue"), []byte(f.base.api.credentials.VerificationToken), []byte("synthetic-registration"), []byte("failed to issue acquire jobs"), []byte("failed to acquire jobs"), []byte("unexpected status code"), []byte("Post \""), []byte("EOF")}
	f.secretMu.Lock()
	secrets = append(secrets, f.additionalSecrets...)
	f.secretMu.Unlock()
	for i, secret := range secrets {
		if len(secret) > 0 && bytes.Contains(data, secret) {
			f.t.Errorf("secret/raw-error class %d escaped command or durable evidence", i)
		}
	}
}

func (f *acquireLossCommandFixture) run(binary string, args []string, inputs ...[]byte) []*os.File {
	return f.runOutcome(binary, args, false, inputs...)
}

func (f *acquireLossCommandFixture) runOutcome(binary string, args []string, success bool, inputs ...[]byte) []*os.File {
	outcome := commandRefused
	if success {
		outcome = commandBrokerCompleted
	}
	return f.runExpected(binary, args, outcome, nil, inputs...)
}

type commandOutcome int

const (
	commandRefused commandOutcome = iota
	commandBrokerCompleted
	commandJournalPrepared
)

// Preparation has a distinct concrete receipt contract; it cannot use the
// broker-success path or silently accept an arbitrary successful command.
func (f *acquireLossCommandFixture) runExpected(binary string, args []string, outcome commandOutcome, prepared *PreparationReceipt, inputs ...[]byte) []*os.File {
	f.t.Helper()
	if outcome < commandRefused || outcome > commandJournalPrepared || outcome == commandBrokerCompleted && binary != f.broker || outcome == commandJournalPrepared && (binary != f.controller || prepared == nil || len(args) == 0 || args[0] != "--prepare-approved-journal" || len(inputs) != 0) {
		f.t.Fatal("invalid explicit command outcome contract")
	}
	success := outcome != commandRefused
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = []string{"LANG=C", "LC_ALL=C"}
	var pipes []*os.File
	var writers []*os.File
	var joined []<-chan struct{}
	for _, input := range inputs {
		r, w, e := os.Pipe()
		if e != nil {
			f.t.Fatal("owned command input pipe setup failed")
		}
		done := make(chan struct{})
		go func(data []byte) {
			defer close(done)
			defer w.Close()
			if n, e := w.Write(data); e != nil || n != len(data) {
				f.t.Error("owned command input write incomplete")
			}
		}(input)
		f.t.Cleanup(func() { r.Close(); w.Close(); <-done })
		pipes = append(pipes, r)
		writers = append(writers, w)
		joined = append(joined, done)
	}
	if len(pipes) > 0 {
		cmd.Stdin = pipes[0]
		cmd.ExtraFiles = pipes[1:]
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	e := cmd.Run()
	for i, w := range writers {
		w.Close()
		<-joined[i]
	}
	f.noSecrets(stdout.Bytes())
	f.noSecrets(stderr.Bytes())
	f.scanCommandJournals()
	if f.afterCommand != nil {
		f.afterCommand()
	}
	var exit *exec.ExitError
	if ctx.Err() != nil || e != nil && (!errors.As(e, &exit) || exit.ExitCode() != 1) {
		f.t.Fatal("actual acquire-loss command setup failure or timeout")
	}
	if (e == nil) != success {
		f.t.Error("actual command success/quarantine outcome differs")
	}
	want := "canary refused; approval, authority or private state requires review\n"
	if binary == f.broker {
		want = "broker stopped; retain private intent; credentials not persisted; no automatic retry\n"
	}
	if stderr.Len() != 0 || (!success && stdout.String() != want) {
		f.t.Error("actual command fixed sanitized outcome differs")
	}
	if outcome == commandBrokerCompleted {
		var result enrollment.BrokerResult
		if binary != f.broker || DecodeStrict(stdout.Bytes(), &result) != nil || result.Status != "controller_completed" || result.ActionsHost != "" {
			f.t.Error("actual command successful broker receipt differs")
		}
	}
	if outcome == commandJournalPrepared && (DecodeStrict(stdout.Bytes(), prepared) != nil || prepared.Version != 1 || prepared.Status != "controller_journal_prepared") {
		f.t.Error("actual command successful preparation receipt differs")
	}
	return pipes
}

// Scan before behavioral assertions can return on an intentional fault. The
// records stay private; diagnostics contain only fixed classes, never contents.
func (f *acquireLossCommandFixture) scanCommandJournals() {
	for _, directory := range []string{f.base.j.directory, f.attempt, f.attempt + "-fresh", f.admission} {
		paths, e := filepath.Glob(filepath.Join(directory, "*.jsonl"))
		if e != nil {
			f.t.Error("private command record inventory failed")
			continue
		}
		for _, path := range paths {
			data, e := os.ReadFile(path)
			if e != nil {
				f.t.Error("private command record unavailable")
				continue
			}
			f.noSecrets(data)
		}
	}
}

func (f *acquireLossCommandFixture) brokerArgs(attempt string) []string {
	return []string{"--execute-experimental-broker", "--approval", f.brokerApprovalPath, "--state-dir", attempt, "--controller-binary", f.controller, "--controller-approval", f.approvalPath, "--controller-state-dir", f.base.j.directory}
}

func (f *acquireLossCommandFixture) assertRetained(variant string) []Event {
	t := f.t
	j, e := OpenJournalForPairedFixtureAt(f.base.j.directory, f.base.a, f.admission)
	if e != nil {
		t.Fatal("strict concrete FileJournal reopen rejected retained history")
	}
	events := j.Events()
	if !reflect.DeepEqual(events, f.base.readEvents()) {
		t.Error("strict reader and concrete reopened journal differ")
	}
	s := replayWithApproval(events, &f.base.a)
	if j.Close() != nil {
		t.Fatal("reopened journal close")
	}
	x := f.trace
	x.mu.Lock()
	defer x.mu.Unlock()
	if !s.reserved {
		t.Error("reopened reserved=true missing")
	}
	if !s.phaseSeen[acquireLossPhase] || !s.observedJobs[42] || !s.workObserved || !s.uncertain || s.sessionID != x.sessionID || s.sessionID == "" || s.setID != 7 || s.deleted || s.phaseSeen["inspect"] || s.phaseSeen["cleanup"] {
		t.Error("reopened quarantine/work/session/set/optional-inspect retention differs")
	}
	responses, unknownAcquire, unknownProbe, results, closeResults := 0, 0, 0, 0, 0
	for _, event := range events {
		if event.ControllerHandoff != nil && event.ControllerHandoff.Phase == acquireLossPhase && event.ControllerHandoff.ApprovalSHA256 != handoff.Digest(f.raw) {
			t.Error("actual receipt did not bind original raw approval")
		}
		if event.Kind == "response" && event.Operation == "acquire" {
			responses++
			if event.Succeeded != (variant == "response-discarded") {
				t.Error("acquire response success category differs")
			}
		}
		if event.Kind == "unknown" && event.Operation == "acquire" {
			if responses != 1 {
				t.Error("unknown acquire preceded response classification")
			}
			unknownAcquire++
		}
		if event.Kind == "unknown" && event.Operation == "probe" {
			if unknownAcquire != 1 {
				t.Error("unknown probe preceded unknown acquire")
			}
			unknownProbe++
		}
		if event.Kind == "result" && event.Operation == "acquire" {
			results++
		}
		if event.Kind == "result" && event.Operation == "session-close" {
			closeResults++
		}
	}
	if responses != 1 || unknownAcquire != 1 || results != 0 {
		t.Errorf("acquire-specific suppression: response=%d unknown(acquire)=%d result(acquire)=%d", responses, unknownAcquire, results)
	}
	if unknownProbe != 1 || closeResults != 0 {
		t.Error("ErrQuarantine probe path or no-close result differs")
	}
	if !reflect.DeepEqual(x.steps, []string{"open", "poll", "ack", "acquire"}) || x.entered != 1 || f.base.sessions.Load() != 1 || f.base.polls.Load() != 1 || f.base.acks.Load() != 1 || f.base.acquires.Load() != 1 {
		t.Errorf("physical acquire1/server-entry/order missing: entered=%d open=%d poll=%d ACK=%d acquire=%d", x.entered, f.base.sessions.Load(), f.base.polls.Load(), f.base.acks.Load(), f.base.acquires.Load())
	}
	if x.close != 0 || x.forbidden != 0 || f.base.forbidden.Load() != 0 {
		t.Errorf("physical close0/forbidden0 violated: close=%d forbidden=%d", x.close, x.forbidden)
	}
	if x.committed != (variant == "disconnect-after-commit") || x.disconnected != (variant != "response-discarded") {
		t.Error("fixture causal commit/disconnect witness absent")
	}
	return events
}

func (f *acquireLossCommandFixture) guardControls(events []Event) {
	f.guardControlsForPhase(events, acquireLossPhase)
}

func (f *acquireLossCommandFixture) guardControlsForPhase(events []Event, phase string) {
	t := f.t
	journal := filepath.Join(f.base.j.directory, "journal.jsonl")
	before, _ := os.ReadFile(journal)
	brokerJournal := filepath.Join(f.attempt, "broker.jsonl")
	brokerBefore, e := os.ReadFile(brokerJournal)
	if e != nil {
		t.Fatal("original actual broker journal unavailable")
	}
	f.brokerMu.Lock()
	originalCalls := len(f.transport.calls)
	f.brokerMu.Unlock()
	requests := f.base.requests.Load()
	var nonce string
	for _, e := range events {
		if e.ControllerHandoff != nil && e.ControllerHandoff.Phase == phase {
			nonce = e.ControllerHandoff.ReceiptNonce
		}
	}
	if nonce == "" {
		t.Fatal("actual broker receipt consumption nonce missing")
	}
	// Valid signed negative admission specimens retain original raw approval and
	// deadline. The three primary runs above always use real broker issuance.
	freshNonce := strings.Repeat("f", 32)
	if freshNonce == nonce {
		freshNonce = strings.Repeat("e", 32)
	}
	for _, n := range []string{nonce, freshNonce} {
		a := f.base.a
		now := time.Now().UTC().Truncate(time.Second)
		policyJSON := []byte(experimentalPolicy(t, a, f.raw, "controller", phase, "controller"))
		policy, workflowRoot, issuerRoot, e := handoff.DecodePublicPolicy(policyJSON, "controller", now)
		if e != nil || len(policy.Grants) != 1 {
			t.Fatal("receiver negative selected policy unavailable")
		}
		var request handoff.Request
		if policy.Version == 3 {
			_, request, e = controllerHandoffRequestForExpected(f.raw, phase, policy.Source, &policy.Grants[0].Request, now)
		} else {
			_, request, e = controllerHandoffRequest(f.raw, phase, policy.Source, now)
		}
		if e != nil || request != policy.Grants[0].Request {
			t.Fatal("receiver negative request does not match selected policy grant")
		}
		key := ed25519.NewKeyFromSeed(f.workflowSeed)
		receipt := signedControllerReceipt(t, request, now, n, key)
		if request.Version == 2 {
			receipt.Version = 2
			receipt.Callee = request.Callee
		}
		receipt.ExpiresAt = a.ExpiresAt
		receipt.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, receipt.SigningBytes()))
		receiptRaw, _ := json.Marshal(receipt)
		payload := handoffCredentialBytes(t, a)
		defer clear(payload)
		binding := handoff.PhaseBinding{BindingClaims: handoff.BindingClaims{Request: request, Version: 2, Kind: handoff.PhaseBindingKind, Algorithm: "ed25519", Audience: handoff.PhaseAudience("controller", phase), KeyID: "fixture-issuer-v1", IssuerSource: handoff.IssuerSource, WorkflowKeyID: receipt.KeyID, ReceiptSHA256: handoff.Digest(receiptRaw), PayloadSHA256: handoff.Digest(payload), PayloadLength: len(payload), ReceiptNonce: n, IssuedAt: now, ExpiresAt: a.ExpiresAt}, Mode: "controller"}
		issuerKey := ed25519.NewKeyFromSeed(f.issuerSeed)
		binding.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(issuerKey, binding.SigningBytes()))
		if workflowRoot.Verify(request, receipt, time.Now()) != nil || issuerRoot.VerifyPhaseBinding(handoff.BindingExpected{Request: request, Receipt: receipt, ReceiptBytes: receiptRaw, ApprovalExpiresAt: a.ExpiresAt, Deadline: a.ExpiresAt}, binding, time.Now()) != nil {
			t.Fatal("receiver negative specimen is not otherwise valid")
		}
		clear(key)
		clear(issuerKey)
		bindingRaw, _ := json.Marshal(binding)
		frame, e := handoff.EncodeControl(receiptRaw, bindingRaw)
		if e != nil {
			t.Fatal("receiver negative frame")
		}
		pipes := f.run(f.controller, []string{"--execute-experimental-phase", "--mode", "controller", "--approval", f.approvalPath, "--state-dir", f.base.j.directory, "--phase", phase}, payload, frame)
		unread, e := io.ReadAll(pipes[0])
		if e != nil || !bytes.Equal(unread, payload) {
			t.Error("receiver replay read credential payload")
		}
	}
	f.run(f.controller, []string{"--prepare-approved-journal", "--approval", f.approvalPath, "--state-dir", f.base.j.directory, "--phase", phase})
	inputs := [][]byte{f.management, []byte("synthetic-observation-credential"), f.workflowSeed, f.issuerSeed}
	pipes := f.run(f.broker, f.brokerArgs(f.attempt), inputs...)
	for i, pipe := range pipes {
		unread, e := io.ReadAll(pipe)
		if e != nil || !bytes.Equal(unread, inputs[i]) {
			t.Error("exact broker-attempt replay read protected input")
		}
	}
	brokerAfter, _ := os.ReadFile(brokerJournal)
	f.brokerMu.Lock()
	replayCalls, replayMints := len(f.transport.calls), f.transport.mints
	replayReadOnlyMints, replayManagementMints, replayRevocations := f.transport.readOnlyMints, f.transport.managementMints, f.transport.revocations
	f.brokerMu.Unlock()
	if !bytes.Equal(brokerBefore, brokerAfter) || replayCalls != originalCalls || replayMints != 2 || replayReadOnlyMints != 1 || replayManagementMints != 1 || replayRevocations != 1 {
		t.Error("exact broker-attempt replay changed history or crossed API/mint boundary")
	}
	claimPath := filepath.Join(f.admission, "broker-admission.jsonl")
	claim, e := os.ReadFile(claimPath)
	if e != nil {
		t.Fatal("original occupied broker claim missing")
	}
	ledger := bytes.Split(bytes.TrimSpace(claim), []byte{'\n'})
	claimIndex := -1
	for i := 1; i < len(ledger); i++ {
		var event struct {
			Kind string `json:"kind"`
			Slot string `json:"slot"`
		}
		if json.Unmarshal(ledger[i], &event) != nil {
			t.Fatal("original broker admission event malformed")
		}
		if event.Kind == "claim" && event.Slot == phase {
			if claimIndex >= 0 {
				t.Fatal("duplicate original occupied broker claim")
			}
			claimIndex = i
		}
	}
	if claimIndex < 0 {
		t.Fatal("original occupied broker claim identity missing")
	}
	// A confirmed account-ledger workflow GET permanently occupies this phase.
	// Older source without account-level GET accounting reaches the existing
	// broker admission fence after observation; both paths must avoid reminting.
	fresh := f.attempt + "-fresh"
	workflowGETAccounted := false
	for i := 1; i < claimIndex; i++ {
		var event struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(ledger[i], &event) != nil {
			t.Fatal("original broker workflow provenance event malformed")
		}
		if event.Kind == "workflow_provenance_get_started" {
			if workflowGETAccounted || i+1 >= claimIndex || !workflowProvenancePairMatchesClaim(ledger[i], ledger[i+1], ledger[claimIndex]) {
				t.Fatal("original broker workflow provenance pair is not bound to its claim")
			}
			workflowGETAccounted = true
		}
	}
	f.brokerMu.Lock()
	callCount := len(f.transport.calls)
	mintsBefore := f.transport.mints
	readOnlyMintsBefore := f.transport.readOnlyMints
	managementMintsBefore := f.transport.managementMints
	revocationsBefore := f.transport.revocations
	f.transport.attempt = fresh
	f.brokerMu.Unlock()
	f.run(f.broker, f.brokerArgs(fresh), inputs...)
	f.brokerMu.Lock()
	calls := append([]string(nil), f.transport.calls[callCount:]...)
	mintsAfter := f.transport.mints
	readOnlyMintsAfter := f.transport.readOnlyMints
	managementMintsAfter := f.transport.managementMints
	revocationsAfter := f.transport.revocations
	f.brokerMu.Unlock()
	wantCalls := []string{"GET " + workflowRunFixturePath(f.transport)}
	if workflowGETAccounted {
		wantCalls = nil
	}
	if !reflect.DeepEqual(calls, wantCalls) || mintsAfter != mintsBefore || readOnlyMintsAfter != readOnlyMintsBefore || managementMintsAfter != managementMintsBefore || revocationsAfter != revocationsBefore {
		t.Error("fresh occupied-phase broker attempt crossed the expected provider or token boundary")
	}
	snapshotPath := filepath.Join(fresh, "controller-approval.json")
	if workflowGETAccounted {
		if _, e := os.Lstat(snapshotPath); !os.IsNotExist(e) {
			t.Error("fresh account-ledger occupied-phase attempt created an approval snapshot")
		}
	} else {
		snapshot, e := os.ReadFile(snapshotPath)
		if e != nil || !bytes.Equal(snapshot, f.raw) {
			t.Error("fresh legacy attempt did not reach the existing admission fence")
		}
	}
	claimAfter, _ := os.ReadFile(claimPath)
	after, _ := os.ReadFile(journal)
	if !bytes.Equal(before, after) || !bytes.Equal(claim, claimAfter) || f.base.requests.Load() != requests {
		t.Error("guard commands changed durable history/claim or receiver effects")
	}
	for _, path := range []string{journal, claimPath, filepath.Join(f.attempt, "broker.jsonl"), filepath.Join(fresh, "broker.jsonl")} {
		data, e := os.ReadFile(path)
		if e != nil {
			t.Fatal("durable command evidence unavailable")
		}
		f.noSecrets(data)
		t.Logf("sanitized durable record hash=%s bytes=%d", handoff.Digest(data), len(data))
	}
}

func TestExperimentalAcquireLossCommand(t *testing.T) {
	controller, broker := acquireLossHelpers(t)
	knowledge := make(map[string][]Event)
	for _, variant := range []string{"response-discarded", "disconnect-after-commit", "disconnect-without-commit"} {
		t.Run(variant, func(t *testing.T) {
			f := newAcquireLossCommandFixture(t, controller, broker, variant)
			f.run(f.broker, f.brokerArgs(f.attempt), f.management, []byte("synthetic-observation-credential"), f.workflowSeed, f.issuerSeed)
			f.brokerMu.Lock()
			mints := f.transport.mints
			managementMints := f.transport.managementMints
			readOnlyMints := f.transport.readOnlyMints
			revocations := f.transport.revocations
			calls := append([]string(nil), f.transport.calls...)
			f.brokerMu.Unlock()
			if managementMints != 1 {
				t.Fatalf("actual broker management issuance did not mint exactly once: total=%d read-only=%d management=%d revocations=%d calls=%v", mints, readOnlyMints, managementMints, revocations, calls)
			}
			if mints != 2 || readOnlyMints != 1 || revocations != 1 {
				t.Fatalf("actual broker issuance lifecycle differs: total=%d read-only=%d management=%d revocations=%d calls=%v", mints, readOnlyMints, managementMints, revocations, calls)
			}
			events := f.assertRetained(variant)
			if t.Failed() {
				return
			} // Preserve the acquire-specific mutation failure.
			f.guardControls(events)
			f.assertRetained(variant)
			for _, e := range events[7:] {
				if e.ControllerHandoff != nil {
					continue
				} // per-invocation provenance
				e.Sequence = 0
				knowledge[variant] = append(knowledge[variant], e)
			}
		})
	}
	if a, b := knowledge["disconnect-after-commit"], knowledge["disconnect-without-commit"]; a != nil && b != nil && !reflect.DeepEqual(a, b) {
		t.Error("fixture commit truth changed controller knowledge")
	}
	for _, mode := range []string{"cancel", "continue"} {
		t.Run("original-"+mode+"-after-acquire-intent", func(t *testing.T) { runOriginalContextCommandCase(t, controller, broker, "acquire-loss", mode) })
	}
}
