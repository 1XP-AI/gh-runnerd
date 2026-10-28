//go:build g01_pair_fixture

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
)

type fixtureBrokerTransport struct {
	target *url.URL
	inner  *http.Transport
}

func (t fixtureBrokerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Context().Err() != nil || r.URL.Host != "api.github.com" {
		return nil, context.Canceled
	}
	copy := r.Clone(r.Context())
	u := *r.URL
	u.Host = t.target.Host
	copy.URL = &u
	copy.Host = t.target.Host
	return t.inner.RoundTrip(copy)
}
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--execute-experimental-broker" {
		var controllerState string
		for n := 2; n+1 < len(os.Args); n++ {
			if os.Args[n] == "--controller-state-dir" {
				controllerState = os.Args[n+1]
			}
		}
		if raw, e := os.ReadFile(filepath.Join(controllerState, "broker-experiment-fixture.json")); e == nil {
			var config struct {
				Policy          string `json:"policy"`
				BaseURL         string `json:"base_url"`
				CAPEM           string `json:"ca_pem"`
				Admission       string `json:"admission"`
				WorkerAdmission string `json:"worker_admission"`
			}
			if len(raw) > 65536 || json.Unmarshal(raw, &config) != nil {
				os.Exit(91)
			}
			target, e := url.Parse(config.BaseURL)
			if e != nil || target.Scheme != "https" || net.ParseIP(target.Hostname()) == nil || !net.ParseIP(target.Hostname()).IsLoopback() {
				os.Exit(92)
			}
			block, _ := pem.Decode([]byte(config.CAPEM))
			if block == nil {
				os.Exit(93)
			}
			cert, e := x509.ParseCertificate(block.Bytes)
			if e != nil {
				os.Exit(94)
			}
			roots := x509.NewCertPool()
			roots.AddCert(cert)
			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.Proxy = nil
			transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != target.Host {
					return nil, context.Canceled
				}
				return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
			}
			if enrollment.ConfigureBrokerExperimentForFixture(config.Policy, fixtureBrokerTransport{target, transport}, config.Admission, config.WorkerAdmission) != nil {
				os.Exit(95)
			}
		}
		os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

func TestExperimentalDispatchCanceledAndRootless(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if canceled {
			cancel()
		}
		if run(ctx, []string{"--execute-experimental-broker", "--approval", "/unread-approval", "--state-dir", "/unread-state"}, nil, io.Discard) == 0 {
			t.Fatal("rootless/canceled dispatch accepted")
		}
		cancel()
	}
}
