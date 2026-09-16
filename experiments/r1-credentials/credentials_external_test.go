package credentials_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/r1-credentials"
)

const externalFixtureAppID int64 = 71

var externalFixtureOrganization = credentials.Organization{Login: "acme", ID: 101}

type fileBackedEmbeddedSource struct {
	credentials.ManualSource
	path  string
	appID int64
	reads int
}

func (s *fileBackedEmbeddedSource) Kind() credentials.SourceKind { return credentials.SourceManual }
func (s *fileBackedEmbeddedSource) AppID() int64                 { return s.appID }
func (*fileBackedEmbeddedSource) ExpiresAt() time.Time           { return time.Time{} }
func (s *fileBackedEmbeddedSource) Read(ctx context.Context) ([]byte, error) {
	s.reads++
	if ctx == nil {
		return nil, credentials.ErrCanceled
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.ReadFile(s.path)
}

type externalFixtureAPI struct {
	expectedFingerprint [32]byte
	calls               []string
}

func (f *externalFixtureAPI) VerifyApp(_ context.Context, ref credentials.CredentialRef) (credentials.AppIdentity, error) {
	f.calls = append(f.calls, "app")
	if ref.AppID() != externalFixtureAppID || ref.PublicKeyFingerprint() != f.expectedFingerprint {
		return credentials.AppIdentity{}, errors.New("fixture credential mismatch")
	}
	return credentials.AppIdentity{ID: externalFixtureAppID}, nil
}

func (f *externalFixtureAPI) VerifyInstallation(_ context.Context, ref credentials.CredentialRef, organization credentials.Organization) (credentials.Installation, error) {
	f.calls = append(f.calls, "installation")
	if ref.AppID() != externalFixtureAppID || ref.PublicKeyFingerprint() != f.expectedFingerprint || organization != externalFixtureOrganization {
		return credentials.Installation{}, errors.New("fixture credential mismatch")
	}
	return credentials.Installation{
		ID: 201, AppID: externalFixtureAppID, AccountID: 101, Login: "acme", AccountType: "Organization",
		TargetID: 101, TargetType: "Organization", SuspensionKnown: true,
		Permissions: map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"},
	}, nil
}

func (f *externalFixtureAPI) VerifyRepository(_ context.Context, ref credentials.CredentialRef, organization credentials.Organization, repository credentials.Repository) (credentials.Repository, error) {
	f.calls = append(f.calls, "repository")
	if ref.AppID() != externalFixtureAppID || ref.PublicKeyFingerprint() != f.expectedFingerprint || organization != externalFixtureOrganization || repository.Name != "private-runner-fixture" {
		return credentials.Repository{}, errors.New("fixture credential mismatch")
	}
	return credentials.Repository{ID: 301, OwnerID: 101, OwnerLogin: "acme", Name: "private-runner-fixture", Private: true}, nil
}

func externalFixtureConfig() credentials.Config {
	return credentials.Config{
		AppID:          externalFixtureAppID,
		Organization:   externalFixtureOrganization,
		InstallationID: 201,
		Repository:     credentials.Repository{ID: 301, OwnerID: 101, OwnerLogin: "acme", Name: "private-runner-fixture", Private: true},
	}
}

func externalFixturePEM(t *testing.T) ([]byte, [32]byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("fixture key generation failed")
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal("fixture public key encoding failed")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), sha256.Sum256(publicKey)
}

func TestValidateRejectsExternalEmbeddedFileBackedManualSource(t *testing.T) {
	pemBytes, fingerprint := externalFixturePEM(t)
	path := filepath.Join(t.TempDir(), "credential.pem")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatal("fixture file setup failed")
	}

	source := &fileBackedEmbeddedSource{appID: externalFixtureAppID, path: path}
	api := &externalFixtureAPI{expectedFingerprint: fingerprint}
	_, err := credentials.Validate(context.Background(), externalFixtureConfig(), source, api, nil)
	if !errors.Is(err, credentials.ErrSource) || source.reads != 0 || len(api.calls) != 0 {
		t.Fatalf("external file-backed source crossed the manual-source boundary: err=%v reads=%d calls=%v", err, source.reads, api.calls)
	}
}
