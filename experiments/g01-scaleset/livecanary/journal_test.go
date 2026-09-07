package livecanary

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private directory failed")
	}
	return dir
}

func TestJournalLocksBindsApprovalAndRetainsIncompleteIntent(t *testing.T) {
	dir := privateDir(t)
	a := approval()
	j, err := openTestJournal(t, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := openTestJournal(t, dir, a); err == nil {
		other.Close()
		t.Fatal("second session owner obtained journal")
	}
	if j.Append(Event{Kind: "intent", Operation: "jit"}) != nil {
		t.Fatal("intent failed")
	}
	j.Close()
	reopened, err := openTestJournal(t, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	s := replay(reopened.Events())
	if !s.uncertain || !s.reserved {
		t.Fatal("restart released incomplete worker reservation")
	}
	reopened.Close()
	a.OwnerNonce = strings.Repeat("a", 32)
	if j, err := openTestJournal(t, dir, a); err == nil {
		j.Close()
		t.Fatal("changed approval adopted prior state")
	}
}

func TestJournalRejectsTornTailSymlinksAndSharedModes(t *testing.T) {
	for _, kind := range []string{"tail", "symlink", "hardlink", "mode", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := privateDir(t)
			a := approval()
			j, err := openTestJournal(t, dir, a)
			if err != nil {
				t.Fatal(err)
			}
			j.Close()
			path := filepath.Join(dir, "journal.jsonl")
			switch kind {
			case "tail":
				f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
				f.WriteString(`{"sequence":1`)
				f.Close()
			case "symlink":
				if os.Rename(path, path+".kept") != nil || os.Symlink(path+".kept", path) != nil {
					t.Fatal("fixture setup failed")
				}
			case "hardlink":
				if os.Link(path, path+".alias") != nil {
					t.Fatal("fixture setup failed")
				}
			case "mode":
				os.Chmod(path, 0644)
			case "directory":
				os.Chmod(dir, 0755)
			}
			if j, err := openTestJournal(t, dir, a); err == nil {
				j.Close()
				t.Fatal("unsafe journal accepted")
			}
		})
	}
}

func TestRealJournalCreateFailureBlocksRetryAndContainsNoErrorBody(t *testing.T) {
	dir := privateDir(t)
	a := approval()
	j, err := openTestJournal(t, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeAPI{createErr: ErrRemote}
	d := Driver{a, j, f}
	// Use a valid synthetic inventory digest with the real journal schema.
	fake := &inventoryAPI{fakeAPI: f}
	d.API = fake
	if d.Run(context.Background(), "create") == nil {
		t.Fatal("remote failure accepted")
	}
	j.Close()
	j, err = openTestJournal(t, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	d.Journal = j
	if d.Run(context.Background(), "create") == nil || f.createCalls != 1 {
		t.Fatal("durable ambiguity fence lost")
	}
}

type inventoryAPI struct{ *fakeAPI }

func TestStrictJSONRejectsDuplicateAuthorityFields(t *testing.T) {
	var a Approval
	if DecodeStrict([]byte(`{"app_id":1,"app_id":2}`), &a) == nil {
		t.Fatal("duplicate authority field accepted")
	}
	if DecodeStrict([]byte(`{"app_id":1} {"app_id":2}`), &a) == nil {
		t.Fatal("trailing authority accepted")
	}
}

func (*inventoryAPI) Inventory(context.Context) (string, error) { return strings.Repeat("0", 64), nil }

func testAdmissionDirectory(t *testing.T, directory string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(directory), "admission")
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		t.Fatal("fixture admission directory")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal("fixture admission identity")
	}
	return canonical
}
func openTestJournal(t *testing.T, directory string, a Approval) (*FileJournal, error) {
	t.Helper()
	return openJournalAtAdmission(directory, a, testAdmissionDirectory(t, directory), func(f *os.File) error { return f.Sync() })
}
