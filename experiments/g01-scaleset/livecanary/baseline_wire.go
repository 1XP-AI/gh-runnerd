package livecanary

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/actions/scaleset"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

type baselineWireKey struct{}
type baselineWireCapture struct {
	mu            sync.Mutex
	stage         string
	setID         int
	sessionID     string
	queue         string // private, captured from the exact session; never journaled
	cursor        int
	count, status int
	invalid       bool
	session       *baselineSessionFacts
	batch         *baselineBatch
	accepted      *baselineAccepted
	set           *baselineSetFacts
	jit           *scaleset.RunnerScaleSetJitRunnerConfig
}

func (c *baselineWireCapture) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, baselineWireKey{}, c)
}
func (c *baselineWireCapture) target(r *http.Request) bool {
	if r.URL.Fragment != "" || r.URL.User != nil || r.URL.EscapedPath() != r.URL.Path {
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
		return c.sessionID != "" && r.Method == "DELETE" && strings.HasSuffix(r.URL.Path, suffix+"sessions/"+c.sessionID) && len(q) == 1 && len(q["api-version"]) == 1 && q.Get("api-version") == "6.0-preview"
	}
	if c.stage == "session-open" {
		suffix += "sessions"
	} else if c.stage == "acquire" {
		// The synthetic drain transport uses the queue URL for its bounded
		// acquisition request. The released SDK uses the Actions API endpoint
		// below; accepting this exact queue form keeps the strict adapter useful
		// for both without widening the target matcher.
		if c.queue != "" {
			u, e := url.Parse(c.queue)
			if e == nil && u.Scheme != "" && u.Host != "" && u.User == nil && u.Fragment == "" {
				u.Path = strings.TrimSuffix(u.Path, "/") + "/acquirejobs"
				if r.Method == "POST" && r.URL.String() == u.String() {
					return true
				}
			}
		}
		suffix += "acquirejobs"
	} else if c.stage == "jit" {
		suffix += "generatejitconfig"
	} else {
		return false
	}
	q := r.URL.Query()
	return r.Method == "POST" && strings.HasSuffix(r.URL.Path, suffix) && len(q) == 1 && len(q["api-version"]) == 1 && q.Get("api-version") == "6.0-preview"
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
