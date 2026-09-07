//go:build g01_worker

package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type unreadable struct{ t *testing.T }

func (r unreadable) Read([]byte) (int, error) {
	r.t.Fatal("JIT read before exact execution approval")
	return 0, nil
}

func TestBlockedJITInputHonorsDeadline(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	// Unblock even an unsafe reader so the red result is an assertion failure.
	watchdog := time.AfterFunc(200*time.Millisecond, func() { writer.Close() })
	defer watchdog.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := readJIT(ctx, reader); err == nil {
		t.Fatal("blocked input accepted")
	}
	if time.Since(start) > 150*time.Millisecond {
		t.Fatal("blocked secret input exceeded deadline")
	}
}
func TestOfflinePlanAndRefusalDoNotReadSecretsOrEchoInput(t *testing.T) {
	for _, args := range [][]string{{"--plan"}, {}, {"--synthetic-secret=private-value"}, {"--execute-approved-worker", "--approval=synthetic-secret", "--state-dir=synthetic-secret", "--phase=create"}} {
		var out bytes.Buffer
		code := run(args, unreadable{t}, &out)
		if len(args) == 1 && args[0] == "--plan" {
			if code != 0 {
				t.Fatal("plan failed")
			}
		} else if code == 0 {
			t.Fatal("unapproved invocation accepted")
		}
		if strings.Contains(out.String(), "synthetic-secret") || strings.Contains(out.String(), "private-value") {
			t.Fatal("input leaked")
		}
	}
}
