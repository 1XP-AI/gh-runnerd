//go:build g01_live

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

func readCredentialInput(ctx context.Context, input io.ReadCloser) ([]byte, error) {
	refused := errors.New("credential input refused")
	if ctx == nil || input == nil {
		return nil, refused
	}
	defer input.Close()
	var pollable *os.File
	if ctx.Err() != nil {
		return nil, refused
	}
	if file, ok := input.(*os.File); ok {
		info, err := file.Stat()
		if err != nil {
			return nil, refused
		}
		if info.Mode()&os.ModeNamedPipe != 0 {
			reader, err := pollableCredentialPipe(ctx, file)
			if err != nil {
				return nil, refused
			}
			defer reader.Close()
			input = reader
			pollable = reader
		} else {
			return nil, refused
		}
	}
	// An inherited blocking fd0 must first join Go's poller. Closing the fresh
	// pollable descriptor interrupts its read; closing plain os.Stdin may not.
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	defer stop()
	var source io.Reader = input
	if pollable != nil {
		source = credentialPipeReader{ctx: ctx, file: pollable}
	}
	data, err := io.ReadAll(io.LimitReader(source, 16385))
	if err != nil || ctx.Err() != nil || len(data) > 16384 {
		clear(data)
		return nil, refused
	}
	return data, nil
}

type credentialPipeReader struct {
	ctx  context.Context
	file *os.File
}

func (r credentialPipeReader) Read(b []byte) (int, error) {
	for {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		// Darwin may omit the final EOF notification for an inherited named FIFO.
		// Periodically retry the nonblocking read without extending the caller's
		// deadline or accepting a timeout as successful end-of-input.
		deadline := time.Now().Add(100 * time.Millisecond)
		if overall, ok := r.ctx.Deadline(); ok && overall.Before(deadline) {
			deadline = overall
		}
		if err := r.file.SetReadDeadline(deadline); err != nil {
			return 0, err
		}
		n, err := r.file.Read(b)
		if os.IsTimeout(err) && r.ctx.Err() == nil {
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func pollableCredentialPipe(ctx context.Context, input *os.File) (*os.File, error) {
	// Fd may switch an existing Go-managed file back to blocking mode. Call it
	// only on the original, never on the new poller-managed duplicate below.
	original := int(input.Fd())
	syscall.ForkLock.RLock()
	fd, err := syscall.Dup(original)
	if err == nil {
		syscall.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return nil, err
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	reader := os.NewFile(uintptr(fd), "private-credential-pipe")
	if reader == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("credential pipe unavailable")
	}
	deadline, _ := ctx.Deadline()
	if err := reader.SetReadDeadline(deadline); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return reader, nil
}
