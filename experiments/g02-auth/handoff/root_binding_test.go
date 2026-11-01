package handoff

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
)

func TestRootBindingPinsBothIndependentRootsAndStrictWire(t *testing.T) {
	workflowKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	issuerKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, ed25519.SeedSize))
	workflow, err := NewTrustRoot("workflow-v1", workflowKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewIssuerRoot("issuer-v1", issuerKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := NewRootBinding(workflow, issuer)
	if err != nil || !binding.Valid() {
		t.Fatalf("binding=%+v err=%v", binding, err)
	}
	wire, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStrictRootBinding(wire)
	if err != nil || !decoded.Equal(binding) {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
	for _, bad := range []string{
		string(wire[:len(wire)-1]) + `,"extra":1}`,
		string(wire[:len(wire)-1]) + `,"version":1}`,
		string(wire) + " {}",
		strings.Replace(string(wire), `"workflow_key_id":"workflow-v1"`, `"workflow_key_id":null`, 1),
	} {
		if _, err := DecodeStrictRootBinding([]byte(bad)); err == nil {
			t.Fatalf("accepted malformed root binding %q", bad)
		}
	}
	otherWorkflowKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, ed25519.SeedSize))
	otherWorkflow, _ := NewTrustRoot("workflow-v1", otherWorkflowKey.Public().(ed25519.PublicKey))
	other, _ := NewRootBinding(otherWorkflow, issuer)
	if binding.Equal(other) {
		t.Fatal("root binding ignored workflow public key")
	}
}
