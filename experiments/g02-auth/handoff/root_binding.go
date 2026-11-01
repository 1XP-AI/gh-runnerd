package handoff

import (
	"bytes"
	"encoding/json"
	"io"
)

const MaxRootBindingBytes = 2048

// RootBinding is credential-free identity evidence returned by controller
// preparation. It binds the independently selected workflow and issuer roots
// without transporting a private key, signer, receipt or management input.
type RootBinding struct {
	Version                 int    `json:"version"`
	WorkflowKeyID           string `json:"workflow_key_id"`
	WorkflowPublicKeySHA256 string `json:"workflow_public_key_sha256"`
	IssuerKeyID             string `json:"issuer_key_id"`
	IssuerPublicKeySHA256   string `json:"issuer_public_key_sha256"`
}

func NewRootBinding(workflow TrustRoot, issuer IssuerRoot) (RootBinding, error) {
	if !issuer.Independent(workflow) {
		return RootBinding{}, ErrInvalid
	}
	return RootBinding{
		Version:                 1,
		WorkflowKeyID:           workflow.keyID,
		WorkflowPublicKeySHA256: Digest(workflow.publicKey),
		IssuerKeyID:             issuer.keyID,
		IssuerPublicKeySHA256:   Digest(issuer.publicKey),
	}, nil
}

func (b RootBinding) Valid() bool {
	return b.Version == 1 && component.MatchString(b.WorkflowKeyID) && sha256Hex.MatchString(b.WorkflowPublicKeySHA256) && component.MatchString(b.IssuerKeyID) && sha256Hex.MatchString(b.IssuerPublicKeySHA256) && b.WorkflowKeyID != b.IssuerKeyID && b.WorkflowPublicKeySHA256 != b.IssuerPublicKeySHA256
}

func (b RootBinding) Equal(other RootBinding) bool {
	return b.Valid() && other.Valid() && b == other
}

func DecodeStrictRootBinding(data []byte) (RootBinding, error) {
	var binding RootBinding
	if len(data) == 0 || len(data) > MaxRootBindingBytes || !hasUniqueObjectKeys(data) {
		return binding, ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return binding, ErrInvalid
	}
	template, _ := json.Marshal(binding)
	var required map[string]json.RawMessage
	if json.Unmarshal(template, &required) != nil || len(fields) != len(required) {
		return binding, ErrInvalid
	}
	for key := range required {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return binding, ErrInvalid
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&binding) != nil {
		return RootBinding{}, ErrInvalid
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || !binding.Valid() {
		return RootBinding{}, ErrInvalid
	}
	return binding, nil
}
