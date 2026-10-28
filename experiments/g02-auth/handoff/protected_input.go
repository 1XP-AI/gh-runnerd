package handoff

import (
	"context"
	"io"
	"os"
	"syscall"
)

// ProtectPipes transfers ownership, rejects aliases and marks ORIGINAL handles
// close-on-exec before a caller may spawn any preparation process. Identity
// checks use Fstat through File.Stat, never Fd (which can disable the poller).
func ProtectPipes(files ...*os.File) ([]*os.File, error) {
	closeAll := func() {
		for _, f := range files {
			if f != nil {
				_ = f.Close()
			}
		}
	}
	infos := make([]os.FileInfo, len(files))
	for n, f := range files {
		if f == nil {
			closeAll()
			return nil, ErrInvalid
		}
		info, e := f.Stat()
		if e != nil || info.Mode()&os.ModeNamedPipe == 0 {
			closeAll()
			return nil, ErrInvalid
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != uint32(os.Geteuid()) || (st.Nlink != 0 && info.Mode().Perm()&0077 != 0) {
			closeAll()
			return nil, ErrInvalid
		}
		for _, prior := range infos[:n] {
			if os.SameFile(info, prior) {
				closeAll()
				return nil, ErrInvalid
			}
		}
		infos[n] = info
	}
	owned := make([]*os.File, 0, len(files))
	fail := func() ([]*os.File, error) {
		closeAll()
		for _, f := range owned {
			f.Close()
		}
		return nil, ErrInvalid
	}
	// Normalize every original before making any duplicate available.
	for _, f := range files {
		raw, e := f.SyscallConn()
		if e != nil {
			return fail()
		}
		if raw.Control(func(fd uintptr) { syscall.CloseOnExec(int(fd)) }) != nil {
			return fail()
		}
	}
	for _, f := range files {
		raw, e := f.SyscallConn()
		if e != nil {
			return fail()
		}
		duplicate := -1
		var de error
		e = raw.Control(func(fd uintptr) {
			syscall.ForkLock.RLock()
			duplicate, de = syscall.Dup(int(fd))
			if de == nil {
				syscall.CloseOnExec(duplicate)
			}
			syscall.ForkLock.RUnlock()
			if de == nil {
				de = syscall.SetNonblock(duplicate, true)
			}
		})
		if e != nil || de != nil || duplicate < 0 {
			if duplicate >= 0 {
				syscall.Close(duplicate)
			}
			return fail()
		}
		out := os.NewFile(uintptr(duplicate), "g01-protected-input")
		if out == nil {
			syscall.Close(duplicate)
			return fail()
		}
		owned = append(owned, out)
	}
	closeAll()
	return owned, nil
}

// ReadProtected requires EOF and joins its cancellation watcher before returning.
func ReadProtected(ctx context.Context, input *os.File, limit int) ([]byte, error) {
	if input == nil {
		return nil, ErrInvalid
	}
	defer input.Close()
	if ctx == nil || ctx.Err() != nil || limit < 1 || limit > 65536 {
		return nil, ErrInvalid
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			input.Close()
		case <-stop:
		}
	}()
	data, e := io.ReadAll(io.LimitReader(input, int64(limit+1)))
	close(stop)
	input.Close()
	<-done
	if e != nil || ctx.Err() != nil || len(data) < 1 || len(data) > limit {
		clear(data)
		return nil, ErrInvalid
	}
	return data, nil
}
