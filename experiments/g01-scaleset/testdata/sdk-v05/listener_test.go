package contract

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
)

func TestV05HandlerFailureLeavesMessageUnacked(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-effects", true: "partial-effects"}[partial], func(t *testing.T) {
			var acks atomic.Int32
			_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/sessions"):
					sessionReply(w, r, 1, 0)
				case r.Method == http.MethodGet && r.URL.Path == "/queue":
					if acks.Load() > 0 {
						w.WriteHeader(http.StatusAccepted)
						return
					}
					messageReply(w, 10, 4, []map[string]any{{"messageType": "JobStarted", "runnerRequestId": 42}})
				case r.Method == http.MethodDelete:
					acks.Add(1)
					w.WriteHeader(http.StatusNoContent)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			session := newSession(t, client)
			ctx, _ := testContext(t)
			calls, effects := 0, 0
			handler := scaleFunc(func(_ context.Context, msg *scaleset.RunnerScaleSetMessage) error {
				if msg.MessageID == listener.InitialMessageID {
					return nil
				}
				calls++
				if partial {
					effects++
				}
				return errStop
			})
			if !errors.Is(newListener(t, session, 4).Run(ctx, handler), errStop) || acks.Load() != 0 || calls != 1 {
				t.Fatal("handler failure must precede ACK; message must remain unacked")
			}
			msg, err := session.GetMessage(ctx, 0, 4)
			if err != nil || msg == nil || msg.MessageID != 10 {
				t.Fatal("unacked fixture message was not retained")
			}
			// A fresh Run sees the fixture message again. This is intentionally NOT
			// a durable handler: a partial effect repeats, proving the limitation.
			if !errors.Is(newListener(t, session, 4).Run(ctx, handler), errStop) || calls != 2 || acks.Load() != 0 {
				t.Fatal("fixture replay did not re-enter the handler without ACK")
			}
			if effects != map[bool]int{false: 0, true: 2}[partial] {
				t.Fatal("partial-effect replay control failed")
			}
		})
	}
}

func TestV05SuccessfulCanceledHandlerStillACKs(t *testing.T) {
	var acks atomic.Int32
	_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sessions"):
			sessionReply(w, r, 1, 0)
		case r.Method == http.MethodGet:
			messageReply(w, 10, 1, nil)
		case r.Method == http.MethodDelete:
			acks.Add(1)
			w.WriteHeader(http.StatusNoContent)
		}
	})
	ctx, cancel := testContext(t)
	err := newListener(t, newSession(t, client), 1).Run(ctx, scaleFunc(func(handlerCtx context.Context, msg *scaleset.RunnerScaleSetMessage) error {
		if msg.MessageID == listener.InitialMessageID {
			return nil
		}
		if acks.Load() != 0 || handlerCtx.Err() != nil {
			t.Error("handler did not precede ACK under original context")
		}
		cancel()
		return nil
	}))
	if !errors.Is(err, context.Canceled) || acks.Load() != 1 {
		t.Fatal("successful handler did not ACK independently of caller cancellation")
	}
}

func TestV05ACKFailureKeepsSideEffectAmbiguity(t *testing.T) {
	for _, mode := range []string{"denied", "lost-before-commit", "lost-after-commit"} {
		t.Run(mode, func(t *testing.T) {
			var attempts atomic.Int32
			var committed atomic.Bool
			_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/sessions"):
					sessionReply(w, r, 1, 0)
				case r.Method == http.MethodGet:
					if committed.Load() {
						w.WriteHeader(http.StatusAccepted)
						return
					}
					messageReply(w, 10, 1, nil)
				case r.Method == http.MethodDelete:
					attempts.Add(1)
					if mode == "denied" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					committed.Store(mode == "lost-after-commit")
					dropResponse(w)
				}
			})
			session := newSession(t, client)
			ctx, _ := testContext(t)
			effects := 0
			err := newListener(t, session, 1).Run(ctx, scaleFunc(func(_ context.Context, msg *scaleset.RunnerScaleSetMessage) error {
				if msg.MessageID != listener.InitialMessageID {
					effects++
				}
				return nil
			}))
			if err == nil || effects != 1 || attempts.Load() != 1 {
				t.Fatal("expected one completed handler and one failed/uncertain ACK")
			}
			msg, err := session.GetMessage(ctx, 0, 1)
			if err != nil || (msg == nil) != committed.Load() {
				t.Fatal("unexpected controlled fixture retention")
			}
		})
	}
}

func TestV05AcquisitionAmbiguityBeforeACK(t *testing.T) {
	for _, mode := range []string{"not-attempted", "uncommitted", "committed"} {
		t.Run(mode, func(t *testing.T) {
			var attempts, acks atomic.Int32
			var acquired atomic.Bool
			_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/sessions"):
					sessionReply(w, r, 1, 0)
				case r.URL.Path == "/queue":
					messageReply(w, 10, 1, []map[string]any{{"messageType": "JobAvailable", "runnerRequestId": 42}})
				case r.Method == http.MethodDelete:
					acks.Add(1)
					w.WriteHeader(http.StatusNoContent)
				case strings.HasSuffix(r.URL.Path, "/acquirejobs"):
					attempts.Add(1)
					acquired.Store(mode == "committed")
					dropResponse(w)
				case strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
					writeJSON(w, scaleset.RunnerScaleSet{ID: 7, Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}})
				case strings.HasSuffix(r.URL.Path, "/agents"):
					writeJSON(w, scaleset.RunnerReferenceList{})
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			session := newSession(t, client)
			ctx, _ := testContext(t)
			err := newListener(t, session, 1).Run(ctx, scaleFunc(func(ctx context.Context, msg *scaleset.RunnerScaleSetMessage) error {
				if msg.MessageID == listener.InitialMessageID {
					return nil
				}
				if mode == "not-attempted" {
					return errStop
				}
				_, err := session.AcquireJobs(ctx, []int64{msg.JobAvailableMessages[0].RunnerRequestID})
				return err
			}))
			want := int32(1)
			if mode == "not-attempted" {
				want = 0
			}
			if err == nil || attempts.Load() != want || acks.Load() != 0 || acquired.Load() != (mode == "committed") {
				t.Fatal("acquisition ambiguity was hidden or ACKed")
			}
			msg, err := session.GetMessage(ctx, 0, 1)
			if err != nil || msg == nil {
				t.Fatal("unacked acquisition fixture did not retain its message")
			}
			state, err := recoverState(ctx, client, recoveryState{Workers: map[string]string{"owned-1": "creating"}})
			if err != nil || !state.AdmissionPaused || state.Workers["owned-1"] != "quarantined" || len(state.References) != 0 {
				t.Fatal("ambiguous acquisition released a reservation")
			}
		})
	}
}

func TestV05CallerOwnsAcquisitionAndUsesTotalDemand(t *testing.T) {
	for _, count := range []int{0, 50, 51} {
		t.Run(map[int]string{0: "no-caller-acquire", 50: "documented-batch", 51: "adversarial-batch"}[count], func(t *testing.T) {
			var acquired, acks atomic.Int32
			_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/sessions"):
					sessionReply(w, r, 1, 0)
				case r.Method == http.MethodGet:
					n := count
					if n == 0 {
						n = 1
					}
					events := make([]map[string]any, n)
					for i := range events {
						events[i] = map[string]any{"messageType": "JobAvailable", "runnerRequestId": i + 1}
					}
					messageReply(w, 10, 125, events)
				case strings.HasSuffix(r.URL.Path, "/acquirejobs"):
					var ids []int64
					if json.NewDecoder(r.Body).Decode(&ids) != nil {
						t.Error("invalid acquisition body")
					}
					acquired.Add(int32(len(ids)))
					writeJSON(w, map[string]any{"count": 1, "value": []int64{1}})
				case r.Method == http.MethodDelete:
					acks.Add(1)
					w.WriteHeader(http.StatusNoContent)
				}
			})
			session := newSession(t, client)
			ctx, cancel := testContext(t)
			desired := 0
			err := newListener(t, session, 125).Run(ctx, scaleFunc(func(ctx context.Context, msg *scaleset.RunnerScaleSetMessage) error {
				if msg.MessageID == listener.InitialMessageID {
					return nil
				}
				desired = msg.Statistics.TotalAssignedJobs
				if count > 0 {
					ids := make([]int64, len(msg.JobAvailableMessages))
					for i, job := range msg.JobAvailableMessages {
						ids[i] = job.RunnerRequestID
					}
					got, err := session.AcquireJobs(ctx, ids)
					if err != nil || !reflect.DeepEqual(got, []int64{1}) {
						t.Error("partial acquisition mismatch")
					}
				}
				cancel()
				return nil
			}))
			if !errors.Is(err, context.Canceled) || desired != 125 || acquired.Load() != int32(count) || acks.Load() != 1 {
				t.Fatal("caller acquisition or total-demand contract changed")
			}
		})
	}
}

func TestV05NilStatisticsPolicyBelongsToCaller(t *testing.T) {
	for _, cache := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown-on-nil", true: "caller-cache"}[cache], func(t *testing.T) {
			var polls, acks atomic.Int32
			_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/sessions"):
					sessionReply(w, r, 1, 0)
				case r.Method == http.MethodDelete:
					acks.Add(1)
					w.WriteHeader(http.StatusNoContent)
				default:
					n := int(polls.Add(1))
					if n == 4 {
						w.WriteHeader(http.StatusAccepted)
						return
					}
					demand := 3
					if n == 3 {
						demand = 1
					}
					messageReply(w, n, demand, nil)
				}
			})
			ctx, _ := testContext(t)
			var seen []int
			last := -1
			nilCalls := 0
			err := newListener(t, newSession(t, client), 4).Run(ctx, scaleFunc(func(_ context.Context, msg *scaleset.RunnerScaleSetMessage) error {
				if msg == nil {
					nilCalls++
					if !cache {
						last = -1
					}
				} else {
					last = msg.Statistics.TotalAssignedJobs
				}
				seen = append(seen, last)
				if len(seen) == 5 {
					return errStop
				}
				return nil
			}))
			want := []int{0, 3, 3, 1, -1}
			if cache {
				want[4] = 1
			}
			if !errors.Is(err, errStop) || nilCalls != 1 || acks.Load() != 3 || !reflect.DeepEqual(seen, want) {
				t.Fatal("nil-message/caller-cache evidence changed")
			}
		})
	}
}

func TestV05HandlerCancellationPreventsACK(t *testing.T) {
	var acks atomic.Int32
	_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sessions"):
			sessionReply(w, r, 1, 0)
		case r.Method == http.MethodGet:
			messageReply(w, 10, 1, nil)
		default:
			acks.Add(1)
			w.WriteHeader(http.StatusNoContent)
		}
	})
	ctx, cancel := testContext(t)
	entered := make(chan struct{})
	done := make(chan error, 1)
	l := newListener(t, newSession(t, client), 1)
	go func() {
		done <- l.Run(ctx, scaleFunc(func(handlerCtx context.Context, msg *scaleset.RunnerScaleSetMessage) error {
			if msg.MessageID == listener.InitialMessageID {
				return nil
			}
			close(entered)
			<-handlerCtx.Done()
			return handlerCtx.Err()
		}))
	}()
	await(t, entered)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || acks.Load() != 0 {
			t.Fatal("canceled handler was ACKed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler cancellation did not terminate")
	}
}

func TestV05CapacityWithdrawalNeedsCallerFence(t *testing.T) {
	for _, fence := range []bool{false, true} {
		t.Run(map[bool]string{false: "unfenced", true: "caller-fence"}[fence], func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			var polls, acquired, acks atomic.Int32
			var draining atomic.Bool
			_, client, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/sessions"):
					sessionReply(w, r, 1, 0)
				case r.URL.Path == "/queue":
					if polls.Add(1) == 1 {
						if r.Header.Get(scaleset.HeaderScaleSetMaxCapacity) != "1" {
							t.Error("initial capacity mismatch")
						}
						close(entered)
						select {
						case <-release:
						case <-r.Context().Done():
							return
						}
						messageReply(w, 10, 1, []map[string]any{{"messageType": "JobAvailable", "runnerRequestId": 42}})
					} else {
						if r.Header.Get(scaleset.HeaderScaleSetMaxCapacity) != "0" {
							t.Error("withdrawn capacity not advertised")
						}
						w.WriteHeader(http.StatusForbidden)
					}
				case strings.HasSuffix(r.URL.Path, "/acquirejobs"):
					acquired.Add(1)
					writeJSON(w, map[string]any{"count": 1, "value": []int64{42}})
				case r.Method == http.MethodDelete:
					acks.Add(1)
					w.WriteHeader(http.StatusNoContent)
				}
			})
			session := newSession(t, client)
			l := newListener(t, session, 1)
			ctx, _ := testContext(t)
			done := make(chan error, 1)
			go func() {
				done <- l.Run(ctx, scaleFunc(func(ctx context.Context, msg *scaleset.RunnerScaleSetMessage) error {
					if msg.MessageID == listener.InitialMessageID {
						return nil
					}
					if fence && draining.Load() {
						return errStop
					}
					_, err := session.AcquireJobs(ctx, []int64{42})
					return err
				}))
			}()
			await(t, entered)
			draining.Store(true)
			l.SetMaxRunners(0)
			releaseOnce.Do(func() { close(release) })
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("expected bounded termination")
				}
			case <-ctx.Done():
				t.Fatal("drain probe did not terminate")
			}
			want := int32(1)
			if fence {
				want = 0
			}
			if acquired.Load() != want || acks.Load() != want {
				t.Fatal("caller-fence/in-flight acquisition evidence changed")
			}
		})
	}
}
