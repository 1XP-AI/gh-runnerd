package livecanary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
)

func TestBaselineAcquireTargetIsActionsOnly(t *testing.T) {
	capture := &baselineWireCapture{
		stage:        "acquire",
		setID:        7,
		queue:        "https://queue.example/queue?access_token=fixture",
		allowedHosts: []string{"api.example"},
		origin:       "https://api.example:443",
	}
	for _, tc := range []struct {
		name   string
		method string
		target string
		want   bool
	}{
		{name: "actions endpoint", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", want: true},
		{name: "queue endpoint", method: http.MethodPost, target: "https://queue.example/queue/acquirejobs?access_token=fixture", want: false},
		{name: "wrong host", method: http.MethodPost, target: "https://other.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", want: false},
		{name: "wrong set path", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets/8/acquirejobs?api-version=6.0-preview", want: false},
		{name: "wrong endpoint path", method: http.MethodPost, target: "https://api.example/runnerscalesets/7/acquirejobs?api-version=6.0-preview", want: false},
		{name: "wrong method", method: http.MethodGet, target: "https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := capture.target(req); got != tc.want {
				t.Fatalf("acquisition target match = %v, want %v for %s", got, tc.want, tc.target)
			}
		})
	}
}

func TestBaselineAcquireTargetMismatchStopsBeforeInner(t *testing.T) {
	for _, target := range []string{
		"https://other.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview",
		"https://api.example/_apis/runtime/runnerscalesets/8/acquirejobs?api-version=6.0-preview",
		"https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview&extra=1",
		"https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview&bad=%zz",
		"https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview;extra=1",
		"https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview&api-version=6.0-preview",
		"https://api.example/_apis/runtime/runnerscalesets/7/wrong?api-version=6.0-preview",
		"https://api.example/_apis/runtime/RunnerScaleSets/7/AcquireJobs?api-version=6.0-preview",
		"https://api.example/_apis/runtime/runnersets/7/acquire?api-version=6.0-preview",
	} {
		t.Run(target, func(t *testing.T) {
			capture := &baselineWireCapture{
				stage:      "acquire",
				setID:      7,
				requestIDs: []int64{41},
				allowedHosts: []string{
					"api.example",
				},
			}
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
			})}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, target, strings.NewReader("[41]"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := transport.RoundTrip(req); !errors.Is(err, ErrRemote) {
				t.Fatalf("mismatched acquisition target error = %v, want remote rejection", err)
			}
			if innerCalls != 0 {
				t.Fatalf("mismatched acquisition target reached inner transport: calls=%d", innerCalls)
			}
			if capture.requestObserved() {
				t.Fatal("mismatched acquisition target was marked observed")
			}
		})
	}
}

func TestBaselineMarkedAcquireWithoutIDsStopsBeforeInner(t *testing.T) {
	capture := &baselineWireCapture{
		stage:        "acquire",
		setID:        7,
		origin:       "https://api.example:443",
		allowedHosts: []string{"api.example"},
	}
	innerCalls := 0
	transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, "https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", strings.NewReader("[]"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(req); !errors.Is(err, ErrRemote) {
		t.Fatalf("marked acquisition without IDs error = %v, want remote rejection", err)
	}
	if innerCalls != 0 {
		t.Fatalf("marked acquisition without IDs reached inner transport: calls=%d", innerCalls)
	}
	if capture.requestObserved() {
		t.Fatal("marked acquisition without IDs was marked observed")
	}
}

func TestBaselineSessionOpenTargetMismatchStopsBeforeInner(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		target string
		reject bool
	}{
		{name: "valid session-open", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview"},
		{name: "wrong host", method: http.MethodPost, target: "https://other.example/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview", reject: true},
		{name: "wrong scale set", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets/8/sessions?api-version=6.0-preview", reject: true},
		{name: "wrong endpoint path", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets/7/not-session?api-version=6.0-preview", reject: true},
		{name: "wrong method", method: http.MethodGet, target: "https://api.example/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview", reject: true},
		{name: "wrong query", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview&unexpected=1", reject: true},
		{name: "malformed query escape", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview&bad=%zz", reject: true},
		{name: "duplicate query", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview&api-version=6.0-preview", reject: true},
		{name: "semicolon query", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview;unexpected=1", reject: true},
		{name: "case route family", method: http.MethodPost, target: "https://api.example/_apis/runtime/RunnerScaleSets/7/sessions?api-version=6.0-preview", reject: true},
		{name: "route family delimiter", method: http.MethodPost, target: "https://api.example/_apis/runtime/runnerscalesets7/sessions?api-version=6.0-preview", reject: true},
		{name: "route family omitted", method: http.MethodPost, target: "https://api.example/_apis/runtime/sessions?api-version=6.0-preview", reject: true},
		{name: "registration bootstrap", method: http.MethodPost, target: "https://api.example/orgs/fixture-org/actions/runners/registration-token"},
		{name: "actions bootstrap", method: http.MethodPost, target: "https://api.example/actions/runner-registration"},
		{name: "bootstrap organization collision", method: http.MethodPost, target: "https://api.example/orgs/runnerscalesets/actions/runners/registration-token"},
		{name: "bootstrap organization collision actions", method: http.MethodPost, target: "https://api.example/orgs/runnerscalesets/actions/runner-registration"},
		{name: "bootstrap wrong organization", method: http.MethodPost, target: "https://api.example/orgs/other-org/actions/runners/registration-token", reject: true},
		{name: "bootstrap extra path", method: http.MethodPost, target: "https://api.example/orgs/fixture-org/actions/runners/registration-token/extra", reject: true},
		{name: "bootstrap unexpected query", method: http.MethodPost, target: "https://api.example/orgs/fixture-org/actions/runners/registration-token?unexpected=1", reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			organization := "fixture-org"
			if strings.Contains(tc.name, "organization collision") {
				organization = "runnerscalesets"
			}
			capture := &baselineWireCapture{stage: "session-open", setID: 7, organization: organization, owner: "g01-test", allowedHosts: []string{"api.example"}}
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
			})}
			body := "{}"
			if tc.name == "valid session-open" {
				body = `{"sessionId":"00000000-0000-0000-0000-000000000000","ownerName":"g01-test"}`
			}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), tc.method, tc.target, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			_, err = transport.RoundTrip(req)
			if tc.reject {
				if !errors.Is(err, ErrRemote) {
					t.Fatalf("mismatched session-open target error = %v, want remote rejection", err)
				}
				if innerCalls != 0 {
					t.Fatalf("mismatched session-open target reached inner transport: calls=%d", innerCalls)
				}
				if capture.observed() {
					t.Fatal("mismatched session-open target was marked observed")
				}
				return
			}
			if err != nil {
				t.Fatalf("valid or bootstrap request error = %v", err)
			}
			if innerCalls != 1 {
				t.Fatalf("valid or bootstrap request inner calls = %d, want one", innerCalls)
			}
		})
	}
}

func TestBaselineSessionOpenBodyMustMatchOwnerBeforeInner(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{name: "approved owner", body: `{"sessionId":"00000000-0000-0000-0000-000000000000","ownerName":"g01-test"}`, want: true},
		{name: "missing session id", body: `{"ownerName":"g01-test"}`},
		{name: "missing owner", body: `{}`},
		{name: "wrong owner", body: `{"ownerName":"foreign-owner"}`},
		{name: "case-fold duplicate owner", body: `{"ownerName":"g01-test","OwnerName":"foreign-owner"}`},
		{name: "unknown field", body: `{"ownerName":"g01-test","unexpected":1}`},
		{name: "malformed", body: `{"ownerName":"g01-test"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{stage: "session-open", setID: 7, owner: "g01-test", organization: "fixture-org", allowedHosts: []string{"api.example"}}
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
			})}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, "https://api.example/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			_, err = transport.RoundTrip(req)
			if tc.want {
				if err != nil || innerCalls != 1 {
					t.Fatalf("approved session-open body err=%v inner calls=%d, want one forwarded request", err, innerCalls)
				}
				return
			}
			if !errors.Is(err, ErrRemote) {
				t.Fatalf("ambiguous session-open body error = %v, want remote rejection", err)
			}
			if innerCalls != 0 {
				t.Fatalf("ambiguous session-open body reached inner transport: calls=%d", innerCalls)
			}
		})
	}
}

func TestBaselineSessionOpenRejectsDuplicateMarkedPOSTBeforeInner(t *testing.T) {
	capture := &baselineWireCapture{
		stage:        "session-open",
		setID:        7,
		owner:        "g01-test",
		organization: "fixture-org",
		allowedHosts: []string{"api.example"},
	}
	innerCalls := 0
	transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})}
	request := func() *http.Request {
		req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, "https://api.example/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview", strings.NewReader(`{"sessionId":"00000000-0000-0000-0000-000000000000","ownerName":"g01-test"}`))
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	if _, err := transport.RoundTrip(request()); err != nil {
		t.Fatalf("first marked session-open = %v, want forwarding", err)
	}
	if _, err := transport.RoundTrip(request()); !errors.Is(err, ErrRemote) {
		t.Fatalf("duplicate marked session-open = %v, want remote rejection", err)
	}
	if innerCalls != 1 {
		t.Fatalf("duplicate marked session-open reached inner transport: calls=%d, want one", innerCalls)
	}
}

func TestBaselineSnapshotRequestsRequireExactOriginBeforeInner(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage string
		path  string
	}{
		{name: "scale-set wrong origin", stage: "set-observe", path: "/tenant/v2/_apis/runtime/runnerscalesets/7"},
		{name: "runner wrong origin", stage: "runner-observe", path: "/tenant/v2/_apis/distributedtask/pools/0/agents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{stage: tc.stage, setID: 7, runnerName: "g01-test-worker-1", origin: "https://api.example:443", allowedHosts: []string{"api.example", "other.example"}}
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
			})}
			query := "api-version=6.0-preview"
			if tc.stage == "runner-observe" {
				query = "agentName=g01-test-worker-1&api-version=6.0-preview"
			}
			method := http.MethodGet
			target := "https://other.example" + tc.path + "?" + query
			req, err := http.NewRequestWithContext(capture.context(context.Background()), method, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = transport.RoundTrip(req)
			if !errors.Is(err, ErrRemote) {
				t.Fatalf("%s error = %v, want remote rejection", tc.name, err)
			}
			if innerCalls != 0 {
				t.Fatalf("%s reached inner transport: calls=%d", tc.name, innerCalls)
			}
		})
	}
}

func TestBaselineRuntimePathPrefixMismatchStopsBeforeInner(t *testing.T) {
	const (
		origin = "https://api.example:443"
		prefix = "/tenant/approved"
	)
	tests := []struct {
		name   string
		stage  string
		method string
		path   string
		body   string
	}{
		{name: "scale-set snapshot", stage: "set-observe", method: http.MethodGet, path: "/tenant/foreign/_apis/runtime/runnerscalesets/7?api-version=6.0-preview"},
		{name: "runner snapshot", stage: "runner-observe", method: http.MethodGet, path: "/tenant/foreign/_apis/distributedtask/pools/0/agents?agentName=g01-test-worker-1&api-version=6.0-preview"},
		{name: "session open", stage: "session-open", method: http.MethodPost, path: "/tenant/foreign/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview", body: `{"sessionId":"00000000-0000-0000-0000-000000000000","ownerName":"g01-test"}`},
		{name: "acquisition", stage: "acquire", method: http.MethodPost, path: "/tenant/foreign/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", body: `[41]`},
		{name: "session close", stage: "terminal-session-close", method: http.MethodDelete, path: "/tenant/foreign/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{stage: tc.stage, setID: 7, owner: "g01-test", runnerName: "g01-test-worker-1", sessionID: "session", origin: origin, runtimePathPrefix: prefix, runtimePathPrefixSet: true, requestIDs: []int64{41}, allowedHosts: []string{"api.example"}}
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
			})}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), tc.method, "https://api.example"+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := transport.RoundTrip(req); !errors.Is(err, ErrRemote) {
				t.Fatalf("same-origin foreign-prefix request = %v, want remote rejection", err)
			}
			if innerCalls != 0 {
				t.Fatalf("same-origin foreign-prefix request reached inner transport: calls=%d", innerCalls)
			}
		})
	}
}

func TestDrainListenerRejectsMarkedPollTargetMismatchBeforeInner(t *testing.T) {
	const target = "https://fixture.invalid/queue?proof=fixture"
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
	}{
		{name: "wrong host", mutate: func(req *http.Request) { req.URL.Host = "other.invalid" }},
		{name: "overridden Host", mutate: func(req *http.Request) { req.Host = "other.invalid" }},
		{name: "wrong path", mutate: func(req *http.Request) { req.URL.Path = "/wrong-queue"; req.URL.RawPath = "" }},
		{name: "wrong origin scheme", mutate: func(req *http.Request) { req.URL.Scheme = "http" }},
		{name: "wrong queue proof", mutate: func(req *http.Request) {
			query := req.URL.Query()
			query.Set("proof", "other")
			req.URL.RawQuery = query.Encode()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hook := newDrainPollHook(target)
			hook.mu.Lock()
			hook.origin = "https://fixture.invalid:443"
			hook.runtimePathPrefix = "/tenant/v2"
			hook.runtimePathPrefixSet = true
			hook.mu.Unlock()
			innerCalls := 0
			hook.inner = drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
			})
			session := &drainSyntheticSession{
				client: &http.Client{Transport: sdkRequestMutationRoundTripper{inner: hook, mutate: tc.mutate}},
				initial: scaleset.RunnerScaleSetSession{
					SessionID: uuid.New(), OwnerName: "fixture-owner", MessageQueueURL: target,
					Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
				},
				order: new([]string),
			}
			_, err := runDrainListener(context.Background(), session, 7, hook)
			if err == nil {
				t.Fatal("marked poll target mismatch was accepted")
			}
			if innerCalls != 0 {
				t.Fatalf("marked poll target mismatch reached inner transport: calls=%d", innerCalls)
			}
		})
	}
}

func TestMarkedRequestHostOverrideStopsBeforeInner(t *testing.T) {
	const (
		origin = "https://api.example:443"
		prefix = "/tenant/v2"
	)
	for _, tc := range []struct {
		name   string
		stage  string
		method string
		target string
		body   string
	}{
		{
			name:   "session-open",
			stage:  "session-open",
			method: http.MethodPost,
			target: "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview",
			body:   `{"sessionId":"00000000-0000-0000-0000-000000000000","ownerName":"g01-test"}`,
		},
		{
			name:   "ACK",
			stage:  "ack",
			method: http.MethodDelete,
			target: "https://api.example/tenant/v2/queue/41?proof=fixture",
		},
		{
			name:   "acquisition",
			stage:  "acquire",
			method: http.MethodPost,
			target: "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview",
			body:   "[41]",
		},
		{
			name:   "session-close",
			stage:  "terminal-session-close",
			method: http.MethodDelete,
			target: "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{
				stage: tc.stage, setID: 7, owner: "g01-test", organization: "fixture-org", sessionID: "session",
				queue: "https://api.example/tenant/v2/queue?proof=fixture", cursor: 41,
				origin: origin, runtimePathPrefix: prefix, runtimePathPrefixSet: true, requestIDs: []int64{41}, allowedHosts: []string{"api.example"},
			}
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
			})}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), tc.method, tc.target, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Host = "other.example"
			_, err = transport.RoundTrip(req)
			if !errors.Is(err, ErrRemote) {
				t.Fatalf("marked %s Host override error = %v, want remote rejection", tc.name, err)
			}
			if innerCalls != 0 {
				t.Fatalf("marked %s Host override reached inner transport: calls=%d", tc.name, innerCalls)
			}
			if capture.requestObserved() {
				t.Fatalf("marked %s Host override was marked observed", tc.name)
			}
		})
	}
}

func TestMarkedRequestHostMatchingURLHostPreservesForwarding(t *testing.T) {
	capture := &baselineWireCapture{
		stage: "acquire", setID: 7, origin: "https://api.example:443", runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true,
		requestIDs: []int64{41}, allowedHosts: []string{"api.example"},
	}
	innerCalls := 0
	transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", strings.NewReader("[41]"))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = req.URL.Host
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("marked request with URL Host override = %v, want forwarding", err)
	}
	if innerCalls != 1 {
		t.Fatalf("marked request with URL Host override inner calls = %d, want one", innerCalls)
	}
}

func TestDrainPollHookRejectsApprovalFromDifferentHook(t *testing.T) {
	const target = "https://fixture.invalid/queue?proof=fixture"
	hook := newDrainPollHook(target)
	other := newDrainPollHook(target)
	innerCalls := 0
	hook.inner = drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	req, err := http.NewRequestWithContext(other.markPoll(context.Background()), http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(scaleset.HeaderScaleSetMaxCapacity, "1")
	if _, err := hook.RoundTrip(req); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("foreign poll approval error = %v, want quarantine", err)
	}
	if innerCalls != 0 {
		t.Fatalf("foreign poll approval reached inner transport: calls=%d", innerCalls)
	}
}

func TestDrainPollHookForwardsUnmarkedNonPollRequest(t *testing.T) {
	hook := newDrainPollHook("https://fixture.invalid/queue?proof=fixture")
	innerCalls := 0
	hook.inner = drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	req, err := http.NewRequest(http.MethodPost, "https://fixture.invalid/_apis/runtime/runnerscalesets/7/sessions?api-version=6.0-preview", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "override.invalid"
	if _, err := hook.RoundTrip(req); err != nil {
		t.Fatalf("unmarked session request: %v", err)
	}
	if innerCalls != 1 {
		t.Fatalf("unmarked session request inner calls = %d, want one", innerCalls)
	}
}

func TestBaselineAcquireTargetRequiresCapturedSessionOrigin(t *testing.T) {
	for _, tc := range []struct {
		name   string
		origin string
		target string
		want   bool
	}{
		{name: "captured origin", origin: "https://api.example:443", target: "https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", want: true},
		{name: "second approved origin", origin: "https://api.example:443", target: "https://actions.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview"},
		{name: "missing captured origin", target: "https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{stage: "acquire", setID: 7, origin: tc.origin, allowedHosts: []string{"api.example", "actions.example"}}
			req, err := http.NewRequest(http.MethodPost, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := capture.target(req); got != tc.want {
				t.Fatalf("acquisition target match = %v, want %v for %s", got, tc.want, tc.target)
			}
		})
	}
}

func TestBaselineAcquireOriginMismatchStopsBeforeInner(t *testing.T) {
	capture := &baselineWireCapture{stage: "acquire", setID: 7, origin: "https://api.example:443", requestIDs: []int64{41}, allowedHosts: []string{"api.example", "actions.example"}}
	innerCalls := 0
	transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, "https://actions.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", strings.NewReader("[41]"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(req); !errors.Is(err, ErrRemote) {
		t.Fatalf("mismatched acquisition origin error = %v, want remote rejection", err)
	}
	if innerCalls != 0 {
		t.Fatalf("mismatched acquisition origin reached inner transport: calls=%d", innerCalls)
	}
}

func TestBaselineSessionCloseTargetRequiresExactOrigin(t *testing.T) {
	capture := &baselineWireCapture{stage: "terminal-session-close", setID: 7, sessionID: "session", origin: "https://actions.example:443", runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true}
	for _, tc := range []struct {
		name   string
		target string
		want   bool
	}{
		{name: "expected origin and dynamic path", target: "https://actions.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview", want: true},
		{name: "wrong scheme", target: "http://actions.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview", want: false},
		{name: "wrong host", target: "https://other.actions.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview", want: false},
		{name: "wrong port", target: "https://actions.example:8443/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview", want: false},
		{name: "malformed query escape", target: "https://actions.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview&bad=%zz", want: false},
		{name: "semicolon query", target: "https://actions.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview;bad=1", want: false},
		{name: "duplicate query", target: "https://actions.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview&api-version=6.0-preview", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodDelete, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := capture.target(req); got != tc.want {
				t.Fatalf("session-close target match = %v, want %v for %s", got, tc.want, tc.target)
			}
		})
	}
}

func TestBaselineMarkedACKMismatchStopsBeforeInner(t *testing.T) {
	const (
		queue  = "https://api.example/tenant/v2/queue?proof=fixture"
		origin = "https://api.example:443"
		prefix = "/tenant/v2"
	)
	for _, tc := range []struct {
		name   string
		target string
	}{
		{name: "wrong message", target: "https://api.example/tenant/v2/queue/42?proof=fixture"},
		{name: "wrong path", target: "https://api.example/tenant/v2/other/41?proof=fixture"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{
				stage: "ack", setID: 7, sessionID: "session", queue: queue, cursor: 41,
				origin: origin, runtimePathPrefix: prefix, runtimePathPrefixSet: true,
				allowedHosts: []string{"api.example"},
			}
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
			})}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodDelete, tc.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = transport.RoundTrip(req)
			if innerCalls != 0 {
				t.Fatalf("mismatched marked ACK reached inner transport: calls=%d", innerCalls)
			}
			if !errors.Is(err, ErrRemote) {
				t.Fatalf("mismatched marked ACK error = %v, want remote rejection", err)
			}
			if capture.requestObserved() {
				t.Fatal("mismatched marked ACK was marked observed")
			}
		})
	}
}

func TestBaselineMarkedACKRequiresIdentityAndOneShotCardinality(t *testing.T) {
	const target = "https://api.example/tenant/v2/queue/41?proof=fixture"
	for _, tc := range []struct {
		name   string
		mutate func(*baselineWireCapture)
	}{
		{name: "missing origin", mutate: func(c *baselineWireCapture) { c.origin = "" }},
		{name: "missing tenant prefix", mutate: func(c *baselineWireCapture) { c.runtimePathPrefix = ""; c.runtimePathPrefixSet = false }},
		{name: "missing scale set", mutate: func(c *baselineWireCapture) { c.setID = 0 }},
		{name: "missing session", mutate: func(c *baselineWireCapture) { c.sessionID = "" }},
		{name: "unapproved queue origin", mutate: func(c *baselineWireCapture) { c.queue = "https://other.example/tenant/v2/queue?proof=fixture" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{
				stage: "ack", setID: 7, sessionID: "session", queue: "https://api.example/tenant/v2/queue?proof=fixture", cursor: 41,
				origin: "https://api.example:443", runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true,
				allowedHosts: []string{"api.example"},
			}
			tc.mutate(capture)
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
			})}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodDelete, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := transport.RoundTrip(req); !errors.Is(err, ErrRemote) {
				t.Fatalf("invalid marked ACK identity error = %v, want remote rejection", err)
			}
			if innerCalls != 0 {
				t.Fatalf("invalid marked ACK identity reached inner transport: calls=%d", innerCalls)
			}
		})
	}

	capture := &baselineWireCapture{
		stage: "ack", setID: 7, sessionID: "session", queue: "https://api.example/tenant/v2/queue?proof=fixture", cursor: 41,
		origin: "https://api.example:443", runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true,
		allowedHosts: []string{"api.example"},
	}
	innerCalls := 0
	transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
	})}
	for i := 0; i < 2; i++ {
		req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodDelete, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = transport.RoundTrip(req)
		if i == 0 && err != nil {
			t.Fatalf("valid marked ACK = %v, want forwarding", err)
		}
		if i == 1 && !errors.Is(err, ErrRemote) {
			t.Fatalf("duplicate marked ACK = %v, want remote rejection", err)
		}
	}
	if innerCalls != 1 {
		t.Fatalf("marked ACK physical cardinality = %d, want one inner call", innerCalls)
	}
}

func TestBaselineMarkedSessionCloseMismatchStopsBeforeInner(t *testing.T) {
	const (
		origin = "https://api.example:443"
		prefix = "/tenant/v2"
	)
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "route family omitted", path: "/tenant/v2/_apis/runtime/sessions/session?api-version=6.0-preview"},
		{name: "wrong session path", path: "/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/other?api-version=6.0-preview"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{
				stage: "terminal-session-close", setID: 7, sessionID: "session", origin: origin,
				runtimePathPrefix: prefix, runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
			}
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
			})}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodDelete, "https://api.example"+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = transport.RoundTrip(req)
			if innerCalls != 0 {
				t.Fatalf("mismatched marked session-close reached inner transport: calls=%d", innerCalls)
			}
			if !errors.Is(err, ErrRemote) {
				t.Fatalf("mismatched marked session-close error = %v, want remote rejection", err)
			}
			if capture.requestObserved() {
				t.Fatal("mismatched marked session-close was marked observed")
			}
		})
	}
}

func TestBaselineMarkedSessionCloseRequiresIdentityAndOneShotCardinality(t *testing.T) {
	const target = "https://api.example/tenant/v2/_apis/runtime/runnerscalesets/7/sessions/session?api-version=6.0-preview"
	for _, tc := range []struct {
		name   string
		mutate func(*baselineWireCapture)
	}{
		{name: "missing origin", mutate: func(c *baselineWireCapture) { c.origin = "" }},
		{name: "missing tenant prefix", mutate: func(c *baselineWireCapture) { c.runtimePathPrefix = ""; c.runtimePathPrefixSet = false }},
		{name: "missing scale set", mutate: func(c *baselineWireCapture) { c.setID = 0 }},
		{name: "missing session", mutate: func(c *baselineWireCapture) { c.sessionID = "" }},
		{name: "wrong origin", mutate: func(c *baselineWireCapture) { c.origin = "https://other.example:443" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &baselineWireCapture{
				stage: "terminal-session-close", setID: 7, sessionID: "session", origin: "https://api.example:443",
				runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
			}
			tc.mutate(capture)
			innerCalls := 0
			transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
				innerCalls++
				return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
			})}
			req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodDelete, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := transport.RoundTrip(req); !errors.Is(err, ErrRemote) {
				t.Fatalf("invalid marked session-close identity error = %v, want remote rejection", err)
			}
			if innerCalls != 0 {
				t.Fatalf("invalid marked session-close identity reached inner transport: calls=%d", innerCalls)
			}
		})
	}

	capture := &baselineWireCapture{
		stage: "terminal-session-close", setID: 7, sessionID: "session", origin: "https://api.example:443",
		runtimePathPrefix: "/tenant/v2", runtimePathPrefixSet: true, allowedHosts: []string{"api.example"},
	}
	innerCalls := 0
	transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
	})}
	for i := 0; i < 2; i++ {
		req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodDelete, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = transport.RoundTrip(req)
		if i == 0 && err != nil {
			t.Fatalf("valid marked session-close = %v, want forwarding", err)
		}
		if i == 1 && !errors.Is(err, ErrRemote) {
			t.Fatalf("duplicate marked session-close = %v, want remote rejection", err)
		}
	}
	if innerCalls != 1 {
		t.Fatalf("marked session-close physical cardinality = %d, want one inner call", innerCalls)
	}
}

func TestUnmarkedDeletePreservesInnerTransport(t *testing.T) {
	innerCalls := 0
	transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		innerCalls++
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
	})}
	req, err := http.NewRequest(http.MethodDelete, "http://unmarked.invalid/arbitrary-target", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "override.invalid"
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("unmarked delete = %v, want forwarding", err)
	}
	if innerCalls != 1 {
		t.Fatalf("unmarked delete inner calls = %d, want one", innerCalls)
	}
}

func TestBaselineAcquireForwardingBodySurvivesAsyncRoundTripClose(t *testing.T) {
	capture := &baselineWireCapture{stage: "acquire", setID: 7, origin: "https://api.example:443", requestIDs: []int64{41}, allowedHosts: []string{"api.example"}}
	release := make(chan struct{})
	returned := make(chan struct{})
	readBody := make(chan string, 1)
	transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		go func() {
			<-release
			data, _ := io.ReadAll(req.Body)
			_ = req.Body.Close()
			readBody <- string(data)
		}()
		close(returned)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, "https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", strings.NewReader("[41]"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("valid acquisition transport = %v", err)
	}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("inner transport did not return")
	}
	close(release)
	select {
	case got := <-readBody:
		if got != "[41]" {
			t.Fatalf("asynchronous forwarding body = %q, want valid request bytes", got)
		}
	case <-time.After(time.Second):
		t.Fatal("asynchronous forwarding read did not finish")
	}
}

func TestBaselineAcquireForwardingBodyConcurrentReadCloseIsSafe(t *testing.T) {
	capture := &baselineWireCapture{stage: "acquire", setID: 7, origin: "https://api.example:443", requestIDs: []int64{41}, allowedHosts: []string{"api.example"}}
	start := make(chan struct{})
	done := make(chan struct{}, 2)
	transport := baselineRequestCaptureTransport{inner: drainRoundTripper(func(req *http.Request) (*http.Response, error) {
		go func() {
			<-start
			buf := make([]byte, 1)
			for i := 0; i < 1024; i++ {
				_, _ = req.Body.Read(buf)
			}
			done <- struct{}{}
		}()
		go func() {
			<-start
			for i := 0; i < 1024; i++ {
				_ = req.Body.Close()
			}
			done <- struct{}{}
		}()
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})}
	req, err := http.NewRequestWithContext(capture.context(context.Background()), http.MethodPost, "https://api.example/_apis/runtime/runnerscalesets/7/acquirejobs?api-version=6.0-preview", strings.NewReader("[41]"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("valid acquisition transport = %v", err)
	}
	close(start)
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("concurrent request body operation did not finish")
		}
	}
}

func TestPinnedSDKDrainRejectsPhysicalPollMutationBeforeInner(t *testing.T) {
	a := approval()
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
	}{
		{
			name: "withdrawn capacity header",
			mutate: func(req *http.Request) {
				if req.Method == http.MethodGet && req.URL.Path == "/queue" && req.URL.Query().Get("lastMessageId") != "" {
					req.Header.Set("X-ScaleSetMaxCapacity", "1")
				}
			},
		},
		{
			name: "withdrawn cursor",
			mutate: func(req *http.Request) {
				if req.Method == http.MethodGet && req.URL.Path == "/queue" && req.URL.Query().Get("lastMessageId") != "" {
					q := req.URL.Query()
					q.Set("lastMessageId", "18")
					req.URL.RawQuery = q.Encode()
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, _, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{mutateRequest: tc.mutate})
			observation, err := runDrainListener(context.Background(), session, 7, hook)
			if !errors.Is(err, ErrQuarantine) || observation.Outcome == drainOutcomeObserved {
				t.Fatalf("physical poll mutation was promoted: observation=%+v err=%v", observation, err)
			}
			if got := fixture.polls.Load(); got != 1 {
				t.Fatalf("mutated withdrawn poll reached server: polls=%d, want first poll only", got)
			}
		})
	}
}

func TestDrainCancellationStopsBeforeReleasingHeldResponse(t *testing.T) {
	var order []string
	cancelAndJoinDrain(
		func() { order = append(order, "cancel") },
		func() { order = append(order, "release") },
		func() { order = append(order, "join") },
	)
	if got, want := strings.Join(order, ","), "cancel,release,join"; got != want {
		t.Fatalf("cancellation order = %s, want %s", got, want)
	}
}

func TestPinnedSDKDrainRejectsWithdrawnPollBodyBeforeAbsent(t *testing.T) {
	a := approval()
	job := pinnedDrainJobBody(a)
	body := fmt.Sprintf(`{"messageId":18,"messageType":"RunnerScaleSetJobMessages","statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1},"body":%s}`, mustJSONQuote(job))
	fixture, _, session, hook := newPinnedDrainSessionOptions(t, a, job, pinnedDrainOptions{withdrawnBody: body})
	observation, err := runDrainListener(context.Background(), session, 7, hook)
	if !errors.Is(err, ErrQuarantine) || observation.Outcome == drainOutcomeObserved {
		t.Fatalf("withdrawn 202 body was classified as absent: observation=%+v err=%v", observation, err)
	}
	if fixture.polls.Load() != 2 || fixture.acks.Load() != 1 || fixture.acquires.Load() != 1 {
		t.Fatalf("withdrawn body effects = polls %d ack %d acquire %d, want one bounded poll plus old effects", fixture.polls.Load(), fixture.acks.Load(), fixture.acquires.Load())
	}
}

func TestPinnedSDKDrainRequiresCompletePollStatsBeforeVerifyRun(t *testing.T) {
	a := approval()
	job := pinnedDrainJobBody(a)
	first := fmt.Sprintf(`{"messageId":17,"messageType":"RunnerScaleSetJobMessages","statistics":{"totalRegisteredRunners":1,"totalIdleRunners":1},"body":%s}`, mustJSONQuote(job))
	fixture, base, session, hook := newPinnedDrainSessionOptions(t, a, job, pinnedDrainOptions{firstPollBody: first})
	d := &Driver{Approval: a, Journal: &memoryJournal{}, API: base}
	c := &journaledDrainClient{d: d, inner: session, sessionID: session.Session().SessionID.String(), hook: hook}
	hook.releaseResponse()
	if _, err := c.GetMessage(context.Background(), 0, drainInitialCapacity); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("incomplete poll statistics = %v, want quarantine", err)
	}
	if fixture.verifyCalls.Load() != 0 {
		t.Fatalf("incomplete poll statistics crossed VerifyRun: calls=%d", fixture.verifyCalls.Load())
	}
	if fixture.acks.Load() != 0 || fixture.acquires.Load() != 0 {
		t.Fatalf("incomplete poll statistics reached effects: ack=%d acquire=%d", fixture.acks.Load(), fixture.acquires.Load())
	}
}

func TestPinnedSDKDrainVerifyRunRejectsAmbiguousWireFieldsBeforeEffects(t *testing.T) {
	a := approval()
	for _, tc := range []struct {
		name string
		body string
		good bool
	}{
		{name: "success", body: pinnedDrainRunJSON(a), good: true},
		{name: "duplicate exact head", body: ambiguousRunJSON(a, "duplicate-head"), good: false},
		{name: "duplicate casefold head", body: ambiguousRunJSON(a, "casefold-head"), good: false},
		{name: "null", body: "null", good: false},
		{name: "missing head", body: ambiguousRunJSON(a, "missing-head"), good: false},
		{name: "contradictory head", body: ambiguousRunJSON(a, "wrong-head"), good: false},
		{name: "malformed", body: `{"id":5`, good: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, base, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{verifyBody: tc.body})
			d := &Driver{Approval: a, Journal: &memoryJournal{}, API: base}
			c := &journaledDrainClient{d: d, inner: session, sessionID: session.Session().SessionID.String(), hook: hook}
			hook.releaseResponse()
			_, err := c.GetMessage(context.Background(), 0, drainInitialCapacity)
			if tc.good {
				if err != nil {
					t.Fatalf("valid VerifyRun = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrApproval) && !errors.Is(err, ErrQuarantine) {
				t.Fatalf("ambiguous VerifyRun = %v, want fixed rejection", err)
			}
			if fixture.acks.Load() != 0 || fixture.acquires.Load() != 0 {
				t.Fatalf("ambiguous VerifyRun reached effects: ack=%d acquire=%d", fixture.acks.Load(), fixture.acquires.Load())
			}
		})
	}
}

func TestPinnedSDKDrainAcquisitionRequiresStrictWireResponse(t *testing.T) {
	a := approval()
	for _, tc := range []struct {
		name string
		body string
		good bool
	}{
		{name: "success", body: `{"count":1,"value":[41]}`, good: true},
		{name: "duplicate count", body: `{"count":0,"count":1,"value":[41]}`, good: false},
		{name: "casefold count", body: `{"Count":0,"count":1,"value":[41]}`, good: false},
		{name: "duplicate value", body: `{"count":1,"value":[99],"value":[41]}`, good: false},
		{name: "casefold value", body: `{"count":1,"Value":[99],"value":[41]}`, good: false},
		{name: "missing count", body: `{"value":[41]}`, good: false},
		{name: "count mismatch", body: `{"count":2,"value":[41]}`, good: false},
		{name: "null count", body: `{"count":null,"value":[41]}`, good: false},
		{name: "malformed", body: `{"count":1,"value":[41]`, good: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, base, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{acquireBody: tc.body})
			journal := &memoryJournal{}
			d := &Driver{Approval: a, Journal: journal, API: base}
			c := &journaledDrainClient{d: d, inner: session, sessionID: session.Session().SessionID.String(), hook: hook}
			hook.releaseResponse()
			message, err := c.GetMessage(context.Background(), 0, drainInitialCapacity)
			if err != nil || message == nil {
				t.Fatalf("setup poll = message %v err %v", message, err)
			}
			if err := c.DeleteMessage(context.Background(), message.MessageID); err != nil {
				t.Fatalf("setup ACK = %v", err)
			}
			_, err = c.AcquireJobs(context.Background(), []int64{41})
			if tc.good {
				if err != nil {
					t.Fatalf("valid acquisition = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrQuarantine) {
				t.Fatalf("ambiguous acquisition = %v, want quarantine", err)
			}
			if fixture.acquires.Load() != 1 {
				t.Fatalf("ambiguous acquisition request count = %d, want one remote attempt", fixture.acquires.Load())
			}
			if state := replay(journal.Events()); !state.uncertain {
				t.Fatal("ambiguous acquisition did not retain uncertainty")
			}
			for _, event := range journal.Events() {
				if event.Kind == "result" && event.Operation == "acquire" {
					t.Fatal("ambiguous acquisition recorded a successful result")
				}
			}
		})
	}
}

func TestPinnedSDKDrainSnapshotsRequireStrictWireFacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(string) string
	}{
		{name: "duplicate id", mutate: func(body string) string { return strings.Replace(body, `"id":7`, `"id":7,"id":7`, 1) }},
		{name: "casefold id", mutate: func(body string) string { return strings.Replace(body, `"id":7`, `"ID":99,"id":7`, 1) }},
		{name: "duplicate statistics", mutate: func(body string) string {
			return strings.Replace(body, `"totalIdleRunners":1`, `"totalIdleRunners":0,"totalIdleRunners":1`, 1)
		}},
		{name: "casefold statistics", mutate: func(body string) string {
			return strings.Replace(body, `"totalIdleRunners":1`, `"TotalIdleRunners":0,"totalIdleRunners":1`, 1)
		}},
		{name: "missing statistics", mutate: func(body string) string {
			return strings.Replace(body, `,"statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1}`, "", 1)
		}},
		{name: "null statistics", mutate: func(body string) string {
			return strings.Replace(body, `"statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1}`, `"statistics":null`, 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := approval()
			valid := pinnedDrainSnapshotJSON(a)
			fixture, base, _, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{snapshotBodies: []string{tc.mutate(valid)}})
			api := fixtureSDK{base}
			a.Phases = append(a.Phases, "drain")
			j := &memoryJournal{events: []Event{
				{Kind: "phase", Operation: "create"},
				{Kind: "intent", Operation: "create"},
				{Kind: "result", Operation: "create", ID: 7},
			}}
			d := Driver{Approval: a, Journal: j, API: api}
			if err := d.Run(context.Background(), "drain"); !errors.Is(err, ErrQuarantine) {
				t.Fatalf("ambiguous snapshot = %v, want quarantine", err)
			}
			if fixture.polls.Load() != 0 || fixture.acquires.Load() != 0 {
				t.Fatalf("ambiguous snapshot reached session effects: polls=%d acquire=%d", fixture.polls.Load(), fixture.acquires.Load())
			}
			_ = hook
		})
	}
}

func mustJSONQuote(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func pinnedDrainRunJSON(a Approval) string {
	data, _ := json.Marshal(pinnedDrainRun(a))
	return string(data)
}

func ambiguousRunJSON(a Approval, mode string) string {
	if mode == "missing-head" {
		run := pinnedDrainRun(a)
		delete(run, "head_sha")
		data, _ := json.Marshal(run)
		return string(data)
	}
	body := pinnedDrainRunJSON(a)
	switch mode {
	case "duplicate-head":
		return strings.Replace(body, `"head_sha":"`+a.WorkflowSHA+`"`, `"head_sha":"foreign","head_sha":"`+a.WorkflowSHA+`"`, 1)
	case "casefold-head":
		return strings.Replace(body, `"head_sha":"`+a.WorkflowSHA+`"`, `"HEAD_SHA":"foreign","head_sha":"`+a.WorkflowSHA+`"`, 1)
	case "wrong-head":
		return strings.Replace(body, a.WorkflowSHA, strings.Repeat("f", 40), 1)
	default:
		return body
	}
}

type pinnedDrainStatsWire struct {
	Available  int `json:"totalAvailableJobs"`
	Acquired   int `json:"totalAcquiredJobs"`
	Assigned   int `json:"totalAssignedJobs"`
	Running    int `json:"totalRunningJobs"`
	Registered int `json:"totalRegisteredRunners"`
	Busy       int `json:"totalBusyRunners"`
	Idle       int `json:"totalIdleRunners"`
}

type pinnedDrainSnapshotWire struct {
	ID            int                    `json:"id"`
	Name          string                 `json:"name"`
	RunnerGroupID int                    `json:"runnerGroupId"`
	Labels        []scaleset.Label       `json:"labels"`
	RunnerSetting scaleset.RunnerSetting `json:"RunnerSetting"`
	Statistics    pinnedDrainStatsWire   `json:"statistics"`
}

func pinnedDrainSnapshotJSON(a Approval) string {
	data, _ := json.Marshal(pinnedDrainSnapshotWire{
		ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID,
		Labels: []scaleset.Label{{Name: a.setName(), Type: "System"}}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		Statistics: pinnedDrainStatsWire{Registered: 1, Idle: 1},
	})
	return string(data)
}
