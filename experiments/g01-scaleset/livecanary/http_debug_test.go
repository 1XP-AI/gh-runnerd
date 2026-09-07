package livecanary

import (
	"context"
	"crypto/tls"
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

// HTTP/2 reads GODEBUG during package initialization. Use a fresh test process,
// with a local TLS server and fake credentials, to exercise the production transport factory
// and its nested response-budget wrapper under each debug setting.
func TestSDKHTTPDebugDoesNotLogCredentials(t *testing.T) {
	if os.Getenv("G01_HTTP_DEBUG_CHILD") == "1" {
		runSDKHTTPDebugFixture(t)
		return
	}
	for _, level := range []string{"0", "1", "2"} {
		t.Run(level, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSDKHTTPDebugDoesNotLogCredentials$")
			child.Env = append(os.Environ(), "G01_HTTP_DEBUG_CHILD=1", "GODEBUG=http2debug="+level, "GORACE=atexit_sleep_ms=0")
			output, err := child.CombinedOutput()
			if err != nil {
				t.Fatalf("local TLS fixture failed: %v", err)
			}
			log := string(output)
			auth := strings.Contains(log, "synthetic-installation-token")
			body := strings.Contains(log, "synthetic-sdk-response-canary")
			if auth || body {
				t.Fatalf("HTTP debug disclosed credentials: authorization=%t response=%t", auth, body)
			}
			if !strings.Contains(log, "SDK_FIXTURE_OK HTTP/1.1") {
				t.Fatal("credential request did not complete over HTTP/1.1")
			}
		})
	}
}

func runSDKHTTPDebugFixture(t *testing.T) {
	t.Helper()
	protocol := make(chan string, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app" || r.Header.Get("Authorization") != "Bearer synthetic-installation-token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		protocol <- r.Proto
		_, _ = io.WriteString(w, `{"value":"synthetic-sdk-response-canary"}`)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	a := approval()
	transport := newSDKTransport(a)
	defer transport.CloseIdleConnections()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = new(tls.Config)
	}
	// Replace only fixture trust, retaining the production ALPN configuration.
	transport.TLSClientConfig.RootCAs = server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	transport.TLSClientConfig.ServerName = server.Certificate().DNSNames[0]
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "api.github.com:443" {
			return nil, ErrApproval
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	api := &SDKAPI{rest: &http.Client{Transport: withResponseBudget(transport), Timeout: time.Second}, baseURL: "https://api.github.com", credentials: credentials(a)}
	defer api.rest.CloseIdleConnections()
	var result struct{ Value string }
	if api.get(context.Background(), "/app", "synthetic-installation-token", &result) != nil || result.Value != "synthetic-sdk-response-canary" {
		t.Fatal("fixture credential request failed")
	}
	fmt.Println("SDK_FIXTURE_OK", <-protocol)
}

func TestSDKTransportOwnership(t *testing.T) {
	original := http.DefaultTransport.(*http.Transport)
	protocols := original.Protocols
	private := newSDKTransport(approval())
	if private == original || private.Protocols == nil || private.Protocols == protocols {
		t.Fatal("SDK transport did not own its protocol configuration")
	}
	if !private.Protocols.HTTP1() || private.Protocols.HTTP2() || original.Protocols != protocols {
		t.Fatal("SDK transport changed default protocols or retained HTTP/2")
	}
}
