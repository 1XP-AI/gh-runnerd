package contract

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
)

// dropResponse models a side effect committed by the server with no response
// delivered to the client. It does not model GitHub's actual replay/expiry rules.
func dropResponse(w http.ResponseWriter) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

func TestSDKACKBoundaries(t *testing.T) {
	for _, boundary := range []string{"before-ack", "ack-response-lost", "after-ack"} {
		t.Run(boundary, func(t *testing.T) {
			var acked atomic.Bool
			_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/sessions"):
					sessionReply(w, r, 1, 0)
				case r.Method == http.MethodGet && r.URL.Path == "/queue":
					if acked.Load() {
						w.WriteHeader(http.StatusAccepted)
						return
					}
					messageReply(w, 10, 1, []map[string]any{{"messageType": "JobStarted", "runnerRequestId": 42}})
				case r.Method == http.MethodDelete && r.URL.Path == "/queue/10":
					if boundary == "before-ack" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					acked.Store(true)
					if boundary == "ack-response-lost" {
						dropResponse(w)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			s := newSession(t, client)
			l, err := listener.New(s, listener.Config{ScaleSetID: 7, MaxRunners: 1})
			if err != nil {
				t.Fatal(err)
			}
			called := 0
			err = l.Run(context.Background(), callbacks{started: func(context.Context, *scaleset.JobStarted) error { called++; return errCrash }})
			if err == nil {
				t.Fatal("expected injected failure")
			}
			wantCallbacks := 0
			if boundary == "after-ack" {
				wantCallbacks = 1
			}
			if called != wantCallbacks {
				t.Fatalf("callbacks=%d, want %d", called, wantCallbacks)
			}
			msg, err := s.GetMessage(context.Background(), 0, 1)
			if err != nil {
				t.Fatal("poll failed")
			}
			if (msg != nil) != (boundary == "before-ack") {
				t.Fatal("unexpected fixture replay state")
			}
		})
	}
}

func TestSDKAcquisitionResponseLossAfterACK(t *testing.T) {
	var acked, acquired atomic.Bool
	var attempts atomic.Int32
	_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sessions"):
			sessionReply(w, r, 1, 0)
		case r.URL.Path == "/queue" && r.Method == http.MethodGet:
			messageReply(w, 10, 1, []map[string]any{{"messageType": "JobAvailable", "runnerRequestId": 42}})
		case r.URL.Path == "/queue/10":
			acked.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/acquirejobs"):
			attempts.Add(1)
			if !acked.Load() {
				t.Error("acquisition preceded ACK")
			}
			var ids []int64
			if json.NewDecoder(r.Body).Decode(&ids) != nil || !reflect.DeepEqual(ids, []int64{42}) {
				t.Error("unexpected acquisition body")
			}
			acquired.Store(true)
			dropResponse(w)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	l, _ := listener.New(newSession(t, client), listener.Config{ScaleSetID: 7, MaxRunners: 1})
	desiredCallbacks := 0
	err := l.Run(context.Background(), callbacks{desired: func(_ context.Context, n int) (int, error) { desiredCallbacks++; return n, nil }})
	if err == nil || !acked.Load() || !acquired.Load() || attempts.Load() != 1 || desiredCallbacks != 1 {
		t.Fatal("expected ACK, remote acquisition, lost response and no message desired callback")
	}
}

func TestSDKDemandAboveFiftyAndPartialAcquisition(t *testing.T) {
	// Upstream documents at most 50 events per response. Demand may be greater.
	for _, eventCount := range []int{50, 51} {
		t.Run(map[int]string{50: "documented-batch", 51: "adversarial-oversized-batch"}[eventCount], func(t *testing.T) {
			var acquiredCount atomic.Int32
			_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/sessions"):
					sessionReply(w, r, 1, 0)
				case r.URL.Path == "/queue":
					events := make([]map[string]any, eventCount)
					for i := range events {
						events[i] = map[string]any{"messageType": "JobAvailable", "runnerRequestId": i + 1}
					}
					messageReply(w, 10, 125, events)
				case r.URL.Path == "/queue/10":
					w.WriteHeader(http.StatusNoContent)
				case strings.HasSuffix(r.URL.Path, "/acquirejobs"):
					var ids []int64
					if json.NewDecoder(r.Body).Decode(&ids) != nil {
						t.Error("bad acquisition body")
					}
					acquiredCount.Store(int32(len(ids)))
					writeJSON(w, map[string]any{"count": 1, "value": []int64{1}})
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			l, _ := listener.New(newSession(t, client), listener.Config{ScaleSetID: 7, MaxRunners: 125})
			var desired []int
			err := l.Run(context.Background(), callbacks{desired: func(_ context.Context, n int) (int, error) {
				desired = append(desired, n)
				if len(desired) == 2 {
					return n, errCrash
				}
				return n, nil
			}})
			if !errors.Is(err, errCrash) || acquiredCount.Load() != int32(eventCount) || !reflect.DeepEqual(desired, []int{0, 125}) {
				t.Fatal("listener did not use total assigned jobs independently of events/acquired subset")
			}
		})
	}
}

func TestSDKRepeatedStatisticsAnd202ReuseLastObservation(t *testing.T) {
	var polls atomic.Int32
	_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sessions"):
			sessionReply(w, r, 1, 0)
		case r.URL.Path == "/queue" && r.Method == http.MethodGet:
			n := polls.Add(1)
			if n == 4 {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			assigned := 3
			if n == 3 {
				assigned = 1
			}
			messageReply(w, int(n), assigned, []map[string]any{{"messageType": "JobAssigned", "runnerRequestId": 42}})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	l, _ := listener.New(newSession(t, client), listener.Config{ScaleSetID: 7, MaxRunners: 4})
	var desired []int
	started, completed := 0, 0
	err := l.Run(context.Background(), callbacks{
		started:   func(context.Context, *scaleset.JobStarted) error { started++; return nil },
		completed: func(context.Context, *scaleset.JobCompleted) error { completed++; return nil },
		desired: func(_ context.Context, n int) (int, error) {
			desired = append(desired, n)
			if len(desired) == 5 {
				return n, errCrash
			}
			return n, nil
		},
	})
	if !errors.Is(err, errCrash) || !reflect.DeepEqual(desired, []int{0, 3, 3, 1, 1}) || started != 0 || completed != 0 {
		t.Fatal("expected overwrite semantics, cached 202 statistics and no assigned lifecycle callback")
	}
}

func TestSDKHTTPFailuresAndSessionRefresh(t *testing.T) {
	for _, operation := range []string{"poll", "ack", "acquire"} {
		for _, status := range []int{401, 403, 429} {
			t.Run(operation+"/"+http.StatusText(status), func(t *testing.T) {
				var attempts, refreshes atomic.Int32
				_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.HasSuffix(r.URL.Path, "/sessions"):
						sessionReply(w, r, 1, 0)
					case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/sessions/"):
						refreshes.Add(1)
						sessionReply(w, r, 2, 8)
					default:
						n := attempts.Add(1)
						if n == 1 {
							w.Header().Set("Retry-After", "0")
							w.WriteHeader(status)
							writeJSON(w, map[string]string{"message": "synthetic-error"})
							return
						}
						if operation == "poll" {
							if r.URL.Query().Get("lastMessageId") != "91" {
								t.Error("SDK changed cursor on refresh")
							}
							w.WriteHeader(http.StatusAccepted)
						} else if operation == "ack" {
							w.WriteHeader(http.StatusNoContent)
						} else {
							writeJSON(w, map[string]any{"count": 1, "value": []int64{42}})
						}
					}
				})
				s := newSession(t, client)
				var err error
				switch operation {
				case "poll":
					_, err = s.GetMessage(context.Background(), 91, 1)
				case "ack":
					err = s.DeleteMessage(context.Background(), 91)
				case "acquire":
					_, err = s.AcquireJobs(context.Background(), []int64{42})
				}
				if status == 401 {
					if err != nil || attempts.Load() != 2 || refreshes.Load() != 1 || s.Session().Statistics.TotalAssignedJobs != 8 {
						t.Fatal("expected one refresh and one retry")
					}
				} else if err == nil || attempts.Load() != 1 || refreshes.Load() != 0 {
					t.Fatal("expected terminal error with offline retry policy")
				}
			})
		}
	}
}

func TestSDKCapacityWithdrawalDoesNotFenceInFlightAcquisition(t *testing.T) {
	pollEntered, releasePoll := make(chan struct{}), make(chan struct{})
	var polls, acquired atomic.Int32
	_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sessions"):
			sessionReply(w, r, 1, 0)
		case r.URL.Path == "/queue":
			if polls.Add(1) == 1 {
				if r.Header.Get(scaleset.HeaderScaleSetMaxCapacity) != "1" {
					t.Error("initial capacity not 1")
				}
				close(pollEntered)
				select {
				case <-releasePoll:
				case <-r.Context().Done():
					return
				}
				messageReply(w, 10, 1, []map[string]any{{"messageType": "JobAvailable", "runnerRequestId": 42}})
			} else {
				if r.Header.Get(scaleset.HeaderScaleSetMaxCapacity) != "0" {
					t.Error("next poll did not withdraw capacity")
				}
				w.WriteHeader(http.StatusForbidden)
			}
		case r.URL.Path == "/queue/10":
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/acquirejobs"):
			acquired.Add(1)
			writeJSON(w, map[string]any{"count": 1, "value": []int64{42}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	l, _ := listener.New(newSession(t, client), listener.Config{ScaleSetID: 7, MaxRunners: 1})
	done := make(chan error, 1)
	go func() { done <- l.Run(context.Background(), callbacks{}) }()
	select {
	case <-pollEntered:
	case <-time.After(3 * time.Second):
		close(releasePoll)
		t.Fatal("poll barrier not reached")
	}
	l.SetMaxRunners(0)
	close(releasePoll)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected final poll error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not terminate")
	}
	if acquired.Load() != 1 {
		t.Fatal("expected acquisition despite capacity withdrawal")
	}
}

func TestSDKBusyRemovalSentinelAndRawErrorExposure(t *testing.T) {
	_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]string{"typeName": "JobStillRunningException", "message": "synthetic-sensitive-body"})
	})
	err := client.RemoveRunner(context.Background(), 11)
	if !errors.Is(err, scaleset.JobStillRunningError) {
		t.Fatal("busy removal sentinel missing")
	}
	if !strings.Contains(err.Error(), "synthetic-sensitive-body") {
		t.Fatal("SDK raw error exposure changed; review adapter redaction requirement")
	}
	// This only proves client decoding. The fixture chose to refuse busy deletion;
	// only a live assignment/deletion race can establish the server-side barrier.
}

type jitResponseLossFixture struct {
	t         *testing.T
	requested atomic.Int32
}

func newJITResponseLossFixture(t *testing.T) (*jitResponseLossFixture, *scaleset.Client) {
	t.Helper()
	fixture := &jitResponseLossFixture{t: t}
	_, client := newServer(t, fixture.handle)
	return fixture, client
}

func (f *jitResponseLossFixture) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/generatejitconfig"):
		f.requested.Add(1)
		dropResponse(w)
	case strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
		writeJSON(w, scaleset.RunnerScaleSet{ID: 7, Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}})
	case strings.HasSuffix(r.URL.Path, "/agents"):
		if r.URL.Query().Get("agentName") != "owned-1" {
			f.t.Error("unstable lookup identity")
			return
		}
		writeJSON(w, scaleset.RunnerReferenceList{Count: 1, RunnerReferences: []scaleset.RunnerReference{{ID: 11, Name: "owned-1", RunnerScaleSetID: 7}}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity(t *testing.T) {
	fixture, client := newJITResponseLossFixture(t)
	got, err := recoverState(context.Background(), client, recoveryState{Workers: map[string]string{"owned-1": "creating"}})
	if err != nil || fixture.requested.Load() != 0 || got.References["owned-1"] != 0 || got.Workers["owned-1"] != "quarantined" || !got.AdmissionPaused {
		t.Fatal("pre-create lookup fabricated a runner identity or changed recovery quarantine")
	}
}

func TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity(t *testing.T) {
	fixture, client := newJITResponseLossFixture(t)
	jit, err := client.GenerateJitRunnerConfig(context.Background(), &scaleset.RunnerScaleSetJitRunnerSetting{Name: "owned-1", WorkFolder: "_work"}, 7)
	if err == nil || jit != nil {
		t.Fatal("expected missing JIT response")
	}
	got, err := recoverState(context.Background(), client, recoveryState{Workers: map[string]string{"owned-1": "creating"}})
	if err != nil || fixture.requested.Load() != 1 || got.References["owned-1"] != 0 || got.Workers["owned-1"] != "quarantined" || !got.AdmissionPaused {
		t.Fatal("non-committed response loss fabricated a runner identity or changed recovery quarantine")
	}
}

func TestSDKJITResponseLossDiscoversIdentityWithoutReissuing(t *testing.T) {
	fixture, client := newJITResponseLossFixture(t)
	jit, err := client.GenerateJitRunnerConfig(context.Background(), &scaleset.RunnerScaleSetJitRunnerSetting{Name: "owned-1", WorkFolder: "_work"}, 7)
	if err == nil || jit != nil {
		t.Fatal("expected missing JIT response")
	}
	got, err := recoverState(context.Background(), client, recoveryState{Workers: map[string]string{"owned-1": "creating"}})
	if err != nil || fixture.requested.Load() != 1 || got.References["owned-1"] != 11 || got.Workers["owned-1"] != "quarantined" || !got.AdmissionPaused {
		t.Fatal("expected discovered reference and quarantine without JIT retry")
	}
}
