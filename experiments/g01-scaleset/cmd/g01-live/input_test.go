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
	if os.Getenv("G01_INPUT_CHILD") == "blocked" {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)
		go func() { _, err := readCredentialInput(ctx, os.Stdin); done <- err }()
		select {
		case err := <-done:
			if err == nil {
				os.Exit(3)
			}
			os.Exit(0)
		case <-time.After(250 * time.Millisecond):
			os.Exit(2)
		}
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestInheritedCredentialPipeStopsAtDeadline$")
	child.Env = []string{"G01_INPUT_CHILD=blocked"}
	child.Stdin = r
	if err := child.Run(); err != nil {
		t.Fatal("inherited stdin did not stop at its input deadline")
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
