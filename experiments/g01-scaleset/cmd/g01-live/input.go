//go:build g01_live

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
)

func readCredentialInput(ctx context.Context, input io.ReadCloser) ([]byte, error) {
	refused := errors.New("credential input refused")
	if ctx == nil || input == nil {
		return nil, refused
	}
	defer input.Close()
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
		} else {
			return nil, refused
		}
	}
	// An inherited blocking fd0 must first join Go's poller. Closing the fresh
	// pollable descriptor interrupts its read; closing plain os.Stdin may not.
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	defer stop()
	data, err := io.ReadAll(io.LimitReader(input, 16385))
	if err != nil || ctx.Err() != nil || len(data) > 16384 {
		clear(data)
		return nil, refused
	}
	return data, nil
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
