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
	mu      sync.Mutex
}

func newSignedBrokerFixtureAdapter(t *testing.T) *signedBrokerFixtureAdapter {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal("fixture provenance key")
	}
	return &signedBrokerFixtureAdapter{private: private, public: public}
}

func (a *signedBrokerFixtureAdapter) Source() string { return brokerFixtureProvenanceSource }

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
		WorkflowRunID:            request.WorkflowRunID,
		WorkflowRef:              request.WorkflowRef,
		WorkflowSHA:              request.WorkflowSHA,
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

func (a *signedBrokerFixtureAdapter) Verify(request BrokerProvenanceRequest, receipt BrokerProvenanceReceipt) error {
	if receipt.Validate(request, time.Now()) != nil {
		return errBroker
	}
	signature, err := base64.RawURLEncoding.DecodeString(receipt.Signature)
	if err != nil || !ed25519.Verify(a.public, receipt.SigningBytes(), signature) {
		return errBroker
	}
	return nil
}

func mutateBrokerProvenanceReceipt(receipt BrokerProvenanceReceipt, kind string) BrokerProvenanceReceipt {
	mutated := receipt
	switch kind {
	case "approval digest":
		mutated.ControllerApprovalSHA256 = strings.Repeat("b", 64)
	case "workflow run":
		mutated.WorkflowRunID++
	case "workflow ref":
		mutated.WorkflowRef = "refs/heads/attacker"
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
		WorkflowRunID:            7,
		WorkflowRef:              "refs/heads/main",
		WorkflowSHA:              strings.Repeat("b", 40),
		Phase:                    "before-ack",
		OwnerNonce:               strings.Repeat("c", 32),
		Source:                   adapter.Source(),
	}
	receipt, err := adapter.Attest(context.Background(), request)
	if err != nil || receipt.Validate(request, time.Now()) != nil || adapter.Verify(request, receipt) != nil {
		t.Fatal("signed fixture receipt was not accepted")
	}
	for _, kind := range []string{"approval digest", "workflow run", "workflow ref", "workflow commit", "phase", "owner nonce", "source", "signature"} {
		t.Run(kind, func(t *testing.T) {
			forged := mutateBrokerProvenanceReceipt(receipt, kind)
			if adapter.Verify(request, forged) == nil {
				t.Fatalf("forged %s receipt accepted", kind)
			}
		})
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
