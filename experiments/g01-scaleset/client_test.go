package contract

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actions/scaleset"
)

func TestV05SessionHTTPFailures(t *testing.T) {
	for _, operation := range []string{"poll", "ack", "acquire"} {
		for _, status := range []int{401, 403, 429} {
			t.Run(operation+"/"+strconv.Itoa(status), func(t *testing.T) {
				var attempts, refreshes atomic.Int32
				_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodPatch:
						refreshes.Add(1)
						sessionReply(w, r, 2, 8)
					case strings.HasSuffix(r.URL.Path, "/sessions"):
						sessionReply(w, r, 1, 0)
					default:
						if attempts.Add(1) == 1 {
							w.WriteHeader(status)
							return
						}
						if r.Header.Get("Authorization") != "Bearer synthetic-session-2" {
							t.Error("refreshed session token not used")
						}
						switch operation {
						case "poll":
							if r.URL.Query().Get("lastMessageId") != "91" {
								t.Error("poll cursor lost on refresh")
							}
							messageReply(w, 92, 8, nil)
						case "ack":
							if !strings.HasSuffix(r.URL.Path, "/91") {
								t.Error("ACK ID lost on refresh")
							}
							w.WriteHeader(http.StatusNoContent)
						case "acquire":
							writeJSON(w, map[string]any{"count": 1, "value": []int64{42}})
						}
					}
				})
				session := newSession(t, client)
				ctx, _ := testContext(t)
				var err error
				switch operation {
				case "poll":
					_, err = session.GetMessage(ctx, 91, 8)
				case "ack":
					err = session.DeleteMessage(ctx, 91)
				case "acquire":
					_, err = session.AcquireJobs(ctx, []int64{42})
				}
				wantAttempts, wantRefresh := int32(1), int32(0)
				if status == 401 {
					wantAttempts, wantRefresh = 2, 1
				}
				if (err == nil) != (status == 401) || attempts.Load() != wantAttempts || refreshes.Load() != wantRefresh {
					t.Fatal("session retry/refresh boundary changed")
				}
				if status == 429 {
					var rateLimit *scaleset.RateLimitError
					if !errors.As(err, &rateLimit) {
						t.Fatal("rate-limit classification missing")
					}
				}
			})
		}
	}
}

func TestV05LateResponseSurvivesSessionRefresh(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	var refreshes atomic.Int32
	_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch:
			refreshes.Add(1)
			sessionReply(w, r, 2, 8)
		case strings.HasSuffix(r.URL.Path, "/sessions"):
			sessionReply(w, r, 1, 0)
		case r.Method == http.MethodDelete:
			if r.Header.Get("Authorization") == "Bearer synthetic-session-1" {
				w.WriteHeader(http.StatusUnauthorized)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
		case r.URL.Path == "/queue":
			if r.Header.Get("Authorization") != "Bearer synthetic-session-1" {
				t.Error("late poll did not start on old session")
			}
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			messageReply(w, 10, 1, nil)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	session := newSession(t, client)
	ctx, _ := testContext(t)
	type result struct {
		message *scaleset.RunnerScaleSetMessage
		err     error
	}
	done := make(chan result, 1)
	go func() { message, err := session.GetMessage(ctx, 0, 1); done <- result{message, err} }()
	await(t, entered)
	if session.DeleteMessage(ctx, 9) != nil || refreshes.Load() != 1 || session.Session().Statistics.TotalAssignedJobs != 8 {
		t.Fatal("concurrent session refresh failed")
	}
	once.Do(func() { close(release) })
	select {
	case got := <-done:
		if got.err != nil || got.message == nil || got.message.Statistics.TotalAssignedJobs != 1 || session.Session().Statistics.TotalAssignedJobs != 8 {
			t.Fatal("old response/new session control changed")
		}
	case <-ctx.Done():
		t.Fatal("late-response probe did not terminate")
	}
	// The returned message carries no generation. This is evidence of a missing
	// caller fence, not permission to treat the late demand=1 as current demand.
}

func TestV05TransportRejectsNonFixtureAddress(t *testing.T) {
	_, _, client := newServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	ctx, _ := testContext(t)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1/denied", nil)
	if err != nil {
		t.Fatal("invalid guard probe")
	}
	response, err := client.Do(request)
	if response != nil {
		response.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "offline fixture denied non-fixture address") {
		t.Fatal("fixture transport did not fail closed before dialing")
	}
}

func TestV05PollCancellationAndClientOwnership(t *testing.T) {
	entered := make(chan struct{})
	finished := make(chan struct{})
	_, client, httpClient := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sessions") {
			sessionReply(w, r, 1, 0)
			return
		}
		close(entered)
		<-r.Context().Done()
		close(finished)
	})
	session := newSession(t, client)
	ctx, cancel := testContext(t)
	done := make(chan error, 1)
	go func() { _, err := session.GetMessage(ctx, 0, 1); done <- err }()
	await(t, entered)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("poll ignored caller cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("poll cancellation did not terminate")
	}
	await(t, finished)
	if httpClient.Timeout != 2*time.Second {
		t.Fatal("poll changed caller-owned HTTP client timeout")
	}
}

func TestV05RequestRetryControlsJITReissue(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent-retries", true: "request-disables"}[override], func(t *testing.T) {
			var attempts atomic.Int32
			retry := scaleset.RetryConfig{Max: 2, Backoff: func(time.Duration, time.Duration, int, *http.Response) time.Duration { return 0 }}
			_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/generatejitconfig") {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				attempts.Add(1)
				dropResponse(w)
			}, scaleset.WithRetry(retry))
			ctx, _ := testContext(t)
			if override {
				ctx = scaleset.WithRequestRetry(ctx, func(config *scaleset.RetryConfig) { config.Max = 0 })
			}
			_, err := client.GenerateJitRunnerConfig(ctx, &scaleset.RunnerScaleSetJitRunnerSetting{Name: "owned-1", WorkFolder: "_work"}, 7)
			want := int32(3)
			if override {
				want = 1
			}
			if err == nil || attempts.Load() != want {
				t.Fatal("JIT transport-loss retry boundary changed")
			}
			if override {
				// Per-request override must not mutate the shared parent policy.
				fresh, _ := testContext(t)
				_, _ = client.GenerateJitRunnerConfig(fresh, &scaleset.RunnerScaleSetJitRunnerSetting{Name: "owned-2", WorkFolder: "_work"}, 7)
				if attempts.Load() != 4 {
					t.Fatal("request override leaked into parent policy")
				}
			}
		})
	}
}

func TestV05RetryAfterCannotOutliveCallerDeadline(t *testing.T) {
	var attempts atomic.Int32
	retry := scaleset.RetryConfig{Max: 2, OnRateLimit: scaleset.RateLimitRetries(scaleset.RateLimitRule{MaxRetries: 2}, scaleset.RateLimitRule{MaxRetries: 2})}
	_, client, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}, scaleset.WithRetry(retry))
	ctx, _ := testContext(t)
	_, err := client.GetRunnerScaleSetByID(ctx, 7)
	var limited *scaleset.RateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter != time.Hour || attempts.Load() != 1 || ctx.Err() != nil {
		t.Fatal("rate-limit wait was not rejected before the caller deadline")
	}
}

func TestV05BusyRemovalKeepsSentinelButRawErrorIsUnsafe(t *testing.T) {
	const marker = "synthetic-private-response-marker"
	var attempts atomic.Int32
	_, client, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]string{"typeName": "JobStillRunningException", "message": marker})
	})
	ctx, _ := testContext(t)
	err := client.RemoveRunner(ctx, 99)
	if !errors.Is(err, scaleset.JobStillRunningError) || !strings.Contains(err.Error(), marker) || attempts.Load() != 1 {
		t.Fatal("busy sentinel/raw-error exposure contract changed")
	}
	// Assert the SDK hazard without writing its raw error to output or artifacts.
}

func TestV05JITLossKeepsRecoveryQuarantined(t *testing.T) {
	for _, mode := range []string{"not-attempted", "uncommitted", "committed"} {
		t.Run(mode, func(t *testing.T) {
			var creates, reads atomic.Int32
			var committed atomic.Bool
			_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/generatejitconfig"):
					creates.Add(1)
					committed.Store(mode == "committed")
					dropResponse(w)
				case strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
					reads.Add(1)
					writeJSON(w, scaleset.RunnerScaleSet{ID: 7, Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 2}})
				case strings.HasSuffix(r.URL.Path, "/agents"):
					reads.Add(1)
					if r.URL.Query().Get("agentName") != "owned-1" {
						t.Error("wrong independent inventory query")
					}
					list := scaleset.RunnerReferenceList{}
					if committed.Load() {
						list.Count = 1
						list.RunnerReferences = []scaleset.RunnerReference{{ID: 99, Name: "owned-1", RunnerScaleSetID: 7}}
					}
					writeJSON(w, list)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			ctx, _ := testContext(t)
			if mode != "not-attempted" {
				if _, err := client.GenerateJitRunnerConfig(ctx, &scaleset.RunnerScaleSetJitRunnerSetting{Name: "owned-1", WorkFolder: "_work"}, 7); err == nil {
					t.Fatal("response-loss fixture unexpectedly succeeded")
				}
			}
			input := recoveryState{Workers: map[string]string{"owned-1": "creating"}}
			state, err := recoverState(ctx, client, input)
			want := int32(1)
			if mode == "not-attempted" {
				want = 0
			}
			if err != nil || creates.Load() != want || reads.Load() != 2 || !state.AdmissionPaused || state.Desired != 2 || !reflect.DeepEqual(state.Workers, map[string]string{"owned-1": "quarantined"}) || input.Workers["owned-1"] != "creating" {
				t.Fatal("lost JIT response released, retried, or mutated reservation")
			}
			id, found := state.References["owned-1"]
			if found != (mode == "committed") || (found && id != 99) {
				t.Fatal("independent recovery read did not reflect fixture commit")
			}
		})
	}
}

func TestV05RecoveryFailsClosed(t *testing.T) {
	for _, mode := range []string{"statistics-error", "nil-statistics", "negative-statistics", "inventory-error", "foreign-owner", "absent"} {
		t.Run(mode, func(t *testing.T) {
			_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
					if mode == "statistics-error" {
						w.WriteHeader(http.StatusForbidden)
						writeJSON(w, map[string]string{"message": "synthetic-private-response-marker"})
						return
					}
					stats := &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 2}
					if mode == "nil-statistics" {
						stats = nil
					}
					if mode == "negative-statistics" {
						stats.TotalAssignedJobs = -1
					}
					writeJSON(w, scaleset.RunnerScaleSet{ID: 7, Statistics: stats})
				case strings.HasSuffix(r.URL.Path, "/agents"):
					if mode == "inventory-error" {
						w.WriteHeader(http.StatusForbidden)
						writeJSON(w, map[string]string{"message": "synthetic-private-response-marker"})
						return
					}
					if mode == "absent" {
						writeJSON(w, scaleset.RunnerReferenceList{})
						return
					}
					writeJSON(w, scaleset.RunnerReferenceList{Count: 1, RunnerReferences: []scaleset.RunnerReference{{ID: 99, Name: "owned-1", RunnerScaleSetID: 8}}})
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			ctx, _ := testContext(t)
			input := recoveryState{Workers: map[string]string{"owned-1": "running"}, References: map[string]int{"owned-1": 88}}
			state, err := recoverState(ctx, client, input)
			if (err == nil) != (mode == "absent") || !state.AdmissionPaused || state.Workers["owned-1"] != "quarantined" || len(state.References) != 0 || input.Workers["owned-1"] != "running" || input.References["owned-1"] != 88 {
				t.Fatal("recovery failed to preserve quarantine/input state")
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-private-response-marker") {
				t.Fatal("recovery leaked raw SDK response")
			}
		})
	}
}
