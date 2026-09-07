package enrollment

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
)

func syntheticCandidate(t *testing.T) Candidate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return Candidate{AppID: 71, PEM: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), Organizations: []Binding{{Login: "org-a", OrganizationID: 101, InstallationID: 201}, {Login: "org-b", OrganizationID: 102, InstallationID: 202}}}
}

type fakeAPI struct {
	appID         int64
	installations map[string]Installation
	err           error
}

func (f fakeAPI) App(context.Context, Credential) (int64, error) { return f.appID, f.err }
func (f fakeAPI) OrganizationInstallation(_ context.Context, _ Credential, org string) (Installation, error) {
	return f.installations[org], f.err
}
func validAPI() fakeAPI {
	return fakeAPI{appID: 71, installations: map[string]Installation{
		"org-a": {ID: 201, AppID: 71, AccountID: 101, Login: "org-a", AccountType: "Organization", TargetID: 101, TargetType: "Organization", Permissions: map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"}},
		"org-b": {ID: 202, AppID: 71, AccountID: 102, Login: "org-b", AccountType: "Organization", TargetID: 102, TargetType: "Organization", Permissions: map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"}},
	}}
}

func TestManualImportRejectsForgedBindingsBeforeStoringAnyCredential(t *testing.T) {
	original := syntheticCandidate(t)
	for _, tc := range []struct {
		name   string
		mutate func(*Candidate, *fakeAPI)
	}{
		{"forged app", func(c *Candidate, a *fakeAPI) { a.appID = 999 }},
		{"forged installation", func(c *Candidate, a *fakeAPI) { c.Organizations[1].InstallationID = 999 }},
		{"cross organization", func(c *Candidate, a *fakeAPI) { a.installations["org-b"] = a.installations["org-a"] }},
		{"recreated organization", func(c *Candidate, a *fakeAPI) { c.Organizations[1].OrganizationID = 999 }},
		{"different app installation", func(c *Candidate, a *fakeAPI) {
			i := a.installations["org-b"]
			i.AppID = 999
			a.installations["org-b"] = i
		}},
		{"personal account", func(c *Candidate, a *fakeAPI) {
			i := a.installations["org-b"]
			i.AccountType = "User"
			a.installations["org-b"] = i
		}},
		{"target mismatch", func(c *Candidate, a *fakeAPI) {
			i := a.installations["org-b"]
			i.TargetID = 999
			a.installations["org-b"] = i
		}},
		{"missing permission", func(c *Candidate, a *fakeAPI) {
			i := a.installations["org-b"]
			i.Permissions = map[string]string{"organization_self_hosted_runners": "read"}
			a.installations["org-b"] = i
		}},
		{"suspended", func(c *Candidate, a *fakeAPI) {
			i := a.installations["org-b"]
			i.Suspended = true
			a.installations["org-b"] = i
		}},
		{"duplicate organization", func(c *Candidate, a *fakeAPI) { c.Organizations[1] = c.Organizations[0] }},
		{"unsafe organization path", func(c *Candidate, a *fakeAPI) { c.Organizations[1].Login = "../app" }},
		{"malformed key", func(c *Candidate, a *fakeAPI) { c.PEM = []byte("synthetic-secret-invalid-key") }},
		{"app api failure", func(c *Candidate, a *fakeAPI) { a.err = errors.New("synthetic-secret-api-body") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := original
			c.Organizations = append([]Binding(nil), original.Organizations...)
			a := validAPI()
			tc.mutate(&c, &a)
			stores := 0
			_, err := ManualImport(context.Background(), c, a, func(context.Context, Credential, []Binding) error { stores++; return nil })
			if err == nil || stores != 0 {
				t.Fatalf("invalid import accepted: error=%v stores=%d", err, stores)
			}
			if strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("secret in error")
			}
		})
	}
}

func TestManualImportVerifiesTwoOrganizationsAndStoresOnce(t *testing.T) {
	c := syntheticCandidate(t)
	stores := 0
	result, err := ManualImport(context.Background(), c, validAPI(), func(_ context.Context, cred Credential, bindings []Binding) error {
		stores++
		if cred.AppID != c.AppID || !reflect.DeepEqual(bindings, c.Organizations) {
			t.Fatal("wrong stored binding")
		}
		return nil
	})
	if err != nil || stores != 1 || result.AppID != 71 || !reflect.DeepEqual(result.Organizations, c.Organizations) {
		t.Fatalf("import failed: result=%+v stores=%d err=%v", result, stores, err)
	}
	_, err = ManualImport(context.Background(), c, validAPI(), func(context.Context, Credential, []Binding) error { return errors.New("synthetic-secret-store-body") })
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal("storage failure not normalized")
	}
}

func TestCredentialsRedactFormattingAndJSON(t *testing.T) {
	c := syntheticCandidate(t)
	cred, err := parseCredential(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []any{c, cred} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			s := fmt.Sprintf(format, sensitive)
			if !strings.Contains(s, "redacted") {
				t.Errorf("sensitive formatting not redacted for %s", format)
			}
		}
		data, err := json.Marshal(sensitive)
		if err != nil || !strings.Contains(string(data), "redacted") {
			t.Error("sensitive JSON not redacted")
		}
	}
}

func TestManualImportRejectsExcessPermissions(t *testing.T) {
	c := syntheticCandidate(t)
	api := validAPI()
	i := api.installations["org-a"]
	i.Permissions["administration"] = "write"
	api.installations["org-a"] = i
	stores := 0
	_, err := ManualImport(context.Background(), c, api, func(context.Context, Credential, []Binding) error { stores++; return nil })
	if err == nil || stores != 0 {
		t.Fatal("minimal profile accepted extra administration permission")
	}
}
