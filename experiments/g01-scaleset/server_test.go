package contract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/google/uuid"
)

// Candidate-only fixtures: no real credential, runner, or non-loopback request.
var errStop = errors.New("fixture stop")

type scaleFunc func(context.Context, *scaleset.RunnerScaleSetMessage) error

func (f scaleFunc) Scale(ctx context.Context, message *scaleset.RunnerScaleSetMessage) error {
	return f(ctx, message)
}

func testContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx, cancel
}

func newServer(t *testing.T, handler http.HandlerFunc, options ...scaleset.HTTPOption) (*httptest.Server, *scaleset.Client, *http.Client) {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/runners/registration-token"):
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]string{"token": "synthetic-registration"})
		case strings.HasSuffix(r.URL.Path, "/actions/runner-registration"):
			claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
			token := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]string{"url": server.URL + "/tenant/", "token": token})
		default:
			handler(w, r)
		}
	}))
	t.Cleanup(server.Close)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != server.Listener.Addr().String() {
			return nil, errors.New("offline fixture denied non-fixture address")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	t.Cleanup(transport.CloseIdleConnections)
	httpClient := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	base := []scaleset.HTTPOption{scaleset.WithHTTPClient(httpClient), scaleset.WithRetry(scaleset.RetryConfig{Max: 0})}
	base = append(base, options...)
	client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{
		GitHubConfigURL: server.URL + "/fixture-org", PersonalAccessToken: "synthetic-pat",
	}, base...)
	if err != nil {
		t.Fatal("offline client construction failed")
	}
	return server, client, httpClient
}

func writeJSON(w http.ResponseWriter, value any) { _ = json.NewEncoder(w).Encode(value) }

func sessionReply(w http.ResponseWriter, r *http.Request, generation, assigned int) {
	writeJSON(w, scaleset.RunnerScaleSetSession{
		SessionID:               uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", generation)),
		MessageQueueURL:         "http://" + r.Host + "/queue",
		MessageQueueAccessToken: fmt.Sprintf("synthetic-session-%d", generation),
		Statistics:              &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: assigned},
	})
}

func messageReply(w http.ResponseWriter, id, assigned int, events []map[string]any) {
	body, _ := json.Marshal(events)
	writeJSON(w, map[string]any{"messageId": id, "messageType": "RunnerScaleSetJobMessages", "body": string(body),
		"statistics": map[string]int{"totalAssignedJobs": assigned}})
}

func newSession(t *testing.T, client *scaleset.Client) *scaleset.MessageSessionClient {
	t.Helper()
	ctx, _ := testContext(t)
	session, err := client.MessageSessionClient(ctx, 7, "fixture-owner")
	if err != nil {
		t.Fatal("offline session creation failed")
	}
	return session
}

func newListener(t *testing.T, session *scaleset.MessageSessionClient, capacity int) *listener.Listener {
	t.Helper()
	value, err := listener.New(session, listener.Config{ScaleSetID: 7, MaxRunners: capacity})
	if err != nil {
		t.Fatal("offline listener construction failed")
	}
	return value
}

func dropResponse(w http.ResponseWriter) {
	connection, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_ = connection.Close()
	}
}

func await(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture barrier not reached")
	}
}
