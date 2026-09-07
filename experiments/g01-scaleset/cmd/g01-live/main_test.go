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

func TestPreparationCommandNeverReadsCredentialsOrRunsRemotePhase(t *testing.T) {
	a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create"}}
	parent := t.TempDir()
	path := filepath.Join(parent, "approval.json")
	data, _ := json.Marshal(a)
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("fixture")
	}
	args := []string{"--prepare-approved-journal", "--approval", path, "--state-dir", parent, "--phase", "create"}
	for _, kind := range []string{"success", "failed preparation", "mixed execution", "mixed plan", "unapproved phase"} {
		t.Run(kind, func(t *testing.T) {
			argv := append([]string(nil), args...)
			called := 0
			switch kind {
			case "mixed execution":
				argv = append(argv, "--execute-approved-canary")
			case "mixed plan":
				argv = append(argv, "--plan")
			case "unapproved phase":
				argv[len(argv)-1] = "cleanup"
			}
			var out bytes.Buffer
			code := runWithPreparation(argv, unreadable{t}, &out, func() (string, bool) { return a.HarnessSHA, true }, func(state string, got livecanary.Approval, phase string) (livecanary.PreparationReceipt, error) {
				called++
				if state != parent || got.HarnessSHA != a.HarnessSHA || phase != "create" {
					t.Fatal("preparation identity changed")
				}
				if kind == "failed preparation" {
					return livecanary.PreparationReceipt{}, livecanary.ErrJournal
				}
				return livecanary.PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: phase}, nil
			})
			if kind == "success" {
				if code != 0 || called != 1 || !strings.Contains(out.String(), `"status":"controller_journal_prepared"`) {
					t.Fatal("fixed local success missing")
				}
			} else {
				if code == 0 {
					t.Fatal("ambiguous preparation accepted")
				}
				if kind != "failed preparation" && called != 0 {
					t.Fatal("invalid flags/phase reached state preparation")
				}
			}
			if strings.Contains(out.String(), parent) {
				t.Fatal("private path disclosed")
			}
		})
	}
}
