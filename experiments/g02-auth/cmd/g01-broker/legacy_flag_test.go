package main

import (
	"bytes"
	"context"
	"testing"
)

func TestLegacyApprovedBrokerFlagIsRejectedBeforeOpeningPrivateFiles(t *testing.T) {
	var out bytes.Buffer
	args := []string{"--execute-approved-broker", "--approval", "synthetic-private-path", "--state-dir", "synthetic-private-state"}
	if code := run(context.Background(), args, nil, &out); code == 0 {
		t.Fatal("legacy approved-broker mode accepted")
	}
	if out.String() != "broker refused; exact approval, private files and reviewed controller required\n" {
		t.Fatalf("legacy route reached execution instead of early refusal: %q", out.String())
	}
}
