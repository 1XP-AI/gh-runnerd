package livecanary

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
)

func baselineMap(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		t.Fatal("fixture object")
	}
	return m
}
func baselineEvents(t *testing.T, f *baselineFixture) []Event { t.Helper(); return f.j.Events() }
func baselineLast(t *testing.T, f *baselineFixture) *baselineRecord {
	t.Helper()
	events := f.j.Events()
	if len(events) == 0 || events[len(events)-1].Baseline == nil {
		t.Fatal("missing normalized evidence")
	}
	return events[len(events)-1].Baseline
}
func baselineRefuses(t *testing.T, f *baselineFixture, ack, acquire, continuation int32) {
	t.Helper()
	err := f.run(t)
	if err == nil || f.acks.Load() != ack || f.acquires.Load() != acquire || f.continuations.Load() != continuation || f.forbidden.Load() != 0 {
		t.Fatalf("refusal: err=%v ACK=%d acquire=%d continuation=%d forbidden=%d", err, f.acks.Load(), f.acquires.Load(), f.continuations.Load(), f.forbidden.Load())
	}
	if !replay(f.j.Events()).uncertain || !replay(f.j.Events()).reserved {
		t.Fatal("generic replay lost its work fence")
	}
}
func TestBaselineOwnedSetReadback(t *testing.T) {
	for _, name := range []string{"id", "name", "group", "label", "updates", "missing-setting"} {
		t.Run(name, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, v any) any {
				if stage != "set" {
					return v
				}
				m := baselineMap(t, v)
				switch name {
				case "id":
					m["id"] = 8
				case "name":
					m["name"] = "other"
				case "group":
					m["runnerGroupId"] = 99
				case "label":
					m["labels"] = []any{}
				case "updates":
					m["RunnerSetting"] = map[string]any{"disableUpdate": false}
				case "missing-setting":
					delete(m, "RunnerSetting")
				}
				return m
			})
			baselineRefuses(t, f, 0, 0, 0)
			if f.sets.Load() != 1 || f.sessions.Load() != 0 || baselineLast(t, f).Stage != "set-observe" {
				t.Fatal("set refusal happened after session mutation")
			}
		})
	}
}
func TestBaselineStatisticsPresenceAndEligibility(t *testing.T) {
	keys := []string{"totalAvailableJobs", "totalAcquiredJobs", "totalAssignedJobs", "totalRunningJobs", "totalRegisteredRunners", "totalBusyRunners", "totalIdleRunners"}
	for _, stage := range []string{"set", "session", "nested", "poll1", "poll2"} {
		for _, key := range keys {
			for _, bad := range []string{"missing", "null", "negative", "above-one", "active"} {
				if bad == "active" && (stage == "poll2" || key == keys[0] || key == keys[2]) {
					continue
				}
				t.Run(stage+"/"+key+"/"+bad, func(t *testing.T) {
					f := newBaselineFixture(t, func(at string, v any) any {
						target := stage
						if target == "nested" {
							target = "session"
						}
						if at != target {
							return v
						}
						m := baselineMap(t, v)
						targetMap := m
						if stage == "nested" {
							targetMap = map[string]any{"id": 7, "name": approval().setName(), "runnerGroupId": approval().RunnerGroupID, "statistics": baselineMap(t, scaleset.RunnerScaleSetStatistic{})}
							m["runnerScaleSet"] = targetMap
						}
						stats := targetMap["statistics"].(map[string]any)
						switch bad {
						case "missing":
							delete(stats, key)
						case "null":
							stats[key] = nil
						case "negative":
							stats[key] = -1
						case "above-one":
							stats[key] = 2
						case "active":
							stats[key] = 1
						}
						return m
					})
					ack, acq, cont := int32(0), int32(0), int32(0)
					if stage == "poll2" {
						ack, acq, cont = 1, 1, 1
					}
					baselineRefuses(t, f, ack, acq, cont)
					if stage == "set" && f.sessions.Load() != 0 {
						t.Fatal("ineligible set created a session")
					}
					if stage == "session" || stage == "nested" {
						if f.polls.Load() != 0 {
							t.Fatal("ineligible session reached polling")
						}
					}
				})
			}
		}
	}
	for _, stage := range []string{"set", "session", "poll1"} {
		for _, missing := range []bool{false, true} {
			t.Run(stage+"/whole-stats/"+map[bool]string{false: "null", true: "missing"}[missing], func(t *testing.T) {
				f := newBaselineFixture(t, func(at string, v any) any {
					if at != stage {
						return v
					}
					m := baselineMap(t, v)
					if missing {
						delete(m, "statistics")
					} else {
						m["statistics"] = nil
					}
					return m
				})
				baselineRefuses(t, f, 0, 0, 0)
			})
		}
	}
}
func TestBaselineSourceAuthority(t *testing.T) {
	for _, key := range []string{"ownerName", "repositoryName", "workflowRunId", "eventName", "jobWorkflowRef", "jobId"} {
		for _, bad := range []string{"missing", "null", "empty", "other"} {
			if (key == "jobWorkflowRef" || key == "jobId") && bad == "other" {
				continue
			} // The first nonempty ref is deliberately opaque.
			t.Run("available/"+key+"/"+bad, func(t *testing.T) {
				f := newBaselineFixture(t, func(stage string, v any) any {
					if stage != "poll1-items" {
						return v
					}
					items := v.([]any)
					m := items[0].(map[string]any)
					switch bad {
					case "missing":
						delete(m, key)
					case "null":
						m[key] = nil
					case "empty":
						if key == "workflowRunId" {
							m[key] = 0
						} else {
							m[key] = ""
						}
					case "other":
						if key == "workflowRunId" {
							m[key] = 6
						} else {
							m[key] = "other"
						}
					}
					return items
				})
				baselineRefuses(t, f, 0, 0, 0)
				if f.sources.Load() != 0 {
					t.Fatal("unanchored message reached source verification")
				}
			})
		}
	}
	for _, name := range []string{"run", "head", "path", "event", "attempt", "base-public", "base-fork", "head-public", "head-fork", "missing-attempt", "wrong-type"} {
		t.Run("REST/"+name, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, v any) any {
				if stage != "source" {
					return v
				}
				m := v.(map[string]any)
				switch name {
				case "run":
					m["id"] = 6
				case "head":
					m["head_sha"] = strings.Repeat("b", 40)
				case "path":
					m["path"] = ".github/workflows/other.yml"
				case "event":
					m["event"] = "push"
				case "attempt":
					m["run_attempt"] = 2
				case "missing-attempt":
					delete(m, "run_attempt")
				case "wrong-type":
					m["run_attempt"] = "1"
				default:
					repo := "repository"
					if strings.HasPrefix(name, "head") {
						repo = "head_repository"
					}
					if strings.HasSuffix(name, "public") {
						m[repo].(map[string]any)["private"] = false
					} else {
						m[repo].(map[string]any)["fork"] = true
					}
				}
				return m
			})
			baselineRefuses(t, f, 0, 0, 0)
			if f.sources.Load() != 1 || baselineLast(t, f).Stage != "source" {
				t.Fatal("strict source gate did not run")
			}
		})
	}
}
func TestBaselineSparseLifecycleAndContradictions(t *testing.T) {
	for _, form := range []string{"missing", "null", "zero", "matching"} {
		t.Run("sparse/"+form, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, v any) any {
				if stage != "poll2-items" {
					return v
				}
				for _, raw := range v.([]any) {
					m := raw.(map[string]any)
					for _, key := range []string{"ownerName", "repositoryName", "eventName", "jobWorkflowRef", "workflowRunId"} {
						switch form {
						case "null":
							m[key] = nil
						case "zero":
							if key == "workflowRunId" {
								m[key] = 0
							} else {
								m[key] = ""
							}
						case "matching":
							m[key] = baselineFixtureItem(approval(), "JobAvailable")[key]
						}
					}
				}
				return v
			})
			if err := f.run(t); err != nil {
				t.Fatal(err)
			}
			if f.acks.Load() != 2 || f.acquires.Load() != 1 || f.continuations.Load() != 1 {
				t.Fatal("sparse control did not complete")
			}
			events := baselineEvents(t, f)
			for _, e := range events {
				if e.Baseline != nil && e.Baseline.Stage == "poll" && e.Baseline.Batch != nil && e.Baseline.Batch.MessageID == 10 {
					if form == "missing" || form == "null" {
						if e.Baseline.Batch.Items[0].Owner != nil || e.Baseline.Batch.Items[0].RunID != nil {
							t.Fatal("invented sparse source")
						}
					}
				}
			}
		})
	}
	for _, key := range []string{"runnerRequestId", "jobId", "ownerName", "repositoryName", "workflowRunId", "eventName", "jobWorkflowRef", "runnerId", "runnerName", "result"} {
		t.Run("contradiction/"+key, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, v any) any {
				if stage != "poll2-items" {
					return v
				}
				items := v.([]any)
				m := items[2].(map[string]any)
				switch key {
				case "runnerRequestId":
					m[key] = 43
				case "workflowRunId":
					m[key] = -1
				case "runnerId":
					m[key] = 82
				default:
					m[key] = "different"
				}
				return items
			})
			baselineRefuses(t, f, 1, 1, 1)
		})
	}
	for _, name := range []string{"available+assigned", "assigned-before-anchor", "completed-before-acquire", "extra-available"} {
		t.Run(name, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, v any) any {
				if stage != "poll1-items" {
					return v
				}
				items := v.([]any)
				switch name {
				case "available+assigned":
					return append(items, baselineFixtureItem(approval(), "JobAssigned"))
				case "assigned-before-anchor":
					return []any{baselineFixtureItem(approval(), "JobAssigned")}
				case "completed-before-acquire":
					return append(items, baselineFixtureItem(approval(), "JobCompleted"))
				case "extra-available":
					return append(items, baselineFixtureItem(approval(), "JobAvailable"))
				}
				return v
			})
			if name == "available+assigned" {
				if err := f.run(t); err != nil {
					t.Fatal(err)
				}
			} else {
				baselineRefuses(t, f, 0, 0, 0)
			}
		})
	}
}
func TestBaselineAmbiguousWireAndAcquireEnvelope(t *testing.T) {
	for _, name := range []string{"outer-duplicate", "inner-folded", "inner-unicode-fold", "trailing", "null-item", "null-body", "wrong-body-type", "five-items", "unknown-outer", "statistics-type", "oversize"} {
		t.Run(name, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, v any) any {
				if stage != "poll1" {
					return v
				}
				m := v.(map[string]any)
				switch name {
				case "outer-duplicate":
					data, _ := json.Marshal(m)
					return json.RawMessage(strings.TrimSuffix(string(data), "}") + `,"MESSAGEID":9}`)
				case "inner-folded", "inner-unicode-fold":
					item, _ := json.Marshal(baselineFixtureItem(approval(), "JobAvailable"))
					key := "JOBID"
					if name == "inner-unicode-fold" {
						key = "jobDiſplayName"
					}
					m["body"] = "[" + strings.TrimSuffix(string(item), "}") + `,"` + key + `":"other"}]`
				case "trailing":
					data, _ := json.Marshal(m)
					return json.RawMessage(string(data) + ` {}`)
				case "null-item":
					m["body"] = "[null]"
				case "null-body":
					m["body"] = nil
				case "wrong-body-type":
					m["body"] = []any{}
				case "five-items":
					m["body"] = "[{},{},{},{},{}]"
				case "unknown-outer":
					m["extra"] = true
				case "statistics-type":
					m["statistics"] = 1
				case "oversize":
					m["body"] = strings.Repeat("x", 1<<20)
				}
				return m
			})
			baselineRefuses(t, f, 0, 0, 0)
		})
	}
	for _, name := range []string{"missing-count", "null-count", "negative-count", "wrong-type", "no-value", "null-value", "wrong-id", "duplicate-id", "extra-id", "folded-count", "trailing"} {
		t.Run("acquire/"+name, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, v any) any {
				if stage != "acquire" {
					return v
				}
				m := v.(map[string]any)
				switch name {
				case "missing-count":
					delete(m, "count")
				case "null-count":
					m["count"] = nil
				case "negative-count":
					m["count"] = -1
				case "wrong-type":
					m["count"] = "1"
				case "no-value":
					delete(m, "value")
				case "null-value":
					m["value"] = nil
				case "wrong-id":
					m["value"] = []int{43}
				case "duplicate-id":
					m["value"] = []int{42, 42}
				case "extra-id":
					m["value"] = []int{42, 43}
				case "folded-count":
					return json.RawMessage(`{"count":1,"COUNT":1,"value":[42]}`)
				case "trailing":
					return json.RawMessage(`{"count":1,"value":[42]} {}`)
				}
				return m
			})
			baselineRefuses(t, f, 1, 1, 0)
		})
	}
}

// Every request is still an actual pinned SDK call over the private TLS server;
// stepping the adapter only exposes boundaries normally traversed by Listener.Run.
func baselineStepped(t *testing.T, f *baselineFixture) *baselineListener {
	t.Helper()
	b, err := newBaselineListenerHeld(context.Background(), f.a, f.j, f.api, 7)
	if err != nil {
		t.Fatal(err)
	}
	b.running = true
	b.after = func(context.Context, baselineAcquisition) error { f.continuations.Add(1); return nil }
	t.Cleanup(func() { b.cancel(); b.running = false; b.invalid = true })
	if err = b.initialize(); err != nil {
		t.Fatal(err)
	}
	return b
}
func baselineAcquireFirst(t *testing.T, b *baselineListener) {
	t.Helper()
	m, err := b.GetMessage(context.Background(), 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.DeleteMessage(context.Background(), m.MessageID); err != nil {
		t.Fatal(err)
	}
	if _, err = b.AcquireJobs(context.Background(), []int64{42}); err != nil {
		t.Fatal(err)
	}
	if _, err = b.HandleDesiredRunnerCount(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}

func TestBaselinePostIntentAuthorityBoundary(t *testing.T) {
	for _, stage := range []string{"set-observe", "session-open", "ack", "acquire", "continuation"} {
		for _, change := range []string{"cancel", "journal", "claim"} {
			t.Run(stage+"/"+change, func(t *testing.T) {
				f := newBaselineFixture(t, nil)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				b, err := newBaselineListenerHeld(ctx, f.a, f.j, f.api, 7)
				if err != nil {
					t.Fatal(err)
				}
				gated := false
				f.j.syncFile = func(file *os.File) error {
					if err := file.Sync(); err != nil {
						return err
					}
					data, err := os.ReadFile(file.Name())
					if err != nil {
						return err
					}
					lines := strings.Split(strings.TrimSpace(string(data)), "\n")
					var e Event
					if json.Unmarshal([]byte(lines[len(lines)-1]), &e) != nil {
						return errors.New("fixture tail")
					}
					if gated || e.Baseline == nil || e.Baseline.Stage != stage || e.Baseline.Outcome != "intent" {
						return nil
					}
					gated = true
					switch change {
					case "cancel":
						cancel()
					case "journal":
						path := filepath.Join(f.j.directory, "journal.jsonl")
						if err := os.Rename(path, path+".held"); err != nil {
							return err
						}
						return os.WriteFile(path, data, 0600)
					case "claim":
						path := f.j.claim.file.Name()
						contents, err := os.ReadFile(path)
						if err != nil {
							return err
						}
						if err = os.Rename(path, path+".held"); err != nil {
							return err
						}
						return os.WriteFile(path, contents, 0600)
					}
					return nil
				}
				err = b.run(func(context.Context, baselineAcquisition) error { f.continuations.Add(1); return nil })
				if !gated || err == nil {
					t.Fatalf("boundary not refused: gated=%v err=%v", gated, err)
				}
				switch stage {
				case "set-observe":
					if f.sets.Load() != 0 {
						t.Fatal("set GET after invalidating intent")
					}
				case "session-open":
					if f.sessions.Load() != 0 {
						t.Fatal("session POST after invalidating intent")
					}
				case "ack":
					if f.acks.Load() != 0 {
						t.Fatal("ACK after invalidating intent")
					}
				case "acquire":
					if f.acquires.Load() != 0 {
						t.Fatal("acquire after invalidating intent")
					}
				case "continuation":
					if f.continuations.Load() != 0 {
						t.Fatal("continuation after invalidating intent")
					}
				}
				if last := baselineLast(t, f); last.Stage != stage || last.Outcome != "intent" {
					t.Fatalf("durable intent replaced: %s/%s", last.Stage, last.Outcome)
				}
			})
		}
	}
}
func TestBaselineCallbackMutation(t *testing.T) {
	for _, key := range []string{"owner", "run", "ref", "time", "runner"} {
		t.Run(key, func(t *testing.T) {
			f := newBaselineFixture(t, nil)
			b := baselineStepped(t, f)
			baselineAcquireFirst(t, b)
			m, err := b.GetMessage(context.Background(), 9, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err = b.DeleteMessage(context.Background(), m.MessageID); err != nil {
				t.Fatal(err)
			}
			x := m.JobStartedMessages[0]
			switch key {
			case "owner":
				x.OwnerName = "wrong"
			case "run":
				x.WorkflowRunID = 999
			case "ref":
				x.JobWorkflowRef = "wrong"
			case "time":
				x.QueueTime = time.Now()
			case "runner":
				x.RunnerID = 999
			}
			before := len(f.j.Events())
			if err = b.HandleJobStarted(context.Background(), x); err == nil || len(f.j.Events()) != before {
				t.Fatal("mutated SDK callback was recorded as original wire fact")
			}
		})
	}
}
