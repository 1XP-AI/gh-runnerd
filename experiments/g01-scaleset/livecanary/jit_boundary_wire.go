package livecanary

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/actions/scaleset"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

type jitBoundaryRequestKey struct{}
type jitBoundaryPermit struct {
	mu              sync.Mutex
	scope           *jitBoundaryScope
	wire            *baselineWireCapture
	exact           map[string]string
	used            map[string]bool
	count, atRecord int
	bootstrap       string
	expected        bool
}

func (p *jitBoundaryPermit) context(ctx context.Context) context.Context {
	ctx = scaleset.WithRequestRetry(ctx, func(c *scaleset.RetryConfig) { c.Max = 0 })
	if p.wire != nil {
		ctx = p.wire.context(ctx)
	}
	return context.WithValue(ctx, jitBoundaryRequestKey{}, p)
}

type jitBoundaryTransport struct {
	scope    *jitBoundaryScope
	physical *http.Transport
}

func (t jitBoundaryTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	s := t.scope
	defer func() {
		if err != nil && !errors.Is(err, errJITNotAttempted) && !errors.Is(err, errJITSuppressed) {
			s.invalid.Store(true)
		}
		if err != nil && req != nil && req.Body != nil {
			_ = req.Body.Close()
		}
	}()
	if req == nil {
		return nil, ErrQuarantine
	}
	p, _ := req.Context().Value(jitBoundaryRequestKey{}).(*jitBoundaryPermit)
	if p == nil || p.scope != s || t.physical != s.network.physical {
		return nil, ErrQuarantine
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err = p.validate(req); err != nil {
		return nil, err
	}
	// Body validation may block. It must finish before taking journal.mu.
	j := s.journal
	j.mu.Lock()
	if err = s.checkLocked(); err != nil {
		j.mu.Unlock()
		return nil, err
	}
	jit := p.wire != nil && p.wire.stage == "jit"
	if jit {
		if err = s.recordLocked(func(r *jitBoundaryRecord) { v := true; r.RequestValidated = &v }); err != nil {
			j.mu.Unlock()
			return nil, err
		}
		if s.phase == beforeJITPhase {
			err = s.recordLocked(func(r *jitBoundaryRecord) {
				yes, zero := true, 0
				r.RequestValidated = &yes
				r.Forwarded = "none"
				r.ObservedForwardCount = &zero
			})
			if err != nil {
				j.mu.Unlock()
				return nil, err
			}
		} else if err = s.recordLocked(func(r *jitBoundaryRecord) { r.Forwarded = "possible" }); err != nil {
			j.mu.Unlock()
			return nil, err
		}
		// This is the sole pre-send stop. The fixture permits a physical POST
		// if it is omitted, so the no-forward assertion has a causal refail.
		if s.phase == beforeJITPhase {
			if req.Body != nil {
				if e := req.Body.Close(); e != nil {
					err = ErrRemote
				}
			}
			j.mu.Unlock()
			if err != nil {
				return nil, err
			}
			p.expected = true
			return nil, errJITNotAttempted
		}
	}
	// No Append can enter between the final post-sync check and physical entry.
	if err = s.checkLocked(); err != nil || req.Context().Err() != nil {
		j.mu.Unlock()
		return nil, ErrQuarantine
	}
	resp, err = t.physical.RoundTrip(req)
	j.mu.Unlock()
	if err != nil {
		return nil, ErrRemote
	}
	if resp == nil || resp.Body == nil {
		return nil, ErrRemote
	}
	if s.check() != nil {
		_ = resp.Body.Close()
		return nil, ErrQuarantine
	}
	if jit {
		return p.suppress(resp)
	}
	if p.bootstrap != "" {
		return p.bootstrapResponse(resp)
	}
	return resp, nil
}
func jitBoundaryHeaders(req *http.Request) bool {
	seen := map[string]bool{}
	for key, values := range req.Header {
		fold := strings.ToLower(key)
		if seen[fold] {
			return false
		}
		seen[fold] = true
		switch strings.ToLower(key) {
		case "authorization", "accept", "user-agent", "content-type", "x-github-api-version", strings.ToLower(baselineWireMarkerHeader):
			if len(values) != 1 {
				return false
			}
		default:
			return false
		}
	}
	return len(req.TransferEncoding) == 0 && !req.Close && !req.URL.ForceQuery
}
func jitBoundaryNoBody(req *http.Request) bool {
	if (req.Body == nil || req.Body == http.NoBody) && req.ContentLength == 0 {
		req.GetBody = nil
		return true
	}
	return false
}
func jitBoundaryBody(req *http.Request, validate func([]byte) bool) error {
	if req.Header.Get("Content-Type") != "application/json" || req.ContentLength <= 0 {
		return ErrRemote
	}
	n := req.ContentLength
	data, err := readBaselineRequestBody(req.Body)
	req.Body = nil
	if err != nil || int64(len(data)) != n || !validate(data) {
		clear(data)
		return ErrRemote
	}
	installBaselineRequestBody(req, data)
	req.GetBody = nil
	return nil
}
func (p *jitBoundaryPermit) validate(req *http.Request) error {
	s := p.scope
	if req.URL == nil || s.check() != nil || !baselineRequestHostMatchesURL(req) || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Fragment != "" || req.URL.EscapedPath() != req.URL.Path || !jitBoundaryHeaders(req) {
		return ErrQuarantine
	}
	deadline, ok := req.Context().Deadline()
	if !ok || !deadline.After(time.Now()) || deadline.After(s.deadline) || deadline.After(time.Now().Add(operationTimeout)) || p.atRecord != s.records || s.records < 1 {
		return ErrQuarantine
	}
	step := jitBoundarySteps(s.phase)[s.records-1]
	if p.exact != nil {
		key := req.Method + " " + req.URL.String()
		auth, ok := p.exact[key]
		if step.stage != "preflight" || step.outcome != "intent" || !ok || p.used[key] || !replacementExactAuth(req, auth) || !jitBoundaryNoBody(req) || len(req.Header) != 3 || req.Header.Get("Accept") != "application/vnd.github+json" || req.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			return ErrQuarantine
		}
		p.used[key] = true
		return nil
	}
	c, _ := req.Context().Value(baselineWireKey{}).(*baselineWireCapture)
	if c == nil || c != p.wire || step.outcome != "intent" || !(c.stage == "jit" && step.stage == "jit" || c.stage == "set-observe" && step.stage == "set-before" || c.stage == "runner-observe" && (step.stage == "worker-before" || step.stage == "worker-after")) {
		return ErrQuarantine
	}
	marker, keys := baselineWireMarkerValues(req)
	if keys != 1 || len(marker) != 1 || marker[0] != baselineWireMarkerValue {
		return ErrQuarantine
	}
	removeBaselineWireMarker(req)
	if len(req.Header) != 3 || req.Header.Get("User-Agent") == "" {
		return ErrQuarantine
	}
	if s.management == "" {
		base := s.api.baseURL
		if base != "https://api.github.com" {
			base += "/api/v3"
		}
		if c.stage != "set-observe" || s.records != 4 || req.Method != "POST" || req.URL.RawQuery != "" {
			return ErrQuarantine
		}
		switch s.bootstrapStep {
		case 0:
			if req.URL.String() != base+"/orgs/"+s.approval.Organization+"/actions/runners/registration-token" || !replacementExactAuth(req, "Bearer "+s.credentials.InstallationToken) || !jitBoundaryNoBody(req) || req.Header.Get("Content-Type") != "application/vnd.github.v3+json" {
				return ErrQuarantine
			}
			p.bootstrap = "registration"
			s.bootstrapStep = 1
			return nil
		case 1:
			if s.registration == "" || req.URL.String() != base+"/actions/runner-registration" || !replacementExactAuth(req, "RemoteAuth "+s.registration) {
				return ErrQuarantine
			}
			config := s.api.baseURL + "/" + s.approval.Organization
			if s.api.baseURL == "https://api.github.com" {
				config = "https://github.com/" + s.approval.Organization
			}
			if jitBoundaryBody(req, func(data []byte) bool {
				var v struct {
					URL   string `json:"url"`
					Event string `json:"runner_event"`
				}
				if DecodeStrict(data, &v) != nil || v.URL != config || v.Event != "register" {
					return false
				}
				canonical, _ := json.Marshal(v)
				return jitBoundaryMemberShape(data, canonical)
			}) != nil {
				return ErrQuarantine
			}
			p.bootstrap = "connection"
			s.bootstrapStep = 2
			return nil
		default:
			return ErrQuarantine
		}
	}
	if p.count != 0 {
		return ErrQuarantine
	}
	if req.Header.Get("Content-Type") != "application/json" {
		return ErrQuarantine
	}
	c.origin = s.origin
	c.runtimePathPrefix = s.tenant
	c.runtimePathPrefixSet = true
	if !c.target(req) || !replacementExactAuth(req, "Bearer "+s.management) {
		return ErrQuarantine
	}
	if c.stage == "jit" {
		if jitBoundaryBody(req, func(data []byte) bool {
			if !validBaselineJITBody(data, s.approval.workerName()) {
				return false
			}
			canonical, _ := json.Marshal(map[string]string{"name": s.approval.workerName(), "workFolder": "_work"})
			return jitBoundaryMemberShape(data, canonical)
		}) != nil {
			return ErrQuarantine
		}
	} else if !jitBoundaryNoBody(req) {
		return ErrQuarantine
	}
	// Existing capture validates the actual SDK tuple once; it never sees JIT's
	// response, which is intercepted inside this adapter before generic capture.
	if err := c.captureRequest(req); err != nil {
		return err
	}
	req.GetBody = nil
	p.count = 1
	return nil
}
func jitBoundaryResponseBytes(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, ErrRemote
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, responseBodyLimit+1))
	closeErr := resp.Body.Close()
	if err != nil || closeErr != nil || int64(len(data)) > responseBodyLimit {
		clear(data)
		return nil, ErrRemote
	}
	return data, nil
}
func (p *jitBoundaryPermit) suppress(resp *http.Response) (*http.Response, error) {
	data, err := jitBoundaryResponseBytes(resp)
	defer clear(data)
	if err != nil || resp.StatusCode != 200 || p.scope.check() != nil {
		return nil, ErrRemote
	}
	var v struct {
		Runner struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			SetID int    `json:"runnerScaleSetId"`
		} `json:"runner"`
		Config string `json:"encodedJITConfig"`
	}
	if DecodeStrict(data, &v) != nil || v.Runner.ID <= 0 || v.Runner.Name != p.scope.approval.workerName() || v.Runner.SetID != p.scope.setID || !validJITSecret(v.Config) {
		return nil, ErrRemote
	}
	canonical, _ := json.Marshal(v)
	shape := jitBoundaryMemberShape(data, canonical)
	clear(canonical)
	// Discard both fields, including the response identity. Only the independent
	// subsequent name GET can supply the persisted lookup ID. No wipe guarantee.
	v.Config = ""
	v.Runner.ID = 0
	v.Runner.Name = ""
	v.Runner.SetID = 0
	if !shape {
		return nil, ErrRemote
	}
	if err = p.scope.record(func(r *jitBoundaryRecord) {
		status, one, yes := 200, 1, true
		r.Status = &status
		r.Forwarded = "observed-one"
		r.ObservedForwardCount = &one
		r.Suppressed = &yes
	}); err != nil {
		return nil, err
	}
	p.expected = true
	return nil, errJITSuppressed
}
func (p *jitBoundaryPermit) bootstrapResponse(resp *http.Response) (*http.Response, error) {
	kind := p.bootstrap
	p.bootstrap = ""
	data, err := jitBoundaryResponseBytes(resp)
	if err != nil {
		return nil, err
	}
	fail := func() (*http.Response, error) { clear(data); return nil, ErrRemote }
	s := p.scope
	if kind == "registration" {
		var v struct {
			Token *string `json:"token"`
		}
		if resp.StatusCode != 201 || !uniqueKeys(json.NewDecoder(bytes.NewReader(data))) || json.Unmarshal(data, &v) != nil || v.Token == nil || !validDrainAuthorizationToken(*v.Token) {
			return fail()
		}
		s.registration = *v.Token
	} else {
		var v struct {
			URL   *string `json:"url"`
			Token *string `json:"token"`
		}
		if kind != "connection" || resp.StatusCode != 200 || !uniqueKeys(json.NewDecoder(bytes.NewReader(data))) || json.Unmarshal(data, &v) != nil || v.URL == nil || v.Token == nil || !validDrainAuthorizationToken(*v.Token) {
			return fail()
		}
		u, e := url.Parse(*v.URL)
		if e != nil || u.Scheme != "https" || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.User != nil || u.Fragment != "" || u.EscapedPath() != u.Path || !strings.HasSuffix(u.Path, "/") || (u.Path != "/" && path.Clean(u.Path) != strings.TrimSuffix(u.Path, "/")) || !baselineOriginAllowed(u, s.hosts) {
			return fail()
		}
		origin, ok := baselineRequestOrigin(u)
		if !ok {
			return fail()
		}
		s.origin = origin
		s.tenant = strings.TrimSuffix(u.Path, "/")
		if s.tenant == "" {
			s.tenant = "/"
		}
		parts := strings.Split(*v.Token, ".")
		if len(parts) != 3 {
			return fail()
		}
		payload, e := base64.RawURLEncoding.DecodeString(parts[1])
		defer clear(payload)
		var claims struct {
			Expires *int64 `json:"exp"`
		}
		if e != nil || !uniqueKeys(json.NewDecoder(bytes.NewReader(payload))) || json.Unmarshal(payload, &claims) != nil || claims.Expires == nil {
			return fail()
		}
		s.managementExpires = time.Unix(*claims.Expires, 0)
		if !s.managementExpires.After(time.Now().Add(time.Minute)) {
			return fail()
		}
		s.management = *v.Token
	}
	if s.check() != nil {
		return fail()
	}
	resp.Body = &baselineCopiedBody{Reader: bytes.NewReader(data), data: data}
	return resp, nil
}
