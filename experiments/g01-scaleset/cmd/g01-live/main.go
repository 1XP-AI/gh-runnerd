//go:build g01_live

// This command is absent from ordinary builds. Building it is not authorization
// to execute it; live calls additionally require a private exact-commit approval.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

func buildRevision() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.GoVersion != "go1.26.8" {
		return "", false
	}
	version, clean, sdk := "", false, false
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			version = setting.Value
		}
		if setting.Key == "vcs.modified" {
			clean = setting.Value == "false"
		}
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/actions/scaleset" {
			sdk = dep.Version == "v0.4.0" && dep.Replace == nil
		}
	}
	return version, clean && sdk && len(version) == 40
}

var openJournalForCommand = livecanary.OpenJournal
var pairedPrepareJournalForCommand = livecanary.PreparePairedJournal
var prepareWorkerJournalForCommand = liveworker.PrepareJournal
var newSDKAPIForCommand = func(a livecanary.Approval, c livecanary.Credentials, _ string) (*livecanary.SDKAPI, error) {
	return livecanary.NewSDKAPI(a, c)
}
var runPairedTerminalForCommand = func(ctx context.Context, files livecanary.PairedTerminalFiles, c livecanary.Credentials) error {
	return livecanary.RunPairedTerminal(ctx, files, c)
}

func run(args []string, in io.Reader, out io.Writer) int {
	return runWithPreparation(args, in, out, buildRevision, livecanary.PrepareJournal)
}
func runWithPreparation(args []string, in io.Reader, out io.Writer, revisionForBuild func() (string, bool), prepareJournal func(string, livecanary.Approval, string) (livecanary.PreparationReceipt, error)) (code int) {
	// SDK errors and panic values can contain bearer credentials/response bodies.
	// This last-resort boundary prints no dynamic exception or caller input.
	defer func() {
		if recover() != nil {
			fmt.Fprintln(out, "canary stopped; retain private state for review")
			code = 1
		}
	}()
	flags := flag.NewFlagSet("g01-live", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	plan := flags.Bool("plan", false, "")
	execute := flags.Bool("execute-approved-canary", false, "")
	pairedExecute := flags.Bool("execute-approved-paired-terminal", false, "")
	prepare := flags.Bool("prepare-approved-journal", false, "")
	pairedPrepare := flags.Bool("prepare-approved-paired-journal", false, "")
	prepareWorker := flags.Bool("prepare-approved-paired-worker-journal", false, "")
	approvalPath := flags.String("approval", "", "")
	statePath := flags.String("state-dir", "", "")
	phase := flags.String("phase", "", "")
	workerApprovalPath := flags.String("worker-approval", "", "")
	workerStatePath := flags.String("worker-state-dir", "", "")
	pairedBinding := flags.String("paired-binding", "", "")
	reject := func() int {
		fmt.Fprintln(out, "canary refused; approval, authority or private state requires review")
		return 1
	}
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return reject()
	}
	workerInputs := *workerApprovalPath != "" || *workerStatePath != ""
	if *plan && !*execute && !*pairedExecute && !*prepare && !*pairedPrepare && !*prepareWorker {
		if *approvalPath != "" || *statePath != "" || *phase != "" || workerInputs || *pairedBinding != "" {
			return reject()
		}
		fmt.Fprintln(out, "Controller-only phases: create, before-ack, after-ack, before-acquire, acquire-loss, jit-loss, inspect, cleanup. Paired terminal mode uses one same-process executable with explicit worker approval/state inputs and fixed terminal sequencing; no worker launch or workflow dispatch. Live execution requires an immutable reviewed build, exact private approval and controller-side broker input.")
		return 0
	}
	modeCount := 0
	if *execute {
		modeCount++
	}
	if *pairedExecute {
		modeCount++
	}
	if *prepare {
		modeCount++
	}
	if *pairedPrepare {
		modeCount++
	}
	if *prepareWorker {
		modeCount++
	}
	if modeCount != 1 || *plan || *approvalPath == "" || *statePath == "" {
		return reject()
	}
	if *prepareWorker {
		if *phase != "" || workerInputs || *pairedBinding != "" || prepareWorkerJournalForCommand == nil {
			return reject()
		}
	} else if *pairedExecute {
		if *phase != "" || *workerApprovalPath == "" || *workerStatePath == "" || *pairedBinding == "" {
			return reject()
		}
	} else if *pairedPrepare {
		if *phase != "" || workerInputs || *pairedBinding != "" {
			return reject()
		}
	} else if *phase == "" || workerInputs || *pairedBinding != "" {
		return reject()
	}
	if *prepareWorker {
		worker, workerErr := liveworker.ReadApproval(*approvalPath)
		if workerErr != nil || worker.Validate(time.Now()) != nil {
			return reject()
		}
		revision, ok := revisionForBuild()
		if !ok || revision != worker.HarnessSHA {
			return reject()
		}
		receipt, preparationErr := prepareWorkerJournalForCommand(*statePath, worker)
		if preparationErr != nil || json.NewEncoder(out).Encode(receipt) != nil {
			return reject()
		}
		return 0
	}
	a, err := livecanary.ReadApproval(*approvalPath)
	if err != nil || a.Validate(time.Now()) != nil || (!*pairedExecute && !*pairedPrepare && !slices.Contains(a.Phases, *phase)) {
		return reject()
	}
	revision, ok := revisionForBuild()
	if !ok || revision != a.HarnessSHA {
		return reject()
	}
	var worker liveworker.Approval
	var binding livecanary.PairedTerminalBinding
	if *pairedExecute {
		var workerErr error
		worker, workerErr = liveworker.ReadApproval(*workerApprovalPath)
		if workerErr != nil || livecanary.ValidatePairedApprovals(a, worker) != nil || livecanary.ValidatePairedStatePaths(*statePath, *workerStatePath) != nil {
			return reject()
		}
		if livecanary.DecodeStrict([]byte(*pairedBinding), &binding) != nil || livecanary.ValidatePairedTerminalBinding(livecanary.PairedTerminalFiles{ControllerApprovalPath: *approvalPath, ControllerStateDirectory: *statePath, WorkerApprovalPath: *workerApprovalPath, WorkerStateDirectory: *workerStatePath}, binding) != nil {
			return reject()
		}
	}
	if *pairedPrepare {
		receipt, e := pairedPrepareJournalForCommand(*statePath, a)
		if e != nil || json.NewEncoder(out).Encode(receipt) != nil {
			return reject()
		}
		return 0
	}
	if *prepare {
		if prepareJournal == nil {
			return reject()
		}
		receipt, e := prepareJournal(*statePath, a, *phase)
		if e != nil {
			return reject()
		}
		if json.NewEncoder(out).Encode(receipt) != nil {
			return reject()
		}
		return 0
	}
	var j *livecanary.FileJournal
	if !*pairedExecute {
		j, err = openJournalForCommand(*statePath, a)
		if err != nil {
			return reject()
		}
		defer j.Close()
	}
	closable, ok := in.(io.ReadCloser)
	if !ok {
		return reject()
	}
	inputDeadline := time.Now().Add(30 * time.Second)
	if a.ExpiresAt.Before(inputDeadline) {
		inputDeadline = a.ExpiresAt
	}
	inputContext, cancelInput := context.WithDeadline(context.Background(), inputDeadline)
	data, err := readCredentialInput(inputContext, closable)
	cancelInput()
	if err != nil {
		return reject()
	}
	defer clear(data) // Best effort; Go/SDK strings and memory retain copies.
	var credentials livecanary.Credentials
	if livecanary.DecodeStrict(data, &credentials) != nil {
		return reject()
	}
	if *pairedExecute {
		if credentials.PairedBinding == nil || *credentials.PairedBinding != binding {
			return reject()
		}
		if runPairedTerminalForCommand(context.Background(), livecanary.PairedTerminalFiles{ControllerApprovalPath: *approvalPath, ControllerStateDirectory: *statePath, WorkerApprovalPath: *workerApprovalPath, WorkerStateDirectory: *workerStatePath}, credentials) != nil {
			fmt.Fprintln(out, "paired terminal stopped; retain private state and all uncertain resources; no automatic retry")
			return 1
		}
		fmt.Fprintln(out, "paired terminal completed; inspect private evidence")
		return 0
	}
	api, err := newSDKAPIForCommand(a, credentials, *statePath)
	if err != nil {
		return reject()
	}
	driver := livecanary.Driver{Approval: a, Journal: j, API: api}
	if driver.Run(context.Background(), *phase) != nil {
		fmt.Fprintln(out, "canary stopped; retain the private journal and all uncertain resources; no automatic retry")
		return 1
	}
	fmt.Fprintln(out, "controller phase completed; live gate remains unresolved; inspect private evidence")
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout)) }
