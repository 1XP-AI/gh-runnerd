package enrollment

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallbackRejectsUntrustedRequests(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*http.Request)
		advance time.Duration
	}{
		{name: "wrong state", mutate: func(r *http.Request) { q := r.URL.Query(); q.Set("state", "wrong"); r.URL.RawQuery = q.Encode() }},
		{name: "expired at deadline", advance: 10 * time.Minute},
		{name: "duplicate state", mutate: func(r *http.Request) { r.URL.RawQuery += "&state=extra" }},
		{name: "duplicate code", mutate: func(r *http.Request) { r.URL.RawQuery += "&code=extra" }},
		{name: "wrong host", mutate: func(r *http.Request) { r.Host = "attacker.example:43111" }},
		{name: "loopback alias", mutate: func(r *http.Request) { r.Host = "localhost:43111" }},
		{name: "wrong path", mutate: func(r *http.Request) { r.URL.Path = "/other" }},
		{name: "encoded path", mutate: func(r *http.Request) { r.URL.RawPath = "/manifest/%63allback" }},
		{name: "post", mutate: func(r *http.Request) { r.Method = "POST" }},
		{name: "foreign origin", mutate: func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") }},
		{name: "unknown parameter", mutate: func(r *http.Request) { r.URL.RawQuery += "&installation_id=999" }},
		{name: "invalid query", mutate: func(r *http.Request) { r.URL.RawQuery += "&bad=%zz" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
			calls := 0
			a, err := NewAttempt("127.0.0.1:43111", func() time.Time { return now }, func(context.Context, string) error { calls++; return nil })
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", "http://127.0.0.1:43111/manifest/callback?state="+url.QueryEscape(a.State())+"&code=synthetic-code", nil)
			if tc.mutate != nil {
				tc.mutate(req)
			}
			now = now.Add(tc.advance)
			w := httptest.NewRecorder()
			a.ServeHTTP(w, req)
			if w.Code < 400 || calls != 0 {
				t.Fatalf("untrusted callback accepted: status=%d calls=%d", w.Code, calls)
			}
			if strings.Contains(w.Body.String(), a.State()) || strings.Contains(w.Body.String(), "synthetic-code") {
				t.Fatal("callback secret echoed")
			}
		})
	}
}

func TestCallbackIsConsumedBeforeConversionIncludingFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "ambiguous conversion failure"}[fail], func(t *testing.T) {
			var calls atomic.Int32
			a, err := NewAttempt("127.0.0.1:43111", time.Now, func(context.Context, string) error {
				calls.Add(1)
				if fail {
					return errors.New("synthetic-secret-upstream-body")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			target := "http://127.0.0.1:43111/manifest/callback?state=" + url.QueryEscape(a.State()) + "&code=synthetic-code"
			var wg sync.WaitGroup
			for range 12 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					w := httptest.NewRecorder()
					a.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
					if strings.Contains(w.Body.String(), "synthetic-secret") {
						t.Error("upstream error escaped")
					}
				}()
			}
			wg.Wait()
			if calls.Load() != 1 {
				t.Fatalf("conversion called %d times, want 1", calls.Load())
			}
			w := httptest.NewRecorder()
			a.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
			if w.Code != http.StatusConflict {
				t.Fatalf("replay status=%d", w.Code)
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Fatal("sensitive redirect caching/referrer not disabled")
			}
		})
	}
}

func TestAttemptHostMustBeExplicitIPv4LoopbackWithPort(t *testing.T) {
	for _, host := range []string{"0.0.0.0:5000", "localhost:5000", "127.0.0.1", "127.0.0.1:0", "127.0.0.1:65536", "127.0.0.2:5000"} {
		if _, err := NewAttempt(host, time.Now, func(context.Context, string) error { return nil }); err == nil {
			t.Errorf("accepted invalid listener host %q", host)
		}
	}
}

func TestLoopbackListenerUsesItsActualRandomPort(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	attempt, err := NewAttempt(listener.Addr().String(), time.Now, func(context.Context, string) error { calls.Add(1); return nil })
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	server := &http.Server{Handler: attempt, ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Close(); <-done })
	target := "http://" + listener.Addr().String() + "/manifest/callback?state=" + url.QueryEscape(attempt.State()) + "&code=synthetic-code"
	client := &http.Client{Timeout: time.Second}
	result, err := client.Get(target)
	if err != nil {
		t.Fatal("local callback request failed")
	}
	result.Body.Close()
	if result.StatusCode != 200 || calls.Load() != 1 {
		t.Fatal("loopback callback failed")
	}
	request, err := http.NewRequest("GET", target, nil)
	if err != nil {
		t.Fatal("local request setup failed")
	}
	request.Host = "attacker.example"
	result, err = client.Do(request)
	if err != nil {
		t.Fatal("local callback request failed")
	}
	result.Body.Close()
	if result.StatusCode != 400 || calls.Load() != 1 {
		t.Fatal("wrong Host crossed local listener")
	}
}

func TestInvalidStateDoesNotConsumeValidAttempt(t *testing.T) {
	calls := 0
	attempt, err := NewAttempt("127.0.0.1:43111", time.Now, func(context.Context, string) error { calls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"wrong", attempt.State()} {
		request := httptest.NewRequest("GET", "http://127.0.0.1:43111/manifest/callback?code=synthetic-code&state="+url.QueryEscape(state), nil)
		response := httptest.NewRecorder()
		attempt.ServeHTTP(response, request)
		if state == attempt.State() && response.Code != 200 {
			t.Fatal("invalid attempt burned valid state")
		}
	}
	if calls != 1 {
		t.Fatal("incorrect conversion count")
	}
}
