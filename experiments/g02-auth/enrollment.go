// Package enrollment is a disposable G02 evidence harness, not the product auth API.
package enrollment

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const callbackPath = "/manifest/callback"

// Attempt has exactly one conversion opportunity. Errors after consumption require
// manual reconciliation of the existing App, never an automatic registration retry.
// State is intentionally process-local: a restart invalidates the old attempt.
type Attempt struct {
	host, state string
	expires     time.Time
	now         func() time.Time
	convert     func(context.Context, string) error
	onSuccess   http.HandlerFunc // optional driver continuation; set before serving
	onReject    http.HandlerFunc // driver removes callback query on rejection too
	mu          sync.Mutex
	consumed    bool
}

// NewAttempt requires the actual address of an already-bound 127.0.0.1:0 listener.
// IPv6, localhost aliases, proxies and persistent setup servers are outside this probe.
func NewAttempt(host string, now func() time.Time, convert func(context.Context, string) error) (*Attempt, error) {
	ip, port, err := net.SplitHostPort(host)
	n, e := strconv.Atoi(port)
	if err != nil || e != nil || ip != "127.0.0.1" || n < 1 || n > 65535 || strconv.Itoa(n) != port || now == nil || convert == nil {
		return nil, errors.New("invalid callback configuration")
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, errors.New("state generation failed")
	}
	return &Attempt{host: host, state: base64.RawURLEncoding.EncodeToString(nonce[:]), expires: now().Add(10 * time.Minute), now: now, convert: convert}, nil
}

// State is for a browser form only; callers must never log or persist it.
func (a *Attempt) State() string { return a.state }

func (a *Attempt) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	reject := func(status int, message string) {
		if a.onReject != nil {
			a.onReject(w, r)
			return
		}
		http.Error(w, message, status)
	}
	if r.Method != http.MethodGet || r.Host != a.host || r.URL.Path != callbackPath || r.URL.EscapedPath() != callbackPath || r.URL.Fragment != "" || len(r.URL.RawQuery) > 2048 {
		reject(http.StatusBadRequest, "invalid callback")
		return
	}
	// A top-level GitHub redirect normally has no Origin. If supplied, accept only
	// GitHub's exact origin; this is defense in depth, not a replacement for state.
	origins := r.Header.Values("Origin")
	if len(origins) > 1 || (len(origins) == 1 && origins[0] != "https://github.com") {
		reject(http.StatusBadRequest, "invalid callback")
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(q) != 2 || len(q["state"]) != 1 || len(q["code"]) != 1 || !validCode(q.Get("code")) {
		reject(http.StatusBadRequest, "invalid callback")
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(a.state)) != 1 {
		reject(http.StatusForbidden, "invalid state")
		return
	}
	a.mu.Lock()
	if a.consumed {
		a.mu.Unlock()
		reject(http.StatusConflict, "attempt already consumed; inspect existing App and use manual import")
		return
	}
	if !a.now().Before(a.expires) {
		a.mu.Unlock()
		reject(http.StatusGone, "attempt expired; inspect existing App and use manual import")
		return
	}
	a.consumed = true
	a.mu.Unlock()
	// Mark consumed before the network call, including transport ambiguity or
	// storage failure. Raw provider errors can contain the code, key or URL.
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := a.convert(ctx, q.Get("code")); err != nil {
		reject(http.StatusBadGateway, "conversion failed; inspect existing App and use manual import")
		return
	}
	if a.onSuccess != nil {
		a.onSuccess(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("conversion accepted; continue organization verification\n"))
}

func validCode(code string) bool {
	if len(code) < 1 || len(code) > 512 {
		return false
	}
	for _, c := range code {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
