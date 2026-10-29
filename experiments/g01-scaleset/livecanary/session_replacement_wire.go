package livecanary

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/actions/scaleset"
)

type replacementRequestKey struct{}
type replacementPermit struct {
	mu                   sync.Mutex
	scope                *replacementScope
	wire                 *baselineWireCapture
	exact                map[string]string // complete method+URL -> complete authorization value
	used                 map[string]bool
	count                int
	nested               *baselineSetFacts
	bootstrap            string
	atRecord, generation int
	operation            string
}

func (p *replacementPermit) context(ctx context.Context) context.Context {
	if p.wire != nil {
		ctx = p.wire.context(ctx)
	}
	return context.WithValue(ctx, replacementRequestKey{}, p)
}

// This gate runs immediately above the physical transport. Every SDK retry,
// bootstrap request and unmarked operation must pass the same finite budget.
func (p *replacementPermit) before(req *http.Request) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.scope
	if s == nil || req == nil || req.URL == nil || s.check() != nil || !baselineRequestHostMatchesURL(req) || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Fragment != "" || req.URL.EscapedPath() != req.URL.Path {
		return ErrQuarantine
	}
	deadline, ok := req.Context().Deadline()
	if !ok || !deadline.After(time.Now()) || deadline.After(s.deadline) || deadline.After(time.Now().Add(operationTimeout)) {
		return ErrQuarantine
	}
	key := req.Method + " " + req.URL.String()
	if p.atRecord != s.records || s.records < 1 {
		return ErrQuarantine
	}
	step := replacementSteps[s.records-1]
	if p.exact != nil {
		if (p.operation == "preflight" && step.stage != "start") || (p.operation == "source" && (step.stage != "poll" || step.outcome != "intent" || step.generation != p.generation)) || (p.operation != "preflight" && p.operation != "source") {
			return ErrQuarantine
		}
		auth, ok := p.exact[key]
		if !ok || p.used[key] || !replacementExactAuth(req, auth) {
			return ErrQuarantine
		}
		p.used[key] = true
		return nil
	}
	c, _ := req.Context().Value(baselineWireKey{}).(*baselineWireCapture)
	if c == nil || c != p.wire {
		return ErrQuarantine
	}
	validStage := false
	if step.outcome == "intent" && step.generation == p.generation {
		switch c.stage {
		case "set-observe":
			validStage = step.stage == "set-before" || step.stage == "set-after"
		case "runner-observe":
			validStage = step.stage == "worker-before" || step.stage == "worker-after"
		case "session-open":
			validStage = step.stage == "open"
		case "poll":
			validStage = step.stage == "poll"
		case "terminal-session-close":
			validStage = step.stage == "close" && s.same(p.generation)
		}
	}
	if !validStage {
		return ErrQuarantine
	}
	// Only the first owned-set read may bootstrap the pinned client. Capture
	// both token responses privately; all subsequent requests bind their bytes.
	if s.management == "" {
		apiBase := s.api.baseURL
		if apiBase != "https://api.github.com" {
			apiBase += "/api/v3"
		}
		if c.stage != "set-observe" || s.records != 2 || req.Method != http.MethodPost || req.URL.RawQuery != "" {
			return ErrQuarantine
		}
		switch s.bootstrapStep {
		case 0:
			if req.URL.String() != apiBase+"/orgs/"+s.approval.Organization+"/actions/runners/registration-token" || !replacementExactAuth(req, "Bearer "+s.credentials.InstallationToken) {
				return ErrQuarantine
			}
			if req.Body != nil && req.Body != http.NoBody {
				return ErrQuarantine
			}
			p.bootstrap = "registration"
			s.bootstrapStep = 1
			return nil
		case 1:
			if s.registration == "" || req.URL.String() != apiBase+"/actions/runner-registration" || !replacementExactAuth(req, "RemoteAuth "+s.registration) {
				return ErrQuarantine
			}
			data, err := readBaselineRequestBody(req.Body)
			var body struct {
				URL   string `json:"url"`
				Event string `json:"runner_event"`
			}
			config := s.api.baseURL + "/" + s.approval.Organization
			if s.api.baseURL == "https://api.github.com" {
				config = "https://github.com/" + s.approval.Organization
			}
			if err != nil || DecodeStrict(data, &body) != nil || body.URL != config || body.Event != "register" {
				clear(data)
				return ErrQuarantine
			}
			installBaselineRequestBody(req, data)
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
	c.origin = s.origin
	c.runtimePathPrefix = s.tenant
	c.runtimePathPrefixSet = true
	expected := "Bearer " + s.management
	if c.stage == "poll" {
		expected = "Bearer " + c.authorization
	}
	if !c.target(req) || !replacementExactAuth(req, expected) {
		return ErrQuarantine
	}
	if c.stage == "poll" {
		capacity := drainHeaderValues(req.Header, scaleset.HeaderScaleSetMaxCapacity)
		if c.cursor != 0 || req.URL.Query().Has("lastMessageId") || len(capacity) != 1 || capacity[0] != "1" {
			return ErrQuarantine
		}
	}
	p.count = 1
	return nil
}

func replacementExactAuth(req *http.Request, want string) bool {
	values := drainHeaderValues(req.Header, "Authorization")
	return want != "" && len(values) == 1 && values[0] == want
}

func (p *replacementPermit) after(req *http.Request, resp *http.Response) (*http.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	kind := p.bootstrap
	p.bootstrap = ""
	open := p.wire != nil && p.wire.stage == "session-open" && p.wire.target(req)
	if kind == "" && !open {
		return resp, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, responseBodyLimit+1))
	closeErr := resp.Body.Close()
	if err != nil || closeErr != nil || int64(len(data)) > responseBodyLimit {
		clear(data)
		return nil, ErrRemote
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
	} else if kind == "connection" {
		var v struct {
			URL   *string `json:"url"`
			Token *string `json:"token"`
		}
		if resp.StatusCode != 200 || !uniqueKeys(json.NewDecoder(bytes.NewReader(data))) || json.Unmarshal(data, &v) != nil || v.URL == nil || v.Token == nil || !validDrainAuthorizationToken(*v.Token) {
			return fail()
		}
		u, e := url.Parse(*v.URL)
		if e != nil || u.Scheme != "https" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.EscapedPath() != u.Path || !strings.HasSuffix(u.Path, "/") || (u.Path != "/" && path.Clean(u.Path) != strings.TrimSuffix(u.Path, "/")) || !baselineOriginAllowed(u, s.hosts) {
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
		s.management = *v.Token
		parts := strings.Split(s.management, ".")
		var claims struct {
			Expires *int64 `json:"exp"`
		}
		if len(parts) != 3 {
			return fail()
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		defer clear(payload)
		if err != nil || !uniqueKeys(json.NewDecoder(bytes.NewReader(payload))) || json.Unmarshal(payload, &claims) != nil || claims.Expires == nil {
			return fail()
		}
		s.managementExpires = time.Unix(*claims.Expires, 0)
		if !s.managementExpires.After(time.Now()) {
			return fail()
		}
	} else if open && resp.StatusCode == 200 {
		var v struct {
			Set json.RawMessage `json:"runnerScaleSet"`
		}
		if json.Unmarshal(data, &v) != nil {
			return fail()
		}
		nested, e := decodeBaselineSet(v.Set)
		if e != nil || !s.setEligible(nested) {
			return fail()
		}
		p.nested = nested
	}
	resp.Body = &baselineCopiedBody{Reader: bytes.NewReader(data), data: data}
	return resp, nil
}
