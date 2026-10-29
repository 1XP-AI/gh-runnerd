package livecanary

import (
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http"
	"reflect"
	"slices"

	"github.com/actions/scaleset"
)

// Capture only stable public transport configuration, never mutable connection
// pools. Same-process pointer checks supplement the fixed wire allowlist.
type jitBoundaryTransportIdentity struct {
	transport                                      *http.Transport
	tls                                            *tls.Config
	roots                                          *x509.CertPool
	proxy, dial, dialTLS, verify, verifyConnection uintptr
	insecure, keepAlive, forceHTTP2                bool
	min, max                                       uint16
	server                                         string
	protocols                                      string
	alpn                                           []string
	maxConns, maxIdle, nextProtocols               int
}

func jitBoundaryFunction(v any) uintptr {
	r := reflect.ValueOf(v)
	if !r.IsValid() || r.IsNil() {
		return 0
	}
	return r.Pointer()
}
func jitBoundaryTransportSnapshot(t *http.Transport) jitBoundaryTransportIdentity {
	if t == nil {
		return jitBoundaryTransportIdentity{}
	}
	out := jitBoundaryTransportIdentity{transport: t, tls: t.TLSClientConfig, proxy: jitBoundaryFunction(t.Proxy), dial: jitBoundaryFunction(t.DialContext), dialTLS: jitBoundaryFunction(t.DialTLSContext), keepAlive: t.DisableKeepAlives, forceHTTP2: t.ForceAttemptHTTP2, maxConns: t.MaxConnsPerHost, maxIdle: t.MaxIdleConnsPerHost, nextProtocols: len(t.TLSNextProto)}
	if t.Protocols != nil {
		out.protocols = t.Protocols.String()
	}
	if c := t.TLSClientConfig; c != nil {
		out.roots = c.RootCAs
		out.verify = jitBoundaryFunction(c.VerifyPeerCertificate)
		out.verifyConnection = jitBoundaryFunction(c.VerifyConnection)
		out.insecure = c.InsecureSkipVerify
		out.min = c.MinVersion
		out.max = c.MaxVersion
		out.server = c.ServerName
		out.alpn = slices.Clone(c.NextProtos)
	}
	return out
}
func (i jitBoundaryTransportIdentity) matches(t *http.Transport) bool {
	return reflect.DeepEqual(i, jitBoundaryTransportSnapshot(t))
}

func (a *SDKAPI) jitBoundaryAPI(s *jitBoundaryScope) (*SDKAPI, error) {
	if a == nil || a.physical == nil || s == nil {
		return nil, ErrApproval
	}
	physical := a.physical.Clone()
	physical.DisableKeepAlives = true
	physical.MaxConnsPerHost = 1
	physical.Proxy = nil
	physical.Protocols = new(http.Protocols)
	physical.Protocols.SetHTTP1(true)
	physical.ForceAttemptHTTP2 = false
	physical.TLSNextProto = nil
	if physical.TLSClientConfig == nil {
		physical.TLSClientConfig = new(tls.Config)
	}
	physical.TLSClientConfig.NextProtos = []string{"http/1.1"}
	outer := physical.Clone()
	outer.Protocols = new(http.Protocols)
	outer.Protocols.SetHTTP1(true)
	var inner http.RoundTripper = jitBoundaryTransport{scope: s, physical: physical}
	if wrap, _ := s.ctx.Value(jitBoundaryMutationKey{}).(func(http.RoundTripper) http.RoundTripper); wrap != nil {
		inner = wrap(inner)
	}
	wrapped := responseBudgetTransport{inner: inner}
	outer.RegisterProtocol("https", wrapped)
	outer.RegisterProtocol("http", wrapped)
	s.outer = outer
	client := &http.Client{Transport: outer, Timeout: operationTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	options := []scaleset.HTTPOption{scaleset.WithHTTPClient(client), scaleset.WithRetry(scaleset.RetryConfig{Max: 0}), scaleset.WithLogger(slog.New(slog.DiscardHandler))}
	config := a.baseURL + "/" + a.approval.Organization
	if a.baseURL == "https://api.github.com" {
		config = "https://github.com/" + a.approval.Organization
	}
	sdk, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: config, PersonalAccessToken: a.credentials.InstallationToken}, options...)
	if err != nil {
		physical.CloseIdleConnections()
		return nil, ErrApproval
	}
	return &SDKAPI{physical: physical, client: sdk, rest: client, baseURL: a.baseURL, approval: a.approval, credentials: a.credentials, options: options}, nil
}
