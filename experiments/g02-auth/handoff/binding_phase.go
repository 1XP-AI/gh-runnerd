package handoff

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"time"
)

const MaxPhaseDuration = 10 * time.Minute
const MinPairedRemaining = time.Minute
const PhaseBindingKind = "g01-issued-phase-binding"

// PhaseBinding is domain separated from the immutable v1 create protocol.
// Request binds raw approval, exact phase, owner, run and workflow source;
// the remaining claims bind the two roles and exact final payload/receipt.
type PhaseBinding struct {
	BindingClaims
	Mode      string `json:"mode"`
	Signature string `json:"signature"`
}

func (b PhaseBinding) SigningBytes() []byte {
	data, _ := json.Marshal(struct {
		BindingClaims
		Mode string `json:"mode"`
	}{b.BindingClaims, b.Mode})
	return append([]byte("gh-runnerd/g01-issued-phase-binding/v2\x00"), data...)
}
func PhaseAudience(mode, phase string) string {
	if mode == "paired-terminal" && phase == mode {
		return "g01-paired-terminal"
	}
	if mode == "controller" && validPhase(phase) && phase != "create" && phase != "paired-terminal" && phase != "discover-actions-host" {
		return "g01-controller-" + phase
	}
	return ""
}
func DecodeStrictPhaseBinding(data []byte) (PhaseBinding, error) {
	var b PhaseBinding
	if decodeExactObject(data, &b, MaxBindingBytes) != nil {
		return b, ErrInvalid
	}
	return b, nil
}
func decodeExactObject(data []byte, target any, limit int) error {
	if len(data) < 1 || len(data) > limit || !hasUniqueObjectKeys(data) {
		return ErrInvalid
	}
	var fields, required map[string]json.RawMessage
	template, _ := json.Marshal(target)
	if json.Unmarshal(data, &fields) != nil || json.Unmarshal(template, &required) != nil || len(fields) != len(required) {
		return ErrInvalid
	}
	for key := range required {
		v, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return ErrInvalid
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return ErrInvalid
	}
	return nil
}
func (r IssuerRoot) VerifyPhaseBinding(e BindingExpected, b PhaseBinding, now time.Time) error {
	audience := PhaseAudience(b.Mode, b.Phase)
	if audience == "" || !r.Valid() || !e.Request.Valid() || b.Request != e.Request || b.Version != 2 || b.Kind != PhaseBindingKind || b.Algorithm != "ed25519" || b.Audience != audience || b.IssuerSource != IssuerSource || b.KeyID != r.keyID || b.WorkflowKeyID != e.Receipt.KeyID || b.ReceiptNonce != e.Receipt.ReceiptNonce || b.ReceiptSHA256 != Digest(e.ReceiptBytes) || !sha256Hex.MatchString(b.PayloadSHA256) || b.PayloadLength < 1 || b.PayloadLength > MaxCredentialBytes || b.IssuedAt.IsZero() || b.IssuedAt.Before(e.Receipt.IssuedAt) || b.IssuedAt.After(now.Add(30*time.Second)) || !b.ExpiresAt.After(now) || !b.ExpiresAt.After(b.IssuedAt) || b.ExpiresAt.After(b.IssuedAt.Add(MaxPhaseDuration)) || b.ExpiresAt.After(e.Receipt.ExpiresAt) || b.ExpiresAt.After(e.ApprovalExpiresAt) || e.Deadline.IsZero() || b.ExpiresAt.After(e.Deadline) || b.Mode == "paired-terminal" && !b.ExpiresAt.After(now.Add(MinPairedRemaining)) {
		return ErrInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(b.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || base64.RawURLEncoding.EncodeToString(sig) != b.Signature || !ed25519.Verify(r.publicKey, b.SigningBytes(), sig) {
		return ErrInvalid
	}
	return nil
}
func (r TrustRoot) MatchesPublicKey(key ed25519.PublicKey) bool {
	return r.Valid() && bytes.Equal(r.publicKey, key)
}
func (r IssuerRoot) MatchesPublicKey(key ed25519.PublicKey) bool {
	return r.Valid() && bytes.Equal(r.publicKey, key)
}
