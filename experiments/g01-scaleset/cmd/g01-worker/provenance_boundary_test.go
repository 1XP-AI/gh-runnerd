//go:build g01_worker && g01_pair_fixture

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

type countedJITInput struct {
	reader *strings.Reader
	reads  int
}

func (r *countedJITInput) Read(p []byte) (int, error) {
	r.reads++
	return r.reader.Read(p)
}

func TestStandaloneWorkerRejectsLocalApprovalBeforeReadingJIT(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "state")
	admission := filepath.Join(root, "admission")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(admission, 0700); err != nil {
		t.Fatal(err)
	}
	state, err = filepath.EvalSymlinks(state)
	if err != nil {
		t.Fatal(err)
	}
	admission, err = filepath.EvalSymlinks(admission)
	if err != nil {
		t.Fatal(err)
	}
	a := liveworker.Approval{
		RunnerUpdatesDisabled: true,
		HarnessSHA:            strings.Repeat("b", 40),
		WorkflowSHA:           strings.Repeat("c", 40),
		OwnerNonce:            strings.Repeat("a", 32),
		Controller:            "fixture-controller",
		Endpoint:              "/tmp/g01-worker-fixture.sock",
		DaemonID:              "fixture-daemon",
		ImageID:               "sha256:" + strings.Repeat("d", 64),
		Image:                 liveworker.ImageReference,
		ExpiresAt:             time.Now().Add(time.Hour),
		Phases:                []string{"create"},
	}
	if _, err := liveworker.PrepareJournalForPairedFixture(state, a, admission); err != nil {
		t.Fatalf("prepare disposable local journal: %v", err)
	}
	approvalPath := filepath.Join(root, "approval.json")
	approvalData, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(approvalPath, approvalData, 0600); err != nil {
		t.Fatal(err)
	}
	input := &countedJITInput{reader: strings.NewReader(`{"jit_config":1}`)}
	var out bytes.Buffer
	code := run([]string{"--execute-approved-worker", "--approval", approvalPath, "--state-dir", state, "--phase", "create"}, input, &out)
	if code == 0 || input.reads != 0 {
		t.Fatalf("unproven worker execution read caller JIT: code=%d reads=%d output=%q", code, input.reads, out.String())
	}
}
