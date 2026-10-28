package handoff

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPhaseBindingExactAuthorityAndCumulativeBudget(t *testing.T) {
	v1, root, e, key := bindingFixture(t)
	e.Request.Phase = "paired-terminal"
	b := PhaseBinding{BindingClaims: v1.BindingClaims, Mode: "paired-terminal"}
	b.Request = e.Request
	b.Version = 2
	b.Kind = PhaseBindingKind
	b.Audience = PhaseAudience(b.Mode, b.Phase)
	b.ExpiresAt = time.Now().UTC().Add(5 * time.Minute)
	sign := func(b *PhaseBinding) {
		b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes()))
	}
	sign(&b)
	if root.VerifyPhaseBinding(e, b, time.Now()) != nil {
		t.Fatal("valid phase refused")
	}
	raw, _ := json.Marshal(b)
	if decoded, err := DecodeStrictPhaseBinding(raw); err != nil || decoded != b {
		t.Fatal("strict phase round trip")
	}
	if bytes.Equal(b.SigningBytes(), v1.SigningBytes()) || !bytes.HasPrefix(b.SigningBytes(), []byte("gh-runnerd/g01-issued-phase-binding/v2\x00")) {
		t.Fatal("domain separation")
	}
	for _, fault := range []string{"mode", "phase", "audience", "workflow-role", "issuer-role", "approval", "receipt", "nonce", "length", "substituted-payload", "short-cadence", "over-ten-minutes", "expired-parent", "grant-cap"} {
		t.Run(fault, func(t *testing.T) {
			x, expected := b, e
			switch fault {
			case "mode":
				x.Mode = "controller"
			case "phase":
				x.Phase = "inspect"
			case "audience":
				x.Audience = "g01-controller-create"
			case "workflow-role":
				x.WorkflowKeyID = "other-workflow"
			case "issuer-role":
				x.KeyID = "other-issuer"
			case "approval":
				x.ControllerApprovalSHA256 = strings.Repeat("f", 64)
			case "receipt":
				expected.ReceiptBytes = []byte("substituted")
			case "nonce":
				x.ReceiptNonce = strings.Repeat("e", 32)
			case "length":
				x.PayloadLength = MaxCredentialBytes + 1
			case "substituted-payload":
				x.PayloadSHA256 = "malformed"
			case "short-cadence":
				x.ExpiresAt = time.Now().Add(time.Minute)
			case "over-ten-minutes":
				x.ExpiresAt = x.IssuedAt.Add(MaxPhaseDuration + time.Second)
			case "expired-parent":
				expected.Deadline = time.Now().Add(-time.Second)
			case "grant-cap":
				expected.Deadline = time.Now().Add(2 * time.Minute)
			}
			sign(&x)
			if root.VerifyPhaseBinding(expected, x, time.Now()) == nil {
				t.Fatal("expanded authority accepted")
			}
		})
	}
	if _, err := DecodeStrictPhaseBinding(append(raw, []byte(" {}")...)); err == nil {
		t.Fatal("trailing envelope")
	}
	if _, err := DecodeStrictBinding(raw); err == nil {
		t.Fatal("phase envelope accepted as legacy v1")
	}
}
func phasePolicyFixture(t *testing.T) PublicPolicy {
	t.Helper()
	v1, _, _, _ := bindingFixture(t)
	return PublicPolicy{Version: 1, Role: "broker", Source: "github-rest-workflow-v1", WorkflowKeyID: "workflow-v1", WorkflowPublicKey: hex.EncodeToString(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32)).Public().(ed25519.PublicKey)), IssuerKeyID: "issuer-v1", IssuerPublicKey: hex.EncodeToString(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{71}, 32)).Public().(ed25519.PublicKey)), Grants: []PhaseGrant{{Request: v1.Request, Mode: "controller", OrganizationID: 101, RepositoryID: 42, ExpiresAt: time.Now().Add(time.Hour)}}}
}
func TestPhasePolicyStrictImmutableRolesAndExactGrants(t *testing.T) {
	p := phasePolicyFixture(t)
	raw, _ := json.Marshal(p)
	if _, _, _, err := DecodePublicPolicy(raw, "broker", time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"rootless", "unknown", "case", "duplicate", "role", "same-key", "same-id", "mode", "source", "run", "expiry", "empty", "expanded", "nested-case", "nested-unknown"} {
		t.Run(fault, func(t *testing.T) {
			x := p
			x.Grants = append([]PhaseGrant(nil), p.Grants...)
			switch fault {
			case "rootless":
				x.WorkflowPublicKey = ""
			case "role":
				x.Role = "controller"
			case "same-key":
				x.IssuerPublicKey = x.WorkflowPublicKey
			case "same-id":
				x.IssuerKeyID = x.WorkflowKeyID
			case "mode":
				x.Grants[0].Mode = "paired-terminal"
			case "source":
				x.Source = "unreviewed-source"
			case "run":
				x.Grants[0].Request.WorkflowRunID = 0
			case "expiry":
				x.Grants[0].ExpiresAt = time.Now().Add(-time.Second)
			case "empty":
				x.Grants = nil
			case "expanded":
				x.Grants = append(x.Grants, x.Grants[0])
			}
			data, _ := json.Marshal(x)
			switch fault {
			case "unknown":
				data = bytes.Replace(data, []byte(`"version":1`), []byte(`"version":1,"unknown":1`), 1)
			case "case":
				data = bytes.Replace(data, []byte(`"role"`), []byte(`"Role"`), 1)
			case "duplicate":
				data = bytes.Replace(data, []byte(`"role":"broker"`), []byte(`"role":"broker","role":"broker"`), 1)
			case "nested-case":
				data = bytes.Replace(data, []byte(`"workflow_run_id"`), []byte(`"Workflow_run_id"`), 1)
			case "nested-unknown":
				data = bytes.Replace(data, []byte(`"mode":"controller"`), []byte(`"mode":"controller","authority":"expanded"`), 1)
			}
			if _, _, _, err := DecodePublicPolicy(data, "broker", time.Now()); err == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
	request := p.Grants[0].Request
	request.Phase = "inspect"
	if _, err := p.Select("controller", request, time.Now()); err == nil {
		t.Fatal("ungranted phase")
	}
	if _, err := p.Select("paired-terminal", p.Grants[0].Request, time.Now()); err == nil {
		t.Fatal("cross-mode grant")
	}
	for _, raw := range []string{"", `{}`, `null`} {
		if _, _, _, err := DecodePublicPolicy([]byte(raw), "broker", time.Now()); err == nil {
			t.Fatal("empty default enabled")
		}
	}
}
