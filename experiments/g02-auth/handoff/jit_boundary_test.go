package handoff

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestJITBoundaryExactV2AudienceAndIndependentGrant(t *testing.T) {
	for _, phase := range []string{"before-jit", "jit-loss"} {
		t.Run(phase, func(t *testing.T) {
			v1, root, expected, key := bindingFixture(t)
			expected.Request.Phase = phase
			b := PhaseBinding{BindingClaims: v1.BindingClaims, Mode: "controller"}
			b.Request = expected.Request
			b.Version = 2
			b.Kind = PhaseBindingKind
			b.Audience = PhaseAudience(b.Mode, phase)
			b.ExpiresAt = time.Now().Add(5 * time.Minute)
			sign := func() { b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes())) }
			sign()
			if b.Audience != "g01-controller-"+phase || root.VerifyPhaseBinding(expected, b, time.Now()) != nil {
				t.Fatal("exact v2 JIT audience refused")
			}
			b.Audience = "g01-controller-before-ack"
			sign()
			if root.VerifyPhaseBinding(expected, b, time.Now()) == nil {
				t.Fatal("old audience granted JIT boundary")
			}
			p := phasePolicyFixture(t)
			request := p.Grants[0].Request
			request.Phase = phase
			if _, err := p.Select("controller", request, time.Now()); err == nil {
				t.Fatal("old grant widened")
			}
			p.Grants[0].Request = request
			for _, role := range []string{"broker", "controller"} {
				p.Role = role
				raw, _ := json.Marshal(p)
				admitted, _, _, err := DecodePublicPolicy(raw, role, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if _, err = admitted.Select("controller", request, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			p.IssuerPublicKey = p.WorkflowPublicKey
			raw, _ := json.Marshal(p)
			if _, _, _, err := DecodePublicPolicy(raw, p.Role, time.Now()); err == nil {
				t.Fatal("JIT roots were not independent")
			}
		})
	}
}
