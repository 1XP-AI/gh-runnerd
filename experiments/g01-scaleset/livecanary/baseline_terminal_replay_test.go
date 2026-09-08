//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPairedTerminalPostIntentJournalIdentity(t *testing.T) {
	for _, stage := range []string{"terminal-session-close", "terminal-worker-delete", "terminal-set-delete"} {
		for _, side := range []string{"controller", "worker", "controller-claim", "worker-claim"} {
			t.Run(stage+"/"+side, func(t *testing.T) {
				t.Parallel()
				f := newTerminalFixture(t, true)
				fired := false
				f.c.j.syncFile = func(file *os.File) error {
					e := terminalLastDiskEvent(file)
					if err := file.Sync(); err != nil {
						return err
					}
					if !fired && e.Baseline != nil && e.Baseline.Stage == stage && e.Baseline.Outcome == "intent" {
						fired = true
						path := file.Name()
						switch side {
						case "worker":
							path = filepath.Join(f.wf.StateDirectory(), "journal.jsonl")
						case "controller-claim":
							path = filepath.Join(f.c.j.claim.directory, "admission.json")
						case "worker-claim":
							path = filepath.Join(f.wf.AdmissionDirectory(), "admission.json")
						}
						old, err := os.ReadFile(path)
						if err != nil {
							return err
						}
						if err = os.Rename(path, path+".retained"); err != nil {
							return err
						}
						return os.WriteFile(path, old, 0600)
					}
					return nil
				}
				out, err := f.run()
				if !fired || err == nil || out.Terminal != terminalUnresolved {
					t.Fatal("replaced journal accepted")
				}
				if stage == "terminal-session-close" && f.sessionDeletes.Load() != 0 || stage == "terminal-worker-delete" && f.workerDeletes.Load() != 0 || stage == "terminal-set-delete" && f.setDeletes.Load() != 0 {
					t.Fatal("effect after journal replacement")
				}
				terminalNoReplay(t, f)
			})
		}
	}
}
func TestPairedTerminalClosedReplayActualFile(t *testing.T) {
	t.Run("persisted_references_use_snake_case_json", func(t *testing.T) {
		checkTerminalReferenceJSONTags(t)
	})
	f := newTerminalFixture(t, true)
	if out, err := f.run(); err != nil || out.Terminal != terminalComplete {
		t.Fatal("complete private fixture")
	}
	events := f.c.j.Events()
	path := f.c.j.file.Name()
	dir, admission := f.c.j.directory, f.c.j.claim.directory
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	header := bytes.SplitN(raw, []byte{'\n'}, 2)[0]
	if err = f.c.j.Close(); err != nil {
		t.Fatal(err)
	}
	var parentResult *baselineRecord
	for _, e := range events {
		if e.Baseline != nil && e.Baseline.Stage == "terminal" && e.Baseline.Outcome == "result" {
			parentResult = cloneEvent(e).Baseline
		}
	}
	cases := []struct {
		name, stage, outcome string
		change               func(*baselineRecord)
		valid                bool
	}{
		{"premature-parent-result", "terminal-roster-final", "intent", func(r *baselineRecord) { *r = *parentResult }, false},
		{"parent-self-ref", "terminal", "intent", func(r *baselineRecord) { r.Terminal.Parent = r.Terminal.Previous }, false},
		{"invented-started", "terminal", "intent", func(r *baselineRecord) { r.Terminal.Evidence.Started = r.Terminal.Evidence.Completed }, false},
		{"decision-wrong-predecessor", "terminal-decision", "observed", func(r *baselineRecord) { r.Terminal.Previous = r.Terminal.Parent }, false},
		{"close404-not-success", "terminal-session-close", "result", func(r *baselineRecord) { r.HTTPStatus = 404 }, false},
		{"missing-worker-absence-not-success", "terminal-worker-delete", "result", func(r *baselineRecord) { r.Terminal.Deletion.AbsenceResult = nil }, false},
		{"set200-not-absence", "terminal-set-absence", "result", func(r *baselineRecord) { r.HTTPStatus = 200 }, false},
		{"closed-parent-evidence-substitution", "terminal", "result", func(r *baselineRecord) { r.Terminal.Evidence.Source = r.Terminal.Evidence.Acquire }, false},
		{"summary-measurement-substitution", "collection", "observed", func(r *baselineRecord) { r.Collection.Terminal.Evidence.Round8 = r.Collection.Terminal.Evidence.Start }, false},
		{"cross-stage-payload", "set-observe", "result", func(r *baselineRecord) { r.Terminal = &baselineTerminalPayload{} }, false},
		{"unknown-close404", "terminal-session-close", "result", func(r *baselineRecord) { r.Outcome = "unknown"; r.HTTPStatus = 404 }, true},
		{"unknown-worker-postread", "terminal-worker-delete", "result", func(r *baselineRecord) { r.Outcome = "unknown"; r.Terminal.Deletion.AbsenceResult = nil }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var candidate []Event
			for _, e := range events {
				candidate = append(candidate, cloneEvent(e))
				r := candidate[len(candidate)-1].Baseline
				if r != nil && r.Stage == tc.stage && r.Outcome == tc.outcome {
					tc.change(r)
					break
				}
			}
			var data bytes.Buffer
			data.Write(header)
			data.WriteByte('\n')
			for _, e := range candidate {
				b, _ := json.Marshal(e)
				data.Write(b)
				data.WriteByte('\n')
			}
			// Truncate/write the same inode. Earlier actual predecessor references are
			// unchanged; no later stale hash can trivially reject this edited final row.
			file, e := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0600)
			if e != nil {
				t.Fatal(e)
			}
			_, e = file.Write(data.Bytes())
			if e == nil {
				e = file.Sync()
			}
			_ = file.Close()
			if e != nil {
				t.Fatal(e)
			}
			before := f.c.requests.Load()
			j, openErr := openJournalAtAdmission(dir, f.c.a, admission, func(file *os.File) error { return file.Sync() })
			if j != nil {
				_ = j.Close()
			}
			if tc.valid != (openErr == nil) {
				t.Fatalf("closed shape accepted=%v want=%v", openErr == nil, tc.valid)
			}
			if before != f.c.requests.Load() {
				t.Fatal("replay issued network request")
			}
		})
	}
}

func checkTerminalReferenceJSONTags(t *testing.T) {
	t.Helper()
	evidence, err := json.Marshal(terminalEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	var evidenceFields map[string]json.RawMessage
	if err := json.Unmarshal(evidence, &evidenceFields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pair", "acquire", "jit", "handoff", "start", "source", "started", "completed", "round8"} {
		if _, ok := evidenceFields[name]; !ok {
			t.Fatalf("terminal evidence omitted JSON field %q: %s", name, evidence)
		}
	}
	facts, err := json.Marshal(terminalCollectionFacts{})
	if err != nil {
		t.Fatal(err)
	}
	var factFields map[string]json.RawMessage
	if err := json.Unmarshal(facts, &factFields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"measurement", "outcome", "intent", "result", "decision", "session_close_intent", "session_close_result", "worker_delete_result", "set_delete_intent", "set_delete_result", "set_absence_result"} {
		if _, ok := factFields[name]; !ok {
			t.Fatalf("terminal collection facts omitted JSON field %q: %s", name, facts)
		}
	}
}
