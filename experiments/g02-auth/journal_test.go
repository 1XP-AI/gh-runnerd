package enrollment

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJournalAbruptExitPreservesOwnershipAndReleasesLock(t *testing.T) {
	if root := os.Getenv("G02_TEST_CRASH_DIRECTORY"); root != "" {
		j, err := openJournal(root, driverProposal(), false, 0)
		if err != nil || j.phase("registration_started", 0, nil) != nil {
			os.Exit(2)
		}
		// Deliberately omit Close: prove the kernel releases the exact flock while
		// the non-secret registration inventory survives an abrupt process exit.
		os.Exit(0)
	}
	root := filepath.Join(t.TempDir(), "attempt")
	command := exec.Command(os.Args[0], "-test.run=^TestJournalAbruptExitPreservesOwnershipAndReleasesLock$")
	command.Env = []string{"G02_TEST_CRASH_DIRECTORY=" + root}
	if command.Run() != nil {
		t.Fatal("synthetic journal child failed")
	}
	api := &driverFake{fakeAPI: validAPI(), candidate: syntheticCandidate(t)}
	if _, err := StartManifest(context.Background(), driverProposal(), root, api, time.Minute); err == nil {
		t.Fatal("crash permitted another registration")
	}
	p := driverProposal()
	p.Organizations = api.candidate.Organizations
	if result, err := VerifyManual(context.Background(), p, root, 71, io.NopCloser(strings.NewReader(string(api.candidate.PEM))), api); err != nil || result.VerifiedOrganizations != 2 {
		t.Fatal("same-App recovery failed after child exit")
	}
}
