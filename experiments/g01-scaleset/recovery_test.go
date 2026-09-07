package contract

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
)

type callbacks struct {
	started   func(context.Context, *scaleset.JobStarted) error
	completed func(context.Context, *scaleset.JobCompleted) error
	desired   func(context.Context, int) (int, error)
}

func TestRecoveryErrorsHoldReservationsAndRedact(t *testing.T) {
	for _, fault := range []string{"statistics-error", "nil-statistics", "negative-statistics", "inventory-error", "foreign-owner", "absent-runner"} {
		t.Run(fault, func(t *testing.T) {
			_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/runnerscalesets/7") {
					if fault == "statistics-error" {
						w.WriteHeader(http.StatusForbidden)
						writeJSON(w, map[string]string{"message": "synthetic-sensitive-body"})
						return
					}
					stats := &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 2}
					if fault == "nil-statistics" {
						stats = nil
					}
					if fault == "negative-statistics" {
						stats.TotalAssignedJobs = -1
					}
					writeJSON(w, scaleset.RunnerScaleSet{ID: 7, Statistics: stats})
					return
				}
				if fault == "inventory-error" {
					w.WriteHeader(http.StatusForbidden)
					writeJSON(w, map[string]string{"message": "synthetic-sensitive-body"})
					return
				}
				refs := scaleset.RunnerReferenceList{}
				if fault == "foreign-owner" {
					refs.Count = 1
					refs.RunnerReferences = []scaleset.RunnerReference{{ID: 11, Name: "owned-1", RunnerScaleSetID: 99}}
				}
				writeJSON(w, refs)
			})
			initial := recoveryState{Desired: 4, Workers: map[string]string{"owned-1": "ready"}}
			got, err := recoverState(context.Background(), client, initial)
			if (err == nil) != (fault == "absent-runner") {
				t.Fatal("unexpected recovery outcome")
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-sensitive-body") {
				t.Fatal("secret-bearing error escaped")
			}
			if !got.AdmissionPaused || got.Workers["owned-1"] != "quarantined" || len(got.Workers) != 1 {
				t.Fatal("unresolved worker lost reservation")
			}
			if initial.Workers["owned-1"] != "ready" {
				t.Fatal("recovery mutated its input snapshot")
			}
		})
	}
}

func (c callbacks) HandleJobStarted(ctx context.Context, j *scaleset.JobStarted) error {
	if c.started != nil {
		return c.started(ctx, j)
	}
	return nil
}
func (c callbacks) HandleJobCompleted(ctx context.Context, j *scaleset.JobCompleted) error {
	if c.completed != nil {
		return c.completed(ctx, j)
	}
	return nil
}
func (c callbacks) HandleDesiredRunnerCount(ctx context.Context, n int) (int, error) {
	if c.desired != nil {
		return c.desired(ctx, n)
	}
	return n, nil
}

func TestRecoveryAfterACKCallbackCrash(t *testing.T) {
	var acked atomic.Bool
	_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sessions"):
			sessionReply(w, r, 1, 0)
		case r.URL.Path == "/queue" && r.Method == http.MethodGet:
			if acked.Load() {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			messageReply(w, 10, 4, []map[string]any{{"messageType": "JobStarted", "runnerRequestId": 42, "runnerName": "owned-1"}})
		case r.URL.Path == "/queue/10" && r.Method == http.MethodDelete:
			acked.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
			writeJSON(w, scaleset.RunnerScaleSet{ID: 7, Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 4}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	session := newSession(t, client)
	l, err := listener.New(session, listener.Config{ScaleSetID: 7, MaxRunners: 5})
	if err != nil {
		t.Fatal(err)
	}
	// Crash barrier: callback observes the completed HTTP DELETE but returns
	// before recording either the job lifecycle or desired capacity.
	err = l.Run(context.Background(), callbacks{started: func(context.Context, *scaleset.JobStarted) error {
		if !acked.Load() {
			t.Error("callback ran before ACK")
		}
		return errCrash
	}})
	if !errors.Is(err, errCrash) {
		t.Fatal("crash was not reached")
	}
	msg, err := session.GetMessage(context.Background(), 0, 5)
	if err != nil || msg != nil {
		t.Fatal("fixture should no longer deliver acknowledged message")
	}
	got, err := recoverState(context.Background(), client, recoveryState{})
	if err != nil {
		t.Fatal("recovery failed")
	}
	if got.Desired != 4 {
		t.Fatalf("desired capacity stranded after ACK/callback crash: got %d, want 4", got.Desired)
	}
}

func TestRecoveryMissingLifecycleCallback(t *testing.T) {
	_, client := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
			writeJSON(w, scaleset.RunnerScaleSet{ID: 7, Statistics: &scaleset.RunnerScaleSetStatistic{}})
		case strings.HasSuffix(r.URL.Path, "/agents"):
			writeJSON(w, scaleset.RunnerReferenceList{Count: 1, RunnerReferences: []scaleset.RunnerReference{{ID: 11, Name: "owned-1", RunnerScaleSetID: 7}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	// This is the surviving creation intent. A lost JobStarted callback leaves
	// the old "ready" record; aggregate zero is not proof of worker idleness.
	got, err := recoverState(context.Background(), client, recoveryState{Workers: map[string]string{"owned-1": "ready"}})
	if err != nil {
		t.Fatal("recovery failed")
	}
	if got.Workers["owned-1"] != "quarantined" {
		t.Fatalf("missing lifecycle callback left worker %q; want quarantined", got.Workers["owned-1"])
	}
}
