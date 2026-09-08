//go:build g01_pair_fixture

package liveworker

import (
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func fixtureWorkerStage(directory, stage string) {
	_ = os.WriteFile(filepath.Join(directory, "paired-fixture-worker-stage"), []byte(stage), 0600)
}

// OpenJournalForPairedFixture is a private test-only seam. It accepts only a
// generated fixture admission root and is unavailable from ordinary builds;
// production OpenJournal continues to derive its permanent root from the
// native account.
func OpenJournalForPairedFixture(directory string, a Approval, admissionDirectory string) (*FileJournal, error) {
	oldTrace := pairedFixtureJournalTrace
	pairedFixtureJournalTrace = fixtureWorkerStage
	defer func() { pairedFixtureJournalTrace = oldTrace }()
	fixtureWorkerStage(directory, "start")
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		fixtureWorkerStage(directory, "state-dir")
	} else if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Geteuid() {
		fixtureWorkerStage(directory, "state-owner")
	} else if a.Validate(time.Now()) != nil {
		fixtureWorkerStage(directory, "approval")
	} else if admissionInfo, admissionErr := os.Lstat(admissionDirectory); admissionErr != nil || !admissionInfo.IsDir() || admissionInfo.Mode().Perm() != 0700 {
		fixtureWorkerStage(directory, "admission-dir")
	} else if parentInfo, parentErr := os.Lstat(filepath.Dir(admissionDirectory)); parentErr != nil || parentInfo.Mode().Perm()&0022 != 0 {
		fixtureWorkerStage(directory, "admission-parent")
	}
	j, err := openJournalAtAdmission(directory, a, admissionDirectory, func(file *os.File) error { return file.Sync() })
	if err != nil {
		stage, readErr := os.ReadFile(filepath.Join(directory, "paired-fixture-worker-stage"))
		if readErr != nil || string(stage) == "start" {
			fixtureWorkerStage(directory, "underlying")
		}
	} else {
		fixtureWorkerStage(directory, "ok")
	}
	return j, err
}
