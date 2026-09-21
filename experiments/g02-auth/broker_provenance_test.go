package enrollment

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const brokerFixtureProvenanceSource = "signed-fixture-v1"

type signedBrokerFixtureAdapter struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	root    BrokerProvenanceTrustRoot
	mu      sync.Mutex
}

func newSignedBrokerFixtureAdapter(t *testing.T) *signedBrokerFixtureAdapter {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal("fixture provenance key")
	}
	root, err := NewBrokerProvenanceTrustRoot("fixture-ed25519-v1", public)
	if err != nil {
		t.Fatal("fixture provenance root")
	}
	return &signedBrokerFixtureAdapter{private: private, public: public, root: root}
}

func (a *signedBrokerFixtureAdapter) Source() string                       { return brokerFixtureProvenanceSource }
func (a *signedBrokerFixtureAdapter) TrustRoot() BrokerProvenanceTrustRoot { return a.root }

func (a *signedBrokerFixtureAdapter) Attest(ctx context.Context, request BrokerProvenanceRequest) (BrokerProvenanceReceipt, error) {
	if ctx == nil || ctx.Err() != nil {
		return BrokerProvenanceReceipt{}, errBroker
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	seed, _ := json.Marshal(request)
	digest := sha256.Sum256(seed)
	issued := time.Now().UTC()
	receipt := BrokerProvenanceReceipt{
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
		ReceiptNonce:             hex.EncodeToString(digest[:16]),
		Source:                   request.Source,
		IssuedAt:                 issued,
		ExpiresAt:                issued.Add(2 * time.Hour),
	}
	receipt.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(a.private, receipt.SigningBytes()))
	return receipt, nil
}

func (a *signedBrokerFixtureAdapter) Verify(ctx context.Context, request BrokerProvenanceRequest, receipt BrokerProvenanceReceipt) error {
	if ctx == nil || ctx.Err() != nil || receipt.Validate(request, time.Now()) != nil {
		return errBroker
	}
	signature, err := base64.RawURLEncoding.DecodeString(receipt.Signature)
	if err != nil || !ed25519.Verify(a.public, receipt.SigningBytes(), signature) {
		return errBroker
	}
	return nil
}

type blockingBrokerProvenanceAdapter struct {
	fixture       *signedBrokerFixtureAdapter
	attestStarted chan context.Context
	verifyStarted chan context.Context
	waitAttest    bool
	waitVerify    bool
}

func (a *blockingBrokerProvenanceAdapter) Source() string { return a.fixture.Source() }
func (a *blockingBrokerProvenanceAdapter) TrustRoot() BrokerProvenanceTrustRoot {
	return a.fixture.TrustRoot()
}

func (a *blockingBrokerProvenanceAdapter) Attest(ctx context.Context, request BrokerProvenanceRequest) (BrokerProvenanceReceipt, error) {
	a.attestStarted <- ctx
	if a.waitAttest {
		<-ctx.Done()
		return BrokerProvenanceReceipt{}, ctx.Err()
	}
	return a.fixture.Attest(ctx, request)
}

func (a *blockingBrokerProvenanceAdapter) Verify(ctx context.Context, request BrokerProvenanceRequest, receipt BrokerProvenanceReceipt) error {
	a.verifyStarted <- ctx
	if a.waitVerify {
		<-ctx.Done()
		return ctx.Err()
	}
	return a.fixture.Verify(ctx, request, receipt)
}

type attemptClaimProbeAdapter struct {
	fixture     *signedBrokerFixtureAdapter
	attemptPath string
	attestHeld  bool
	verifyHeld  bool
}

func (a *attemptClaimProbeAdapter) Source() string { return a.fixture.Source() }
func (a *attemptClaimProbeAdapter) TrustRoot() BrokerProvenanceTrustRoot {
	return a.fixture.TrustRoot()
}

func (a *attemptClaimProbeAdapter) claimHeld() bool {
	file, err := os.OpenFile(filepath.Join(a.attemptPath, "broker.jsonl"), os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer file.Close()
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil
}

func (a *attemptClaimProbeAdapter) Attest(ctx context.Context, request BrokerProvenanceRequest) (BrokerProvenanceReceipt, error) {
	a.attestHeld = a.claimHeld()
	if !a.attestHeld {
		return BrokerProvenanceReceipt{}, errBroker
	}
	return a.fixture.Attest(ctx, request)
}

func (a *attemptClaimProbeAdapter) Verify(ctx context.Context, request BrokerProvenanceRequest, receipt BrokerProvenanceReceipt) error {
	a.verifyHeld = a.claimHeld()
	if !a.verifyHeld {
		return errBroker
	}
	return a.fixture.Verify(ctx, request, receipt)
}

type rootlessBrokerProvenanceAdapter struct {
	fixture *signedBrokerFixtureAdapter
	called  bool
}

func (a *rootlessBrokerProvenanceAdapter) Source() string { return a.fixture.Source() }
func (a *rootlessBrokerProvenanceAdapter) TrustRoot() BrokerProvenanceTrustRoot {
	return BrokerProvenanceTrustRoot{}
}
func (a *rootlessBrokerProvenanceAdapter) Attest(ctx context.Context, request BrokerProvenanceRequest) (BrokerProvenanceReceipt, error) {
	a.called = true
	return a.fixture.Attest(ctx, request)
}
func (a *rootlessBrokerProvenanceAdapter) Verify(ctx context.Context, request BrokerProvenanceRequest, receipt BrokerProvenanceReceipt) error {
	a.called = true
	return a.fixture.Verify(ctx, request, receipt)
}

func brokerProvenanceDeadlineInputs(now time.Time) (BrokerApproval, controllerApproval) {
	a := brokerApprovalFixture()
	a.Mode, a.Phase = "controller", "create"
	a.ExpiresAt = now.Add(150 * time.Millisecond)
	a.ControllerApprovalSHA256 = strings.Repeat("a", 64)
	c := controllerApproval{
		AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization,
		Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID,
		OwnerNonce: a.OwnerNonce, HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40),
		WorkflowRef: "refs/heads/main", WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7,
		Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"},
		Phases: []string{"create"},
	}
	return a, c
}

func TestBrokerWorkflowReceiptBoundsAttestToApprovalDeadline(t *testing.T) {
	now := time.Now().UTC()
	a, c := brokerProvenanceDeadlineInputs(now)
	adapter := &blockingBrokerProvenanceAdapter{fixture: newSignedBrokerFixtureAdapter(t), attestStarted: make(chan context.Context, 1), verifyStarted: make(chan context.Context, 1), waitAttest: true}
	api := newBrokerAPI(func() time.Time { return now }, nil)
	api.provenance = adapter

	started := time.Now()
	if _, err := brokerWorkflowReceipt(context.Background(), api, a, c, a.ControllerApprovalSHA256); err == nil {
		t.Fatal("deadline-bound attestation unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("attestation exceeded bounded deadline: %s", elapsed)
	}
	select {
	case callContext := <-adapter.attestStarted:
		deadline, ok := callContext.Deadline()
		if !ok || deadline.After(a.ExpiresAt) || deadline.Sub(a.ExpiresAt) > time.Millisecond {
			t.Fatalf("attestation context deadline = %v, want approval deadline %v", deadline, a.ExpiresAt)
		}
	case <-time.After(time.Second):
		t.Fatal("attestation was not invoked")
	}
}

func TestBrokerWorkflowReceiptBoundsVerifyToApprovalDeadline(t *testing.T) {
	now := time.Now().UTC()
	a, c := brokerProvenanceDeadlineInputs(now)
	adapter := &blockingBrokerProvenanceAdapter{fixture: newSignedBrokerFixtureAdapter(t), attestStarted: make(chan context.Context, 1), verifyStarted: make(chan context.Context, 1), waitVerify: true}
	api := newBrokerAPI(func() time.Time { return now }, nil)
	api.provenance = adapter

	started := time.Now()
	if _, err := brokerWorkflowReceipt(context.Background(), api, a, c, a.ControllerApprovalSHA256); err == nil {
		t.Fatal("deadline-bound verification unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("verification exceeded bounded deadline: %s", elapsed)
	}
	select {
	case callContext := <-adapter.verifyStarted:
		deadline, ok := callContext.Deadline()
		if !ok || deadline.After(a.ExpiresAt) || deadline.Sub(a.ExpiresAt) > time.Millisecond {
			t.Fatalf("verification context deadline = %v, want approval deadline %v", deadline, a.ExpiresAt)
		}
	case <-time.After(time.Second):
		t.Fatal("verification was not invoked")
	}
}

func TestBrokerWorkflowReceiptRejectsCanceledContextBeforeAdapter(t *testing.T) {
	now := time.Now().UTC()
	a, c := brokerProvenanceDeadlineInputs(now)
	adapter := &blockingBrokerProvenanceAdapter{fixture: newSignedBrokerFixtureAdapter(t), attestStarted: make(chan context.Context, 1), verifyStarted: make(chan context.Context, 1)}
	api := newBrokerAPI(func() time.Time { return now }, nil)
	api.provenance = adapter
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := brokerWorkflowReceipt(ctx, api, a, c, a.ControllerApprovalSHA256); err == nil {
		t.Fatal("canceled provenance context unexpectedly succeeded")
	}
	select {
	case <-adapter.attestStarted:
		t.Fatal("canceled context invoked attestation")
	default:
	}
}

func mutateBrokerProvenanceReceipt(receipt BrokerProvenanceReceipt, kind string) BrokerProvenanceReceipt {
	mutated := receipt
	switch kind {
	case "approval digest":
		mutated.ControllerApprovalSHA256 = strings.Repeat("b", 64)
	case "repository":
		mutated.Repository = "fixture-org/attacker"
	case "workflow run":
		mutated.WorkflowRunID++
	case "workflow ref":
		mutated.WorkflowRef = "refs/heads/attacker"
	case "workflow path":
		mutated.WorkflowPath = ".github/workflows/attacker.yml"
	case "workflow commit":
		mutated.WorkflowSHA = strings.Repeat("d", 40)
	case "phase":
		mutated.Phase = "cleanup"
	case "owner nonce":
		mutated.OwnerNonce = strings.Repeat("e", 32)
	case "source":
		mutated.Source = "unsigned-input"
	case "signature":
		mutated.Signature = strings.Repeat("A", len(mutated.Signature))
	}
	return mutated
}

func TestBrokerProvenanceReceiptBindsApprovalWorkflowPhaseNonceAndSource(t *testing.T) {
	adapter := newSignedBrokerFixtureAdapter(t)
	request := BrokerProvenanceRequest{
		ControllerApprovalSHA256: strings.Repeat("a", 64),
		Repository:               "fixture-org/canary",
		WorkflowRunID:            7,
		WorkflowRef:              "refs/heads/main",
		WorkflowSHA:              strings.Repeat("b", 40),
		WorkflowPath:             ".github/workflows/canary.yml",
		Phase:                    "before-ack",
		OwnerNonce:               strings.Repeat("c", 32),
		Source:                   adapter.Source(),
	}
	receipt, err := adapter.Attest(context.Background(), request)
	if err != nil || receipt.Validate(request, time.Now()) != nil || adapter.Verify(context.Background(), request, receipt) != nil {
		t.Fatal("signed fixture receipt was not accepted")
	}
	for _, kind := range []string{"approval digest", "repository", "workflow run", "workflow ref", "workflow path", "workflow commit", "phase", "owner nonce", "source", "signature"} {
		t.Run(kind, func(t *testing.T) {
			forged := mutateBrokerProvenanceReceipt(receipt, kind)
			if adapter.Verify(context.Background(), request, forged) == nil {
				t.Fatalf("forged %s receipt accepted", kind)
			}
		})
	}
}

func TestBrokerProvenanceRequiresExplicitPinnedTrustRoot(t *testing.T) {
	adapter := newSignedBrokerFixtureAdapter(t)
	request := BrokerProvenanceRequest{
		ControllerApprovalSHA256: strings.Repeat("a", 64),
		Repository:               "fixture-org/canary",
		WorkflowRunID:            7,
		WorkflowRef:              "refs/heads/main",
		WorkflowSHA:              strings.Repeat("b", 40),
		WorkflowPath:             ".github/workflows/canary.yml",
		Phase:                    "before-ack",
		OwnerNonce:               strings.Repeat("c", 32),
		Source:                   adapter.Source(),
	}
	receipt, err := adapter.Attest(context.Background(), request)
	if err != nil {
		t.Fatal("signed fixture receipt", err)
	}

	root, err := NewBrokerProvenanceTrustRoot(receipt.KeyID, adapter.public)
	if err != nil {
		t.Fatal("pinned trust root", err)
	}
	if err := root.Verify(request, receipt, time.Now()); err != nil {
		t.Fatalf("valid receipt rejected by pinned root: %v", err)
	}

	for _, name := range []string{"missing key", "wrong key id", "wrong public key"} {
		t.Run(name, func(t *testing.T) {
			candidate := root
			switch name {
			case "missing key":
				candidate, err = NewBrokerProvenanceTrustRoot(receipt.KeyID, nil)
			case "wrong key id":
				candidate, err = NewBrokerProvenanceTrustRoot("other-root", adapter.public)
			case "wrong public key":
				other := newSignedBrokerFixtureAdapter(t)
				candidate, err = NewBrokerProvenanceTrustRoot(receipt.KeyID, other.public)
			}
			if err == nil && candidate.Verify(request, receipt, time.Now()) == nil {
				t.Fatal("receipt accepted without the exact pinned trust root")
			}
		})
	}
}

func TestBrokerWorkflowReceiptRejectsMissingPinnedRootBeforeCredentialInput(t *testing.T) {
	e := newPairedBrokerEntryFixture(t)
	adapter := &rootlessBrokerProvenanceAdapter{fixture: newSignedBrokerFixtureAdapter(t)}
	e.api.provenance = adapter
	fifo := filepath.Join(e.parent, "rootless-input.fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal("FIFO setup")
	}
	input, err := os.OpenFile(fifo, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal("FIFO open")
	}
	defer input.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := runBrokerWithAPI(ctx, e.files, input, e.api)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || adapter.called || e.fixture.tokenCalls != 0 || len(e.fixture.calls) != 0 {
			t.Fatalf("rootless provenance crossed a credential/API boundary: err=%v adapter_called=%t mints=%d calls=%v", err, adapter.called, e.fixture.tokenCalls, e.fixture.calls)
		}
	case <-time.After(300 * time.Millisecond):
		cancel()
		select {
		case <-done:
			t.Fatal("rootless provenance reached the blocking credential read")
		case <-time.After(2 * time.Second):
			t.Fatal("rootless provenance refusal remained blocked")
		}
	}
}

func TestBrokerControllerReceiptRequiresExplicitSourceBeforeInput(t *testing.T) {
	e := newPairedBrokerEntryFixture(t)
	e.api.provenance = nil
	var controller controllerApproval
	if _, err := readBrokerPrivateJSON(e.files.ControllerApproval, &controller); err != nil {
		t.Fatal("controller fixture")
	}
	a, err := brokerApprovalFromEntryFixture(t, e)
	if err != nil {
		t.Fatal("broker fixture approval")
	}
	if _, err := brokerWorkflowReceipt(context.Background(), e.api, a, controller, a.ControllerApprovalSHA256); err == nil {
		t.Fatal("controller receipt was synthesized without an explicit adapter")
	}
}

func brokerApprovalFromEntryFixture(t *testing.T, e pairedBrokerEntryFixture) (BrokerApproval, error) {
	t.Helper()
	var approval BrokerApproval
	_, err := readBrokerPrivateJSON(e.files.ApprovalPath, &approval)
	return approval, err
}

// A controller-shaped broker invocation must not consume controller input or
// reach the API merely because the caller supplied the existing private input
// file. This remains a regression guard for the broker-only provenance gate.
func TestBrokerControllerFrontDoorRequiresBrokerProvenance(t *testing.T) {
	e := newPairedBrokerEntryFixture(t)
	e.api.provenance = nil
	result, err := e.run(t, e.files.ControllerStateDirectory, e.files.WorkerStateDirectory)
	if err == nil || result.Status != "" || e.fixture.tokenCalls != 0 || len(e.fixture.calls) != 0 {
		t.Fatalf("controller front door accepted input without broker provenance: result=%+v err=%v mints=%d calls=%v", result, err, e.fixture.tokenCalls, e.fixture.calls)
	}
}

// A direct FIFO must be rejected before its blocking read. This keeps the
// no-attestation path bounded even when a caller presents a valid-looking
// controller approval and a private named pipe.
func TestBrokerControllerFrontDoorRejectsDirectFIFOBeforeRead(t *testing.T) {
	e := newPairedBrokerEntryFixture(t)
	e.api.provenance = nil
	fifo := filepath.Join(e.parent, "direct-input.fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal("FIFO setup")
	}
	input, err := os.OpenFile(fifo, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal("FIFO open")
	}
	defer input.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := runBrokerWithAPI(ctx, e.files, input, e.api)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || e.fixture.tokenCalls != 0 || len(e.fixture.calls) != 0 {
			t.Fatalf("direct FIFO reached broker/API before provenance refusal: err=%v mints=%d calls=%v", err, e.fixture.tokenCalls, e.fixture.calls)
		}
	case <-time.After(300 * time.Millisecond):
		cancel()
		select {
		case <-done:
			t.Fatal("direct FIFO was read before broker provenance refusal")
		case <-time.After(2 * time.Second):
			t.Fatal("direct FIFO refusal remained blocked")
		}
	}
}

func TestBrokerControllerClaimsAttemptBeforeProvenanceSideEffects(t *testing.T) {
	e := newPairedBrokerEntryFixture(t)
	adapter := &attemptClaimProbeAdapter{fixture: newSignedBrokerFixtureAdapter(t), attemptPath: e.files.StateDirectory}
	e.api.provenance = adapter
	result, err := e.run(t, e.files.ControllerStateDirectory, e.files.WorkerStateDirectory)
	if err != nil || result.Status != "paired_terminal_completed" || !adapter.attestHeld || !adapter.verifyHeld {
		t.Fatalf("provenance adapter ran without a held attempt claim: result=%+v err=%v attest=%t verify=%t", result, err, adapter.attestHeld, adapter.verifyHeld)
	}
}
