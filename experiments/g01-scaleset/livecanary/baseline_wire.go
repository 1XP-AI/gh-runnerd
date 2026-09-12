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
	mu                   sync.Mutex
	stage                string
	setID                int
	organization         string
	owner                string
	runnerName           string
	sessionID            string
	queue                string // private, captured from the exact session; never journaled
	runtimePathPrefix    string // private tenant path prefix; never journaled
	runtimePathPrefixSet bool
	allowedHosts         []string
	cursor               int
	count, status        int
	requestIDs           []int64
	requestCount         int
	origin               string
	invalid              bool
	session              *baselineSessionFacts
	batch                *baselineBatch
	accepted             *baselineAccepted
	set                  *baselineSetFacts
	runner               *baselineRunnerFacts
	jit                  *scaleset.RunnerScaleSetJitRunnerConfig
}

func (c *baselineWireCapture) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, baselineWireKey{}, c)
}

// baselinePathPrefix returns the path before an exact endpoint marker. The
// prefix is intentionally retained only in memory: it binds later marked
// requests to the first approved tenant/runtime route without putting private
// URLs in journal evidence.
func baselinePathPrefix(path, marker string) (string, bool) {
	if path == "" || marker == "" || !strings.HasSuffix(path, marker) {
		return "", false
	}
	prefix := strings.TrimSuffix(path, marker)
	if prefix != "" && (!strings.HasPrefix(prefix, "/") || strings.HasSuffix(prefix, "/")) {
		return "", false
	}
	if prefix == "" {
		prefix = "/"
	}
	return prefix, true
}

func baselineRuntimeScaleSetPrefix(path string, setID int) (string, bool) {
	return baselineRuntimeScaleSetPrefixForTail(path, setID, "")
}

func baselineRuntimeScaleSetPrefixForTail(path string, setID int, tail string) (string, bool) {
	if setID <= 0 {
		return "", false
	}
	return baselinePathPrefix(path, "/_apis/runtime/runnerscalesets/"+strconv.Itoa(setID)+tail)
}

func baselineRuntimeRunnerPrefix(path string) (string, bool) {
	return baselinePathPrefix(path, "/_apis/distributedtask/pools/0/agents")
}

func baselineRuntimeScaleSetPath(prefix string, setID int, tail string) string {
	if prefix == "/" {
		prefix = ""
	}
	return prefix + "/_apis/runtime/runnerscalesets/" + strconv.Itoa(setID) + tail
}

func baselineRuntimeRunnerPath(prefix string) string {
	if prefix == "/" {
		prefix = ""
	}
	return prefix + "/_apis/distributedtask/pools/0/agents"
}

func (c *baselineWireCapture) runtimePrefixMatches(prefix string) bool {
	return c == nil || !c.runtimePathPrefixSet || c.runtimePathPrefix == prefix
}

func (c *baselineWireCapture) runtimeRequestPrefix(r *http.Request) (string, bool) {
	if c == nil || r == nil || r.URL == nil {
		return "", false
	}
	var (
		prefix string
		ok     bool
	)
	if c.stage == "runner-observe" {
		prefix, ok = baselineRuntimeRunnerPrefix(r.URL.Path)
	} else {
		tail := ""
		switch c.stage {
		case "session-open":
			tail = "/sessions"
		case "acquire":
			tail = "/acquirejobs"
		case "terminal-session-close":
			if c.sessionID == "" {
				return "", false
			}
			tail = "/sessions/" + c.sessionID
		}
		prefix, ok = baselineRuntimeScaleSetPrefixForTail(r.URL.Path, c.setID, tail)
	}
	if !ok || !c.runtimePrefixMatches(prefix) {
		return "", false
	}
	return prefix, true
}

func (c *baselineWireCapture) target(r *http.Request) bool {
	if r == nil || r.URL == nil || r.URL.Fragment != "" || r.URL.User != nil || r.URL.EscapedPath() != r.URL.Path {
		return false
	}
	if c.stage == "poll" {
		u, e := url.Parse(c.queue)
		if e != nil {
			return false
		}
		if c.cursor > 0 {
			q := u.Query()
			q.Set("lastMessageId", strconv.Itoa(c.cursor))
			u.RawQuery = q.Encode()
		}
		return r.Method == "GET" && r.URL.String() == u.String()
	}
	if c.stage == "ack" {
		if !c.markedDeleteIdentityReady() || c.queue == "" || c.cursor <= 0 {
			return false
		}
		u, err := url.Parse(c.queue)
		if err != nil || u.Fragment != "" || u.User != nil || u.EscapedPath() != u.Path {
			return false
		}
		queueOrigin, ok := baselineRequestOrigin(u)
		if !ok || queueOrigin != c.origin {
			return false
		}
		if len(c.allowedHosts) > 0 && !baselineOriginAllowed(u, c.allowedHosts) {
			return false
		}
		u.Path += "/" + strconv.Itoa(c.cursor)
		return r.Method == http.MethodDelete && r.URL.String() == u.String()
	}
	suffix := "/runnerscalesets/" + strconv.Itoa(c.setID) + "/"
	if c.stage == "set-observe" || c.stage == "terminal-set" || c.stage == "terminal-set-recheck" || c.stage == "terminal-set-absence" || c.stage == "terminal-set-delete" {
		if !c.snapshotOriginAllowed(r) {
			return false
		}
		method := "GET"
		if c.stage == "terminal-set-delete" {
			method = "DELETE"
		}
		return r.Method == method && c.runtimeRequestTarget(r, "") && baselineExactAPIVersionQuery(r.URL.RawQuery)
	}
	if c.stage == "terminal-session-close" {
		origin, ok := baselineRequestOrigin(r.URL)
		return c.markedDeleteIdentityReady() && ok && origin == c.origin && (len(c.allowedHosts) == 0 || baselineOriginAllowed(r.URL, c.allowedHosts)) && r.Method == http.MethodDelete && c.runtimeRequestTarget(r, "sessions/"+c.sessionID) && baselineExactAPIVersionQuery(r.URL.RawQuery)
	}
	if c.stage == "session-open" {
		origin, ok := baselineRequestOrigin(r.URL)
		if !ok || (c.origin != "" && origin != c.origin) || (len(c.allowedHosts) > 0 && !baselineOriginAllowed(r.URL, c.allowedHosts)) {
			return false
		}
		return r.Method == "POST" && c.runtimeRequestTarget(r, "sessions") && baselineExactAPIVersionQuery(r.URL.RawQuery)
	} else if c.stage == "runner-observe" {
		return c.snapshotOriginAllowed(r) && r.Method == http.MethodGet && c.runtimeRequestTarget(r, "") && baselineExactQuery(r.URL.RawQuery, map[string]string{"agentName": c.runnerName, "api-version": "6.0-preview"})
	} else if c.stage == "acquire" {
		origin, ok := baselineRequestOrigin(r.URL)
		if c.origin == "" || !ok || origin != c.origin || len(c.allowedHosts) == 0 || !baselineOriginAllowed(r.URL, c.allowedHosts) {
			return false
		}
		return r.Method == "POST" && c.runtimeRequestTarget(r, "acquirejobs") && baselineExactAPIVersionQuery(r.URL.RawQuery)
	} else if c.stage == "jit" {
		suffix += "generatejitconfig"
	} else {
		return false
	}
	return r.Method == "POST" && strings.HasSuffix(r.URL.Path, suffix) && baselineExactAPIVersionQuery(r.URL.RawQuery)
}

// markedDeleteIdentityReady requires the private identity captured by the
// approved G01 session and snapshot sequence before a marked DELETE can be
// forwarded. The fields are never serialized; they bind the physical request
// to the one-shot operation that created the capture.
func (c *baselineWireCapture) markedDeleteIdentityReady() bool {
	return c != nil && c.setID > 0 && c.sessionID != "" && c.origin != "" && c.runtimePathPrefixSet && c.runtimePathPrefix != ""
}

func (c *baselineWireCapture) reserveOneShot(req *http.Request) error {
	c.mu.Lock()
	if c.requestCount != 0 || c.invalid {
		c.invalid = true
		c.mu.Unlock()
		if req != nil && req.Body != nil {
			_ = req.Body.Close()
		}
		return ErrRemote
	}
	c.requestCount = 1
	c.mu.Unlock()
	return nil
}

func (c *baselineWireCapture) runtimeRequestTarget(r *http.Request, tail string) bool {
	if c == nil || r == nil || r.URL == nil {
		return false
	}
	if c.stage == "runner-observe" {
		prefix, ok := baselineRuntimeRunnerPrefix(r.URL.Path)
		return ok && c.runtimePrefixMatches(prefix) && r.URL.Path == baselineRuntimeRunnerPath(prefix)
	}
	prefixTail := ""
	if tail != "" {
		prefixTail = "/" + tail
	}
	prefix, ok := baselineRuntimeScaleSetPrefixForTail(r.URL.Path, c.setID, prefixTail)
	if !ok || !c.runtimePrefixMatches(prefix) {
		return false
	}
	return r.URL.Path == baselineRuntimeScaleSetPath(prefix, c.setID, "/"+tail) || (tail == "" && r.URL.Path == baselineRuntimeScaleSetPath(prefix, c.setID, ""))
}

func (c *baselineWireCapture) snapshotOriginAllowed(r *http.Request) bool {
	if c == nil || r == nil || r.URL == nil || len(c.allowedHosts) == 0 || !baselineOriginAllowed(r.URL, c.allowedHosts) {
		return false
	}
	origin, ok := baselineRequestOrigin(r.URL)
	if !ok {
		return false
	}
	return c.origin == "" || c.origin == origin
}

func snapshotRequestCandidate(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	if r.Method == http.MethodGet {
		return true
	}
	path := r.URL.Path
	return strings.Contains(path, "/runnerscalesets/") || strings.HasSuffix(path, "/agents")
}

func baselineExactQuery(rawQuery string, expected map[string]string) bool {
	if rawQuery == "" || len(strings.Split(rawQuery, "&")) != len(expected) {
		return false
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil || len(values) != len(expected) {
		return false
	}
	for key, want := range expected {
		got, ok := values[key]
		if !ok || len(got) != 1 || got[0] != want {
			return false
		}
	}
	return true
}

func baselineExactAPIVersionQuery(rawQuery string) bool {
	return baselineExactQuery(rawQuery, map[string]string{"api-version": "6.0-preview"})
}

func (c *baselineWireCapture) sessionOpenBootstrapTarget(r *http.Request) bool {
	if c == nil || r == nil || r.URL == nil || r.URL.Fragment != "" || r.URL.User != nil || r.URL.EscapedPath() != r.URL.Path || r.Method != http.MethodPost || r.URL.RawQuery != "" || len(c.allowedHosts) == 0 || !baselineOriginAllowed(r.URL, c.allowedHosts) || c.organization == "" || !component.MatchString(c.organization) || c.organization == "." || c.organization == ".." {
		return false
	}
	paths := make([]string, 0, 10)
	for _, prefix := range []string{"", "/api/v3"} {
		paths = append(paths,
			prefix+"/orgs/"+c.organization+"/actions/runners/registration-token",
			prefix+"/orgs/"+c.organization+"/actions/runner-registration",
			prefix+"/"+c.organization+"/actions/runners/registration-token",
			prefix+"/"+c.organization+"/actions/runner-registration",
			prefix+"/actions/runner-registration",
		)
	}
	return slices.Contains(paths, r.URL.Path)
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
// process. Only explicitly marked G01 runtime requests are inspected.
type baselineRequestCaptureTransport struct{ inner http.RoundTripper }

// baselineRequestHostMatchesURL fences Request.Host, which overrides the
// physical HTTP Host header. The SDK normally leaves it empty; an explicitly
// supplied value is accepted only when it equals the URL's canonical host.
func baselineRequestHostMatchesURL(req *http.Request) bool {
	return req != nil && req.URL != nil && req.URL.Opaque == "" && (req.Host == "" || req.Host == req.URL.Host)
}

func (t baselineRequestCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c, _ := req.Context().Value(baselineWireKey{}).(*baselineWireCapture)
	if c != nil {
		if !baselineRequestHostMatchesURL(req) {
			return nil, c.rejectRequest(req)
		}
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
	if c.stage == "ack" {
		if !c.target(req) {
			return c.rejectRequest(req)
		}
		return c.reserveOneShot(req)
	}
	if c.stage == "terminal-session-close" {
		// A close DELETE is a marked terminal effect, not a snapshot read. It
		// must reject every mismatch, including a route that does not contain
		// the runnerscalesets family, before the inner transport is called.
		if !c.markedDeleteIdentityReady() || !c.target(req) {
			return c.rejectRequest(req)
		}
		return c.reserveOneShot(req)
	}
	if c.stage == "session-open" {
		if !c.target(req) {
			if c.sessionOpenBootstrapTarget(req) {
				return nil
			}
			return c.rejectRequest(req)
		}
		c.mu.Lock()
		if c.requestCount != 0 || c.invalid {
			c.mu.Unlock()
			return c.rejectRequest(req)
		}
		c.requestCount = 1
		c.mu.Unlock()
		data, err := readBaselineRequestBody(req.Body)
		req.Body = nil
		if err != nil || !validBaselineSessionOpenBody(data, c.owner) {
			clear(data)
			return c.rejectReservedRequest(req)
		}
		installBaselineRequestBody(req, data)
		origin, ok := baselineRequestOrigin(req.URL)
		if !ok {
			return c.rejectReservedRequest(req)
		}
		prefix, ok := c.runtimeRequestPrefix(req)
		if !ok {
			return c.rejectReservedRequest(req)
		}
		c.mu.Lock()
		if c.origin != "" && c.origin != origin {
			c.invalid = true
		}
		c.origin = origin
		if !c.runtimePathPrefixSet {
			c.runtimePathPrefix = prefix
			c.runtimePathPrefixSet = true
		} else if c.runtimePathPrefix != prefix {
			c.invalid = true
		}
		c.mu.Unlock()
		return nil
	}
	if c.stage == "set-observe" || c.stage == "runner-observe" || strings.HasPrefix(c.stage, "terminal-set") {
		if !snapshotRequestCandidate(req) {
			if c.sessionOpenBootstrapTarget(req) {
				return nil
			}
			return c.rejectRequest(req)
		}
		if !c.target(req) {
			return c.rejectRequest(req)
		}
		prefix, ok := c.runtimeRequestPrefix(req)
		if !ok {
			return c.rejectRequest(req)
		}
		origin, ok := baselineRequestOrigin(req.URL)
		if !ok {
			return c.rejectRequest(req)
		}
		c.mu.Lock()
		c.requestCount++
		if c.origin == "" {
			c.origin = origin
		} else if c.origin != origin {
			c.invalid = true
		}
		if !c.runtimePathPrefixSet {
			c.runtimePathPrefix = prefix
			c.runtimePathPrefixSet = true
		} else if c.runtimePathPrefix != prefix {
			c.invalid = true
		}
		valid := c.requestCount == 1 && !c.invalid
		c.mu.Unlock()
		if !valid {
			return c.rejectRequest(req)
		}
		return nil
	}
	if c.stage != "acquire" {
		return nil
	}
	if len(c.requestIDs) == 0 {
		return c.rejectRequest(req)
	}
	if !c.target(req) {
		return c.rejectRequest(req)
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
	installBaselineRequestBody(req, data)
	return nil
}

func validBaselineSessionOpenBody(data []byte, owner string) bool {
	if owner == "" {
		return false
	}
	var body struct {
		SessionID *string `json:"sessionId"`
		Owner     *string `json:"ownerName"`
	}
	return DecodeStrict(data, &body) == nil && body.SessionID != nil && *body.SessionID == "00000000-0000-0000-0000-000000000000" && body.Owner != nil && *body.Owner == owner
}

func installBaselineRequestBody(req *http.Request, data []byte) {
	if req == nil {
		clear(data)
		return
	}
	req.Body = &baselineRequestBody{reader: bytes.NewReader(data), data: data}
	req.ContentLength = int64(len(data))
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

func (c *baselineWireCapture) rejectReservedRequest(req *http.Request) error {
	c.mu.Lock()
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

func (c *baselineWireCapture) requestRuntimePathPrefix() (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runtimePathPrefix, c.runtimePathPrefixSet && !c.invalid
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
