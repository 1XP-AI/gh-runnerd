package enrollment

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// Only the post-mint/preflight block in brokerExecute constructs this owned
// value. It cannot be supplied through an exported issuance/signing API.
type issuedControllerHandoff struct {
	receipt, envelope, payload               []byte
	deadline                                 time.Time
	mode, phase, workerApproval, workerState string
}

func (h *issuedControllerHandoff) clear() {
	if h != nil {
		clear(h.receipt)
		clear(h.envelope)
		clear(h.payload)
	}
}

type brokerHandoffAssembly struct {
	experimental bool
	signer       receiptSigner
	issuer       handoff.IssuerRoot
	workflow     handoff.TrustRoot
	source       string
	launch       func(context.Context, *issuedControllerHandoff, string) error
}

func (a *brokerHandoffAssembly) valid(root handoff.TrustRoot) bool {
	return a != nil && a.signer != nil && a.launch != nil && a.issuer.Independent(a.workflow) && a.workflow.Equal(root) && a.workflow.Valid() && brokerProvenanceSource.MatchString(a.source)
}

// Own both complete pipes, close every duplicate child endpoint after Start,
// and join the writer even on early child exit or withheld EOF. No pipe-capacity
// assumption orders these streams: control is fully closed before payload write.
func invokeBrokerControllerHandoff(parent context.Context, binary *verifiedBrokerBinary, workingDirectory, approvalPath, stateDirectory string, h *issuedControllerHandoff) error {
	if parent == nil || parent.Err() != nil || binary == nil || binary.check() != nil || h == nil || len(h.payload) < 1 || len(h.payload) > handoff.MaxCredentialBytes || !h.deadline.After(time.Now()) || !brokerCanonicalPath(workingDirectory) || !brokerCanonicalPath(approvalPath) || !brokerCanonicalPath(stateDirectory) || filepath.Clean(binary.path) != binary.path {
		return errBroker
	}
	frame, err := handoff.EncodeControl(h.receipt, h.envelope)
	if err != nil {
		return errBroker
	}
	defer clear(frame)
	deadline := h.deadline
	if h.mode == "" {
		deadline = minTime(deadline, time.Now().Add(handoff.MaxProcessDuration))
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	cr, cw, err := os.Pipe()
	if err != nil {
		return errBroker
	}
	defer cr.Close()
	defer cw.Close()
	ir, iw, err := os.Pipe()
	if err != nil {
		return errBroker
	}
	defer ir.Close()
	defer iw.Close()
	args := []string{"--execute-approved-canary", "--approval", approvalPath, "--state-dir", stateDirectory, "--phase", "create"}
	if h.mode != "" {
		if h.mode != "controller" && h.mode != "paired-terminal" {
			return errBroker
		}
		args = []string{"--execute-experimental-phase", "--mode", h.mode, "--approval", approvalPath, "--state-dir", stateDirectory, "--phase", h.phase}
		if h.mode == "paired-terminal" {
			args = append(args, "--worker-approval", h.workerApproval, "--worker-state-dir", h.workerState)
		}
	}
	command := exec.CommandContext(ctx, binary.path, args...)
	command.Dir = workingDirectory
	command.Env = []string{"LANG=C", "LC_ALL=C"}
	command.WaitDelay = time.Second
	output := &brokerOutputBudget{cancel: cancel}
	command.Stdout = output
	command.Stderr = output
	command.Stdin = ir
	command.ExtraFiles = []*os.File{cr}
	if ctx.Err() != nil || binary.check() != nil || command.Start() != nil {
		return errBroker
	}
	_ = cr.Close()
	_ = ir.Close()
	stop, watchDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-ctx.Done():
			_ = cw.Close()
			_ = iw.Close()
		case <-stop:
		}
	}()
	wrote := make(chan error, 1)
	go func() {
		_, e := cw.Write(frame)
		ce := cw.Close()
		if e == nil {
			e = ce
		}
		if e == nil && ctx.Err() == nil {
			_, e = iw.Write(h.payload)
		}
		ce = iw.Close()
		if e == nil {
			e = ce
		}
		wrote <- e
	}()
	waited := command.Wait()
	// Wait may finish before a blocked writer, so close writers before joining.
	_ = cw.Close()
	_ = iw.Close()
	writeErr := <-wrote
	close(stop)
	<-watchDone
	output.mu.Lock()
	overflow := output.overflow
	output.mu.Unlock()
	if waited != nil || writeErr != nil || overflow || ctx.Err() != nil {
		return errBroker
	}
	return nil
}
