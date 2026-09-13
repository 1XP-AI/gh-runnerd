package credentials

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const fixtureAppID int64 = 71

var fixtureNow = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

type fixtureAPI struct {
	app                 AppIdentity
	installation        Installation
	repository          Repository
	appErr              error
	installErr          error
	repositoryErr       error
	calls               []string
	fingerprints        [][32]byte
	expectedFingerprint [32]byte
	boundFingerprint    [32]byte
}

func (f *fixtureAPI) VerifyApp(_ context.Context, credential CredentialRef) (AppIdentity, error) {
	f.calls = append(f.calls, "app")
	if err := f.checkCredential(credential); err != nil {
		return AppIdentity{}, err
	}
	if f.appErr != nil {
		return AppIdentity{}, f.appErr
	}
	return f.app, nil
}

func (f *fixtureAPI) VerifyInstallation(_ context.Context, credential CredentialRef, organization Organization) (Installation, error) {
	f.calls = append(f.calls, "installation:"+organization.Login)
	if err := f.checkCredential(credential); err != nil {
		return Installation{}, err
	}
	if f.installErr != nil {
		return Installation{}, f.installErr
	}
	return f.installation, nil
}

func (f *fixtureAPI) VerifyRepository(_ context.Context, credential CredentialRef, organization Organization, repository Repository) (Repository, error) {
	f.calls = append(f.calls, "repository:"+organization.Login+"/"+repository.Name)
	if err := f.checkCredential(credential); err != nil {
		return Repository{}, err
	}
	if f.repositoryErr != nil {
		return Repository{}, f.repositoryErr
	}
	return f.repository, nil
}

func (f *fixtureAPI) checkCredential(credential CredentialRef) error {
	fingerprints := credential.PublicKeyFingerprint()
	f.fingerprints = append(f.fingerprints, fingerprints)
	if credential.AppID() != fixtureAppID {
		return errors.New("fixture credential mismatch")
	}
	if f.expectedFingerprint != ([32]byte{}) && fingerprints != f.expectedFingerprint {
		return errors.New("fixture credential mismatch")
	}
	if f.boundFingerprint == ([32]byte{}) {
		f.boundFingerprint = fingerprints
	}
	if fingerprints != f.boundFingerprint {
		return errors.New("fixture credential mismatch")
	}
	return nil
}

type sourceFixture struct {
	kind      SourceKind
	appID     int64
	expiresAt time.Time
	pem       []byte
	err       error
	onRead    func()
	reads     int
}

type spoofedManualSource struct {
	appID     int64
	expiresAt time.Time
	pem       []byte
	reads     int
}

func (s *spoofedManualSource) Kind() SourceKind     { return SourceManual }
func (s *spoofedManualSource) AppID() int64         { return s.appID }
func (s *spoofedManualSource) ExpiresAt() time.Time { return s.expiresAt }
func (s *spoofedManualSource) Read(context.Context) ([]byte, error) {
	s.reads++
	return append([]byte(nil), s.pem...), nil
}

type cancelOnErrContext struct {
	context.Context
	cancelAt int
	checks   int
	done     chan struct{}
	canceled bool
}

func newCancelOnErrContext(parent context.Context, cancelAt int) *cancelOnErrContext {
	return &cancelOnErrContext{Context: parent, cancelAt: cancelAt, done: make(chan struct{})}
}

func (c *cancelOnErrContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		if !c.canceled {
			close(c.done)
			c.canceled = true
		}
		return context.Canceled
	}
	return nil
}

func (c *cancelOnErrContext) Done() <-chan struct{} { return c.done }

func (s *sourceFixture) Kind() SourceKind      { return s.kind }
func (s *sourceFixture) AppID() int64          { return s.appID }
func (s *sourceFixture) ExpiresAt() time.Time  { return s.expiresAt }
func (*sourceFixture) credentialSourceMarker() {}
func (s *sourceFixture) Read(context.Context) ([]byte, error) {
	s.reads++
	if s.onRead != nil {
		s.onRead()
	}
	if s.err != nil {
		return nil, s.err
	}
	return append([]byte(nil), s.pem...), nil
}

func fixtureConfig() Config {
	return Config{
		AppID:          fixtureAppID,
		Organization:   Organization{Login: "acme", ID: 101},
		InstallationID: 201,
		Repository:     Repository{ID: 301, OwnerID: 101, OwnerLogin: "acme", Name: "private-runner-fixture", Private: true},
	}
}

func fixtureAPIValue() *fixtureAPI {
	return &fixtureAPI{
		app: AppIdentity{ID: fixtureAppID},
		installation: Installation{
			ID: 201, AppID: fixtureAppID, AccountID: 101, Login: "acme", AccountType: "Organization",
			TargetID: 101, TargetType: "Organization", Permissions: map[string]string{
				"organization_self_hosted_runners": "write",
				"metadata":                         "read",
			}, SuspensionKnown: true,
		},
		repository: Repository{ID: 301, OwnerID: 101, OwnerLogin: "acme", Name: "private-runner-fixture", Private: true},
	}
}

func fixturePEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("fixture key generation failed")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func fixtureSource(t *testing.T) CredentialSource {
	t.Helper()
	return NewManualSource(fixtureAppID, fixturePEM(t))
}

func TestValidateManualSingleOrganizationBindsIdentityBeforeCommit(t *testing.T) {
	source := fixtureSource(t)
	api := fixtureAPIValue()
	var committed ValidatedBinding
	commits := 0
	got, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), source, api, func(_ context.Context, binding ValidatedBinding) error {
		commits++
		committed = binding
		return nil
	})
	if err != nil {
		t.Fatalf("manual fixture rejected: %v", err)
	}
	want := ValidatedBinding{
		AppID:          fixtureAppID,
		Organization:   Organization{Login: "acme", ID: 101},
		InstallationID: 201,
		Repository:     Repository{ID: 301, OwnerID: 101, OwnerLogin: "acme", Name: "private-runner-fixture", Private: true},
		Permissions:    map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"},
	}
	if got.AppID != want.AppID || got.Organization != want.Organization || got.InstallationID != want.InstallationID || got.Repository != want.Repository || !reflect.DeepEqual(got.Permissions, want.Permissions) {
		t.Fatalf("binding mismatch: got=%+v want=%+v", got, want)
	}
	if committed.AppID != got.AppID || committed.Organization != got.Organization || committed.InstallationID != got.InstallationID || committed.Repository != got.Repository || !reflect.DeepEqual(committed.Permissions, got.Permissions) {
		t.Fatalf("commit received a different binding: got=%+v committed=%+v", got, committed)
	}
	if commits != 1 || !reflect.DeepEqual(api.calls, []string{"app", "installation:acme", "repository:acme/private-runner-fixture"}) {
		t.Fatalf("unexpected fixture sequence: commits=%d calls=%v", commits, api.calls)
	}
	data, marshalErr := json.Marshal(committed)
	if marshalErr != nil {
		t.Fatal("validated binding should be serializable")
	}
	if strings.Contains(string(data), "PRIVATE KEY") {
		t.Fatal("validated binding serialized private key material")
	}
}

func TestValidationRejectsWrongValidKeyForBoundIdentity(t *testing.T) {
	trustedPEM := fixturePEM(t)
	wantFingerprint, err := fingerprintPrivateKey(trustedPEM)
	if err != nil {
		t.Fatal("trusted fixture key fingerprint failed")
	}
	api := fixtureAPIValue()
	api.expectedFingerprint = wantFingerprint
	wrongPEM := fixturePEM(t)
	_, err = validateAt(context.Background(), fixtureNow, fixtureConfig(), NewManualSource(fixtureAppID, wrongPEM), api, nil)
	if !errors.Is(err, ErrAppIdentity) || !reflect.DeepEqual(api.calls, []string{"app"}) {
		t.Fatalf("unrelated valid key was accepted: err=%v calls=%v", err, api.calls)
	}
}

func TestValidationRejectsUnmarkedManualSource(t *testing.T) {
	source := &spoofedManualSource{appID: fixtureAppID, pem: fixturePEM(t)}
	api := fixtureAPIValue()
	_, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), source, api, nil)
	if !errors.Is(err, ErrSource) || source.reads != 0 || len(api.calls) != 0 {
		t.Fatalf("unmarked manual source crossed boundary: err=%v reads=%d calls=%v", err, source.reads, api.calls)
	}
}

func TestValidationRejectsSourceThatExpiresDuringRead(t *testing.T) {
	var source *sourceFixture
	source = &sourceFixture{
		kind:      SourceManual,
		appID:     fixtureAppID,
		expiresAt: fixtureNow.Add(time.Hour),
		pem:       fixturePEM(t),
		onRead: func() {
			source.expiresAt = fixtureNow.Add(-time.Nanosecond)
		},
	}
	api := fixtureAPIValue()
	_, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), source, api, nil)
	if !errors.Is(err, ErrExpired) || source.reads != 1 || len(api.calls) != 0 {
		t.Fatalf("late-expiring source crossed boundary: err=%v reads=%d calls=%v", err, source.reads, api.calls)
	}
}

func TestValidateUsesTrustedCurrentClockForExpiry(t *testing.T) {
	source := NewManualSourceWithExpiry(fixtureAppID, fixturePEM(t), time.Now().Add(-time.Second))
	api := fixtureAPIValue()
	_, err := Validate(context.Background(), fixtureConfig(), source, api, nil)
	if !errors.Is(err, ErrExpired) || len(api.calls) != 0 {
		t.Fatalf("production validation used a stale clock: err=%v calls=%v", err, api.calls)
	}
}

func TestValidationChecksCancellationImmediatelyBeforeAppBoundary(t *testing.T) {
	ctx := newCancelOnErrContext(context.Background(), 4)
	source := &sourceFixture{kind: SourceManual, appID: fixtureAppID, pem: fixturePEM(t)}
	api := fixtureAPIValue()
	_, err := validateAt(ctx, fixtureNow, fixtureConfig(), source, api, nil)
	if !errors.Is(err, ErrCanceled) || len(api.calls) != 0 {
		t.Fatalf("canceled API boundary crossed remote effect: err=%v calls=%v", err, api.calls)
	}
}

func TestValidationReportsCancellationAfterCommitCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := fixtureAPIValue()
	commitCalls := 0
	_, err := validateAt(ctx, fixtureNow, fixtureConfig(), fixtureSource(t), api, func(callbackCtx context.Context, _ ValidatedBinding) error {
		commitCalls++
		if callbackCtx == nil {
			t.Fatal("commit callback received a nil context")
		}
		cancel()
		return nil
	})
	if !errors.Is(err, ErrCanceled) || commitCalls != 1 {
		t.Fatalf("commit cancellation was not reported at boundary: err=%v calls=%d", err, commitCalls)
	}
}

func TestValidationCommitUsesCredentialExpiryDeadlineAndAbortsAtBoundary(t *testing.T) {
	pemBytes := fixturePEM(t)
	expiresAt := time.Now().Add(300 * time.Millisecond)
	source := NewManualSourceWithExpiry(fixtureAppID, pemBytes, expiresAt)
	api := fixtureAPIValue()
	var callbackDeadline time.Time
	var callbackErr error
	commitApplied := false

	got, err := Validate(context.Background(), fixtureConfig(), source, api, func(callbackCtx context.Context, _ ValidatedBinding) error {
		var ok bool
		callbackDeadline, ok = callbackCtx.Deadline()
		if !ok {
			return errors.New("commit context has no expiry deadline")
		}
		if !callbackDeadline.Equal(expiresAt) {
			return errors.New("commit context deadline does not match credential expiry")
		}
		<-callbackCtx.Done()
		callbackErr = callbackCtx.Err()
		if !errors.Is(callbackErr, context.DeadlineExceeded) {
			return errors.New("commit context ended for an unexpected reason")
		}
		if callbackCtx.Err() == nil {
			commitApplied = true
		}
		return callbackErr
	})
	if !errors.Is(err, ErrExpired) || !reflect.DeepEqual(got, ValidatedBinding{}) {
		t.Fatalf("expiry during commit was not terminal: binding=%+v err=%v", got, err)
	}
	if !callbackDeadline.Equal(expiresAt) || !errors.Is(callbackErr, context.DeadlineExceeded) || commitApplied {
		t.Fatalf("commit callback did not abort transactionally at expiry: deadline=%v want=%v callbackErr=%v applied=%t", callbackDeadline, expiresAt, callbackErr, commitApplied)
	}
}

func TestValidationRejectsTypedNilSourceAndAdapter(t *testing.T) {
	var nilSource *sourceFixture
	var source CredentialSource = nilSource
	api := fixtureAPIValue()
	_, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), source, api, nil)
	if !errors.Is(err, ErrSource) || len(api.calls) != 0 {
		t.Fatalf("typed-nil source crossed validation boundary: err=%v calls=%v", err, api.calls)
	}

	var nilAPI *fixtureAPI
	var typedNilAPI API = nilAPI
	source = &sourceFixture{kind: SourceManual, appID: fixtureAppID, pem: fixturePEM(t)}
	_, err = validateAt(context.Background(), fixtureNow, fixtureConfig(), source, typedNilAPI, nil)
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("typed-nil adapter crossed validation boundary: err=%v", err)
	}
}

func TestValidationRejectsInvalidSourceBeforeAnyRemoteEffect(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Config, *sourceFixture)
		want   error
	}{
		{name: "wrong source kind", mutate: func(_ *Config, s *sourceFixture) { s.kind = SourceFile }, want: ErrSource},
		{name: "source App mismatch", mutate: func(_ *Config, s *sourceFixture) { s.appID = 999 }, want: ErrSource},
		{name: "expired source", mutate: func(_ *Config, s *sourceFixture) { s.expiresAt = fixtureNow.Add(-time.Nanosecond) }, want: ErrExpired},
		{name: "source read failure", mutate: func(_ *Config, s *sourceFixture) { s.err = errors.New("fixture-private-key-body") }, want: ErrSource},
		{name: "malformed key", mutate: func(_ *Config, s *sourceFixture) { s.pem = []byte("fixture-private-key") }, want: ErrCredential},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := fixtureConfig()
			source := &sourceFixture{kind: SourceManual, appID: fixtureAppID, pem: fixturePEM(t)}
			tc.mutate(&config, source)
			api := fixtureAPIValue()
			commits := 0
			_, err := validateAt(context.Background(), fixtureNow, config, source, api, func(context.Context, ValidatedBinding) error {
				commits++
				return nil
			})
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("invalid source accepted or wrong error: %v", err)
			}
			if commits != 0 || len(api.calls) != 0 {
				t.Fatalf("invalid source crossed boundary: commits=%d calls=%v", commits, api.calls)
			}
			if tc.name == "wrong source kind" || tc.name == "source App mismatch" || tc.name == "expired source" {
				if source.reads != 0 {
					t.Fatalf("source was read before metadata validation: reads=%d", source.reads)
				}
			}
			if strings.Contains(err.Error(), "fixture-private-key-body") || strings.Contains(err.Error(), "fixture-private-key") {
				t.Fatal("source error leaked into validation error")
			}
		})
	}
}

func TestValidationRejectsBadConfigurationBeforeReadingSource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "missing organization", mutate: func(c *Config) { c.Organization.ID = 0 }},
		{name: "organization repository mismatch", mutate: func(c *Config) { c.Repository.OwnerLogin = "other" }},
		{name: "unsafe repository name", mutate: func(c *Config) { c.Repository.Name = "../private" }},
		{name: "dot repository name", mutate: func(c *Config) { c.Repository.Name = "." }},
		{name: "dot-dot repository name", mutate: func(c *Config) { c.Repository.Name = ".." }},
		{name: "public repository requested", mutate: func(c *Config) { c.Repository.Private = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := fixtureConfig()
			tc.mutate(&config)
			source := &sourceFixture{kind: SourceManual, appID: fixtureAppID, pem: fixturePEM(t)}
			api := fixtureAPIValue()
			_, err := validateAt(context.Background(), fixtureNow, config, source, api, nil)
			if !errors.Is(err, ErrConfig) || source.reads != 0 || len(api.calls) != 0 {
				t.Fatalf("unsafe configuration crossed boundary: err=%v reads=%d calls=%v", err, source.reads, api.calls)
			}
		})
	}
}

func TestValidationPassesOnlyFingerprintToFixtureAdapter(t *testing.T) {
	pemBytes := fixturePEM(t)
	wantFingerprint, err := fingerprintPrivateKey(pemBytes)
	if err != nil {
		t.Fatal("fixture key fingerprint failed")
	}
	api := fixtureAPIValue()
	_, err = validateAt(context.Background(), fixtureNow, fixtureConfig(), NewManualSource(fixtureAppID, pemBytes), api, nil)
	if err != nil {
		t.Fatalf("manual fixture rejected: %v", err)
	}
	if len(api.fingerprints) != 3 {
		t.Fatalf("adapter did not receive the credential fingerprint at every identity boundary: got=%v", api.fingerprints)
	}
	for _, fingerprint := range api.fingerprints {
		if fingerprint != wantFingerprint {
			t.Fatalf("adapter received an inconsistent key fingerprint: got=%v want=%v", api.fingerprints, wantFingerprint)
		}
	}
}

func TestValidationRejectsNilSourceAndAdapterBeforeBoundary(t *testing.T) {
	api := fixtureAPIValue()
	_, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), nil, api, nil)
	if !errors.Is(err, ErrSource) || len(api.calls) != 0 {
		t.Fatalf("nil source crossed validation boundary: err=%v calls=%v", err, api.calls)
	}

	source := &sourceFixture{kind: SourceManual, appID: fixtureAppID, pem: fixturePEM(t)}
	_, err = validateAt(context.Background(), fixtureNow, fixtureConfig(), source, nil, nil)
	if !errors.Is(err, ErrConfig) || source.reads != 0 {
		t.Fatalf("nil adapter crossed source boundary: err=%v reads=%d", err, source.reads)
	}
}

func TestValidationRejectsOversizedCredentialBeforeFixture(t *testing.T) {
	source := &sourceFixture{kind: SourceManual, appID: fixtureAppID, pem: make([]byte, maxCredentialBytes+1)}
	api := fixtureAPIValue()
	_, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), source, api, nil)
	if !errors.Is(err, ErrCredential) || source.reads != 1 || len(api.calls) != 0 {
		t.Fatalf("oversized credential crossed fixture boundary: err=%v reads=%d calls=%v", err, source.reads, api.calls)
	}
}

func TestValidationRejectsIdentityAndPermissionDriftWithoutCommit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(*fixtureAPI)
		wantCalls []string
	}{
		{name: "wrong App", mutate: func(a *fixtureAPI) { a.app.ID = 72 }, wantCalls: []string{"app"}},
		{name: "wrong installation", mutate: func(a *fixtureAPI) { a.installation.ID = 999 }, wantCalls: []string{"app", "installation:acme"}},
		{name: "installation App mismatch", mutate: func(a *fixtureAPI) { a.installation.AppID = 999 }, wantCalls: []string{"app", "installation:acme"}},
		{name: "installation target mismatch", mutate: func(a *fixtureAPI) { a.installation.TargetID = 999 }, wantCalls: []string{"app", "installation:acme"}},
		{name: "suspended installation", mutate: func(a *fixtureAPI) { a.installation.Suspended = true }, wantCalls: []string{"app", "installation:acme"}},
		{name: "suspension unknown", mutate: func(a *fixtureAPI) { a.installation.SuspensionKnown = false }, wantCalls: []string{"app", "installation:acme"}},
		{name: "missing metadata permission", mutate: func(a *fixtureAPI) { delete(a.installation.Permissions, "metadata") }, wantCalls: []string{"app", "installation:acme"}},
		{name: "extra permission", mutate: func(a *fixtureAPI) { a.installation.Permissions["administration"] = "write" }, wantCalls: []string{"app", "installation:acme"}},
		{name: "repository ID mismatch", mutate: func(a *fixtureAPI) { a.repository.ID = 999 }, wantCalls: []string{"app", "installation:acme", "repository:acme/private-runner-fixture"}},
		{name: "wrong repository", mutate: func(a *fixtureAPI) { a.repository.OwnerLogin = "other" }, wantCalls: []string{"app", "installation:acme", "repository:acme/private-runner-fixture"}},
		{name: "public repository", mutate: func(a *fixtureAPI) { a.repository.Private = false }, wantCalls: []string{"app", "installation:acme", "repository:acme/private-runner-fixture"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := fixtureAPIValue()
			tc.mutate(api)
			commits := 0
			_, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), fixtureSource(t), api, func(context.Context, ValidatedBinding) error {
				commits++
				return nil
			})
			if err == nil || commits != 0 || !reflect.DeepEqual(api.calls, tc.wantCalls) {
				t.Fatalf("identity drift accepted: err=%v commits=%d calls=%v", err, commits, api.calls)
			}
		})
	}
}

func TestValidationNormalizesEveryAdapterFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(*fixtureAPI)
		want      error
		wantCalls []string
	}{
		{name: "App failure", mutate: func(a *fixtureAPI) { a.appErr = errors.New("fixture-secret-app-response") }, want: ErrAppIdentity, wantCalls: []string{"app"}},
		{name: "installation failure", mutate: func(a *fixtureAPI) { a.installErr = errors.New("fixture-secret-installation-response") }, want: ErrInstallation, wantCalls: []string{"app", "installation:acme"}},
		{name: "repository failure", mutate: func(a *fixtureAPI) { a.repositoryErr = errors.New("fixture-secret-repository-response") }, want: ErrRepository, wantCalls: []string{"app", "installation:acme", "repository:acme/private-runner-fixture"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := fixtureAPIValue()
			tc.mutate(api)
			_, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), fixtureSource(t), api, nil)
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(api.calls, tc.wantCalls) {
				t.Fatalf("adapter failure was not normalized: err=%v calls=%v", err, api.calls)
			}
			if strings.Contains(err.Error(), "fixture-secret-") {
				t.Fatal("adapter response details leaked into validation error")
			}
		})
	}
}

func TestValidationIsAtomicAndNormalizesBoundaryErrors(t *testing.T) {
	api := fixtureAPIValue()
	api.installErr = errors.New("fixture-private-key-api-body")
	commits := 0
	_, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), fixtureSource(t), api, func(context.Context, ValidatedBinding) error {
		commits++
		return nil
	})
	if !errors.Is(err, ErrInstallation) || commits != 0 || !reflect.DeepEqual(api.calls, []string{"app", "installation:acme"}) {
		t.Fatalf("partial identity crossed commit boundary: err=%v commits=%d calls=%v", err, commits, api.calls)
	}
	if strings.Contains(err.Error(), "fixture-private-key-api-body") {
		t.Fatal("API error leaked into validation error")
	}

	api = fixtureAPIValue()
	_, err = validateAt(context.Background(), fixtureNow, fixtureConfig(), fixtureSource(t), api, func(context.Context, ValidatedBinding) error {
		return errors.New("fixture-private-key-store-body")
	})
	if !errors.Is(err, ErrCommit) || strings.Contains(err.Error(), "fixture-private-key-store-body") {
		t.Fatalf("commit error was not normalized: %v", err)
	}
}

func TestCommitReceivesIndependentMetadataSnapshot(t *testing.T) {
	api := fixtureAPIValue()
	got, err := validateAt(context.Background(), fixtureNow, fixtureConfig(), fixtureSource(t), api, func(_ context.Context, binding ValidatedBinding) error {
		binding.Permissions["metadata"] = "write"
		return nil
	})
	if err != nil {
		t.Fatalf("manual fixture rejected: %v", err)
	}
	if got.Permissions["metadata"] != "read" {
		t.Fatalf("commit callback mutated returned binding: permissions=%v", got.Permissions)
	}
}

func TestCanceledValidationDoesNotReadOrCallFixture(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source := &sourceFixture{kind: SourceManual, appID: fixtureAppID, pem: fixturePEM(t)}
	api := fixtureAPIValue()
	_, err := validateAt(ctx, fixtureNow, fixtureConfig(), source, api, nil)
	if !errors.Is(err, ErrCanceled) || source.reads != 0 || len(api.calls) != 0 {
		t.Fatalf("canceled validation crossed boundary: err=%v reads=%d calls=%v", err, source.reads, api.calls)
	}
}

func TestCancellationAfterSourceReadStopsBeforeFixture(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &sourceFixture{kind: SourceManual, appID: fixtureAppID, pem: fixturePEM(t), onRead: cancel}
	api := fixtureAPIValue()

	_, err := validateAt(ctx, fixtureNow, fixtureConfig(), source, api, nil)
	if !errors.Is(err, ErrCanceled) || source.reads != 1 || len(api.calls) != 0 {
		t.Fatalf("canceled source read crossed fixture boundary: err=%v reads=%d calls=%v", err, source.reads, api.calls)
	}
}

func TestCredentialReferencesAndSourcesDoNotExposePrivateMaterial(t *testing.T) {
	pemBytes := fixturePEM(t)
	source := NewManualSource(fixtureAppID, pemBytes)
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, source), "PRIVATE KEY") {
			t.Fatalf("manual source formatting exposed private material for %s", format)
		}
	}
	ref := CredentialRef{}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, ref), "PRIVATE KEY") {
			t.Fatalf("credential reference formatting exposed private material for %s", format)
		}
	}
	for _, value := range []any{source, ref} {
		data, err := json.Marshal(value)
		if err != nil || strings.Contains(string(data), "PRIVATE KEY") {
			t.Fatal("credential boundary JSON exposed private material")
		}
	}
}
