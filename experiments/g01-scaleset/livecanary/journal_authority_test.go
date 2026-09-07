package livecanary

import (
	"context"
	"testing"
	"time"
)

func TestFileJournalRejectsUnrecordedAuthorityBeforeRawDriverEffect(t *testing.T) {
	a := approval()
	a.Phases = []string{"create"}
	j, err := OpenJournal(privateDir(t), a)
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
	j, err := OpenJournal(dir, a)
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
	j, err = OpenJournal(dir, renewed)
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
		if unsafe, err := OpenJournal(dir, bad); err == nil {
			unsafe.Close()
			t.Errorf("%s authority accepted", kind)
		}
	}
}
