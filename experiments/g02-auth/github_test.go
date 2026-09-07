package enrollment

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestGitHubAdapterAuthenticatesExpectedAppWithoutFollowingRedirects(t *testing.T) {
	c := syntheticCandidate(t)
	cred, err := parseCredential(c)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	calls := 0
	api := NewGitHubAPI(func() time.Time { return now }, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" || r.URL.Path != "/app" {
			t.Fatal("unexpected destination")
		}
		if r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Fatal("missing API version")
		}
		jwt := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
		if len(jwt) != 3 {
			t.Fatal("missing JWT")
		}
		sig, e := base64.RawURLEncoding.DecodeString(jwt[2])
		if e != nil {
			t.Fatal("invalid signature encoding")
		}
		digest := sha256.Sum256([]byte(jwt[0] + "." + jwt[1]))
		if rsa.VerifyPKCS1v15(&cred.key.PublicKey, crypto.SHA256, digest[:], sig) != nil {
			t.Fatal("wrong signing key")
		}
		payload, _ := base64.RawURLEncoding.DecodeString(jwt[1])
		var claims struct {
			Iss string `json:"iss"`
			Iat int64  `json:"iat"`
			Exp int64  `json:"exp"`
		}
		if json.Unmarshal(payload, &claims) != nil || claims.Iss != "71" || claims.Iat != now.Add(-time.Minute).Unix() || claims.Exp != now.Add(9*time.Minute).Unix() {
			t.Fatal("wrong JWT claims")
		}
		result := response(302, "synthetic-secret-response")
		result.Header.Set("Location", "https://attacker.example/exfiltrate")
		return result, nil
	}))
	_, err = api.App(context.Background(), cred)
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") || calls != 1 {
		t.Fatalf("redirect/error not contained: calls=%d err=%v", calls, err)
	}
}

func TestGitHubAdapterParsesIdentityAndRejectsFailedResponses(t *testing.T) {
	c := syntheticCandidate(t)
	cred, err := parseCredential(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body     string
		status         int
		transportError bool
	}{
		{name: "forbidden", status: 403, body: "synthetic-secret-body"},
		{name: "network failure", transportError: true},
		{name: "malformed json", status: 200, body: "{synthetic-secret-body"},
		{name: "trailing data", status: 200, body: "{\"id\":71} synthetic-secret-body"},
		{name: "oversized response", status: 200, body: strings.Repeat("s", maxResponseBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := NewGitHubAPI(time.Now, transportFunc(func(*http.Request) (*http.Response, error) {
				if tc.transportError {
					return nil, errors.New("synthetic-secret-network")
				}
				return response(tc.status, tc.body), nil
			}))
			_, err := api.App(context.Background(), cred)
			if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("unsafe response accepted or disclosed")
			}
		})
	}
	api := NewGitHubAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/orgs/org-a/installation" {
			t.Fatal("wrong organization lookup")
		}
		return response(200, `{"id":201,"app_id":71,"account":{"id":101,"login":"org-a","type":"Organization"},"target_id":101,"target_type":"Organization","permissions":{"organization_self_hosted_runners":"write","metadata":"read"},"suspended_at":null}`), nil
	}))
	i, err := api.OrganizationInstallation(context.Background(), cred, "org-a")
	if err != nil || i.ID != 201 || i.AccountID != 101 || i.AppID != 71 || i.Suspended {
		t.Fatal("lost installation identity")
	}
}

func TestManifestConversionDoesNotExposeSecretsOrRetry(t *testing.T) {
	c := syntheticCandidate(t)
	body, _ := json.Marshal(map[string]any{"id": c.AppID, "pem": string(c.PEM), "webhook_secret": "synthetic-secret-webhook", "client_secret": "synthetic-secret-client"})
	calls := 0
	api := NewGitHubAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/app-manifests/synthetic-code/conversions" || r.Header.Get("Authorization") != "" {
			t.Fatal("wrong conversion request")
		}
		return response(201, string(body)), nil
	}))
	result, err := api.Convert(context.Background(), "synthetic-code")
	if err != nil || result.AppID != 71 || len(result.PEM) == 0 || calls != 1 {
		t.Fatal("conversion failed")
	}
	for _, code := range []string{"../app", "", "with space", strings.Repeat("a", 513)} {
		if _, err := api.Convert(context.Background(), code); err == nil {
			t.Error("unsafe conversion code accepted")
		}
	}
	if calls != 1 {
		t.Fatal("invalid code reached network")
	}
}

func TestDescribeAppAuthenticatesOwnerAndSlug(t *testing.T) {
	c := syntheticCandidate(t)
	cred, _ := parseCredential(c)
	api := NewGitHubAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/app" || r.Method != "GET" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Fatal("identity not authenticated")
		}
		return response(200, `{"id":71,"slug":"synthetic-app","owner":{"login":"org-a","id":101,"type":"Organization"}}`), nil
	}))
	got, err := api.DescribeApp(context.Background(), cred)
	if err != nil || got.ID != 71 || got.Slug != "synthetic-app" || got.OwnerLogin != "org-a" || got.OwnerID != 101 || got.OwnerType != "Organization" {
		t.Fatal("lost App owner identity")
	}
}
