package handoff

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestProtectedInputPipesAliasEOFAndOriginalClose(t *testing.T) {
	for _, mode := range []string{"complete", "alias", "withheld", "oversize", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			r, w, e := os.Pipe()
			if e != nil {
				t.Fatal(e)
			}
			defer r.Close()
			defer w.Close()
			if mode == "alias" {
				raw, _ := r.SyscallConn()
				fd := -1
				raw.Control(func(n uintptr) { fd, _ = syscall.Dup(int(n)) })
				copy := os.NewFile(uintptr(fd), "test-alias")
				defer copy.Close()
				if files, e := ProtectPipes(r, copy); e == nil || files != nil {
					t.Fatal("alias accepted")
				}
				return
			}
			owned, e := ProtectPipes(r)
			if e != nil {
				t.Fatal(e)
			}
			if _, e := r.Stat(); e == nil {
				t.Fatal("original survived transfer")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			if mode == "canceled" {
				cancel()
			} else {
				if mode == "oversize" {
					w.Write([]byte("12345"))
				} else {
					w.Write([]byte("1234"))
				}
				if mode != "withheld" {
					w.Close()
				}
			}
			started := time.Now()
			data, e := ReadProtected(ctx, owned[0], 4)
			if (e == nil) != (mode == "complete") || mode == "complete" && string(data) != "1234" || time.Since(started) > time.Second {
				t.Fatal("bounded EOF/cancel contract failed", mode, e)
			}
		})
	}
}
func TestProtectedInputDoesNotInheritIntoOwnedChild(t *testing.T) {
	if os.Getenv("G01_PROTECTED_CHILD") == "1" {
		for fd := 3; fd < 96; fd++ {
			var st syscall.Stat_t
			if syscall.Fstat(fd, &st) == nil && uint64(st.Ino) == protectedChildInode() {
				os.Exit(9)
			}
		}
		os.Exit(0)
	}
	r, w, _ := os.Pipe()
	defer r.Close()
	defer w.Close()
	// Construct an inherited-style original with CLOEXEC cleared.
	raw, _ := r.SyscallConn()
	raw.Control(func(fd uintptr) { syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_SETFD, 0) })
	info, _ := r.Stat()
	ino := info.Sys().(*syscall.Stat_t).Ino
	files, e := ProtectPipes(r)
	if e != nil {
		t.Fatal(e)
	}
	defer files[0].Close()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, "-test.run=^TestProtectedInputDoesNotInheritIntoOwnedChild$")
	cmd.Env = append(os.Environ(), "G01_PROTECTED_CHILD=1", "G01_PROTECTED_INODE="+strconv.FormatUint(uint64(ino), 10))
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatal("protected input leaked to owned test child", e, string(output))
	}
}
func protectedChildInode() uint64 {
	n, _ := strconv.ParseUint(os.Getenv("G01_PROTECTED_INODE"), 10, 64)
	return n
}
