//go:build g01_live

package main

import (
	"bytes"
	"strings"
	"testing"
)

type unreadable struct{ t *testing.T }

func (r unreadable) Read([]byte) (int, error) {
	r.t.Fatal("credentials read before explicit exact-build approval")
	return 0, nil
}

func TestPlanAndRefusalsNeverReadCredentialsOrEchoInputs(t *testing.T) {
	for _, args := range [][]string{{"--plan"}, {}, {"--synthetic-secret=do-not-print"}, {"--execute-approved-canary", "--approval=synthetic-secret", "--state-dir=synthetic-secret", "--phase=create"}} {
		var out bytes.Buffer
		code := run(args, unreadable{t}, &out)
		if len(args) == 1 && args[0] == "--plan" {
			if code != 0 {
				t.Fatal("offline plan failed")
			}
		} else if code == 0 {
			t.Fatal("unsafe invocation accepted")
		}
		if strings.Contains(out.String(), "synthetic-secret") || strings.Contains(out.String(), "do-not-print") {
			t.Fatal("caller input leaked")
		}
	}
}
