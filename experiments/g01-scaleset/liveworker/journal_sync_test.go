package liveworker

import (
	"errors"
	"os"
	"testing"
)

func TestFailedDirectorySyncMustBeRetried(t *testing.T) {
	calls := 0
	syncDirectory := func(*os.File) error { calls++; return errors.New("synthetic directory sync failure") }
	dir := t.TempDir()
	a := approval()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("fixture private directory")
	}
	j, err := openJournalWithSync(dir, a, syncDirectory)
	if err == nil || j != nil || calls != 1 {
		if j != nil {
			_ = j.Close()
		}
		t.Fatal("injected initial directory sync failure did not stop")
	}
	j, err = openJournalWithSync(dir, a, syncDirectory)
	if j != nil {
		_ = j.Close()
	}
	if err == nil || calls != 2 {
		t.Fatalf("restart bypassed failed directory durability: error=%v directory_sync_calls=%d", err, calls)
	}
}
