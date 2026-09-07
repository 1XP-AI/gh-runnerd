// g02-enroll is a verify-only experiment. It never persists a private key or
// requests installation/runner tokens. No browser is opened automatically.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
)

type orgFlags []string

func (v *orgFlags) String() string { return "organization binding" }
func (v *orgFlags) Set(s string) error {
	if len(*v) >= 2 {
		return errors.New("exactly two organizations required")
	}
	*v = append(*v, s)
	return nil
}

const usage = `Verify-only GitHub App enrollment experiment.
Usage: g02-enroll manifest|manual --live-github --owner ORGANIZATION --app-name NAME --org LOGIN:ORG_ID[:INSTALLATION_ID] --org LOGIN:ORG_ID[:INSTALLATION_ID] --journal-dir PRIVATE_DIRECTORY [--app-id ID]
Manifest: one local browser flow, at most ten minutes. Manual: App ID and both installation IDs required; read PEM only from protected redirected stdin or a private pipe.
The existing journal prevents a fresh Manifest registration. Reconcile the existing App and use manual import after interruption. Credentials are not persisted.
`

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != value {
		return 0, errors.New("invalid ID")
	}
	return id, nil
}
func privateInput(input *os.File) bool {
	if input == nil {
		return false
	}
	info, err := input.Stat()
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm()&0077 != 0 {
		return false
	}
	return (info.Mode().IsRegular() && stat.Nlink == 1) || info.Mode()&os.ModeNamedPipe != 0
}
func run(ctx context.Context, args []string, input *os.File, out, diagnostics io.Writer, api enrollment.DriverAPI) int {
	bad := func() int {
		fmt.Fprintln(diagnostics, "invalid invocation; use --help; private inputs and argument values are not echoed")
		return 2
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(out, usage)
		return 0
	}
	if len(args) == 0 || (args[0] != "manifest" && args[0] != "manual") {
		return bad()
	}
	manual := args[0] == "manual"
	flags := flag.NewFlagSet("g02-enroll", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var owner, name, path, appIDText string
	var live bool
	var orgs orgFlags
	flags.StringVar(&owner, "owner", "", "")
	flags.StringVar(&name, "app-name", "", "")
	flags.StringVar(&path, "journal-dir", "", "")
	flags.StringVar(&appIDText, "app-id", "", "")
	flags.BoolVar(&live, "live-github", false, "")
	flags.Var(&orgs, "org", "")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || !live || owner == "" || name == "" || path == "" || len(orgs) != 2 || (!manual && appIDText != "") {
		return bad()
	}
	proposal := enrollment.Proposal{Owner: owner, AppName: name}
	for _, org := range orgs {
		fields := strings.Split(org, ":")
		want := 2
		if manual {
			want = 3
		}
		if len(fields) != want {
			return bad()
		}
		id, err := parseID(fields[1])
		if err != nil {
			return bad()
		}
		binding := enrollment.Binding{Login: fields[0], OrganizationID: id}
		if manual {
			binding.InstallationID, err = parseID(fields[2])
			if err != nil {
				return bad()
			}
		}
		proposal.Organizations = append(proposal.Organizations, binding)
	}
	var appID int64
	if manual {
		var err error
		appID, err = parseID(appIDText)
		if err != nil || !privateInput(input) {
			return bad()
		}
	}
	if api == nil {
		api = enrollment.NewGitHubAPI(time.Now, nil)
	}
	var summary enrollment.DriverSummary
	var err error
	if manual {
		summary, err = enrollment.VerifyManual(ctx, proposal, path, appID, input, api)
	} else {
		var driver *enrollment.Driver
		driver, err = enrollment.StartManifest(ctx, proposal, path, api, 10*time.Minute)
		if err == nil {
			// The bare loopback URL has no state, code, token or private path.
			ready := struct {
				Status                  string `json:"status"`
				URL                     string `json:"url"`
				CredentialsNotPersisted bool   `json:"credentials_not_persisted"`
			}{"awaiting_local_browser", driver.URL(), true}
			if json.NewEncoder(out).Encode(ready) != nil {
				driver.Close()
				return 1
			}
			summary, err = driver.Wait()
		}
	}
	if err != nil {
		fmt.Fprintln(diagnostics, "verification incomplete; inspect the recorded App and use manual import; credentials_not_persisted=true")
		return 1
	}
	if json.NewEncoder(out).Encode(summary) != nil {
		return 1
	}
	return 0
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, nil))
}
