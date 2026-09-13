// Package credentials contains the offline R1 manual credential boundary.
//
// It deliberately has no filesystem, process, Keychain, GitHub or worker
// integration. A production caller must provide an independently reviewed
// adapter for those effects after this identity contract is accepted.
package credentials

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"regexp"
	"strings"
	"time"
)

const maxCredentialBytes = 32 * 1024

var (
	// Errors are intentionally stable and do not expose adapter/source details.
	ErrConfig       = errors.New("invalid credential configuration")
	ErrSource       = errors.New("invalid credential source")
	ErrExpired      = errors.New("expired credential source")
	ErrCredential   = errors.New("invalid credential")
	ErrAppIdentity  = errors.New("App identity verification failed")
	ErrInstallation = errors.New("installation identity verification failed")
	ErrRepository   = errors.New("repository identity verification failed")
	ErrCommit       = errors.New("credential commit failed")
	ErrCanceled     = errors.New("credential validation canceled")

	organizationLoginPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	repositoryNamePattern    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// SourceKind identifies the explicitly supported source boundary. R1 accepts
// only manual input held by the caller; file, environment and Keychain import
// are intentionally not implicit behaviors of this package.
type SourceKind string

const (
	SourceManual      SourceKind = "manual"
	SourceFile        SourceKind = "file"
	SourceEnvironment SourceKind = "environment"
	SourceKeychain    SourceKind = "keychain"
)

// CredentialSource supplies an App private key to the foreground controller.
// Implementations must keep the key in memory and must not persist or log it.
// Validate reads it once, after checking the non-secret source metadata.
type CredentialSource interface {
	Kind() SourceKind
	AppID() int64
	ExpiresAt() time.Time
	Read(context.Context) ([]byte, error)
}

// ManualSource is an in-memory source intended for explicit operator input and
// fixture tests. It never reads or writes a path, environment variable or
// Keychain item.
type ManualSource struct {
	appID     int64
	expiresAt time.Time
	pem       []byte
}

// NewManualSource copies pemBytes into memory and returns an explicitly manual
// source. A zero expiry denotes a static App key; a non-zero expiry is useful
// for callers that wrap a short-lived credential envelope.
func NewManualSource(appID int64, pemBytes []byte) CredentialSource {
	return NewManualSourceWithExpiry(appID, pemBytes, time.Time{})
}

// NewManualSourceWithExpiry is NewManualSource with an explicit source expiry.
func NewManualSourceWithExpiry(appID int64, pemBytes []byte, expiresAt time.Time) CredentialSource {
	return ManualSource{appID: appID, expiresAt: expiresAt, pem: append([]byte(nil), pemBytes...)}
}

func (s ManualSource) Kind() SourceKind     { return SourceManual }
func (s ManualSource) AppID() int64         { return s.appID }
func (s ManualSource) ExpiresAt() time.Time { return s.expiresAt }

func (s ManualSource) Read(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		return nil, ErrCanceled
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), s.pem...), nil
}

func (ManualSource) String() string   { return "[redacted manual credential source]" }
func (ManualSource) GoString() string { return "[redacted manual credential source]" }
func (ManualSource) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted manual credential source]"`), nil
}

// Organization is the configured target organization identity.
type Organization struct {
	Login string
	ID    int64
}

// Repository is the configured private repository identity. OwnerID and
// OwnerLogin must identify the configured organization.
type Repository struct {
	ID         int64
	OwnerID    int64
	OwnerLogin string
	Name       string
	Private    bool
}

// Config contains the complete one-organization R1 binding. The repository is
// intentionally explicit so a valid installation cannot be redirected to a
// different owner or repository.
type Config struct {
	AppID          int64
	Organization   Organization
	InstallationID int64
	Repository     Repository
}

// AppIdentity is the non-secret identity returned by App verification.
type AppIdentity struct {
	ID int64
}

// Installation is the non-secret organization installation identity returned
// by the adapter. SuspensionKnown is required to avoid treating an omitted
// suspension field as proof of an active installation.
type Installation struct {
	ID              int64
	AppID           int64
	AccountID       int64
	Login           string
	AccountType     string
	TargetID        int64
	TargetType      string
	Permissions     map[string]string
	Suspended       bool
	SuspensionKnown bool
}

// CredentialRef is the only credential-shaped value exposed to the adapter.
// It contains an App ID and a public-key fingerprint, never private key bytes.
type CredentialRef struct {
	appID       int64
	fingerprint [32]byte
}

// AppID returns the configured App identity without exposing key material.
func (r CredentialRef) AppID() int64 { return r.appID }

// PublicKeyFingerprint identifies the parsed key without returning the key.
func (r CredentialRef) PublicKeyFingerprint() [32]byte { return r.fingerprint }

func (CredentialRef) String() string   { return "[redacted credential reference]" }
func (CredentialRef) GoString() string { return "[redacted credential reference]" }
func (CredentialRef) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted credential reference]"`), nil
}

// API is the narrow identity adapter used by Validate. Production adapters
// must keep SDK/authentication details behind these calls. Tests implement it
// with fixture responses and perform no network requests.
type API interface {
	VerifyApp(context.Context, CredentialRef) (AppIdentity, error)
	VerifyInstallation(context.Context, CredentialRef, Organization) (Installation, error)
	VerifyRepository(context.Context, CredentialRef, Organization, Repository) (Repository, error)
}

// ValidatedBinding is intentionally metadata-only. It has no private key,
// source, JWT, token or worker bootstrap field, so a commit callback cannot
// receive management credentials through this R1 boundary.
type ValidatedBinding struct {
	AppID          int64
	Organization   Organization
	InstallationID int64
	Repository     Repository
	Permissions    map[string]string
}

// Commit receives an all-or-nothing metadata binding after every identity
// check succeeds. A nil callback performs validation only; this is the offline
// fixture mode and does not persist credentials.
type Commit func(context.Context, ValidatedBinding) error

// Validate checks one manually supplied credential source and verifies the
// configured App, organization installation and private repository in order.
// No callback or external effect is reached until all checks succeed.
func Validate(ctx context.Context, config Config, source CredentialSource, api API, commit Commit) (ValidatedBinding, error) {
	return ValidateAt(ctx, time.Now(), config, source, api, commit)
}

// ValidateAt is Validate with an injected clock for deterministic offline
// tests. It does not contact GitHub; api is expected to be a fixture adapter
// in this module.
func ValidateAt(ctx context.Context, now time.Time, config Config, source CredentialSource, api API, commit Commit) (ValidatedBinding, error) {
	if err := contextStatus(ctx); err != nil {
		return ValidatedBinding{}, err
	}
	config, err := normalizeConfig(config)
	if err != nil {
		return ValidatedBinding{}, ErrConfig
	}
	if api == nil {
		return ValidatedBinding{}, ErrConfig
	}
	ref, err := credentialReference(ctx, now, config, source)
	if err != nil {
		return ValidatedBinding{}, err
	}

	app, err := api.VerifyApp(ctx, ref)
	if err != nil {
		if contextStatus(ctx) != nil {
			return ValidatedBinding{}, ErrCanceled
		}
		return ValidatedBinding{}, ErrAppIdentity
	}
	if err := contextStatus(ctx); err != nil {
		return ValidatedBinding{}, err
	}
	if app.ID != config.AppID {
		return ValidatedBinding{}, ErrAppIdentity
	}

	installation, err := api.VerifyInstallation(ctx, ref, config.Organization)
	if err != nil {
		if contextStatus(ctx) != nil {
			return ValidatedBinding{}, ErrCanceled
		}
		return ValidatedBinding{}, ErrInstallation
	}
	if err := contextStatus(ctx); err != nil {
		return ValidatedBinding{}, err
	}
	if !validInstallation(config, installation) {
		return ValidatedBinding{}, ErrInstallation
	}

	repository, err := api.VerifyRepository(ctx, ref, config.Organization, config.Repository)
	if err != nil {
		if contextStatus(ctx) != nil {
			return ValidatedBinding{}, ErrCanceled
		}
		return ValidatedBinding{}, ErrRepository
	}
	if err := contextStatus(ctx); err != nil {
		return ValidatedBinding{}, err
	}
	if !validRepository(config, repository) {
		return ValidatedBinding{}, ErrRepository
	}

	binding := ValidatedBinding{
		AppID:          config.AppID,
		Organization:   config.Organization,
		InstallationID: config.InstallationID,
		Repository:     config.Repository,
		Permissions:    clonePermissions(installation.Permissions),
	}
	if err := contextStatus(ctx); err != nil {
		return ValidatedBinding{}, err
	}
	if commit != nil {
		commitBinding := binding
		commitBinding.Permissions = clonePermissions(binding.Permissions)
		if err := commit(ctx, commitBinding); err != nil {
			return ValidatedBinding{}, ErrCommit
		}
		if err := contextStatus(ctx); err != nil {
			return ValidatedBinding{}, err
		}
	}
	return binding, nil
}

func credentialReference(ctx context.Context, now time.Time, config Config, source CredentialSource) (CredentialRef, error) {
	if source == nil {
		return CredentialRef{}, ErrSource
	}
	kind, appID, expiresAt := source.Kind(), source.AppID(), source.ExpiresAt()
	if kind != SourceManual || appID != config.AppID || appID < 1 {
		return CredentialRef{}, ErrSource
	}
	if !expiresAt.IsZero() && !now.Before(expiresAt) {
		return CredentialRef{}, ErrExpired
	}
	if err := contextStatus(ctx); err != nil {
		return CredentialRef{}, err
	}
	pemBytes, err := source.Read(ctx)
	if err != nil {
		if contextStatus(ctx) != nil {
			return CredentialRef{}, ErrCanceled
		}
		return CredentialRef{}, ErrSource
	}
	if err := contextStatus(ctx); err != nil {
		return CredentialRef{}, err
	}
	fingerprint, err := fingerprintPrivateKey(pemBytes)
	if err != nil {
		return CredentialRef{}, ErrCredential
	}
	return CredentialRef{appID: config.AppID, fingerprint: fingerprint}, nil
}

func contextStatus(ctx context.Context) error {
	if ctx == nil {
		return ErrCanceled
	}
	if err := ctx.Err(); err != nil {
		return ErrCanceled
	}
	return nil
}

func normalizeConfig(config Config) (Config, error) {
	if config.AppID < 1 || config.InstallationID < 1 || config.Organization.ID < 1 || config.Repository.ID < 1 || config.Repository.OwnerID != config.Organization.ID || !config.Repository.Private {
		return Config{}, ErrConfig
	}
	config.Organization.Login = strings.ToLower(config.Organization.Login)
	config.Repository.OwnerLogin = strings.ToLower(config.Repository.OwnerLogin)
	if !organizationLoginPattern.MatchString(config.Organization.Login) || !organizationLoginPattern.MatchString(config.Repository.OwnerLogin) || config.Repository.OwnerLogin != config.Organization.Login || !repositoryNamePattern.MatchString(config.Repository.Name) || config.Repository.Name == "." || config.Repository.Name == ".." {
		return Config{}, ErrConfig
	}
	return config, nil
}

func fingerprintPrivateKey(data []byte) ([32]byte, error) {
	if len(data) == 0 || len(data) > maxCredentialBytes {
		return [32]byte{}, ErrCredential
	}
	block, rest := pem.Decode(data)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 || len(block.Headers) != 0 {
		return [32]byte{}, ErrCredential
	}
	var (
		key *rsa.PrivateKey
		err error
	)
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		key, _ = parsed.(*rsa.PrivateKey)
	default:
		return [32]byte{}, ErrCredential
	}
	if err != nil || key == nil || key.N == nil || key.N.BitLen() < 2048 || key.Validate() != nil {
		return [32]byte{}, ErrCredential
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return [32]byte{}, ErrCredential
	}
	return sha256.Sum256(publicKey), nil
}

func validInstallation(config Config, installation Installation) bool {
	return installation.ID == config.InstallationID &&
		installation.AppID == config.AppID &&
		installation.AccountID == config.Organization.ID &&
		strings.EqualFold(installation.Login, config.Organization.Login) &&
		installation.AccountType == "Organization" &&
		installation.TargetID == config.Organization.ID &&
		installation.TargetType == "Organization" &&
		installation.SuspensionKnown &&
		!installation.Suspended &&
		minimalPermissions(installation.Permissions)
}

func validRepository(config Config, repository Repository) bool {
	return repository.ID == config.Repository.ID &&
		repository.OwnerID == config.Organization.ID &&
		strings.EqualFold(repository.OwnerLogin, config.Organization.Login) &&
		repository.Name == config.Repository.Name &&
		repository.Private
}

func minimalPermissions(permissions map[string]string) bool {
	return len(permissions) == 2 &&
		permissions["organization_self_hosted_runners"] == "write" &&
		permissions["metadata"] == "read"
}

func clonePermissions(permissions map[string]string) map[string]string {
	return map[string]string{
		"organization_self_hosted_runners": permissions["organization_self_hosted_runners"],
		"metadata":                         permissions["metadata"],
	}
}
