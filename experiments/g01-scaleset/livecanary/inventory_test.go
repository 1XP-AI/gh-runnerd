package livecanary

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/hashicorp/go-retryablehttp"
)

// This fixture uses only a private TLS listener. No configurable external endpoint
// or credential source is reachable by its transport.
func inventoryFixtureTLS(t *testing.T, handler http.Handler) (*httptest.Server, *http.Client) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != server.Listener.Addr().String() {
			return nil, errors.New("non-fixture address denied")
		}
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
	}
	client := &http.Client{Transport: withResponseBudget(transport), Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	t.Cleanup(func() { transport.CloseIdleConnections(); server.Close() })
	return server, client
}

func inventoryFixturePage(total int, first int, n int) string {
	runners := make([]map[string]int, 0, n)
	for i := 0; i < n; i++ {
		runners = append(runners, map[string]int{"id": first + i})
	}
	raw, _ := json.Marshal(map[string]any{"total_count": total, "runners": runners})
	return string(raw)
}

func TestInventoryStrictPages(t *testing.T) {
	maximumPages := make([]string, 10)
	exhaustedPages := make([]string, 10)
	for page := range maximumPages {
		maximumPages[page] = inventoryFixturePage(1000, page*100+1, 100)
		exhaustedPages[page] = inventoryFixturePage(11, page+1, 1)
	}
	cases := []struct {
		name  string
		pages []string
		valid bool
	}{
		{"valid_empty", []string{`{"total_count":0,"runners":[]}`}, true},
		{"valid_nonempty", []string{`{"total_count":2,"runners":[{"id":43},{"id":42}]}`}, true},
		{"valid_pagination", []string{inventoryFixturePage(102, 1, 100), inventoryFixturePage(102, 101, 2)}, true},
		{"valid_lone_alias", []string{`{"TOTAL_COUNT":0,"RUNNERS":[]}`}, true},
		{"valid_maximum", maximumPages, true},
		{"valid_large_id", []string{`{"total_count":1,"runners":[{"id":9223372036854775807}]}`}, true},
		{"valid_metadata", []string{`{"total_count":1,"runners":[{"id":42,"name":"fixture","future":{"large":1e999,"labels":[null,true,{}]}}],"future":null}`}, true},
		{"valid_unicode_alias", []string{`{"TOTAL_COUNT":1,"runnerſ":[{"ID":42}]}`}, true},
		{"page_over_100", []string{inventoryFixturePage(101, 1, 101)}, false},
		{"total_over_1000", []string{inventoryFixturePage(1001, 1, 1001)}, false},
		{"duplicate_across_pages", []string{inventoryFixturePage(101, 1, 100), inventoryFixturePage(101, 100, 1)}, false},
		{"empty_unfinished", []string{inventoryFixturePage(1, 1, 0)}, false},
		{"excess_records", []string{inventoryFixturePage(1, 1, 2)}, false},
		{"exhausted_pages", exhaustedPages, false},
		{"metadata_collision", []string{`{"total_count":1,"runners":[{"id":42,"future":{"k":1,"K":2}}]}`}, false},
		{"outer_array", []string{`[]`}, false},
		{"null_runner", []string{`{"total_count":1,"runners":[null]}`}, false},
		{"scalar_runner", []string{`{"total_count":1,"runners":[42]}`}, false},
		{"string_id", []string{`{"total_count":1,"runners":[{"id":"42"}]}`}, false},
		{"fractional_id", []string{`{"total_count":1,"runners":[{"id":42.5}]}`}, false},
		{"negative_id", []string{`{"total_count":1,"runners":[{"id":-1}]}`}, false},
		{"zero_id", []string{`{"total_count":1,"runners":[{"id":0}]}`}, false},
		{"overflow_id", []string{`{"total_count":1,"runners":[{"id":9223372036854775808}]}`}, false},
		{"empty_object", []string{`{}`}, false},
		{"outer_null", []string{`null`}, false},
		{"missing_count", []string{`{"runners":[]}`}, false},
		{"null_count", []string{`{"total_count":null,"runners":[]}`}, false},
		{"missing_runners", []string{`{"total_count":0}`}, false},
		{"null_runners", []string{`{"total_count":0,"runners":null}`}, false},
		{"duplicate_count", []string{`{"total_count":1,"total_count":0,"runners":[]}`}, false},
		{"folded_count_collision", []string{`{"total_count":1,"TOTAL_COUNT":0,"runners":[]}`}, false},
		{"duplicate_runners", []string{`{"total_count":0,"runners":[{"id":42}],"runners":[]}`}, false},
		{"unicode_folded_runners_collision", []string{`{"total_count":0,"runners":[{"id":42}],"runnerſ":[]}`}, false},
		{"duplicate_runner_id", []string{`{"total_count":1,"runners":[{"id":42,"id":43}]}`}, false},
		{"folded_runner_id_collision", []string{`{"total_count":1,"runners":[{"id":42,"ID":43}]}`}, false},
		{"escaped_duplicate_id", []string{`{"total_count":1,"runners":[{"id":42,"\u0069d":43}]}`}, false},
		{"count_decreases", []string{inventoryFixturePage(102, 1, 100), inventoryFixturePage(101, 101, 1)}, false},
		{"count_increases", []string{inventoryFixturePage(101, 1, 100), inventoryFixturePage(102, 101, 2)}, false},
		{"wrong_count_type", []string{`{"total_count":"0","runners":[]}`}, false},
		{"fractional_count", []string{`{"total_count":0.5,"runners":[]}`}, false},
		{"negative_count", []string{`{"total_count":-1,"runners":[]}`}, false},
		{"wrong_runners_type", []string{`{"total_count":0,"runners":{}}`}, false},
		{"null_runner_id", []string{`{"total_count":1,"runners":[{"id":null}]}`}, false},
		{"missing_runner_id", []string{`{"total_count":1,"runners":[{}]}`}, false},
		{"repeated_runner_identity", []string{`{"total_count":2,"runners":[{"id":42},{"id":42}]}`}, false},
		{"trailing_document", []string{`{"total_count":0,"runners":[]} {}`}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := approval()
			c := credentials(a)
			var requests atomic.Int32
			server, client := inventoryFixtureTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/orgs/"+a.Organization+"/actions/runners" || r.URL.Query().Get("per_page") != "100" || r.Header.Get("Authorization") != "Bearer "+c.InstallationToken {
					t.Error("unexpected inventory request")
					http.Error(w, "fixture request rejected", 400)
					return
				}
				page, e := strconv.Atoi(r.URL.Query().Get("page"))
				if e != nil || page < 1 || page > len(tc.pages) {
					t.Error("unexpected inventory page")
					http.Error(w, "fixture page rejected", 400)
					return
				}
				_, _ = w.Write([]byte(tc.pages[page-1]))
			}))
			api := &SDKAPI{rest: client, baseURL: server.URL, approval: a, credentials: c}
			digest, err := api.Inventory(context.Background())
			if tc.valid {
				if err != nil || len(digest) != 64 || int(requests.Load()) != len(tc.pages) {
					t.Fatalf("valid inventory rejected: requests=%d digest_present=%v error=%v", requests.Load(), digest != "", err)
				}
				var expectedJSON string
				switch tc.name {
				case "valid_empty", "valid_lone_alias":
					expectedJSON = "null"
				case "valid_nonempty":
					expectedJSON = "[42,43]"
				case "valid_large_id":
					expectedJSON = "[9223372036854775807]"
				case "valid_metadata", "valid_unicode_alias":
					expectedJSON = "[42]"
				case "valid_pagination", "valid_maximum":
					n := 102
					if tc.name == "valid_maximum" {
						n = 1000
					}
					ids := make([]int64, n)
					for i := range ids {
						ids[i] = int64(i + 1)
					}
					raw, _ := json.Marshal(ids)
					expectedJSON = string(raw)
				default:
					t.Fatal("missing exact legacy digest control")
				}
				sum := sha256.Sum256([]byte(expectedJSON))
				if digest != hex.EncodeToString(sum[:]) {
					t.Fatal("valid inventory digest changed")
				}
			} else if err == nil || digest != "" {
				t.Fatalf("incomplete or ambiguous inventory accepted as known: requests=%d digest=%s error=%v", requests.Load(), digest, err)
			}
		})
	}
}

// Only the already separately tested remote policy preflight is suppressed.
// Inventory and all scale-set calls below use the production SDKAPI methods,
// actual pinned SDK, TLS transport, and actual private FileJournal.
type inventoryFixturePolicyApproved struct{ *SDKAPI }

func (inventoryFixturePolicyApproved) Preflight(context.Context, Approval) error { return nil }

func TestInventoryMalformedStopsLegacyEffects(t *testing.T) {
	cases := []struct{ name, body string }{{"valid_empty_control", ""}, {"malformed_create", "{}"}, {"malformed_cleanup", "{}"}}
	for _, fault := range []struct{ name, body string }{
		{"outer_null", `null`}, {"missing_count", `{"runners":[]}`}, {"null_count", `{"total_count":null,"runners":[]}`},
		{"missing_runners", `{"total_count":0}`}, {"null_runners", `{"total_count":0,"runners":null}`},
		{"duplicate_count", `{"total_count":1,"total_count":0,"runners":[]}`},
		{"folded_runners", `{"total_count":0,"runners":[{"id":42}],"runnerſ":[]}`},
		{"secret_trailing", `{"total_count":0,"runners":[],"secret":"synthetic-inventory-secret"} {}`},
	} {
		for _, phase := range []string{"create", "cleanup"} {
			cases = append(cases, struct{ name, body string }{fault.name + "_" + phase, fault.body})
		}
	}
	for _, tc := range cases {
		scenario := tc.name
		t.Run(scenario, func(t *testing.T) {
			a := approval()
			c := credentials(a)
			var creates, deletes, inventories atomic.Int32
			set := &scaleset.RunnerScaleSet{ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID, Labels: []scaleset.Label{{Name: a.setName(), Type: "System"}}, Statistics: &scaleset.RunnerScaleSetStatistic{}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true}}
			var baseURL string
			server, client := inventoryFixtureTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body any
				switch {
				case r.URL.Path == "/orgs/"+a.Organization+"/actions/runners":
					call := inventories.Add(1)
					if r.Method != http.MethodGet || r.URL.Query().Get("per_page") != "100" || r.URL.Query().Get("page") != "1" || r.Header.Get("Authorization") != "Bearer "+c.InstallationToken {
						t.Error("unexpected inventory call")
						http.Error(w, "fixture request rejected", 400)
						return
					}
					if strings.HasSuffix(scenario, "_create") || (strings.HasSuffix(scenario, "_cleanup") && call > 1) {
						_, _ = w.Write([]byte(tc.body))
					} else {
						_, _ = w.Write([]byte(`{"total_count":0,"runners":[]}`))
					}
					return
				case strings.HasSuffix(r.URL.Path, "/runners/registration-token"):
					w.WriteHeader(http.StatusCreated)
					body = map[string]string{"token": "synthetic-registration"}
				case strings.HasSuffix(r.URL.Path, "/actions/runner-registration"):
					claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
					w.WriteHeader(http.StatusCreated)
					body = map[string]string{"url": baseURL + "/tenant/", "token": "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."}
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/runnerscalesets"):
					body = map[string]any{"count": 0, "value": []any{}}
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/runnerscalesets"):
					creates.Add(1)
					body = set
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
					body = set
				case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
					deletes.Add(1)
					w.WriteHeader(http.StatusNoContent)
					return
				default:
					t.Errorf("unexpected fixture path: %s %s", r.Method, r.URL.Path)
					http.Error(w, "fixture route rejected", 404)
					return
				}
				_ = json.NewEncoder(w).Encode(body)
			}))
			baseURL = server.URL
			retry := retryablehttp.NewClient()
			retry.RetryMax = 0
			retry.Logger = nil
			retry.HTTPClient = client
			options := []scaleset.HTTPOption{scaleset.WithRetryableHTTPClint(retry)}
			sdk, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: server.URL + "/fixture-org", PersonalAccessToken: c.InstallationToken}, options...)
			if err != nil {
				t.Fatal("SDK construction failed")
			}
			api := inventoryFixturePolicyApproved{&SDKAPI{client: sdk, rest: client, baseURL: server.URL, approval: a, credentials: c, options: options}}
			journal, err := openTestJournal(t, privateDir(t), a)
			if err != nil {
				t.Fatal(err)
			}
			defer journal.Close()
			defer func() {
				raw, _ := json.Marshal(journal.Events())
				if strings.Contains(string(raw), "synthetic-inventory-secret") {
					t.Error("inventory response secret persisted")
				}
			}()
			driver := Driver{a, journal, api}
			createErr := driver.Run(context.Background(), "create")
			if strings.HasSuffix(scenario, "_create") {
				if createErr == nil || creates.Load() != 0 {
					t.Fatalf("malformed inventory authorized legacy create: creates=%d inventories=%d error=%v", creates.Load(), inventories.Load(), createErr)
				}
				return
			}
			if createErr != nil || creates.Load() != 1 {
				t.Fatalf("valid initial inventory control failed: creates=%d error=%v", creates.Load(), createErr)
			}
			cleanupErr := driver.Run(context.Background(), "cleanup")
			if strings.HasSuffix(scenario, "_cleanup") {
				if cleanupErr == nil || deletes.Load() != 0 {
					t.Fatalf("malformed inventory authorized legacy delete: deletes=%d inventories=%d error=%v", deletes.Load(), inventories.Load(), cleanupErr)
				}
			} else if cleanupErr != nil || deletes.Load() != 1 || inventories.Load() != 3 {
				t.Fatalf("valid empty lifecycle control failed: creates=%d deletes=%d inventory=%d error=%v", creates.Load(), deletes.Load(), inventories.Load(), cleanupErr)
			}
		})
	}
}

func TestInventoryTransportRefusalIsBoundedAndSanitized(t *testing.T) {
	for _, scenario := range []string{"cancel_before", "cancel_inflight", "deadline", "expired_credential", "status", "redirect", "truncated", "oversize", "gzip_oversize", "malformed_secret", "connection_closed"} {
		t.Run(scenario, func(t *testing.T) {
			a := approval()
			c := credentials(a)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var requests atomic.Int32
			server, client := inventoryFixtureTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/orgs/"+a.Organization+"/actions/runners" {
					t.Error("unexpected endpoint")
					http.Error(w, "fixture rejected", 400)
					return
				}
				switch scenario {
				case "cancel_inflight":
					cancel()
					<-r.Context().Done()
					return
				case "deadline":
					<-r.Context().Done()
					return
				case "status":
					http.Error(w, "synthetic-inventory-secret", http.StatusForbidden)
					return
				case "redirect":
					w.Header().Set("Location", "https://fixture.invalid/synthetic-inventory-secret")
					w.WriteHeader(http.StatusFound)
					return
				case "truncated":
					w.Header().Set("Content-Length", "100")
					_, _ = w.Write([]byte(`{"total_count":0,"runners":[]}`))
					return
				case "connection_closed":
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error("fixture hijack failed")
						return
					}
					_ = conn.Close()
					return
				case "oversize":
					_, _ = w.Write([]byte(strings.Repeat("synthetic-inventory-secret", 50000)))
					return
				case "gzip_oversize":
					w.Header().Set("Content-Encoding", "gzip")
					gz := gzip.NewWriter(w)
					_, _ = gz.Write([]byte(strings.Repeat("synthetic-inventory-secret", 50000)))
					_ = gz.Close()
					return
				case "malformed_secret":
					_, _ = w.Write([]byte(`{"total_count":0,"runners":[],"secret":"synthetic-inventory-secret"} {}`))
					return
				}
				_, _ = w.Write([]byte(`{"total_count":0,"runners":[]}`))
			}))
			if scenario == "cancel_before" {
				cancel()
			}
			if scenario == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer deadlineCancel()
			}
			if scenario == "expired_credential" {
				c.ExpiresAt = time.Now().Add(-time.Second)
			}
			api := &SDKAPI{rest: client, baseURL: server.URL, approval: a, credentials: c}
			before := time.Now()
			digest, err := api.Inventory(ctx)
			if digest != "" || !errors.Is(err, ErrRemote) || strings.Contains(err.Error(), "synthetic") {
				t.Fatalf("transport failure returned non-normalized fact: digest_present=%v error=%v", digest != "", err)
			}
			want := int32(1)
			if scenario == "cancel_before" || scenario == "expired_credential" {
				want = 0
			}
			if requests.Load() != want || time.Since(before) > time.Second {
				t.Fatalf("unbounded or repeated inventory request: requests=%d", requests.Load())
			}
		})
	}
}

func TestInventoryImpossibleTotalStopsBeforeNextPage(t *testing.T) {
	a := approval()
	var requests atomic.Int32
	server, client := inventoryFixtureTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(inventoryFixturePage(1001, 1, 100)))
	}))
	api := &SDKAPI{rest: client, baseURL: server.URL, approval: a, credentials: credentials(a)}
	digest, err := api.Inventory(context.Background())
	if digest != "" || !errors.Is(err, ErrRemote) || requests.Load() != 1 {
		t.Fatalf("impossible total did not stop at first page: requests=%d digest_present=%v error=%v", requests.Load(), digest != "", err)
	}
}
