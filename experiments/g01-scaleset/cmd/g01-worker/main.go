//go:build g01_worker

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

func buildRevision() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	return buildRevisionFromInfo(info, ok)
}

func buildRevisionFromInfo(info *debug.BuildInfo, ok bool) (string, bool) {
	if !ok || info == nil || info.GoVersion != "go1.27.3" {
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
	flags.String("approval", "", "")
	flags.String("state-dir", "", "")
	flags.String("phase", "", "")
	refuse := func() int {
		fmt.Fprintln(out, "worker refused; exact approval, authority or private state requires review")
		return 1
	}
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return refuse()
	}
	if *execute {
		fmt.Fprintln(out, "standalone worker execution disabled; signed controller handoff is required")
		return 1
	}
	if *plan {
		fmt.Fprintln(out, "Plan only: one preloaded pinned ARM64 image; 1 CPU, 1 GiB, 256 PIDs; non-root, no host mounts/socket, no restart or log capture. This standalone command has no Docker execution path; the signed controller handoff remains separately gated.")
		return 0
	}
	return refuse()
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout)) }
