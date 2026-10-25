package livecanary

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/actions/scaleset"
)

const responseBodyLimit int64 = 1 << 20

var errResponseBudget = errors.New("response body budget exceeded")

// Keep the established response-capture transport shape when selecting SDK
// v0.5.0. It no longer requires a concrete transport, but RegisterProtocol still
// preserves the inspected inner TLS/host/proxy restrictions without patching
// SDK code. Session clients inherit this guarded HTTP client.
func withResponseBudget(transport *http.Transport, wrappers ...func(http.RoundTripper) http.RoundTripper) *http.Transport {
	var inner http.RoundTripper = baselineRequestCaptureTransport{inner: transport}
	for _, wrap := range wrappers {
		if wrap != nil {
			inner = wrap(inner)
		}
	}
	wrapped := responseBudgetTransport{inner: inner}
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
	wireReq := req
	if req != nil {
		if c, _ := req.Context().Value(baselineWireKey{}).(*baselineWireCapture); c != nil {
			wireReq = markBaselineWireRequest(req)
		}
	}
	// No phase intentionally PATCHes. In particular, stop SDK 401 refresh BEFORE
	// it can replace the session/queue and retry an ACK under a different owner.
	if wireReq == nil {
		return nil, ErrRemote
	}
	if wireReq.Method == http.MethodPatch {
		return nil, ErrQuarantine
	}
	response, err := t.inner.RoundTrip(wireReq)
	if err != nil {
		return nil, err
	}
	captureRunnerResponse(wireReq, response.StatusCode)
	if response.ContentLength > responseBodyLimit {
		_ = response.Body.Close()
		return nil, errResponseBudget
	}
	response.Body = &responseBudgetBody{source: response.Body, remaining: responseBodyLimit}
	if response, err = guardQueueMessageIdentity(wireReq, response); err != nil {
		return nil, err
	}
	return guardBaselineResponse(wireReq, response)
}

// SDK DTO decoding collapses absent/null messageId to zero, but zero is a real
// queue identity. Validate presence before the SDK can lose that distinction.
// Only successful queue GETs carry the capacity header; bootstrap, REST reads
// and 202 no-message responses keep their existing handling. Bytes are replayed
// unchanged, bounded, and cleared on close; no raw response enters evidence.
func guardQueueMessageIdentity(req *http.Request, response *http.Response) (*http.Response, error) {
	if req.Method != http.MethodGet || response.StatusCode != http.StatusOK || len(drainHeaderValues(req.Header, scaleset.HeaderScaleSetMaxCapacity)) == 0 {
		return response, nil
	}
	data, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	var w struct {
		ID *int `json:"messageId"`
	}
	if readErr != nil || closeErr != nil || int64(len(data)) > responseBodyLimit || !uniqueKeys(json.NewDecoder(bytes.NewReader(data))) || json.Unmarshal(data, &w) != nil || w.ID == nil || *w.ID < 0 {
		clear(data)
		return nil, ErrRemote
	}
	response.Body = &baselineCopiedBody{Reader: bytes.NewReader(data), data: data}
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
