package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
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
			body = fmt.Sprintf(`{"id":%d,"app_id":71,"target_id":%d,"target_type":"Organization","account":{"id":%d,"login":%q,"type":"Organization"},"permissions":{"organization_self_hosted_runners":"write","metadata":"read"}}`, inst, org, org, login)
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
			code := run(context.Background(), args, input, &out, &diagnostics, api)
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
func TestExecutableRejectsUnexpectedArgumentsWithoutEchoing(t *testing.T) {
	var out, diagnostics bytes.Buffer
	if code := run(context.Background(), []string{"manual", "--synthetic-secret"}, nil, &out, &diagnostics, nil); code != 2 {
		t.Fatal("unknown arguments accepted")
	}
	if strings.Contains(out.String()+diagnostics.String(), "synthetic-secret") {
		t.Fatal("flag value echoed")
	}
}
