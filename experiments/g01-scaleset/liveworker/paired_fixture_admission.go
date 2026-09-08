//go:build g01_pair_fixture

package liveworker

import "os"

// OpenJournalForPairedFixture is a private test-only seam. It accepts only a
// generated fixture admission root and is unavailable from ordinary builds;
// production OpenJournal continues to derive its permanent root from the
// native account.
func OpenJournalForPairedFixture(directory string, a Approval, admissionDirectory string) (*FileJournal, error) {
	return openJournalAtAdmission(directory, a, admissionDirectory, func(file *os.File) error { return file.Sync() })
}
