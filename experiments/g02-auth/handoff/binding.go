package handoff

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"time"
)

const (
	// One cumulative budget covers control acquisition, credentials and create.
	MaxProcessDuration = 30 * time.Second
	MaxBindingBytes    = 4096
	MaxCredentialBytes = 16384
	BindingKind        = "g01-issued-credential-binding"
	BindingAudience    = "g01-controller-create"
	IssuerSource       = "g02-credential-issuer-v1"
)

// BindingClaims has a deterministic typed encoding, separate from v1 receipts.
// Source in Request is the workflow observer's source, never the issuer's.
type BindingClaims struct {
	Request
	Version       int       `json:"version"`
	Kind          string    `json:"kind"`
	Algorithm     string    `json:"algorithm"`
	Audience      string    `json:"audience"`
	KeyID         string    `json:"key_id"`
	IssuerSource  string    `json:"issuer_source"`
	WorkflowKeyID string    `json:"workflow_key_id"`
	ReceiptSHA256 string    `json:"receipt_sha256"`
	PayloadSHA256 string    `json:"payload_sha256"`
	PayloadLength int       `json:"payload_length"`
	ReceiptNonce  string    `json:"receipt_nonce"`
	IssuedAt      time.Time `json:"issued_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}
type CredentialBinding struct {
	BindingClaims
	Signature string `json:"signature"`
}

func (b CredentialBinding) SigningBytes() []byte {
	data, _ := json.Marshal(b.BindingClaims)
	return append([]byte("gh-runnerd/g01-issued-credential-binding/v1\x00"), data...)
}
func Digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func DecodeStrictBinding(data []byte) (CredentialBinding, error) {
	var b CredentialBinding
	if len(data) == 0 || len(data) > MaxBindingBytes || !hasUniqueObjectKeys(data) {
		return b, ErrInvalid
	}
	// Require every exact, case-sensitive field, with no null or unknown values.
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return b, ErrInvalid
	}
	template, _ := json.Marshal(b)
	var required map[string]json.RawMessage
	_ = json.Unmarshal(template, &required)
	if len(fields) != len(required) {
		return b, ErrInvalid
	}
	for key := range required {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return b, ErrInvalid
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&b) != nil {
		return CredentialBinding{}, ErrInvalid
	}
	return b, nil
}

// Equal compares the full pinned workflow identity, never only a key label.
func (r TrustRoot) Equal(other TrustRoot) bool {
	return r.Valid() && other.Valid() && r.keyID == other.keyID && bytes.Equal(r.publicKey, other.publicKey)
}

// IssuerRoot is independently selected by reviewed receiver assembly. Its key
// must differ from the independently selected workflow authority key.
type IssuerRoot struct {
	keyID     string
	publicKey ed25519.PublicKey
}

func NewIssuerRoot(keyID string, key ed25519.PublicKey) (IssuerRoot, error) {
	r := IssuerRoot{keyID, bytes.Clone(key)}
	if !r.Valid() {
		return IssuerRoot{}, ErrInvalid
	}
	return r, nil
}
func (r IssuerRoot) Valid() bool {
	return component.MatchString(r.keyID) && len(r.publicKey) == ed25519.PublicKeySize
}
func (r IssuerRoot) KeyID() string { return r.keyID }
func (r IssuerRoot) Independent(workflow TrustRoot) bool {
	return r.Valid() && workflow.Valid() && r.keyID != workflow.keyID && !bytes.Equal(r.publicKey, workflow.publicKey)
}

// BindingExpected contains independently verified approval/receipt facts.
// VerifyBinding establishes issuer binding, not workflow authority.
type BindingExpected struct {
	Request           Request
	Receipt           Receipt
	ReceiptBytes      []byte
	ApprovalExpiresAt time.Time
	Deadline          time.Time
}

func (r IssuerRoot) VerifyBinding(e BindingExpected, b CredentialBinding, now time.Time) error {
	if !r.Valid() || !e.Request.Valid() || b.Request != e.Request || b.Phase != "create" || b.Version != 1 || b.Kind != BindingKind || b.Algorithm != "ed25519" || b.Audience != BindingAudience || b.IssuerSource != IssuerSource || b.KeyID != r.keyID || b.WorkflowKeyID != e.Receipt.KeyID || b.ReceiptNonce != e.Receipt.ReceiptNonce || b.ReceiptSHA256 != Digest(e.ReceiptBytes) || !sha256Hex.MatchString(b.PayloadSHA256) || b.PayloadLength < 1 || b.PayloadLength > MaxCredentialBytes || b.IssuedAt.IsZero() || b.IssuedAt.Before(e.Receipt.IssuedAt) || b.IssuedAt.After(now.Add(30*time.Second)) || !b.ExpiresAt.After(now) || !b.ExpiresAt.After(b.IssuedAt) || b.ExpiresAt.After(e.Receipt.ExpiresAt) || b.ExpiresAt.After(e.ApprovalExpiresAt) || e.Deadline.IsZero() || b.ExpiresAt.After(e.Deadline) {
		return ErrInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(b.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || base64.RawURLEncoding.EncodeToString(sig) != b.Signature || !ed25519.Verify(r.publicKey, b.SigningBytes(), sig) {
		return ErrInvalid
	}
	return nil
}

// ReadControl requires EOF; callers own a cancellable concrete pipe.
func ReadControl(input io.Reader) ([]byte, []byte, error) {
	var header [16]byte
	if _, err := io.ReadFull(input, header[:]); err != nil || string(header[:4]) != "G01H" || binary.BigEndian.Uint32(header[4:8]) != 1 {
		return nil, nil, ErrInvalid
	}
	nr, nb := int(binary.BigEndian.Uint32(header[8:12])), int(binary.BigEndian.Uint32(header[12:16]))
	if nr < 1 || nr > MaxReceiptBytes || nb < 1 || nb > MaxBindingBytes {
		return nil, nil, ErrInvalid
	}
	receipt, binding := make([]byte, nr), make([]byte, nb)
	if _, err := io.ReadFull(input, receipt); err != nil {
		clear(receipt)
		return nil, nil, ErrInvalid
	}
	if _, err := io.ReadFull(input, binding); err != nil {
		clear(receipt)
		clear(binding)
		return nil, nil, ErrInvalid
	}
	var tail [1]byte
	n, err := input.Read(tail[:])
	if n != 0 || err != io.EOF {
		clear(receipt)
		clear(binding)
		return nil, nil, ErrInvalid
	}
	return receipt, binding, nil
}
func EncodeControl(receipt, binding []byte) ([]byte, error) {
	if len(receipt) < 1 || len(receipt) > MaxReceiptBytes || len(binding) < 1 || len(binding) > MaxBindingBytes {
		return nil, ErrInvalid
	}
	header := []byte{'G', '0', '1', 'H', 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[8:12], uint32(len(receipt)))
	binary.BigEndian.PutUint32(header[12:16], uint32(len(binding)))
	return append(append(header, receipt...), binding...), nil
}
