package handoff

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestSessionReplacementExactV2AudienceAndGrant(t *testing.T) {
	v1, root, expected, key := bindingFixture(t)
	expected.Request.Phase = "session-replacement"
	b := PhaseBinding{BindingClaims: v1.BindingClaims, Mode: "controller"}
	b.Request = expected.Request
	b.Version = 2
	b.Kind = PhaseBindingKind
	b.Audience = PhaseAudience(b.Mode, b.Phase)
	b.ExpiresAt = time.Now().Add(5 * time.Minute)
	sign := func() { b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes())) }
	sign()
	if b.Audience != "g01-controller-session-replacement" || root.VerifyPhaseBinding(expected, b, time.Now()) != nil {
		t.Fatal("replacement v2 binding refused")
	}
	b.Audience = "g01-controller-before-ack"
	sign()
	if root.VerifyPhaseBinding(expected, b, time.Now()) == nil {
		t.Fatal("old audience granted replacement")
	}
	p := phasePolicyFixture(t)
	original := p.Grants[0].Request
	replacement := original
	replacement.Phase = "session-replacement"
	if _, err := p.Select("controller", replacement, time.Now()); err == nil {
		t.Fatal("old grant widened to replacement")
	}
	p.Grants[0].Request = replacement
	raw, _ := json.Marshal(p)
	admitted, _, _, err := DecodePublicPolicy(raw, "broker", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admitted.Select("controller", replacement, time.Now()); err != nil {
		t.Fatal(err)
	}
	p.IssuerPublicKey = p.WorkflowPublicKey
	raw, _ = json.Marshal(p)
	if _, _, _, err = DecodePublicPolicy(raw, "broker", time.Now()); err == nil {
		t.Fatal("replacement roots were not independent")
	}
}
