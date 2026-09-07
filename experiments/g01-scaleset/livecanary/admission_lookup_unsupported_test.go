//go:build !cgo || osusergo || android

package livecanary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnsupportedAccountLookupRefusesBeforeJournal(t *testing.T) {
	home, err := filepath.EvalSymlinks(privateDir(t))
	if err != nil {
		t.Fatal("private fixture home")
	}
	t.Setenv("HOME", home)
	t.Setenv("USER", "synthetic-controller")
	if os.Mkdir(filepath.Join(home, ".gh-runnerd-g01-experiment"), 0700) != nil {
		t.Fatal("private fixture admission root")
	}
	state := privateDir(t)
	if j, err := OpenJournal(state, approval()); err == nil {
		_ = j.Close()
		t.Fatal("unsupported lookup admitted an environment-selected root")
	}
	if _, err := os.Lstat(filepath.Join(state, "journal.jsonl")); !os.IsNotExist(err) {
		t.Fatal("unsupported lookup reached journal preparation")
	}
}
