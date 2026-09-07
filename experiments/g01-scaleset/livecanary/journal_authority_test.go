package livecanary

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileJournalRejectsUnrecordedAuthorityBeforeRawDriverEffect(t *testing.T) {
	a := approval()
	a.Phases = []string{"create"}
	j, err := openTestJournal(t, privateDir(t), a)
	if err != nil {
		t.Fatal("private fixture journal")
	}
	defer j.Close()
	f := &fakeAPI{}
	d := Driver{a, j, &inventoryAPI{f}}
	if d.Run(context.Background(), "create") != nil {
		t.Fatal("fixture creation")
	}
	d.Approval.Phases = []string{"jit-loss"}
	d.Approval.ExpiresAt = a.ExpiresAt.Add(time.Hour)
	if err := d.Run(context.Background(), "jit-loss"); err == nil || f.jitCalls != 0 {
		t.Fatal("mutable Driver approval bypassed recorded authority before JIT")
	}
}

func TestRenewedRecoveryApprovalRetainsOwnedState(t *testing.T) {
	a := approval()
	a.ExpiresAt = time.Now().Add(time.Minute)
	dir := privateDir(t)
	j, err := openTestJournal(t, dir, a)
	if err != nil {
		t.Fatal("initial private journal")
	}
	if j.Append(Event{Kind: "intent", Operation: "create"}) != nil || j.Append(Event{Kind: "result", Operation: "create", ID: 7}) != nil {
		t.Fatal("synthetic ownership receipt")
	}
	_ = j.Close()
	renewed := a
	renewed.ExpiresAt = a.ExpiresAt.Add(time.Hour)
	renewed.Phases = []string{"inspect", "cleanup"}
	if a.Validate(a.ExpiresAt.Add(time.Nanosecond)) == nil || renewed.Validate(a.ExpiresAt.Add(time.Nanosecond)) != nil {
		t.Fatal("fixture expired/renewed authority mismatch")
	}
	j, err = openTestJournal(t, dir, renewed)
	if err != nil {
		t.Fatalf("explicit recovery renewal cannot reopen owned state: %v", err)
	}
	authorities, creates := 0, 0
	for _, e := range j.Events() {
		if e.Kind == "authority" {
			authorities++
		}
		if e.Operation == "create" {
			creates++
		}
	}
	if authorities != 1 || creates != 2 {
		t.Error("renewal did not durably retain ownership and record its authority")
	}
	_ = j.Close()
	for _, kind := range []string{"rollback", "new work", "changed owner"} {
		bad := renewed
		switch kind {
		case "rollback":
			bad = a
		case "new work":
			bad.ExpiresAt = renewed.ExpiresAt.Add(time.Hour)
			bad.Phases = []string{"inspect", "create"}
		case "changed owner":
			bad.OwnerNonce = "99999999999999999999999999999999"
		}
		if unsafe, err := openTestJournal(t, dir, bad); err == nil {
			unsafe.Close()
			t.Errorf("%s authority accepted", kind)
		}
	}
}

func TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose(t *testing.T) {
	a := approval()
	j, err := openTestJournal(t, privateDir(t), a)
	if err != nil {
		t.Fatal("private fixture journal")
	}
	first, err := j.authorize(a)
	if err != nil {
		t.Fatal("first lease")
	}
	type result struct {
		release func()
		err     error
	}
	second := make(chan result, 1)
	go func() { release, err := j.authorize(a); second <- result{release, err} }()
	select {
	case acquired := <-second:
		if acquired.err == nil {
			acquired.release()
			t.Error("same journal authorized concurrent phase execution")
		}
	case <-time.After(time.Second):
		first()
		t.Fatal("busy admission did not fail promptly")
	}
	closed := make(chan struct{})
	go func() { _ = j.Close(); close(closed) }()
	select {
	case <-closed:
		t.Error("close released ownership during a phase")
	case <-time.After(50 * time.Millisecond):
	}
	first()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close did not resume")
	}
	if release, err := j.authorize(a); err == nil {
		release()
		t.Fatal("closed journal authorized a phase")
	}
}

func TestAuthorityRejectsReplacedJournalOrDirectory(t *testing.T) {
	for _, kind := range []string{"journal", "directory"} {
		t.Run(kind, func(t *testing.T) {
			a := approval()
			dir := privateDir(t)
			j, err := openTestJournal(t, dir, a)
			if err != nil {
				t.Fatal("fixture journal")
			}
			defer j.Close()
			if kind == "journal" {
				path := filepath.Join(dir, "journal.jsonl")
				if os.Rename(path, path+".original") != nil || os.WriteFile(path, []byte("replacement"), 0600) != nil {
					t.Fatal("fixture replacement")
				}
			} else {
				if os.Rename(dir, dir+".original") != nil || os.Mkdir(dir, 0700) != nil {
					t.Fatal("fixture directory replacement")
				}
			}
			if release, err := j.authorize(a); err == nil {
				release()
				t.Fatal("replaced ownership inventory authorized a phase")
			}
		})
	}
}
