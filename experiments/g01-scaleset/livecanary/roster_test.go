package livecanary

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
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
