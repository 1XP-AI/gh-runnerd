//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
)

func TestJITBoundaryPhysicalMutationsStop(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*http.Request) *http.Request
	}{
		{"unmarked", func(r *http.Request) *http.Request { r.Header.Del(baselineWireMarkerHeader); return r }},
		{"context-stripped", func(r *http.Request) *http.Request { return r.Clone(context.Background()) }},
		{"foreign-permit", func(r *http.Request) *http.Request {
			return r.WithContext(context.WithValue(r.Context(), jitBoundaryRequestKey{}, &jitBoundaryPermit{}))
		}},
		{"foreign-capture", func(r *http.Request) *http.Request {
			return r.WithContext(context.WithValue(r.Context(), baselineWireKey{}, &baselineWireCapture{}))
		}},
		{"host", func(r *http.Request) *http.Request { r.Host = "foreign.example"; return r }},
		{"origin", func(r *http.Request) *http.Request { r.URL.Host = "foreign.example"; r.Host = ""; return r }},
		{"tenant", func(r *http.Request) *http.Request {
			r.URL.Path = strings.Replace(r.URL.Path, "/tenant/", "/foreign/", 1)
			return r
		}},
		{"query", func(r *http.Request) *http.Request { r.URL.RawQuery += "&extra=1"; return r }},
		{"duplicate-query", func(r *http.Request) *http.Request { r.URL.RawQuery += "&api-version=6.0-preview"; return r }},
		{"escaped-path", func(r *http.Request) *http.Request {
			r.URL.RawPath = strings.Replace(r.URL.Path, "tenant", "%74enant", 1)
			return r
		}},
		{"opaque", func(r *http.Request) *http.Request { r.URL.Opaque = "//foreign.example/"; return r }},
		{"userinfo", func(r *http.Request) *http.Request { r.URL.Fragment = "foreign"; return r }},
		{"auth", func(r *http.Request) *http.Request { r.Header.Set("Authorization", "Bearer foreign"); return r }},
		{"duplicate-auth", func(r *http.Request) *http.Request {
			r.Header.Add("Authorization", r.Header.Get("Authorization"))
			return r
		}},
		{"folded-content-type", func(r *http.Request) *http.Request { r.Header["content-type"] = []string{"application/json"}; return r }},
		{"capacity", func(r *http.Request) *http.Request { r.Header.Set(scaleset.HeaderScaleSetMaxCapacity, "1"); return r }},
		{"idempotency", func(r *http.Request) *http.Request { r.Header.Set("Idempotency-Key", "synthetic"); return r }},
		{"x-idempotency", func(r *http.Request) *http.Request { r.Header.Set("X-Idempotency-Key", "synthetic"); return r }},
		{"retry", func(r *http.Request) *http.Request { r.Header.Set("Retry-After", "1"); return r }},
		{"body-name", func(r *http.Request) *http.Request {
			data := []byte(`{"name":"foreign","workFolder":"_work"}`)
			installBaselineRequestBody(r, data)
			return r
		}},
		{"body-folder", func(r *http.Request) *http.Request {
			p := r.Context().Value(jitBoundaryRequestKey{}).(*jitBoundaryPermit)
			data, _ := json.Marshal(map[string]string{"name": p.scope.approval.workerName(), "workFolder": "foreign"})
			installBaselineRequestBody(r, data)
			return r
		}},
		{"body-null", func(r *http.Request) *http.Request {
			installBaselineRequestBody(r, []byte(`{"name":null,"workFolder":"_work"}`))
			return r
		}},
		{"body-folded", func(r *http.Request) *http.Request {
			p := r.Context().Value(jitBoundaryRequestKey{}).(*jitBoundaryPermit)
			data, _ := json.Marshal(map[string]string{"Name": p.scope.approval.workerName(), "workFolder": "_work"})
			installBaselineRequestBody(r, data)
			return r
		}},
		{"body-framing", func(r *http.Request) *http.Request { r.ContentLength++; return r }},
		{"body-chunked", func(r *http.Request) *http.Request { r.TransferEncoding = []string{"chunked"}; return r }},
		{"content-type", func(r *http.Request) *http.Request { r.Header.Set("Content-Type", "text/plain"); return r }},
		{"method", func(r *http.Request) *http.Request { r.Method = "PUT"; return r }},
	}
	for _, phase := range []string{beforeJITPhase, "jit-loss"} {
		for _, tc := range cases {
			t.Run(phase+"/"+tc.name, func(t *testing.T) {
				f := newJITBoundaryFixture(t, phase, "")
				hit := false
				var stopped int32
				ctx := context.WithValue(context.Background(), jitBoundaryMutationKey{}, func(inner http.RoundTripper) http.RoundTripper {
					return baselineRoundTrip(func(r *http.Request) (*http.Response, error) {
						if strings.HasSuffix(r.URL.Path, "/generatejitconfig") {
							hit = true
							stopped = f.base.requests.Load()
							r = tc.mutate(r)
						}
						return inner.RoundTrip(r)
					})
				})
				if f.run(t, ctx) == nil || !hit || f.base.requests.Load() != stopped {
					t.Fatal("physical mutation crossed next request", hit, stopped, f.base.requests.Load())
				}
				jit, workers, _ := f.counts()
				if jit != 0 || workers != 1 {
					t.Fatal("physical mutation forwarded JIT or post-error lookup", jit, workers)
				}
			})
		}
	}
}

func TestJITBoundaryBootstrapAndResponseFailures(t *testing.T) {
	for _, stage := range []string{"registration-token", "runner-registration", "generatejitconfig"} {
		for _, fault := range []string{"401", "403", "429", "500", "redirect", "disconnect", "null", "missing", "duplicate", "near-expiry", "url", "oversize", "malformed"} {
			if (fault == "near-expiry" || fault == "url") && stage != "runner-registration" {
				continue
			}
			t.Run(stage+"/"+fault, func(t *testing.T) {
				f := newJITBoundaryFixture(t, "jit-loss", "")
				hits := 0
				f.before = func(w http.ResponseWriter, r *http.Request) bool {
					if !strings.HasSuffix(r.URL.Path, "/"+stage) {
						return false
					}
					hits++
					switch fault {
					case "401", "403", "429", "500":
						status := map[string]int{"401": 401, "403": 403, "429": 429, "500": 500}[fault]
						w.WriteHeader(status)
					case "redirect":
						w.Header().Set("Location", f.base.api.baseURL+"/foreign")
						w.WriteHeader(307)
					case "disconnect":
						conn, _, err := w.(http.Hijacker).Hijack()
						if err == nil {
							conn.Close()
						}
					case "null":
						if stage == "registration-token" {
							w.WriteHeader(201)
						}
						io.WriteString(w, `{"token":null,"url":null}`)
					case "missing":
						if stage == "registration-token" {
							w.WriteHeader(201)
						}
						io.WriteString(w, `{}`)
					case "duplicate":
						if stage == "registration-token" {
							w.WriteHeader(201)
						}
						io.WriteString(w, `{"token":"synthetic","TOKEN":"other"}`)
					case "near-expiry", "url":
						claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(30 * time.Second).Unix()})
						url := f.base.api.baseURL + "/tenant/"
						if fault == "url" {
							url = "https://foreign.example/tenant/"
						}
						json.NewEncoder(w).Encode(map[string]string{"url": url, "token": "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."})
					case "oversize":
						io.WriteString(w, strings.Repeat("x", int(responseBodyLimit)+1))
					case "malformed":
						io.WriteString(w, `{"token":`)
					}
					return true
				}
				// A caller cannot restore SDK retry authority via its context.
				ctx := scaleset.WithRequestRetry(context.Background(), func(c *scaleset.RetryConfig) { c.Max = 2; c.WaitMin = time.Millisecond; c.WaitMax = time.Millisecond })
				if f.run(t, ctx) == nil || hits != 1 {
					t.Fatal("bootstrap/JIT failure retried or completed", hits)
				}
				_, workers, total := f.counts()
				want := 5
				if stage == "runner-registration" {
					want = 6
				}
				if stage == "generatejitconfig" {
					want = 9
				}
				if total != want || workers > 1 {
					t.Fatal("failure caused next traffic", total, want, workers)
				}
			})
		}
	}
}

func TestJITBoundaryStrictWireObservations(t *testing.T) {
	for _, stage := range []string{"set", "worker"} {
		for _, fault := range []string{"missing", "null", "duplicate", "nonzero", "owner", "count", "value", "label", "updates"} {
			t.Run(stage+"/"+fault, func(t *testing.T) {
				f := newJITBoundaryFixture(t, beforeJITPhase, "")
				f.before = func(w http.ResponseWriter, r *http.Request) bool {
					if stage == "set" && !strings.HasSuffix(r.URL.Path, "/runnerscalesets/7") || stage == "worker" && !strings.HasSuffix(r.URL.Path, "/agents") {
						return false
					}
					if stage == "worker" {
						raw := `{"count":0,"value":[]}`
						switch fault {
						case "missing":
							raw = `{"value":[]}`
						case "null":
							raw = `{"count":0,"value":null}`
						case "duplicate":
							raw = `{"count":0,"COUNT":0,"value":[]}`
						case "nonzero", "owner", "label", "updates":
							raw = `{"count":1,"value":[{"id":99,"name":"foreign","runnerScaleSetId":7}]}`
						case "count":
							raw = `{"count":1,"value":[]}`
						case "value":
							raw = `{"count":0}`
						}
						io.WriteString(w, raw)
					} else {
						stats := map[string]any{"totalAvailableJobs": 0, "totalAcquiredJobs": 0, "totalAssignedJobs": 0, "totalRunningJobs": 0, "totalRegisteredRunners": 0, "totalBusyRunners": 0, "totalIdleRunners": 0}
						body := map[string]any{"id": 7, "name": f.base.a.setName(), "runnerGroupId": 3, "labels": []any{map[string]any{"name": f.base.a.setName(), "type": "System"}}, "runnerSetting": map[string]any{"disableUpdate": true}, "statistics": stats}
						switch fault {
						case "missing":
							delete(stats, "totalAssignedJobs")
						case "null":
							stats["totalAssignedJobs"] = nil
						case "duplicate":
							stats["TOTALASSIGNEDJOBS"] = 0
						case "nonzero":
							stats["totalAvailableJobs"] = 1
						case "owner":
							body["name"] = "foreign"
						case "count":
							body["id"] = 8
						case "value":
							body["statistics"] = nil
						case "label":
							body["labels"] = []any{}
						case "updates":
							body["runnerSetting"] = map[string]any{"disableUpdate": false}
						}
						json.NewEncoder(w).Encode(body)
					}
					return true
				}
				if f.run(t, context.Background()) == nil {
					t.Fatal("incomplete/nonzero/foreign wire observation admitted")
				}
				jit, workers, total := f.counts()
				want := 7
				if stage == "worker" {
					want = 8
				}
				if jit != 0 || workers != 0 || total != want {
					t.Fatal("wire refusal crossed next traffic", jit, workers, total, want)
				}
			})
		}
	}
}

func TestJITBoundaryExpiryAtEveryAppend(t *testing.T) {
	for _, phase := range []string{beforeJITPhase, "jit-loss"} {
		for target := 1; target <= len(jitBoundarySteps(phase))+2; target++ {
			t.Run(fmt.Sprintf("%s/%02d", phase, target), func(t *testing.T) {
				checkJITBoundaryExpiryAtAppend(t, phase, target, 0)
			})
		}
	}
	t.Run("slow-append", func(t *testing.T) {
		// Reproduce pre-target wall-clock exhaustion at the hosted failure's
		// late target: hold the preceding append until 2.1 seconds after
		// context creation, without changing context errors or the real fsync.
		checkJITBoundaryExpiryAtAppend(t, "jit-loss", 16, 2100*time.Millisecond)
	})
}

func checkJITBoundaryExpiryAtAppend(t *testing.T, phase string, target int, minPreTargetElapsed time.Duration) {
	t.Helper()
	f := newJITBoundaryFixture(t, phase, "")
	// Allow pre-target fsync/network setup to exceed the old two-second
	// deadline. All 31 targets plus the slow-append witness budget 96 seconds
	// on these deadlines, leaving 24 seconds of the 120-second
	// invocation cap for fixture setup, cleanup and scheduling overhead.
	// This remains a finite scheduling allowance, not a flake-proof bound.
	started := time.Now()
	ctx, cancel := context.WithDeadline(context.Background(), started.Add(3*time.Second))
	defer cancel()
	calls, stopped := 0, int32(-1)
	f.base.j.syncFile = func(file *os.File) error {
		err := file.Sync()
		calls++
		if calls == target-1 && minPreTargetElapsed > 0 {
			time.Sleep(time.Until(started.Add(minPreTargetElapsed)))
		}
		if calls == target {
			stopped = f.base.requests.Load()
			<-ctx.Done()
		}
		return err
	}
	runErr := f.run(t, ctx)
	if stopped < 0 {
		t.Fatalf("expiry fixture exhausted before target append: phase=%s target=%d calls=%d requests=%d deadlineExceeded=%t", phase, target, calls, f.base.requests.Load(), ctx.Err() == context.DeadlineExceeded)
	}
	if runErr == nil || ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("target append did not refuse after real deadline: phase=%s target=%d calls=%d refused=%t deadlineExceeded=%t", phase, target, calls, runErr != nil, ctx.Err() == context.DeadlineExceeded)
	}
	if f.base.requests.Load() != stopped {
		t.Fatalf("expiry crossed physical boundary: phase=%s target=%d calls=%d stopped=%d requests=%d", phase, target, calls, stopped, f.base.requests.Load())
	}
}

func TestJITBoundaryIdentityAndCapacityAtJIT(t *testing.T) {
	for _, fault := range []string{"raw-rewrite", "approval-inode", "journal-inode", "claim-inode", "state-inode", "memory-prefix", "physical-prefix", "partial-tail", "capacity", "poisoned", "original-api", "original-rest", "original-transport", "dedicated-client", "dedicated-transport", "keepalive", "tls", "parent-cancel", "original-parent-cancel", "token-expiry", "proof-expiry"} {
		t.Run(fault, func(t *testing.T) {
			f := newJITBoundaryFixture(t, "jit-loss", "")
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			hit := false
			var stopped int32
			ctx := context.WithValue(parent, jitBoundaryWireProbeKey{}, func(p *jitBoundaryPermit) {
				if p.wire.stage != "jit" {
					return
				}
				hit = true
				stopped = f.base.requests.Load()
				s := p.scope
				j := s.journal
				replace := func(path string) {
					data, _ := os.ReadFile(path)
					if os.Rename(path, path+".held") != nil || os.WriteFile(path, data, 0600) != nil {
						t.Fatal("identity mutation")
					}
				}
				switch fault {
				case "raw-rewrite":
					data, _ := json.Marshal(f.base.a)
					os.WriteFile(f.path, data, 0600)
				case "approval-inode":
					replace(f.path)
				case "journal-inode":
					replace(filepath.Join(j.directory, "journal.jsonl"))
				case "claim-inode":
					replace(j.claim.file.Name())
				case "state-inode":
					os.Rename(j.directory, j.directory+".held")
					os.Mkdir(j.directory, 0700)
				case "memory-prefix":
					j.mu.Lock()
					j.events[6].ID++
					j.mu.Unlock()
				case "physical-prefix":
					file, err := os.OpenFile(j.file.Name(), os.O_WRONLY, 0)
					if err != nil {
						t.Fatal(err)
					}
					file.WriteAt([]byte(" "), 1)
					file.Close()
				case "partial-tail":
					j.file.Write([]byte("{"))
				case "capacity":
					j.file.Truncate(baselineJournalLimit - 1)
				case "poisoned":
					j.writeFailed = true
				case "original-api":
					s.driver.API = &SDKAPI{}
				case "original-rest":
					s.api.rest = &http.Client{}
				case "original-transport":
					s.api.physical = s.api.physical.Clone()
				case "dedicated-client":
					s.network.client = nil
				case "dedicated-transport":
					s.network.physical = s.network.physical.Clone()
				case "keepalive":
					s.network.physical.DisableKeepAlives = false
				case "tls":
					s.network.physical.TLSClientConfig.InsecureSkipVerify = true
				case "parent-cancel":
					cancel()
				case "original-parent-cancel":
					s.ctx = context.WithValue(context.Background(), jitBoundaryGuardKey{}, s.ctx.Value(jitBoundaryGuardKey{}))
					cancel()
				case "token-expiry":
					s.managementExpires = time.Now()
				case "proof-expiry":
					s.proof.EffectiveDeadline = time.Now()
				}
			})
			if f.run(t, ctx) == nil || !hit || f.base.requests.Load() != stopped {
				t.Fatal("identity/capacity/lifetime drift allowed next traffic", hit, stopped, f.base.requests.Load())
			}
		})
	}
}

func TestJITBoundaryRawPhysicalRecordLimit(t *testing.T) {
	f := newJITBoundaryFixture(t, beforeJITPhase, "")
	if f.run(t, context.Background()) != nil {
		t.Fatal("setup")
	}
	j := f.base.j
	dir, admission := j.directory, j.claim.directory
	data, _ := os.ReadFile(j.file.Name())
	j.Close()
	lines := bytes.Split(data, []byte{'\n'})
	lines[10] = append(bytes.Repeat([]byte{' '}, jitBoundaryRecordLimit), lines[10]...)
	os.WriteFile(filepath.Join(dir, "journal.jsonl"), bytes.Join(lines, []byte{'\n'}), 0600)
	if opened, err := openJournalAtAdmission(dir, f.base.a, admission, func(file *os.File) error { return file.Sync() }); err == nil {
		opened.Close()
		t.Fatal("oversize raw physical line admitted")
	}
}

func TestJITBoundaryDuplicatePhysicalTupleStops(t *testing.T) {
	for _, suffix := range []string{"registration-token", "runnerscalesets/7", "agents", "generatejitconfig"} {
		t.Run(suffix, func(t *testing.T) {
			f := newJITBoundaryFixture(t, "jit-loss", "")
			hit := false
			ctx := context.WithValue(context.Background(), jitBoundaryMutationKey{}, func(inner http.RoundTripper) http.RoundTripper {
				return baselineRoundTrip(func(r *http.Request) (*http.Response, error) {
					response, err := inner.RoundTrip(r)
					if !hit && strings.HasSuffix(r.URL.Path, "/"+suffix) {
						hit = true
						if response != nil {
							response.Body.Close()
						}
						duplicate, second := inner.RoundTrip(r)
						if duplicate != nil {
							duplicate.Body.Close()
						}
						if second == nil {
							t.Error("duplicate physical tuple accepted")
						}
						return nil, ErrQuarantine
					}
					return response, err
				})
			})
			if f.run(t, ctx) == nil || !hit {
				t.Fatal("duplicate test did not stop")
			}
			_, workers, total := f.counts()
			want := map[string]int{"registration-token": 5, "runnerscalesets/7": 7, "agents": 8, "generatejitconfig": 9}[suffix]
			if total != want || workers > 1 {
				t.Fatal("duplicate tuple caused next traffic", total, want, workers)
			}
		})
	}
}
func TestJITBoundaryCompleteJITResponseSchema(t *testing.T) {
	for _, fault := range []string{"missing-runner", "null-runner", "missing-id", "null-id", "zero-id", "foreign-name", "foreign-set", "missing-config", "null-config", "bad-config", "extra", "nested-extra", "folded", "truncated", "close-loss"} {
		t.Run(fault, func(t *testing.T) {
			f := newJITBoundaryFixture(t, "jit-loss", "")
			hits := 0
			f.before = func(w http.ResponseWriter, r *http.Request) bool {
				if !strings.HasSuffix(r.URL.Path, "/generatejitconfig") {
					return false
				}
				hits++
				runner := map[string]any{"id": 991, "name": f.base.a.workerName(), "runnerScaleSetId": 7}
				body := map[string]any{"runner": runner, "encodedJITConfig": jitBoundaryFixtureSecret}
				switch fault {
				case "missing-runner":
					delete(body, "runner")
				case "null-runner":
					body["runner"] = nil
				case "missing-id":
					delete(runner, "id")
				case "null-id":
					runner["id"] = nil
				case "zero-id":
					runner["id"] = 0
				case "foreign-name":
					runner["name"] = "foreign"
				case "foreign-set":
					runner["runnerScaleSetId"] = 8
				case "missing-config":
					delete(body, "encodedJITConfig")
				case "null-config":
					body["encodedJITConfig"] = nil
				case "bad-config":
					body["encodedJITConfig"] = "invalid"
				case "extra":
					body["unexpected"] = true
				case "nested-extra":
					runner["unexpected"] = true
				case "folded":
					delete(body, "encodedJITConfig")
					body["EncodedJITConfig"] = jitBoundaryFixtureSecret
				case "truncated", "close-loss":
					raw, _ := json.Marshal(body)
					w.Header().Set("Content-Length", fmt.Sprint(len(raw)+100))
					w.WriteHeader(200)
					w.Write(raw[:len(raw)/2])
					return true
				}
				json.NewEncoder(w).Encode(body)
				return true
			}
			if f.run(t, context.Background()) == nil || hits != 1 {
				t.Fatal("invalid JIT schema completed")
			}
			_, workers, total := f.counts()
			if workers != 1 || total != 9 {
				t.Fatal("malformed response caused lookup/retry", workers, total)
			}
			events := f.base.j.Events()
			if events[len(events)-1].JITBoundary.Outcome != "send-intent" {
				t.Fatal("invalid response claimed suppression")
			}
		})
	}
}

type jitBoundaryFailBody struct {
	io.ReadCloser
	readFail, closeFail bool
	onClose             func()
}

func (b *jitBoundaryFailBody) Read(p []byte) (int, error) {
	if b.readFail {
		return 0, io.ErrUnexpectedEOF
	}
	return b.ReadCloser.Read(p)
}
func (b *jitBoundaryFailBody) Close() error {
	err := b.ReadCloser.Close()
	if b.onClose != nil {
		b.onClose()
	}
	if b.closeFail {
		return io.ErrClosedPipe
	}
	return err
}
func TestJITBoundaryPostBodyReadCloseAndCancellation(t *testing.T) {
	for _, fault := range []string{"read", "close", "cancel", "raw-change"} {
		t.Run(fault, func(t *testing.T) {
			f := newJITBoundaryFixture(t, "jit-loss", "")
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			hit := false
			ctx := context.WithValue(parent, jitBoundaryMutationKey{}, func(inner http.RoundTripper) http.RoundTripper {
				return baselineRoundTrip(func(r *http.Request) (*http.Response, error) {
					if strings.HasSuffix(r.URL.Path, "/generatejitconfig") {
						hit = true
						body := &jitBoundaryFailBody{ReadCloser: r.Body, readFail: fault == "read", closeFail: fault == "close"}
						if fault == "cancel" {
							body.onClose = cancel
						}
						if fault == "raw-change" {
							body.onClose = func() { raw, _ := json.Marshal(f.base.a); os.WriteFile(f.path, raw, 0600) }
						}
						r.Body = body
					}
					return inner.RoundTrip(r)
				})
			})
			if f.run(t, ctx) == nil || !hit {
				t.Fatal("body/lifetime mutation completed")
			}
			jit, workers, total := f.counts()
			if jit != 0 || workers != 1 || total != 8 {
				t.Fatal("body/lifetime failure crossed physical entry", jit, workers, total)
			}
		})
	}
}
