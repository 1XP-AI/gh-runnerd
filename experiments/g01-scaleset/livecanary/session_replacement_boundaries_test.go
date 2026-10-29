//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actions/scaleset"
)

func TestSessionReplacementPhysicalGate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*http.Request) *http.Request
	}{
		{"capacity", func(r *http.Request) *http.Request { r.Header.Set(scaleset.HeaderScaleSetMaxCapacity, "2"); return r }},
		{"duplicate-capacity", func(r *http.Request) *http.Request { r.Header.Add(scaleset.HeaderScaleSetMaxCapacity, "1"); return r }},
		{"token", func(r *http.Request) *http.Request { r.Header.Set("Authorization", "Bearer foreign"); return r }},
		{"duplicate-auth", func(r *http.Request) *http.Request {
			r.Header.Add("Authorization", r.Header.Get("Authorization"))
			return r
		}},
		{"host", func(r *http.Request) *http.Request { r.Host = "foreign.example"; return r }},
		{"origin", func(r *http.Request) *http.Request { r.URL.Host = "foreign.example"; r.Host = ""; return r }},
		{"path", func(r *http.Request) *http.Request { r.URL.Path += "/foreign"; return r }},
		{"tenant", func(r *http.Request) *http.Request {
			r.URL.Path = strings.Replace(r.URL.Path, "/tenant/", "/foreign/", 1)
			return r
		}},
		{"cursor", func(r *http.Request) *http.Request { r.URL.RawQuery = "lastMessageId=41"; return r }},
		{"zero-cursor-query", func(r *http.Request) *http.Request { r.URL.RawQuery = "lastMessageId=0"; return r }},
		{"PATCH", func(r *http.Request) *http.Request { r.Method = "PATCH"; return r }},
		{"unmarked", func(r *http.Request) *http.Request { return r.Clone(context.Background()) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReplacementFixture(t, "old41")
			exercised := false
			forwarded := 0
			var stoppedAt int32
			ctx := context.WithValue(context.Background(), replacementWireProbeKey{}, func(p *replacementPermit) {
				stage := "poll"
				if tc.name == "tenant" {
					stage = "terminal-session-close"
				}
				if exercised || p.wire.stage != stage || p.generation != 1 {
					return
				}
				exercised = true
				bounded, cancel := context.WithTimeout(p.scope.ctx, operationTimeout)
				defer cancel()
				req, _ := http.NewRequestWithContext(p.context(bounded), "GET", p.scope.snapshots[1].MessageQueueURL, nil)
				req.Header.Set("Authorization", "Bearer "+p.scope.snapshots[1].MessageQueueAccessToken)
				req.Header.Set(scaleset.HeaderScaleSetMaxCapacity, "1")
				if tc.name == "tenant" {
					req, _ = http.NewRequestWithContext(p.context(bounded), "DELETE", p.scope.origin+baselineRuntimeScaleSetPath(p.scope.tenant, p.scope.setID, "/sessions/"+p.scope.snapshots[1].SessionID.String())+"?api-version=6.0-preview", nil)
					req.Header.Set("Authorization", "Bearer "+p.scope.management)
				}
				req = tc.mutate(req)
				physical := baselineRequestCaptureTransport{replacement: p.scope, inner: baselineRoundTrip(func(*http.Request) (*http.Response, error) {
					forwarded++
					return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(""))}, nil
				})}
				if _, err := physical.RoundTrip(req); err == nil {
					t.Error("physical gate accepted mutation")
				}
				stoppedAt = f.base.requests.Load()
			})
			if f.run(t, ctx) == nil || !exercised || forwarded != 0 || f.base.requests.Load() != stoppedAt || f.callbacks.Load() != 0 {
				t.Fatal("mutated physical request crossed boundary", exercised, forwarded, f.trace.snapshot())
			}
		})
	}
}

func TestSessionReplacementPhysicalDuplicateAndBootstrapRetry(t *testing.T) {
	t.Run("poll-duplicate", func(t *testing.T) {
		f := newReplacementFixture(t, "old41")
		exercised := false
		forwarded := 0
		ctx := context.WithValue(context.Background(), replacementWireProbeKey{}, func(p *replacementPermit) {
			if exercised || p.wire.stage != "poll" {
				return
			}
			exercised = true
			bounded, cancel := context.WithTimeout(p.scope.ctx, operationTimeout)
			defer cancel()
			req, _ := http.NewRequestWithContext(p.context(bounded), "GET", p.scope.snapshots[1].MessageQueueURL, nil)
			req.Header.Set("Authorization", "Bearer "+p.scope.snapshots[1].MessageQueueAccessToken)
			req.Header.Set(scaleset.HeaderScaleSetMaxCapacity, "1")
			physical := baselineRequestCaptureTransport{replacement: p.scope, inner: baselineRoundTrip(func(*http.Request) (*http.Response, error) {
				forwarded++
				return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			response, err := physical.RoundTrip(req)
			if err != nil {
				t.Error("valid physical control refused")
			}
			if response != nil {
				response.Body.Close()
			}
			if _, err = physical.RoundTrip(req); err == nil {
				t.Error("duplicate forwarded")
			}
		})
		if f.run(t, ctx) == nil || !exercised || forwarded != 1 || f.trace.polls != 0 || f.trace.closes != 0 {
			t.Fatal("physical duplicate budget failed", forwarded, f.trace.snapshot())
		}
	})
	t.Run("bootstrap-401", func(t *testing.T) {
		f := newReplacementFixture(t, "old41")
		before := f.base.beforeRequest
		connections := 0
		f.base.beforeRequest = func(w http.ResponseWriter, r *http.Request) bool {
			if strings.HasSuffix(r.URL.Path, "/actions/runner-registration") {
				connections++
				w.WriteHeader(401)
				return true
			}
			return before(w, r)
		}
		if f.run(t, context.Background()) == nil || connections != 1 || len(f.trace.snapshot()) != 0 || f.base.requests.Load() != 6 || f.base.forbidden.Load() != 0 {
			t.Fatal("bootstrap retry crossed physical budget", connections, f.base.requests.Load(), f.trace.snapshot())
		}
	})
}

func TestSessionReplacementStrictJournalAndPhysicalReferences(t *testing.T) {
	for _, name := range []string{"missing-zero", "null-zero", "forbidden-field", "union-field", "changed-owner", "changed-set", "false-result", "fabricated-reference", "raw-prior-bytes", "order"} {
		t.Run(name, func(t *testing.T) {
			f := newReplacementFixture(t, "zero")
			if err := f.run(t, context.Background()); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(f.base.j.directory, "journal.jsonl")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			dir, admission := f.base.j.directory, f.base.j.claim.directory
			f.base.j.Close()
			lines := bytes.Split(data, []byte("\n"))
			// Physical event 18 is the held real message-zero observation.
			var event map[string]any
			if json.Unmarshal(lines[18], &event) != nil {
				t.Fatal("event")
			}
			r := event["replacement"].(map[string]any)
			switch name {
			case "missing-zero":
				delete(r, "message_id")
			case "null-zero":
				r["message_id"] = nil
			case "forbidden-field":
				r["token"] = "forbidden"
			case "union-field":
				event["operation"] = "delete"
			case "changed-owner":
				r["owner"] = strings.Repeat("f", 32)
			case "changed-set":
				r["set_id"] = 99
			case "false-result":
				r["status"] = 204
			case "fabricated-reference":
				r["previous"].(map[string]any)["event_sha256"] = strings.Repeat("0", 64)
			case "raw-prior-bytes":
				lines[3] = append([]byte(" "), lines[3]...)
			case "order":
				lines[18], lines[19] = lines[19], lines[18]
			}
			if name != "raw-prior-bytes" && name != "order" {
				lines[18], _ = json.Marshal(event)
			}
			if os.WriteFile(path, bytes.Join(lines, []byte("\n")), 0600) != nil {
				t.Fatal("rewrite owned fixture")
			}
			if j, err := openJournalAtAdmission(dir, f.base.a, admission, func(file *os.File) error { return file.Sync() }); err == nil {
				j.Close()
				t.Fatal("malformed replacement replay accepted")
			}
		})
	}
}

func TestSessionReplacementRawPreparationAndInspectSlot(t *testing.T) {
	t.Run("raw-snapshot", func(t *testing.T) {
		f := newReplacementFixture(t, "old41")
		path := filepath.Join(filepath.Dir(f.base.j.directory), "approval.json")
		original := f.handoff.snapshot
		if os.WriteFile(path, original, 0600) != nil {
			t.Fatal("approval")
		}
		dir, admission := f.base.j.directory, f.base.j.claim.directory
		f.base.j.Close()
		open := func(dir string, a Approval) (*FileJournal, error) {
			return openJournalAtAdmission(dir, a, admission, func(file *os.File) error { return file.Sync() })
		}
		if _, err := prepareSessionReplacementJournal(dir, f.base.a, path, open); err != nil {
			t.Fatal("exact original raw rejected", err)
		}
		canonical, _ := json.Marshal(f.base.a)
		if bytes.Equal(canonical, original) {
			t.Fatal("fixture did not vary raw bytes")
		}
		os.WriteFile(path, canonical, 0600)
		if _, err := prepareSessionReplacementJournal(dir, f.base.a, path, open); err == nil || f.base.requests.Load() != 0 {
			t.Fatal("canonical digest substituted for raw snapshot")
		}
	})
	t.Run("inspect-once", func(t *testing.T) {
		f := newReplacementFixture(t, "old41")
		if err := f.run(t, context.Background()); err != nil {
			t.Fatal(err)
		}
		driver := &Driver{f.base.a, f.base.j, f.base.api}
		if err := driver.Run(context.Background(), "inspect"); err != nil {
			t.Fatal("existing inspect slot refused", err)
		}
		requests := f.base.requests.Load()
		if driver.Run(context.Background(), "inspect") == nil || f.base.requests.Load() != requests || !replay(f.base.j.Events()).reserved {
			t.Fatal("inspect replay became authority or released work")
		}
	})
}
