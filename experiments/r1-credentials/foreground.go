package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// PersistenceKind is the declared credential storage boundary. R1 foreground
// setup accepts only PersistenceNone; file, environment and Keychain import
// require separately scoped authorization that this slice does not grant.
type PersistenceKind string

const (
	PersistenceNone        PersistenceKind = "none"
	PersistenceFile        PersistenceKind = "file"
	PersistenceEnvironment PersistenceKind = "environment"
	PersistenceKeychain    PersistenceKind = "keychain"
)

// LifecycleKind is the declared controller lifecycle. R1 is foreground only;
// launchd and daemon service identity remain G02 evidence, not this slice.
type LifecycleKind string

const (
	LifecycleForeground LifecycleKind = "foreground"
	LifecycleLaunchd    LifecycleKind = "launchd"
	LifecycleDaemon     LifecycleKind = "daemon"
)

const foregroundControllerIdentity = "foreground-controller"

var (
	ErrPartialSetup     = errors.New("partial credential setup")
	ErrPersistence      = errors.New("implicit credential persistence is not authorized")
	ErrLifecycle        = errors.New("unattended service lifecycle is not supported")
	ErrLiveUnauthorized = errors.New("authorized GitHub App/API path is not available")
	ErrWorkerCredential = errors.New("management credentials cannot enter worker launch")
	ErrJIT              = errors.New("worker JIT bootstrap is not available")
)

// ForegroundDocument is the non-secret R1 identity declaration. It must not
// contain PEM, JWT, installation tokens or filesystem/Keychain locators.
type ForegroundDocument struct {
	AppID          int64
	Organization   Organization
	InstallationID int64
	Repository     Repository
	SourceKind     SourceKind
	Persistence    PersistenceKind
	Lifecycle      LifecycleKind
}

// ForegroundSession is metadata-only controller state after a successful
// manual identity check. It never retains the credential source or key bytes.
type ForegroundSession struct {
	Binding            ValidatedBinding
	Lifecycle          LifecycleKind
	Persistence        PersistenceKind
	ControllerIdentity string
}

func (ForegroundSession) String() string   { return "[redacted foreground session]" }
func (ForegroundSession) GoString() string { return "[redacted foreground session]" }
func (ForegroundSession) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted foreground session]"`), nil
}

type foregroundJSON struct {
	AppID          int64            `json:"app_id"`
	Organization   organizationJSON `json:"organization"`
	InstallationID int64            `json:"installation_id"`
	Repository     repositoryJSON   `json:"repository"`
	SourceKind     SourceKind       `json:"source_kind"`
	Persistence    PersistenceKind  `json:"persistence"`
	Lifecycle      LifecycleKind    `json:"lifecycle"`
}

type organizationJSON struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

type repositoryJSON struct {
	ID         int64  `json:"id"`
	OwnerID    int64  `json:"owner_id"`
	OwnerLogin string `json:"owner_login"`
	Name       string `json:"name"`
	Private    bool   `json:"private"`
}

func (raw foregroundJSON) document() ForegroundDocument {
	return ForegroundDocument{
		AppID:          raw.AppID,
		Organization:   Organization{Login: raw.Organization.Login, ID: raw.Organization.ID},
		InstallationID: raw.InstallationID,
		Repository: Repository{
			ID:         raw.Repository.ID,
			OwnerID:    raw.Repository.OwnerID,
			OwnerLogin: raw.Repository.OwnerLogin,
			Name:       raw.Repository.Name,
			Private:    raw.Repository.Private,
		},
		SourceKind:  raw.SourceKind,
		Persistence: raw.Persistence,
		Lifecycle:   raw.Lifecycle,
	}
}

// ParseForegroundDocument decodes one operator-declared identity document.
// Unknown fields, embedded credential material and extra JSON values are
// rejected as ErrConfig without returning decoder details.
func ParseForegroundDocument(data []byte) (ForegroundDocument, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return ForegroundDocument{}, ErrPartialSetup
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var raw foregroundJSON
	if err := decoder.Decode(&raw); err != nil {
		return ForegroundDocument{}, ErrConfig
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ForegroundDocument{}, ErrConfig
	}
	return checkForegroundDocument(raw.document())
}

// PrepareForeground validates one manually supplied in-memory credential
// against a complete foreground document. It does not persist credentials,
// start a service, or call GitHub except through the supplied adapter.
func PrepareForeground(ctx context.Context, doc ForegroundDocument, source CredentialSource, api API, commit Commit) (ForegroundSession, error) {
	doc, err := checkForegroundDocument(doc)
	if err != nil {
		return ForegroundSession{}, err
	}
	if isNilInterface(source) {
		return ForegroundSession{}, ErrPartialSetup
	}
	if source.Kind() != doc.SourceKind {
		return ForegroundSession{}, ErrSource
	}
	binding, err := Validate(ctx, Config{
		AppID:          doc.AppID,
		Organization:   doc.Organization,
		InstallationID: doc.InstallationID,
		Repository:     doc.Repository,
	}, source, api, commit)
	if err != nil {
		return ForegroundSession{}, err
	}
	return ForegroundSession{
		Binding:            binding,
		Lifecycle:          LifecycleForeground,
		Persistence:        PersistenceNone,
		ControllerIdentity: foregroundControllerIdentity,
	}, nil
}

func checkForegroundDocument(doc ForegroundDocument) (ForegroundDocument, error) {
	if doc.AppID < 1 || doc.InstallationID < 1 || doc.Organization.ID < 1 || doc.Repository.ID < 1 || strings.TrimSpace(doc.Organization.Login) == "" || doc.SourceKind == "" || doc.Persistence == "" || doc.Lifecycle == "" {
		return ForegroundDocument{}, ErrPartialSetup
	}
	if doc.Persistence != PersistenceNone {
		return ForegroundDocument{}, ErrPersistence
	}
	if doc.Lifecycle != LifecycleForeground {
		return ForegroundDocument{}, ErrLifecycle
	}
	if doc.SourceKind != SourceManual {
		return ForegroundDocument{}, ErrSource
	}
	config, err := normalizeConfig(Config{
		AppID:          doc.AppID,
		Organization:   doc.Organization,
		InstallationID: doc.InstallationID,
		Repository:     doc.Repository,
	})
	if err != nil {
		return ForegroundDocument{}, ErrConfig
	}
	doc.AppID = config.AppID
	doc.Organization = config.Organization
	doc.InstallationID = config.InstallationID
	doc.Repository = config.Repository
	return doc, nil
}
