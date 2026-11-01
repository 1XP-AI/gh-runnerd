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

func TestPhaseV2IndependentGrantsReceiptAndWire(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	b, issuer, expected, issuerKey := bindingFixture(t)
	req := expected.Request
	req.Phase, req.Version, req.Source = "inspect", 2, ReusableCalleeSource
	req.Callee = CalleeIdentity{Repository: req.Repository, WorkflowPath: ".github/workflows/callee.yml", SHA: strings.Repeat("a", 40)}
	gp := GroupPolicy{Version: 2, OrganizationID: 101, Workflow: req.Repository + "/" + req.Callee.WorkflowPath + "@" + req.Callee.SHA}
	grant := PhaseGrant{Request: req, Mode: "controller", OrganizationID: 101, RepositoryID: 42, ExpiresAt: now.Add(time.Hour)}
	grant.GroupPolicySHA256 = gp.GrantDigest(req, 42, 7, "organization-only")
	policy := phasePolicyFixture(t)
	policy.Version, policy.Source, policy.Grants = 3, ReusableCalleeSource, []PhaseGrant{grant}
	caller := req
	caller.Callee = CalleeIdentity{}
	selectRole := func(role string) (PhaseGrant, TrustRoot) {
		p := policy
		p.Role = role
		raw, _ := json.Marshal(p)
		decoded, root, _, err := DecodePublicPolicy(raw, role, now)
		if err != nil {
			t.Fatal(err)
		}
		selected, err := decoded.SelectCallee("controller", caller, 101, 42, 7, "organization-only", gp, now)
		if err != nil {
			t.Fatal(err)
		}
		return selected, root
	}
	broker, _ := selectRole("broker")
	controller, workflow := selectRole("controller")
	receipt := Receipt{Version: 2, Algorithm: "ed25519", KeyID: "workflow-v1", ControllerApprovalSHA256: broker.Request.ControllerApprovalSHA256, Repository: broker.Request.Repository, WorkflowRunID: broker.Request.WorkflowRunID, WorkflowRef: broker.Request.WorkflowRef, WorkflowSHA: broker.Request.WorkflowSHA, WorkflowPath: broker.Request.WorkflowPath, Phase: broker.Request.Phase, OwnerNonce: broker.Request.OwnerNonce, ReceiptNonce: strings.Repeat("d", 32), Source: broker.Request.Source, Callee: broker.Request.Callee, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}
	workflowKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	receipt.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(workflowKey, receipt.SigningBytes()))
	receiptWire, _ := json.Marshal(receipt)
	received, err := DecodeStrictReceipt(receiptWire)
	if err != nil || workflow.Verify(controller.Request, received, now) != nil {
		t.Fatal("independently selected controller rejected receipt")
	}
	expected.Request, expected.Receipt, expected.ReceiptBytes = controller.Request, received, receiptWire
	phase := PhaseBinding{BindingClaims: b.BindingClaims, Mode: "controller"}
	phase.Request, phase.Version, phase.Kind, phase.Audience = broker.Request, 2, PhaseBindingKind, PhaseAudience("controller", req.Phase)
	phase.ReceiptSHA256, phase.ReceiptNonce, phase.WorkflowKeyID = Digest(receiptWire), receipt.ReceiptNonce, receipt.KeyID
	phase.IssuedAt, phase.ExpiresAt = now, now.Add(time.Minute)
	phase.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(issuerKey, phase.SigningBytes()))
	raw, _ := json.Marshal(phase)
	decoded, err := DecodeStrictPhaseBinding(raw)
	if err != nil || decoded != phase || issuer.VerifyPhaseBinding(expected, decoded, now) != nil {
		t.Fatal("v2 phase wire rejected")
	}
	for name, data := range map[string][]byte{
		"missing version": bytes.Replace(raw, []byte(`,"request_version":2`), nil, 1),
		"wrong version":   bytes.Replace(raw, []byte(`"request_version":2`), []byte(`"request_version":1`), 1),
		"nested case":     bytes.Replace(raw, []byte(`"workflow_path":".github/workflows/callee.yml"`), []byte(`"Workflow_path":".github/workflows/callee.yml"`), 1),
		"duplicate":       bytes.Replace(raw, []byte(`"request_version":2`), []byte(`"request_version":2,"request_version":2`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeStrictPhaseBinding(data); err == nil {
				t.Fatal("malformed accepted")
			}
		})
	}
	altered := expected
	altered.Request.Callee.SHA = strings.Repeat("f", 40)
	if workflow.Verify(altered.Request, received, now) == nil || issuer.VerifyPhaseBinding(altered, decoded, now) == nil {
		t.Fatal("changed independent callee accepted")
	}
}
