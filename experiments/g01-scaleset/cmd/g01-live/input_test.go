//go:build g01_live

package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestInheritedCredentialPipeStopsAtDeadline(t *testing.T) {
	if mode := os.Getenv("G01_INPUT_CHILD"); mode != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		done := make(chan bool, 1)
		go func() {
			data, err := readCredentialInput(ctx, os.Stdin)
			done <- (mode == "blocked" && err != nil) || (mode == "complete" && err == nil && string(data) == "synthetic-credentials")
		}()
		select {
		case accepted := <-done:
			if !accepted {
				os.Exit(3)
			}
			os.Exit(0)
		case <-time.After(250 * time.Millisecond):
			os.Exit(2)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"blocked", "complete"} {
		t.Run(mode, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			defer w.Close()
			if mode == "complete" {
				if _, err := io.WriteString(w, "synthetic-credentials"); err != nil {
					t.Fatal(err)
				}
				_ = w.Close()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, executable, "-test.run=^TestInheritedCredentialPipeStopsAtDeadline$")
			child.Env = []string{"G01_INPUT_CHILD=" + mode}
			child.Stdin = r
			if err := child.Run(); err != nil {
				t.Fatal("inherited stdin did not enforce its input contract")
			}
		})
	}
}

func TestCredentialInputRejectsNonPipeDescriptor(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if data, err := readCredentialInput(context.Background(), f); err == nil || len(data) != 0 {
		t.Fatal("controller accepted a non-pipe credential descriptor")
	}
}

func TestBlockedCredentialPipeStopsAtDeadline(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := readCredentialInput(ctx, r)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("incomplete credentials accepted after deadline")
		}
	case <-time.After(250 * time.Millisecond):
		_ = r.Close()
		<-done
		t.Fatal("blocked credential input outlived its deadline")
	}
}

func TestCredentialInputAcceptsCompleteAndRejectsOversize(t *testing.T) {
	want := `{"installation_token":"synthetic-private-token"}`
	data, err := readCredentialInput(context.Background(), io.NopCloser(strings.NewReader(want)))
	if err != nil || string(data) != want {
		t.Fatal("complete bounded input rejected")
	}
	data, err = readCredentialInput(context.Background(), io.NopCloser(strings.NewReader(strings.Repeat("x", 16385))))
	if err == nil || len(data) != 0 {
		t.Fatal("oversize credential input returned to caller")
	}
}
