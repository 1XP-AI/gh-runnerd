package enrollment

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrokerPrivateFileFrontDoorDiscoveryAndSecretFreeState(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	parent := filepath.Dir(root)
	approvalPath := filepath.Join(parent, "approval.json")
	encoded, _ := json.Marshal(a)
	if os.WriteFile(approvalPath, encoded, 0600) != nil {
		t.Fatal("fixture")
	}
	inputPath := filepath.Join(parent, "synthetic-input.json")
	data, _ := json.Marshal(brokerInput{PEM: string(c.PEM)})
	if os.WriteFile(inputPath, data, 0600) != nil {
		t.Fatal("fixture")
	}
	input, _ := os.Open(inputPath)
	defer input.Close()
	result, err := runBrokerWithAPI(context.Background(), BrokerFiles{ApprovalPath: approvalPath, StateDirectory: root}, input, api)
	if err != nil || result.ActionsHost != "fixture.actions.githubusercontent.com" || f.tokenCalls != 1 {
		t.Fatalf("approved private front door failed: status=%s calls=%d error=%v", result.Status, len(f.calls), err)
	}
	assertNoSecretFiles(t, root, string(c.PEM), f.token)
}
func TestBrokerUnsafeFilesAndDuplicateApprovalRefuseBeforeNetwork(t *testing.T) {
	for _, kind := range []string{"approval symlink", "approval broad", "duplicate approval", "input broad", "unexpected authority", "controller binary"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			parent := filepath.Dir(root)
			approvalPath := filepath.Join(parent, "approval.json")
			inputPath := filepath.Join(parent, "input.json")
			inputData, _ := json.Marshal(brokerInput{PEM: string(c.PEM)})
			if kind == "controller binary" {
				a.Mode = "controller"
				a.Phase = "create"
				a.ControllerBinarySHA256 = strings.Repeat("a", 64)
				a.ControllerApprovalSHA256 = strings.Repeat("b", 64)
				a.ControllerHarnessSHA = strings.Repeat("c", 40)
			}
			data, _ := json.Marshal(a)
			if kind == "duplicate approval" {
				data = append(data[:len(data)-1], []byte(`,"app_id":71}`)...)
			}
			if kind == "unexpected authority" {
				inputData, _ = json.Marshal(brokerInput{PEM: string(c.PEM), VerificationToken: "synthetic-unapproved-authority"})
			}
			if os.WriteFile(approvalPath, data, 0600) != nil || os.WriteFile(inputPath, inputData, 0600) != nil {
				t.Fatal("fixture")
			}
			if kind == "approval symlink" {
				link := filepath.Join(parent, "link.json")
				os.Symlink(approvalPath, link)
				approvalPath = link
			}
			if kind == "approval broad" {
				os.Chmod(approvalPath, 0644)
			}
			if kind == "input broad" {
				os.Chmod(inputPath, 0644)
			}
			input, _ := os.Open(inputPath)
			defer input.Close()
			if _, err := runBrokerWithAPI(context.Background(), BrokerFiles{ApprovalPath: approvalPath, StateDirectory: root}, input, api); err == nil {
				t.Fatal("unsafe front door accepted")
			}
			if len(f.calls) != 0 {
				t.Fatal("unsafe front door reached API")
			}
		})
	}
}
func TestBrokerDuplicateAPIJSONCannotReachMint(t *testing.T) {
	a, c, _, f, root := newBrokerFixture(t)
	api := newBrokerAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/app" {
			return response(200, `{"id":71,"id":71,"slug":"synthetic-app","owner":{"id":101,"login":"org-a","type":"Organization"}}`), nil
		}
		return f.RoundTrip(r)
	}))
	api.admissionDirectory = func() (string, error) { return f.admissionRoot, nil }
	if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil); err == nil || f.tokenCalls != 0 {
		t.Fatal("ambiguous identity response minted a token")
	}
}
func TestBrokerRejectsRedirectsAndWrongOriginBeforeSecretTransmission(t *testing.T) {
	calls := 0
	api := newBrokerAPI(time.Now, transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		res := response(302, "synthetic-private-error")
		res.Header.Set("Location", "https://attacker.example/exfiltrate")
		return res, nil
	}))
	var out any
	if api.call(context.Background(), "GET", "/app", "Bearer synthetic-private-token", nil, 200, &out) == nil || calls != 1 {
		t.Fatal("redirect followed or accepted")
	}
	for _, destination := range []string{"http://api.github.com/app", "https://api.github.com:444/app", "https://api.github.com.attacker.example/app", "https://user@api.github.com/app", "https://api.github.com/app#fragment"} {
		request, _ := http.NewRequest("GET", destination, nil)
		request.Header.Set("Authorization", "Bearer synthetic-private-token")
		if response, err := api.client.Do(request); err == nil {
			response.Body.Close()
			t.Error("wrong origin accepted")
		}
	}
	if calls != 1 {
		t.Fatal("wrong origin reached transport")
	}
	real := newBrokerAPI(time.Now, nil).client.Transport.(brokerTransport).inner.(*http.Transport)
	if real.Proxy != nil {
		t.Fatal("environment proxy inherited")
	}
}

type brokerCountingBody struct {
	remaining int64
	read      int64
}

func (b *brokerCountingBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > b.remaining {
		n = b.remaining
	}
	for i := int64(0); i < n; i++ {
		p[i] = 'x'
	}
	b.remaining -= n
	b.read += n
	return int(n), nil
}
func (b *brokerCountingBody) Close() error { return nil }
func TestBrokerBoundsEveryResponseIncludingSharedIdentityErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		length int64
	}{{"success known", 200, maxResponseBytes + 2}, {"success chunked", 200, -1}, {"error known", 502, maxResponseBytes + 2}, {"error chunked", 502, -1}} {
		t.Run(tc.name, func(t *testing.T) {
			body := &brokerCountingBody{remaining: 4 * maxResponseBytes}
			api := newBrokerAPI(time.Now, transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: body, ContentLength: tc.length, Header: make(http.Header)}, nil
			}))
			_, err := api.github.DescribeApp(context.Background(), Credential{AppID: 71, key: mustBrokerKey(t)})
			if err == nil || body.read > maxResponseBytes+1 || (tc.length > maxResponseBytes && body.read != 0) {
				t.Fatalf("response bound missing: read=%d", body.read)
			}
		})
	}
}

func mustBrokerKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	c, err := parseCredential(syntheticCandidate(t))
	if err != nil {
		t.Fatal("fixture key")
	}
	return c.key
}

func TestBrokerMissingPolicyFactsAndForgedIssuanceRefuse(t *testing.T) {
	for _, kind := range []string{"missing fork", "missing public policy", "extra token permission", "expired token", "wrong token repository", "unknown suspension"} {
		t.Run(kind, func(t *testing.T) {
			a, c, _, f, root := newBrokerFixture(t)
			api := newBrokerAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
				res, err := f.RoundTrip(r)
				if err != nil {
					return res, err
				}
				data, _ := io.ReadAll(res.Body)
				res.Body.Close()
				var body map[string]any
				json.Unmarshal(data, &body)
				switch kind {
				case "missing fork":
					if r.URL.Path == "/repos/org-a/canary" {
						delete(body, "fork")
					}
				case "missing public policy":
					if r.URL.Path == "/orgs/org-a/actions/runner-groups/3" {
						delete(body, "allows_public_repositories")
					}
				case "extra token permission":
					if strings.HasSuffix(r.URL.Path, "/access_tokens") {
						body["permissions"].(map[string]any)["actions"] = "read"
					}
				case "expired token":
					if strings.HasSuffix(r.URL.Path, "/access_tokens") {
						body["expires_at"] = time.Now().Add(-time.Minute)
					}
				case "wrong token repository":
					if strings.HasSuffix(r.URL.Path, "/access_tokens") {
						body["repositories"].([]any)[0].(map[string]any)["id"] = 502
					}
				case "unknown suspension":
					if r.URL.Path == "/orgs/org-a/installation" {
						delete(body, "suspended_at")
					}
				}
				data, _ = json.Marshal(body)
				res.Body = io.NopCloser(strings.NewReader(string(data)))
				return res, nil
			}))
			api.admissionDirectory = func() (string, error) { return f.admissionRoot, nil }
			if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil); err == nil {
				t.Fatal("missing/forged policy accepted")
			}
			for _, call := range f.calls {
				if strings.Contains(call, "registration-token") || strings.Contains(call, "runner-registration") {
					t.Fatal("unverified scope obtained Actions auth")
				}
			}
		})
	}
}

func TestBrokerControllerApprovalBindsPhaseAuthorityAndSource(t *testing.T) {
	a := brokerApprovalFixture()
	a.Mode = "controller"
	a.Phase = "before-ack"
	a.AllowVerificationAuthority = true
	a.ControllerHarnessSHA = strings.Repeat("a", 40)
	c := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: a.ControllerHarnessSHA, WorkflowSHA: strings.Repeat("b", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"before-ack"}}
	if c.validate(a, time.Now()) != nil {
		t.Fatal("valid reviewed contract rejected")
	}
	for _, kind := range []string{"source", "phase", "host", "repository", "authority"} {
		bad := c
		config := a
		switch kind {
		case "source":
			bad.HarnessSHA = strings.Repeat("c", 40)
		case "phase":
			bad.Phases = []string{"cleanup"}
		case "host":
			bad.ActionsHosts = []string{"attacker.example"}
		case "repository":
			bad.RepositoryID++
		case "authority":
			config.AllowVerificationAuthority = false
		}
		if bad.validate(config, time.Now()) == nil {
			t.Errorf("unapproved %s accepted", kind)
		}
	}
}
func TestBrokerPrivateInputBoundAndCancellation(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "synthetic.json")
	if os.WriteFile(path, []byte(strings.Repeat("x", 65537)), 0600) != nil {
		t.Fatal("fixture")
	}
	file, _ := os.Open(path)
	if _, err := readBrokerInput(context.Background(), file); err == nil {
		t.Fatal("oversized input accepted")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal("fixture")
	}
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := readBrokerInput(ctx, reader); done <- e }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled input accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("private input did not unblock")
	}
}
func TestBrokerJournalRequiresPrivateParentBeforeAPI(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	if os.Chmod(filepath.Dir(root), 0755) != nil {
		t.Fatal("fixture")
	}
	api.admissionDirectory = func() (string, error) { return f.admissionRoot, nil }
	if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil); err == nil || len(f.calls) != 0 {
		t.Fatal("shared journal parent permitted issuance")
	}
}
