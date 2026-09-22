package handoff

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const MaxReceiptBytes = 16 * 1024

var ErrInvalid = errors.New("invalid broker provenance")

var (
	component          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
	sha256Hex          = regexp.MustCompile(`^[a-f0-9]{64}$`)
	sha40Hex           = regexp.MustCompile(`^[a-f0-9]{40}$`)
	nonceHex           = regexp.MustCompile(`^[a-f0-9]{32}$`)
	provenanceSource   = regexp.MustCompile(`^[a-z][a-z0-9._/-]{0,63}$`)
	repositoryFullName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}/[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
	workflowRef        = regexp.MustCompile(`^refs/(?:heads|tags)/[A-Za-z0-9._/-]+$|^refs/pull/[1-9][0-9]*/(?:head|merge)$`)
	workflowPath       = regexp.MustCompile(`^\.github/workflows/[a-zA-Z0-9_-]+\.ya?ml$`)
)

type Request struct {
	ControllerApprovalSHA256 string `json:"controller_approval_sha256"`
	Repository               string `json:"repository"`
	WorkflowRunID            int64  `json:"workflow_run_id"`
	WorkflowRef              string `json:"workflow_ref"`
	WorkflowSHA              string `json:"workflow_sha"`
	WorkflowPath             string `json:"workflow_path"`
	Phase                    string `json:"phase"`
	OwnerNonce               string `json:"owner_nonce"`
	Source                   string `json:"source"`
}

func (r Request) Valid() bool {
	return sha256Hex.MatchString(r.ControllerApprovalSHA256) && repositoryFullName.MatchString(r.Repository) && r.WorkflowRunID > 0 && workflowRef.MatchString(r.WorkflowRef) && sha40Hex.MatchString(r.WorkflowSHA) && workflowPath.MatchString(r.WorkflowPath) && validPhase(r.Phase) && nonceHex.MatchString(r.OwnerNonce) && provenanceSource.MatchString(r.Source)
}

func validPhase(phase string) bool {
	switch phase {
	case "create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "jit-loss", "drain", "inspect", "cleanup", "discover-actions-host", "paired-terminal":
		return true
	default:
		return false
	}
}

type Receipt struct {
	Version                  int       `json:"version"`
	Algorithm                string    `json:"algorithm"`
	KeyID                    string    `json:"key_id"`
	ControllerApprovalSHA256 string    `json:"controller_approval_sha256"`
	Repository               string    `json:"repository"`
	WorkflowRunID            int64     `json:"workflow_run_id"`
	WorkflowRef              string    `json:"workflow_ref"`
	WorkflowSHA              string    `json:"workflow_sha"`
	WorkflowPath             string    `json:"workflow_path"`
	Phase                    string    `json:"phase"`
	OwnerNonce               string    `json:"owner_nonce"`
	ReceiptNonce             string    `json:"receipt_nonce"`
	Source                   string    `json:"source"`
	IssuedAt                 time.Time `json:"issued_at"`
	ExpiresAt                time.Time `json:"expires_at"`
	Signature                string    `json:"signature"`
}

type signedPayload struct {
	Version                  int       `json:"version"`
	Algorithm                string    `json:"algorithm"`
	KeyID                    string    `json:"key_id"`
	ControllerApprovalSHA256 string    `json:"controller_approval_sha256"`
	Repository               string    `json:"repository"`
	WorkflowRunID            int64     `json:"workflow_run_id"`
	WorkflowRef              string    `json:"workflow_ref"`
	WorkflowSHA              string    `json:"workflow_sha"`
	WorkflowPath             string    `json:"workflow_path"`
	Phase                    string    `json:"phase"`
	OwnerNonce               string    `json:"owner_nonce"`
	ReceiptNonce             string    `json:"receipt_nonce"`
	Source                   string    `json:"source"`
	IssuedAt                 time.Time `json:"issued_at"`
	ExpiresAt                time.Time `json:"expires_at"`
}

func (r Receipt) SigningBytes() []byte {
	payload := signedPayload{
		Version:                  r.Version,
		Algorithm:                r.Algorithm,
		KeyID:                    r.KeyID,
		ControllerApprovalSHA256: r.ControllerApprovalSHA256,
		Repository:               r.Repository,
		WorkflowRunID:            r.WorkflowRunID,
		WorkflowRef:              r.WorkflowRef,
		WorkflowSHA:              r.WorkflowSHA,
		WorkflowPath:             r.WorkflowPath,
		Phase:                    r.Phase,
		OwnerNonce:               r.OwnerNonce,
		ReceiptNonce:             r.ReceiptNonce,
		Source:                   r.Source,
		IssuedAt:                 r.IssuedAt,
		ExpiresAt:                r.ExpiresAt,
	}
	data, _ := json.Marshal(payload)
	return data
}

func (r Receipt) Validate(request Request, now time.Time) error {
	if !request.Valid() || r.Version != 1 || r.Algorithm != "ed25519" || !component.MatchString(r.KeyID) || r.ControllerApprovalSHA256 != request.ControllerApprovalSHA256 || r.Repository != request.Repository || r.WorkflowRunID != request.WorkflowRunID || r.WorkflowRef != request.WorkflowRef || r.WorkflowSHA != request.WorkflowSHA || r.WorkflowPath != request.WorkflowPath || r.Phase != request.Phase || r.OwnerNonce != request.OwnerNonce || !nonceHex.MatchString(r.ReceiptNonce) || r.ReceiptNonce == r.OwnerNonce || r.Source != request.Source || r.IssuedAt.IsZero() || r.ExpiresAt.IsZero() || r.IssuedAt.After(now.Add(30*time.Second)) || !r.ExpiresAt.After(now) || !r.ExpiresAt.After(r.IssuedAt) || r.ExpiresAt.After(now.Add(24*time.Hour)) {
		return ErrInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(r.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ErrInvalid
	}
	return nil
}

func (r Receipt) VerifyEd25519(publicKey ed25519.PublicKey) error {
	request := Request{
		ControllerApprovalSHA256: r.ControllerApprovalSHA256,
		Repository:               r.Repository,
		WorkflowRunID:            r.WorkflowRunID,
		WorkflowRef:              r.WorkflowRef,
		WorkflowSHA:              r.WorkflowSHA,
		WorkflowPath:             r.WorkflowPath,
		Phase:                    r.Phase,
		OwnerNonce:               r.OwnerNonce,
		Source:                   r.Source,
	}
	if len(publicKey) != ed25519.PublicKeySize || r.Validate(request, time.Now()) != nil {
		return ErrInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(r.Signature)
	if err != nil || !ed25519.Verify(publicKey, r.SigningBytes(), signature) {
		return ErrInvalid
	}
	return nil
}

type TrustRoot struct {
	keyID     string
	publicKey ed25519.PublicKey
}

func NewTrustRoot(keyID string, publicKey ed25519.PublicKey) (TrustRoot, error) {
	root := TrustRoot{keyID: keyID, publicKey: append(ed25519.PublicKey(nil), publicKey...)}
	if !root.Valid() {
		return TrustRoot{}, ErrInvalid
	}
	return root, nil
}

func (r TrustRoot) Valid() bool {
	return component.MatchString(r.keyID) && len(r.publicKey) == ed25519.PublicKeySize
}

func (r TrustRoot) KeyID() string {
	if !r.Valid() {
		return ""
	}
	return r.keyID
}

func (r TrustRoot) Verify(request Request, receipt Receipt, now time.Time) error {
	if !r.Valid() || receipt.KeyID != r.keyID || receipt.Validate(request, now) != nil {
		return ErrInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(receipt.Signature)
	if err != nil || !ed25519.Verify(r.publicKey, receipt.SigningBytes(), signature) {
		return ErrInvalid
	}
	return nil
}

func DecodeStrictReceipt(data []byte) (Receipt, error) {
	var receipt Receipt
	if len(data) == 0 || len(data) > MaxReceiptBytes || !hasUniqueObjectKeys(data) {
		return receipt, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil {
		return Receipt{}, ErrInvalid
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return Receipt{}, ErrInvalid
	}
	return receipt, nil
}

func hasUniqueObjectKeys(data []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') || !uniqueObjectBody(decoder) {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func uniqueObjectBody(decoder *json.Decoder) bool {
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		key, ok := token.(string)
		if !ok {
			return false
		}
		key = foldedJSONName(key)
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
		if !uniqueJSONValue(decoder) {
			return false
		}
	}
	token, err := decoder.Token()
	return err == nil && token == json.Delim('}')
}

func uniqueArrayBody(decoder *json.Decoder) bool {
	for decoder.More() {
		if !uniqueJSONValue(decoder) {
			return false
		}
	}
	token, err := decoder.Token()
	return err == nil && token == json.Delim(']')
}

func uniqueJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		return uniqueObjectBody(decoder)
	case '[':
		return uniqueArrayBody(decoder)
	default:
		return false
	}
}

func foldedJSONName(input string) string {
	var result strings.Builder
	result.Grow(len(input))
	for _, character := range input {
		minimum := character
		for next := unicode.SimpleFold(character); next != character; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		result.WriteRune(minimum)
	}
	return result.String()
}
