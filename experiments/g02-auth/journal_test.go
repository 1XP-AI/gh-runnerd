package enrollment

import (
	"context"
	"errors"
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

func TestJournalParentSyncPrecedesIntentAndRepeatsAfterFailure(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "attempt")
	calls := 0
	syncFailure := func(root *os.Root) error {
		calls++
		actual, err := root.Stat(".")
		expected, e := os.Stat(parent)
		if err != nil || e != nil || !os.SameFile(actual, expected) {
			t.Error("sync did not capture the existing parent")
		}
		if _, err := root.Stat("attempt"); err != nil {
			t.Error("parent sync preceded directory creation")
		}
		if _, err := root.Stat("attempt/attempt.json"); !os.IsNotExist(err) {
			t.Error("intent written before parent sync")
		}
		return errors.New("synthetic-sync-failure")
	}
	for i := 0; i < 2; i++ {
		j, err := openJournalWithParentSync(path, driverProposal(), false, 0, syncFailure)
		if j != nil {
			j.close()
		}
		if err == nil {
			t.Fatal("unsynced directory accepted")
		}
	}
	if calls != 2 {
		t.Fatalf("parent sync attempts=%d; retry skipped existing entry", calls)
	}
	j, err := openJournalWithParentSync(path, driverProposal(), false, 0, func(root *os.Root) error { calls++; return nil })
	if err != nil {
		t.Fatal("successful sync did not permit initial journal")
	}
	defer j.close()
	if calls != 3 || j.phase("registration_started", 0, nil) != nil {
		t.Fatal("registration did not follow parent sync")
	}
}
