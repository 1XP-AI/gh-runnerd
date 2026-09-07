//go:build g01_live

// This command is absent from ordinary builds. Building it is not authorization
// to execute it; live calls additionally require a private exact-commit approval.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
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

func run(args []string, in io.Reader, out io.Writer) (code int) {
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
	approvalPath := flags.String("approval", "", "")
	statePath := flags.String("state-dir", "", "")
	phase := flags.String("phase", "", "")
	reject := func() int {
		fmt.Fprintln(out, "canary refused; approval, authority or private state requires review")
		return 1
	}
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return reject()
	}
	if *plan && !*execute {
		fmt.Fprintln(out, "Controller-only phases: create, before-ack, after-ack, before-acquire, acquire-loss, jit-loss, inspect, cleanup. No worker launch or workflow dispatch. Live execution requires an immutable reviewed build, exact private approval and controller-side broker input.")
		return 0
	}
	if !*execute || *plan || *approvalPath == "" || *statePath == "" || *phase == "" {
		return reject()
	}
	a, err := livecanary.ReadApproval(*approvalPath)
	if err != nil || a.Validate(time.Now()) != nil {
		return reject()
	}
	revision, ok := buildRevision()
	if !ok || revision != a.HarnessSHA {
		return reject()
	}
	j, err := livecanary.OpenJournal(*statePath, a)
	if err != nil {
		return reject()
	}
	defer j.Close()
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
	api, err := livecanary.NewSDKAPI(a, credentials)
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
