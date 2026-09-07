package enrollment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The child receives an actual blocking inherited fd 0. An os.Pipe reader used
// directly in the parent is already pollable and does not cover this boundary.
func TestManualInheritedInput(t *testing.T) {
	candidate := syntheticCandidate(t)
	for _, flow := range []string{"manual"} {
		for _, mode := range []string{"success", "delayed-eof", "deadline", "cancel"} {
			t.Run(flow+"/"+mode, func(t *testing.T) {
				root := t.TempDir()
				fifoPath := filepath.Join(root, "input.fifo")
				if syscall.Mkfifo(fifoPath, 0600) != nil {
					t.Fatal("private FIFO setup failed")
				}
				// The keeper makes opening the read end nonblocking to the parent,
				// while fd 0 in the child remains blocking. Retain the writer for
				// deadline/cancel cases so no EOF can accidentally unblock a read.
				keeper, err := os.OpenFile(fifoPath, os.O_RDWR, 0)
				if err != nil {
					t.Fatal("FIFO keeper setup failed")
				}
				defer keeper.Close()
				reader, err := os.Open(fifoPath)
				if err != nil {
					t.Fatal("FIFO reader setup failed")
				}
				defer reader.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestManualInheritedInputChild$")
				command.Env = []string{"G02_INPUT_FIXTURE=" + flow + ":" + mode, "G02_INPUT_JOURNAL=" + filepath.Join(root, "attempt"), "GORACE=atexit_sleep_ms=0"}
				command.Stdin = reader
				var output strings.Builder
				command.Stdout, command.Stderr = &output, &output
				if command.Start() != nil {
					t.Fatal("fixture child start failed")
				}
				_ = reader.Close()
				if mode == "success" || mode == "delayed-eof" {
					payload := candidate.PEM
					if _, err := keeper.Write(payload); err != nil {
						t.Error("synthetic private input write failed")
					}
					if mode == "delayed-eof" {
						time.Sleep(300 * time.Millisecond)
					}
					_ = keeper.Close()
				}
				if err := command.Wait(); err != nil || ctx.Err() != nil || !strings.Contains(output.String(), "inherited-input-ok") {
					t.Fatal("inherited private input failed, returned early, or exceeded its cancellation guard")
				}
			})
		}
	}
}

func TestManualInheritedInputChild(t *testing.T) {
	fixture := strings.Split(os.Getenv("G02_INPUT_FIXTURE"), ":")
	if len(fixture) != 2 {
		t.Skip("subprocess-only fixture")
	}
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 || info.Mode().Perm() != 0600 {
		t.Fatal("fixture did not inherit the private FIFO")
	}
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, 0, syscall.F_GETFL, 0)
	if errno != 0 || flags&syscall.O_NONBLOCK != 0 {
		t.Fatal("fixture did not inherit blocking stdin")
	}
	// This guard reports the old blocking-read defect without leaving a test
	// process hung. It does not supply the behavior being tested.
	guard := time.AfterFunc(2*time.Second, func() { os.Exit(42) })
	defer guard.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if fixture[1] == "deadline" {
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), 150*time.Millisecond)
	} else if fixture[1] == "cancel" {
		timer := time.AfterFunc(150*time.Millisecond, cancel)
		defer timer.Stop()
	}
	defer cancel()
	wantSuccess := fixture[1] == "success" || fixture[1] == "delayed-eof"
	if fixture[0] == "manual" {
		p := driverProposal()
		p.Organizations[0].InstallationID, p.Organizations[1].InstallationID = 201, 202
		api := &manualIdentityAPI{driverFake: &driverFake{fakeAPI: validAPI()}}
		result, e := VerifyManual(ctx, p, os.Getenv("G02_INPUT_JOURNAL"), 71, os.Stdin, api)
		err = e
		if !wantSuccess && api.identityCalls != 0 {
			t.Fatal("incomplete input reached credential authentication")
		}
		if wantSuccess && (!result.CredentialsNotPersisted || result.VerifiedOrganizations != 2) {
			t.Fatal("successful inherited input did not complete verification")
		}
	} else {
		t.Fatal("unknown fixture")
	}
	if wantSuccess {
		if err != nil {
			t.Fatal("successful inherited input failed")
		}
	} else if err == nil || ctx.Err() == nil || (fixture[1] == "cancel" && ctx.Err() != context.Canceled) {
		t.Fatal("input returned without honoring the requested cancellation")
	}
	fmt.Println("inherited-input-ok")
}

type failedPrivateRead struct{}

func (failedPrivateRead) Read(b []byte) (int, error) {
	return copy(b, "synthetic-private-prefix"), errors.New("synthetic read failure")
}
func (failedPrivateRead) Close() error { return nil }

func TestPrivateInputPreservesBudgetAndRegularFile(t *testing.T) {
	for _, n := range []int{32768, 32769} {
		data, err := readPrivateInput(context.Background(), io.NopCloser(strings.NewReader(strings.Repeat("x", n))), 32768)
		if n == 32768 && (err != nil || len(data) != n) {
			t.Fatal("exact bounded input refused")
		}
		if n == 32769 && (err == nil || len(data) != 0) {
			t.Fatal("oversized input returned bytes")
		}
	}
	data, err := readPrivateInput(context.Background(), failedPrivateRead{}, 32768)
	if err == nil || len(data) != 0 {
		t.Fatal("failed input exposed a partial prefix")
	}
	f, err := os.CreateTemp(t.TempDir(), "private-input")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := io.WriteString(f, "synthetic-private-input"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err = readPrivateInput(context.Background(), f, 32768)
	if err != nil || string(data) != "synthetic-private-input" {
		t.Fatal("bounded regular input refused")
	}
}
