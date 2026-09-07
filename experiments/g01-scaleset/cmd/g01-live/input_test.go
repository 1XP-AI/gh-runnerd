//go:build g01_live

package main

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestBlockedCredentialPipeStopsAtDeadline(t *testing.T) {
	r, w := io.Pipe()
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
