package contract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
)

var errCrash = errors.New("injected crash")

// Every SDK request goes to this loopback server. Authentication is synthetic;
// no environment credentials, GitHub App, runner, or production endpoint is used.
func newServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *scaleset.Client) {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/runners/registration-token"):
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]string{"token": "synthetic-registration"})
		case strings.HasSuffix(r.URL.Path, "/actions/runner-registration"):
			// The SDK parses expiry without verifying the JWT. This unsigned token
			// is deliberately unusable outside this fake server.
			claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
			token := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]string{"url": server.URL + "/tenant/", "token": token})
		default:
			handler(w, r)
		}
	}))
	t.Cleanup(server.Close)
	retry := retryablehttp.NewClient()
	retry.RetryMax = 0 // Disambiguate one SDK operation from implicit HTTP retries.
	retry.Logger = nil
	transport := retry.HTTPClient.Transport.(*http.Transport)
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != server.Listener.Addr().String() {
			return nil, errors.New("offline harness denied non-fixture address")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	retry.HTTPClient.Timeout = 2 * time.Second
	t.Cleanup(transport.CloseIdleConnections)
	client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{
		GitHubConfigURL: server.URL + "/fixture-org", PersonalAccessToken: "synthetic-pat",
	}, scaleset.WithRetryableHTTPClint(retry))
	if err != nil {
		t.Fatal("constructing offline SDK client failed")
	}
	return server, client
}

func writeJSON(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }

func sessionReply(w http.ResponseWriter, r *http.Request, generation int, assigned int) {
	writeJSON(w, scaleset.RunnerScaleSetSession{
		SessionID:       uuid.MustParse("00000000-0000-4000-8000-00000000000" + string(rune('0'+generation))),
		MessageQueueURL: "http://" + r.Host + "/queue", MessageQueueAccessToken: "synthetic-session",
		Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: assigned},
	})
}

func messageReply(w http.ResponseWriter, id, assigned int, events []map[string]any) {
	body, _ := json.Marshal(events)
	writeJSON(w, map[string]any{
		"messageId": id, "messageType": "RunnerScaleSetJobMessages", "body": string(body),
		"statistics": map[string]int{"totalAssignedJobs": assigned},
	})
}

func newSession(t *testing.T, client *scaleset.Client) *scaleset.MessageSessionClient {
	t.Helper()
	s, err := client.MessageSessionClient(context.Background(), 7, "fixture-owner")
	if err != nil {
		t.Fatal("creating offline session failed")
	}
	return s
}
