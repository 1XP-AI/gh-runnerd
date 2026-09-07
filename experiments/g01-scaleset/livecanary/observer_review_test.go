package livecanary

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestObserveRejectsIndependentBaseFork(t *testing.T) {
	for _, reader := range []string{"observer", "verify-run"} {
		for _, repository := range []string{"neither", "repository", "head_repository"} {
			t.Run(reader+"/"+repository, func(t *testing.T) {
				a := approval()
				var calls atomic.Int32
				api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if !strings.HasSuffix(r.URL.Path, "/runs/5") {
						t.Error("fork contradiction allowed a downstream read")
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					run := observationRun(a)
					if repository != "neither" {
						run[repository].(map[string]any)["fork"] = true
					}
					_ = json.NewEncoder(w).Encode(run)
				})
				var err error
				if reader == "observer" {
					_, err = api.observeApprovedRun(context.Background())
				} else {
					err = api.VerifyRun(context.Background(), a, a.WorkflowRunID)
				}
				if (err == nil) != (repository == "neither") || calls.Load() != 1 {
					t.Fatalf("source fork policy mismatch: error=%v calls=%d", err, calls.Load())
				}
			})
		}
	}
}

func TestObserveJobAttemptRequiresDetailCorroboration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		list       any
		detail     any
		omitList   bool
		omitDetail bool
		good       bool
		calls      int32
	}{
		{name: "explicit", list: 1, detail: 1, good: true, calls: 3},
		{name: "list-omitted", omitList: true, detail: 1, good: true, calls: 3},
		{name: "list-null", detail: 1, good: true, calls: 3},
		{name: "list-zero", list: 0, detail: 1, calls: 2},
		{name: "list-other", list: 2, detail: 1, calls: 2},
		{name: "list-string", list: "synthetic-secret", detail: 1, calls: 2},
		{name: "detail-omitted", list: 1, omitDetail: true, calls: 3},
		{name: "detail-null", list: 1, calls: 3},
		{name: "detail-zero", list: 1, detail: 0, calls: 3},
		{name: "detail-other", list: 1, detail: 2, calls: 3},
		{name: "detail-negative", list: 1, detail: -1, calls: 3},
		{name: "detail-string", list: 1, detail: "synthetic-secret", calls: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := approval()
			var calls atomic.Int32
			api := observationFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body any
				switch {
				case strings.HasSuffix(r.URL.Path, "/runs/5"):
					body = observationRun(a)
				case strings.HasSuffix(r.URL.Path, "/runs/5/attempts/1/jobs"):
					job := observationJob(a)
					job["run_attempt"] = tc.list
					if tc.omitList {
						delete(job, "run_attempt")
					}
					body = map[string]any{"total_count": 1, "jobs": []any{job}}
				case strings.HasSuffix(r.URL.Path, "/jobs/9007199254740995"):
					job := observationJob(a)
					job["run_attempt"] = tc.detail
					if tc.omitDetail {
						delete(job, "run_attempt")
					}
					body = job
				default:
					t.Error("unexpected attempt endpoint")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				_ = json.NewEncoder(w).Encode(body)
			})
			got, err := api.observeRESTJob(context.Background(), 1, 0)
			if (err == nil) != tc.good || calls.Load() != tc.calls {
				t.Fatalf("job attempt policy mismatch: error=%v calls=%d", err, calls.Load())
			}
			if tc.good && (got.Attempt != 1 || got.Response.Outcome != observationPresent) {
				t.Fatal("corroborated attempt facts lost")
			}
			if !tc.good && (got.Attempt != 0 || got.Response.Outcome == observationPresent) {
				t.Fatal("uncorroborated attempt became a reported fact")
			}
			assertObservationSecretSafe(t, got, err)
		})
	}
}
