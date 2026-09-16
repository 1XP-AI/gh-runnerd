package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixtureDocument() ForegroundDocument {
	config := fixtureConfig()
	return ForegroundDocument{
		AppID:          config.AppID,
		Organization:   config.Organization,
		InstallationID: config.InstallationID,
		Repository:     config.Repository,
		SourceKind:     SourceManual,
		Persistence:    PersistenceNone,
		Lifecycle:      LifecycleForeground,
	}
}

func fixtureDocumentJSON() []byte {
	return []byte(`{
		"app_id": 71,
		"organization": {"login":"acme","id":101},
		"installation_id": 201,
		"repository": {"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},
		"source_kind": "manual",
		"persistence": "none",
		"lifecycle": "foreground"
	}`)
}

func TestParseForegroundDocumentAcceptsExplicitManualForegroundDeclaration(t *testing.T) {
	got, err := ParseForegroundDocument(fixtureDocumentJSON())
	if err != nil {
		t.Fatalf("valid foreground document rejected: %v", err)
	}
	want := fixtureDocument()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("foreground document mismatch: got=%+v want=%+v", got, want)
	}
}

func TestParseForegroundDocumentRejectsMissingWrongAndPartialDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{name: "empty document", data: []byte(" "), want: ErrPartialSetup},
		{name: "missing installation", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"none","lifecycle":"foreground"}`), want: ErrPartialSetup},
		{name: "missing persistence", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","lifecycle":"foreground"}`), want: ErrPartialSetup},
		{name: "missing lifecycle", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"none"}`), want: ErrPartialSetup},
		{name: "extra organization", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"organizations":[{"login":"other","id":102}],"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"none","lifecycle":"foreground"}`), want: ErrConfig},
		{name: "embedded pem field", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"none","lifecycle":"foreground","pem":"fixture-private-key"}`), want: ErrConfig},
		{name: "embedded token field", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"none","lifecycle":"foreground","installation_token":"fixture-token"}`), want: ErrConfig},
		{name: "organization token field", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101,"token":"fixture-token"},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"none","lifecycle":"foreground"}`), want: ErrConfig},
		{name: "keychain persistence", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"keychain","lifecycle":"foreground"}`), want: ErrPersistence},
		{name: "file persistence", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"file","lifecycle":"foreground"}`), want: ErrPersistence},
		{name: "environment persistence", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"environment","lifecycle":"foreground"}`), want: ErrPersistence},
		{name: "launchd lifecycle", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"none","lifecycle":"launchd"}`), want: ErrLifecycle},
		{name: "daemon lifecycle", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"manual","persistence":"none","lifecycle":"daemon"}`), want: ErrLifecycle},
		{name: "file source kind", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":true},"source_kind":"file","persistence":"none","lifecycle":"foreground"}`), want: ErrSource},
		{name: "public repository", data: []byte(`{"app_id":71,"organization":{"login":"acme","id":101},"installation_id":201,"repository":{"id":301,"owner_id":101,"owner_login":"acme","name":"private-runner-fixture","private":false},"source_kind":"manual","persistence":"none","lifecycle":"foreground"}`), want: ErrConfig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseForegroundDocument(tc.data)
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, ForegroundDocument{}) {
				t.Fatalf("unsafe or partial document accepted: got=%+v err=%v", got, err)
			}
			if strings.Contains(err.Error(), "fixture-private-key") || strings.Contains(err.Error(), "fixture-token") {
				t.Fatal("document parse error leaked credential material")
			}
		})
	}
}

func TestPrepareForegroundBindsManualIdentityWithoutRetainingCredentials(t *testing.T) {
	pemBytes := fixturePEM(t)
	source := NewManualSource(fixtureAppID, pemBytes)
	api := fixtureAPIValue()
	commits := 0
	session, err := PrepareForeground(context.Background(), fixtureDocument(), source, api, func(context.Context, ValidatedBinding) error {
		commits++
		return nil
	})
	if err != nil {
		t.Fatalf("foreground preparation rejected: %v", err)
	}
	want := fixtureValidatedBinding()
	if !reflect.DeepEqual(session.Binding, want) {
		t.Fatalf("foreground binding mismatch: got=%+v want=%+v", session.Binding, want)
	}
	if session.Lifecycle != LifecycleForeground || session.Persistence != PersistenceNone || session.ControllerIdentity != foregroundControllerIdentity {
		t.Fatalf("foreground identity was not declared: %+v", session)
	}
	if commits != 1 || !reflect.DeepEqual(api.calls, []string{"app", "installation:acme", "repository:acme/private-runner-fixture"}) {
		t.Fatalf("unexpected foreground sequence: commits=%d calls=%v", commits, api.calls)
	}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, session), "PRIVATE KEY") {
			t.Fatalf("foreground session formatting exposed private material for %s", format)
		}
	}
	data, err := json.Marshal(session)
	if err != nil || strings.Contains(string(data), "PRIVATE KEY") {
		t.Fatal("foreground session JSON exposed private material")
	}
}

func TestPrepareForegroundRejectsMissingWrongExpiredAndPartialSetup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ForegroundDocument, *CredentialSource, *fixtureAPI)
		want   error
	}{
		{name: "missing source", mutate: func(_ *ForegroundDocument, source *CredentialSource, _ *fixtureAPI) { *source = nil }, want: ErrPartialSetup},
		{name: "missing installation", mutate: func(doc *ForegroundDocument, _ *CredentialSource, _ *fixtureAPI) { doc.InstallationID = 0 }, want: ErrPartialSetup},
		{name: "keychain persistence", mutate: func(doc *ForegroundDocument, _ *CredentialSource, _ *fixtureAPI) {
			doc.Persistence = PersistenceKeychain
		}, want: ErrPersistence},
		{name: "launchd lifecycle", mutate: func(doc *ForegroundDocument, _ *CredentialSource, _ *fixtureAPI) { doc.Lifecycle = LifecycleLaunchd }, want: ErrLifecycle},
		{name: "daemon lifecycle", mutate: func(doc *ForegroundDocument, _ *CredentialSource, _ *fixtureAPI) { doc.Lifecycle = LifecycleDaemon }, want: ErrLifecycle},
		{name: "file source kind", mutate: func(doc *ForegroundDocument, _ *CredentialSource, _ *fixtureAPI) { doc.SourceKind = SourceFile }, want: ErrSource},
		{name: "expired source", mutate: func(_ *ForegroundDocument, source *CredentialSource, _ *fixtureAPI) {
			*source = NewManualSourceWithExpiry(fixtureAppID, fixturePEM(t), time.Now().Add(-time.Second))
		}, want: ErrExpired},
		{name: "wrong installation", mutate: func(_ *ForegroundDocument, _ *CredentialSource, api *fixtureAPI) {
			api.installation.ID = 999
		}, want: ErrInstallation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := fixtureDocument()
			source := fixtureSource(t)
			api := fixtureAPIValue()
			tc.mutate(&doc, &source, api)
			commits := 0
			session, err := PrepareForeground(context.Background(), doc, source, api, func(context.Context, ValidatedBinding) error {
				commits++
				return nil
			})
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(session, ForegroundSession{}) || commits != 0 {
				t.Fatalf("invalid foreground setup accepted: session=%+v err=%v commits=%d calls=%v", session, err, commits, api.calls)
			}
		})
	}
}

func TestPrepareForegroundRejectsUnrelatedValidKeyBeforeRemoteEffectsComplete(t *testing.T) {
	trustedPEM := fixturePEM(t)
	wantFingerprint, err := fingerprintPrivateKey(trustedPEM)
	if err != nil {
		t.Fatal("trusted fixture key fingerprint failed")
	}
	api := fixtureAPIValue()
	api.expectedFingerprint = wantFingerprint
	session, err := PrepareForeground(context.Background(), fixtureDocument(), NewManualSource(fixtureAppID, fixturePEM(t)), api, nil)
	if !errors.Is(err, ErrAppIdentity) || !reflect.DeepEqual(session, ForegroundSession{}) || !reflect.DeepEqual(api.calls, []string{"app"}) {
		t.Fatalf("unrelated valid key was accepted: session=%+v err=%v calls=%v", session, err, api.calls)
	}
}
