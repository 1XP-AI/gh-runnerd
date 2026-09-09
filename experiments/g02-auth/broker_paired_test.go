package enrollment

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func pairedPlanInputs(t *testing.T) (BrokerApproval, controllerApproval, string, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	controllerState := filepath.Join(root, "controller-state")
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Hour)
	nonce := strings.Repeat("a", 32)
	harness := strings.Repeat("b", 40)
	workflow := strings.Repeat("c", 40)
	a := brokerApprovalFixture()
	a.Mode, a.Phase = "paired-terminal", "paired-terminal"
	a.ExpiresAt = now
	a.ControllerHarnessSHA = harness
	c := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: nonce, HarnessSHA: harness, WorkflowSHA: workflow, WorkflowPath: ".github/workflows/canary.yml", Controller: "trusted-controller", ExpiresAt: now, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "inspect", "cleanup", "jit-loss"}}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: harness, WorkflowSHA: workflow, OwnerNonce: nonce, Controller: c.Controller, Endpoint: "/tmp/g01-paired-docker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: now, Phases: []string{"create", "start", "inspect", "cleanup"}}
	data, err := json.Marshal(worker)
	if err != nil {
		t.Fatal(err)
	}
	workerPath := filepath.Join(root, "worker.json")
	if err := os.WriteFile(workerPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	return a, c, controllerState, workerState, workerPath
}

func TestPairedWorkerBindingRetainsApprovalAndStateIdentity(t *testing.T) {
	a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
	plan, err := openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c)
	if err != nil {
		t.Fatal("valid paired worker plan refused")
	}
	defer plan.close()
	if _, err := plan.binding(); err != nil {
		t.Fatal("valid paired worker binding unavailable")
	}
	if err := os.Rename(workerPath, workerPath+".retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, plan.raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.binding(); err == nil {
		t.Fatal("replaced worker approval retained authority")
	}
}

func TestPairedWorkerPreparationReceiptFencesJournalMutation(t *testing.T) {
	for _, kind := range []string{"hash", "replacement"} {
		t.Run(kind, func(t *testing.T) {
			a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
			plan, err := openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c)
			if err != nil {
				t.Fatal("valid paired worker plan refused")
			}
			defer plan.close()
			admission := filepath.Join(filepath.Dir(workerState), "receipt-worker-admission")
			plan.prepare = func(context.Context) (brokerPreparationReceipt, error) {
				return brokerSyntheticWorkerPreparation(t, plan, admission)
			}
			if err := plan.checkPrepared(context.Background()); err != nil {
				t.Fatalf("fresh worker preparation refused: %v", err)
			}
			journalPath := filepath.Join(workerState, "journal.jsonl")
			data, err := os.ReadFile(journalPath)
			if err != nil {
				t.Fatal("worker journal")
			}
			switch kind {
			case "hash":
				if err := os.WriteFile(journalPath, append(data, 'x'), 0600); err != nil {
					t.Fatal("mutate worker journal")
				}
			case "replacement":
				if err := os.Rename(journalPath, journalPath+".retained"); err != nil || os.WriteFile(journalPath, data, 0600) != nil {
					t.Fatal("replace worker journal")
				}
			}
			if err := plan.checkPrepared(context.Background()); err == nil {
				t.Fatal("worker journal mutation crossed receipt fence")
			}
		})
	}
}

func TestPairedWorkerPreparationReceiptFencesClaimMutation(t *testing.T) {
	for _, kind := range []string{"hash", "replacement", "missing", "malformed", "locked"} {
		t.Run(kind, func(t *testing.T) {
			a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
			plan, err := openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c)
			if err != nil {
				t.Fatal("valid paired worker plan refused")
			}
			defer plan.close()
			admission := filepath.Join(filepath.Dir(workerState), "receipt-worker-claim")
			plan.prepare = func(context.Context) (brokerPreparationReceipt, error) {
				return brokerSyntheticWorkerPreparation(t, plan, admission)
			}
			if err := plan.checkPrepared(context.Background()); err != nil {
				t.Fatalf("fresh worker preparation refused: %v", err)
			}
			claimPath := filepath.Join(admission, "admission.json")
			data, err := os.ReadFile(claimPath)
			if err != nil {
				t.Fatal("worker admission claim")
			}
			var held *os.File
			switch kind {
			case "hash":
				if err := os.WriteFile(claimPath, append(data, 'x'), 0600); err != nil {
					t.Fatal("mutate worker claim")
				}
			case "replacement":
				if err := os.Rename(claimPath, claimPath+".retained"); err != nil || os.WriteFile(claimPath, data, 0600) != nil {
					t.Fatal("replace worker claim")
				}
			case "missing":
				if err := os.Remove(claimPath); err != nil {
					t.Fatal("remove worker claim")
				}
			case "malformed":
				if err := os.WriteFile(claimPath, []byte("{"), 0600); err != nil {
					t.Fatal("malform worker claim")
				}
			case "locked":
				held, err = os.OpenFile(claimPath, os.O_RDWR, 0)
				if err != nil || syscall.Flock(int(held.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
					t.Fatal("lock worker claim")
				}
			}
			if err := plan.checkPrepared(context.Background()); err == nil {
				t.Fatal("worker admission claim mutation crossed receipt fence")
			}
			if held != nil {
				syscall.Flock(int(held.Fd()), syscall.LOCK_UN)
				held.Close()
				if err := plan.checkPrepared(context.Background()); err != nil {
					t.Fatalf("released worker claim lock remained fail-stop: %v", err)
				}
			}
		})
	}
}

func TestPairedWorkerPreparationReceiptFencesClaimRelocation(t *testing.T) {
	for _, kind := range []string{"relocated-inode", "relocated-directory"} {
		t.Run(kind, func(t *testing.T) {
			a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
			plan, err := openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c)
			if err != nil {
				t.Fatal("valid paired worker plan refused")
			}
			defer plan.close()
			admission := filepath.Join(filepath.Dir(workerState), "canonical-worker-claim")
			plan.prepare = func(context.Context) (brokerPreparationReceipt, error) {
				return brokerSyntheticWorkerPreparation(t, plan, admission)
			}
			if err := plan.checkPrepared(context.Background()); err != nil {
				t.Fatalf("fresh worker preparation refused: %v", err)
			}
			claimPath := filepath.Join(admission, "admission.json")
			fallback := filepath.Join(filepath.Dir(workerState), "worker-admission")
			switch kind {
			case "relocated-inode":
				if err := os.Mkdir(fallback, 0700); err != nil {
					t.Fatal("fallback worker admission")
				}
				if err := os.Rename(claimPath, filepath.Join(fallback, "admission.json")); err != nil {
					t.Fatal("relocate worker claim inode")
				}
				if err := os.WriteFile(claimPath, []byte("{}"), 0600); err != nil {
					t.Fatal("replace canonical worker claim path")
				}
			case "relocated-directory":
				if err := os.Rename(admission, fallback); err != nil {
					t.Fatal("relocate worker claim directory")
				}
				if err := os.Mkdir(admission, 0700); err != nil {
					t.Fatal("replacement worker claim directory")
				}
				if err := os.WriteFile(claimPath, []byte("{}"), 0600); err != nil {
					t.Fatal("replace canonical worker claim path")
				}
			}
			if err := plan.checkPrepared(context.Background()); err == nil {
				t.Fatal("relocated receipt inode bypassed changed canonical claim path")
			}
		})
	}
}

func replaceWorkerAdmissionRoot(t *testing.T, admission, kind string) {
	t.Helper()
	claimPath := filepath.Join(admission, "admission.json")
	retained := admission + ".retained"
	switch kind {
	case "replaced-root":
		claim, err := os.Lstat(claimPath)
		if err != nil {
			t.Fatal("worker admission claim")
		}
		root, err := os.Lstat(admission)
		if err != nil {
			t.Fatal("worker admission root")
		}
		if os.Rename(admission, retained) != nil {
			t.Fatal("retain worker admission root")
		}
		if os.Mkdir(admission, 0700) != nil {
			t.Fatal("replacement worker admission root")
		}
		if os.Rename(filepath.Join(retained, "admission.json"), claimPath) != nil {
			t.Fatal("restore worker claim inode")
		}
		restored, err := os.Lstat(claimPath)
		replaced, replErr := os.Lstat(admission)
		if err != nil || replErr != nil || !os.SameFile(claim, restored) || os.SameFile(root, replaced) {
			t.Fatal("worker admission root replacement did not keep the claim inode")
		}
	case "symlink":
		if os.Rename(admission, retained) != nil || os.Symlink(retained, admission) != nil {
			t.Fatal("symlink worker admission root")
		}
	case "mode":
		if os.Chmod(admission, 0755) != nil {
			t.Fatal("relax worker admission root")
		}
	case "missing":
		if os.Rename(admission, retained) != nil {
			t.Fatal("remove worker admission root")
		}
	default:
		t.Fatalf("unknown worker admission root kind %s", kind)
	}
}

func TestPairedWorkerPreparationReceiptFencesAdmissionRootReplacement(t *testing.T) {
	for _, kind := range []string{"replaced-root", "symlink", "mode", "missing"} {
		t.Run(kind, func(t *testing.T) {
			a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
			plan, err := openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c)
			if err != nil {
				t.Fatal("valid paired worker plan refused")
			}
			defer plan.close()
			admission := filepath.Join(filepath.Dir(workerState), "canonical-worker-claim")
			plan.prepare = func(context.Context) (brokerPreparationReceipt, error) {
				return brokerSyntheticWorkerPreparation(t, plan, admission)
			}
			if err := plan.checkPrepared(context.Background()); err != nil {
				t.Fatalf("fresh worker preparation refused: %v", err)
			}
			replaceWorkerAdmissionRoot(t, admission, kind)
			if err := plan.checkPrepared(context.Background()); err == nil {
				t.Fatal("replaced worker admission root crossed receipt fence")
			}
		})
	}
}

func workerReceiptThenReplaceAdmissionRoot(t *testing.T, plan *brokerWorkerPlan, admission string) (brokerPreparationReceipt, error) {
	t.Helper()
	receipt, err := brokerSyntheticWorkerPreparation(t, plan, admission)
	if err != nil {
		return brokerPreparationReceipt{}, err
	}
	plan.claimDirectory = ""
	plan.claimDirectoryInfo = nil
	replaceWorkerAdmissionRoot(t, admission, "replaced-root")
	return receipt, nil
}

func TestPairedBrokerRejectsWorkerAdmissionRootReplacementBetweenReceiptAndBind(t *testing.T) {
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
	parent := filepath.Dir(attempt)
	launches := 0
	plan, admission := pairedWorkerExecutePlan(t, &a, parent, func(context.Context, []byte, string) error {
		launches++
		return nil
	})
	bindWorkerClaimDirectory(t, admission)
	plan.worker.claimDirectory = ""
	plan.worker.claimDirectoryInfo = nil
	plan.worker.prepare = func(context.Context) (brokerPreparationReceipt, error) {
		return workerReceiptThenReplaceAdmissionRoot(t, plan.worker, admission)
	}
	_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"}, attempt, api, plan)
	if err == nil || fixture.tokenCalls != 0 || len(fixture.calls) != 0 || launches != 0 {
		t.Fatalf("worker root replacement crossed receipt-sampling fence: err=%v mints=%d calls=%v launches=%d", err, fixture.tokenCalls, fixture.calls, launches)
	}
}

func TestPairedWorkerPrepareJournalRejectsReplacedRootBetweenReceiptAndBind(t *testing.T) {
	a, _, _, _, attempt := newBrokerFixture(t)
	a.Mode, a.Phase = "paired-terminal", "paired-terminal"
	parent := filepath.Dir(attempt)
	plan, admission := pairedWorkerExecutePlan(t, &a, parent, func(context.Context, []byte, string) error { return nil })
	bindWorkerClaimDirectory(t, admission)
	plan.worker.claimDirectory = ""
	plan.worker.claimDirectoryInfo = nil
	plan.worker.prepare = func(context.Context) (brokerPreparationReceipt, error) {
		return workerReceiptThenReplaceAdmissionRoot(t, plan.worker, admission)
	}
	if err := plan.worker.prepareJournal(context.Background()); err == nil {
		t.Fatal("root replaced after receipt sampling was rebound as trusted")
	}
}

func TestPairedWorkerPreparationReceiptRejectsAbsentOrMismatchedAdmissionRoot(t *testing.T) {
	for _, kind := range []string{"absent", "zero-device", "zero-inode", "mismatched"} {
		t.Run(kind, func(t *testing.T) {
			a, candidate, api, fixture, attempt := newBrokerFixture(t)
			a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
			parent := filepath.Dir(attempt)
			launches := 0
			plan, admission := pairedWorkerExecutePlan(t, &a, parent, func(context.Context, []byte, string) error {
				launches++
				return nil
			})
			bindWorkerClaimDirectory(t, admission)
			plan.worker.prepare = func(context.Context) (brokerPreparationReceipt, error) {
				receipt, err := brokerSyntheticWorkerPreparation(t, plan.worker, admission)
				if err != nil {
					return receipt, err
				}
				plan.worker.claimDirectory = ""
				plan.worker.claimDirectoryInfo = nil
				switch kind {
				case "absent", "zero-device", "zero-inode":
					receipt.AdmissionDirectory = brokerInode{}
					if kind == "zero-device" {
						receipt.AdmissionDirectory.Inode = 1
					}
					if kind == "zero-inode" {
						receipt.AdmissionDirectory.Device = 1
					}
				case "mismatched":
					receipt.AdmissionDirectory.Inode++
				}
				return receipt, nil
			}
			_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"}, attempt, api, plan)
			if err == nil || fixture.tokenCalls != 0 || len(fixture.calls) != 0 || launches != 0 {
				t.Fatalf("worker admission root receipt %s reached effects: err=%v mints=%d calls=%v launches=%d", kind, err, fixture.tokenCalls, fixture.calls, launches)
			}
		})
	}
}

func TestPairedWorkerApprovalMismatchRefusesBeforeBinding(t *testing.T) {
	a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
	raw, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatal(err)
	}
	var worker pairedWorkerApproval
	if err := decodeBrokerJSON(raw, &worker, true); err != nil {
		t.Fatal(err)
	}
	worker.WorkflowSHA = strings.Repeat("e", 40)
	raw, err = json.Marshal(worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c); err == nil {
		t.Fatal("mismatched worker identity accepted")
	}
}

func TestPairedWorkerDaemonIDMatchesCanonicalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		daemon string
		valid  bool
	}{
		{name: "colon and 128 bytes", daemon: "a:" + strings.Repeat("d", 126), valid: true},
		{name: "129 bytes", daemon: "a" + strings.Repeat("d", 128), valid: false},
		{name: "invalid slash", daemon: "a/b", valid: false},
		{name: "invalid leading punctuation", daemon: ":daemon", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
			data, err := os.ReadFile(workerPath)
			if err != nil {
				t.Fatal(err)
			}
			var worker pairedWorkerApproval
			if err := decodeBrokerJSON(data, &worker, true); err != nil {
				t.Fatal(err)
			}
			worker.DaemonID = tc.daemon
			data, err = json.Marshal(worker)
			if err != nil || os.WriteFile(workerPath, data, 0600) != nil {
				t.Fatal("worker approval rewrite")
			}
			_, err = openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c)
			if (err == nil) != tc.valid {
				t.Fatalf("daemon ID validity=%v want=%v: %v", err == nil, tc.valid, err)
			}
		})
	}
}

func TestPairedApprovalRejectsInsufficientTerminalAuthority(t *testing.T) {
	a := brokerApprovalFixture()
	a.Mode, a.Phase = "paired-terminal", "paired-terminal"
	a.ExpiresAt = time.Now().Add(90 * time.Second)
	if err := a.validate(time.Now()); err == nil {
		t.Fatal("paired approval accepted less than the bounded terminal completion budget")
	}
}

func pairedWorkerExecutePlan(t *testing.T, a *BrokerApproval, parent string, launch func(context.Context, []byte, string) error) (*brokerControllerPlan, string) {
	t.Helper()
	return pairedWorkerExecutePlanAt(t, a, parent, filepath.Join(parent, "canonical-worker-claim"), launch)
}

func pairedWorkerExecutePlanAt(t *testing.T, a *BrokerApproval, parent, admission string, launch func(context.Context, []byte, string) error) (*brokerControllerPlan, string) {
	t.Helper()
	plan := brokerTestPlan(t, a, parent, launch)
	plan.controller.Phases = []string{"create", "before-ack", "inspect", "cleanup"}
	plan.controller.WorkflowRunID = 7
	plan.raw, _ = json.Marshal(plan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(plan.raw)
	plan.approval = *a
	workerState := filepath.Join(parent, "worker-state")
	if err := os.Mkdir(workerState, 0700); err != nil && !os.IsExist(err) {
		t.Fatal("worker state")
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: plan.controller.HarnessSHA, WorkflowSHA: plan.controller.WorkflowSHA, OwnerNonce: plan.controller.OwnerNonce, Controller: plan.controller.Controller, Endpoint: "/tmp/g01-paired-claim-fence.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: plan.controller.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal("worker approval")
	}
	workerPath := filepath.Join(parent, "worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal("worker approval file")
	}
	plan.worker, err = openBrokerWorkerPlan(workerPath, workerState, plan.statePath, *a, plan.controller)
	if err != nil {
		t.Fatal("valid paired worker plan refused")
	}
	brokerAttachSyntheticWorkerPreparation(t, plan, admission)
	return plan, admission
}

func TestPairedBrokerRejectsWorkerClaimChangeBeforeAuth(t *testing.T) {
	for _, kind := range []string{"hash", "replacement", "relocated-inode", "relocated-directory"} {
		t.Run(kind, func(t *testing.T) {
			a, candidate, api, fixture, attempt := newBrokerFixture(t)
			a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
			parent := filepath.Dir(attempt)
			launches := 0
			plan, admission := pairedWorkerExecutePlan(t, &a, parent, func(context.Context, []byte, string) error {
				launches++
				return nil
			})
			prepare := plan.worker.prepare
			plan.worker.prepare = func(ctx context.Context) (brokerPreparationReceipt, error) {
				receipt, err := prepare(ctx)
				if err != nil {
					return receipt, err
				}
				claimPath := filepath.Join(admission, "admission.json")
				data, readErr := os.ReadFile(claimPath)
				if readErr != nil {
					t.Fatal("worker admission claim")
				}
				switch kind {
				case "hash":
					if os.WriteFile(claimPath, append(data, 'x'), 0600) != nil {
						t.Fatal("mutate worker claim")
					}
				case "replacement":
					if os.Rename(claimPath, claimPath+".retained") != nil || os.WriteFile(claimPath, data, 0600) != nil {
						t.Fatal("replace worker claim")
					}
				case "relocated-inode":
					fallback := filepath.Join(filepath.Dir(admission), "worker-admission")
					if os.Mkdir(fallback, 0700) != nil || os.Rename(claimPath, filepath.Join(fallback, "admission.json")) != nil || os.WriteFile(claimPath, []byte("{}"), 0600) != nil {
						t.Fatal("relocate worker claim inode")
					}
				case "relocated-directory":
					fallback := filepath.Join(filepath.Dir(admission), "worker-admission")
					if os.Rename(admission, fallback) != nil || os.Mkdir(admission, 0700) != nil || os.WriteFile(claimPath, []byte("{}"), 0600) != nil {
						t.Fatal("relocate worker claim directory")
					}
				}
				return receipt, nil
			}
			_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"}, attempt, api, plan)
			if err == nil || fixture.tokenCalls != 0 || len(fixture.calls) != 0 || launches != 0 {
				t.Fatalf("worker admission claim %s crossed pre-auth fence: err=%v mints=%d calls=%v launches=%d", kind, err, fixture.tokenCalls, fixture.calls, launches)
			}
		})
	}
}

func TestPairedBrokerRejectsWorkerClaimChangeBeforeMint(t *testing.T) {
	for _, kind := range []string{"hash", "replacement", "missing", "malformed", "locked", "relocated-inode", "relocated-directory"} {
		t.Run(kind, func(t *testing.T) {
			a, candidate, _, fixture, attempt := newBrokerFixture(t)
			a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
			parent := filepath.Dir(attempt)
			launches := 0
			plan, admission := pairedWorkerExecutePlan(t, &a, parent, func(context.Context, []byte, string) error {
				launches++
				return nil
			})
			claimPath := filepath.Join(admission, "admission.json")
			var held *os.File
			api := newBrokerAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
				response, err := fixture.RoundTrip(r)
				if r.URL.Path == "/app" {
					data, readErr := os.ReadFile(claimPath)
					if readErr != nil {
						t.Fatal("worker admission claim")
					}
					switch kind {
					case "hash":
						if os.WriteFile(claimPath, append(data, 'x'), 0600) != nil {
							t.Fatal("mutate worker claim")
						}
					case "replacement":
						if os.Rename(claimPath, claimPath+".retained") != nil || os.WriteFile(claimPath, data, 0600) != nil {
							t.Fatal("replace worker claim")
						}
					case "missing":
						if os.Remove(claimPath) != nil {
							t.Fatal("remove worker claim")
						}
					case "malformed":
						if os.WriteFile(claimPath, []byte("{"), 0600) != nil {
							t.Fatal("malform worker claim")
						}
					case "locked":
						held, err = os.OpenFile(claimPath, os.O_RDWR, 0)
						if err != nil || syscall.Flock(int(held.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
							t.Fatal("lock worker claim")
						}
					case "relocated-inode":
						fallback := filepath.Join(filepath.Dir(admission), "worker-admission")
						if os.Mkdir(fallback, 0700) != nil || os.Rename(claimPath, filepath.Join(fallback, "admission.json")) != nil || os.WriteFile(claimPath, []byte("{}"), 0600) != nil {
							t.Fatal("relocate worker claim inode")
						}
					case "relocated-directory":
						fallback := filepath.Join(filepath.Dir(admission), "worker-admission")
						if os.Rename(admission, fallback) != nil || os.Mkdir(admission, 0700) != nil || os.WriteFile(claimPath, []byte("{}"), 0600) != nil {
							t.Fatal("relocate worker claim directory")
						}
					}
				}
				return response, err
			}))
			api.admissionDirectory = func() (string, error) { return fixture.admissionRoot, nil }
			_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"}, attempt, api, plan)
			if held != nil {
				syscall.Flock(int(held.Fd()), syscall.LOCK_UN)
				held.Close()
			}
			if err == nil || fixture.tokenCalls != 0 || launches != 0 {
				t.Fatalf("worker admission claim %s crossed pre-mint fence: err=%v mints=%d calls=%v launches=%d", kind, err, fixture.tokenCalls, fixture.calls, launches)
			}
			for _, call := range fixture.calls {
				if strings.Contains(call, "access_tokens") {
					t.Fatal("worker admission claim change reached token mint")
				}
			}
		})
	}
}

func TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeAuth(t *testing.T) {
	for _, kind := range []string{"replaced-root", "symlink", "mode", "missing"} {
		t.Run(kind, func(t *testing.T) {
			a, candidate, api, fixture, attempt := newBrokerFixture(t)
			a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
			parent := filepath.Dir(attempt)
			launches := 0
			plan, admission := pairedWorkerExecutePlan(t, &a, parent, func(context.Context, []byte, string) error {
				launches++
				return nil
			})
			prepare := plan.worker.prepare
			plan.worker.prepare = func(ctx context.Context) (brokerPreparationReceipt, error) {
				receipt, err := prepare(ctx)
				if err != nil {
					return receipt, err
				}
				replaceWorkerAdmissionRoot(t, admission, kind)
				return receipt, nil
			}
			_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"}, attempt, api, plan)
			if err == nil || fixture.tokenCalls != 0 || len(fixture.calls) != 0 || launches != 0 {
				t.Fatalf("worker admission root %s crossed pre-auth fence: err=%v mints=%d calls=%v launches=%d", kind, err, fixture.tokenCalls, fixture.calls, launches)
			}
		})
	}
}

func TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeMint(t *testing.T) {
	for _, kind := range []string{"replaced-root", "symlink", "mode", "missing"} {
		t.Run(kind, func(t *testing.T) {
			a, candidate, _, fixture, attempt := newBrokerFixture(t)
			a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
			parent := filepath.Dir(attempt)
			launches := 0
			plan, admission := pairedWorkerExecutePlan(t, &a, parent, func(context.Context, []byte, string) error {
				launches++
				return nil
			})
			replaced := false
			api := newBrokerAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
				response, err := fixture.RoundTrip(r)
				if r.URL.Path == "/app" && !replaced {
					replaceWorkerAdmissionRoot(t, admission, kind)
					replaced = true
				}
				return response, err
			}))
			api.admissionDirectory = func() (string, error) { return fixture.admissionRoot, nil }
			_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"}, attempt, api, plan)
			if err == nil || fixture.tokenCalls != 0 || launches != 0 {
				t.Fatalf("worker admission root %s crossed pre-mint fence: err=%v mints=%d calls=%v launches=%d", kind, err, fixture.tokenCalls, fixture.calls, launches)
			}
			for _, call := range fixture.calls {
				if strings.Contains(call, "access_tokens") {
					t.Fatal("worker admission root change reached token mint")
				}
			}
		})
	}
}

func TestPairedBrokerParentAuthorityStopsBeforeMintOrLaunch(t *testing.T) {
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	a.Mode, a.Phase = "paired-terminal", "paired-terminal"
	launches := 0
	plan := brokerTestPlan(t, &a, filepath.Dir(attempt), func(context.Context, []byte, string) error {
		launches++
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := brokerExecute(ctx, a, brokerInput{PEM: string(candidate.PEM)}, attempt, api, plan); err == nil || fixture.tokenCalls != 0 || len(fixture.calls) != 0 || launches != 0 {
		t.Fatalf("insufficient parent authority reached effects: err=%v mints=%d calls=%d launches=%d", err, fixture.tokenCalls, len(fixture.calls), launches)
	}
}

func TestPairedBrokerBindsWorkerBeforeWorkflowVerifiedHandoff(t *testing.T) {
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
	parent := filepath.Dir(attempt)
	launches := 0
	plan := brokerTestPlan(t, &a, parent, func(_ context.Context, data []byte, _ string) error {
		launches++
		verified := false
		for _, call := range fixture.calls {
			if strings.HasPrefix(call, "GET /repos/org-a/canary/actions/runs/") {
				verified = true
				break
			}
		}
		if !verified {
			t.Fatal("paired handoff launched before workflow verification")
		}
		var payload map[string]any
		if json.Unmarshal(data, &payload) != nil || payload["installation_token"] != fixture.token || payload["verification_token"] != "synthetic-private-verification-token" {
			t.Fatal("paired handoff lost broker-issued credentials")
		}
		if _, found := payload["pem"]; found {
			t.Fatal("PEM crossed the broker boundary")
		}
		return nil
	})

	// The paired mode requires the same reviewed workflow verification authority
	// as the original terminal fixture before the child is launched.
	plan.controller.Phases = []string{"create", "before-ack", "inspect", "cleanup"}
	plan.controller.WorkflowRunID = 7
	plan.raw, _ = json.Marshal(plan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(plan.raw)
	plan.approval = a

	workerState := filepath.Join(parent, "worker-state")
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: plan.controller.HarnessSHA, WorkflowSHA: plan.controller.WorkflowSHA, OwnerNonce: plan.controller.OwnerNonce, Controller: plan.controller.Controller, Endpoint: "/tmp/g01-paired-broker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: plan.controller.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal(err)
	}
	workerPath := filepath.Join(parent, "worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal(err)
	}
	workerPlan, err := openBrokerWorkerPlan(workerPath, workerState, plan.statePath, a, plan.controller)
	if err != nil {
		t.Fatal("valid paired worker plan refused")
	}
	plan.worker = workerPlan
	brokerAttachSyntheticWorkerPreparation(t, plan, filepath.Join(parent, "worker-admission"))

	result, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"}, attempt, api, plan)
	if err != nil || result.Status != "paired_terminal_completed" || fixture.tokenCalls != 1 || launches != 1 {
		t.Fatalf("paired broker handoff incomplete: result=%+v err=%v mints=%d launches=%d", result, err, fixture.tokenCalls, launches)
	}
	verified := false
	for _, call := range fixture.calls {
		if strings.HasPrefix(call, "GET /repos/org-a/canary/actions/runs/") {
			verified = true
		}
	}
	if !verified {
		t.Fatal("workflow identity was not verified")
	}
	if fixture.tokenCalls != 1 {
		t.Fatal("paired handoff retried issuance")
	}
	ledger, err := os.ReadFile(filepath.Join(fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !strings.Contains(string(ledger), `"slot":"paired-terminal"`) || !strings.Contains(string(ledger), `"worker"`) || strings.Contains(string(ledger), fixture.token) || strings.Contains(string(ledger), string(candidate.PEM)) {
		t.Fatal("paired admission did not retain bounded worker binding")
	}
}

func TestPairedFailureAllowsAuthorizedInspectWithoutPairedRetry(t *testing.T) {
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	a.Mode, a.Phase, a.AllowVerificationAuthority = "paired-terminal", "paired-terminal", true
	parent := filepath.Dir(attempt)
	pairedLaunches := 0
	pairedPlan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error {
		pairedLaunches++
		return errBroker
	})
	pairedPlan.controller.Phases = []string{"create", "before-ack", "after-ack", "before-acquire", "inspect", "cleanup"}
	pairedPlan.controller.WorkflowRunID = 7
	pairedPlan.raw, _ = json.Marshal(pairedPlan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(pairedPlan.raw)
	pairedPlan.approval = a
	workerState := filepath.Join(parent, "failed-paired-worker-state")
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal("worker state")
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: pairedPlan.controller.HarnessSHA, WorkflowSHA: pairedPlan.controller.WorkflowSHA, OwnerNonce: pairedPlan.controller.OwnerNonce, Controller: pairedPlan.controller.Controller, Endpoint: "/tmp/g01-paired-failure.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: pairedPlan.controller.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal("worker approval")
	}
	workerPath := filepath.Join(parent, "failed-paired-worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal("worker approval file")
	}
	pairedPlan.worker, err = openBrokerWorkerPlan(workerPath, workerState, pairedPlan.statePath, a, pairedPlan.controller)
	if err != nil {
		t.Fatal("paired worker plan")
	}
	brokerAttachSyntheticWorkerPreparation(t, pairedPlan, filepath.Join(parent, "failed-worker-admission"))
	if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-workflow-token"}, attempt, api, pairedPlan); err == nil || fixture.tokenCalls != 1 || pairedLaunches != 1 {
		t.Fatalf("failed paired attempt was accepted or retried: err=%v mints=%d launches=%d", err, fixture.tokenCalls, pairedLaunches)
	}

	// Inspect is a distinct, explicitly authorized controller slot. It may
	// collect its own authenticated evidence, but it must not replay the
	// incomplete paired claim or invoke the worker handoff again.
	a.Mode, a.Phase = "controller", "inspect"
	inspectLaunches := 0
	inspectPlan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error {
		inspectLaunches++
		return nil
	})
	inspectPlan.controller.Phases = append([]string(nil), pairedPlan.controller.Phases...)
	inspectPlan.controller.WorkflowRunID = pairedPlan.controller.WorkflowRunID
	inspectPlan.raw, _ = json.Marshal(inspectPlan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(inspectPlan.raw)
	inspectPlan.approval = a
	_ = inspectPlan.state.Close()
	inspectPlan.state, err = openBrokerPrivateDirectory(pairedPlan.statePath)
	if err != nil {
		t.Fatal("reopen controller state")
	}
	t.Cleanup(func() { _ = inspectPlan.state.Close() })
	inspectPlan.statePath = pairedPlan.statePath
	inspectPlan.stateInfo, err = inspectPlan.state.Stat(".")
	if err != nil {
		t.Fatal("controller state identity")
	}
	result, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-workflow-token"}, filepath.Join(parent, "inspect-attempt"), api, inspectPlan)
	if err != nil || result.Status != "controller_completed" || fixture.tokenCalls != 2 || pairedLaunches != 1 || inspectLaunches != 1 {
		t.Fatalf("authorized inspect did not remain distinct from failed pair: result=%+v err=%v mints=%d paired=%d inspect=%d", result, err, fixture.tokenCalls, pairedLaunches, inspectLaunches)
	}
	// The inspect slot is also one-shot; reopening it cannot mint or launch.
	retryPlan := brokerTestPlan(t, &a, parent, func(context.Context, []byte, string) error {
		t.Fatal("inspect retry launched")
		return nil
	})
	retryPlan.controller.Phases = append([]string(nil), pairedPlan.controller.Phases...)
	retryPlan.controller.WorkflowRunID = pairedPlan.controller.WorkflowRunID
	retryPlan.raw, _ = json.Marshal(retryPlan.controller)
	a.ControllerApprovalSHA256 = brokerBytesDigest(retryPlan.raw)
	retryPlan.approval = a
	_ = retryPlan.state.Close()
	retryPlan.state, err = openBrokerPrivateDirectory(pairedPlan.statePath)
	if err != nil {
		t.Fatal("reopen controller state for retry")
	}
	t.Cleanup(func() { _ = retryPlan.state.Close() })
	retryPlan.statePath = pairedPlan.statePath
	retryPlan.stateInfo, err = retryPlan.state.Stat(".")
	if err != nil {
		t.Fatal("controller state retry identity")
	}
	if _, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-workflow-token"}, filepath.Join(parent, "inspect-retry"), api, retryPlan); err == nil || fixture.tokenCalls != 2 || pairedLaunches != 1 || inspectLaunches != 1 {
		t.Fatalf("inspect retry replayed paired or inspect effects: err=%v mints=%d paired=%d inspect=%d", err, fixture.tokenCalls, pairedLaunches, inspectLaunches)
	}
}

// This is intentionally an entrypoint-level fixture. It uses the real
// BrokerFiles loader, paired preparation process and brokerExecute closure, so
// the canonical preparation receipt and fixed child handoff are both exercised.
func TestPairedBrokerRealEntrypointUsesPairedPreparationClosure(t *testing.T) {
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	parent := filepath.Dir(attempt)
	controllerState := filepath.Join(parent, "paired-controller-state")
	workerState := filepath.Join(parent, "paired-worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal("controller state")
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal("worker state")
	}
	now := time.Now().Add(time.Hour)
	harness := strings.Repeat("c", 40)
	workflow := strings.Repeat("b", 40)
	a.Mode, a.Phase, a.AllowVerificationAuthority, a.ExpiresAt, a.ControllerHarnessSHA = "paired-terminal", "paired-terminal", true, now, harness
	controller := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: harness, WorkflowSHA: workflow, WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: now, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "inspect", "cleanup"}}
	controllerData, err := json.Marshal(controller)
	if err != nil {
		t.Fatal("controller approval")
	}
	controllerPath := filepath.Join(parent, "controller-approval.json")
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal("controller approval file")
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: harness, WorkflowSHA: workflow, OwnerNonce: a.OwnerNonce, Controller: controller.Controller, Endpoint: "/tmp/g01-paired-entry.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: now, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal("worker approval")
	}
	workerPath := filepath.Join(parent, "worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal("worker approval file")
	}
	binary := testBrokerBinary(t)
	a.ControllerBinarySHA256 = binary.digest
	a.ControllerApprovalSHA256 = brokerBytesDigest(controllerData)
	approvalPath := filepath.Join(parent, "broker-approval.json")
	approvalData, err := json.Marshal(a)
	if err != nil {
		t.Fatal("broker approval")
	}
	if err := os.WriteFile(approvalPath, approvalData, 0600); err != nil {
		t.Fatal("broker approval file")
	}
	inputPath := filepath.Join(parent, "broker-input.json")
	inputData, err := json.Marshal(brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"})
	if err != nil {
		t.Fatal("broker input")
	}
	if err := os.WriteFile(inputPath, inputData, 0600); err != nil {
		t.Fatal("broker input file")
	}
	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatal("broker input open")
	}
	defer input.Close()
	oldOpener := brokerBinaryOpener
	brokerBinaryOpener = func(string, BrokerApproval) (*verifiedBrokerBinary, error) { return binary, nil }
	defer func() { brokerBinaryOpener = oldOpener }()
	bindWorkerClaimDirectory(t, filepath.Join(parent, "worker-admission"))
	result, err := runBrokerWithAPI(context.Background(), BrokerFiles{ApprovalPath: approvalPath, StateDirectory: attempt, ControllerBinary: binary.path, ControllerApproval: controllerPath, ControllerStateDirectory: controllerState, WorkerApproval: workerPath, WorkerStateDirectory: workerState}, input, api)
	if err != nil || result.Status != "paired_terminal_completed" || fixture.tokenCalls != 1 {
		t.Fatalf("real paired entrypoint did not complete one handoff: result=%+v err=%v mints=%d calls=%v", result, err, fixture.tokenCalls, fixture.calls)
	}
	retryPath := filepath.Join(parent, "broker-input-retry.json")
	if err := os.WriteFile(retryPath, inputData, 0600); err != nil {
		t.Fatal("retry input file")
	}
	retryInput, err := os.Open(retryPath)
	if err != nil {
		t.Fatal("retry input open")
	}
	defer retryInput.Close()
	_, retryErr := runBrokerWithAPI(context.Background(), BrokerFiles{ApprovalPath: approvalPath, StateDirectory: attempt, ControllerBinary: binary.path, ControllerApproval: controllerPath, ControllerStateDirectory: controllerState, WorkerApproval: workerPath, WorkerStateDirectory: workerState}, retryInput, api)
	if retryErr == nil || fixture.tokenCalls != 1 {
		t.Fatalf("paired handoff replayed after a completed attempt: err=%v mints=%d", retryErr, fixture.tokenCalls)
	}
}

func TestPairedBrokerRejectsMalformedWorkerJournalBeforeMint(t *testing.T) {
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	parent := filepath.Dir(attempt)
	controllerState := filepath.Join(parent, "paired-controller-state")
	workerState := filepath.Join(parent, "paired-worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal("controller state")
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal("worker state")
	}
	if err := os.WriteFile(filepath.Join(workerState, "journal.jsonl"), []byte("{malformed-worker-journal}\n"), 0600); err != nil {
		t.Fatal("malformed worker journal")
	}
	now := time.Now().Add(time.Hour)
	harness := strings.Repeat("c", 40)
	workflow := strings.Repeat("b", 40)
	a.Mode, a.Phase, a.AllowVerificationAuthority, a.ExpiresAt, a.ControllerHarnessSHA = "paired-terminal", "paired-terminal", true, now, harness
	controller := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: harness, WorkflowSHA: workflow, WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: now, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "inspect", "cleanup"}}
	controllerData, err := json.Marshal(controller)
	if err != nil {
		t.Fatal("controller approval")
	}
	controllerPath := filepath.Join(parent, "controller-approval.json")
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal("controller approval file")
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: harness, WorkflowSHA: workflow, OwnerNonce: a.OwnerNonce, Controller: controller.Controller, Endpoint: "/tmp/g01-paired-entry.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: now, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal("worker approval")
	}
	workerPath := filepath.Join(parent, "worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal("worker approval file")
	}
	binary := testBrokerBinary(t)
	a.ControllerBinarySHA256 = binary.digest
	a.ControllerApprovalSHA256 = brokerBytesDigest(controllerData)
	approvalPath := filepath.Join(parent, "broker-approval.json")
	approvalData, err := json.Marshal(a)
	if err != nil {
		t.Fatal("broker approval")
	}
	if err := os.WriteFile(approvalPath, approvalData, 0600); err != nil {
		t.Fatal("broker approval file")
	}
	inputPath := filepath.Join(parent, "broker-input.json")
	inputData, err := json.Marshal(brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"})
	if err != nil {
		t.Fatal("broker input")
	}
	if err := os.WriteFile(inputPath, inputData, 0600); err != nil {
		t.Fatal("broker input file")
	}
	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatal("broker input open")
	}
	defer input.Close()
	oldOpener := brokerBinaryOpener
	brokerBinaryOpener = func(string, BrokerApproval) (*verifiedBrokerBinary, error) { return binary, nil }
	defer func() { brokerBinaryOpener = oldOpener }()
	bindWorkerClaimDirectory(t, filepath.Join(parent, "worker-admission"))
	_, err = runBrokerWithAPI(context.Background(), BrokerFiles{ApprovalPath: approvalPath, StateDirectory: attempt, ControllerBinary: binary.path, ControllerApproval: controllerPath, ControllerStateDirectory: controllerState, WorkerApproval: workerPath, WorkerStateDirectory: workerState}, input, api)
	if err == nil || fixture.tokenCalls != 0 {
		t.Fatalf("malformed worker journal crossed pre-mint boundary: err=%v mints=%d calls=%v", err, fixture.tokenCalls, fixture.calls)
	}
}

type pairedBrokerEntryFixture struct {
	files   BrokerFiles
	api     *brokerAPI
	fixture *brokerHTTPFixture
	parent  string
	input   []byte
	runs    int
}

func newPairedBrokerEntryFixture(t *testing.T) pairedBrokerEntryFixture {
	t.Helper()
	a, candidate, api, fixture, attempt := newBrokerFixture(t)
	parent := filepath.Dir(attempt)
	controllerState := filepath.Join(parent, "paired-controller-state")
	workerState := filepath.Join(parent, "paired-worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal("controller state")
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal("worker state")
	}
	now := time.Now().Add(time.Hour)
	harness := strings.Repeat("c", 40)
	workflow := strings.Repeat("b", 40)
	a.Mode, a.Phase, a.AllowVerificationAuthority, a.ExpiresAt, a.ControllerHarnessSHA = "paired-terminal", "paired-terminal", true, now, harness
	controller := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: harness, WorkflowSHA: workflow, WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: now, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "inspect", "cleanup"}}
	controllerData, err := json.Marshal(controller)
	if err != nil {
		t.Fatal("controller approval")
	}
	controllerPath := filepath.Join(parent, "controller-approval.json")
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal("controller approval file")
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: harness, WorkflowSHA: workflow, OwnerNonce: a.OwnerNonce, Controller: controller.Controller, Endpoint: "/tmp/g01-paired-entry.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: pairedWorkerImage, ExpiresAt: now, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal("worker approval")
	}
	workerPath := filepath.Join(parent, "worker-approval.json")
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal("worker approval file")
	}
	binary := testBrokerBinary(t)
	a.ControllerBinarySHA256 = binary.digest
	a.ControllerApprovalSHA256 = brokerBytesDigest(controllerData)
	approvalPath := filepath.Join(parent, "broker-approval.json")
	approvalData, err := json.Marshal(a)
	if err != nil {
		t.Fatal("broker approval")
	}
	if err := os.WriteFile(approvalPath, approvalData, 0600); err != nil {
		t.Fatal("broker approval file")
	}
	inputData, err := json.Marshal(brokerInput{PEM: string(candidate.PEM), VerificationToken: "synthetic-private-verification-token"})
	if err != nil {
		t.Fatal("broker input")
	}
	oldOpener := brokerBinaryOpener
	brokerBinaryOpener = func(string, BrokerApproval) (*verifiedBrokerBinary, error) {
		f, err := os.Open(binary.path)
		if err != nil {
			return nil, errBroker
		}
		return &verifiedBrokerBinary{path: binary.path, file: f, digest: binary.digest}, nil
	}
	t.Cleanup(func() { brokerBinaryOpener = oldOpener })
	bindWorkerClaimDirectory(t, filepath.Join(parent, "worker-admission"))
	return pairedBrokerEntryFixture{
		files: BrokerFiles{
			ApprovalPath:             approvalPath,
			StateDirectory:           attempt,
			ControllerBinary:         binary.path,
			ControllerApproval:       controllerPath,
			ControllerStateDirectory: controllerState,
			WorkerApproval:           workerPath,
			WorkerStateDirectory:     workerState,
		},
		api:     api,
		fixture: fixture,
		parent:  parent,
		input:   inputData,
	}
}

func (e *pairedBrokerEntryFixture) run(t *testing.T, controllerState, workerState string) (BrokerResult, error) {
	t.Helper()
	e.runs++
	inputPath := filepath.Join(e.parent, "broker-input-"+strings.Repeat("x", e.runs)+".json")
	if err := os.WriteFile(inputPath, e.input, 0600); err != nil {
		t.Fatal("broker input file")
	}
	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatal("broker input open")
	}
	defer input.Close()
	files := e.files
	files.ControllerStateDirectory = controllerState
	files.WorkerStateDirectory = workerState
	return runBrokerWithAPI(context.Background(), files, input, e.api)
}

func pairedAdmissionHasClaim(t *testing.T, fixture *brokerHTTPFixture) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), `"slot":"paired-terminal"`)
}

func TestPairedBrokerRejectsNoncanonicalControllerStateBeforeClaim(t *testing.T) {
	for _, tc := range []struct {
		name       string
		controller func(string) string
		worker     func(string) string
	}{
		{name: "trailing slash", controller: func(p string) string { return p + string(filepath.Separator) }},
		{name: "dot", controller: func(p string) string { return p + string(filepath.Separator) + "." }},
		{name: "dot-dot", controller: func(p string) string { return p + string(filepath.Separator) + ".." }},
		{name: "relative", controller: func(p string) string { return filepath.Base(p) }},
		{name: "worker trailing slash", worker: func(p string) string { return p + string(filepath.Separator) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newPairedBrokerEntryFixture(t)
			controllerState := e.files.ControllerStateDirectory
			workerState := e.files.WorkerStateDirectory
			if tc.controller != nil {
				controllerState = tc.controller(controllerState)
			}
			if tc.worker != nil {
				workerState = tc.worker(workerState)
			}
			_, err := e.run(t, controllerState, workerState)
			if err == nil {
				t.Fatal("noncanonical private path was accepted")
			}
			if e.fixture.tokenCalls != 0 || len(e.fixture.calls) != 0 {
				t.Fatalf("noncanonical path reached API/mint: mints=%d calls=%v", e.fixture.tokenCalls, e.fixture.calls)
			}
			if pairedAdmissionHasClaim(t, e.fixture) {
				t.Fatal("noncanonical path appended a permanent paired claim")
			}
			result, err := e.run(t, e.files.ControllerStateDirectory, e.files.WorkerStateDirectory)
			if err != nil || result.Status != "paired_terminal_completed" || e.fixture.tokenCalls != 1 {
				t.Fatalf("canonical path could not start after lexical refusal: result=%+v err=%v mints=%d", result, err, e.fixture.tokenCalls)
			}
		})
	}
}

func TestPairedBrokerAcceptsCanonicalControllerStateDirectory(t *testing.T) {
	e := newPairedBrokerEntryFixture(t)
	result, err := e.run(t, e.files.ControllerStateDirectory, e.files.WorkerStateDirectory)
	if err != nil || result.Status != "paired_terminal_completed" || e.fixture.tokenCalls != 1 || len(e.fixture.calls) == 0 {
		t.Fatalf("canonical paired entry failed: result=%+v err=%v mints=%d calls=%v", result, err, e.fixture.tokenCalls, e.fixture.calls)
	}
	if !pairedAdmissionHasClaim(t, e.fixture) {
		t.Fatal("canonical paired entry left no admission claim")
	}
}

func TestBrokerControllerModeKeepsCanonicalStateDirectory(t *testing.T) {
	a, candidate, api, fixture, root := newBrokerFixture(t)
	a.Mode = "controller"
	a.Phase = "create"
	calls := 0
	plan := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error {
		calls++
		return nil
	})
	if filepath.Clean(plan.statePath) != plan.statePath || !filepath.IsAbs(plan.statePath) {
		t.Fatalf("controller plan rewrote state path to %q", plan.statePath)
	}
	result, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(candidate.PEM)}, root, api, plan)
	if err != nil || result.Status != "controller_completed" || calls != 1 || fixture.tokenCalls != 1 {
		t.Fatalf("controller-only canonical path lost compatibility: result=%+v err=%v launches=%d mints=%d", result, err, calls, fixture.tokenCalls)
	}
}
