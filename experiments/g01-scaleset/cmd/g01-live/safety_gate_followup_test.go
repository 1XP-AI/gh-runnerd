//go:build g01_live && g01_pair_fixture

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

func TestTaggedControllerExecutionRequiresBrokerProvenanceBeforeInput(t *testing.T) {
	a := livecanary.Approval{
		AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42,
		RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40),
		WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 5, Controller: "fixture-controller",
		ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "inspect", "cleanup", "jit-loss"},
	}
	worker := liveworker.Approval{RunnerUpdatesDisabled: true, HarnessSHA: a.HarnessSHA, WorkflowSHA: a.WorkflowSHA, OwnerNonce: a.OwnerNonce, Controller: a.Controller, Endpoint: "/tmp/g01-paired-docker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: liveworker.ImageReference, ExpiresAt: a.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	root := t.TempDir()
	controllerState := filepath.Join(root, "controller-state")
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	approvalPath := filepath.Join(root, "controller.json")
	workerPath := filepath.Join(root, "worker.json")
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(approvalPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal(err)
	}
	bindingData, err := json.Marshal(pairedBindingFixture(t, approvalPath, controllerState, workerPath, workerState))
	if err != nil {
		t.Fatal(err)
	}
	input := &countedInput{Reader: strings.NewReader(`{}`)}
	var out bytes.Buffer
	code := runWithPreparation(
		[]string{"--execute-approved-paired-terminal", "--approval", approvalPath, "--state-dir", controllerState, "--worker-approval", workerPath, "--worker-state-dir", workerState, "--paired-binding", string(bindingData)},
		input, &out,
		func() (string, bool) { return a.HarnessSHA, true },
		nil,
	)
	if code == 0 {
		t.Fatal("direct tagged controller execution was accepted")
	}
	if input.reads != 0 {
		t.Fatalf("controller input was read %d times before broker provenance", input.reads)
	}
}
