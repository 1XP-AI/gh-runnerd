package livecanary

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// Cross-module fixtures are ONLY the Go test executable. Production has no
// injected roots/entry point. Preparation requires empty stdin and fixed argv;
// the private process receiver uses compiled test pins and an exclusively fake API.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == "--execute-approved-canary" || os.Args[1] == "--execute-experimental-phase") {
		os.Exit(runProcessHandoffTestChild())
	}
	if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-paired-journal" {
		if len(os.Args) != 6 || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" {
			os.Exit(2)
		}
		input, e := io.ReadAll(io.LimitReader(os.Stdin, 1))
		if e != nil || len(input) != 0 {
			os.Exit(3)
		}
		for _, entry := range os.Environ() {
			if entry != "LANG=C" && entry != "LC_ALL=C" {
				os.Exit(4)
			}
		}
		a, e := ReadApproval(os.Args[3])
		if e != nil {
			os.Exit(5)
		}
		directory, e := filepath.EvalSymlinks(filepath.Join(filepath.Dir(os.Args[5]), "admission"))
		if e != nil {
			os.Exit(6)
		}
		receipt, e := preparePairedJournal(os.Args[5], a, func(path string, a Approval) (*FileJournal, error) {
			return openJournalAtAdmission(path, a, directory, func(f *os.File) error { return f.Sync() })
		})
		if e != nil {
			os.Exit(7)
		}
		if json.NewEncoder(os.Stdout).Encode(receipt) != nil {
			os.Exit(8)
		}
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-journal" {
		os.Exit(runPreparationTestChild())
	}
	os.Exit(m.Run())
}

func runPreparationTestChild() int {
	rootBindingArg := len(os.Args) == 10
	if (len(os.Args) != 8 && len(os.Args) != 10) || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" || os.Args[6] != "--phase" || (rootBindingArg && os.Args[8] != "--root-binding") {
		return 2
	}
	input, e := io.ReadAll(io.LimitReader(os.Stdin, 1))
	if e != nil || len(input) != 0 {
		return 3
	}
	for _, entry := range os.Environ() {
		if entry != "LANG=C" && entry != "LC_ALL=C" {
			return 4
		}
	}
	var requested handoff.RootBinding
	if rootBindingArg {
		if requested, e = handoff.DecodeStrictRootBinding([]byte(os.Args[9])); e != nil {
			return 5
		}
	}
	a, e := ReadApproval(os.Args[3])
	if e != nil {
		return 5
	}
	directory, e := filepath.EvalSymlinks(filepath.Join(filepath.Dir(os.Args[5]), "admission"))
	if e != nil {
		return 6
	}
	if rootBindingArg {
		// The copied controller policy is the fixture's independent public-root
		// source. Derive the receipt from it, then compare the broker argument;
		// never echo the argument as controller evidence.
		policy, policyErr := os.ReadFile(filepath.Join(os.Args[5], "controller-policy.json"))
		if policyErr != nil {
			return 7
		}
		controllerExperimentPolicyJSON = string(policy)
	}
	open := func(path string, a Approval) (*FileJournal, error) {
		return openJournalAtAdmission(path, a, directory, func(f *os.File) error { return f.Sync() })
	}
	var receipt PreparationReceipt
	if rootBindingArg {
		receipt, e = prepareJournalWithRootBinding(os.Args[5], a, os.Args[7], requested, open)
	} else {
		receipt, e = prepareJournal(os.Args[5], a, os.Args[7], open)
	}
	if e != nil {
		return 7
	}
	if json.NewEncoder(os.Stdout).Encode(receipt) != nil {
		return 8
	}
	return 0
}
