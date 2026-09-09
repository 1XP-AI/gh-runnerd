// g01-broker is an opt-in controller credential experiment, not a product store.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
)

func run(ctx context.Context, args []string, input *os.File, out io.Writer) (code int) {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(out, "broker stopped; retain private intent for review")
			code = 1
		}
	}()
	reject := func() int {
		fmt.Fprintln(out, "broker refused; exact approval, private files and reviewed controller required")
		return 1
	}
	flags := flag.NewFlagSet("g01-broker", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	plan := flags.Bool("plan", false, "")
	execute := flags.Bool("execute-approved-broker", false, "")
	var files enrollment.BrokerFiles
	flags.StringVar(&files.ApprovalPath, "approval", "", "")
	flags.StringVar(&files.StateDirectory, "state-dir", "", "")
	flags.StringVar(&files.ControllerBinary, "controller-binary", "", "")
	flags.StringVar(&files.ControllerApproval, "controller-approval", "", "")
	flags.StringVar(&files.ControllerStateDirectory, "controller-state-dir", "", "")
	flags.StringVar(&files.WorkerApproval, "worker-approval", "", "")
	flags.StringVar(&files.WorkerStateDirectory, "worker-state-dir", "", "")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return reject()
	}
	if *plan && !*execute {
		fmt.Fprintln(out, "Modes in the exact private approval: discover-actions-host, controller, or paired-terminal. Paired-terminal uses one fixed g01-live executable invocation with explicit worker approval/state inputs and one bounded controller credential stdin; no worker subprocess, App creation, workflow dispatch or persistent credentials. Required fixed private native-account admission root and owner nonce; one permanently consumed issuance slot per approved mode/phase. Live execution requires separate explicit authorization.")
		return 0
	}
	if !*execute || *plan || files.ApprovalPath == "" || files.StateDirectory == "" {
		return reject()
	}
	result, err := enrollment.RunBroker(ctx, files, input)
	if err != nil {
		fmt.Fprintln(out, "broker stopped; retain private intent; credentials not persisted; no automatic retry")
		return 1
	}
	if json.NewEncoder(out).Encode(result) != nil {
		return 1
	}
	return 0
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout))
}
