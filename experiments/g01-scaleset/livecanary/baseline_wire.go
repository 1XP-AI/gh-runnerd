package livecanary

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/actions/scaleset"
)

type baselineWireKey struct{}
type baselineWireCapture struct {
	mu            sync.Mutex
	stage         string
	setID         int
	runnerName    string
	sessionID     string
	queue         string // private, captured from the exact session; never journaled
	allowedHosts  []string
	cursor        int
	count, status int
	requestIDs    []int64
	requestCount  int
	origin        string
	invalid       bool
	session       *baselineSessionFacts
	batch         *baselineBatch
	accepted      *baselineAccepted
	set           *baselineSetFacts
	runner        *baselineRunnerFacts
	jit           *scaleset.RunnerScaleSetJitRunnerConfig
}

func (c *baselineWireCapture) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, baselineWireKey{}, c)
}
func (c *baselineWireCapture) target(r *http.Request) bool {
	if r == nil || r.URL == nil || r.URL.Fragment != "" || r.URL.User != nil || r.URL.EscapedPath() != r.URL.Path {
		return false
	}
	if c.stage == "poll" || c.stage == "ack" {
		u, e := url.Parse(c.queue)
		if e != nil {
			return false
		}
		if c.stage == "ack" {
			u.Path += "/" + strconv.Itoa(c.cursor)
			return r.Method == "DELETE" && r.URL.String() == u.String()
		}
		if c.cursor > 0 {
			q := u.Query()
			q.Set("lastMessageId", strconv.Itoa(c.cursor))
			u.RawQuery = q.Encode()
		}
		return r.Method == "GET" && r.URL.String() == u.String()
	}
	suffix := "/runnerscalesets/" + strconv.Itoa(c.setID) + "/"
	if c.stage == "set-observe" || c.stage == "terminal-set" || c.stage == "terminal-set-recheck" || c.stage == "terminal-set-absence" || c.stage == "terminal-set-delete" {
		q := r.URL.Query()
		method := "GET"
		if c.stage == "terminal-set-delete" {
			method = "DELETE"
		}
		return r.Method == method && strings.HasSuffix(r.URL.Path, strings.TrimSuffix(suffix, "/")) && len(q) == 1 && len(q["api-version"]) == 1 && q.Get("api-version") == "6.0-preview"
	}
	if c.stage == "terminal-session-close" {
		q := r.URL.Query()
		origin, ok := baselineRequestOrigin(r.URL)
		return c.origin != "" && ok && origin == c.origin && c.sessionID != "" && r.Method == "DELETE" && strings.HasSuffix(r.URL.Path, suffix+"sessions/"+c.sessionID) && len(q) == 1 && len(q["api-version"]) == 1 && q.Get("api-version") == "6.0-preview"
	}
	if c.stage == "session-open" {
		if len(c.allowedHosts) > 0 && !baselineOriginAllowed(r.URL, c.allowedHosts) {
			return false
		}
		suffix += "sessions"
	} else if c.stage == "runner-observe" {
		q := r.URL.Query()
		return r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/_apis/distributedtask/pools/0/agents") && len(q) == 2 && len(q["agentName"]) == 1 && q.Get("agentName") == c.runnerName && len(q["api-version"]) == 1 && q.Get("api-version") == "6.0-preview"
	} else if c.stage == "acquire" {
		if len(c.allowedHosts) == 0 || !slices.Contains(c.allowedHosts, r.URL.Host) {
			return false
		}
		suffix = "/_apis/runtime" + suffix + "acquirejobs"
	} else if c.stage == "jit" {
		suffix += "generatejitconfig"
	} else {
		return false
	}
	q := r.URL.Query()
	return r.Method == "POST" && strings.HasSuffix(r.URL.Path, suffix) && len(q) == 1 && len(q["api-version"]) == 1 && q.Get("api-version") == "6.0-preview"
}

func baselineAcquireRequestCandidate(r *http.Request) bool {
	return r != nil && r.URL != nil && (strings.Contains(r.URL.Path, "/_apis/runtime/runnerscalesets/") || strings.HasSuffix(r.URL.Path, "/acquirejobs"))
}

func baselineRequestOrigin(u *url.URL) (string, bool) {
	if u == nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", false
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	parsed, err := strconv.Atoi(port)
	if err != nil || parsed <= 0 || parsed > 65535 {
		return "", false
	}
	return "https://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port), true
}

func baselineOriginAllowed(u *url.URL, approvedHosts []string) bool {
	origin, ok := baselineRequestOrigin(u)
	if !ok {
		return false
	}
	for _, approved := range approvedHosts {
		host, port, valid := drainApprovedHostPort(approved)
		if !valid {
			continue
		}
		if origin == "https://"+net.JoinHostPort(strings.ToLower(host), port) {
			return true
		}
	}
	return false
}

func baselineWireAllowedHosts(a Approval, apiHost string) []string {
	hosts := slices.Clone(a.ActionsHosts)
	if apiHost != "" && !slices.Contains(hosts, apiHost) {
		hosts = append(hosts, apiHost)
	}
	return hosts
}

// baselineRequestCaptureTransport runs immediately above the physical
// transport. User-supplied test wrappers may mutate a request before it gets
// here, so this is the final request-side boundary before any bytes leave the
// process. Only the explicitly expected acquisition request is inspected.
type baselineRequestCaptureTransport struct{ inner http.RoundTripper }

func (t baselineRequestCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c, _ := req.Context().Value(baselineWireKey{}).(*baselineWireCapture)
	if c != nil {
		if err := c.captureRequest(req); err != nil {
			return nil, err
		}
	}
	return t.inner.RoundTrip(req)
}

func validBaselineRequestIDs(ids []int64) bool {
	if len(ids) == 0 || len(ids) > 4 {
		return false
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return false
		}
		if _, ok := seen[id]; ok {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func readBaselineRequestBody(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return nil, ErrRemote
	}
	data, readErr := io.ReadAll(io.LimitReader(body, responseBodyLimit+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || int64(len(data)) > responseBodyLimit {
		clear(data)
		return nil, ErrRemote
	}
	return data, nil
}

func (c *baselineWireCapture) captureRequest(req *http.Request) error {
	if c == nil {
		return nil
	}
	if c.stage == "session-open" {
		if !c.target(req) {
			return nil
		}
		origin, ok := baselineRequestOrigin(req.URL)
		if !ok {
			return c.rejectRequest(req)
		}
		c.mu.Lock()
		if c.origin != "" && c.origin != origin {
			c.invalid = true
		}
		c.origin = origin
		c.mu.Unlock()
		return nil
	}
	if c.stage != "acquire" || len(c.requestIDs) == 0 {
		return nil
	}
	if !c.target(req) {
		if baselineAcquireRequestCandidate(req) {
			return c.rejectRequest(req)
		}
		return nil
	}
	c.mu.Lock()
	c.requestCount++
	if c.requestCount != 1 {
		c.invalid = true
		c.mu.Unlock()
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return ErrRemote
	}
	expected := slices.Clone(c.requestIDs)
	c.mu.Unlock()

	data, err := readBaselineRequestBody(req.Body)
	if err != nil {
		c.mu.Lock()
		c.invalid = true
		c.mu.Unlock()
		return ErrRemote
	}
	var got []int64
	if DecodeStrict(data, &got) != nil || !validBaselineRequestIDs(got) || !validBaselineRequestIDs(expected) || !slices.Equal(got, expected) {
		clear(data)
		c.mu.Lock()
		c.invalid = true
		c.mu.Unlock()
		return ErrRemote
	}
	// Keep the one bounded copy only as the request stream the pinned SDK must
	// forward. The capture itself retains no body bytes or decoded payload.
	replacement := &baselineRequestBody{reader: bytes.NewReader(data), data: data}
	req.Body = replacement
	req.ContentLength = int64(len(data))
	return nil
}

func (c *baselineWireCapture) rejectRequest(req *http.Request) error {
	c.mu.Lock()
	c.requestCount++
	c.invalid = true
	c.mu.Unlock()
	if req != nil && req.Body != nil {
		_ = req.Body.Close()
	}
	return ErrRemote
}

type baselineRequestBody struct {
	mu     sync.Mutex
	reader *bytes.Reader
	data   []byte
}

func (b *baselineRequestBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reader.Read(p)
}
func (b *baselineRequestBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	clear(b.data)
	b.data = nil
	b.reader.Reset(nil)
	return nil
}

func (c *baselineWireCapture) requestOrigin() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.origin
}

func (c *baselineWireCapture) requestObserved() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requestCount == 1 && !c.invalid
}

// Run after the ordinary response budget has wrapped the body. Only this
// explicitly marked exact request is read ahead; the SDK receives identical
// bytes. Nothing is captured from bootstrap, unrelated or unmarked traffic.
func guardBaselineResponse(req *http.Request, response *http.Response) (*http.Response, error) {
	c, _ := req.Context().Value(baselineWireKey{}).(*baselineWireCapture)
	if c == nil || !c.target(req) {
		return response, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	c.status = response.StatusCode
	if c.count != 1 {
		_ = response.Body.Close()
		c.invalid = true
		return nil, ErrRemote
	}
	if response.StatusCode != http.StatusOK {
		return response, nil
	}
	data, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || int64(len(data)) > responseBodyLimit {
		c.invalid = true
		clear(data)
		return nil, ErrRemote
	}
	switch c.stage {
	case "set-observe", "terminal-set", "terminal-set-recheck", "terminal-set-absence":
		c.set, err = decodeBaselineSet(data)
	case "session-open":
		c.session, err = decodeBaselineSession(data)
	case "runner-observe":
		c.runner, err = decodeBaselineRunner(data)
	case "poll":
		c.batch, err = decodeBaselineBatch(data)
	case "jit":
		var w scaleset.RunnerScaleSetJitRunnerConfig
		err = DecodeStrict(data, &w)
		if err == nil && validJITSecret(w.EncodedJITConfig) {
			c.jit = &w
		} else {
			err = ErrRemote
		}
	case "acquire":
		var w struct {
			Count *int    `json:"count"`
			Value []int64 `json:"value"`
		}
		err = DecodeStrict(data, &w)
		if len(w.Value) > 4 {
			err = ErrRemote
		} else {
			c.accepted = &baselineAccepted{Count: w.Count, IDs: w.Value}
		}
	}
	if err != nil {
		c.invalid = true
		clear(data)
		return nil, ErrRemote
	}
	response.Body = &baselineCopiedBody{Reader: bytes.NewReader(data), data: data}
	return response, nil
}

type baselineCopiedBody struct {
	*bytes.Reader
	data []byte
}

func (b *baselineCopiedBody) Close() error { clear(b.data); b.data = nil; return nil }

func (c *baselineWireCapture) observed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count == 1 && !c.invalid
}
func (c *baselineWireCapture) facts() (*baselineSessionFacts, *baselineBatch, *baselineAccepted, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Called only after the synchronous SDK request returns. Later transport
	// use is forbidden; journal append performs a separate deep copy.
	return c.session, c.batch, c.accepted, c.status
}

func (c *baselineWireCapture) setFacts() (*baselineSetFacts, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.set, c.status
}

func (c *baselineWireCapture) runnerFacts() (*baselineRunnerFacts, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runner, c.status
}

func validJITSecret(secret string) bool {
	if len(secret) < 16 || len(secret) > 1<<20 {
		return false
	}
	data, err := base64.StdEncoding.Strict().DecodeString(secret)
	clear(data)
	return err == nil
}
func (c *baselineWireCapture) takeJIT() (*scaleset.RunnerScaleSetJitRunnerConfig, int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	jit := c.jit
	c.jit = nil
	return jit, c.status, c.count == 1 && !c.invalid
}
