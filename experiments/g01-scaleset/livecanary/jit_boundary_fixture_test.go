//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
	"github.com/actions/scaleset"
)

const jitBoundaryFixtureSecret = "c3ludGhldGljLWppdC1ib3VuZGFyeS1zZWNyZXQ="

type jitBoundaryFixture struct {
	base                 *baselineFixture
	handoff              controllerHandoffFixture
	path, phase, variant string
	mu                   sync.Mutex
	ops                  []string
	committed            string
	jit, workers         int
	before               func(http.ResponseWriter, *http.Request) bool
}

func newJITBoundaryFixture(t *testing.T, phase, variant string) *jitBoundaryFixture {
	t.Helper()
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	if phase == beforeJITPhase {
		a.Phases = append(a.Phases, phase)
	}
	if variant == "org-only" {
		a.ManagementScope = "organization-only"
	}
	var original []byte
	base := newBaselineFixtureWithCreate(t, func(stage string, value any) any {
		if stage == "set" {
			x := value.(scaleset.RunnerScaleSet)
			x.Labels[0].Type = "System"
			x.Statistics = &scaleset.RunnerScaleSetStatistic{}
			return x
		}
		return value
	}, "", a, func(j *FileJournal) { created, _ := completedHandoffCreate(t, j, a); original = created.snapshot })
	base.release()
	base.release = nil
	f := controllerHandoffFixtureForApproval(t, a, phase, time.Now().UTC().Truncate(time.Second))
	f.snapshot = original
	var err error
	_, f.request, err = controllerHandoffRequest(original, phase, f.request.Source, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.receipt.ControllerApprovalSHA256 = f.request.ControllerApprovalSHA256
	f.receipt.ReceiptNonce = strings.Repeat("e", 32)
	f.receiptBytes = f.encodeReceipt(t, f.receipt, true)
	path := filepath.Join(filepath.Dir(base.j.directory), "jit-approval.json")
	if os.WriteFile(path, original, 0600) != nil {
		t.Fatal("approval fixture")
	}
	out := &jitBoundaryFixture{base: base, handoff: f, path: path, phase: phase, variant: variant}
	base.beforeRequest = func(w http.ResponseWriter, r *http.Request) bool {
		out.mu.Lock()
		defer out.mu.Unlock()
		out.ops = append(out.ops, r.Method+" "+r.URL.Path)
		if !r.Close {
			t.Error("dedicated physical request allows connection reuse")
		}
		if out.before != nil && out.before(w, r) {
			return true
		}
		if strings.HasSuffix(r.URL.Path, "/generatejitconfig") {
			out.jit++
			if phase == "jit-loss" {
				disk, err := os.ReadFile(filepath.Join(base.j.directory, "journal.jsonl"))
				if err != nil {
					t.Error("flushed send-intent unavailable")
				}
				lines := bytes.Split(bytes.TrimSpace(disk), []byte{'\n'})
				var last Event
				if DecodeStrict(lines[len(lines)-1], &last) != nil || last.JITBoundary == nil || last.JITBoundary.Outcome != "send-intent" {
					t.Error("physical entry preceded durable send-intent")
				}
			}
			var body struct {
				Name string `json:"name"`
				Work string `json:"workFolder"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name != a.workerName() || body.Work != "_work" {
				t.Error("actual SDK physical JIT body differs")
			}
			// Inventory is derived only from this server's own committed creation.
			if variant == "disconnect" {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
				return true
			}
			if variant != "absent" {
				out.committed = body.Name
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"runner": map[string]any{"id": 991, "name": body.Name, "runnerScaleSetId": 7}, "encodedJITConfig": jitBoundaryFixtureSecret})
			return true
		}
		if strings.HasSuffix(r.URL.Path, "/agents") {
			out.workers++
			value := []any{}
			if out.committed != "" {
				value = append(value, map[string]any{"id": 992, "name": out.committed, "runnerScaleSetId": 7})
			}
			json.NewEncoder(w).Encode(map[string]any{"count": len(value), "value": value})
			return true
		}
		repo := map[string]any{"id": a.RepositoryID, "full_name": a.Organization + "/" + a.Repository, "private": true, "fork": false, "owner": map[string]any{"login": a.Organization}}
		var body any
		switch r.URL.Path {
		case "/installation/repositories", "/orgs/fixture-org/actions/runner-groups/3/repositories":
			body = map[string]any{"total_count": 1, "repositories": []any{repo}}
		case "/repos/fixture-org/canary":
			body = repo
		case "/orgs/fixture-org/actions/runner-groups/3":
			body = map[string]any{"id": 3, "visibility": "selected", "default": false, "allows_public_repositories": false, "inherited": false}
		default:
			return false
		}
		json.NewEncoder(w).Encode(body)
		return true
	}
	return out
}
func (f *jitBoundaryFixture) run(t *testing.T, ctx context.Context) error {
	t.Helper()
	h := f.handoff
	original, err := os.Stat(f.path)
	if err != nil {
		t.Fatal("original approval stat")
	}
	ctx = context.WithValue(ctx, jitBoundaryGuardKey{}, func() error {
		var a Approval
		current, err := readPairedApproval(f.path, &a, DecodeStrict)
		if err != nil || !os.SameFile(original, current.info) || handoff.Digest(current.raw) != handoff.Digest(h.snapshot) || approvalDigest(a) != approvalDigest(h.approval) {
			return ErrApproval
		}
		return nil
	})
	payload := handoffCredentialBytes(t, h.approval)
	var deadlines []time.Time
	if d, ok := ctx.Deadline(); ok {
		deadlines = append(deadlines, d)
	}
	c, in, issuer := phasePipesDelivered(t, h, payload, payload, "controller", deadlines...)
	return runControllerProcessHandoffPhase(ctx, f.base.j, h.snapshot, c, in, "controller", f.phase, nil, h.request.Source, h.root, issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) { return f.base.api, nil })
}
func (f *jitBoundaryFixture) counts() (int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jit, f.workers, len(f.ops)
}
func jitBoundaryStages(events []Event) []string {
	var out []string
	for _, e := range events {
		if e.JITBoundary != nil {
			out = append(out, e.JITBoundary.Stage+"/"+e.JITBoundary.Outcome)
		}
	}
	return out
}
func TestJITBoundaryProtocol(t *testing.T) {
	for _, phase := range []string{beforeJITPhase, "jit-loss"} {
		for _, variant := range []string{"committed", "absent", "org-only"} {
			t.Run(phase+"/"+variant, func(t *testing.T) {
				f := newJITBoundaryFixture(t, phase, variant)
				seen := false
				ctx := context.WithValue(context.Background(), jitBoundaryProbeKey{}, func(dto *scaleset.RunnerScaleSetJitRunnerConfig, err error) {
					seen = true
					if dto != nil {
						t.Error("SDK return boundary leaked nonnil DTO/secret/runner")
					}
				})
				runErr := f.run(t, ctx)
				if jit, _, _ := f.counts(); phase == beforeJITPhase && jit != 0 {
					t.Fatalf("physical JIT count = %d, want 0", jit)
				}
				if err := runErr; err != nil {
					t.Fatalf("boundary: %v stages=%v counts=%v", err, jitBoundaryStages(f.base.j.Events()), f.base.requests.Load())
				}
				want, forwards := 0, 9
				if phase == "jit-loss" {
					want = 1
					forwards++
				}
				if variant == "org-only" {
					forwards -= 2
				}
				jit, workers, total := f.counts()
				if jit != want {
					t.Fatalf("physical JIT count = %d, want %d", jit, want)
				}
				if workers != 2 || total != forwards || !seen || f.base.sessions.Load() != 0 || f.base.acks.Load() != 0 || f.base.acquires.Load() != 0 || f.base.forbidden.Load() != 0 {
					t.Fatal("physical finite trace incomplete", jit, workers, total, seen)
				}
				events := f.base.j.Events()
				wantStages := []string{}
				for _, step := range jitBoundarySteps(phase) {
					wantStages = append(wantStages, step.stage+"/"+step.outcome)
				}
				if !reflect.DeepEqual(jitBoundaryStages(events), wantStages) || !replay(events).reserved || !replay(events).uncertain || len(events) != 9+len(wantStages) {
					t.Fatal("typed grammar or held reservation lost")
				}
				raw, _ := os.ReadFile(filepath.Join(f.base.j.directory, "journal.jsonl"))
				if bytes.Contains(raw, []byte(jitBoundaryFixtureSecret)) || bytes.Contains(raw, []byte(`"runner_id":991`)) {
					t.Fatal("response secret or hidden response identity persisted")
				}
				if phase == "jit-loss" && variant != "absent" && !bytes.Contains(raw, []byte(`"runner_id":992`)) {
					t.Fatal("independent lookup ID missing")
				}
			})
		}
	}
}
func TestJITBoundaryDisconnectStopsWithoutLookup(t *testing.T) {
	f := newJITBoundaryFixture(t, "jit-loss", "disconnect")
	if f.run(t, context.Background()) == nil {
		t.Fatal("disconnect completed")
	}
	jit, workers, total := f.counts()
	if jit != 1 || workers != 1 || total != 9 || f.committed != "" {
		t.Fatal("loss caused retry, invented inventory, or automatic post-error GET", jit, workers, total)
	}
	last := f.base.j.Events()
	if last[len(last)-1].JITBoundary.Outcome != "send-intent" || !replay(last).reserved {
		t.Fatal("ambiguous durable reservation lost")
	}
}
func TestJITBoundaryStorageAndCancellationEveryAppend(t *testing.T) {
	for _, phase := range []string{beforeJITPhase, "jit-loss"} {
		for _, mode := range []string{"write", "short", "sync", "cancel"} {
			for target := 1; target <= len(jitBoundarySteps(phase))+2; target++ {
				t.Run(fmt.Sprintf("%s/%s-%02d", phase, mode, target), func(t *testing.T) {
					f := newJITBoundaryFixture(t, phase, "")
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					calls, stopped := 0, int32(-1)
					if mode == "write" || mode == "short" {
						f.base.j.writeFile = func(file *os.File, data []byte) (int, error) {
							calls++
							if calls == target {
								stopped = f.base.requests.Load()
								if mode == "short" {
									return file.Write(data[:len(data)/2])
								}
								return 0, io.ErrClosedPipe
							}
							return file.Write(data)
						}
					} else {
						f.base.j.syncFile = func(file *os.File) error {
							err := file.Sync()
							calls++
							if calls == target {
								stopped = f.base.requests.Load()
								if mode == "cancel" {
									cancel()
								} else {
									return io.ErrClosedPipe
								}
							}
							return err
						}
					}
					if f.run(t, ctx) == nil || stopped < 0 || f.base.requests.Load() != stopped {
						t.Fatal("storage/lifetime failure allowed later traffic", calls, stopped, f.base.requests.Load())
					}
				})
			}
		}
	}
}
