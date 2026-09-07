package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestBrokerPlanAndRefusalsNeverReadInputOrEchoValues(t *testing.T) {
	var out bytes.Buffer
	if code := run(context.Background(), []string{"--plan"}, nil, &out); code != 0 || !strings.Contains(out.String(), "discover-actions-host") {
		t.Fatal("safe plan unavailable")
	}
	for _, args := range [][]string{{}, {"--synthetic-private-value"}, {"--execute-approved-broker", "--approval", "synthetic-private-path", "--state-dir", "synthetic-private-state"}, {"--plan", "--execute-approved-broker"}} {
		out.Reset()
		if run(context.Background(), args, nil, &out) == 0 {
			t.Fatal("unapproved execution accepted")
		}
		if strings.Contains(out.String(), "synthetic-private") {
			t.Fatal("argument value leaked")
		}
	}
}
