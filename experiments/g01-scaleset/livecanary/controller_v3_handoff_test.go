package livecanary

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

type controllerV3HandoffFixture struct {
	approval    Approval
	snapshot    []byte
	request     handoff.Request
	workflow    handoff.TrustRoot
	issuer      handoff.IssuerRoot
	workflowKey ed25519.PrivateKey
	receipt     handoff.Receipt
	receiptRaw  []byte
	bindingRaw  []byte
	payload     []byte
	policyRaw   []byte
	stateDir    string
	files       PairedTerminalFiles
}

func newControllerV3HandoffFixture(t *testing.T, phase string) controllerV3HandoffFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	a := controllerHandoffApprovalAt(now)
	a.Phases = []string{"create"}
	if phase != "create" {
		a.Phases = append(a.Phases, phase)
	}
	a.GroupPolicy = controllerGroupPolicy()
	a.GroupPolicy.Workflow = a.Organization + "/" + a.Repository + "/.github/workflows/runner.yml@" + strings.Repeat("d", 40)
	snapshot, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	_, caller, err := controllerHandoffRequest(snapshot, phase, "github-rest-workflow-v1", now)
	if err != nil {
		t.Fatal(err)
	}
	request := caller
	request.Version = 2
	request.Source = handoff.ReusableCalleeSource
	request.Callee = handoff.CalleeIdentity{Repository: request.Repository, WorkflowPath: ".github/workflows/runner.yml", SHA: strings.Repeat("d", 40)}
	grant := handoff.PhaseGrant{Request: request, Mode: "controller", OrganizationID: a.GroupPolicy.OrganizationID, RepositoryID: a.RepositoryID, ExpiresAt: a.ExpiresAt}
	grant.GroupPolicySHA256 = a.GroupPolicy.GrantDigest(request, a.RepositoryID, int64(a.RunnerGroupID), a.ManagementScope)
	policy := handoff.PublicPolicy{
		Version:           3,
		Role:              "controller",
		Source:            handoff.ReusableCalleeSource,
		WorkflowKeyID:     "workflow-v1",
		WorkflowPublicKey: "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8",
		IssuerKeyID:       "issuer-v1",
		IssuerPublicKey:   "e28a8970753332bd72fef413e6b0b2ef1b4aadda7aa2c141f233712a6876b351",
		Grants:            []handoff.PhaseGrant{grant},
	}
	policyRaw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	workflowKey := ed25519.NewKeyFromSeed(func() []byte {
		seed := make([]byte, ed25519.SeedSize)
		for i := range seed {
			seed[i] = byte(i)
		}
		return seed
	}())
	issuerKey := ed25519.NewKeyFromSeed(stringsToBytes(71, ed25519.SeedSize))
	workflow, err := handoff.NewTrustRoot(policy.WorkflowKeyID, workflowKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := handoff.NewIssuerRoot(policy.IssuerKeyID, issuerKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	nonce := "f"
	if phase == "create" {
		nonce = "e"
	}
	receipt := handoff.Receipt{
		Version:                  2,
		Algorithm:                "ed25519",
		KeyID:                    policy.WorkflowKeyID,
		ControllerApprovalSHA256: request.ControllerApprovalSHA256,
		Repository:               request.Repository,
		WorkflowRunID:            request.WorkflowRunID,
		WorkflowRef:              request.WorkflowRef,
		WorkflowSHA:              request.WorkflowSHA,
		WorkflowPath:             request.WorkflowPath,
		Phase:                    request.Phase,
		OwnerNonce:               request.OwnerNonce,
		ReceiptNonce:             strings.Repeat(nonce, 32),
		Source:                   request.Source,
		Callee:                   request.Callee,
		IssuedAt:                 now,
		ExpiresAt:                now.Add(time.Hour),
	}
	receiptRaw := signV3Receipt(t, receipt, workflowKey)
	payload := handoffCredentialBytes(t, a)
	bindingRaw := signV3Binding(t, request, receipt, receiptRaw, payload, phase, issuerKey, issuer)
	directory := privateDir(t)
	stateDir := filepath.Join(directory, "state")
	if err := os.Mkdir(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	approvalPath := filepath.Join(directory, "approval.json")
	if err := os.WriteFile(approvalPath, snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	return controllerV3HandoffFixture{
		approval: a, snapshot: snapshot, request: request,
		workflow: workflow, issuer: issuer, workflowKey: workflowKey,
		receipt: receipt, receiptRaw: receiptRaw, bindingRaw: bindingRaw, payload: payload,
		policyRaw: policyRaw, stateDir: stateDir,
		files: PairedTerminalFiles{ControllerApprovalPath: approvalPath, ControllerStateDirectory: stateDir},
	}
}

func signV3Receipt(t *testing.T, receipt handoff.Receipt, key ed25519.PrivateKey) []byte {
	t.Helper()
	receipt.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, receipt.SigningBytes()))
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func signV3Binding(t *testing.T, request handoff.Request, receipt handoff.Receipt, receiptRaw, payload []byte, phase string, key ed25519.PrivateKey, issuer handoff.IssuerRoot) []byte {
	t.Helper()
	claims := handoff.BindingClaims{
		Request:       request,
		Version:       1,
		Kind:          handoff.BindingKind,
		Algorithm:     "ed25519",
		Audience:      handoff.BindingAudience,
		KeyID:         issuer.KeyID(),
		IssuerSource:  handoff.IssuerSource,
		WorkflowKeyID: receipt.KeyID,
		ReceiptSHA256: handoff.Digest(receiptRaw),
		PayloadSHA256: handoff.Digest(payload),
		PayloadLength: len(payload),
		ReceiptNonce:  receipt.ReceiptNonce,
		IssuedAt:      receipt.IssuedAt,
		ExpiresAt:     receipt.IssuedAt.Add(20 * time.Second),
	}
	if phase == "create" {
		binding := handoff.CredentialBinding{BindingClaims: claims}
		binding.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, binding.SigningBytes()))
		raw, err := json.Marshal(binding)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	binding := handoff.PhaseBinding{BindingClaims: claims, Mode: "controller"}
	binding.Version = 2
	binding.Kind = handoff.PhaseBindingKind
	binding.Audience = handoff.PhaseAudience("controller", phase)
	binding.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, binding.SigningBytes()))
	raw, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func v3Pipes(t *testing.T, receiptRaw, bindingRaw, delivered []byte) (*os.File, *os.File) {
	t.Helper()
	frame, err := handoff.EncodeControl(receiptRaw, bindingRaw)
	if err != nil {
		t.Fatal(err)
	}
	control, controlWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	input, inputWriter, err := os.Pipe()
	if err != nil {
		_ = control.Close()
		_ = controlWriter.Close()
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = controlWriter.Write(frame)
		_ = controlWriter.Close()
		_, _ = inputWriter.Write(delivered)
		_ = inputWriter.Close()
	}()
	t.Cleanup(func() {
		_ = control.Close()
		_ = input.Close()
		<-done
	})
	return control, input
}

func installV3ControllerFixture(t *testing.T, fixture controllerV3HandoffFixture) *fakeAPI {
	t.Helper()
	oldPolicy, oldOpen, oldAPI := controllerExperimentPolicyJSON, controllerExperimentFixtureOpen, controllerExperimentFixtureAPI
	t.Cleanup(func() {
		controllerExperimentPolicyJSON = oldPolicy
		controllerExperimentFixtureOpen = oldOpen
		controllerExperimentFixtureAPI = oldAPI
	})
	controllerExperimentPolicyJSON = string(fixture.policyRaw)
	controllerExperimentFixtureOpen = func(path string, approval Approval) (*FileJournal, error) {
		return openTestJournal(t, path, approval)
	}
	fake := &fakeAPI{}
	controllerExperimentFixtureAPI = func(Approval, Credentials) (API, error) {
		return handoffValidInventoryAPI{fake}, nil
	}
	return fake
}

func journalEventsForV3(t *testing.T, fixture controllerV3HandoffFixture) []Event {
	t.Helper()
	j, err := openTestJournal(t, fixture.stateDir, fixture.approval)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	return j.Events()
}

func TestControllerExperimentV3CreateCrossesProcessBoundary(t *testing.T) {
	fixture := newControllerV3HandoffFixture(t, "create")
	control, input := v3Pipes(t, fixture.receiptRaw, fixture.bindingRaw, fixture.payload)
	fake := installV3ControllerFixture(t, fixture)
	if err := RunControllerExperimentWithInputs(context.Background(), fixture.files, "controller", "create", control, input); err != nil {
		t.Fatalf("v3 selected request did not cross process/handoff boundary: %v", err)
	}
	if fake.createCalls != 1 {
		t.Fatalf("selected request reached %d create calls, want 1", fake.createCalls)
	}
}

func TestControllerExperimentV3RejectsReceiptCalleeMismatch(t *testing.T) {
	fixture := newControllerV3HandoffFixture(t, "create")
	badReceipt := fixture.receipt
	badReceipt.Callee.SHA = strings.Repeat("f", 40)
	badReceiptRaw := signV3Receipt(t, badReceipt, fixture.workflowKey)
	control, input := v3Pipes(t, badReceiptRaw, fixture.bindingRaw, fixture.payload)
	fake := installV3ControllerFixture(t, fixture)
	if err := RunControllerExperimentWithInputs(context.Background(), fixture.files, "controller", "create", control, input); err == nil {
		t.Fatal("receipt with a callee different from selected policy was accepted")
	}
	if fake.createCalls != 0 || len(journalEventsForV3(t, fixture)) != 0 {
		t.Fatalf("receipt mismatch crossed effect boundary: creates=%d", fake.createCalls)
	}
}

func TestControllerExperimentV3RejectsPayloadSubstitution(t *testing.T) {
	fixture := newControllerV3HandoffFixture(t, "create")
	substitute := append([]byte(nil), fixture.payload...)
	substitute[len(substitute)-2] ^= 1
	control, input := v3Pipes(t, fixture.receiptRaw, fixture.bindingRaw, substitute)
	fake := installV3ControllerFixture(t, fixture)
	if err := RunControllerExperimentWithInputs(context.Background(), fixture.files, "controller", "create", control, input); err == nil {
		t.Fatal("payload substitution was accepted")
	}
	events := journalEventsForV3(t, fixture)
	if fake.createCalls != 0 || len(events) != 1 || events[0].ControllerHandoff == nil {
		t.Fatalf("payload substitution crossed factory/effect boundary: creates=%d events=%d", fake.createCalls, len(events))
	}
}

func TestControllerProcessHandoffV3ConcurrentOnce(t *testing.T) {
	fixture := newControllerV3HandoffFixture(t, "create")
	j, err := openTestJournal(t, fixture.stateDir, fixture.approval)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	controls := make([]*os.File, 2)
	inputs := make([]*os.File, 2)
	for i := range controls {
		controls[i], inputs[i] = v3Pipes(t, fixture.receiptRaw, fixture.bindingRaw, fixture.payload)
	}
	var factories atomic.Int32
	start := make(chan struct{})
	results := make(chan error, len(controls))
	for i := range controls {
		go func(i int) {
			<-start
			results <- runControllerProcessHandoffCreateExpected(context.Background(), j, fixture.snapshot, controls[i], inputs[i], handoff.ReusableCalleeSource, &fixture.request, fixture.workflow, fixture.issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) {
				factories.Add(1)
				return handoffValidInventoryAPI{&fakeAPI{}}, nil
			})
		}(i)
	}
	close(start)
	var successes int
	for range controls {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 || factories.Load() != 1 || len(j.Events()) != 7 {
		t.Fatalf("v3 concurrent handoff crossed singleton fence: successes=%d factories=%d events=%d", successes, factories.Load(), len(j.Events()))
	}
}

func TestControllerProcessHandoffV3PhaseBinding(t *testing.T) {
	fixture := newControllerV3HandoffFixture(t, "inspect")
	j, err := openTestJournal(t, fixture.stateDir, fixture.approval)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	_, fake := completedHandoffCreate(t, j, fixture.approval)
	control, input := v3Pipes(t, fixture.receiptRaw, fixture.bindingRaw, fixture.payload)
	factories := 0
	err = runControllerProcessHandoffPhaseExpected(context.Background(), j, fixture.snapshot, control, input, "controller", "inspect", nil, handoff.ReusableCalleeSource, &fixture.request, fixture.workflow, fixture.issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) {
		factories++
		return completedCreateAPI{fake}, nil
	})
	if err != nil || factories != 1 {
		t.Fatalf("v3 phase binding was not accepted at process boundary: err=%v factories=%d", err, factories)
	}
}

func stringsToBytes(value byte, length int) []byte {
	data := make([]byte, length)
	for i := range data {
		data[i] = value
	}
	return data
}
