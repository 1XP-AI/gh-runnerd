package livecanary

import (
	"bytes"
	"context"
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
	queue         string // private, captured from the exact session; never journaled
	cursor        int
	count, status int
	invalid       bool
	session       *baselineSessionFacts
	batch         *baselineBatch
	accepted      *baselineAccepted
	set           *baselineSetFacts
}

func (c *baselineWireCapture) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, baselineWireKey{}, c)
}
func (c *baselineWireCapture) target(r *http.Request) bool {
	if r.URL.Fragment != "" || r.URL.User != nil || r.URL.EscapedPath() != r.URL.Path {
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
	suffix := "/runnerscalesets/" + strconv.Itoa(c.setID) + "/"
	if c.stage == "set-observe" {
		q := r.URL.Query()
		return r.Method == "GET" && strings.HasSuffix(r.URL.Path, strings.TrimSuffix(suffix, "/")) && len(q) == 1 && len(q["api-version"]) == 1 && q.Get("api-version") == "6.0-preview"
	}
	if c.stage == "session-open" {
		suffix += "sessions"
	} else if c.stage == "acquire" {
		suffix += "acquirejobs"
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
	case "set-observe":
		c.set, err = decodeBaselineSet(data)
	case "session-open":
		c.session, err = decodeBaselineSession(data)
	case "poll":
		c.batch, err = decodeBaselineBatch(data)
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
