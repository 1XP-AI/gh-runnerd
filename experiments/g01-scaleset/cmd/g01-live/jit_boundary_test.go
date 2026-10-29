//go:build g01_live

package main

import (
	"bytes"
	"encoding/json"
	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJITBoundaryPreparationCommandKeepsRawPathAndPhase(t *testing.T) {
	for _, phase := range []string{"before-jit", "jit-loss"} {
		t.Run(phase, func(t *testing.T) {
			a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowRef: "refs/heads/main", WorkflowRunID: 7, WorkflowPath: ".github/workflows/canary.yml", Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", phase}}
			dir := t.TempDir()
			path := filepath.Join(dir, "approval.json")
			raw, _ := json.Marshal(a)
			os.WriteFile(path, raw, 0600)
			original := prepareJITBoundaryJournalForCommand
			defer func() { prepareJITBoundaryJournalForCommand = original }()
			called := 0
			prepareJITBoundaryJournalForCommand = func(state string, got livecanary.Approval, approvalPath, gotPhase string) (livecanary.PreparationReceipt, error) {
				called++
				if state != dir || approvalPath != path || gotPhase != phase || got.OwnerNonce != a.OwnerNonce {
					t.Fatal("raw preparation identity changed")
				}
				return livecanary.PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: phase}, nil
			}
			var out bytes.Buffer
			if runWithPreparation([]string{"--prepare-approved-journal", "--approval", path, "--state-dir", dir, "--phase", phase}, unreadable{t}, &out, func() (string, bool) { return a.HarnessSHA, true }, func(string, livecanary.Approval, string) (livecanary.PreparationReceipt, error) {
				t.Fatal("JIT command selected decoded-only preparation")
				return livecanary.PreparationReceipt{}, nil
			}) != 0 || called != 1 {
				t.Fatal("JIT preparation failed")
			}
			if strings.Contains(out.String(), dir) {
				t.Fatal("private path disclosed")
			}
		})
	}
}
