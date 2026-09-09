//go:build g01_live

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

type unreadable struct{ t *testing.T }

func (r unreadable) Read([]byte) (int, error) {
	r.t.Fatal("credentials read before explicit exact-build approval")
	return 0, nil
}

type countedInput struct {
	io.Reader
	reads         int
	eofs          int
	readsAfterEOF int
}

func pairedBindingFixture(t *testing.T, controllerPath, controllerState, workerPath, workerState string) livecanary.PairedTerminalBinding {
	t.Helper()
	identity := func(path string) (uint64, uint64) {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			t.Fatal("fixture identity")
		}
		return uint64(stat.Dev), stat.Ino
	}
	digest := func(path string) string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	controllerDevice, controllerInode := identity(controllerPath)
	controllerStateDevice, controllerStateInode := identity(controllerState)
	workerDevice, workerInode := identity(workerPath)
	workerStateDevice, workerStateInode := identity(workerState)
	return livecanary.PairedTerminalBinding{ControllerApprovalSHA256: digest(controllerPath), ControllerApprovalDevice: controllerDevice, ControllerApprovalInode: controllerInode, ControllerStateDevice: controllerStateDevice, ControllerStateInode: controllerStateInode, WorkerApprovalSHA256: digest(workerPath), WorkerApprovalDevice: workerDevice, WorkerApprovalInode: workerInode, WorkerStateDevice: workerStateDevice, WorkerStateInode: workerStateInode}
}

func (r *countedInput) Close() error { return nil }
func (r *countedInput) Read(p []byte) (int, error) {
	r.reads++
	if r.eofs != 0 {
		r.readsAfterEOF++
	}
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.eofs++
	}
	return n, err
}

type chunkedControllerInput struct {
	chunks        [][]byte
	index         int
	reads         int
	eofs          int
	readsAfterEOF int
}

func (r *chunkedControllerInput) Read(p []byte) (int, error) {
	r.reads++
	if r.eofs != 0 {
		r.readsAfterEOF++
		return 0, io.EOF
	}
	if r.index == len(r.chunks) {
		r.eofs++
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.index])
	r.index++
	return n, nil
}

func (r *chunkedControllerInput) Close() error { return nil }

func TestPlanAndRefusalsNeverReadCredentialsOrEchoInputs(t *testing.T) {
	for _, args := range [][]string{{"--plan"}, {}, {"--synthetic-secret=do-not-print"}, {"--execute-approved-canary", "--approval=synthetic-secret", "--state-dir=synthetic-secret", "--phase=create"}} {
		var out bytes.Buffer
		code := run(args, unreadable{t}, &out)
		if len(args) == 1 && args[0] == "--plan" {
			if code != 0 {
				t.Fatal("offline plan failed")
			}
		} else if code == 0 {
			t.Fatal("unsafe invocation accepted")
		}
		if strings.Contains(out.String(), "synthetic-secret") || strings.Contains(out.String(), "do-not-print") {
			t.Fatal("caller input leaked")
		}
	}
}

func TestPreparationCommandNeverReadsCredentialsOrRunsRemotePhase(t *testing.T) {
	a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create"}}
	parent := t.TempDir()
	path := filepath.Join(parent, "approval.json")
	data, _ := json.Marshal(a)
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("fixture")
	}
	args := []string{"--prepare-approved-journal", "--approval", path, "--state-dir", parent, "--phase", "create"}
	for _, kind := range []string{"success", "failed preparation", "mixed execution", "mixed plan", "unapproved phase"} {
		t.Run(kind, func(t *testing.T) {
			argv := append([]string(nil), args...)
			called := 0
			switch kind {
			case "mixed execution":
				argv = append(argv, "--execute-approved-canary")
			case "mixed plan":
				argv = append(argv, "--plan")
			case "unapproved phase":
				argv[len(argv)-1] = "cleanup"
			}
			var out bytes.Buffer
			code := runWithPreparation(argv, unreadable{t}, &out, func() (string, bool) { return a.HarnessSHA, true }, func(state string, got livecanary.Approval, phase string) (livecanary.PreparationReceipt, error) {
				called++
				if state != parent || got.HarnessSHA != a.HarnessSHA || phase != "create" {
					t.Fatal("preparation identity changed")
				}
				if kind == "failed preparation" {
					return livecanary.PreparationReceipt{}, livecanary.ErrJournal
				}
				return livecanary.PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: phase}, nil
			})
			if kind == "success" {
				if code != 0 || called != 1 || !strings.Contains(out.String(), `"status":"controller_journal_prepared"`) {
					t.Fatal("fixed local success missing")
				}
			} else {
				if code == 0 {
					t.Fatal("ambiguous preparation accepted")
				}
				if kind != "failed preparation" && called != 0 {
					t.Fatal("invalid flags/phase reached state preparation")
				}
			}
			if strings.Contains(out.String(), parent) {
				t.Fatal("private path disclosed")
			}
		})
	}
}

func TestPairedTerminalModeReadsControllerInputAfterAllGates(t *testing.T) {
	a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 5, Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "inspect", "cleanup", "jit-loss"}}
	worker := liveworker.Approval{RunnerUpdatesDisabled: true, HarnessSHA: a.HarnessSHA, WorkflowSHA: a.WorkflowSHA, OwnerNonce: a.OwnerNonce, Controller: a.Controller, Endpoint: "/tmp/g01-paired-docker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: liveworker.ImageReference, ExpiresAt: a.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	root := t.TempDir()
	controllerState := filepath.Join(root, "controller-state")
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	controllerPath := filepath.Join(root, "controller.json")
	workerPath := filepath.Join(root, "worker.json")
	controllerData, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal(err)
	}
	binding := pairedBindingFixture(t, controllerPath, controllerState, workerPath, workerState)
	input := &chunkedControllerInput{chunks: [][]byte{[]byte(`{`), []byte(`}`)}}
	var out bytes.Buffer
	bindingData, _ := json.Marshal(binding)
	code := runWithPreparation([]string{"--execute-approved-paired-terminal", "--approval", controllerPath, "--state-dir", controllerState, "--worker-approval", workerPath, "--worker-state-dir", workerState, "--paired-binding", string(bindingData)}, input, &out, func() (string, bool) { return a.HarnessSHA, true }, func(string, livecanary.Approval, string) (livecanary.PreparationReceipt, error) {
		t.Fatal("paired mode entered controller-only preparation")
		return livecanary.PreparationReceipt{}, nil
	})
	if code == 0 || input.reads != 3 || input.eofs != 1 || input.readsAfterEOF != 0 {
		t.Fatalf("paired mode did not consume one logical controller input: code=%d reads=%d eofs=%d rereads=%d output=%q", code, input.reads, input.eofs, input.readsAfterEOF, out.String())
	}
}

func TestPairedTerminalModeRejectsUnusedPhaseAndControllerFlagsBeforeInput(t *testing.T) {
	a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 5, Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "inspect", "cleanup", "jit-loss"}}
	worker := liveworker.Approval{RunnerUpdatesDisabled: true, HarnessSHA: a.HarnessSHA, WorkflowSHA: a.WorkflowSHA, OwnerNonce: a.OwnerNonce, Controller: a.Controller, Endpoint: "/tmp/g01-paired-docker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: liveworker.ImageReference, ExpiresAt: a.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	root := t.TempDir()
	controllerState := filepath.Join(root, "controller-state")
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	controllerPath := filepath.Join(root, "controller.json")
	workerPath := filepath.Join(root, "worker.json")
	controllerData, _ := json.Marshal(a)
	workerData, _ := json.Marshal(worker)
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal(err)
	}
	bindingData, _ := json.Marshal(pairedBindingFixture(t, controllerPath, controllerState, workerPath, workerState))
	base := []string{"--execute-approved-paired-terminal", "--approval", controllerPath, "--state-dir", controllerState, "--worker-approval", workerPath, "--worker-state-dir", workerState, "--paired-binding", string(bindingData)}
	for _, extra := range [][]string{{"--phase", "cleanup"}, {"--execute-approved-canary"}} {
		input := &countedInput{Reader: strings.NewReader(`{}`)}
		args := append(append([]string(nil), base...), extra...)
		var out bytes.Buffer
		if code := runWithPreparation(args, input, &out, func() (string, bool) { return a.HarnessSHA, true }, nil); code == 0 || input.reads != 0 {
			t.Fatalf("incompatible paired flags reached input: extra=%v code=%d reads=%d", extra, code, input.reads)
		}
	}
}

func TestPairedTerminalModeRequiresWorkflowVerificationAuthorityBeforeInput(t *testing.T) {
	a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 5, Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "inspect", "cleanup"}}
	worker := liveworker.Approval{RunnerUpdatesDisabled: true, HarnessSHA: a.HarnessSHA, WorkflowSHA: a.WorkflowSHA, OwnerNonce: a.OwnerNonce, Controller: a.Controller, Endpoint: "/tmp/g01-paired-docker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: liveworker.ImageReference, ExpiresAt: a.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	root := t.TempDir()
	controllerState := filepath.Join(root, "controller-state")
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	controllerPath := filepath.Join(root, "controller.json")
	workerPath := filepath.Join(root, "worker.json")
	controllerData, _ := json.Marshal(a)
	workerData, _ := json.Marshal(worker)
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal(err)
	}
	bindingData, _ := json.Marshal(pairedBindingFixture(t, controllerPath, controllerState, workerPath, workerState))
	input := &countedInput{Reader: strings.NewReader(`{}`)}
	var out bytes.Buffer
	args := []string{"--execute-approved-paired-terminal", "--approval", controllerPath, "--state-dir", controllerState, "--worker-approval", workerPath, "--worker-state-dir", workerState, "--paired-binding", string(bindingData)}
	code := runWithPreparation(args, input, &out, func() (string, bool) { return a.HarnessSHA, true }, nil)
	if code == 0 || input.reads != 0 {
		t.Fatalf("paired mode accepted missing verification authority or read input: code=%d reads=%d output=%q", code, input.reads, out.String())
	}
}
