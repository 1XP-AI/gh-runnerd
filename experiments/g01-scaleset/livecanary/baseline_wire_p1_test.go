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
