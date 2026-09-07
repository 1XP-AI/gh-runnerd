package livecanary

import (
	"errors"
	"io"
	"net/http"
)

const responseBodyLimit int64 = 1 << 20

var errResponseBudget = errors.New("response body budget exceeded")

// SDK v0.4.0 requires its HTTPClient.Transport to be a *http.Transport, including
// when it constructs session clients. Standard RegisterProtocol permits a shared
// response wrapper without replacing that required type or patching the SDK.
// The inner clone preserves the already configured TLS/host/proxy restrictions.
func withResponseBudget(transport *http.Transport) *http.Transport {
	wrapped := responseBudgetTransport{inner: transport}
	outer := transport.Clone()
	// The outer transport only dispatches to the wrapper. Disable its own HTTP/2
	// setup so it cannot register a competing HTTPS handler; the inner transport
	// retains its original protocols and all TLS/destination checks.
	outer.Protocols = new(http.Protocols)
	outer.Protocols.SetHTTP1(true)
	outer.RegisterProtocol("http", wrapped)
	outer.RegisterProtocol("https", wrapped)
	return outer
}

type responseBudgetTransport struct{ inner http.RoundTripper }

func (t responseBudgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// No phase intentionally PATCHes. In particular, stop SDK 401 refresh BEFORE
	// it can replace the session/queue and retry an ACK under a different owner.
	if req.Method == http.MethodPatch {
		return nil, ErrQuarantine
	}
	response, err := t.inner.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	captureRunnerResponse(req, response.StatusCode)
	if response.ContentLength > responseBodyLimit {
		_ = response.Body.Close()
		return nil, errResponseBudget
	}
	response.Body = &responseBudgetBody{source: response.Body, remaining: responseBodyLimit}
	return response, nil
}

// Read at most the budget plus one detection byte, including decoded gzip and
// unknown/chunked lengths. A truncated prefix is never reported as clean EOF.
type responseBudgetBody struct {
	source    io.ReadCloser
	remaining int64
	exceeded  bool
}

func (b *responseBudgetBody) Read(p []byte) (int, error) {
	if b.exceeded {
		return 0, errResponseBudget
	}
	if len(p) == 0 {
		return 0, nil
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.source.Read(p)
	if int64(n) > b.remaining {
		b.exceeded = true
		_ = b.source.Close()
		return 0, errResponseBudget
	}
	b.remaining -= int64(n)
	return n, err
}
func (b *responseBudgetBody) Close() error { return b.source.Close() }
