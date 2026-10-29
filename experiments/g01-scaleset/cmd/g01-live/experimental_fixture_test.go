//go:build g01_live && g01_pair_fixture

package main

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
)

// This owned TEST process exercises the actual dispatcher with an explicit
// synthetic build-revision seam. Production build admission rejects its tags.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == "--execute-experimental-phase" || os.Args[1] == "--prepare-approved-journal" || os.Args[1] == "--prepare-approved-paired-journal" || os.Args[1] == "--prepare-approved-paired-worker-journal") {
		var state, approvalPath string
		for n := 2; n+1 < len(os.Args); n++ {
			if os.Args[n] == "--state-dir" {
				state = os.Args[n+1]
			}
			if os.Args[n] == "--approval" {
				approvalPath = os.Args[n+1]
			}
		}
		config, e := readFixtureEndpointConfig(state)
		if e != nil {
			os.Exit(91)
		}
		if config.ExperimentalPolicy != "" {
			if livecanary.ConfigureControllerExperimentForFixture(config.ExperimentalPolicy, config.BaseURL, []byte(config.CAPEM), config.AdmissionDirectory, config.WorkerAdmissionDirectory) != nil {
				os.Exit(92)
			}
		}
		a, e := livecanary.ReadApproval(approvalPath)
		revision := a.HarnessSHA
		// Worker preparation has a different structural approval type, but the
		// fixture's harness identity is fixed and reviewed by its broker request.
		if os.Args[1] == "--prepare-approved-paired-worker-journal" {
			revision = config.HarnessSHA
			e = nil
		}
		if e != nil {
			os.Exit(93)
		}
		prepareReplacementJournalForCommand = func(path string, a livecanary.Approval, approvalPath string) (livecanary.PreparationReceipt, error) {
			return livecanary.PrepareSessionReplacementJournalForExperimentFixture(path, a, approvalPath, config.AdmissionDirectory)
		}
		prepareJITBoundaryJournalForCommand = func(path string, a livecanary.Approval, approvalPath, phase string) (livecanary.PreparationReceipt, error) {
			return livecanary.PrepareJITBoundaryJournalForExperimentFixture(path, a, approvalPath, phase, config.AdmissionDirectory)
		}
		os.Exit(runWithPreparation(os.Args[1:], os.Stdin, os.Stdout, func() (string, bool) { return revision, true }, func(path string, a livecanary.Approval, phase string) (livecanary.PreparationReceipt, error) {
			return livecanary.PrepareJournalForExperimentFixture(path, a, phase, config.AdmissionDirectory)
		}))
	}
	os.Exit(m.Run())
}

func TestExperimentalDispatchCanceledBeforeInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if runWithPreparationContext(ctx, []string{"--execute-experimental-phase", "--mode", "controller", "--approval", "/unread-approved-file", "--state-dir", "/unread-state", "--phase", "inspect"}, unreadable{t}, &out, func() (string, bool) { t.Fatal("canceled command inspected build"); return "", false }, nil) == 0 {
		t.Fatal("canceled command accepted")
	}
}
