package livecanary

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

// PairedTerminalFiles identify the two already-prepared private state domains.
// The paired terminal mode has one process and one controller input stream; the
// worker is never launched as a child process.
type PairedTerminalFiles struct {
	ControllerApprovalPath   string
	ControllerStateDirectory string
	WorkerApprovalPath       string
	WorkerStateDirectory     string
}

// ValidatePairedApprovals is the credential-free front door shared by the CLI
// and the same-process runner. PairInput is still derived from the controller
// journal and approval; worker input only proves the intended trusted worker
// profile and shared immutable identity.
func ValidatePairedApprovals(controller Approval, worker liveworker.Approval) error {
	now := time.Now()
	if controller.Validate(now) != nil || worker.Validate(now) != nil {
		return ErrApproval
	}
	if controller.OwnerNonce != worker.OwnerNonce || controller.HarnessSHA != worker.HarnessSHA || controller.WorkflowSHA != worker.WorkflowSHA || controller.Controller != worker.Controller || !controller.ExpiresAt.Equal(worker.ExpiresAt) {
		return ErrApproval
	}
	if controller.WorkflowRunID <= 0 {
		return ErrApproval
	}
	verificationPhase := false
	for _, phase := range controller.Phases {
		if phase == "before-ack" || phase == "after-ack" || phase == "before-acquire" || phase == "acquire-loss" {
			verificationPhase = true
			break
		}
	}
	if !verificationPhase {
		return ErrApproval
	}
	for _, phase := range []string{"create", "inspect", "cleanup"} {
		found := false
		for _, candidate := range controller.Phases {
			if candidate == phase {
				found = true
				break
			}
		}
		if !found {
			return ErrApproval
		}
	}
	for _, phase := range []string{"create", "start", "inspect", "cleanup"} {
		found := false
		for _, candidate := range worker.Phases {
			if candidate == phase {
				found = true
				break
			}
		}
		if !found {
			return ErrApproval
		}
	}
	return nil
}

// ValidatePairedStatePaths checks the private roots before the controller
// credential stream is read. OpenJournal repeats the inode/claim checks while
// holding each domain lease; this early check only prevents an invalid or
// aliased root from reaching credential handling.
func ValidatePairedStatePaths(controller, worker string) error {
	if !privateStateDirectory(controller) || !privateStateDirectory(worker) {
		return ErrJournal
	}
	controllerReal, err := filepath.EvalSymlinks(controller)
	if err != nil {
		return ErrJournal
	}
	workerReal, err := filepath.EvalSymlinks(worker)
	if err != nil || workerReal == controllerReal {
		return ErrJournal
	}
	return nil
}

func privateStateDirectory(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}

// RunPairedTerminal opens both existing journals, constructs the pinned SDK and
// direct Unix-socket runtime, and runs the reviewed terminal sequence in this
// process. It returns only fixed error categories; no private SDK/Docker error
// or path is part of the result boundary.
func RunPairedTerminal(ctx context.Context, files PairedTerminalFiles, credentials Credentials) error {
	if ctx == nil {
		return ErrApproval
	}
	controller, err := ReadApproval(files.ControllerApprovalPath)
	if err != nil {
		return ErrApproval
	}
	worker, err := liveworker.ReadApproval(files.WorkerApprovalPath)
	if err != nil || ValidatePairedApprovals(controller, worker) != nil || ValidatePairedStatePaths(files.ControllerStateDirectory, files.WorkerStateDirectory) != nil {
		return ErrApproval
	}
	if credentials.validate(controller, time.Now()) != nil || len(credentials.VerificationToken) < 20 || len(credentials.VerificationToken) > 1024 || credentials.VerificationToken == credentials.InstallationToken || strings.ContainsAny(credentials.VerificationToken, "\r\n\x00") {
		return ErrApproval
	}
	controllerJournal, err := OpenJournal(files.ControllerStateDirectory, controller)
	if err != nil {
		return ErrJournal
	}
	defer controllerJournal.Close()
	workerJournal, err := liveworker.OpenJournal(files.WorkerStateDirectory, worker)
	if err != nil {
		return ErrJournal
	}
	// LIFO closes the worker claim before the controller claim, matching the
	// paired cleanup order even when the terminal exits through an error path.
	defer workerJournal.Close()
	api, err := NewSDKAPI(controller, credentials)
	if err != nil {
		return ErrApproval
	}
	docker, err := liveworker.NewDocker(worker)
	if err != nil {
		return ErrJournal
	}
	result, err := runPairedTerminal(ctx, &Driver{Approval: controller, Journal: controllerJournal, API: api}, &liveworker.Driver{Approval: worker, Journal: workerJournal, Runtime: docker})
	if err != nil || result.Terminal != terminalComplete {
		return ErrQuarantine
	}
	return nil
}
