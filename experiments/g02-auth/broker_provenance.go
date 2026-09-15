package enrollment

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"time"
)

// BrokerProvenanceRequest is the exact non-secret tuple a trusted provenance
// adapter must attest. The adapter is intentionally separate from the GitHub
// API client: the current REST API does not attest which workflow input chose
// the controller phase.
type BrokerProvenanceRequest struct {
	ControllerApprovalSHA256 string `json:"controller_approval_sha256"`
	WorkflowRunID            int64  `json:"workflow_run_id"`
	WorkflowRef              string `json:"workflow_ref"`
	WorkflowSHA              string `json:"workflow_sha"`
	Phase                    string `json:"phase"`
	OwnerNonce               string `json:"owner_nonce"`
	Source                   string `json:"source"`
}

// BrokerProvenanceReceipt is a one-shot, credential-free broker receipt. Its
// signature covers every field except Signature. ReceiptNonce is distinct from
// OwnerNonce so a stable multi-phase controller approval cannot replay one
// phase's receipt for another phase.
type BrokerProvenanceReceipt struct {
	Version                  int       `json:"version"`
	Algorithm                string    `json:"algorithm"`
	KeyID                    string    `json:"key_id"`
	ControllerApprovalSHA256 string    `json:"controller_approval_sha256"`
	WorkflowRunID            int64     `json:"workflow_run_id"`
	WorkflowRef              string    `json:"workflow_ref"`
	WorkflowSHA              string    `json:"workflow_sha"`
	Phase                    string    `json:"phase"`
	OwnerNonce               string    `json:"owner_nonce"`
	ReceiptNonce             string    `json:"receipt_nonce"`
	Source                   string    `json:"source"`
	IssuedAt                 time.Time `json:"issued_at"`
	ExpiresAt                time.Time `json:"expires_at"`
	Signature                string    `json:"signature"`
}

// BrokerProvenanceAdapter is the narrow trust boundary for workflow-input
// provenance. Production currently supplies no adapter and therefore refuses
// controller/paired execution. Offline tests may supply a signed fixture
// adapter with an independently held public key; they must not label that
// fixture as GitHub workflow-input verification.
type BrokerProvenanceAdapter interface {
	Source() string
	Attest(context.Context, BrokerProvenanceRequest) (BrokerProvenanceReceipt, error)
	Verify(BrokerProvenanceRequest, BrokerProvenanceReceipt) error
}

var brokerProvenanceSource = regexp.MustCompile(`^[a-z][a-z0-9._/-]{0,63}$`)
var brokerWorkflowRef = regexp.MustCompile(`^refs/(?:heads|tags)/[A-Za-z0-9._/-]+$|^refs/pull/[1-9][0-9]*/(?:head|merge)$`)

type brokerProvenanceSignedPayload struct {
	Version                  int       `json:"version"`
	Algorithm                string    `json:"algorithm"`
	KeyID                    string    `json:"key_id"`
	ControllerApprovalSHA256 string    `json:"controller_approval_sha256"`
	WorkflowRunID            int64     `json:"workflow_run_id"`
	WorkflowRef              string    `json:"workflow_ref"`
	WorkflowSHA              string    `json:"workflow_sha"`
	Phase                    string    `json:"phase"`
	OwnerNonce               string    `json:"owner_nonce"`
	ReceiptNonce             string    `json:"receipt_nonce"`
	Source                   string    `json:"source"`
	IssuedAt                 time.Time `json:"issued_at"`
	ExpiresAt                time.Time `json:"expires_at"`
}

func (r BrokerProvenanceReceipt) signedPayload() brokerProvenanceSignedPayload {
	return brokerProvenanceSignedPayload{
		Version:                  r.Version,
		Algorithm:                r.Algorithm,
		KeyID:                    r.KeyID,
		ControllerApprovalSHA256: r.ControllerApprovalSHA256,
		WorkflowRunID:            r.WorkflowRunID,
		WorkflowRef:              r.WorkflowRef,
		WorkflowSHA:              r.WorkflowSHA,
		Phase:                    r.Phase,
		OwnerNonce:               r.OwnerNonce,
		ReceiptNonce:             r.ReceiptNonce,
		Source:                   r.Source,
		IssuedAt:                 r.IssuedAt,
		ExpiresAt:                r.ExpiresAt,
	}
}

// SigningBytes returns the canonical JSON payload that an adapter signs with
// Ed25519. It omits Signature and is deterministic for the fixed struct schema.
func (r BrokerProvenanceReceipt) SigningBytes() []byte {
	b, _ := json.Marshal(r.signedPayload())
	return b
}

func (r BrokerProvenanceReceipt) Validate(req BrokerProvenanceRequest, now time.Time) error {
	if !brokerProvenanceRequestValid(req) || r.Version != 1 || r.Algorithm != "ed25519" || !brokerComponent.MatchString(r.KeyID) || r.ControllerApprovalSHA256 != req.ControllerApprovalSHA256 || r.WorkflowRunID != req.WorkflowRunID || r.WorkflowRef != req.WorkflowRef || r.WorkflowSHA != req.WorkflowSHA || r.Phase != req.Phase || r.OwnerNonce != req.OwnerNonce || r.Source != req.Source || !brokerNonce.MatchString(r.ReceiptNonce) || r.ReceiptNonce == r.OwnerNonce || r.IssuedAt.IsZero() || r.ExpiresAt.IsZero() || r.IssuedAt.After(now.Add(30*time.Second)) || !r.ExpiresAt.After(now) || !r.ExpiresAt.After(r.IssuedAt) || r.ExpiresAt.After(now.Add(24*time.Hour)) {
		return errBroker
	}
	signature, err := base64.RawURLEncoding.DecodeString(r.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errBroker
	}
	return nil
}

func (r BrokerProvenanceReceipt) VerifyEd25519(publicKey ed25519.PublicKey) error {
	if len(publicKey) != ed25519.PublicKeySize || r.Validate(BrokerProvenanceRequest{ControllerApprovalSHA256: r.ControllerApprovalSHA256, WorkflowRunID: r.WorkflowRunID, WorkflowRef: r.WorkflowRef, WorkflowSHA: r.WorkflowSHA, Phase: r.Phase, OwnerNonce: r.OwnerNonce, Source: r.Source}, time.Now()) != nil {
		return errBroker
	}
	signature, err := base64.RawURLEncoding.DecodeString(r.Signature)
	if err != nil || !ed25519.Verify(publicKey, r.SigningBytes(), signature) {
		return errBroker
	}
	return nil
}

func brokerProvenanceRequestValid(req BrokerProvenanceRequest) bool {
	return brokerSHA256.MatchString(req.ControllerApprovalSHA256) && req.WorkflowRunID > 0 && brokerWorkflowRef.MatchString(req.WorkflowRef) && brokerSHA40.MatchString(req.WorkflowSHA) && brokerSlotAllowed(req.Phase) && brokerNonce.MatchString(req.OwnerNonce) && brokerProvenanceSource.MatchString(req.Source)
}

func brokerProvenanceRequest(a BrokerApproval, c controllerApproval, source string) (BrokerProvenanceRequest, error) {
	phase := a.Phase
	if a.Mode == "paired-terminal" {
		phase = "paired-terminal"
	}
	request := BrokerProvenanceRequest{ControllerApprovalSHA256: a.ControllerApprovalSHA256, WorkflowRunID: c.WorkflowRunID, WorkflowRef: c.WorkflowRef, WorkflowSHA: c.WorkflowSHA, Phase: phase, OwnerNonce: a.OwnerNonce, Source: source}
	if a.Mode != "controller" && a.Mode != "paired-terminal" || !brokerProvenanceRequestValid(request) {
		return BrokerProvenanceRequest{}, errBroker
	}
	return request, nil
}
