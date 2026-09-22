package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type commandFakeAPI struct {
	mu        sync.Mutex
	calls     int
	candidate enrollment.Candidate
}

func (api *commandFakeAPI) recordCall() {
	api.mu.Lock()
	api.calls++
	api.mu.Unlock()
}

func (api *commandFakeAPI) callCount() int {
	api.mu.Lock()
	defer api.mu.Unlock()
	return api.calls
}

func (api *commandFakeAPI) App(context.Context, enrollment.Credential) (int64, error) {
	api.recordCall()
	return 71, nil
}

func (api *commandFakeAPI) OrganizationInstallation(_ context.Context, _ enrollment.Credential, login string) (enrollment.Installation, error) {
	api.recordCall()
	if login == "org-a" {
		return commandInstallation(201, 101, "org-a"), nil
	}
	return commandInstallation(202, 102, "org-b"), nil
}

func (api *commandFakeAPI) Convert(context.Context, string) (enrollment.Candidate, error) {
	api.recordCall()
	candidate := api.candidate
	candidate.PEM = append([]byte(nil), candidate.PEM...)
	candidate.Organizations = append([]enrollment.Binding(nil), candidate.Organizations...)
	return candidate, nil
}

func (api *commandFakeAPI) DescribeApp(context.Context, enrollment.Credential) (enrollment.AppIdentity, error) {
	api.recordCall()
	return enrollment.AppIdentity{ID: 71, Slug: "synthetic-app", OwnerLogin: "org-a", OwnerID: 101, OwnerType: "Organization"}, nil
}

func commandInstallation(id, organizationID int64, login string) enrollment.Installation {
	return enrollment.Installation{
		ID: id, AppID: 71, AccountID: organizationID, Login: login,
		AccountType: "Organization", TargetID: organizationID, TargetType: "Organization",
		Permissions:     map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"},
		SuspensionKnown: true,
	}
}

func commandTestPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func newCommandFakeAPI(t *testing.T) *commandFakeAPI {
	t.Helper()
	return &commandFakeAPI{candidate: enrollment.Candidate{
		AppID: 71,
		PEM:   commandTestPEM(t),
		Organizations: []enrollment.Binding{
			{Login: "org-a", OrganizationID: 101, InstallationID: 201},
			{Login: "org-b", OrganizationID: 102, InstallationID: 202},
		},
	}}
}

func commandIdentity() processIdentity {
	return processIdentity{uid: 1001, euid: 1001}
}

func commandManualArgs(journal string) []string {
	return []string{"manual", "--live-github", "--owner", "org-a", "--app-name", "synthetic-app", "--org", "org-a:101:201", "--org", "org-b:102:202", "--app-id", "71", "--journal-dir", journal}
}

func commandManifestArgs(journal string) []string {
	return []string{"manifest", "--live-github", "--owner", "org-a", "--app-name", "synthetic-app", "--org", "org-a:101", "--org", "org-b:102", "--journal-dir", journal}
}

func commandInputWithProbe(t *testing.T, data []byte) (*os.File, *os.File, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.pem")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal("fixture setup failed")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal("fixture setup failed")
	}
	input, err := os.Open(path)
	if err != nil {
		t.Fatal("fixture setup failed")
	}
	probeFD, err := syscall.Dup(int(input.Fd()))
	if err != nil {
		input.Close()
		t.Fatal("fixture setup failed")
	}
	probe := os.NewFile(uintptr(probeFD), "input-probe")
	t.Cleanup(func() {
		input.Close()
		probe.Close()
	})
	return input, probe, path
}

type commandOutput struct {
	lines chan []byte
}

func (output *commandOutput) Write(data []byte) (int, error) {
	output.lines <- append([]byte(nil), data...)
	return len(data), nil
}

func commandRequest(t *testing.T, method, endpoint, body, origin string) (int, string, string) {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal("request setup failed")
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("local enrollment request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal("local enrollment response failed")
	}
	return response.StatusCode, string(responseBody), response.Header.Get("Location")
}

func commandCSRF(t *testing.T, body string) string {
	t.Helper()
	parts := strings.Split(body, `name="csrf" value="`)
	if len(parts) != 2 {
		t.Fatal("missing local enrollment token")
	}
	return strings.SplitN(parts[1], `"`, 2)[0]
}

func TestManualExecutableSyntheticAdapterAndPrivateStdin(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	secret := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	parent := t.TempDir()
	keyPath := filepath.Join(parent, "synthetic.pem")
	if os.WriteFile(keyPath, secret, 0600) != nil {
		t.Fatal("fixture")
	}
	calls := 0
	api := enrollment.NewGitHubAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.github.com" || r.Method != "GET" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Fatal("unexpected API request")
		}
		var body string
		switch r.URL.Path {
		case "/app":
			body = `{"id":71,"slug":"synthetic-app","owner":{"login":"org-a","id":101,"type":"Organization"}}`
		case "/orgs/org-a/installation", "/orgs/org-b/installation":
			login := "org-a"
			org, inst := 101, 201
			if strings.Contains(r.URL.Path, "org-b") {
				login = "org-b"
				org = 102
				inst = 202
			}
			body = fmt.Sprintf(`{"id":%d,"app_id":71,"target_id":%d,"target_type":"Organization","account":{"id":%d,"login":%q,"type":"Organization"},"permissions":{"organization_self_hosted_runners":"write","metadata":"read"},"suspended_at":null}`, inst, org, org, login)
		default:
			t.Fatal("unexpected API destination")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}))
	base := []string{"manual", "--owner", "org-a", "--app-name", "synthetic-app", "--org", "org-a:101:201", "--org", "org-b:102:202", "--app-id", "71", "--journal-dir", filepath.Join(parent, "journal")}
	for _, tc := range []struct {
		name string
		live bool
		mode os.FileMode
		want int
	}{{"missing explicit mode", false, 0600, 2}, {"broad key file", true, 0644, 2}, {"protected input", true, 0600, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			if os.Chmod(keyPath, tc.mode) != nil {
				t.Fatal("fixture")
			}
			input, e := os.Open(keyPath)
			if e != nil {
				t.Fatal("fixture")
			}
			defer input.Close()
			args := append([]string(nil), base...)
			if tc.live {
				args = append(args, "--live-github")
			}
			var out, diagnostics bytes.Buffer
			code := runWithIdentity(context.Background(), args, input, &out, &diagnostics, api, commandIdentity())
			if code != tc.want {
				t.Fatalf("status %d wanted %d", code, tc.want)
			}
			if code == 0 && !strings.Contains(out.String(), `"credentials_not_persisted":true`) {
				t.Fatal("missing verify-only result")
			}
			if code != 0 && calls != 0 {
				t.Fatal("unsafe boundary reached API")
			}
			for _, s := range []string{string(secret), "PRIVATE KEY", keyPath} {
				if strings.Contains(out.String()+diagnostics.String(), s) {
					t.Fatal("private input leaked")
				}
			}
		})
	}
	if calls != 4 {
		t.Fatalf("expected identity+App+two bindings: %d", calls)
	}
	filepath.WalkDir(filepath.Join(parent, "journal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if !entry.IsDir() {
			data, _ := os.ReadFile(path)
			if bytes.Contains(data, secret) || bytes.Contains(data, []byte("PRIVATE KEY")) {
				t.Error("PEM copied to journal")
			}
		}
		return nil
	})
}

func TestExecutableRejectsInvalidProcessIdentityBeforeEffects(t *testing.T) {
	invalidUID := int(uint64(^uint32(0)))
	uidCases := []struct {
		name string
		uid  int
		euid int
	}{
		{name: "real UID root", uid: 0, euid: 1001},
		{name: "effective UID root", uid: 1001, euid: 0},
		{name: "both UIDs root", uid: 0, euid: 0},
		{name: "UID mismatch", uid: 1001, euid: 1002},
		{name: "real UID unavailable", uid: -1, euid: 1001},
		{name: "effective UID unavailable", uid: 1001, euid: -1},
		{name: "both UIDs unavailable", uid: -1, euid: -1},
		{name: "reserved UID sentinel", uid: invalidUID, euid: invalidUID},
	}
	for _, uidCase := range uidCases {
		for _, mode := range []string{"manifest", "manual"} {
			t.Run(uidCase.name+" / "+mode, func(t *testing.T) {
				api := newCommandFakeAPI(t)
				journal := filepath.Join(t.TempDir(), "journal")
				var input, probe *os.File
				var inputPath string
				args := commandManifestArgs(journal)
				if mode == "manual" {
					args = commandManualArgs(journal)
					input, probe, inputPath = commandInputWithProbe(t, api.candidate.PEM)
				}
				var output, diagnostics bytes.Buffer
				ctx := context.Background()
				if mode == "manifest" {
					canceledContext, cancel := context.WithCancel(ctx)
					cancel()
					ctx = canceledContext
				}
				code := runWithIdentity(ctx, args, input, &output, &diagnostics, api, processIdentity{uid: uidCase.uid, euid: uidCase.euid})
				if code != 2 {
					t.Errorf("invalid identity returned status %d, want 2", code)
				}
				if api.callCount() != 0 {
					t.Errorf("invalid identity reached fake API %d times", api.callCount())
				}
				if _, err := os.Stat(journal); !os.IsNotExist(err) {
					t.Errorf("invalid identity created journal: %v", err)
				}
				if strings.Contains(output.String(), "awaiting_local_browser") || strings.Contains(output.String(), "127.0.0.1") {
					t.Error("invalid identity started Manifest listener")
				}
				if bytes.Contains(output.Bytes(), api.candidate.PEM) || bytes.Contains(diagnostics.Bytes(), api.candidate.PEM) || (inputPath != "" && strings.Contains(output.String()+diagnostics.String(), inputPath)) {
					t.Error("invalid identity diagnostics exposed private input or path")
				}
				if mode == "manual" {
					remaining, err := io.ReadAll(probe)
					if err != nil {
						t.Errorf("could not inspect input read state: %v", err)
					} else if !bytes.Equal(remaining, api.candidate.PEM) {
						t.Error("invalid identity consumed protected input")
					}
				}
			})
		}
	}
}

func TestExecutableAllowsHelpBeforeIdentityGate(t *testing.T) {
	api := newCommandFakeAPI(t)
	var output, diagnostics bytes.Buffer
	code := runWithIdentity(context.Background(), []string{"--help"}, nil, &output, &diagnostics, api, processIdentity{uid: 0, euid: 0})
	if code != 0 || output.String() != usage || diagnostics.Len() != 0 {
		t.Fatal("read-only help was not available for a root identity")
	}
	if api.callCount() != 0 {
		t.Fatal("help reached fake API")
	}
}

func TestManifestExecutableAcceptsMatchingNonRootIdentity(t *testing.T) {
	api := newCommandFakeAPI(t)
	journal := filepath.Join(t.TempDir(), "journal")
	output := &commandOutput{lines: make(chan []byte, 2)}
	diagnostics := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result := make(chan int, 1)
	go func() {
		result <- runWithIdentity(ctx, commandManifestArgs(journal), nil, output, diagnostics, api, commandIdentity())
	}()

	var readyLine []byte
	select {
	case readyLine = <-output.lines:
	case <-ctx.Done():
		t.Fatal("Manifest command did not start")
	}
	var ready struct {
		Status string `json:"status"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(readyLine, &ready); err != nil || ready.Status != "awaiting_local_browser" || !strings.HasPrefix(ready.URL, "http://127.0.0.1:") {
		t.Fatal("Manifest command did not expose its expected local-only status")
	}
	status, homeBody, _ := commandRequest(t, http.MethodGet, ready.URL+"/", "", "")
	if status != http.StatusOK {
		t.Fatalf("Manifest home status %d", status)
	}
	csrf := commandCSRF(t, homeBody)
	status, _, _ = commandRequest(t, http.MethodPost, ready.URL+"/register", url.Values{"csrf": {csrf}}.Encode(), ready.URL)
	if status != http.StatusOK {
		t.Fatalf("Manifest preparation status %d", status)
	}
	status, _, location := commandRequest(t, http.MethodGet, ready.URL+"/manifest/callback?state="+url.QueryEscape(csrf)+"&code=synthetic-code", "", "")
	if status != http.StatusSeeOther || location != "/" {
		t.Fatalf("Manifest callback status=%d location=%q", status, location)
	}
	status, homeBody, _ = commandRequest(t, http.MethodGet, ready.URL+"/", "", "")
	if status != http.StatusOK {
		t.Fatalf("verified Manifest home status %d", status)
	}
	csrf = commandCSRF(t, homeBody)
	verify := url.Values{"csrf": {csrf}, "installation_0": {"201"}, "installation_1": {"202"}}
	status, _, _ = commandRequest(t, http.MethodPost, ready.URL+"/verify", verify.Encode(), ready.URL)
	if status != http.StatusOK {
		t.Fatalf("Manifest verification status %d", status)
	}

	var summaryLine []byte
	select {
	case summaryLine = <-output.lines:
	case <-ctx.Done():
		t.Fatal("Manifest command did not finish verification")
	}
	select {
	case code := <-result:
		if code != 0 {
			t.Fatalf("Manifest command returned status %d: %s", code, diagnostics.String())
		}
	case <-ctx.Done():
		t.Fatal("Manifest command did not return")
	}
	var summary enrollment.DriverSummary
	if err := json.Unmarshal(summaryLine, &summary); err != nil || !summary.CredentialsNotPersisted || summary.VerifiedOrganizations != 2 || summary.AppID != 71 {
		t.Fatal("Manifest command returned an invalid verify-only summary")
	}
	if api.callCount() != 5 {
		t.Fatalf("Manifest fake API call count %d, want 5", api.callCount())
	}
	if strings.Contains(diagnostics.String(), string(api.candidate.PEM)) {
		t.Fatal("Manifest diagnostics exposed the synthetic key")
	}
}

func TestExecutableRejectsUnexpectedArgumentsWithoutEchoing(t *testing.T) {
	var out, diagnostics bytes.Buffer
	if code := run(context.Background(), []string{"manual", "--synthetic-secret"}, nil, &out, &diagnostics, nil); code != 2 {
		t.Fatal("unknown arguments accepted")
	}
	if strings.Contains(out.String()+diagnostics.String(), "synthetic-secret") {
		t.Fatal("flag value echoed")
	}
}
