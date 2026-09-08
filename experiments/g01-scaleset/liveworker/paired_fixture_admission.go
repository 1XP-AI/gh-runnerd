//go:build g01_pair_fixture

package liveworker

import (
	"os"
)

// OpenJournalForPairedFixture is a private test-only seam. It accepts only a
// generated fixture admission root and is unavailable from ordinary builds;
// production OpenJournal continues to derive its permanent root from the
// native account.
func OpenJournalForPairedFixture(directory string, a Approval, admissionDirectory string) (*FileJournal, error) {
	return openJournalAtAdmission(directory, a, admissionDirectory, func(file *os.File) error { return file.Sync() })
}

// PrepareJournalForPairedFixture is the explicit offline-only counterpart to
// PrepareJournal. It keeps the canonical worker parser and admission checks in
// this package while allowing generated tests to supply a disposable root.
func PrepareJournalForPairedFixture(directory string, a Approval, admissionDirectory string) (PreparationReceipt, error) {
	return prepareJournal(directory, a, func(path string, approval Approval) (*FileJournal, error) {
		return OpenJournalForPairedFixture(path, approval, admissionDirectory)
	})
}
