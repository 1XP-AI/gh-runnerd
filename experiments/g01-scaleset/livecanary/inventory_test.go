package livecanary

import (
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
	cases := []struct {
		name  string
		pages []string
		valid bool
	}{
		{"valid_empty", []string{`{"total_count":0,"runners":[]}`}, true},
		{"valid_nonempty", []string{`{"total_count":2,"runners":[{"id":43},{"id":42}]}`}, true},
		{"valid_pagination", []string{inventoryFixturePage(102, 1, 100), inventoryFixturePage(102, 101, 2)}, true},
		{"valid_lone_alias", []string{`{"TOTAL_COUNT":0,"RUNNERS":[]}`}, true},
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
				if tc.name == "valid_empty" || tc.name == "valid_lone_alias" {
					sum := sha256.Sum256([]byte("null"))
					if digest != hex.EncodeToString(sum[:]) {
						t.Fatal("legacy empty digest changed")
					}
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
	for _, scenario := range []string{"valid_empty_control", "malformed_create", "malformed_cleanup"} {
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
					if scenario == "malformed_create" || (scenario == "malformed_cleanup" && call > 1) {
						_, _ = w.Write([]byte(`{}`))
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
			driver := Driver{a, journal, api}
			createErr := driver.Run(context.Background(), "create")
			if scenario == "malformed_create" {
				if createErr == nil || creates.Load() != 0 {
					t.Fatalf("malformed inventory authorized legacy create: creates=%d inventories=%d error=%v", creates.Load(), inventories.Load(), createErr)
				}
				return
			}
			if createErr != nil || creates.Load() != 1 {
				t.Fatalf("valid initial inventory control failed: creates=%d error=%v", creates.Load(), createErr)
			}
			cleanupErr := driver.Run(context.Background(), "cleanup")
			if scenario == "malformed_cleanup" {
				if cleanupErr == nil || deletes.Load() != 0 {
					t.Fatalf("malformed inventory authorized legacy delete: deletes=%d inventories=%d error=%v", deletes.Load(), inventories.Load(), cleanupErr)
				}
			} else if cleanupErr != nil || deletes.Load() != 1 || inventories.Load() != 3 {
				t.Fatalf("valid empty lifecycle control failed: creates=%d deletes=%d inventory=%d error=%v", creates.Load(), deletes.Load(), inventories.Load(), cleanupErr)
			}
		})
	}
}
