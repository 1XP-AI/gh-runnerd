//go:build g01_live

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
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

func TestSpecializedPreparationWithRootBindingUsesControllerDerivedPath(t *testing.T) {
	root := handoff.RootBinding{Version: 1, WorkflowKeyID: "workflow", WorkflowPublicKeySHA256: strings.Repeat("a", 64), IssuerKeyID: "issuer", IssuerPublicKeySHA256: strings.Repeat("b", 64)}
	rootData, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"before-jit", "jit-loss", "session-replacement"} {
		t.Run(phase, func(t *testing.T) {
			a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowRef: "refs/heads/main", WorkflowRunID: 7, WorkflowPath: ".github/workflows/canary.yml", Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", phase}}
			dir := t.TempDir()
			path := filepath.Join(dir, "approval.json")
			raw, _ := json.Marshal(a)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			originalJIT := prepareJITBoundaryJournalForCommand
			originalReplacement := prepareReplacementJournalForCommand
			originalRootJIT := prepareJITBoundaryJournalWithRootBindingForCommand
			originalRootReplacement := prepareReplacementJournalWithRootBindingForCommand
			originalRoot := prepareRootJournalForCommand
			originalVerify := verifyRootBindingForCommand
			defer func() {
				prepareJITBoundaryJournalForCommand = originalJIT
				prepareReplacementJournalForCommand = originalReplacement
				prepareJITBoundaryJournalWithRootBindingForCommand = originalRootJIT
				prepareReplacementJournalWithRootBindingForCommand = originalRootReplacement
				prepareRootJournalForCommand = originalRoot
				verifyRootBindingForCommand = originalVerify
			}()
			nonRootCalls, rootCalls := 0, 0
			legacy := func(state string, got livecanary.Approval, approvalPath, gotPhase string) (livecanary.PreparationReceipt, error) {
				nonRootCalls++
				return livecanary.PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: gotPhase}, nil
			}
			prepareJITBoundaryJournalForCommand = legacy
			prepareReplacementJournalForCommand = func(state string, got livecanary.Approval, approvalPath string) (livecanary.PreparationReceipt, error) {
				nonRootCalls++
				return livecanary.PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: phase}, nil
			}
			prepareJITBoundaryJournalWithRootBindingForCommand = func(state string, got livecanary.Approval, approvalPath, gotPhase string, expected handoff.RootBinding) (livecanary.PreparationReceipt, error) {
				if gotPhase != phase || !expected.Equal(root) {
					t.Fatal("JIT root-aware preparation identity changed")
				}
				rootCalls++
				return livecanary.PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: gotPhase, RootBinding: &expected}, nil
			}
			prepareReplacementJournalWithRootBindingForCommand = func(state string, got livecanary.Approval, approvalPath string, expected handoff.RootBinding) (livecanary.PreparationReceipt, error) {
				if phase != "session-replacement" || !expected.Equal(root) {
					t.Fatal("replacement root-aware preparation identity changed")
				}
				rootCalls++
				return livecanary.PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: phase, RootBinding: &expected}, nil
			}
			prepareRootJournalForCommand = func(string, livecanary.Approval, string, handoff.RootBinding) (livecanary.PreparationReceipt, error) {
				t.Fatal("specialized phase selected generic root preparation")
				return livecanary.PreparationReceipt{}, nil
			}
			verifyRootBindingForCommand = func(got handoff.RootBinding) error {
				if !got.Equal(root) {
					t.Fatal("root verifier received wrong binding")
				}
				return nil
			}
			var out bytes.Buffer
			code := runWithPreparation([]string{"--prepare-approved-journal", "--approval", path, "--state-dir", dir, "--phase", phase, "--root-binding", string(rootData)}, unreadable{t}, &out, func() (string, bool) { return a.HarnessSHA, true }, func(string, livecanary.Approval, string) (livecanary.PreparationReceipt, error) {
				t.Fatal("specialized root preparation selected decoded-only path")
				return livecanary.PreparationReceipt{}, nil
			})
			if code != 0 || rootCalls != 1 || nonRootCalls != 0 {
				t.Fatalf("root binding did not select specialized root-aware preparation: code=%d root=%d nonroot=%d output=%s", code, rootCalls, nonRootCalls, fmt.Sprintf("%q", out.String()))
			}
		})
	}
}
