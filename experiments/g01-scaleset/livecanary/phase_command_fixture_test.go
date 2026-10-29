//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func experimentalPolicy(t *testing.T, a Approval, raw []byte, mode, phase, role string) string {
	t.Helper()
	request := handoff.Request{ControllerApprovalSHA256: handoff.Digest(raw), Repository: a.Organization + "/" + a.Repository, WorkflowRunID: a.WorkflowRunID, WorkflowRef: a.WorkflowRef, WorkflowSHA: a.WorkflowSHA, WorkflowPath: a.WorkflowPath, Phase: phase, OwnerNonce: a.OwnerNonce, Source: "github-rest-workflow-v1"}
	p := handoff.PublicPolicy{Version: 1, Role: role, Source: request.Source, WorkflowKeyID: "fixture-ed25519-v1", WorkflowPublicKey: "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8", IssuerKeyID: "fixture-issuer-v1", IssuerPublicKey: "e28a8970753332bd72fef413e6b0b2ef1b4aadda7aa2c141f233712a6876b351", Grants: []handoff.PhaseGrant{{Request: request, Mode: mode, OrganizationID: 101, RepositoryID: a.RepositoryID, ExpiresAt: a.ExpiresAt}}}
	data, _ := json.Marshal(p)
	return string(data)
}
func ownedExperimentalPipe(t *testing.T, data []byte) *os.File {
	t.Helper()
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	go func() { defer close(done); w.Write(data); w.Close() }()
	t.Cleanup(func() { r.Close(); w.Close(); <-done })
	return r
}
func experimentalManagement(t *testing.T, verification string) []byte {
	t.Helper()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	defer clear(private)
	raw, _ := json.Marshal(map[string]string{"pem": string(private), "verification_token": verification})
	return raw
}
func writePhaseFixtureConfig(t *testing.T, state, admission, policy, harness string, api *SDKAPI, ca []byte) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"base_url": api.baseURL, "ca_pem": string(ca), "admission_directory": admission, "experimental_policy": policy, "harness_sha": harness})
	if e := os.WriteFile(filepath.Join(state, "paired-fixture.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestExperimentalActualCommandBrokerReceiver(t *testing.T) {
	binary := os.Getenv("G01_PHASE_CONTROLLER_TEST_BINARY")
	if binary == "" {
		t.Skip("requires explicitly built current-source controller TEST executable")
	}
	// Run the actual dispatcher cancellation refusals from both independently
	// compiled TEST executables; the official fixture partition supplies them.
	if brokerBinary := os.Getenv("G01_PHASE_BROKER_TEST_BINARY"); brokerBinary != "" {
		for _, step := range []struct{ binary, test string }{{binary, "^TestExperimentalDispatchCanceledBeforeInput$"}, {brokerBinary, "^TestExperimentalDispatchCanceledAndRootless$"}} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			command := exec.CommandContext(ctx, step.binary, "-test.run="+step.test, "-test.count=1")
			output, e := command.CombinedOutput()
			cancel()
			if e != nil {
				t.Fatalf("actual dispatcher cancellation refusal: %v %s", e, output)
			}
		}
	}
	cases := []string{"before-ack", "inspect", "paired-terminal", "session-replacement", "session-replacement-zero", "session-replacement-new202", "session-replacement-missing", "session-replacement-null", "session-replacement-raw-drift"}
	if os.Getenv("G01_PHASE_BROKER_TEST_BINARY") != "" {
		cases = append(cases, "alias", "workflow-seed", "issuer-seed", "rootless", "unknown-policy")
	}
	for _, caseName := range cases {
		t.Run(caseName, func(t *testing.T) {
			mode := caseName
			variant := "old41"
			if strings.HasPrefix(caseName, "session-replacement-") {
				mode = replacementPhase
				variant = strings.TrimPrefix(caseName, "session-replacement-")
			}
			negative := mode != "before-ack" && mode != "inspect" && mode != "paired-terminal" && mode != "session-replacement"
			if negative {
				mode = "before-ack"
			}
			var terminal *terminalFixture
			var base *baselineFixture
			var replacement *replacementTrace
			var files PairedTerminalFiles
			if mode == "paired-terminal" {
				terminal = newTerminalFixtureWithCreate(t, []string{"create", "start", "inspect", "cleanup"}, nil, func(j *FileJournal, a Approval) { completedHandoffCreate(t, j, a) })
				base = terminal.c
				fixture := newExportedPairedFixture(t, terminal)
				files = fixture.files
			} else {
				a := approval()
				a.WorkflowRef = "refs/heads/main"
				if mode == "session-replacement" {
					a.Phases = append(a.Phases, mode)
				}
				var original []byte
				var change func(string, any) any
				if mode == "session-replacement" {
					change = replacementResponse(a, variant)
				}
				base = newBaselineFixtureWithCreate(t, change, "", a, func(j *FileJournal) { f, _ := completedHandoffCreate(t, j, a); original = f.snapshot })
				base.release()
				base.release = nil
				root := filepath.Dir(base.j.directory)
				files = PairedTerminalFiles{ControllerStateDirectory: base.j.directory, ControllerApprovalPath: filepath.Join(root, "controller-approval.json")}
				raw, _ := json.Marshal(a)
				if mode == "session-replacement" && variant != "raw-drift" {
					raw = original
				}
				if os.WriteFile(files.ControllerApprovalPath, raw, 0600) != nil {
					t.Fatal("approval")
				}
				// Actual Driver Preflight routes, with all other calls on the pinned SDK.
				base.extra = func(w http.ResponseWriter, r *http.Request) bool {
					repo := map[string]any{"id": a.RepositoryID, "full_name": a.Organization + "/" + a.Repository, "private": true, "fork": false}
					var body any
					if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/sessions/") {
						w.WriteHeader(204)
						return true
					}
					if strings.HasSuffix(r.URL.Path, "/agents") {
						json.NewEncoder(w).Encode(map[string]any{"count": 0, "value": []any{}})
						return true
					}
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
				if mode == replacementPhase {
					replacement = attachReplacementTrace(t, base, variant)
				}
			}
			base.readEvents = func() []Event {
				data, e := os.ReadFile(filepath.Join(files.ControllerStateDirectory, "journal.jsonl"))
				if e != nil {
					t.Error("fixture current journal unavailable")
					return nil
				}
				var events []Event
				for index, line := range bytes.Split(data, []byte("\n")) {
					if index == 0 {
						var header journalHeader
						if DecodeStrict(line, &header) != nil {
							t.Error("fixture journal header malformed")
						}
						continue
					}
					if len(line) == 0 {
						continue
					}
					var event Event
					if DecodeStrict(line, &event) != nil || event.Sequence != len(events)+1 || !validEvent(event) {
						t.Error("fixture current journal malformed")
						return nil
					}
					events = append(events, event)
				}
				return events
			}
			a := base.a
			admission := base.j.claim.directory
			workerAdmission := ""
			if terminal != nil {
				workerAdmission = terminal.wf.AdmissionDirectory()
			}
			_ = base.j.Close()
			raw, e := os.ReadFile(files.ControllerApprovalPath)
			if e != nil {
				t.Fatal(e)
			}
			brokerMode, phase := "controller", mode
			if mode == "paired-terminal" {
				brokerMode = mode
			}
			controllerPolicy := experimentalPolicy(t, a, raw, brokerMode, phase, "controller")
			writePhaseFixtureConfig(t, files.ControllerStateDirectory, admission, controllerPolicy, a.HarnessSHA, base.api, base.caPEM)
			if mode == "paired-terminal" {
				writePhaseFixtureConfig(t, files.WorkerStateDirectory, workerAdmission, controllerPolicy, a.HarnessSHA, base.api, base.caPEM)
				path := filepath.Join(files.ControllerStateDirectory, "paired-fixture.json")
				data, _ := os.ReadFile(path)
				var config map[string]string
				_ = json.Unmarshal(data, &config)
				config["worker_admission_directory"] = workerAdmission
				data, _ = json.Marshal(config)
				if os.WriteFile(path, data, 0600) != nil {
					t.Fatal("paired worker admission config")
				}
			}
			parent := filepath.Dir(files.ControllerStateDirectory)
			binData, e := os.ReadFile(binary)
			if e != nil {
				t.Fatal(e)
			}
			binPath := filepath.Join(parent, "experimental-controller-test")
			if os.WriteFile(binPath, binData, 0500) != nil {
				t.Fatal("binary")
			}
			ba := enrollment.BrokerApproval{OwnerNonce: a.OwnerNonce, Mode: brokerMode, Phase: phase, AppID: a.AppID, AppName: "fixture-app", AppOwner: a.Organization, AppOwnerID: 101, InstallationID: a.InstallationID, Organization: a.Organization, OrganizationID: 101, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: int64(a.RunnerGroupID), RunnerGroupName: "fixture-group", ExpiresAt: a.ExpiresAt, ControllerBinarySHA256: handoff.Digest(binData), ControllerApprovalSHA256: handoff.Digest(raw), ControllerHarnessSHA: a.HarnessSHA, AllowVerificationAuthority: true}
			if brokerMode == "paired-terminal" {
				ba.Phase = "paired-terminal"
			}
			brokerApproval, _ := json.Marshal(ba)
			approvalPath := filepath.Join(parent, "broker-"+mode+".json")
			if os.WriteFile(approvalPath, brokerApproval, 0600) != nil {
				t.Fatal("broker approval")
			}
			brokerFiles := enrollment.BrokerFiles{ApprovalPath: approvalPath, StateDirectory: filepath.Join(parent, "attempt-"+mode), ControllerBinary: binPath, ControllerApproval: files.ControllerApprovalPath, ControllerStateDirectory: files.ControllerStateDirectory, WorkerApproval: files.WorkerApprovalPath, WorkerStateDirectory: files.WorkerStateDirectory}
			transport := &processBrokerHTTP{t: t, attempt: brokerFiles.StateDirectory, approval: &a}
			if e := enrollment.ConfigureBrokerExperimentForFixture(experimentalPolicy(t, a, raw, brokerMode, phase, "broker"), transport, admission, workerAdmission); e != nil {
				t.Fatal(e)
			}
			// Synthetic inputs enter only owned pipes, never argv/environment/files.
			workflowSeed, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
			issuerSeed := bytes.Repeat([]byte{71}, ed25519.SeedSize)
			if caseName == "workflow-seed" {
				workflowSeed = bytes.Repeat([]byte{8}, 32)
			}
			if caseName == "issuer-seed" {
				issuerSeed = bytes.Repeat([]byte{9}, 32)
			}
			management := experimentalManagement(t, base.api.credentials.VerificationToken)
			defer clear(management)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			var result enrollment.BrokerResult
			brokerBinary := os.Getenv("G01_PHASE_BROKER_TEST_BINARY")
			if brokerBinary == "" {
				result, e = enrollment.RunBrokerExperimentWithInputs(ctx, brokerFiles, ownedExperimentalPipe(t, management), ownedExperimentalPipe(t, []byte("synthetic-observation-credential")), ownedExperimentalPipe(t, workflowSeed), ownedExperimentalPipe(t, issuerSeed))
			} else {
				var mu sync.Mutex
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					response, err := transport.RoundTrip(r)
					if err != nil {
						w.WriteHeader(500)
						return
					}
					defer response.Body.Close()
					for k, v := range response.Header {
						w.Header()[k] = v
					}
					w.WriteHeader(response.StatusCode)
					io.Copy(w, response.Body)
				}))
				defer server.Close()
				policy := experimentalPolicy(t, a, raw, brokerMode, phase, "broker")
				if caseName == "unknown-policy" {
					policy = strings.Replace(policy, `"version":1`, `"version":1,"unreviewed":true`, 1)
				}
				config, _ := json.Marshal(map[string]string{"policy": policy, "base_url": server.URL, "ca_pem": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})), "admission": admission, "worker_admission": workerAdmission})
				if os.WriteFile(filepath.Join(files.ControllerStateDirectory, "broker-experiment-fixture.json"), config, 0600) != nil {
					t.Fatal("broker fixture")
				}
				args := []string{"--execute-experimental-broker", "--approval", brokerFiles.ApprovalPath, "--state-dir", brokerFiles.StateDirectory, "--controller-binary", brokerFiles.ControllerBinary, "--controller-approval", brokerFiles.ControllerApproval, "--controller-state-dir", brokerFiles.ControllerStateDirectory}
				if mode == "paired-terminal" {
					args = append(args, "--worker-approval", brokerFiles.WorkerApproval, "--worker-state-dir", brokerFiles.WorkerStateDirectory)
				}
				command := exec.CommandContext(ctx, brokerBinary, args...)
				command.Env = []string{"LANG=C", "LC_ALL=C"}
				command.Stdin = ownedExperimentalPipe(t, management)
				command.ExtraFiles = []*os.File{ownedExperimentalPipe(t, []byte("synthetic-observation-credential")), ownedExperimentalPipe(t, workflowSeed), ownedExperimentalPipe(t, issuerSeed)}
				if caseName == "alias" {
					command.ExtraFiles[0] = command.Stdin.(*os.File)
				}
				if caseName == "rootless" {
					if os.Remove(filepath.Join(files.ControllerStateDirectory, "broker-experiment-fixture.json")) != nil {
						t.Fatal("rootless fixture")
					}
				}
				var output []byte
				output, e = command.CombinedOutput()
				if e == nil {
					e = json.Unmarshal(output, &result)
				}
				if negative {
					unread, readErr := io.ReadAll(command.Stdin.(*os.File))
					if readErr != nil || !bytes.Equal(unread, management) {
						t.Fatal("invalid fixed inputs read management credentials before admission")
					}
					if caseName != "alias" {
						unread, readErr = io.ReadAll(command.ExtraFiles[0])
						if readErr != nil || string(unread) != "synthetic-observation-credential" {
							t.Fatal("invalid fixed inputs read observation credentials before role/policy checks")
						}
					}
				}
				mu.Lock()
				defer mu.Unlock()
			}
			if negative {
				if e == nil || transport.mints != 0 || len(transport.calls) != 0 || base.requests.Load() != 0 {
					t.Fatalf("invalid fixed inputs crossed effects: case=%s err=%v mints=%d calls=%v", caseName, e, transport.mints, transport.calls)
				}
				return
			}
			if mode == replacementPhase && variant == "raw-drift" {
				if e == nil || transport.mints != 0 || base.requests.Load() != 0 {
					t.Fatal("raw approval mismatch crossed preparation/remint boundary")
				}
				return
			}
			if mode == replacementPhase && (variant == "missing" || variant == "null") {
				if e == nil || transport.mints != 1 || !reflect.DeepEqual(replacement.snapshot(), []string{"set", "worker", "open1", "poll1"}) || base.acks.Load() != 0 || base.acquires.Load() != 0 {
					t.Fatal("missing/null identity crossed replacement boundary", replacement.snapshot())
				}
				return
			}
			if e != nil || transport.mints != 1 {
				for _, path := range []string{filepath.Join(brokerFiles.StateDirectory, "broker.jsonl"), filepath.Join(files.ControllerStateDirectory, "journal.jsonl")} {
					data, _ := os.ReadFile(path)
					for _, line := range bytes.Split(data, []byte("\n")) {
						var fields map[string]json.RawMessage
						if json.Unmarshal(line, &fields) == nil {
							t.Logf("safe stage: kind=%s operation=%s event=%s", fields["kind"], fields["operation"], fields["phase"])
						}
					}
				}
				t.Logf("safe counts: requests=%d forbidden=%d", base.requests.Load(), base.forbidden.Load())
				t.Fatalf("actual %s command failed: %v result=%+v mints=%d calls=%v", mode, e, result, transport.mints, transport.calls)
			}
			data, e := os.ReadFile(filepath.Join(files.ControllerStateDirectory, "journal.jsonl"))
			if e != nil {
				t.Fatal(e)
			}
			if strings.Count(string(data), `"phase":"`+phase+`"`) != 1 {
				t.Fatal("phase singleton missing")
			}
			if mode == "before-ack" && (!bytes.Contains(data, []byte(`"kind":"barrier"`)) || base.acks.Load() != 0 || base.acquires.Load() != 0) {
				t.Fatal("fault barrier not exercised")
			}
			if mode == "session-replacement" && (base.sessions.Load() != 2 || base.polls.Load() != 2 || base.acks.Load() != 0 || base.acquires.Load() != 0 || !bytes.Contains(data, []byte(`"stage":"complete"`))) {
				t.Fatal("actual two-generation replacement trace incomplete")
			}
			if replacement != nil && !reflect.DeepEqual(replacement.snapshot(), replacementSuccessTrace) {
				t.Fatal("actual physical two-generation trace differed", replacement.snapshot())
			}
			if mode == "paired-terminal" && (terminal.c.acks.Load() != 2 || terminal.c.acquires.Load() != 1 || terminal.jit.Load() != 1 || terminal.creates.Load() != 1 || terminal.starts.Load() != 1 || terminal.sessionDeletes.Load() != 1 || terminal.workerDeletes.Load() != 1 || terminal.setDeletes.Load() != 1 || terminal.setAbsences.Load() != 1 || terminal.workerAbsences.Load() != 1 || terminal.rosters.Load() != 4) {
				t.Fatal("full paired command incomplete")
			}
		})
	}
}
