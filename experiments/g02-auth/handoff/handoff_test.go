package handoff

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func deterministicFixture(t *testing.T) (Request, Receipt, ed25519.PublicKey, time.Time) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	request := Request{
		ControllerApprovalSHA256: "1343ffdc5440d65d48e2b1916e6aff470f1d11cdfa30aefe54b63293f15ecdf3",
		Repository:               "fixture-org/canary",
		WorkflowRunID:            7,
		WorkflowRef:              "refs/heads/main",
		WorkflowSHA:              strings.Repeat("b", 40),
		WorkflowPath:             ".github/workflows/canary.yml",
		Phase:                    "before-ack",
		OwnerNonce:               strings.Repeat("c", 32),
		Source:                   "signed-fixture-v1",
	}
	receipt := Receipt{
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
		ReceiptNonce:             strings.Repeat("d", 32),
		Source:                   request.Source,
		IssuedAt:                 now,
		ExpiresAt:                now.Add(time.Hour),
	}
	receipt.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, receipt.SigningBytes()))
	return request, receipt, publicKey, now
}

func TestDeterministicV1WireSigningAndPinnedRoot(t *testing.T) {
	request, receipt, publicKey, now := deterministicFixture(t)
	want := []byte(`{"version":1,"algorithm":"ed25519","key_id":"fixture-ed25519-v1","controller_approval_sha256":"1343ffdc5440d65d48e2b1916e6aff470f1d11cdfa30aefe54b63293f15ecdf3","repository":"fixture-org/canary","workflow_run_id":7,"workflow_ref":"refs/heads/main","workflow_sha":"` + strings.Repeat("b", 40) + `","workflow_path":".github/workflows/canary.yml","phase":"before-ack","owner_nonce":"` + strings.Repeat("c", 32) + `","receipt_nonce":"` + strings.Repeat("d", 32) + `","source":"signed-fixture-v1","issued_at":"2026-09-22T12:00:00Z","expires_at":"2026-09-22T13:00:00Z"}`)
	if !bytes.Equal(receipt.SigningBytes(), want) {
		t.Fatalf("v1 signing payload changed: %s", receipt.SigningBytes())
	}
	if !request.Valid() || receipt.Validate(request, now) != nil {
		t.Fatal("deterministic v1 fixture failed validation")
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal("marshal fixture receipt")
	}
	wantReceipt := append(bytes.Clone(want[:len(want)-1]), []byte(`,"signature":"`+receipt.Signature+`"}`)...)
	if !bytes.Equal(encoded, wantReceipt) {
		t.Fatalf("v1 receipt field order/schema changed: %s", encoded)
	}
	decoded, err := DecodeStrictReceipt(encoded)
	if err != nil || !bytes.Equal(decoded.SigningBytes(), want) {
		t.Fatalf("strict decode changed signed payload: err=%v", err)
	}
	root, err := NewTrustRoot(receipt.KeyID, publicKey)
	if err != nil {
		t.Fatal("construct fixture trust root")
	}
	publicKey[0] ^= 0xff
	if root.Verify(request, decoded, now) != nil {
		t.Fatal("copied pinned root failed after caller key mutation")
	}
}

func TestStrictReceiptDecoderRejectsAmbiguousOrExtraInput(t *testing.T) {
	_, receipt, _, _ := deterministicFixture(t)
	valid, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal("marshal fixture receipt")
	}
	tooLarge := append(bytes.Clone(valid), bytes.Repeat([]byte(" "), MaxReceiptBytes+1)...)
	cases := map[string][]byte{
		"missing":               nil,
		"truncated":             valid[:len(valid)-1],
		"malformed":             []byte("{"),
		"duplicate":             bytes.Replace(valid, []byte("{"), []byte(`{"version":1,`), 1),
		"casefold duplicate":    bytes.Replace(valid, []byte("{"), []byte(`{"VERSION":1,`), 1),
		"unknown credential":    bytes.Replace(valid, []byte("{"), []byte(`{"credential":"fixture-secret",`), 1),
		"controller wire field": bytes.Replace(valid, []byte("{"), []byte(`{"controller":"fixture-controller",`), 1),
		"approval expiry field": bytes.Replace(valid, []byte("{"), []byte(`{"approval_expires_at":"2026-09-22T13:00:00Z",`), 1),
		"trailing value":        append(bytes.Clone(valid), []byte(` {}`)...),
		"oversized":             tooLarge,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeStrictReceipt(input); err == nil {
				t.Fatal("invalid receipt input accepted")
			}
		})
	}
	if _, err := DecodeStrictReceipt(append(bytes.Clone(valid), []byte(" \n\t")...)); err != nil {
		t.Fatalf("trailing JSON whitespace rejected: %v", err)
	}
}

func TestPinnedRootRejectsWrongExpectedTupleAndTimeBoundaries(t *testing.T) {
	request, receipt, publicKey, now := deterministicFixture(t)
	root, err := NewTrustRoot(receipt.KeyID, publicKey)
	if err != nil {
		t.Fatal("construct fixture trust root")
	}
	mutations := map[string]func(*Request){
		"approval digest": func(r *Request) { r.ControllerApprovalSHA256 = strings.Repeat("e", 64) },
		"repository":      func(r *Request) { r.Repository = "other-org/canary" },
		"run":             func(r *Request) { r.WorkflowRunID++ },
		"ref":             func(r *Request) { r.WorkflowRef = "refs/heads/other" },
		"sha":             func(r *Request) { r.WorkflowSHA = strings.Repeat("e", 40) },
		"path":            func(r *Request) { r.WorkflowPath = ".github/workflows/other.yml" },
		"phase":           func(r *Request) { r.Phase = "cleanup" },
		"owner nonce":     func(r *Request) { r.OwnerNonce = strings.Repeat("e", 32) },
		"source":          func(r *Request) { r.Source = "untrusted-input" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := request
			mutate(&candidate)
			if root.Verify(candidate, receipt, now) == nil {
				t.Fatal("receipt accepted for a changed expected tuple")
			}
		})
	}
	if root.Verify(request, receipt, now.Add(2*time.Hour)) == nil {
		t.Fatal("expired receipt accepted")
	}
	if root.Verify(request, receipt, now.Add(-31*time.Second)) == nil {
		t.Fatal("receipt issued too far in the future accepted")
	}
	if !root.Valid() || root.KeyID() != receipt.KeyID {
		t.Fatal("valid root identity was not retained")
	}
}

func TestRequestRequiresRunAndWorkflowRef(t *testing.T) {
	request, _, _, _ := deterministicFixture(t)
	request.WorkflowRunID = 0
	if request.Valid() {
		t.Fatal("request without a positive run ID accepted")
	}
	request.WorkflowRunID = 7
	request.WorkflowRef = ""
	if request.Valid() {
		t.Fatal("request without a workflow ref accepted")
	}
}
