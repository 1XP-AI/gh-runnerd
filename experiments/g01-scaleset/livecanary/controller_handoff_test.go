package livecanary

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func TestControllerHandoffConsumptionEventHasStrictTypedJournalShape(t *testing.T) {
	data := []byte(`{"sequence":1,"kind":"controller-handoff-consumed","controller_handoff":{"receipt_nonce":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","phase":"before-ack","approval_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","receipt_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","key_id":"fixture-ed25519-v1"}}`)
	var event Event
	if err := DecodeStrict(data, &event); err != nil {
		t.Fatalf("valid credential-free handoff consumption event was rejected: %v", err)
	}
	if !validEvent(event) {
		t.Fatal("valid credential-free handoff consumption event failed journal validation")
	}
}

type controllerHandoffFixture struct {
	now          time.Time
	approval     Approval
	snapshot     []byte
	request      handoff.Request
	root         handoff.TrustRoot
	privateKey   ed25519.PrivateKey
	receipt      handoff.Receipt
	receiptBytes []byte
}

func newControllerHandoffFixture(t *testing.T, phase string) controllerHandoffFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	return controllerHandoffFixtureForApproval(t, controllerHandoffApprovalAt(now), phase, now)
}

func controllerHandoffFixtureForApproval(t *testing.T, approval Approval, phase string, now time.Time) controllerHandoffFixture {
	t.Helper()
	snapshot, err := json.Marshal(approval)
	if err != nil {
		t.Fatal("marshal synthetic approval")
	}
	validatedApproval, request, err := controllerHandoffRequest(snapshot, phase, "signed-fixture-v1", now)
	if err != nil {
		t.Fatalf("build synthetic request: %v", err)
	}
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	root, err := handoff.NewTrustRoot("fixture-ed25519-v1", publicKey)
	if err != nil {
		t.Fatal("construct synthetic trust root")
	}
	receipt := signedControllerReceipt(t, request, now, "dddddddddddddddddddddddddddddddd", privateKey)
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal("marshal synthetic receipt")
	}
	return controllerHandoffFixture{now: now, approval: validatedApproval, snapshot: snapshot, request: request, root: root, privateKey: privateKey, receipt: receipt, receiptBytes: receiptBytes}
}

func signedControllerReceipt(t *testing.T, request handoff.Request, now time.Time, receiptNonce string, privateKey ed25519.PrivateKey) handoff.Receipt {
	t.Helper()
	receipt := handoff.Receipt{
		Version:                  1,
		Algorithm:                "ed25519",
		KeyID:                    "fixture-ed25519-v1",
		ControllerApprovalSHA256: request.ControllerApprovalSHA256,
		Repository:               request.Repository,
		WorkflowRunID:            request.WorkflowRunID,
		WorkflowRef:              request.WorkflowRef,
		WorkflowSHA:              request.WorkflowSHA,
		WorkflowPath:             request.WorkflowPath,
		Phase:                    request.Phase,
		OwnerNonce:               request.OwnerNonce,
		ReceiptNonce:             receiptNonce,
		Source:                   request.Source,
		IssuedAt:                 now,
		ExpiresAt:                now.Add(time.Hour),
	}
	receipt.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, receipt.SigningBytes()))
	return receipt
}

func (fixture controllerHandoffFixture) encodeReceipt(t *testing.T, receipt handoff.Receipt, resign bool) []byte {
	t.Helper()
	if resign {
		receipt.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(fixture.privateKey, receipt.SigningBytes()))
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal("marshal mutated receipt")
	}
	return data
}

func controllerHandoffApprovalAt(now time.Time) Approval {
	approval := approval()
	approval.WorkflowRunID = 7
	approval.WorkflowRef = "refs/heads/main"
	approval.ExpiresAt = now.Add(2 * time.Hour)
	approval.Phases = []string{"before-ack", "after-ack"}
	return approval
}

func controllerHandoffSnapshot(t *testing.T, approval Approval) []byte {
	t.Helper()
	data, err := json.Marshal(approval)
	if err != nil {
		t.Fatal("marshal synthetic approval")
	}
	return data
}

func requireHandoffReaderUntouched(t *testing.T, approvalSnapshot []byte, phase, source string, receiptBytes []byte, root handoff.TrustRoot, now time.Time) {
	t.Helper()
	var approval Approval
	if DecodeStrict(approvalSnapshot, &approval) != nil {
		t.Fatal("test approval snapshot is malformed")
	}
	journal, err := openTestJournal(t, privateDir(t), approval)
	if err != nil {
		t.Fatal("open synthetic journal")
	}
	defer journal.Close()
	reads := 0
	_, err = consumeControllerHandoff(context.Background(), journal, approvalSnapshot, phase, source, bytes.NewReader(receiptBytes), root, func() time.Time { return now }, func(context.Context) ([]byte, error) {
		reads++
		return []byte("synthetic-credential"), nil
	})
	if err == nil || reads != 0 {
		t.Fatalf("invalid handoff crossed reader boundary: err=%v reads=%d", err, reads)
	}
}

func TestControllerHandoffRequestUsesOneRawApprovalSnapshot(t *testing.T) {
	fixedNow := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	approvalSnapshot := []byte(`{"app_id":11,"installation_id":12,"organization":"fixture-org","repository":"canary","repository_id":42,"runner_group_id":3,"owner_nonce":"cccccccccccccccccccccccccccccccc","harness_sha":"1111111111111111111111111111111111111111","workflow_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","workflow_ref":"refs/heads/main","workflow_path":".github/workflows/canary.yml","workflow_run_id":7,"controller":"fixture-controller","expires_at":"2026-09-22T13:00:00Z","actions_hosts":["fixture.actions.githubusercontent.com"],"phases":["before-ack","after-ack"]}`)
	approval, request, err := controllerHandoffRequest(approvalSnapshot, "before-ack", "signed-fixture-v1", fixedNow)
	if err != nil {
		t.Fatalf("valid approval snapshot rejected: %v", err)
	}
	approvalHash := sha256.Sum256(approvalSnapshot)
	if request.ControllerApprovalSHA256 != hex.EncodeToString(approvalHash[:]) || request.ControllerApprovalSHA256 != "1343ffdc5440d65d48e2b1916e6aff470f1d11cdfa30aefe54b63293f15ecdf3" || request.Repository != "fixture-org/canary" || request.WorkflowRunID != 7 || request.WorkflowRef != "refs/heads/main" || request.WorkflowSHA != strings.Repeat("b", 40) || request.WorkflowPath != ".github/workflows/canary.yml" || request.Phase != "before-ack" || request.OwnerNonce != strings.Repeat("c", 32) || request.Source != "signed-fixture-v1" {
		t.Fatalf("request was not derived from exact approval bytes and explicit policy: %+v", request)
	}
	if approval.Controller != "fixture-controller" || approval.ExpiresAt != fixedNow.Add(time.Hour) {
		t.Fatal("strict approval snapshot did not bind controller and expiry")
	}
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	receipt := signedControllerReceipt(t, request, fixedNow, strings.Repeat("d", 32), privateKey)
	if !bytes.Equal(receipt.SigningBytes(), []byte(`{"version":1,"algorithm":"ed25519","key_id":"fixture-ed25519-v1","controller_approval_sha256":"`+request.ControllerApprovalSHA256+`","repository":"fixture-org/canary","workflow_run_id":7,"workflow_ref":"refs/heads/main","workflow_sha":"`+strings.Repeat("b", 40)+`","workflow_path":".github/workflows/canary.yml","phase":"before-ack","owner_nonce":"`+strings.Repeat("c", 32)+`","receipt_nonce":"`+strings.Repeat("d", 32)+`","source":"signed-fixture-v1","issued_at":"2026-09-22T12:00:00Z","expires_at":"2026-09-22T13:00:00Z"}`)) {
		t.Fatalf("G01 fixture changed v1 signing bytes: %s", receipt.SigningBytes())
	}
	root, err := handoff.NewTrustRoot(receipt.KeyID, privateKey.Public().(ed25519.PublicKey))
	if err != nil || root.Verify(request, receipt, fixedNow) != nil {
		t.Fatal("G01 did not verify deterministic shared v1 receipt")
	}
	whitespaceSnapshot := append([]byte(" \n"), approvalSnapshot...)
	whitespaceApproval, whitespaceRequest, err := controllerHandoffRequest(whitespaceSnapshot, "before-ack", "signed-fixture-v1", fixedNow)
	if err != nil || approvalDigest(whitespaceApproval) != approvalDigest(approval) || whitespaceRequest.ControllerApprovalSHA256 == request.ControllerApprovalSHA256 || root.Verify(whitespaceRequest, receipt, fixedNow) == nil {
		t.Fatal("equivalent JSON bytes reused a receipt instead of binding the raw snapshot")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Approval)
	}{
		{name: "controller", mutate: func(a *Approval) { a.Controller = "other-controller" }},
		{name: "approval expiry", mutate: func(a *Approval) { a.ExpiresAt = a.ExpiresAt.Add(time.Minute) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := approval
			tc.mutate(&changed)
			changedBytes := controllerHandoffSnapshot(t, changed)
			_, changedRequest, err := controllerHandoffRequest(changedBytes, "before-ack", "signed-fixture-v1", fixedNow)
			if err != nil || changedRequest.ControllerApprovalSHA256 == request.ControllerApprovalSHA256 || root.Verify(changedRequest, receipt, fixedNow) == nil {
				t.Fatal("receipt remained valid after raw approval-bound field changed")
			}
		})
	}
}

func TestControllerHandoffRejectsParserTrustAndTupleFailuresBeforeReader(t *testing.T) {
	fixture := newControllerHandoffFixture(t, "before-ack")
	valid := fixture.receiptBytes
	oversized := append(bytes.Clone(valid), bytes.Repeat([]byte(" "), handoff.MaxReceiptBytes+1)...)
	wrongKeyReceipt := fixture.receipt
	wrongKeyReceipt.KeyID = "other-key-v1"
	wrongPhaseReceipt := fixture.receipt
	wrongPhaseReceipt.Phase = "after-ack"
	malformedNonceReceipt := fixture.receipt
	malformedNonceReceipt.ReceiptNonce = "not-a-valid-nonce"
	badSignature := fixture.receipt
	badSignature.Signature = strings.Repeat("A", len(badSignature.Signature))
	rootFromOtherKey, err := handoff.NewTrustRoot(fixture.root.KeyID(), ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x55}, ed25519.SeedSize)).Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal("construct wrong test root")
	}
	cases := []struct {
		name     string
		snapshot []byte
		phase    string
		source   string
		data     []byte
		root     handoff.TrustRoot
	}{
		{name: "missing receipt", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", root: fixture.root},
		{name: "oversized receipt", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: oversized, root: fixture.root},
		{name: "malformed receipt", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: []byte("{"), root: fixture.root},
		{name: "truncated receipt", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: valid[:len(valid)-1], root: fixture.root},
		{name: "duplicate key", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: bytes.Replace(valid, []byte("{"), []byte(`{"version":1,`), 1), root: fixture.root},
		{name: "casefold duplicate", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: bytes.Replace(valid, []byte("{"), []byte(`{"VERSION":1,`), 1), root: fixture.root},
		{name: "unknown credential field", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: bytes.Replace(valid, []byte("{"), []byte(`{"credential":"synthetic-secret",`), 1), root: fixture.root},
		{name: "trailing value", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: append(bytes.Clone(valid), []byte(` {}`)...), root: fixture.root},
		{name: "wrong root", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: valid, root: rootFromOtherKey},
		{name: "wrong key ID", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: fixture.encodeReceipt(t, wrongKeyReceipt, true), root: fixture.root},
		{name: "wrong receipt phase", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: fixture.encodeReceipt(t, wrongPhaseReceipt, true), root: fixture.root},
		{name: "malformed receipt nonce", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: fixture.encodeReceipt(t, malformedNonceReceipt, true), root: fixture.root},
		{name: "tampered signature", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: fixture.encodeReceipt(t, badSignature, false), root: fixture.root},
		{name: "expired receipt", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: fixture.encodeReceipt(t, func() handoff.Receipt {
			r := fixture.receipt
			r.IssuedAt = fixture.now.Add(-2 * time.Hour)
			r.ExpiresAt = fixture.now.Add(-time.Hour)
			return r
		}(), true), root: fixture.root},
		{name: "future receipt", snapshot: fixture.snapshot, phase: "before-ack", source: "signed-fixture-v1", data: fixture.encodeReceipt(t, func() handoff.Receipt { r := fixture.receipt; r.IssuedAt = fixture.now.Add(31 * time.Second); return r }(), true), root: fixture.root},
		{name: "wrong source", snapshot: fixture.snapshot, phase: "before-ack", source: "other-fixture", data: valid, root: fixture.root},
		{name: "unapproved phase", snapshot: fixture.snapshot, phase: "cleanup", source: "signed-fixture-v1", data: valid, root: fixture.root},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireHandoffReaderUntouched(t, tc.snapshot, tc.phase, tc.source, tc.data, tc.root, fixture.now)
		})
	}

	requestMutations := []struct {
		name   string
		mutate func(*Approval)
	}{
		{name: "repository", mutate: func(a *Approval) { a.Organization = "other-org" }},
		{name: "workflow run", mutate: func(a *Approval) { a.WorkflowRunID++ }},
		{name: "workflow ref", mutate: func(a *Approval) { a.WorkflowRef = "refs/heads/other" }},
		{name: "workflow SHA", mutate: func(a *Approval) { a.WorkflowSHA = strings.Repeat("e", 40) }},
		{name: "workflow path", mutate: func(a *Approval) { a.WorkflowPath = ".github/workflows/other.yml" }},
		{name: "controller", mutate: func(a *Approval) { a.Controller = "other-controller" }},
		{name: "owner nonce", mutate: func(a *Approval) { a.OwnerNonce = strings.Repeat("e", 32) }},
		{name: "approval expiry", mutate: func(a *Approval) { a.ExpiresAt = a.ExpiresAt.Add(time.Minute) }},
	}
	for _, tc := range requestMutations {
		t.Run(tc.name, func(t *testing.T) {
			modifiedApproval := fixture.approval
			tc.mutate(&modifiedApproval)
			modifiedSnapshot := controllerHandoffSnapshot(t, modifiedApproval)
			requireHandoffReaderUntouched(t, modifiedSnapshot, "before-ack", "signed-fixture-v1", valid, fixture.root, fixture.now)
		})
	}
	whitespaceSnapshot := append([]byte("\n"), fixture.snapshot...)
	requireHandoffReaderUntouched(t, whitespaceSnapshot, "before-ack", "signed-fixture-v1", valid, fixture.root, fixture.now)
	for _, name := range []string{"empty ref", "missing run"} {
		t.Run(name, func(t *testing.T) {
			modifiedApproval := fixture.approval
			if name == "empty ref" {
				modifiedApproval.WorkflowRef = ""
			} else {
				modifiedApproval.WorkflowRunID = 0
			}
			var reads int
			_, err := consumeControllerHandoff(context.Background(), nil, controllerHandoffSnapshot(t, modifiedApproval), "before-ack", "signed-fixture-v1", bytes.NewReader(valid), fixture.root, func() time.Time { return fixture.now }, func(context.Context) ([]byte, error) { reads++; return nil, nil })
			if err == nil || reads != 0 {
				t.Fatal("request lacking run/ref crossed the credential boundary")
			}
		})
	}
}

func TestControllerHandoffFsyncsBeforeReadAndConsumesNonceGlobally(t *testing.T) {
	fixture := newControllerHandoffFixture(t, "before-ack")
	directory := privateDir(t)
	journal, err := openTestJournal(t, directory, fixture.approval)
	if err != nil {
		t.Fatal("open synthetic journal")
	}
	var syncs int
	journal.syncFile = func(file *os.File) error {
		syncs++
		return file.Sync()
	}
	credentials, err := consumeControllerHandoff(context.Background(), journal, fixture.snapshot, "before-ack", "signed-fixture-v1", bytes.NewReader(fixture.receiptBytes), fixture.root, func() time.Time { return fixture.now }, func(context.Context) ([]byte, error) {
		if syncs != 1 {
			t.Fatalf("credential reader ran before receipt fsync: syncs=%d", syncs)
		}
		events := journal.Events()
		if len(events) != 1 || events[0].Kind != "controller-handoff-consumed" || events[0].ControllerHandoff == nil {
			t.Fatalf("credential reader ran before durable event: %+v", events)
		}
		consumption := events[0].ControllerHandoff
		receiptHash := sha256.Sum256(fixture.receiptBytes)
		if consumption.ReceiptNonce != fixture.receipt.ReceiptNonce || consumption.Phase != "before-ack" || consumption.ApprovalSHA256 != fixture.request.ControllerApprovalSHA256 || consumption.ReceiptSHA256 != hex.EncodeToString(receiptHash[:]) || consumption.KeyID != fixture.root.KeyID() {
			t.Fatalf("durable event does not bind exact non-secret inputs: %+v", consumption)
		}
		return []byte("synthetic-credential"), nil
	})
	if err != nil || string(credentials) != "synthetic-credential" || syncs != 1 {
		t.Fatalf("valid fixture handoff failed: credentials=%q syncs=%d err=%v", credentials, syncs, err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal("close first journal")
	}
	reopened, err := openTestJournal(t, directory, fixture.approval)
	if err != nil {
		t.Fatalf("reopen durable consumption: %v", err)
	}
	defer reopened.Close()
	reads := 0
	reader := func(context.Context) ([]byte, error) { reads++; return []byte("synthetic-credential"), nil }
	if _, err := consumeControllerHandoff(context.Background(), reopened, fixture.snapshot, "before-ack", "signed-fixture-v1", bytes.NewReader(fixture.receiptBytes), fixture.root, func() time.Time { return fixture.now }, reader); err == nil || reads != 0 {
		t.Fatal("same nonce replay crossed reader after reopen")
	}
	otherPhaseRequest := fixture.request
	otherPhaseRequest.Phase = "after-ack"
	otherPhaseReceipt := signedControllerReceipt(t, otherPhaseRequest, fixture.now, fixture.receipt.ReceiptNonce, fixture.privateKey)
	otherPhaseBytes, _ := json.Marshal(otherPhaseReceipt)
	if _, err := consumeControllerHandoff(context.Background(), reopened, fixture.snapshot, "after-ack", "signed-fixture-v1", bytes.NewReader(otherPhaseBytes), fixture.root, func() time.Time { return fixture.now }, reader); err == nil || reads != 0 {
		t.Fatal("same nonce under a different phase crossed reader after reopen")
	}
	if got := reopened.Events(); len(got) != 1 {
		t.Fatalf("failed replays changed durable journal: events=%d", len(got))
	}
	freshNonceReceipt := signedControllerReceipt(t, fixture.request, fixture.now, strings.Repeat("e", 32), fixture.privateKey)
	freshNonceBytes, err := json.Marshal(freshNonceReceipt)
	if err != nil {
		t.Fatal("marshal fresh same-phase receipt")
	}
	if got, err := consumeControllerHandoff(context.Background(), reopened, fixture.snapshot, "before-ack", "signed-fixture-v1", bytes.NewReader(freshNonceBytes), fixture.root, func() time.Time { return fixture.now }, reader); err != nil || string(got) != "synthetic-credential" || reads != 1 {
		t.Fatalf("fresh nonce in an already-consumed phase was incorrectly claimed exactly once: credentials=%q reads=%d err=%v", got, reads, err)
	}
	_ = credentials
}

func TestControllerHandoffRejectsSecondOpenerAndParallelSameHandle(t *testing.T) {
	fixture := newControllerHandoffFixture(t, "before-ack")
	directory := privateDir(t)
	journal, err := openTestJournal(t, directory, fixture.approval)
	if err != nil {
		t.Fatal("open synthetic journal")
	}
	defer journal.Close()
	if second, err := openTestJournal(t, directory, fixture.approval); err == nil {
		second.Close()
		t.Fatal("second journal opener bypassed active admission claim")
	}
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	var readerCalls int
	var mu sync.Mutex
	go func() {
		_, err := consumeControllerHandoff(context.Background(), journal, fixture.snapshot, "before-ack", "signed-fixture-v1", bytes.NewReader(fixture.receiptBytes), fixture.root, func() time.Time { return fixture.now }, func(context.Context) ([]byte, error) {
			mu.Lock()
			readerCalls++
			mu.Unlock()
			close(started)
			<-release
			return []byte("synthetic-credential"), nil
		})
		firstDone <- err
	}()
	<-started
	var secondReads int
	if _, err := consumeControllerHandoff(context.Background(), journal, fixture.snapshot, "before-ack", "signed-fixture-v1", bytes.NewReader(fixture.receiptBytes), fixture.root, func() time.Time { return fixture.now }, func(context.Context) ([]byte, error) {
		secondReads++
		return nil, nil
	}); err == nil || secondReads != 0 {
		close(release)
		t.Fatal("parallel same-handle consumer was not rejected")
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first receipt consumer failed: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if readerCalls != 1 {
		t.Fatalf("same-handle attempts reached reader %d times", readerCalls)
	}
}

func TestControllerHandoffCancellationAndExpirySuppressReaderResults(t *testing.T) {
	for _, testCase := range []string{"cancel before reader", "receipt expires before reader", "approval expires during reader", "cancel during reader"} {
		t.Run(testCase, func(t *testing.T) {
			fixture := newControllerHandoffFixture(t, "before-ack")
			directory := privateDir(t)
			journal, err := openTestJournal(t, directory, fixture.approval)
			if err != nil {
				t.Fatal("open synthetic journal")
			}
			defer journal.Close()
			currentTime := fixture.now
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			readerCalls := 0
			secretBuffer := []byte("synthetic-credential")
			if testCase == "cancel before reader" || testCase == "receipt expires before reader" {
				journal.syncFile = func(file *os.File) error {
					if testCase == "cancel before reader" {
						cancel()
					} else {
						currentTime = fixture.receipt.ExpiresAt
					}
					return file.Sync()
				}
			}
			read := func(context.Context) ([]byte, error) {
				readerCalls++
				if testCase == "approval expires during reader" {
					currentTime = fixture.approval.ExpiresAt
				}
				if testCase == "cancel during reader" {
					cancel()
				}
				return secretBuffer, nil
			}
			credentials, err := consumeControllerHandoff(ctx, journal, fixture.snapshot, "before-ack", "signed-fixture-v1", bytes.NewReader(fixture.receiptBytes), fixture.root, func() time.Time { return currentTime }, read)
			if testCase == "cancel before reader" || testCase == "receipt expires before reader" {
				if err == nil || readerCalls != 0 {
					t.Fatalf("authorization ended before reader but read proceeded: calls=%d err=%v", readerCalls, err)
				}
				return
			}
			if err == nil || credentials != nil || readerCalls != 1 || !bytes.Equal(secretBuffer, make([]byte, len(secretBuffer))) {
				t.Fatalf("expired/cancelled read result escaped: credentials=%q calls=%d buffer=%q err=%v", credentials, readerCalls, secretBuffer, err)
			}
		})
	}
}

func TestControllerHandoffJournalFailuresDenyBeforeReader(t *testing.T) {
	for _, failure := range []string{"sync", "partial write", "unowned journal", "replaced journal", "replaced claim", "baseline mode"} {
		t.Run(failure, func(t *testing.T) {
			fixture := newControllerHandoffFixture(t, "before-ack")
			var journal *FileJournal
			var err error
			if failure == "baseline mode" {
				baselineApproval := controllerHandoffApprovalAt(time.Now().UTC())
				baseline := newBaselineFixtureWithApproval(t, nil, "", baselineApproval)
				if err := baseline.run(t); err != nil {
					t.Fatalf("prepare baseline-mode journal: %v", err)
				}
				baseline.release()
				baseline.release = nil
				fixture = controllerHandoffFixtureForApproval(t, baseline.a, "before-ack", time.Now().UTC())
				journal = baseline.j
			} else {
				journal, err = openTestJournal(t, privateDir(t), fixture.approval)
			}
			if err != nil {
				t.Fatal("open synthetic journal")
			}
			defer journal.Close()
			if failure == "sync" {
				journal.syncFile = func(*os.File) error { return errors.New("synthetic sync failure") }
			}
			if failure == "partial write" {
				journal.writeFile = func(file *os.File, data []byte) (int, error) { return file.Write(data[:len(data)/2]) }
			}
			if failure == "unowned journal" {
				if err := os.Chmod(journal.file.Name(), 0644); err != nil {
					t.Fatal("change synthetic journal mode")
				}
			}
			if failure == "replaced journal" {
				path := journal.directory + "/journal.jsonl"
				if err := os.Rename(path, path+".held"); err != nil {
					t.Fatal("replace test journal")
				}
				if err := os.WriteFile(path, []byte("replacement\n"), 0600); err != nil {
					t.Fatal("create replacement journal")
				}
			}
			if failure == "replaced claim" {
				path := journal.claim.directory + "/admission.json"
				if err := os.Rename(path, path+".held"); err != nil {
					t.Fatal("replace test admission claim")
				}
				if err := os.WriteFile(path, []byte("replacement\n"), 0600); err != nil {
					t.Fatal("create replacement admission claim")
				}
			}
			reads := 0
			_, err = consumeControllerHandoff(context.Background(), journal, fixture.snapshot, "before-ack", "signed-fixture-v1", bytes.NewReader(fixture.receiptBytes), fixture.root, func() time.Time { return fixture.now }, func(context.Context) ([]byte, error) {
				reads++
				return []byte("synthetic-credential"), nil
			})
			if err == nil || reads != 0 {
				t.Fatalf("journal %s crossed credential boundary: reads=%d err=%v", failure, reads, err)
			}
			if failure == "partial write" {
				if err := journal.Close(); err != nil {
					t.Fatal("close partial-write journal")
				}
				if reopened, err := openTestJournal(t, journal.directory, fixture.approval); err == nil {
					reopened.Close()
					t.Fatal("torn handoff record was repaired or accepted")
				}
			}
			if failure == "sync" {
				if err := journal.Close(); err != nil {
					t.Fatal("close sync-failure journal")
				}
				reopened, err := openTestJournal(t, journal.directory, fixture.approval)
				if err != nil {
					t.Fatalf("reopen sync-failure journal without rollback: %v", err)
				}
				defer reopened.Close()
				if _, err := consumeControllerHandoff(context.Background(), reopened, fixture.snapshot, "before-ack", "signed-fixture-v1", bytes.NewReader(fixture.receiptBytes), fixture.root, func() time.Time { return fixture.now }, func(context.Context) ([]byte, error) {
					reads++
					return nil, nil
				}); err == nil || reads != 0 {
					t.Fatal("sync-failed nonce was rolled back across reopen")
				}
			}
		})
	}
}
