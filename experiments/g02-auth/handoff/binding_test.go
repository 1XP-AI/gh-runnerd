package handoff

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func bindingFixture(t *testing.T) (CredentialBinding, IssuerRoot, BindingExpected, ed25519.PrivateKey) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	request := Request{ControllerApprovalSHA256: strings.Repeat("a", 64), Repository: "fixture-org/canary", WorkflowRunID: 7, WorkflowRef: "refs/heads/main", WorkflowSHA: strings.Repeat("b", 40), WorkflowPath: ".github/workflows/canary.yml", Phase: "create", OwnerNonce: strings.Repeat("c", 32), Source: "github-rest-workflow-v1"}
	receipt := Receipt{KeyID: "workflow-v1", ReceiptNonce: strings.Repeat("d", 32), IssuedAt: now, ExpiresAt: now.Add(time.Hour)}
	raw := []byte("exact fixture receipt bytes")
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{71}, 32))
	root, _ := NewIssuerRoot("issuer-v1", key.Public().(ed25519.PublicKey))
	b := CredentialBinding{BindingClaims: BindingClaims{Request: request, Version: 1, Kind: BindingKind, Algorithm: "ed25519", Audience: BindingAudience, KeyID: root.KeyID(), IssuerSource: IssuerSource, WorkflowKeyID: receipt.KeyID, ReceiptSHA256: Digest(raw), PayloadSHA256: Digest([]byte("payload")), PayloadLength: 7, ReceiptNonce: receipt.ReceiptNonce, IssuedAt: now, ExpiresAt: now.Add(time.Minute)}}
	b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes()))
	return b, root, BindingExpected{request, receipt, raw, now.Add(time.Hour), now.Add(time.Hour)}, key
}
func TestBindingStrictAndRoleVerification(t *testing.T) {
	b, root, expected, key := bindingFixture(t)
	raw, _ := json.Marshal(b)
	decoded, e := DecodeStrictBinding(raw)
	if e != nil || decoded != b || root.VerifyBinding(expected, b, time.Now()) != nil {
		t.Fatal("valid binding refused")
	}
	if !bytes.HasPrefix(b.SigningBytes(), []byte("gh-runnerd/g01-issued-credential-binding/v1\x00")) {
		t.Fatal("missing domain separation")
	}
	for _, fault := range []string{"version", "kind", "algorithm", "audience", "issuer source", "workflow source", "key", "workflow key", "approval", "repository", "run", "ref", "sha", "path", "phase", "owner", "nonce", "receipt", "payload hash", "length zero", "length large", "expiry", "approval expiry", "receipt expiry", "parent expiry", "issued future", "issued before receipt", "signature", "workflow signature", "root"} {
		t.Run(fault, func(t *testing.T) {
			x := b
			r := root
			e := expected
			switch fault {
			case "version":
				x.Version = 2
			case "kind":
				x.Kind = "other"
			case "algorithm":
				x.Algorithm = "other"
			case "audience":
				x.Audience = "other"
			case "issuer source":
				x.IssuerSource = "other"
			case "workflow source":
				x.Source = "other"
			case "key":
				x.KeyID = "other"
			case "workflow key":
				x.WorkflowKeyID = "other"
			case "approval":
				x.ControllerApprovalSHA256 = strings.Repeat("f", 64)
			case "repository":
				x.Repository = "other/repo"
			case "run":
				x.WorkflowRunID++
			case "ref":
				x.WorkflowRef = "refs/heads/other"
			case "sha":
				x.WorkflowSHA = strings.Repeat("f", 40)
			case "path":
				x.WorkflowPath = ".github/workflows/other.yml"
			case "phase":
				x.Phase = "cleanup"
			case "owner":
				x.OwnerNonce = strings.Repeat("e", 32)
			case "nonce":
				x.ReceiptNonce = strings.Repeat("e", 32)
			case "receipt":
				e.ReceiptBytes = append(bytes.Clone(e.ReceiptBytes), ' ')
			case "payload hash":
				x.PayloadSHA256 = "bad"
			case "length zero":
				x.PayloadLength = 0
			case "length large":
				x.PayloadLength = MaxCredentialBytes + 1
			case "expiry":
				x.ExpiresAt = time.Now().Add(-time.Second)
			case "approval expiry":
				e.ApprovalExpiresAt = x.ExpiresAt.Add(-time.Second)
			case "receipt expiry":
				e.Receipt.ExpiresAt = x.ExpiresAt.Add(-time.Second)
			case "parent expiry":
				e.Deadline = x.ExpiresAt.Add(-time.Second)
			case "issued future":
				x.IssuedAt = time.Now().Add(time.Minute)
			case "issued before receipt":
				x.IssuedAt = e.Receipt.IssuedAt.Add(-time.Second)
			case "root":
				other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{72}, 32))
				r, _ = NewIssuerRoot(r.KeyID(), other.Public().(ed25519.PublicKey))
			}
			x.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, x.SigningBytes()))
			if fault == "signature" {
				x.Signature = strings.Repeat("A", 86)
			}
			if fault == "workflow signature" {
				x.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, e.Receipt.SigningBytes()))
			}
			if r.VerifyBinding(e, x, time.Now()) == nil {
				t.Fatal("invalid signed binding accepted")
			}
		})
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for name := range fields {
		t.Run("required "+name, func(t *testing.T) {
			for _, replacement := range []json.RawMessage{nil, []byte("null")} {
				copyFields := make(map[string]json.RawMessage)
				for k, v := range fields {
					copyFields[k] = v
				}
				if replacement == nil {
					delete(copyFields, name)
				} else {
					copyFields[name] = replacement
				}
				value, _ := json.Marshal(copyFields)
				if _, e := DecodeStrictBinding(value); e == nil {
					t.Fatal("missing/null field accepted")
				}
			}
		})
	}
	for _, value := range [][]byte{append([]byte(`{"extra":1,`), raw[1:]...), append([]byte(`{"PHASE":"create",`), raw[1:]...), bytes.Replace(raw, []byte(`"phase"`), []byte(`"PHASE"`), 1), append(bytes.Clone(raw), []byte(` {}`)...), bytes.Repeat([]byte(" "), MaxBindingBytes+1)} {
		if _, e := DecodeStrictBinding(value); e == nil {
			t.Fatal("non-strict envelope accepted")
		}
	}
	workflow, _ := NewTrustRoot(root.KeyID(), key.Public().(ed25519.PublicKey))
	if root.Independent(workflow) {
		t.Fatal("shared role key accepted")
	}
	differentID, _ := NewTrustRoot("other", key.Public().(ed25519.PublicKey))
	if root.Independent(differentID) {
		t.Fatal("same key under another role accepted")
	}
	other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{72}, 32))
	sameID, _ := NewTrustRoot(root.KeyID(), other.Public().(ed25519.PublicKey))
	if workflow.Equal(sameID) {
		t.Fatal("same ID different key equal")
	}
}
func TestControlExactBoundedFraming(t *testing.T) {
	receipt, binding := []byte("receipt"), []byte("binding")
	frame, _ := EncodeControl(receipt, binding)
	if !bytes.Equal(frame[:16], []byte("G01H\x00\x00\x00\x01\x00\x00\x00\x07\x00\x00\x00\x07")) {
		t.Fatal("control header is not fixed big endian v1")
	}
	a, b, e := ReadControl(bytes.NewReader(frame))
	if e != nil || !bytes.Equal(a, receipt) || !bytes.Equal(b, binding) {
		t.Fatal("valid frame refused")
	}
	for n := 0; n < len(frame); n++ {
		if _, _, e := ReadControl(bytes.NewReader(frame[:n])); e == nil {
			t.Fatal("truncated control accepted")
		}
	}
	for _, fault := range []string{"magic", "version", "receipt zero", "binding zero", "receipt over", "binding over", "tail"} {
		t.Run(fault, func(t *testing.T) {
			x := bytes.Clone(frame)
			switch fault {
			case "magic":
				x[0] = 'X'
			case "version":
				binary.BigEndian.PutUint32(x[4:8], 2)
			case "receipt zero":
				binary.BigEndian.PutUint32(x[8:12], 0)
			case "binding zero":
				binary.BigEndian.PutUint32(x[12:16], 0)
			case "receipt over":
				binary.BigEndian.PutUint32(x[8:12], MaxReceiptBytes+1)
			case "binding over":
				binary.BigEndian.PutUint32(x[12:16], MaxBindingBytes+1)
			case "tail":
				x = append(x, 'x')
			}
			if _, _, e := ReadControl(bytes.NewReader(x)); e == nil {
				t.Fatal("invalid frame accepted")
			}
		})
	}
	for _, sizes := range [][2]int{{0, 1}, {1, 0}, {MaxReceiptBytes + 1, 1}, {1, MaxBindingBytes + 1}} {
		if _, e := EncodeControl(make([]byte, sizes[0]), make([]byte, sizes[1])); e == nil {
			t.Fatal("out of bounds frame encoded")
		}
	}
}
