//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
)

func replacementResponse(a Approval, variant string) func(string, any) any {
	opens := 0
	return func(stage string, value any) any {
		if stage == "set" {
			x := value.(scaleset.RunnerScaleSet)
			x.Labels[0].Type = "System"
			return x
		}
		if stage == "session" {
			opens++
			if variant == "open2-loss" && opens == 2 {
				return baselineReply{lost: true}
			}
			x := value.(scaleset.RunnerScaleSetSession)
			x.SessionID = uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", opens))
			x.MessageQueueAccessToken = fmt.Sprintf("synthetic-queue-generation-%d", opens)
			x.RunnerScaleSet = &scaleset.RunnerScaleSet{ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID, Labels: []scaleset.Label{{Name: a.setName(), Type: "System"}}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true}, Statistics: &scaleset.RunnerScaleSetStatistic{}}
			if opens == 2 {
				switch variant {
				case "same-uuid":
					x.SessionID = uuid.MustParse("00000000-0000-4000-8000-000000000001")
				case "owner-drift":
					x.OwnerName = "foreign"
				case "queue-origin":
					x.MessageQueueURL = "https://invalid.example/queue"
				case "seeded-query":
					x.MessageQueueURL += "?lastMessageId=41"
				case "nested-drift":
					x.RunnerScaleSet.Name = "foreign"
				}
			}
			return x
		}
		if strings.HasPrefix(stage, "poll") && strings.HasSuffix(stage, "-items") {
			item := baselineFixtureItem(a, "JobAvailable")
			item["requestLabels"] = []string{a.setName()}
			item["jobWorkflowRef"] = a.WorkflowRef
			return []any{item}
		}
		if stage == "poll1" || stage == "poll2" {
			if stage == "poll2" && variant == "new202" {
				return baselineReply{status: 202}
			}
			if stage == "poll1" && variant == "old401" {
				return baselineReply{status: 401}
			}
			x := value.(map[string]any)
			x["messageId"] = 41
			x["statistics"] = scaleset.RunnerScaleSetStatistic{TotalAvailableJobs: 1}
			if stage == "poll1" {
				switch variant {
				case "zero":
					x["messageId"] = 0
				case "missing":
					delete(x, "messageId")
				case "null":
					x["messageId"] = nil
				}
			}
			return x
		}
		return value
	}
}

type replacementTrace struct {
	t                    *testing.T
	base                 *baselineFixture
	mu                   sync.Mutex
	ops                  []string
	closes, opens, polls int
	complete             bool
	variant              string
}

func attachReplacementTrace(t *testing.T, base *baselineFixture, variant string) *replacementTrace {
	f := &replacementTrace{t: t, base: base, variant: variant}
	base.beforeRequest = f.before
	base.extra = f.extra
	return f
}
func (f *replacementTrace) before(w http.ResponseWriter, r *http.Request) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.complete && !r.Close {
		f.t.Error("physical request permits hidden reused-connection retry")
	}
	op := ""
	switch {
	case strings.HasSuffix(r.URL.Path, "/runnerscalesets/7") && r.Method == "GET":
		op = "set"
	case strings.HasSuffix(r.URL.Path, "/agents") && r.Method == "GET":
		op = "worker"
	case strings.HasSuffix(r.URL.Path, "/sessions") && r.Method == "POST":
		f.opens++
		op = fmt.Sprintf("open%d", f.opens)
	case r.URL.Path == "/queue" && r.Method == "GET":
		f.polls++
		op = fmt.Sprintf("poll%d", f.polls)
		if r.URL.Query().Has("lastMessageId") || r.Header.Get(scaleset.HeaderScaleSetMaxCapacity) != "1" {
			f.t.Error("fresh listener inherited cursor or capacity")
		}
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer synthetic-queue-generation-%d", f.polls) {
			f.t.Error("physical generation token mismatch")
		}
	case strings.Contains(r.URL.Path, "/sessions/") && r.Method == "DELETE":
		f.closes++
		if f.closes == 2 {
			f.complete = true
		}
		op = fmt.Sprintf("close%d", f.closes)
		want := fmt.Sprintf("00000000-0000-4000-8000-%012d", f.closes)
		if !strings.HasSuffix(r.URL.Path, "/"+want) {
			f.t.Error("physical Close targeted wrong SDK object")
		}
	}
	if op != "" {
		f.ops = append(f.ops, op)
	}
	if (f.variant == "close1-loss" && op == "close1") || (f.variant == "close2-loss" && op == "close2") {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
		return true
	}
	return false
}
func (f *replacementTrace) extra(w http.ResponseWriter, r *http.Request) bool {
	a := f.base.a
	if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/sessions/") {
		w.WriteHeader(204)
		return true
	}
	if strings.HasSuffix(r.URL.Path, "/agents") {
		json.NewEncoder(w).Encode(map[string]any{"count": 0, "value": []any{}})
		return true
	}
	repo := map[string]any{"id": a.RepositoryID, "full_name": a.Organization + "/" + a.Repository, "private": true, "fork": false, "owner": map[string]any{"login": a.Organization}}
	var value any
	switch r.URL.Path {
	case "/installation/repositories", "/orgs/fixture-org/actions/runner-groups/3/repositories":
		value = map[string]any{"total_count": 1, "repositories": []any{repo}}
	case "/repos/fixture-org/canary":
		value = repo
	case "/orgs/fixture-org/actions/runner-groups/3":
		value = map[string]any{"id": 3, "visibility": "selected", "default": false, "allows_public_repositories": false, "inherited": false}
	default:
		return false
	}
	json.NewEncoder(w).Encode(value)
	return true
}
func (f *replacementTrace) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ops...)
}

var replacementSuccessTrace = []string{"set", "worker", "open1", "poll1", "close1", "open2", "poll2", "set", "worker", "close2"}

type replacementFixture struct {
	base      *baselineFixture
	trace     *replacementTrace
	handoff   controllerHandoffFixture
	callbacks atomic.Int32
}

func newReplacementFixture(t *testing.T, variant string) *replacementFixture {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	a.Phases = append(a.Phases, replacementPhase)
	var original []byte
	base := newBaselineFixtureWithCreate(t, replacementResponse(a, variant), "", a, func(j *FileJournal) { created, _ := completedHandoffCreate(t, j, a); original = created.snapshot })
	base.release()
	base.release = nil
	f := controllerHandoffFixtureForApproval(t, a, replacementPhase, time.Now().UTC().Truncate(time.Second))
	f.snapshot = original
	var err error
	_, f.request, err = controllerHandoffRequest(original, replacementPhase, f.request.Source, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.receipt.ControllerApprovalSHA256 = f.request.ControllerApprovalSHA256
	f.receipt.ReceiptNonce = strings.Repeat("e", 32)
	f.receiptBytes = f.encodeReceipt(t, f.receipt, true)
	return &replacementFixture{base: base, trace: attachReplacementTrace(t, base, variant), handoff: f}
}
func (f *replacementFixture) run(t *testing.T, ctx context.Context) error {
	ctx = context.WithValue(ctx, replacementProbeKey{}, func(stage string, g int) {
		if stage == "cursor2" && g != 0 {
			t.Error("fresh listener inherited positive cursor argument")
		}
		if stage == "callback" {
			f.callbacks.Add(1)
			t.Error("stale real-message callback entered")
		}
	})
	h := f.handoff
	payload := handoffCredentialBytes(t, h.approval)
	var deadlines []time.Time
	if deadline, ok := ctx.Deadline(); ok {
		deadlines = append(deadlines, deadline)
	}
	c, in, issuer := phasePipesDelivered(t, h, payload, payload, "controller", deadlines...)
	return runControllerProcessHandoffPhase(ctx, f.base.j, h.snapshot, c, in, "controller", replacementPhase, nil, h.request.Source, h.root, issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) { return f.base.api, nil })
}

func TestSessionReplacementProtocol(t *testing.T) {
	for _, variant := range []string{"old41", "zero", "new202"} {
		t.Run(variant, func(t *testing.T) {
			f := newReplacementFixture(t, variant)
			if err := f.run(t, context.Background()); err != nil {
				t.Fatalf("replacement: %v trace=%v stages=%v", err, f.trace.snapshot(), replacementStages(f.base.j.Events()))
			}
			if !reflect.DeepEqual(f.trace.snapshot(), replacementSuccessTrace) || f.callbacks.Load() != 0 || f.base.acks.Load() != 0 || f.base.acquires.Load() != 0 || f.base.forbidden.Load() != 0 {
				t.Fatal("exact replacement trace or stale callback assertion failed", f.trace.snapshot())
			}
			events := f.base.j.Events()
			last := events[len(events)-1].Replacement
			if last == nil || last.Stage != "complete" || !replay(events).reserved || !replay(events).uncertain {
				t.Fatal("replacement lost durable reservations or summary")
			}
			if len(events) != 9+len(replacementSteps) {
				t.Fatal("unexpected replacement event budget")
			}
			if variant == "zero" && (events[17].Replacement.MessageID == nil || *events[17].Replacement.MessageID != 0) {
				t.Fatal("present zero was not durable")
			}
		})
	}
}
func replacementStages(events []Event) []string {
	var out []string
	for _, e := range events {
		if e.Replacement != nil {
			out = append(out, e.Replacement.Stage+"/"+e.Replacement.Outcome)
		}
	}
	return out
}

func TestSessionReplacementFailureStops(t *testing.T) {
	for _, tc := range []struct {
		variant              string
		opens, polls, closes int
	}{
		{"missing", 1, 1, 0}, {"null", 1, 1, 0}, {"old401", 1, 1, 0}, {"close1-loss", 1, 1, 1}, {"open2-loss", 2, 1, 1}, {"close2-loss", 2, 2, 2}, {"same-uuid", 2, 1, 1}, {"owner-drift", 2, 1, 1}, {"queue-origin", 2, 1, 1}, {"seeded-query", 2, 1, 1}, {"nested-drift", 2, 1, 1},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			f := newReplacementFixture(t, tc.variant)
			if f.run(t, context.Background()) == nil {
				t.Fatal("fault completed replacement")
			}
			f.trace.mu.Lock()
			got := []int{f.trace.opens, f.trace.polls, f.trace.closes}
			f.trace.mu.Unlock()
			if !reflect.DeepEqual(got, []int{tc.opens, tc.polls, tc.closes}) || f.base.acks.Load() != 0 || f.base.acquires.Load() != 0 || f.callbacks.Load() != 0 {
				t.Fatal("fault crossed next network boundary", got, f.trace.snapshot())
			}
			if !replay(f.base.j.Events()).reserved {
				t.Fatal("fault lost reservation")
			}
		})
	}
}

func TestSessionReplacementHistoryNeverResumes(t *testing.T) {
	f := newReplacementFixture(t, "old41")
	if err := f.run(t, context.Background()); err != nil {
		t.Fatal(err)
	}
	before := f.base.requests.Load()
	f.handoff.receipt.ReceiptNonce = strings.Repeat("f", 32)
	f.handoff.receiptBytes = f.handoff.encodeReceipt(t, f.handoff.receipt, true)
	dir, admission := f.base.j.directory, f.base.j.claim.directory
	if err := f.base.j.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.base.j, err = openJournalAtAdmission(dir, f.base.a, admission, func(file *os.File) error { return file.Sync() })
	if err != nil {
		t.Fatal("replacement reopen rejected historical truth", err)
	}
	defer f.base.j.Close()
	if f.run(t, context.Background()) == nil || (&Driver{f.base.a, f.base.j, f.base.api}).Run(context.Background(), replacementPhase) == nil || f.base.requests.Load() != before {
		t.Fatal("reopen or legacy invocation restored network authority")
	}
	renewed := f.base.a
	renewed.ExpiresAt = renewed.ExpiresAt.Add(time.Minute)
	f.base.j.Close()
	if j, err := openJournalAtAdmission(dir, renewed, admission, func(file *os.File) error { return file.Sync() }); err == nil {
		j.Close()
		t.Fatal("renewal restored replacement grant")
	}
}

func TestSessionReplacementStorageStopsAtEveryAppend(t *testing.T) {
	for _, mode := range []string{"append", "sync"} {
		for target := 1; target <= len(replacementSteps)+2; target++ {
			t.Run(fmt.Sprintf("%s-%02d", mode, target), func(t *testing.T) {
				f := newReplacementFixture(t, "old41")
				calls := 0
				var atFault int32 = -1
				if mode == "append" {
					f.base.j.writeFile = func(file *os.File, data []byte) (int, error) {
						calls++
						if calls == target {
							atFault = f.base.requests.Load()
							return 0, errors.New("synthetic storage loss")
						}
						return file.Write(data)
					}
				} else {
					f.base.j.syncFile = func(file *os.File) error {
						calls++
						if calls == target {
							atFault = f.base.requests.Load()
							return errors.New("synthetic sync loss")
						}
						return file.Sync()
					}
				}
				if f.run(t, context.Background()) == nil || atFault < 0 || f.base.requests.Load() != atFault || f.base.acks.Load() != 0 || f.base.acquires.Load() != 0 || f.callbacks.Load() != 0 {
					t.Fatal("storage uncertainty allowed later traffic", calls, atFault, f.base.requests.Load(), f.trace.snapshot())
				}
			})
		}
	}
}

func TestSessionReplacementCancellationAtEveryAppend(t *testing.T) {
	for target := 1; target <= len(replacementSteps)+2; target++ {
		t.Run(fmt.Sprintf("append-%02d", target), func(t *testing.T) {
			f := newReplacementFixture(t, "old41")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			var stoppedAt int32 = -1
			f.base.j.syncFile = func(file *os.File) error {
				err := file.Sync()
				calls++
				if calls == target {
					stoppedAt = f.base.requests.Load()
					cancel()
				}
				return err
			}
			if f.run(t, ctx) == nil || stoppedAt < 0 || f.base.requests.Load() != stoppedAt || f.callbacks.Load() != 0 {
				t.Fatal("canceled parent crossed an effect boundary", calls, stoppedAt, f.base.requests.Load())
			}
		})
	}
}

func TestSessionReplacementExpiryAtEveryEffectIntent(t *testing.T) {
	for index, step := range replacementSteps {
		if step.outcome != "intent" {
			continue
		}
		t.Run(fmt.Sprintf("%s-%d", step.stage, step.generation), func(t *testing.T) {
			f := newReplacementFixture(t, "old41")
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
			defer cancel()
			calls, target := 0, index+3
			var stoppedAt int32 = -1
			f.base.j.syncFile = func(file *os.File) error {
				err := file.Sync()
				calls++
				if calls == target {
					stoppedAt = f.base.requests.Load()
					<-ctx.Done()
				}
				return err
			}
			if f.run(t, ctx) == nil || stoppedAt < 0 || f.base.requests.Load() != stoppedAt || f.callbacks.Load() != 0 {
				t.Fatal("expired parent crossed an effect boundary", calls, stoppedAt, f.base.requests.Load())
			}
		})
	}
}
