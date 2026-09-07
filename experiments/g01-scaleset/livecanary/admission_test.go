package livecanary

import (
	"context"
	"errors"
	"github.com/actions/scaleset"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

type auditSharedAPI struct {
	fakeAPI
	mu   sync.Mutex
	sets map[string]*scaleset.RunnerScaleSet
}

func admissionState(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if os.Mkdir(path, 0700) != nil {
		t.Fatal("private state fixture")
	}
	return path
}

func TestAdmissionRemainsPinnedAfterCloseAndDeletion(t *testing.T) {
	parent := privateDir(t)
	first := admissionState(t, parent, "first")
	capRoot := testAdmissionDirectory(t, first)
	a := approval()
	j, err := openJournalAtAdmission(first, a, capRoot, func(f *os.File) error { return f.Sync() })
	if err != nil {
		t.Fatal("first admission")
	}
	for _, e := range []Event{{Kind: "intent", Operation: "create"}, {Kind: "result", Operation: "create", ID: 7}, {Kind: "intent", Operation: "delete"}, {Kind: "result", Operation: "delete"}} {
		if j.Append(e) != nil {
			t.Fatal("synthetic owned create/delete receipts")
		}
	}
	_ = j.Close()
	second := admissionState(t, parent, "second")
	if other, err := openJournalAtAdmission(second, a, capRoot, func(f *os.File) error { return f.Sync() }); err == nil {
		other.Close()
		t.Error("closing/deleting the first experiment released its permanent admission")
	}
	reopened, err := openJournalAtAdmission(first, a, capRoot, func(f *os.File) error { return f.Sync() })
	if err != nil {
		t.Fatal("exact same owned inode could not reopen")
	}
	defer reopened.Close()
	if !replay(reopened.Events()).deleted {
		t.Fatal("reopen lost original terminal receipt")
	}
}

func TestAdmissionRejectsCopiedJournalInDifferentDirectory(t *testing.T) {
	parent := privateDir(t)
	first := admissionState(t, parent, "first")
	capRoot := testAdmissionDirectory(t, first)
	a := approval()
	j, err := openJournalAtAdmission(first, a, capRoot, func(f *os.File) error { return f.Sync() })
	if err != nil {
		t.Fatal("first admission")
	}
	j.Close()
	data, err := os.ReadFile(filepath.Join(first, "journal.jsonl"))
	if err != nil {
		t.Fatal("fixture receipt")
	}
	second := admissionState(t, parent, "second")
	if os.WriteFile(filepath.Join(second, "journal.jsonl"), data, 0600) != nil {
		t.Fatal("fixture copy")
	}
	if other, err := openJournalAtAdmission(second, a, capRoot, func(f *os.File) error { return f.Sync() }); err == nil {
		other.Close()
		t.Fatal("copied state bypassed pinned inode identity")
	}
}

func TestAdmissionSyncFailureMustBeRetried(t *testing.T) {
	parent := privateDir(t)
	state := admissionState(t, parent, "state")
	capRoot := testAdmissionDirectory(t, state)
	a := approval()
	info, err := os.Stat(capRoot)
	if err != nil {
		t.Fatal("fixture root")
	}
	failures := 0
	syncDirectory := func(f *os.File) error {
		current, err := f.Stat()
		if err != nil {
			return err
		}
		if os.SameFile(current, info) {
			failures++
			return errors.New("synthetic admission sync failure")
		}
		return f.Sync()
	}
	for attempt := 1; attempt <= 2; attempt++ {
		j, err := openJournalAtAdmission(state, a, capRoot, syncDirectory)
		if j != nil {
			j.Close()
		}
		if err == nil || failures != attempt {
			t.Errorf("admission durability failure bypassed: attempt=%d calls=%d error=%v", attempt, failures, err)
		}
	}
}

func TestAdmissionRefusesMissingUnsafeOrUnknownRootState(t *testing.T) {
	for _, fault := range []string{"missing", "shared", "symlink", "empty claim", "malformed claim", "linked claim"} {
		t.Run(fault, func(t *testing.T) {
			parent := privateDir(t)
			state := admissionState(t, parent, "state")
			capRoot := testAdmissionDirectory(t, state)
			switch fault {
			case "missing":
				capRoot = filepath.Join(parent, "absent")
			case "shared":
				if os.Chmod(capRoot, 0755) != nil {
					t.Fatal("fixture mode")
				}
			case "symlink":
				alias := filepath.Join(parent, "alias")
				if os.Symlink(capRoot, alias) != nil {
					t.Fatal("fixture alias")
				}
				capRoot = alias
			case "empty claim", "malformed claim", "linked claim":
				data := []byte{}
				if fault == "malformed claim" {
					data = []byte("{unknown")
				}
				path := filepath.Join(capRoot, "admission.json")
				if os.WriteFile(path, data, 0600) != nil {
					t.Fatal("fixture claim")
				}
				if fault == "linked claim" && os.Link(path, path+".alias") != nil {
					t.Fatal("fixture link")
				}
			}
			if j, err := openJournalAtAdmission(state, approval(), capRoot, func(f *os.File) error { return f.Sync() }); err == nil {
				j.Close()
				t.Fatal("unsafe or unknown fixed admission accepted")
			}
		})
	}
}

func (f *auditSharedAPI) Inventory(context.Context) (string, error) {
	return strings.Repeat("a", 64), nil
}
func (f *auditSharedAPI) FindScaleSet(_ context.Context, name string, _ int) (*scaleset.RunnerScaleSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sets[name], nil
}
func (f *auditSharedAPI) CreateScaleSet(_ context.Context, set *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := *set
	result.ID = len(f.sets) + 1
	result.Statistics = &scaleset.RunnerScaleSetStatistic{}
	f.sets[result.Name] = &result
	return &result, nil
}
func TestAuditPR25DistinctStateDirectoriesMustShareCap(t *testing.T) {
	f := &auditSharedAPI{sets: map[string]*scaleset.RunnerScaleSet{}}
	parent := privateDir(t)
	capRoot := testAdmissionDirectory(t, filepath.Join(parent, "unused-state"))
	for i, nonce := range []string{strings.Repeat("a", 32), strings.Repeat("b", 32)} {
		a := approval()
		a.OwnerNonce = nonce
		dir := filepath.Join(parent, nonce)
		if os.Mkdir(dir, 0700) != nil {
			t.Fatal("fixture directory")
		}
		j, err := openJournalAtAdmission(dir, a, capRoot, func(f *os.File) error { return f.Sync() })
		if err != nil {
			if i == 1 {
				return
			}
			t.Fatal("first fixture journal refused")
		}
		defer j.Close()
		d := Driver{a, j, f}
		err = d.Run(context.Background(), "create")
		if i == 0 && err != nil {
			t.Fatal("first fixture creation refused")
		}
	}
	if len(f.sets) > 1 {
		t.Fatalf("different simultaneously owned journals admitted %d scale sets under a cap of one", len(f.sets))
	}
}

// Holding the shared directory lock models a competing initializer before the
// permanent claim has been created. The loser must not create an empty claim.
func TestAdmissionInitializationLockPrecedesClaimCreation(t *testing.T) {
	parent := privateDir(t)
	state := filepath.Join(parent, "state")
	if os.Mkdir(state, 0700) != nil {
		t.Fatal("state fixture")
	}
	capRoot := testAdmissionDirectory(t, state)
	lock, err := os.Open(capRoot)
	if err != nil {
		t.Fatal("directory fixture")
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		t.Fatal("fixture directory lock")
	}
	a := approval()
	j, err := openJournalAtAdmission(state, a, capRoot, func(f *os.File) error { return f.Sync() })
	if j != nil {
		j.Close()
	}
	if err == nil {
		t.Error("competing initializer ignored the shared initialization lock")
	}
	if _, err := os.Lstat(filepath.Join(capRoot, "admission.json")); !os.IsNotExist(err) {
		t.Error("losing initializer created a permanent claim")
	}
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) != nil {
		t.Fatal("fixture unlock")
	}
	j, err = openJournalAtAdmission(state, a, capRoot, func(f *os.File) error { return f.Sync() })
	if err != nil {
		t.Fatal("admission did not recover after competing initializer left")
	}
	if j.Close() != nil {
		t.Fatal("fixture close")
	}
}
