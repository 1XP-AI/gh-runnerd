package enrollment

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestGitHubHTTPDebugDoesNotLogCredentials(t *testing.T) {
	if os.Getenv("G02_HTTP_DEBUG_CHILD") == "1" {
		runGitHubHTTPDebugFixture(t)
		return
	}
	for _, level := range []string{"0", "1", "2"} {
		t.Run(level, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGitHubHTTPDebugDoesNotLogCredentials$")
			child.Env = append(os.Environ(), "G02_HTTP_DEBUG_CHILD=1", "GODEBUG=http2debug="+level, "GORACE=atexit_sleep_ms=0")
			output, err := child.CombinedOutput()
			if err != nil {
				t.Fatalf("local TLS fixture failed: %v", err)
			}
			log := string(output)
			auth := strings.Contains(log, `encoding header "authorization" = "Bearer `)
			body := strings.Contains(log, "synthetic-github-response-canary")
			if auth || body {
				t.Fatalf("HTTP debug disclosed credentials: authorization=%t response=%t", auth, body)
			}
			if !strings.Contains(log, "GITHUB_FIXTURE_OK HTTP/1.1") {
				t.Fatal("credential request did not complete over HTTP/1.1")
			}
		})
	}
}

func runGitHubHTTPDebugFixture(t *testing.T) {
	t.Helper()
	protocol := make(chan string, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		protocol <- r.Proto
		_, _ = io.WriteString(w, `{"id":71,"value":"synthetic-github-response-canary"}`)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	// Confine the default production-constructor path to an isolated test
	// process and its own TLS listener. No real GitHub hostname is contacted.
	original := http.DefaultTransport
	fixture := original.(*http.Transport).Clone()
	fixture.Proxy = nil
	fixture.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	fixture.TLSClientConfig.ServerName = server.Certificate().DNSNames[0]
	fixture.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "api.github.com:443" {
			return nil, fmt.Errorf("non-fixture address denied")
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	defer fixture.CloseIdleConnections()
	http.DefaultTransport = fixture
	defer func() { http.DefaultTransport = original }()
	api := NewGitHubAPI(time.Now, nil)
	defer api.client.CloseIdleConnections()
	credential, err := parseCredential(syntheticCandidate(t))
	if err != nil {
		t.Fatal("fixture credential failed")
	}
	id, err := api.App(context.Background(), credential)
	if err != nil || id != 71 {
		t.Fatal("fixture credential request failed")
	}
	fmt.Println("GITHUB_FIXTURE_OK", <-protocol)
}

func TestGitHubTransportOwnership(t *testing.T) {
	original := http.DefaultTransport.(*http.Transport)
	api := NewGitHubAPI(time.Now, nil)
	private, ok := api.client.Transport.(*http.Transport)
	if !ok || private == original {
		t.Fatal("default API did not own its transport")
	}
	if private.Protocols == nil || !private.Protocols.HTTP1() || private.Protocols.HTTP2() {
		t.Fatal("credential transport protocols are not restricted")
	}
	injected := original.Clone()
	injected.Protocols = new(http.Protocols)
	injected.Protocols.SetHTTP2(true)
	fixtureAPI := NewGitHubAPI(time.Now, injected)
	if fixtureAPI.client.Transport != injected || !injected.Protocols.HTTP2() || injected.Protocols.HTTP1() {
		t.Fatal("injected synthetic transport was changed")
	}
}
