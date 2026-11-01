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

func TestLegacySourceAndExplicitVersion(t *testing.T) {
	b, root, expected, key := bindingFixture(t)
	b.Request.Source = ReusableCalleeSource
	expected.Request = b.Request
	if !b.Request.Valid() {
		t.Fatal("legacy v1 source rejected")
	}
	b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes()))
	raw, _ := json.Marshal(b)
	decoded, err := DecodeStrictBinding(raw)
	if err != nil || root.VerifyBinding(expected, decoded, time.Now()) != nil {
		t.Fatal("legacy source binding rejected")
	}

}

func TestExplicitVersionOneRoundTripOrReject(t *testing.T) {
	b, root, expected, key := bindingFixture(t)
	var raw []byte
	var decoded CredentialBinding
	var err error
	explicit := b.Request
	explicit.Version = 1
	if explicit.Valid() {
		b.Request = explicit
		expected.Request = explicit
		b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes()))
		raw, _ = json.Marshal(b)
		decoded, err = DecodeStrictBinding(raw)
		if err != nil || decoded.Request != explicit || root.VerifyBinding(expected, decoded, time.Now()) != nil {
			t.Fatal("accepted explicit v1 request did not round trip")
		}
	}
}

func TestV2SignedReceiptAndBindingIndependentAuthorities(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	request := Request{Version: 2, Callee: CalleeIdentity{Repository: "fixture-org/canary", WorkflowPath: ".github/workflows/reusable.yml", SHA: strings.Repeat("a", 40)}, ControllerApprovalSHA256: strings.Repeat("b", 64), Repository: "fixture-org/canary", WorkflowRunID: 7, WorkflowRef: "refs/heads/main", WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/caller.yml", Phase: "create", OwnerNonce: strings.Repeat("d", 32), Source: ReusableCalleeSource}
	workflowKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{31}, 32))
	workflowRoot, _ := NewTrustRoot("workflow", workflowKey.Public().(ed25519.PublicKey))
	issuerKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{41}, 32))
	issuerRoot, _ := NewIssuerRoot("issuer", issuerKey.Public().(ed25519.PublicKey))
	if !issuerRoot.Independent(workflowRoot) {
		t.Fatal("roots not independent")
	}
	receipt := Receipt{Version: 2, Callee: request.Callee, Algorithm: "ed25519", KeyID: "workflow", ControllerApprovalSHA256: request.ControllerApprovalSHA256, Repository: request.Repository, WorkflowRunID: request.WorkflowRunID, WorkflowRef: request.WorkflowRef, WorkflowSHA: request.WorkflowSHA, WorkflowPath: request.WorkflowPath, Phase: request.Phase, OwnerNonce: request.OwnerNonce, ReceiptNonce: strings.Repeat("e", 32), Source: request.Source, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}
	signReceipt := func(r *Receipt) {
		r.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(workflowKey, r.SigningBytes()))
	}
	signReceipt(&receipt)
	receiptBytes, _ := json.Marshal(receipt)
	decodedReceipt, err := DecodeStrictReceipt(receiptBytes)
	if err != nil || workflowRoot.Verify(request, decodedReceipt, now) != nil {
		t.Fatal("signed workflow receipt rejected")
	}
	expectedRequest := Request{Version: 2, Callee: CalleeIdentity{Repository: "fixture-org/canary", WorkflowPath: ".github/workflows/reusable.yml", SHA: strings.Repeat("a", 40)}, ControllerApprovalSHA256: strings.Repeat("b", 64), Repository: "fixture-org/canary", WorkflowRunID: 7, WorkflowRef: "refs/heads/main", WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/caller.yml", Phase: "create", OwnerNonce: strings.Repeat("d", 32), Source: ReusableCalleeSource}
	expected := BindingExpected{Request: expectedRequest, Receipt: decodedReceipt, ReceiptBytes: receiptBytes, ApprovalExpiresAt: now.Add(time.Hour), Deadline: now.Add(time.Hour)}
	b := CredentialBinding{BindingClaims: BindingClaims{Request: request, Version: 1, Kind: BindingKind, Algorithm: "ed25519", Audience: BindingAudience, KeyID: "issuer", IssuerSource: IssuerSource, WorkflowKeyID: "workflow", ReceiptSHA256: Digest(receiptBytes), PayloadSHA256: Digest([]byte("payload")), PayloadLength: 7, ReceiptNonce: receipt.ReceiptNonce, IssuedAt: now, ExpiresAt: now.Add(time.Minute)}}
	signBinding := func(b *CredentialBinding) {
		b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(issuerKey, b.SigningBytes()))
	}
	signBinding(&b)
	raw, _ := json.Marshal(b)
	decoded, err := DecodeStrictBinding(raw)
	if err != nil || issuerRoot.VerifyBinding(expected, decoded, now) != nil {
		t.Fatal("signed binding round trip rejected")
	}
	for _, fault := range []string{"identity", "receipt", "source", "version", "nonce", "key", "signature"} {
		t.Run(fault, func(t *testing.T) {
			x, e := decoded, expected
			switch fault {
			case "identity":
				x.Callee.SHA = strings.Repeat("f", 40)
			case "receipt":
				e.ReceiptBytes = append(bytes.Clone(e.ReceiptBytes), ' ')
			case "source":
				x.Source = "other"
			case "version":
				x.Request.Version = 0
			case "nonce":
				x.ReceiptNonce = strings.Repeat("f", 32)
			case "key":
				x.KeyID = "other"
			case "signature":
				x.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(workflowKey, x.SigningBytes()))
			}
			if fault != "signature" {
				signBinding(&x)
			}
			if issuerRoot.VerifyBinding(e, x, now) == nil {
				t.Fatal("altered binding accepted")
			}
		})
	}
	for _, fault := range []string{"identity", "source", "version", "nonce", "key", "signature"} {
		t.Run("receipt "+fault, func(t *testing.T) {
			x := decodedReceipt
			switch fault {
			case "identity":
				x.Callee.SHA = strings.Repeat("f", 40)
			case "source":
				x.Source = "other"
			case "version":
				x.Version = 1
				x.Callee = CalleeIdentity{}
			case "nonce":
				x.ReceiptNonce = request.OwnerNonce
			case "key":
				x.KeyID = "other"
			case "signature":
				x.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(issuerKey, x.SigningBytes()))
			}
			if fault != "signature" {
				signReceipt(&x)
			}
			if workflowRoot.Verify(request, x, now) == nil {
				t.Fatal("altered receipt accepted")
			}
		})
	}
	legacy := request
	legacy.Version = 0
	legacy.Callee = CalleeIdentity{}
	legacyReceipt := decodedReceipt
	legacyReceipt.Version = 1
	legacyReceipt.Callee = CalleeIdentity{}
	signReceipt(&legacyReceipt)
	if workflowRoot.Verify(legacy, legacyReceipt, now) != nil {
		t.Fatal("legacy v1 receipt with original source rejected")
	}
	if workflowRoot.Verify(request, legacyReceipt, now) == nil {
		t.Fatal("v1 receipt accepted as v2")
	}
	if workflowRoot.Verify(legacy, decodedReceipt, now) == nil {
		t.Fatal("v2 receipt accepted as v1")
	}
	expected.Request = legacy
	if issuerRoot.VerifyBinding(expected, decoded, now) == nil {
		t.Fatal("v2 binding accepted as v1")
	}
}
