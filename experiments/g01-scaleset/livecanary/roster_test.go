package livecanary

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRosterActualTLSCompleteObservation(t *testing.T) {
	for _, total := range []int{0, 102} {
		t.Run(strconv.Itoa(total), func(t *testing.T) {
			a := approval()
			c := credentials(a)
			var calls atomic.Int32
			server, client := inventoryFixtureTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				n := total
				first := 1
				if total > 100 {
					if page == 1 {
						n = 100
					} else {
						n = 2
						first = 101
					}
				}
				_, _ = w.Write([]byte(inventoryFixturePage(total, first, n)))
			}))
			api := &SDKAPI{rest: client, baseURL: server.URL, approval: a, credentials: c}
			wantDigest, err := api.Inventory(context.Background())
			if err != nil {
				t.Fatal("legacy control")
			}
			calls.Store(0)
			got, err := api.observeRoster(context.Background())
			wantPages := 1
			if total > 100 {
				wantPages = 2
			}
			if err != nil || got.Organization != a.Organization || got.Completeness != rosterComplete || got.Count == nil || *got.Count != total || got.Pages != wantPages || got.Digest != wantDigest || calls.Load() != int32(wantPages) {
				t.Fatal("complete real roster observation unavailable")
			}
		})
	}
}

type rosterClosingBody struct {
	io.ReadCloser
	closeHook func()
}

func (b *rosterClosingBody) Close() error {
	err := b.ReadCloser.Close()
	b.closeHook()
	return err
}

func TestRosterPreservesOnlyAcceptedPagePrefix(t *testing.T) {
	for _, mode := range []string{"malformed-second", "duplicate-second", "count-drift", "cancel-first", "cancel-second", "approval-mutation", "credential-mutation", "dependency-mutation"} {
		t.Run(mode, func(t *testing.T) {
			a := approval()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls atomic.Int32
			server, client := inventoryFixtureTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				calls.Add(1)
				body := inventoryFixturePage(101, 1, 100)
				if page == 2 {
					body = inventoryFixturePage(101, 101, 1)
					switch mode {
					case "malformed-second":
						body = `{"total_count":101,"runners":null}`
					case "duplicate-second":
						body = inventoryFixturePage(101, 1, 1)
					case "count-drift":
						body = inventoryFixturePage(102, 101, 1)
					}
				}
				w.Header().Set("Link", "forward-compatible inventory metadata")
				_, _ = w.Write([]byte(body))
			}))
			api := &SDKAPI{rest: client, baseURL: server.URL, approval: a, credentials: credentials(a)}
			inner := client.Transport
			client.Transport = observationRoundTripper(func(r *http.Request) (*http.Response, error) {
				res, err := inner.RoundTrip(r)
				if err == nil {
					res.Body = &rosterClosingBody{res.Body, func() {
						switch mode {
						case "cancel-first":
							cancel()
						case "cancel-second":
							if r.URL.Query().Get("page") == "2" {
								cancel()
							}
						case "approval-mutation":
							api.approval.ActionsHosts[0] = "changed.example.com"
						case "credential-mutation":
							api.credentials.InstallationToken = strings.Repeat("x", 32)
						case "dependency-mutation":
							api.baseURL += "/changed"
						}
					}}
				}
				return res, err
			})
			got, err := api.observeRoster(ctx)
			pages, requests := 0, 1
			if mode == "malformed-second" || mode == "duplicate-second" || mode == "count-drift" || mode == "cancel-second" {
				pages, requests = 1, 2
			}
			if err == nil || got.Completeness != rosterUnresolved || got.Pages != pages || got.Digest != "" || got.Count != nil || int(calls.Load()) != requests {
				t.Fatalf("unresolved prefix mismatch: pages=%d requests=%d", got.Pages, calls.Load())
			}
		})
	}
}

func TestRosterFinalPublicationGuardAfterDigest(t *testing.T) {
	a := approval()
	server, client := inventoryFixtureTLS(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(inventoryFixturePage(0, 1, 0))) }))
	api := &SDKAPI{rest: client, baseURL: server.URL, approval: a, credentials: credentials(a)}
	calls := 0
	digest, _, pages, err := api.enumerateRoster(context.Background(), func() error {
		calls++
		if calls == 3 {
			return ErrApproval
		}
		return nil
	})
	if err == nil || digest != "" || pages != 1 || calls != 3 {
		t.Fatal("final guard did not fence completed digest")
	}
}

func TestRosterRefusesInvalidEntryWithoutNetwork(t *testing.T) {
	for _, mode := range []string{"nil", "context", "canceled", "authority", "credentials", "transport"} {
		t.Run(mode, func(t *testing.T) {
			a := approval()
			var calls atomic.Int32
			server, client := inventoryFixtureTLS(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			api := &SDKAPI{rest: client, baseURL: server.URL, approval: a, credentials: credentials(a)}
			ctx := context.Background()
			switch mode {
			case "nil":
				api = nil
			case "context":
				ctx = nil
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "authority":
				api.approval.ExpiresAt = time.Now().Add(-time.Second)
			case "credentials":
				api.credentials.AppID++
			case "transport":
				api.rest = nil
			}
			got, err := api.observeRoster(ctx)
			if err == nil || got.Count != nil || got.Digest != "" || got.Pages != 0 || got.Completeness != rosterUnresolved || calls.Load() != 0 {
				t.Fatal("invalid roster entry did work")
			}
		})
	}
}
