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

func TestBrokerHTTPDebugDoesNotExposeCredentials(t *testing.T) {
	if os.Getenv("G01_BROKER_HTTP_FIXTURE") == "1" {
		protocol := make(chan string, 1)
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/app/installations/201/access_tokens" || r.Header.Get("Authorization") != "Bearer synthetic-broker-auth-canary" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			protocol <- r.Proto
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"token":"synthetic-broker-issued-canary"}`)
		}))
		server.EnableHTTP2 = true
		server.StartTLS()
		defer server.Close()
		original := http.DefaultTransport
		inherited := original.(*http.Transport).Clone()
		inherited.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		inherited.TLSClientConfig.NextProtos = []string{"h2", "http/1.1"}
		http.DefaultTransport = inherited
		defer func() { http.DefaultTransport = original }()
		defer inherited.CloseIdleConnections()
		api := newBrokerAPI(time.Now, nil)
		transport := api.client.Transport.(brokerTransport).inner.(*http.Transport)
		// Configure only this constructor's private transport for its own TLS
		// fixture. Preserve the production policy, including inherited h2 ALPN.
		transport.TLSClientConfig.RootCAs = server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
		transport.TLSClientConfig.ServerName = server.Certificate().DNSNames[0]
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "api.github.com:443" {
				return nil, errBroker
			}
			return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
		}
		defer transport.CloseIdleConnections()
		var result map[string]string
		if api.call(context.Background(), "POST", "/app/installations/201/access_tokens", "Bearer synthetic-broker-auth-canary", nil, http.StatusCreated, &result) != nil || result["token"] != "synthetic-broker-issued-canary" {
			t.Fatal("synthetic credential exchange failed")
		}
		fmt.Println("BROKER_TLS_OK", <-protocol)
		return
	}
	for _, level := range []string{"0", "1", "2"} {
		t.Run(level, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBrokerHTTPDebugDoesNotExposeCredentials$")
			child.Env = []string{"G01_BROKER_HTTP_FIXTURE=1", "GODEBUG=http2debug=" + level, "GORACE=atexit_sleep_ms=0"}
			data, err := child.CombinedOutput()
			if err != nil {
				t.Fatal("synthetic HTTP child failed")
			}
			log := string(data)
			auth := strings.Contains(log, "synthetic-broker-auth-canary")
			issued := strings.Contains(log, "synthetic-broker-issued-canary")
			if auth || issued {
				t.Fatalf("broker debug leaked credentials: authorization=%t issued=%t", auth, issued)
			}
			if !strings.Contains(log, "BROKER_TLS_OK HTTP/1.1") {
				t.Fatal("synthetic credential exchange did not use HTTP/1.1")
			}
		})
	}
}
