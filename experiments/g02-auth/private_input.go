package enrollment

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

// readPrivateInput bounds owned input bytes and makes inherited pipes cancellable.
// The caller owns and closes the original input. Regular files remain subject to
// filesystem stalls; arbitrary ReadClosers must support cooperative Close.
func readPrivateInput(ctx context.Context, input io.ReadCloser, limit int64) ([]byte, error) {
	refused := errors.New("private input refused")
	if ctx == nil || input == nil || limit < 1 || limit > 1<<20 {
		return nil, refused
	}
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
			reader, err := pollablePrivatePipe(ctx, file)
			if err != nil {
				return nil, refused
			}
			defer reader.Close()
			input = reader
			pollable = reader
		} else if !info.Mode().IsRegular() {
			return nil, refused
		}
	}
	// An inherited blocking fd0 must first join Go's poller. Closing the fresh
	// pollable descriptor interrupts its read; closing plain os.Stdin may not.
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	defer stop()
	var source io.Reader = input
	if pollable != nil {
		source = privatePipeReader{ctx: ctx, file: pollable}
	}
	data, err := io.ReadAll(io.LimitReader(source, limit+1))
	if err != nil || ctx.Err() != nil || int64(len(data)) > limit {
		clear(data)
		return nil, refused
	}
	return data, nil
}

type privatePipeReader struct {
	ctx  context.Context
	file *os.File
}

func (r privatePipeReader) Read(b []byte) (int, error) {
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

func pollablePrivatePipe(ctx context.Context, input *os.File) (*os.File, error) {
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
	reader := os.NewFile(uintptr(fd), "private-input-pipe")
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
