//go:build g01_pair_fixture && !g01_live && !g01_worker

package liveworker

import (
	"os"
	"path/filepath"
	"testing"
)

// PairFileFixture is trusted generated-root test support, not a runtime opener.
// No caller-supplied root or account lookup is accepted.
type PairFileFixture struct {
	Journal            *FileJournal
	stateDirectory     string
	admissionDirectory string
}

func NewPairFixtureForTest(t *testing.T, a Approval) *PairFileFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("worker fixture root")
	}
	f := &PairFileFixture{stateDirectory: filepath.Join(root, "state"), admissionDirectory: filepath.Join(root, "admission")}
	if os.Mkdir(f.stateDirectory, 0700) != nil {
		t.Fatal("worker fixture state")
	}
	if os.Mkdir(f.admissionDirectory, 0700) != nil {
		t.Fatal("worker fixture admission")
	}
	parent, err := os.Open(root)
	if err != nil {
		t.Fatal("worker fixture parent")
	}
	if err := parent.Sync(); err != nil {
		_ = parent.Close()
		t.Fatal("worker fixture sync")
	}
	_ = parent.Close()
	if err := f.Reopen(a); err != nil {
		t.Fatal("worker fixture journal")
	}
	t.Cleanup(func() {
		if f.Journal != nil {
			_ = f.Journal.Close()
		}
	})
	return f
}

func (f *PairFileFixture) Reopen(a Approval) error {
	if f.Journal != nil {
		if err := f.Journal.Close(); err != nil {
			return err
		}
	}
	j, err := openJournalAtAdmission(f.stateDirectory, a, f.admissionDirectory, func(file *os.File) error { return file.Sync() })
	if err == nil {
		f.Journal = j
	}
	return err
}

func (f *PairFileFixture) StateDirectory() string     { return f.stateDirectory }
func (f *PairFileFixture) AdmissionDirectory() string { return f.admissionDirectory }
