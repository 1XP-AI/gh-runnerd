package enrollment

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"regexp"
	"strings"
)

// Candidate's PEM is sensitive. This harness accepts bytes in memory only; a
// product file-import boundary still needs owned-path, mode and symlink checks.
type Candidate struct {
	AppID         int64
	PEM           []byte
	Organizations []Binding
}

func (Candidate) String() string               { return "[redacted import candidate]" }
func (Candidate) GoString() string             { return "[redacted import candidate]" }
func (Candidate) MarshalJSON() ([]byte, error) { return []byte(`"[redacted import candidate]"`), nil }

type Binding struct {
	Login          string
	OrganizationID int64
	InstallationID int64
}
type Credential struct {
	AppID int64
	key   *rsa.PrivateKey
}

func (Credential) String() string               { return "[redacted App credential]" }
func (Credential) GoString() string             { return "[redacted App credential]" }
func (Credential) MarshalJSON() ([]byte, error) { return []byte(`"[redacted App credential]"`), nil }

type Installation struct {
	ID, AppID, AccountID, TargetID int64
	Login, AccountType, TargetType string
	Permissions                    map[string]string
	Suspended                      bool
	SuspensionKnown                bool // response explicitly supplied suspended_at
}
type API interface {
	App(context.Context, Credential) (int64, error)
	OrganizationInstallation(context.Context, Credential, string) (Installation, error)
}

// Commit must atomically accept all bindings and the credential or store neither.
// Implementations must not put the key in logs, argv, workers or a state database.
// No real Keychain implementation is claimed by this gate experiment.
type Commit func(context.Context, Credential, []Binding) error
type Result struct {
	AppID         int64
	Organizations []Binding
}

var organizationLogin = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

// ManualImport does not create an App or installation. All bindings are verified
// with the imported App's own JWT before any credential persistence is attempted.
func ManualImport(ctx context.Context, c Candidate, api API, commit Commit) (Result, error) {
	invalid := errors.New("invalid import")
	if api == nil || commit == nil || len(c.Organizations) == 0 || len(c.Organizations) > 32 {
		return Result{}, invalid
	}
	cred, err := parseCredential(c)
	if err != nil {
		return Result{}, invalid
	}
	seenLogin := map[string]bool{}
	seenID := map[int64]bool{}
	seenInstallation := map[int64]bool{}
	for _, b := range c.Organizations {
		login := strings.ToLower(b.Login)
		if !organizationLogin.MatchString(b.Login) || b.OrganizationID < 1 || b.InstallationID < 1 || seenLogin[login] || seenID[b.OrganizationID] || seenInstallation[b.InstallationID] {
			return Result{}, invalid
		}
		seenLogin[login] = true
		seenID[b.OrganizationID] = true
		seenInstallation[b.InstallationID] = true
	}
	id, err := api.App(ctx, cred)
	if err != nil {
		return Result{}, errors.New("App verification failed")
	}
	if id != c.AppID {
		return Result{}, errors.New("App identity mismatch")
	}
	for _, b := range c.Organizations {
		i, err := api.OrganizationInstallation(ctx, cred, b.Login)
		if err != nil {
			return Result{}, errors.New("installation verification failed")
		}
		if i.ID != b.InstallationID || i.AppID != c.AppID || i.AccountID != b.OrganizationID || !strings.EqualFold(i.Login, b.Login) || i.AccountType != "Organization" || i.TargetID != b.OrganizationID || i.TargetType != "Organization" || i.Suspended || !minimalPermissions(i.Permissions) {
			return Result{}, errors.New("installation identity or permission mismatch")
		}
	}
	bindings := append([]Binding(nil), c.Organizations...)
	if err := ctx.Err(); err != nil {
		return Result{}, errors.New("import canceled")
	}
	if err := commit(ctx, cred, append([]Binding(nil), bindings...)); err != nil {
		return Result{}, errors.New("credential storage failed")
	}
	return Result{AppID: c.AppID, Organizations: bindings}, nil
}

func parseCredential(c Candidate) (Credential, error) {
	fail := errors.New("invalid App private key")
	if c.AppID < 1 || len(c.PEM) > 32*1024 {
		return Credential{}, fail
	}
	block, rest := pem.Decode(c.PEM)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 || len(block.Headers) != 0 {
		return Credential{}, fail
	}
	var key *rsa.PrivateKey
	var err error
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		key, _ = parsed.(*rsa.PrivateKey)
	default:
		return Credential{}, fail
	}
	if err != nil || key == nil || key.N.BitLen() < 2048 || key.Validate() != nil {
		return Credential{}, fail
	}
	return Credential{AppID: c.AppID, key: key}, nil
}

func minimalPermissions(permissions map[string]string) bool {
	if permissions["organization_self_hosted_runners"] != "write" {
		return false
	}
	for name, value := range permissions {
		if name == "organization_self_hosted_runners" {
			continue
		}
		if name != "metadata" || value != "read" {
			return false
		}
	}
	return true
}
