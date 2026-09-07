package livecanary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent(t *testing.T) {
	parent := privateDir(t)
	directory := admissionState(t, parent, "state")
	capRoot := testAdmissionDirectory(t, directory)
	a := approval()
	open := func(path string, a Approval) (*FileJournal, error) {
		return openJournalAtAdmission(path, a, capRoot, func(f *os.File) error { return f.Sync() })
	}
	receipt, e := prepareJournal(directory, a, "create", open)
	if e != nil {
		t.Fatal("fresh local preparation")
	}

	journalBytes, _ := os.ReadFile(filepath.Join(directory, "journal.jsonl"))
	claimBytes, _ := os.ReadFile(filepath.Join(capRoot, "admission.json"))
	jd := sha256.Sum256(journalBytes)
	cd := sha256.Sum256(claimBytes)
	si, _ := os.Stat(directory)
	ji, _ := os.Stat(filepath.Join(directory, "journal.jsonl"))
	ci, _ := os.Stat(filepath.Join(capRoot, "admission.json"))
	if receipt.Version != 1 || receipt.Status != "controller_journal_prepared" || receipt.Phase != "create" || receipt.ApprovalDigest != approvalDigest(a) || receipt.JournalDigest != hex.EncodeToString(jd[:]) || receipt.ClaimDigest != hex.EncodeToString(cd[:]) || receipt.State != preparedIdentity(si) || receipt.Journal != preparedIdentity(ji) || receipt.Claim != preparedIdentity(ci) {
		t.Fatal("receipt does not describe actual files after close")
	}
	j, e := open(directory, a)
	if e != nil {
		t.Fatal("prepared journal reopen")
	}
	defer j.Close()
	if len(j.Events()) != 0 {
		t.Fatal("preparation recorded phase or remote intent")
	}
}
func TestCanonicalPreparationRefusesInvalidJournalAndPhase(t *testing.T) {
	for _, kind := range []string{"locked", "malformed", "oversized", "permission", "authority", "seen phase", "unknown", "deleted", "unapproved"} {
		t.Run(kind, func(t *testing.T) {
			parent := privateDir(t)
			directory := admissionState(t, parent, "state")
			capRoot := testAdmissionDirectory(t, directory)
			a := approval()
			open := func(path string, a Approval) (*FileJournal, error) {
				return openJournalAtAdmission(path, a, capRoot, func(f *os.File) error { return f.Sync() })
			}
			j, e := open(directory, a)
			if e != nil {
				t.Fatal("canonical fixture")
			}
			switch kind {
			case "seen phase":
				j.Append(Event{Kind: "phase", Operation: "create"})
			case "unknown":
				j.Append(Event{Kind: "intent", Operation: "create"})
			case "deleted":
				j.Append(Event{Kind: "intent", Operation: "delete"})
				j.Append(Event{Kind: "result", Operation: "delete"})
			}
			j.Close()
			path := filepath.Join(directory, "journal.jsonl")
			switch kind {
			case "locked":
				f, e := os.OpenFile(path, os.O_RDWR, 0)
				if e != nil {
					t.Fatal(e)
				}
				defer f.Close()
				if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
					t.Fatal("fixture lease")
				}
			case "malformed":
				f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				f.WriteString("{invalid-event}\n")
				f.Close()
			case "oversized":
				os.WriteFile(path, []byte(strings.Repeat("x", (1<<20)+1)), 0600)
			case "permission":
				os.Chmod(path, 0644)
			case "authority":
				a.WorkflowSHA = strings.Repeat("a", 40)
			}
			phase := "create"
			if kind == "unapproved" {
				phase = "invalid"
			}
			before, _ := os.ReadFile(path)
			if _, e := prepareJournal(directory, a, phase, open); e == nil {
				t.Fatal("ineligible local state prepared")
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("refused preparation appended a phase/remote intent")
			}
		})
	}
}
func TestCanonicalPreparationRecoveryAndDriverShareLocalGate(t *testing.T) {
	parent := privateDir(t)
	directory := admissionState(t, parent, "state")
	capRoot := testAdmissionDirectory(t, directory)
	a := approval()
	open := func(path string, a Approval) (*FileJournal, error) {
		return openJournalAtAdmission(path, a, capRoot, func(f *os.File) error { return f.Sync() })
	}
	j, e := open(directory, a)
	if e != nil {
		t.Fatal(e)
	}
	j.Append(Event{Kind: "phase", Operation: "create"})
	j.Append(Event{Kind: "intent", Operation: "create"})
	j.Close()
	renewed := a
	renewed.ExpiresAt = a.ExpiresAt.Add(time.Minute)
	renewed.Phases = []string{"inspect", "cleanup"}
	if _, e := prepareJournal(directory, renewed, "inspect", open); e != nil {
		t.Fatal("renewed inspection refused")
	}
	if _, e := prepareJournal(directory, renewed, "cleanup", open); e == nil {
		t.Fatal("uncertainty released by local preparation")
	}
	j, e = open(directory, renewed)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	if !replay(j.Events()).uncertain || len(j.Events()) != 3 {
		t.Fatal("preparation reset evidence or recorded phase")
	}
	// This phase must return through the shared local gate before touching API.
	d := Driver{Approval: renewed, Journal: j, API: nil}
	if d.Run(context.Background(), "cleanup") == nil {
		t.Fatal("Driver bypassed canonical preparation gate")
	}
}
