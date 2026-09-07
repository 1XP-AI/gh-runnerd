package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type driverFake struct {
	fakeAPI
	candidate    Candidate
	convertError error
	conversions  int
	mu           sync.Mutex
}

func (f *driverFake) Convert(context.Context, string) (Candidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.conversions++
	return f.candidate, f.convertError
}
func (f *driverFake) DescribeApp(context.Context, Credential) (AppIdentity, error) {
	return AppIdentity{ID: 71, Slug: "synthetic-app", OwnerLogin: "org-a", OwnerID: 101, OwnerType: "Organization"}, nil
}
func driverProposal() Proposal {
	return Proposal{Owner: "org-a", AppName: "synthetic-app", Organizations: []Binding{{Login: "org-a", OrganizationID: 101}, {Login: "org-b", OrganizationID: 102}}}
}
func newDriverFixture(t *testing.T) (*Driver, *driverFake, string) {
	t.Helper()
	api := &driverFake{fakeAPI: validAPI(), candidate: syntheticCandidate(t)}
	root := filepath.Join(t.TempDir(), "attempt")
	d, err := StartManifest(context.Background(), driverProposal(), root, api, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d, api, root
}
func requestDriver(t *testing.T, d *Driver, method, path, body, origin string) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, d.URL()+path, strings.NewReader(body))
	if err != nil {
		t.Fatal("request setup failed")
	}
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(r)
	if err != nil {
		t.Fatal("loopback request failed")
	}
	return res
}
func bodyOf(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func csrfFrom(t *testing.T, body string) string {
	t.Helper()
	parts := strings.Split(body, `name="csrf" value="`)
	if len(parts) != 2 {
		t.Fatal("missing CSRF field")
	}
	return strings.SplitN(parts[1], `"`, 2)[0]
}
func beginDriver(t *testing.T, d *Driver) string {
	t.Helper()
	csrf := csrfFrom(t, bodyOf(t, requestDriver(t, d, "GET", "/", "", "")))
	res := requestDriver(t, d, "POST", "/register", url.Values{"csrf": {csrf}}.Encode(), d.URL())
	if res.StatusCode != 200 {
		t.Fatalf("register rejected: %d", res.StatusCode)
	}
	body := bodyOf(t, res)
	if !strings.Contains(body, "organization_self_hosted_runners") || !strings.Contains(body, "example.invalid") || !strings.Contains(body, "https://github.com/organizations/org-a/settings/apps/new") {
		t.Fatal("incorrect Manifest shape")
	}
	return csrf
}
func convertDriver(t *testing.T, d *Driver, state string) {
	t.Helper()
	res := requestDriver(t, d, "GET", "/manifest/callback?state="+url.QueryEscape(state)+"&code=synthetic-code", "", "")
	bodyOf(t, res)
	if res.StatusCode != 303 {
		t.Fatalf("conversion status=%d", res.StatusCode)
	}
}
func assertNoSecretFiles(t *testing.T, root string, secrets ...string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, secret := range secrets {
			if secret != "" && strings.Contains(string(data), secret) {
				t.Error("secret persisted in driver files")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestManifestDriverSyntheticEndToEndAndRestartRefusal(t *testing.T) {
	d, api, root := newDriverFixture(t)
	state := beginDriver(t, d)
	// The durable intent exists before the browser is given a remote form.
	data, err := os.ReadFile(filepath.Join(root, "attempt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "registration_started") {
		t.Fatal("remote form exposed before durable registration intent")
	}
	convertDriver(t, d, state)
	csrf := csrfFrom(t, bodyOf(t, requestDriver(t, d, "GET", "/", "", "")))
	res := requestDriver(t, d, "POST", "/verify", url.Values{"csrf": {csrf}, "installation_0": {"201"}, "installation_1": {"202"}}.Encode(), d.URL())
	if res.StatusCode != 200 {
		t.Fatalf("verification failed %d", res.StatusCode)
	}
	body := bodyOf(t, res)
	if !strings.Contains(body, "credentials_not_persisted") {
		t.Fatal("missing verify-only result")
	}
	summary, err := d.Wait()
	if err != nil || !summary.CredentialsNotPersisted || summary.VerifiedOrganizations != 2 {
		t.Fatalf("missing atomic verification: %+v %v", summary, err)
	}
	if api.conversions != 1 {
		t.Fatal("unexpected conversion count")
	}
	assertNoSecretFiles(t, root, state, "synthetic-code", string(api.candidate.PEM), "PRIVATE KEY")
	if _, err := StartManifest(context.Background(), driverProposal(), root, api, time.Minute); err == nil {
		t.Fatal("restart exposed another registration form")
	}
}

func TestManifestDriverRejectsCSRFAndDuplicateRegistration(t *testing.T) {
	d, api, _ := newDriverFixture(t)
	for _, tc := range []struct{ path, body, origin string }{{"/register", "csrf=wrong", "https://attacker.example"}, {"/register", "csrf=wrong", d.URL()}, {"/register", "csrf=wrong", ""}} {
		res := requestDriver(t, d, "POST", tc.path, tc.body, tc.origin)
		if res.StatusCode < 400 {
			t.Error("unsafe local mutation accepted")
		}
		bodyOf(t, res)
	}
	state := beginDriver(t, d)
	res := requestDriver(t, d, "POST", "/register", url.Values{"csrf": {state}}.Encode(), d.URL())
	if res.StatusCode != 409 {
		t.Error("duplicate registration allowed")
	}
	bodyOf(t, res)
	if api.conversions != 0 {
		t.Fatal("unexpected conversion")
	}
}

func TestManifestDriverConversionAmbiguityRetainsNameAndBlocksNewApp(t *testing.T) {
	d, api, root := newDriverFixture(t)
	api.convertError = errors.New("synthetic-secret-upstream")
	state := beginDriver(t, d)
	res := requestDriver(t, d, "GET", "/manifest/callback?state="+url.QueryEscape(state)+"&code=synthetic-code", "", "")
	if res.StatusCode != 502 {
		t.Fatal("conversion failure hidden")
	}
	if strings.Contains(bodyOf(t, res), "synthetic-secret") {
		t.Fatal("raw error leaked")
	}
	d.Close()
	data, err := os.ReadFile(filepath.Join(root, "attempt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "synthetic-app") || !strings.Contains(string(data), "conversion_failed") {
		t.Fatal("lost ambiguous App ownership record")
	}
	if _, err := StartManifest(context.Background(), driverProposal(), root, api, time.Minute); err == nil {
		t.Fatal("ambiguous conversion permitted another App")
	}
	assertNoSecretFiles(t, root, state, "synthetic-code", "synthetic-secret")
}

func TestManifestDriverForgedSecondInstallationDoesNotCommit(t *testing.T) {
	d, _, root := newDriverFixture(t)
	state := beginDriver(t, d)
	convertDriver(t, d, state)
	res := requestDriver(t, d, "POST", "/verify", url.Values{"csrf": {state}, "installation_0": {"201"}, "installation_1": {"999"}}.Encode(), d.URL())
	if res.StatusCode < 400 {
		t.Fatal("forged second installation committed")
	}
	bodyOf(t, res)
	data, _ := os.ReadFile(filepath.Join(root, "attempt.json"))
	if strings.Contains(string(data), `"phase":"verified"`) {
		t.Fatal("partial verification committed")
	}
	d.Close()
}

func TestManualDriverUsesMemoryAndNeverCopiesPEM(t *testing.T) {
	candidate := syntheticCandidate(t)
	p := driverProposal()
	p.Organizations = candidate.Organizations
	api := &driverFake{fakeAPI: validAPI(), candidate: candidate}
	root := filepath.Join(t.TempDir(), "manual")
	summary, err := VerifyManual(context.Background(), p, root, 71, io.NopCloser(strings.NewReader(string(candidate.PEM))), api)
	if err != nil || !summary.CredentialsNotPersisted || summary.VerifiedOrganizations != 2 || api.conversions != 0 {
		t.Fatalf("manual import failed: %+v %v", summary, err)
	}
	assertNoSecretFiles(t, root, string(candidate.PEM), "PRIVATE KEY")
	data, _ := os.ReadFile(filepath.Join(root, "attempt.json"))
	var record map[string]any
	if json.Unmarshal(data, &record) != nil || record["phase"] != "verified" {
		t.Fatal("manual verification not recorded")
	}
}

func TestJournalRejectsUnsafeDirectoryAndConcurrentOwner(t *testing.T) {
	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if os.Mkdir(real, 0755) != nil {
		t.Fatal("fixture setup")
	}
	api := &driverFake{fakeAPI: validAPI(), candidate: syntheticCandidate(t)}
	if _, err := StartManifest(context.Background(), driverProposal(), real, api, time.Minute); err == nil {
		t.Fatal("unsafe state directory accepted")
	}
	link := filepath.Join(parent, "link")
	if os.Symlink(real, link) != nil {
		t.Fatal("fixture setup")
	}
	if _, err := StartManifest(context.Background(), driverProposal(), link, api, time.Minute); err == nil {
		t.Fatal("symlink state directory accepted")
	}
	d, api, root := newDriverFixture(t)
	defer d.Close()
	p := driverProposal()
	p.Organizations = api.candidate.Organizations
	if _, err := VerifyManual(context.Background(), p, root, 71, io.NopCloser(strings.NewReader(string(api.candidate.PEM))), api); err == nil {
		t.Fatal("concurrent journal owner accepted")
	}
}

func TestCallbackBeforeRegistrationDoesNotConsumeAttempt(t *testing.T) {
	d, api, _ := newDriverFixture(t)
	csrf := csrfFrom(t, bodyOf(t, requestDriver(t, d, "GET", "/", "", "")))
	res := requestDriver(t, d, "GET", "/manifest/callback?state="+csrf+"&code=synthetic-code", "", "")
	if res.StatusCode != 409 {
		t.Error("out-of-order callback not rejected")
	}
	bodyOf(t, res)
	if api.conversions != 0 {
		t.Fatal("conversion before registration")
	}
	beginDriver(t, d)
	convertDriver(t, d, csrf)
}

func TestDriverHTTPBoundsAndHostDoNotConsumeRegistration(t *testing.T) {
	d, _, _ := newDriverFixture(t)
	csrf := csrfFrom(t, bodyOf(t, requestDriver(t, d, "GET", "/", "", "")))
	for _, tc := range []struct {
		name, path, body, host string
		origins                []string
	}{
		{"wrong Host", "/register", "csrf=" + csrf, "localhost:123", []string{d.URL()}},
		{"duplicate Origin", "/register", "csrf=" + csrf, "", []string{d.URL(), d.URL()}},
		{"duplicate field", "/register", "csrf=" + csrf + "&csrf=" + csrf, "", []string{d.URL()}},
		{"oversized body", "/register", "csrf=" + csrf + "&padding=" + strings.Repeat("x", 4096), "", []string{d.URL()}},
		{"query parameters", "/register?state=untrusted", "csrf=" + csrf, "", []string{d.URL()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest("POST", d.URL()+tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for _, o := range tc.origins {
				r.Header.Add("Origin", o)
			}
			if tc.host != "" {
				r.Host = tc.host
			}
			res, e := (&http.Client{Timeout: time.Second}).Do(r)
			if e != nil {
				t.Fatal("loopback failed")
			}
			if res.StatusCode < 400 {
				t.Error("unsafe request accepted")
			}
			bodyOf(t, res)
		})
	}
	beginDriver(t, d)
}

type alteredIdentityAPI struct {
	*driverFake
	identity AppIdentity
}

func (a alteredIdentityAPI) DescribeApp(context.Context, Credential) (AppIdentity, error) {
	return a.identity, nil
}
func TestManualDriverRejectsForgedAppOwnerOrName(t *testing.T) {
	c := syntheticCandidate(t)
	p := driverProposal()
	p.Organizations = c.Organizations
	good := AppIdentity{ID: 71, Slug: "synthetic-app", OwnerLogin: "org-a", OwnerID: 101, OwnerType: "Organization"}
	for _, field := range []string{"ID", "Slug", "OwnerID", "OwnerLogin", "OwnerType"} {
		t.Run(field, func(t *testing.T) {
			bad := good
			switch field {
			case "ID":
				bad.ID = 72
			case "Slug":
				bad.Slug = "another-app"
			case "OwnerID":
				bad.OwnerID = 102
			case "OwnerLogin":
				bad.OwnerLogin = "org-b"
			case "OwnerType":
				bad.OwnerType = "User"
			}
			root := filepath.Join(t.TempDir(), "attempt")
			api := alteredIdentityAPI{&driverFake{fakeAPI: validAPI(), candidate: c}, bad}
			summary, err := VerifyManual(context.Background(), p, root, 71, io.NopCloser(strings.NewReader(string(c.PEM))), api)
			if err == nil || summary.CredentialsNotPersisted || summary.VerifiedOrganizations != 0 {
				t.Fatal("forged owner committed")
			}
			assertNoSecretFiles(t, root, string(c.PEM))
		})
	}
}

func TestAmbiguousAttemptRecoversOnlySameAppManually(t *testing.T) {
	d, api, root := newDriverFixture(t)
	api.convertError = errors.New("uncertain")
	state := beginDriver(t, d)
	bodyOf(t, requestDriver(t, d, "GET", "/manifest/callback?state="+state+"&code=synthetic-code", "", ""))
	d.Close()
	p := driverProposal()
	p.Organizations = api.candidate.Organizations
	summary, err := VerifyManual(context.Background(), p, root, 71, io.NopCloser(strings.NewReader(string(api.candidate.PEM))), api)
	if err != nil || summary.VerifiedOrganizations != 2 {
		t.Fatal("same App manual recovery failed")
	}
	if _, err = StartManifest(context.Background(), driverProposal(), root, api, time.Minute); err == nil {
		t.Fatal("recovered journal allowed another App")
	}
	p.AppName = "different-app"
	if _, err = VerifyManual(context.Background(), p, root, 71, io.NopCloser(strings.NewReader(string(api.candidate.PEM))), api); err == nil {
		t.Fatal("different proposal replaced inventory")
	}
}

func TestJournalFailureNeverExposesRemoteForm(t *testing.T) {
	d, _, root := newDriverFixture(t)
	csrf := csrfFrom(t, bodyOf(t, requestDriver(t, d, "GET", "/", "", "")))
	if os.WriteFile(filepath.Join(root, "record.next"), []byte(`{"phase":"interrupted"}`), 0600) != nil {
		t.Fatal("fixture")
	}
	res := requestDriver(t, d, "POST", "/register", "csrf="+csrf, d.URL())
	if res.StatusCode != 500 || strings.Contains(bodyOf(t, res), "settings/apps/new") {
		t.Fatal("remote form exposed without durable intent")
	}
	d.Close()
	if _, err := StartManifest(context.Background(), driverProposal(), root, &driverFake{fakeAPI: validAPI()}, time.Minute); err == nil {
		t.Fatal("failed journal permitted fresh creation")
	}
}

func TestDriverLifetimeClosesListenerAndReleasesJournal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	api := &driverFake{fakeAPI: validAPI(), candidate: syntheticCandidate(t)}
	root := filepath.Join(t.TempDir(), "attempt")
	d, err := StartManifest(ctx, driverProposal(), root, api, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	summary, err := d.Wait()
	if err == nil || summary.CredentialsNotPersisted {
		t.Fatal("canceled attempt reported verified")
	}
	client := &http.Client{Timeout: time.Second}
	if res, err := client.Get(d.URL()); err == nil {
		res.Body.Close()
		t.Fatal("listener survived cancellation")
	}
	p := driverProposal()
	p.Organizations = api.candidate.Organizations
	if _, err := VerifyManual(context.Background(), p, root, 71, io.NopCloser(strings.NewReader(string(api.candidate.PEM))), api); err != nil {
		t.Fatal("private journal lock survived cancellation")
	}
}

func TestManualCanceledInputUnblocksWithoutSecretCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	defer writer.Close()
	c := syntheticCandidate(t)
	p := driverProposal()
	p.Organizations = c.Organizations
	done := make(chan error, 1)
	go func() {
		_, err := VerifyManual(ctx, p, filepath.Join(t.TempDir(), "attempt"), 71, reader, &driverFake{fakeAPI: validAPI(), candidate: c})
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled input accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("input cancellation did not unblock")
	}
}
