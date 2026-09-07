package livecanary

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBaselineDurableReceiptAndScope(t *testing.T) {
	f := newBaselineFixture(t, nil)
	b, err := newBaselineListenerHeld(context.Background(), f.a, f.j, f.api, 7)
	if err != nil {
		t.Fatal(err)
	}
	if f.requests.Load() != 0 {
		t.Fatal("constructor contacted server")
	}
	var receipt baselineAcquisition
	err = b.run(func(ctx context.Context, r baselineAcquisition) error {
		f.continuations.Add(1)
		receipt = r
		// Calling actual SDK Session here would deadlock if the continuation ran from
		// the acquire transport hook while the SDK held its session mutex.
		if b.session.Session().SessionID.String() != r.SessionID {
			t.Fatal("session mutex/identity")
		}
		events := f.j.Events()
		want := []struct {
			ref   controllerRecordRef
			stage string
		}{{r.WholeBatch, "poll"}, {r.SourceCheck, "source"}, {r.ACK, "ack"}, {r.Intent, "acquire"}, {r.Result, "acquire"}}
		for _, v := range want {
			if v.ref.Sequence <= 0 || v.ref.Sequence > len(events) {
				t.Fatal("unassigned receipt")
			}
			e := events[v.ref.Sequence-1]
			if controllerEventRef(b.identity, e) != v.ref || e.Baseline.Stage != v.stage {
				t.Fatal("receipt not linked to actual assigned evidence")
			}
		}
		if r.WholeBatch.Sequence >= r.SourceCheck.Sequence || r.SourceCheck.Sequence >= r.ACK.Sequence || r.ACK.Sequence >= r.Intent.Sequence || r.Intent.Sequence >= r.Result.Sequence {
			t.Fatal("out-of-order acquisition")
		}
		if r.RequestID != 42 || r.JobID != "opaque-job-not-a-REST-id" || r.AvailableIndex != 0 {
			t.Fatal("ID domains conflated")
		}
		if _, e := b.AcquireJobs(ctx, []int64{42}); e == nil {
			t.Fatal("reentrant acquisition")
		}
		if b.DeleteMessage(ctx, 9) == nil {
			t.Fatal("reentrant ACK")
		}
		events[r.WholeBatch.Sequence-1].Baseline.Batch.Items[0].JobID = "mutated"
		r.Result.Sequence = 0
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Result.Sequence == 0 || f.continuations.Load() != 1 || f.forbidden.Load() != 0 {
		t.Fatal("receipt/call count")
	}
	events := f.j.Events()
	var wireOrder, callbackOrder []string
	for _, e := range events {
		if r := e.Baseline; r != nil {
			if r.Batch != nil && r.Batch.MessageID == 10 {
				for _, x := range r.Batch.Items {
					wireOrder = append(wireOrder, x.Kind)
					if x.Kind == "JobStarted" && baselineValue(x.RunnerID) != 81 {
						t.Fatal("SDK runner observation")
					}
				}
			}
			if r.Stage == "started" || r.Stage == "completed" {
				callbackOrder = append(callbackOrder, r.Stage)
				batch := events[r.BatchRef.Sequence-1].Baseline.Batch
				if batch.Items[*r.ItemIndex].Kind != map[string]string{"started": "JobStarted", "completed": "JobCompleted"}[r.Stage] {
					t.Fatal("callback lost original index")
				}
			}
		}
	}
	if !reflect.DeepEqual(wireOrder, []string{"JobAssigned", "JobCompleted", "JobStarted"}) || !reflect.DeepEqual(callbackOrder, []string{"started", "completed"}) {
		t.Fatal("wire order replaced by callback grouping")
	}
	if events[receipt.WholeBatch.Sequence-1].Baseline.Batch.Items[0].JobID != receipt.JobID {
		t.Fatal("mutable event escaped")
	}
	before := f.requests.Load()
	if b.run(func(context.Context, baselineAcquisition) error { return nil }) == nil {
		t.Fatal("second run")
	}
	if _, err = b.GetMessage(context.Background(), 10, 1); err == nil {
		t.Fatal("retained receiver")
	}
	if _, err = b.AcquireJobs(context.Background(), []int64{42}); err == nil {
		t.Fatal("retained acquisition")
	}
	if f.requests.Load() != before {
		t.Fatal("late effect")
	}
	data, err := os.ReadFile(filepath.Join(f.j.directory, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(receipt)
	data = append(data, encoded...)
	for _, secret := range []string{"synthetic-secret-queue", "synthetic-secret-acquire", "synthetic-secret-display", f.api.credentials.InstallationToken, f.api.credentials.VerificationToken, f.api.baseURL, "acquireJobUrl", "messageQueue"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("secret or private endpoint persisted")
		}
	}
	if b.session == nil {
		t.Fatal("known session not retained for future owner")
	}
}

func TestBaselineKnownResponseSurvivesOriginalCancellation(t *testing.T) {
	for _, stage := range []string{"set", "session", "poll1", "source", "ack", "acquire"} {
		t.Run(stage, func(t *testing.T) {
			f := newBaselineFixture(t, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			triggered := false
			f.afterResponse = func(r *http.Request, response *http.Response) {
				match := stage == "set" && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7") || stage == "session" && strings.HasSuffix(r.URL.Path, "/sessions") || stage == "poll1" && r.URL.Path == "/queue" || stage == "source" && strings.Contains(r.URL.Path, "/actions/runs/") || stage == "ack" && r.Method == "DELETE" || stage == "acquire" && strings.HasSuffix(r.URL.Path, "/acquirejobs")
				if match && !triggered {
					triggered = true
					cancel()
				}
			}
			b, err := newBaselineListenerHeld(ctx, f.a, f.j, f.api, 7)
			if err != nil {
				t.Fatal(err)
			}
			err = b.run(func(context.Context, baselineAcquisition) error { f.continuations.Add(1); return nil })
			if err == nil || !triggered || f.continuations.Load() != 0 {
				t.Fatal("cancelled authority continued")
			}
			want := stage
			if stage == "set" {
				want = "set-observe"
			}
			if stage == "session" {
				want = "session-open"
			}
			if stage == "poll1" {
				want = "poll"
			}
			last := baselineLast(t, f)
			if last.Stage != want {
				t.Fatalf("wrong boundary: %s", last.Stage)
			}
			// Source is read by the unmodified REST decoder after cancellation, so its
			// response may be unknown. The pre-decoded SDK stages have known responses.
			if stage != "source" && last.Outcome != "result" {
				t.Fatalf("known %s response lost: %s", stage, last.Outcome)
			}
			wantACK, wantAcquire := int32(0), int32(0)
			if stage == "ack" || stage == "acquire" {
				wantACK = 1
			}
			if stage == "acquire" {
				wantAcquire = 1
			}
			if f.acks.Load() != wantACK || f.acquires.Load() != wantAcquire {
				t.Fatal("SDK WithoutCancel extended authority")
			}
		})
	}
}
func TestBaselineResponseLossAndNoReplay(t *testing.T) {
	for _, stage := range []string{"session", "ack", "acquire"} {
		for _, failure := range []string{"lost", "401", "write", "closed-file"} {
			t.Run(stage+"/"+failure, func(t *testing.T) {
				var f *baselineFixture
				f = newBaselineFixture(t, func(at string, v any) any {
					if at != stage {
						return v
					}
					switch failure {
					case "lost":
						return baselineReply{lost: true}
					case "401":
						return baselineReply{status: 401}
					case "closed-file":
						_ = f.j.file.Close()
					case "write":
						f.j.syncFile = func(file *os.File) error {
							if err := file.Sync(); err != nil {
								return err
							}
							return errors.New("synthetic fsync failure")
						}
					}
					return v
				})
				err := f.run(t)
				if err == nil || f.continuations.Load() != 0 || f.sessions.Load() != 1 || f.forbidden.Load() != 0 {
					t.Fatalf("lost effect: err=%v sessions=%d forbidden=%d", err, f.sessions.Load(), f.forbidden.Load())
				}
				wantACK, wantAcquire := int32(0), int32(0)
				if stage == "ack" || stage == "acquire" {
					wantACK = 1
				}
				if stage == "acquire" {
					wantAcquire = 1
				}
				if f.acks.Load() != wantACK || f.acquires.Load() != wantAcquire {
					t.Fatalf("automatic retry: ACK=%d acquire=%d", f.acks.Load(), f.acquires.Load())
				}
				before := f.requests.Load()
				dir := f.j.directory
				f.release()
				f.release = nil
				_ = f.j.Close()
				reopened, err := openTestJournal(t, dir, f.a)
				if err != nil {
					t.Fatal("intact uncertain history did not reopen:", err)
				}
				f.j = reopened
				release, err := f.j.authorize(f.a)
				if err != nil {
					t.Fatal(err)
				}
				f.release = release
				if _, err = newBaselineListenerHeld(context.Background(), f.a, f.j, f.api, 7); err == nil {
					t.Fatal("reopened baseline allowed replay")
				}
				if !replay(f.j.Events()).uncertain || !replay(f.j.Events()).reserved || !replay(f.j.Events()).workObserved {
					t.Fatal("generic cleanup fence cleared")
				}
				if f.requests.Load() != before {
					t.Fatal("reopen contacted remote")
				}
			})
		}
	}
}

func TestBaselineNilPollBudgetAndUnknowns(t *testing.T) {
	f := newBaselineFixture(t, func(stage string, v any) any {
		if strings.HasPrefix(stage, "poll") && !strings.HasSuffix(stage, "-items") {
			return baselineReply{status: 202}
		}
		return v
	})
	baselineRefuses(t, f, 0, 0, 0)
	if f.polls.Load() != 16 {
		t.Fatalf("poll budget: %d", f.polls.Load())
	}
	for _, e := range f.j.Events() {
		r := e.Baseline
		if r != nil && r.Stage == "poll" && r.Outcome == "result" && (!r.NoMessage || r.Batch != nil) {
			t.Fatal("nil poll invented message/statistics")
		}
	}
}
func TestBaselineCapturedAuthorityAndInvalidInputs(t *testing.T) {
	for _, name := range []string{"approval", "credential", "verification", "no-create", "context"} {
		t.Run(name, func(t *testing.T) {
			f := newBaselineFixture(t, nil)
			ctx := context.Background()
			switch name {
			case "approval":
				f.api.approval.WorkflowRunID++
			case "credential":
				f.api.credentials.AppID++
			case "verification":
				f.api.credentials.VerificationToken = f.api.credentials.InstallationToken
			case "no-create":
				f.j.events = f.j.events[:1]
			case "context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := newBaselineListenerHeld(ctx, f.a, f.j, f.api, 7); err == nil || f.requests.Load() != 0 {
				t.Fatal("invalid constructor authority admitted")
			}
		})
	}
	f := newBaselineFixture(t, nil)
	b, err := newBaselineListenerHeld(context.Background(), f.a, f.j, f.api, 7)
	if err != nil {
		t.Fatal(err)
	}
	f.api.approval.WorkflowRunID++
	f.api.credentials.VerificationToken = "changed"
	f.api.options = nil
	f.a.Phases[0] = "changed"
	if err = b.run(func(context.Context, baselineAcquisition) error { return nil }); err != nil {
		t.Fatal("outside input mutation altered captured authority", err)
	}
}
func TestBaselineCloseWaitsForHeldScope(t *testing.T) {
	f := newBaselineFixture(t, nil)
	b := baselineStepped(t, f)
	done := make(chan error, 1)
	go func() { done <- f.j.Close() }()
	select {
	case <-done:
		t.Fatal("Close released held controller lease")
	case <-time.After(25 * time.Millisecond):
	}
	baselineAcquireFirst(t, b)
	f.release()
	f.release = nil
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not release after scope")
	}
	before := f.requests.Load()
	if _, err := b.GetMessage(context.Background(), 9, 1); err == nil || f.requests.Load() != before {
		t.Fatal("closed receiver performed work")
	}
}

func TestBaselineLoneAliasesAgreeWithSDK(t *testing.T) {
	f := newBaselineFixture(t, func(stage string, v any) any {
		if stage == "poll1-items" {
			m := v.([]any)[0].(map[string]any)
			m["RUNNERREQUESTID"] = m["runnerRequestId"]
			delete(m, "runnerRequestId")
		}
		if stage == "acquire" {
			m := v.(map[string]any)
			m["COUNT"] = m["count"]
			delete(m, "count")
		}
		return v
	})
	if err := f.run(t); err != nil || f.acquires.Load() != 1 || f.acks.Load() != 2 {
		t.Fatal("single alias disagreed with pinned SDK", err)
	}
}

func TestBaselineOriginalContextCancelsInflightSDK(t *testing.T) {
	for _, stage := range []string{"poll1", "source", "ack", "acquire"} {
		t.Run(stage, func(t *testing.T) {
			entered := make(chan struct{})
			unblock := make(chan struct{})
			f := newBaselineFixture(t, func(at string, v any) any {
				if at == stage {
					close(entered)
					<-unblock
				}
				return v
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b, err := newBaselineListenerHeld(ctx, f.a, f.j, f.api, 7)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				done <- b.run(func(context.Context, baselineAcquisition) error { f.continuations.Add(1); return nil })
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				close(unblock)
				t.Fatal("request boundary not reached")
			}
			cancel()
			select {
			case err := <-done:
				close(unblock)
				if err == nil || f.continuations.Load() != 0 {
					t.Fatal("cancelled in-flight operation continued")
				}
			case <-time.After(time.Second):
				close(unblock)
				t.Fatal("SDK WithoutCancel escaped original scope")
			}
		})
	}
}

// Independent reviewer reproduction: synchronous mutation after the real SDK response.
func TestBaselineAcquisitionCapturesRequestSlice(t *testing.T) {
	f := newBaselineFixture(t, nil)
	b := baselineStepped(t, f)
	m, err := b.GetMessage(context.Background(), 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.DeleteMessage(context.Background(), m.MessageID); err != nil {
		t.Fatal(err)
	}
	ids := []int64{42}
	changed := false
	f.afterResponse = func(r *http.Request, response *http.Response) {
		if strings.HasSuffix(r.URL.Path, "/acquirejobs") {
			ids[0] = 43
			changed = true
		}
	}
	got, err := b.AcquireJobs(context.Background(), ids)
	if !changed {
		t.Fatal("actual acquired-response boundary not reached")
	}
	if err != nil || len(got) != 1 || got[0] != 42 || f.acquires.Load() != 1 || f.continuations.Load() != 1 {
		t.Fatalf("caller slice changed captured request: error=%v result=%v acquire=%d continuation=%d", err, got, f.acquires.Load(), f.continuations.Load())
	}
	last := baselineLast(t, f)
	if last.Stage != "continuation" || last.Outcome != "result" {
		t.Fatal("durable singleton continuation lost")
	}
}
