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
	"reflect"
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

// manualCredentialSource is an unexported capability marker. Kind is retained
// as descriptive metadata, but a caller-provided implementation is not a
// manual source unless it also has this package-private marker.
type manualCredentialSource interface {
	CredentialSource
	credentialSourceMarker()
}

// ManualSource is the exported in-memory source shape used by the manual
// source constructor. It intentionally has no capability marker: embedding
// this shape in another package must not authorize the embedded type as a
// package-created manual source. Use NewManualSource or
// NewManualSourceWithExpiry to obtain a CredentialSource accepted by Validate.
// It never reads or writes a path, environment variable or Keychain item.
type ManualSource struct {
	appID     int64
	expiresAt time.Time
	pem       []byte
	oversized bool
}

// manualSource is the package-created capability returned by the constructors.
// Keeping the marker on this unexported concrete type prevents an external
// package from promoting it through embedding and replacing its behavior.
type manualSource struct {
	ManualSource
}

// NewManualSource copies a bounded pemBytes value into memory and returns an
// explicitly manual source. Oversized input is retained only as an invalid
// marker and is rejected by Read before any key bytes are copied. A zero expiry
// denotes a static App key; a non-zero expiry is useful for callers that wrap a
// short-lived credential envelope.
func NewManualSource(appID int64, pemBytes []byte) CredentialSource {
	return NewManualSourceWithExpiry(appID, pemBytes, time.Time{})
}

// NewManualSourceWithExpiry is NewManualSource with an explicit source expiry.
func NewManualSourceWithExpiry(appID int64, pemBytes []byte, expiresAt time.Time) CredentialSource {
	source := ManualSource{appID: appID, expiresAt: expiresAt}
	if len(pemBytes) > maxCredentialBytes {
		source.oversized = true
	} else {
		source.pem = append([]byte(nil), pemBytes...)
	}
	return manualSource{ManualSource: source}
}

func (manualSource) credentialSourceMarker() {}

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
	if s.oversized {
		return nil, ErrCredential
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
// check succeeds. The callback must honor ctx before and during its operation
// and apply the metadata transactionally: it must not expose a partial binding
// if it returns an error or observes cancellation. For a non-expiring source,
// ctx retains the caller's cancellation and deadline. For an expiring source,
// ctx has the earlier of the caller's deadline and the source expiry, so a
// callback can stop before applying state at the credential boundary. A
// callback may perform an external side effect, but this boundary does not
// claim that such a side effect can be rolled back. A nil callback performs
// validation only; this is the offline fixture mode and does not persist
// credentials.
type Commit func(context.Context, ValidatedBinding) error

// Validate checks one manually supplied credential source and verifies the
// configured App, organization installation and private repository in order.
// No callback or external effect is reached until all checks succeed.
func Validate(ctx context.Context, config Config, source CredentialSource, api API, commit Commit) (ValidatedBinding, error) {
	return validateWithClock(ctx, time.Now, config, source, api, commit)
}

// validateAt is a package-private deterministic clock hook for offline tests.
// Production callers must use Validate so each boundary reads the trusted
// process clock instead of supplying an arbitrary stale time.
func validateAt(ctx context.Context, now time.Time, config Config, source CredentialSource, api API, commit Commit) (ValidatedBinding, error) {
	return validateWithClock(ctx, func() time.Time { return now }, config, source, api, commit)
}

func validateWithClock(ctx context.Context, now func() time.Time, config Config, source CredentialSource, api API, commit Commit) (ValidatedBinding, error) {
	if err := contextStatus(ctx); err != nil {
		return ValidatedBinding{}, err
	}
	config, err := normalizeConfig(config)
	if err != nil {
		return ValidatedBinding{}, ErrConfig
	}
	if isNilInterface(api) {
		return ValidatedBinding{}, ErrConfig
	}
	ref, expiresAt, err := credentialReference(ctx, now, config, source)
	if err != nil {
		return ValidatedBinding{}, err
	}
	validationCtx, cancelValidation := contextWithExpiry(ctx, expiresAt)
	defer cancelValidation()

	if err := boundedBoundaryStatus(ctx, validationCtx, now, expiresAt); err != nil {
		return ValidatedBinding{}, err
	}
	app, err := api.VerifyApp(validationCtx, ref)
	if boundaryErr := boundedBoundaryStatus(ctx, validationCtx, now, expiresAt); boundaryErr != nil {
		return ValidatedBinding{}, boundaryErr
	}
	if err != nil {
		return ValidatedBinding{}, ErrAppIdentity
	}
	if app.ID != config.AppID {
		return ValidatedBinding{}, ErrAppIdentity
	}

	if err := boundedBoundaryStatus(ctx, validationCtx, now, expiresAt); err != nil {
		return ValidatedBinding{}, err
	}
	installation, err := api.VerifyInstallation(validationCtx, ref, config.Organization)
	if boundaryErr := boundedBoundaryStatus(ctx, validationCtx, now, expiresAt); boundaryErr != nil {
		return ValidatedBinding{}, boundaryErr
	}
	if err != nil {
		return ValidatedBinding{}, ErrInstallation
	}
	if !validInstallation(config, installation) {
		return ValidatedBinding{}, ErrInstallation
	}

	if err := boundedBoundaryStatus(ctx, validationCtx, now, expiresAt); err != nil {
		return ValidatedBinding{}, err
	}
	repository, err := api.VerifyRepository(validationCtx, ref, config.Organization, config.Repository)
	if boundaryErr := boundedBoundaryStatus(ctx, validationCtx, now, expiresAt); boundaryErr != nil {
		return ValidatedBinding{}, boundaryErr
	}
	if err != nil {
		return ValidatedBinding{}, ErrRepository
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
	if err := boundedBoundaryStatus(ctx, validationCtx, now, expiresAt); err != nil {
		return ValidatedBinding{}, err
	}
	if commit != nil {
		commitBinding := binding
		commitBinding.Permissions = clonePermissions(binding.Permissions)
		if err := boundedBoundaryStatus(ctx, validationCtx, now, expiresAt); err != nil {
			return ValidatedBinding{}, err
		}
		if err := commit(validationCtx, commitBinding); err != nil {
			if boundaryErr := commitBoundaryStatus(ctx, now, validationCtx, expiresAt); boundaryErr != nil {
				return ValidatedBinding{}, boundaryErr
			}
			return ValidatedBinding{}, ErrCommit
		}
		if err := commitBoundaryStatus(ctx, now, validationCtx, expiresAt); err != nil {
			return ValidatedBinding{}, err
		}
	}
	return binding, nil
}

func credentialReference(ctx context.Context, now func() time.Time, config Config, source CredentialSource) (CredentialRef, time.Time, error) {
	if isNilInterface(source) {
		return CredentialRef{}, time.Time{}, ErrSource
	}
	manual, ok := source.(manualCredentialSource)
	if !ok {
		return CredentialRef{}, time.Time{}, ErrSource
	}
	kind, appID, expiresAt := manual.Kind(), manual.AppID(), manual.ExpiresAt()
	if kind != SourceManual || appID != config.AppID || appID < 1 {
		return CredentialRef{}, time.Time{}, ErrSource
	}
	if sourceExpired(now(), expiresAt) {
		return CredentialRef{}, time.Time{}, ErrExpired
	}
	if err := contextStatus(ctx); err != nil {
		return CredentialRef{}, time.Time{}, err
	}
	pemBytes, err := manual.Read(ctx)
	if err != nil {
		if contextStatus(ctx) != nil {
			return CredentialRef{}, time.Time{}, ErrCanceled
		}
		if errors.Is(err, ErrCredential) {
			return CredentialRef{}, time.Time{}, ErrCredential
		}
		return CredentialRef{}, time.Time{}, ErrSource
	}
	if err := contextStatus(ctx); err != nil {
		return CredentialRef{}, time.Time{}, err
	}
	// Read may cross the source's expiry boundary, and a same-package fixture
	// can update its envelope metadata while reading. Re-read rather than
	// relying on the pre-read snapshot before deriving the adapter reference.
	expiresAt = manual.ExpiresAt()
	if sourceExpired(now(), expiresAt) {
		return CredentialRef{}, time.Time{}, ErrExpired
	}
	fingerprint, err := fingerprintPrivateKey(pemBytes)
	if err != nil {
		return CredentialRef{}, time.Time{}, ErrCredential
	}
	return CredentialRef{appID: config.AppID, fingerprint: fingerprint}, expiresAt, nil
}

func boundaryStatus(ctx context.Context, now func() time.Time, expiresAt time.Time) error {
	if err := contextStatus(ctx); err != nil {
		return err
	}
	if sourceExpired(now(), expiresAt) {
		return ErrExpired
	}
	return nil
}

func boundedBoundaryStatus(parent context.Context, bounded context.Context, now func() time.Time, expiresAt time.Time) error {
	if err := boundaryStatus(parent, now, expiresAt); err != nil {
		return err
	}
	if !expiresAt.IsZero() && errors.Is(bounded.Err(), context.DeadlineExceeded) {
		return ErrExpired
	}
	return nil
}

func contextWithExpiry(parent context.Context, expiresAt time.Time) (context.Context, context.CancelFunc) {
	if expiresAt.IsZero() {
		return parent, func() {}
	}
	return context.WithDeadline(parent, expiresAt)
}

func commitBoundaryStatus(ctx context.Context, now func() time.Time, commitCtx context.Context, expiresAt time.Time) error {
	return boundedBoundaryStatus(ctx, commitCtx, now, expiresAt)
}

func sourceExpired(now time.Time, expiresAt time.Time) bool {
	return !expiresAt.IsZero() && !now.Before(expiresAt)
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
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
