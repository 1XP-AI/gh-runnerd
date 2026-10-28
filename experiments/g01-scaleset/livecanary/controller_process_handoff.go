package livecanary

import (
	"bytes"
	"context"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// Private injection-only receiver. Neither the CLI nor ordinary runtime supplies
// role roots. Files transfer ownership even on rejection; possession is no proof.
func runControllerProcessHandoffCreate(ctx context.Context, journal *FileJournal, approvalSnapshot []byte, control, input *os.File, source string, workflow handoff.TrustRoot, issuer handoff.IssuerRoot, now func() time.Time, factory func(context.Context, Approval, Credentials) (API, error)) error {
	if control != nil {
		defer control.Close()
	}
	if input != nil {
		defer input.Close()
	}
	if len(approvalSnapshot) < 1 || len(approvalSnapshot) > maxControllerApprovalBytes || ctx == nil || ctx.Err() != nil || journal == nil || now == nil || factory == nil || !issuer.Independent(workflow) || !distinctHandoffPipes(control, input) {
		return ErrApproval
	}
	// Inherited blocking descriptors must be registered with Go's poller.
	// Merely closing a descriptor cannot interrupt a blocking kernel read on macOS.
	var err error
	control, err = pollableHandoffPipe(control)
	if err != nil {
		return ErrApproval
	}
	defer control.Close()
	input, err = pollableHandoffPipe(input)
	if err != nil {
		return ErrApproval
	}
	defer input.Close()
	snapshot := bytes.Clone(approvalSnapshot)
	defer clear(snapshot)
	a, request, err := controllerHandoffRequestOwned(snapshot, "create", source, now())
	if err != nil {
		return ErrApproval
	}
	deadline := controllerHandoffAcquisitionDeadline(ctx, a.ExpiresAt, now())
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	var receiptBytes, bindingBytes []byte
	err = withHandoffPipe(bounded, control, func() error { var e error; receiptBytes, bindingBytes, e = handoff.ReadControl(control); return e })
	defer clear(receiptBytes)
	defer clear(bindingBytes)
	if err != nil || bounded.Err() != nil {
		return ErrApproval
	}
	receipt, err := handoff.DecodeStrictReceipt(receiptBytes)
	if err != nil || workflow.Verify(request, receipt, now()) != nil {
		return ErrApproval
	}
	binding, err := handoff.DecodeStrictBinding(bindingBytes)
	expected := handoff.BindingExpected{Request: request, Receipt: receipt, ReceiptBytes: receiptBytes, ApprovalExpiresAt: a.ExpiresAt, Deadline: controllerHandoffDeadline(bounded, a.ExpiresAt, receipt.ExpiresAt)}
	if err != nil || issuer.VerifyBinding(expected, binding, now()) != nil {
		return ErrApproval
	}
	// Preserve the original parent deadline, additionally capped by the envelope.
	entry, cancelEntry := context.WithDeadline(bounded, binding.ExpiresAt)
	defer cancelEntry()
	return runControllerHandoffCreate(entry, journal, snapshot, receiptBytes, source, workflow, now, func(readCtx context.Context) ([]byte, error) {
		var data []byte
		e := withHandoffPipe(readCtx, input, func() error {
			var e error
			data, e = io.ReadAll(io.LimitReader(input, handoff.MaxCredentialBytes+1))
			return e
		})
		if e != nil || readCtx.Err() != nil || len(data) < 1 || len(data) > handoff.MaxCredentialBytes {
			clear(data)
			return nil, ErrApproval
		}
		// Exact serialized bytes are checked here, after durable consumption and
		// before DecodeStrict or the network-lazy factory.
		if len(data) != binding.PayloadLength || handoff.Digest(data) != binding.PayloadSHA256 {
			clear(data)
			return nil, ErrApproval
		}
		var c Credentials
		if DecodeStrict(data, &c) != nil || binding.ExpiresAt.After(c.ExpiresAt.Add(-time.Minute)) {
			clear(data)
			return nil, ErrApproval
		}
		return data, nil
	}, factory)
}

func distinctHandoffPipes(control, input *os.File) bool {
	if control == nil || input == nil || control == input {
		return false
	}
	a, e := control.Stat()
	b, f := input.Stat()
	return e == nil && f == nil && a.Mode()&os.ModeNamedPipe != 0 && b.Mode()&os.ModeNamedPipe != 0 && !os.SameFile(a, b)
}

// The reader runs in the caller; the joined watcher closes the owned concrete
// descriptor on timeout, including a withheld EOF. No lifecycle lease spans IO.
func withHandoffPipe(ctx context.Context, file *os.File, read func() error) error {
	if ctx == nil || file == nil || ctx.Err() != nil {
		return ErrApproval
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			_ = file.Close()
		case <-stop:
		}
	}()
	err := read()
	close(stop)
	_ = file.Close()
	<-done
	if err != nil || ctx.Err() != nil {
		return ErrApproval
	}
	return nil
}

// Transfer ownership to a nonblocking, close-on-exec duplicate before any read.
// The original is closed immediately; no duplicate remains to withhold EOF.
func pollableHandoffPipe(file *os.File) (*os.File, error) {
	raw, err := file.SyscallConn()
	if err != nil {
		return nil, ErrApproval
	}
	duplicate := -1
	var duplicateErr error
	err = raw.Control(func(fd uintptr) {
		duplicate, duplicateErr = syscall.Dup(int(fd))
		if duplicateErr == nil {
			syscall.CloseOnExec(duplicate)
			duplicateErr = syscall.SetNonblock(duplicate, true)
		}
	})
	_ = file.Close()
	if err != nil || duplicateErr != nil || duplicate < 0 {
		if duplicate >= 0 {
			syscall.Close(duplicate)
		}
		return nil, ErrApproval
	}
	owned := os.NewFile(uintptr(duplicate), "g01-owned-pipe")
	if owned == nil {
		syscall.Close(duplicate)
		return nil, ErrApproval
	}
	return owned, nil
}
