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

func TestPairedPreparationUsesDedicatedPhaseWithoutCleanupAuthority(t *testing.T) {
	parent := privateDir(t)
	directory := admissionState(t, parent, "paired-state")
	capRoot := testAdmissionDirectory(t, directory)
	a := approval()
	open := func(path string, a Approval) (*FileJournal, error) {
		return openJournalAtAdmission(path, a, capRoot, func(f *os.File) error { return f.Sync() })
	}
	j, err := open(directory, a)
	if err != nil {
		t.Fatal("canonical controller journal")
	}
	for _, event := range []Event{
		{Kind: "phase", Operation: "create"},
		{Kind: "inventory", Digest: strings.Repeat("a", 64)},
		{Kind: "intent", Operation: "observe-discovery"},
		{Kind: "result", Operation: "observe-discovery"},
		{Kind: "intent", Operation: "create"},
		{Kind: "result", Operation: "create", ID: 7},
	} {
		if err := j.Append(event); err != nil {
			t.Fatalf("canonical prerequisite event %q: %v", event.Operation, err)
		}
	}
	if err := j.Close(); err != nil {
		t.Fatal("close canonical controller journal")
	}
	receipt, err := preparePairedJournal(directory, a, open)
	if err != nil || receipt.Phase != pairedPreparationPhase || receipt.Status != "controller_journal_prepared" {
		t.Fatalf("paired preparation did not produce its own receipt: receipt=%+v err=%v", receipt, err)
	}
	j, err = open(directory, a)
	if err != nil {
		t.Fatal("paired journal reopen")
	}
	defer j.Close()
	if len(j.Events()) != 6 {
		t.Fatal("paired preparation borrowed cleanup authority or recorded an effect")
	}
	withoutVerification := a
	withoutVerification.Phases = []string{"create", "inspect", "cleanup"}
	withoutDirectory := admissionState(t, parent, "without-verification")
	if _, err := preparePairedJournal(withoutDirectory, withoutVerification, open); err == nil {
		t.Fatal("paired preparation accepted missing verification authority")
	}
}

func TestPairedPreparationPreservesCanonicalControllerHistory(t *testing.T) {
	parent := privateDir(t)
	directory := admissionState(t, parent, "paired-history")
	capRoot := testAdmissionDirectory(t, directory)
	a := approval()
	open := func(path string, a Approval) (*FileJournal, error) {
		return openJournalAtAdmission(path, a, capRoot, func(f *os.File) error { return f.Sync() })
	}
	j, err := open(directory, a)
	if err != nil {
		t.Fatal("canonical controller journal")
	}
	for _, event := range []Event{
		{Kind: "phase", Operation: "create"},
		{Kind: "inventory", Digest: strings.Repeat("a", 64)},
		{Kind: "intent", Operation: "observe-discovery"},
		{Kind: "result", Operation: "observe-discovery"},
		{Kind: "intent", Operation: "create"},
		{Kind: "result", Operation: "create", ID: 7},
	} {
		if err := j.Append(event); err != nil {
			t.Fatalf("canonical prerequisite event %q: %v", event.Operation, err)
		}
	}
	if err := j.Close(); err != nil {
		t.Fatal("close canonical controller journal")
	}
	before, err := os.ReadFile(filepath.Join(directory, "journal.jsonl"))
	if err != nil {
		t.Fatal("read prerequisite journal")
	}
	receipt, err := preparePairedJournal(directory, a, open)
	if err != nil || receipt.Phase != pairedPreparationPhase {
		t.Fatalf("valid canonical controller history refused: receipt=%+v err=%v", receipt, err)
	}
	after, err := os.ReadFile(filepath.Join(directory, "journal.jsonl"))
	if err != nil {
		t.Fatal("read prepared journal")
	}
	if string(before) != string(after) {
		t.Fatal("paired preparation changed prerequisite controller history")
	}
	reopened, err := open(directory, a)
	if err != nil {
		t.Fatal("reopen prepared controller journal")
	}
	defer reopened.Close()
	if events := reopened.Events(); len(events) != 6 || events[1].Kind != "inventory" || events[5].Operation != "create" || events[5].ID != 7 {
		t.Fatalf("paired preparation did not preserve canonical events: %+v", events)
	}
}

func TestPairedPreparationRejectsFreshAndNonCanonicalHistory(t *testing.T) {
	for _, kind := range []string{"fresh", "pending", "deleted", "previous-paired"} {
		t.Run(kind, func(t *testing.T) {
			parent := privateDir(t)
			directory := admissionState(t, parent, "paired-"+kind)
			capRoot := testAdmissionDirectory(t, directory)
			a := approval()
			open := func(path string, a Approval) (*FileJournal, error) {
				return openJournalAtAdmission(path, a, capRoot, func(f *os.File) error { return f.Sync() })
			}
			j, err := open(directory, a)
			if err != nil {
				t.Fatal("fixture journal")
			}
			switch kind {
			case "pending":
				_ = j.Append(Event{Kind: "phase", Operation: "create"})
				_ = j.Append(Event{Kind: "inventory", Digest: strings.Repeat("a", 64)})
				_ = j.Append(Event{Kind: "intent", Operation: "create"})
			case "deleted":
				_ = j.Append(Event{Kind: "phase", Operation: "create"})
				_ = j.Append(Event{Kind: "inventory", Digest: strings.Repeat("a", 64)})
				_ = j.Append(Event{Kind: "intent", Operation: "create"})
				_ = j.Append(Event{Kind: "result", Operation: "create", ID: 7})
				_ = j.Append(Event{Kind: "intent", Operation: "delete"})
				_ = j.Append(Event{Kind: "result", Operation: "delete"})
			case "previous-paired":
				// Baseline records are rejected by replayBaseline before preparation
				// can issue a receipt, and must never be treated as controller setup.
				_ = j.Append(Event{Kind: "phase", Operation: "create"})
				_ = j.Append(Event{Kind: "inventory", Digest: strings.Repeat("a", 64)})
				_ = j.Append(Event{Kind: "intent", Operation: "create"})
				_ = j.Append(Event{Kind: "result", Operation: "create", ID: 7})
				_ = j.Append(Event{Kind: "baseline", Baseline: &baselineRecord{Version: 1, Stage: "pair", Outcome: "intent", SetID: 7}})
			}
			if err := j.Close(); err != nil {
				t.Fatal("close fixture journal")
			}
			if _, err := preparePairedJournal(directory, a, open); err == nil {
				t.Fatal("non-canonical paired preparation state accepted")
			}
		})
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
