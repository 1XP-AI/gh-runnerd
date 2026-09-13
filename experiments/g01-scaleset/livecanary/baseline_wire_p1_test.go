package livecanary

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type baselineCloseErrorBody struct {
	io.Reader
	err error
}

func (b baselineCloseErrorBody) Close() error { return b.err }

func TestGuardBaselineResponseRejectsRunnerFactsWhenBodyCloseFails(t *testing.T) {
	capture := &baselineWireCapture{
		stage:                "runner-observe",
		setID:                7,
		runnerName:           "fixture-runner",
		origin:               "https://api.example:443",
		runtimePathPrefix:    "/tenant/v2",
		runtimePathPrefixSet: true,
		allowedHosts:         []string{"api.example"},
	}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodGet, "https://api.example/tenant/v2/_apis/distributedtask/pools/0/agents?agentName=fixture-runner&api-version=6.0-preview", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := guardBaselineResponse(req, &http.Response{
		StatusCode: http.StatusOK,
		Body: &baselineCloseErrorBody{
			Reader: strings.NewReader(`{"count":1,"value":[{"id":19,"name":"fixture-runner","runnerScaleSetId":7}]}`),
			err:    io.ErrClosedPipe,
		},
	})
	if !errors.Is(err, ErrRemote) || response != nil {
		t.Fatalf("complete runner body with close error = response %v err %v, want remote rejection", response, err)
	}
	runner, status := capture.runnerFacts()
	if runner != nil || status != http.StatusOK || capture.observed() {
		t.Fatalf("runner close error published evidence: runner=%+v status=%d observed=%v", runner, status, capture.observed())
	}
}

func TestGuardBaselineResponseRejectsTerminalCloseWhenBodyCloseFails(t *testing.T) {
	capture := &baselineWireCapture{
		stage:                "terminal-session-close",
		setID:                7,
		sessionID:            "session",
		origin:               "https://api.example:443",
		runtimePathPrefix:    "/tenant/v2",
		runtimePathPrefixSet: true,
	}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodDelete, "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := guardBaselineResponse(req, &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       &baselineCloseErrorBody{Reader: strings.NewReader(""), err: io.ErrClosedPipe},
	})
	if !errors.Is(err, ErrRemote) || response != nil {
		t.Fatalf("terminal close body with close error = response %v err %v, want remote rejection", response, err)
	}
	_, _, _, status := capture.facts()
	if status != http.StatusNoContent || capture.observed() {
		t.Fatalf("terminal close body close error published evidence: status=%d observed=%v", status, capture.observed())
	}
}

func TestGuardBaselineResponseRejectsEvidenceDeleteWhenBodyCloseFails(t *testing.T) {
	tests := []struct {
		name    string
		capture *baselineWireCapture
		target  string
	}{
		{
			name: "ack",
			capture: &baselineWireCapture{
				stage: "ack", setID: 7, sessionID: "session", queue: "https://api.example/tenant/v2/queue?proof=fixture", cursor: 41,
				origin: "https://api.example:443", runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true,
			},
			target: "https://api.example/tenant/v2/queue/41?proof=fixture",
		},
		{
			name: "terminal-session-close",
			capture: &baselineWireCapture{
				stage: "terminal-session-close", setID: 7, sessionID: "session", origin: "https://api.example:443",
				runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true,
			},
			target: "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview",
		},
		{
			name: "terminal-set-delete",
			capture: &baselineWireCapture{
				stage: "terminal-set-delete", setID: 7, origin: "https://api.example:443",
				runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
			},
			target: "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7?api-version=6.0-preview",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(tc.capture.context(context.Background()), http.MethodDelete, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := guardBaselineResponse(req, &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       &baselineCloseErrorBody{Reader: strings.NewReader(""), err: io.ErrClosedPipe},
			})
			if !errors.Is(err, ErrRemote) || response != nil {
				t.Fatalf("evidence DELETE body with close error = response %v err %v, want remote rejection", response, err)
			}
			_, _, _, status := tc.capture.facts()
			if status != http.StatusNoContent || tc.capture.observed() {
				t.Fatalf("evidence DELETE close error published evidence: status=%d observed=%v", status, tc.capture.observed())
			}
		})
	}
}

func TestGuardBaselineResponseRejectsTerminalSetAbsenceWhenBodyCloseFails(t *testing.T) {
	capture := &baselineWireCapture{
		stage:                "terminal-set-absence",
		setID:                7,
		origin:               "https://api.example:443",
		runtimePathPrefix:    "/tenant/v2",
		runtimePathPrefixSet: true,
		allowedHosts:         []string{"api.example"},
	}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodGet, "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7?api-version=6.0-preview", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := guardBaselineResponse(req, &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       &baselineCloseErrorBody{Reader: strings.NewReader(""), err: io.ErrClosedPipe},
	})
	if !errors.Is(err, ErrRemote) || response != nil {
		t.Fatalf("terminal set absence body with close error = response %v err %v, want remote rejection", response, err)
	}
	_, _, _, status := capture.facts()
	if status != http.StatusNotFound || capture.observed() {
		t.Fatalf("terminal set absence close error published evidence: status=%d observed=%v", status, capture.observed())
	}
}

func TestGuardBaselineResponsePreservesTerminalSetAbsenceWhenBodyCloseSucceeds(t *testing.T) {
	capture := &baselineWireCapture{
		stage:                "terminal-set-absence",
		setID:                7,
		origin:               "https://api.example:443",
		runtimePathPrefix:    "/tenant/v2",
		runtimePathPrefixSet: true,
		allowedHosts:         []string{"api.example"},
	}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodGet, "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7?api-version=6.0-preview", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := guardBaselineResponse(req, &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       &baselineCloseErrorBody{Reader: strings.NewReader(""), err: nil},
	})
	if err != nil || response == nil || response.StatusCode != http.StatusNotFound || !capture.observed() {
		t.Fatalf("valid terminal set absence = response %v err %v observed=%v, want retained 404 evidence", response, err, capture.observed())
	}
}

type baselineRequestMutationRoundTripper struct {
	inner  http.RoundTripper
	mutate func(*http.Request)
}

func (t baselineRequestMutationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.mutate != nil {
		t.mutate(req)
	}
	return t.inner.RoundTrip(req)
}

type baselineJITRoundTripper func(*http.Request) (*http.Response, error)

func (f baselineJITRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBaselineMarkedJITRequestRejectsPhysicalTupleMutationBeforeInner(t *testing.T) {
	const (
		origin      = "https://api.example:443"
		prefix      = "/tenant/v2"
		runnerName  = "g01-test-worker-1"
		target      = "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/generatejitconfig?api-version=6.0-preview"
		requestBody = `{"name":"g01-test-worker-1","workFolder":"_work"}`
		foreignBody = `{"name":"foreign-worker","workFolder":"_work"}`
	)
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
	}{
		{
			name: "wrong physical origin",
			mutate: func(req *http.Request) {
				req.URL.Host = "other.example"
				req.Host = req.URL.Host
				req.URL.RawPath = ""
			},
		},
		{
			name: "wrong runtime tenant prefix",
			mutate: func(req *http.Request) {
				req.URL.Path = "/tenant/foreign/_apis/runtime/runnerscalesets/7/generatejitconfig"
				req.URL.RawPath = ""
			},
		},
		{
			name: "wrong request body",
			mutate: func(req *http.Request) {
				req.Body = io.NopCloser(strings.NewReader(foreignBody))
				req.ContentLength = int64(len(foreignBody))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{
				stage:                "jit",
				setID:                7,
				runnerName:           runnerName,
				origin:               origin,
				runtimePathPrefix:    prefix,
				runtimePathPrefixSet: true,
				allowedHosts:         []string{"api.example", "other.example"},
			}
			innerCalls := 0
			inner := baselineJITRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"encodedJITConfig":"AAAAAAAAAAAAAAAAAAAAAA=="}`)), Request: req}, nil
			})
			transport := responseBudgetTransport{inner: baselineRequestMutationRoundTripper{
				inner:  baselineRequestCaptureTransport{inner: inner},
				mutate: tc.mutate,
			}}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, target, strings.NewReader(requestBody))
			if err != nil {
				t.Fatal(err)
			}
			response, err := transport.RoundTrip(req)
			if !errors.Is(err, ErrRemote) || response != nil {
				t.Fatalf("mutated marked JIT = response %v err %v, want remote rejection", response, err)
			}
			if innerCalls != 0 {
				t.Fatalf("mutated marked JIT reached inner transport: calls=%d", innerCalls)
			}
			if capture.requestObserved() || capture.observed() {
				t.Fatalf("mutated marked JIT published evidence: requestObserved=%v observed=%v", capture.requestObserved(), capture.observed())
			}
		})
	}
}

func TestBaselineJITPreservesValidMarkedAndUnmarkedForwarding(t *testing.T) {
	const (
		origin      = "https://api.example:443"
		prefix      = "/tenant/v2"
		runnerName  = "g01-test-worker-1"
		target      = "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/generatejitconfig?api-version=6.0-preview"
		requestBody = `{"name":"g01-test-worker-1","workFolder":"_work"}`
	)
	capture := &baselineWireCapture{
		stage:                "jit",
		setID:                7,
		runnerName:           runnerName,
		origin:               origin,
		runtimePathPrefix:    prefix,
		runtimePathPrefixSet: true,
		allowedHosts:         []string{"api.example"},
	}
	innerCalls := 0
	inner := baselineJITRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"encodedJITConfig":"AAAAAAAAAAAAAAAAAAAAAA=="}`)), Request: req}, nil
	})
	transport := responseBudgetTransport{inner: baselineRequestCaptureTransport{inner: inner}}
	marked, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, target, strings.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(marked)
	if err != nil || response == nil || response.StatusCode != http.StatusOK || innerCalls != 1 || !capture.observed() {
		t.Fatalf("valid marked JIT = response %v err %v inner calls=%d observed=%v, want one accepted request", response, err, innerCalls, capture.observed())
	}

	unmarked, err := http.NewRequest(http.MethodPost, "https://other.example/rewritten", strings.NewReader(`{"name":"foreign-worker","workFolder":"foreign"}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err = transport.RoundTrip(unmarked)
	if err != nil || response == nil || response.StatusCode != http.StatusOK || innerCalls != 2 {
		t.Fatalf("unmarked forwarding = response %v err %v inner calls=%d, want forwarding", response, err, innerCalls)
	}
}

type replacingContextRoundTripper struct{ inner http.RoundTripper }

func (t replacingContextRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.inner.RoundTrip(req.Clone(context.Background()))
}

func TestBaselineMarkedBoundariesRejectReplacementContextBeforeInner(t *testing.T) {
	tests := []struct {
		name     string
		capture  *baselineWireCapture
		method   string
		target   string
		body     string
		status   int
		response string
	}{
		{
			name: "set-observe",
			capture: &baselineWireCapture{
				stage: "set-observe", setID: 7, organization: "fixture-org",
				allowedHosts: []string{"api.example"},
			},
			method:   http.MethodGet,
			target:   "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7?api-version=6.0-preview",
			status:   http.StatusOK,
			response: `{}`,
		},
		{
			name: "runner-observe",
			capture: &baselineWireCapture{
				stage: "runner-observe", setID: 7, runnerName: "fixture-runner",
				origin: "https://api.example:443", runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true,
				allowedHosts: []string{"api.example"},
			},
			method:   http.MethodGet,
			target:   "https://api.example/tenant/v2/_apis/distributedtask/pools/0/agents?agentName=fixture-runner&api-version=6.0-preview",
			status:   http.StatusOK,
			response: `{}`,
		},
		{
			name: "session-open",
			capture: &baselineWireCapture{
				stage: "session-open", setID: 7, organization: "fixture-org", owner: "fixture-owner",
				allowedHosts: []string{"api.example"},
			},
			method:   http.MethodPost,
			target:   "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview",
			body:     `{"sessionId":"00000000-0000-0000-0000-000000000000","ownerName":"fixture-owner"}`,
			status:   http.StatusOK,
			response: `{}`,
		},
		{
			name: "ack",
			capture: &baselineWireCapture{
				stage: "ack", setID: 7, sessionID: "session", queue: "https://api.example/tenant/v2/queue?proof=fixture", cursor: 41,
				origin: "https://api.example:443", runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true,
				allowedHosts: []string{"api.example"},
			},
			method: http.MethodDelete,
			target: "https://api.example/tenant/v2/queue/41?proof=fixture",
			status: http.StatusNoContent,
		},
		{
			name: "acquire",
			capture: &baselineWireCapture{
				stage: "acquire", setID: 7, requestIDs: []int64{41}, origin: "https://api.example:443",
				runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
			},
			method:   http.MethodPost,
			target:   "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview",
			body:     `[41]`,
			status:   http.StatusOK,
			response: `{"count":1,"value":[41]}`,
		},
		{
			name: "jit",
			capture: &baselineWireCapture{
				stage: "jit", setID: 7, origin: "https://api.example:443", runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true,
				allowedHosts: []string{"api.example"},
			},
			method:   http.MethodPost,
			target:   "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/generatejitconfig?api-version=6.0-preview",
			status:   http.StatusOK,
			response: `{"encodedJITConfig":"AAAAAAAAAAAAAAAAAAAAAA=="}`,
		},
		{
			name: "terminal-session-close",
			capture: &baselineWireCapture{
				stage: "terminal-session-close", setID: 7, sessionID: "session", origin: "https://api.example:443",
				runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
			},
			method: http.MethodDelete,
			target: "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview",
			status: http.StatusNoContent,
		},
		{
			name: "terminal-set",
			capture: &baselineWireCapture{
				stage: "terminal-set", setID: 7, origin: "https://api.example:443",
				runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
			},
			method:   http.MethodGet,
			target:   "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7?api-version=6.0-preview",
			status:   http.StatusOK,
			response: `{}`,
		},
		{
			name: "terminal-set-recheck",
			capture: &baselineWireCapture{
				stage: "terminal-set-recheck", setID: 7, origin: "https://api.example:443",
				runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
			},
			method:   http.MethodGet,
			target:   "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7?api-version=6.0-preview",
			status:   http.StatusOK,
			response: `{}`,
		},
		{
			name: "terminal-set-absence",
			capture: &baselineWireCapture{
				stage: "terminal-set-absence", setID: 7, origin: "https://api.example:443",
				runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
			},
			method: http.MethodGet,
			target: "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7?api-version=6.0-preview",
			status: http.StatusNotFound,
		},
		{
			name: "terminal-set-delete",
			capture: &baselineWireCapture{
				stage: "terminal-set-delete", setID: 7, origin: "https://api.example:443",
				runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
			},
			method: http.MethodDelete,
			target: "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7?api-version=6.0-preview",
			status: http.StatusNoContent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			innerCalls := 0
			inner := drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.response)), Request: req}, nil
			})
			transport := responseBudgetTransport{inner: replacingContextRoundTripper{inner: baselineRequestCaptureTransport{inner: inner}}}
			req, err := http.NewRequestWithContext(tc.capture.context(context.Background()), tc.method, tc.target, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			response, err := transport.RoundTrip(req)
			if !errors.Is(err, ErrRemote) || response != nil {
				t.Fatalf("replacement context boundary = response %v err %v, want remote rejection", response, err)
			}
			if innerCalls != 0 {
				t.Fatalf("replacement context reached inner transport: calls=%d", innerCalls)
			}
		})
	}
}

func TestBaselineWireMarkerIsRemovedBeforeInner(t *testing.T) {
	capture := &baselineWireCapture{
		stage: "acquire", setID: 7, requestIDs: []int64{41}, origin: "https://api.example:443",
		runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
	}
	innerCalls := 0
	inner := drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		if values := baselineWireMarkerValues(req); len(values) != 0 {
			t.Fatalf("private marker reached inner transport: %v", values)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"count":1,"value":[41]}`)), Request: req}, nil
	})
	transport := responseBudgetTransport{inner: baselineRequestCaptureTransport{inner: inner}}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", strings.NewReader("[41]"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("valid marked acquisition = %v", err)
	}
	if innerCalls != 1 || !capture.observed() {
		t.Fatalf("valid marked acquisition calls=%d observed=%v, want one accepted request", innerCalls, capture.observed())
	}
}
