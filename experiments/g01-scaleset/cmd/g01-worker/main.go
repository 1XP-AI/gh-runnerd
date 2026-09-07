//go:build g01_worker

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

func buildRevision() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.GoVersion != "go1.26.8" {
		return "", false
	}
	revision := ""
	clean := false
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			revision = setting.Value
		}
		if setting.Key == "vcs.modified" {
			clean = setting.Value == "false"
		}
	}
	return revision, clean && len(revision) == 40
}

func readJIT(ctx context.Context, in io.Reader) (string, error) {
	type readResult struct {
		data []byte
		err  error
	}
	ready := make(chan readResult)
	go func() {
		data, err := io.ReadAll(io.LimitReader(in, (1<<20)+1025))
		select {
		case ready <- readResult{data, err}:
		case <-ctx.Done():
			clear(data)
		}
	}()
	var result readResult
	select {
	case result = <-ready:
	case <-ctx.Done():
		return "", liveworker.ErrApproval
	}
	data, err := result.data, result.err
	defer clear(data)
	if err != nil || ctx.Err() != nil || len(data) > (1<<20)+1024 {
		return "", liveworker.ErrApproval
	}
	var input struct {
		JIT string `json:"jit_config"`
	}
	if liveworker.DecodeStrict(data, &input) != nil {
		return "", liveworker.ErrApproval
	}
	return input.JIT, nil
}

func run(args []string, in io.Reader, out io.Writer) (code int) {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(out, "worker experiment stopped; retain private state and reservation")
			code = 1
		}
	}()
	flags := flag.NewFlagSet("g01-worker", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	plan := flags.Bool("plan", false, "")
	execute := flags.Bool("execute-approved-worker", false, "")
	approvalPath := flags.String("approval", "", "")
	statePath := flags.String("state-dir", "", "")
	phase := flags.String("phase", "", "")
	refuse := func() int {
		fmt.Fprintln(out, "worker refused; exact approval, authority or private state requires review")
		return 1
	}
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return refuse()
	}
	if *plan && !*execute {
		fmt.Fprintln(out, "Explicit worker phases: create, start, inspect, cleanup. One preloaded pinned ARM64 image; 1 CPU, 1 GiB, 256 PIDs; non-root, no host mounts/socket, no restart, no log capture. No pull, dispatch, stop or kill. Reviewed exact-build approval and controller-provided JIT are required.")
		return 0
	}
	if !*execute || *plan || *approvalPath == "" || *statePath == "" || *phase == "" {
		return refuse()
	}
	a, err := liveworker.ReadApproval(*approvalPath)
	if err != nil || a.Validate(time.Now()) != nil {
		return refuse()
	}
	revision, ok := buildRevision()
	if !ok || revision != a.HarnessSHA {
		return refuse()
	}
	journal, err := liveworker.OpenJournal(*statePath, a)
	if err != nil {
		return refuse()
	}
	defer journal.Close()
	jit := ""
	if *phase == "create" {
		deadline := time.Now().Add(30 * time.Second)
		if a.ExpiresAt.Before(deadline) {
			deadline = a.ExpiresAt
		}
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		jit, err = readJIT(ctx, in)
		cancel()
		if err != nil {
			return refuse()
		}
	}
	runtime, err := liveworker.NewDocker(a)
	if err != nil {
		return refuse()
	}
	driver := liveworker.Driver{Approval: a, Journal: journal, Runtime: runtime}
	if driver.Run(context.Background(), *phase, jit) != nil {
		fmt.Fprintln(out, "worker phase stopped; retain unknown resources and reservation; no automatic retry")
		return 1
	}
	fmt.Fprintln(out, "worker phase completed; GitHub reservation and live evidence gate remain unresolved")
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout)) }
