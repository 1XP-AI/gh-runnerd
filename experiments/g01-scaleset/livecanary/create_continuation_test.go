package livecanary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// This helper exercises receipt verification, strict credential admission and
// the real writer; its consumption record is never fabricated.
func completedHandoffCreate(t *testing.T, j *FileJournal, a Approval) (controllerHandoffFixture, *fakeAPI) {
	t.Helper()
	a.WorkflowRef = "refs/heads/main"
	f := controllerHandoffFixtureForApproval(t, a, "create", time.Now().UTC().Truncate(time.Second))
	// Equivalent JSON with different raw bytes must not be compared with the
	// canonical typed approval digest by a historical parser.
	f.snapshot = append([]byte(" \n"), f.snapshot...)
	var err error
	_, f.request, err = controllerHandoffRequest(f.snapshot, "create", f.request.Source, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.receipt.ControllerApprovalSHA256 = f.request.ControllerApprovalSHA256
	f.receiptBytes = f.encodeReceipt(t, f.receipt, true)
	if f.request.ControllerApprovalSHA256 == approvalDigest(a) {
		t.Fatal("fixture failed to distinguish raw approval bytes")
	}

	fake := &fakeAPI{}
	reads, factories, syncs := 0, 0, 0
	j.syncFile = func(file *os.File) error { syncs++; return file.Sync() }
	err = runControllerHandoffCreate(context.Background(), j, f.snapshot, f.receiptBytes, f.request.Source, f.root, time.Now,
		func(context.Context) ([]byte, error) {
			reads++
			if syncs != 1 {
				t.Fatal("credential read preceded durable consumption")
			}
			return handoffCredentialBytes(t, a), nil
		},
		func(context.Context, Approval, Credentials) (API, error) {
			factories++
			return completedCreateAPI{fake}, nil
		})
	if err != nil || reads != 1 || factories != 1 || fake.createCalls != 1 || syncs != 7 || len(j.Events()) != 7 {
		t.Fatalf("receipt create: err=%v reads=%d factories=%d creates=%d syncs=%d events=%d", err, reads, factories, fake.createCalls, syncs, len(j.Events()))
	}
	return f, fake
}

func TestCreateContinuationReceiptBackedPairedPreparation(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	directory := privateDir(t)
	j, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	completedHandoffCreate(t, j, a)
	before, err := os.ReadFile(j.file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	receipt, err := preparePairedJournal(directory, a, func(dir string, a Approval) (*FileJournal, error) { return openTestJournal(t, dir, a) })
	if err != nil {
		t.Fatalf("completed receipt-backed create refused paired preparation: %v", err)
	}
	after, err := os.ReadFile(directory + "/journal.jsonl")
	if err != nil || !bytes.Equal(before, after) || receipt.Status != "controller_journal_prepared" || receipt.Phase != pairedPreparationPhase {
		t.Fatal("preparation rewrote history or omitted receipt")
	}
}

// Match the paired fixture's complete empty roster, using the real create writer.
type completedCreateAPI struct{ *fakeAPI }

func (completedCreateAPI) Inventory(context.Context) (string, error) {
	sum := sha256.Sum256([]byte("null"))
	return hex.EncodeToString(sum[:]), nil
}

func TestCreateContinuationReceiptBackedBaselineAndReopen(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	var receipt controllerHandoffFixture
	var fake *fakeAPI
	f := newBaselineFixtureWithCreate(t, nil, "", a, func(j *FileJournal) { receipt, fake = completedHandoffCreate(t, j, a) })
	id, err := f.j.controllerIdentity()
	if err != nil {
		t.Fatal(err)
	}
	original := f.j.Events()
	inventory, creation := controllerEventRef(id, original[2]), controllerEventRef(id, original[6])
	if inventory.Sequence != 3 || creation.Sequence != 7 {
		t.Fatal("lost physical references")
	}
	if err = f.run(t); err != nil {
		t.Fatal(err)
	}
	if f.acks.Load() != 2 || f.acquires.Load() != 1 || f.continuations.Load() != 1 || fake.createCalls != 1 || f.forbidden.Load() != 0 {
		t.Fatal("singleton continuation violated")
	}
	for _, e := range f.j.Events() {
		if e.Baseline != nil && e.Baseline.Creation != creation {
			t.Fatal("creation reference changed")
		}
	}
	dir := f.j.directory
	f.release()
	f.release = nil
	if err = f.j.Close(); err != nil {
		t.Fatal(err)
	}
	f.j, err = openTestJournal(t, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	afterID, err := f.j.controllerIdentity()
	if err != nil || afterID != id {
		t.Fatal("reopen changed journal identity")
	}
	h, err := replayBaseline(f.j.Events(), id, a)
	if err != nil || !h.complete || !h.continued || h.creation != creation {
		t.Fatalf("replay lost continuation: %v", err)
	}
	if controllerEventRef(id, f.j.Events()[2]) != inventory || controllerEventRef(id, f.j.Events()[6]) != creation {
		t.Fatal("reopen changed original refs")
	}
	for _, fresh := range []bool{false, true} {
		b := receipt.receiptBytes
		if fresh {
			r := receipt.receipt
			r.ReceiptNonce = strings.Repeat("e", 32)
			b = receipt.encodeReceipt(t, r, true)
		}
		if runControllerHandoffCreate(context.Background(), f.j, receipt.snapshot, b, receipt.request.Source, receipt.root, time.Now, func(context.Context) ([]byte, error) { t.Fatal("replayed create read credentials"); return nil, nil }, func(context.Context, Approval, Credentials) (API, error) {
			t.Fatal("replayed create built API")
			return nil, nil
		}) == nil {
			t.Fatal("completed create resumed")
		}
	}
	f.release, err = f.j.authorize(a)
	if err != nil {
		t.Fatal(err)
	}
	before := f.requests.Load()
	if f.run(t) == nil || f.requests.Load() != before || fake.createCalls != 1 {
		t.Fatal("reopen repeated protocol effects")
	}
}

func cloneCreateEvents(events []Event) []Event {
	out := make([]Event, len(events))
	for i, e := range events {
		out[i] = cloneEvent(e)
	}
	return out
}

func TestCreateContinuationRejectsNoncanonicalPrelude(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	f := newBaselineFixtureWithCreate(t, nil, "", a, func(j *FileJournal) { completedHandoffCreate(t, j, a) })
	original := f.j.Events()
	id, err := f.j.controllerIdentity()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func([]Event) []Event
	}{
		{"metadata nonce", func(e []Event) []Event { e[0].ControllerHandoff.ReceiptNonce = "bad"; return e }},
		{"metadata hash", func(e []Event) []Event { e[0].ControllerHandoff.ReceiptSHA256 = strings.Repeat("A", 64); return e }},
		{"metadata key", func(e []Event) []Event { e[0].ControllerHandoff.KeyID = ""; return e }},
		{"metadata extra field", func(e []Event) []Event { e[0].ID = 7; return e }},
		{"noncreate handoff", func(e []Event) []Event { e[0].ControllerHandoff.Phase = "inspect"; return e }},
		{"duplicate handoff", func(e []Event) []Event { return append(e, cloneEvent(e[0])) }},
		{"distinct second handoff", func(e []Event) []Event {
			x := cloneEvent(e[0])
			x.ControllerHandoff.ReceiptNonce = strings.Repeat("e", 32)
			return append(e, x)
		}},
		{"handoff after phase", func(e []Event) []Event { e[0], e[1] = e[1], e[0]; return e }},
		{"wrong order", func(e []Event) []Event { e[2], e[3] = e[3], e[2]; return e }},
		{"uppercase inventory", func(e []Event) []Event { e[2].Digest = strings.Repeat("A", 64); return e }},
		{"empty inventory", func(e []Event) []Event { e[2].Digest = ""; return e }},
		{"zero create id", func(e []Event) []Event { e[6].ID = 0; return e }},
		{"work discovery", func(e []Event) []Event { e[4].Work = workDemand; return e }},
		{"work create", func(e []Event) []Event { e[6].Work = workUnresolved; return e }},
		{"unrelated record", func(e []Event) []Event { return append(e, Event{Kind: "observation", Operation: "inspect"}) }},
	}
	for step := 1; step <= 6; step++ {
		at := step
		cases = append(cases, struct {
			name   string
			change func([]Event) []Event
		}{"missing step " + string(rune('0'+at)), func(e []Event) []Event { return append(e[:at], e[at+1:]...) }})
		cases = append(cases, struct {
			name   string
			change func([]Event) []Event
		}{"duplicate step " + string(rune('0'+at)), func(e []Event) []Event { return append(e, cloneEvent(e[at])) }})
		cases = append(cases, struct {
			name   string
			change func([]Event) []Event
		}{"extra payload step " + string(rune('0'+at)), func(e []Event) []Event { e[at].SessionID = "unexpected"; return e }})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := tc.change(cloneCreateEvents(original))
			for i := range events {
				events[i].Sequence = i + 1
			}
			if _, err := parseCompletedCreateHistory(events); err == nil {
				t.Fatal("noncanonical history parsed")
			}
			f.j.events = events
			before := f.requests.Load()
			if _, err := newBaselineListenerHeld(context.Background(), a, f.j, f.api, 7); err == nil || f.requests.Load() != before {
				t.Fatal("invalid prelude constructed listener or contacted API")
			}
			// Rebind the create reference to the mutated physical record, so rejection
			// proves the complete prelude grammar rather than merely a stale hash.
			var created Event
			for _, e := range events {
				if e.Kind == "result" && e.Operation == "create" {
					created = e
				}
			}
			first := Event{Sequence: len(events) + 1, Kind: "baseline", Baseline: &baselineRecord{Version: 1, Stage: "set-observe", Outcome: "intent", SetID: 7, Creation: controllerEventRef(id, created)}}
			if _, err := replayBaseline(append(events, first), id, a); err == nil {
				t.Fatal("baseline replay accepted noncanonical prelude")
			}
		})
	}
	f.j.events = original
	// Assigned positions themselves are part of the grammar.
	bad := cloneCreateEvents(original)
	bad[2].Sequence = 2
	if _, err := parseCompletedCreateHistory(bad); err == nil {
		t.Fatal("physical sequence mismatch accepted")
	}
	// Unknown JSON fields are refused before the typed structural parser.
	raw, _ := json.Marshal(original[0])
	raw = bytes.Replace(raw, []byte(`"key_id":`), []byte(`"unknown":1,"key_id":`), 1)
	var event Event
	if DecodeStrict(raw, &event) == nil {
		t.Fatal("unknown metadata field accepted")
	}
}

func TestCreateContinuationRenewalCannotRestoreAuthority(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	directory := privateDir(t)
	j, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	completedHandoffCreate(t, j, a)
	old := j.Events()
	id, err := j.controllerIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	renewed := a
	renewed.ExpiresAt = renewed.ExpiresAt.Add(time.Minute)
	renewed.Phases = []string{"inspect", "cleanup"}
	j, err = openTestJournal(t, directory, renewed)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	h, err := parseCompletedCreateHistory(j.Events())
	if err != nil || h.creation.Sequence != 7 || h.inventory.Sequence != 3 {
		t.Fatal("structural renewal lost completed history")
	}
	if len(j.Events()) != 8 || controllerEventRef(id, old[6]) != controllerEventRef(id, h.creation) {
		t.Fatal("renewal rewrote create evidence")
	}
	// An authority event is metadata, never a new paired/create permission.
	if _, err = preparePairedJournal(directory, renewed, func(dir string, a Approval) (*FileJournal, error) { return openTestJournal(t, dir, a) }); err == nil {
		t.Fatal("recovery-only renewal restored pairing")
	}
	if _, err = j.authorize(a); err == nil {
		t.Fatal("stale authority remained usable")
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	expanded := renewed
	expanded.ExpiresAt = expanded.ExpiresAt.Add(time.Minute)
	expanded.Phases = a.Phases
	if next, err := openTestJournal(t, directory, expanded); err == nil {
		next.Close()
		t.Fatal("renewal expanded back to paired/create authority")
	}
	if next, err := openTestJournal(t, directory, a); err == nil {
		next.Close()
		t.Fatal("stale authority reopened")
	}
	for _, fault := range []string{"approval", "foreign admission"} {
		t.Run(fault, func(t *testing.T) {
			incoming := renewed
			root := testAdmissionDirectory(t, directory)
			if fault == "approval" {
				incoming.Controller = "foreign-controller"
			} else {
				foreignDir := privateDir(t)
				foreign, err := openTestJournal(t, foreignDir, renewed)
				if err != nil {
					t.Fatal(err)
				}
				root = foreign.claim.directory
				if err = foreign.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if next, err := openJournalAtAdmission(directory, incoming, root, func(f *os.File) error { return f.Sync() }); err == nil {
				next.Close()
				t.Fatal("foreign identity admitted")
			}
		})
	}
}

func TestCreateContinuationStoredHashesNeverAuthorizeCreate(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	f := newBaselineFixtureWithCreate(t, nil, "", a, func(j *FileJournal) { completedHandoffCreate(t, j, a) })
	// Even well-shaped replacement hashes remain merely structural evidence.
	events := f.j.Events()
	events[0].ControllerHandoff.ApprovalSHA256 = strings.Repeat("0", 64)
	events[0].ControllerHandoff.ReceiptSHA256 = strings.Repeat("1", 64)
	if _, err := parseCompletedCreateHistory(events); err != nil {
		t.Fatal("parser invented historical cryptographic verification")
	}
	f.release()
	f.release = nil
	f.j.events = events
	fake := &fakeAPI{}
	d := Driver{a, f.j, completedCreateAPI{fake}}
	if d.Run(context.Background(), "create") == nil || fake.createCalls != 0 {
		t.Fatal("stored metadata became create proof")
	}
	bad := controllerHandoffFixtureForApproval(t, a, "create", time.Now().UTC().Truncate(time.Second))
	if runControllerHandoffCreate(context.Background(), f.j, bad.snapshot, bad.receiptBytes, bad.request.Source, bad.root, time.Now, func(context.Context) ([]byte, error) {
		t.Fatal("stored hashes authorized credential read")
		return nil, nil
	}, func(context.Context, Approval, Credentials) (API, error) {
		t.Fatal("stored hashes authorized factory")
		return nil, nil
	}) == nil {
		t.Fatal("fresh receipt resumed occupied create")
	}
}

func TestCreateContinuationConsumptionOnlyCannotContinue(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	f := controllerHandoffFixtureForApproval(t, a, "create", time.Now().UTC().Truncate(time.Second))
	j, err := openTestJournal(t, privateDir(t), a)
	if err != nil {
		t.Fatal(err)
	}
	if runControllerHandoffCreate(context.Background(), j, f.snapshot, f.receiptBytes, f.request.Source, f.root, time.Now, func(context.Context) ([]byte, error) { return []byte("invalid"), nil }, func(context.Context, Approval, Credentials) (API, error) {
		t.Fatal("invalid credentials built API")
		return nil, nil
	}) == nil {
		t.Fatal("invalid credential create succeeded")
	}
	if len(j.Events()) != 1 {
		t.Fatal("failed create lost consumption")
	}
	if _, err := parseCompletedCreateHistory(j.Events()); err == nil {
		t.Fatal("consumption alone became completed create")
	}
	directory := j.directory
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = openTestJournal(t, directory, f.approval)
	if err != nil {
		t.Fatal("ordinary partial reopen refused", err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = preparePairedJournal(directory, a, func(dir string, a Approval) (*FileJournal, error) { return openTestJournal(t, dir, a) }); err == nil {
		t.Fatal("consumption-only history granted paired preparation")
	}
}

func TestCreateContinuationReplayRejectsStoredPreludeMutations(t *testing.T) {
	for _, fault := range []string{"phase payload", "inventory payload", "discovery work", "handoff phase"} {
		t.Run(fault, func(t *testing.T) {
			a := approval()
			a.WorkflowRef = "refs/heads/main"
			f := newBaselineFixtureWithCreate(t, nil, "", a, func(j *FileJournal) { completedHandoffCreate(t, j, a) })
			if err := f.run(t); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(f.j.file.Name())
			if err != nil {
				t.Fatal(err)
			}
			lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
			index := 2 // header occupies line zero; mutate the phase, leaving create refs intact.
			if fault == "inventory payload" {
				index = 3
			}
			if fault == "discovery work" {
				index = 5
			}
			if fault == "handoff phase" {
				index = 1
			}
			var event Event
			if DecodeStrict(lines[index], &event) != nil {
				t.Fatal("fixture event")
			}
			switch fault {
			case "phase payload":
				event.ID = 7
			case "inventory payload":
				event.Operation = "create"
			case "discovery work":
				event.Work = workDemand
			case "handoff phase":
				event.ControllerHandoff.Phase = "inspect"
			}
			if !validEvent(event) {
				t.Fatal("mutation should pass generic event shape; baseline grammar must reject it")
			}
			lines[index], err = json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			dir, path := f.j.directory, f.j.file.Name()
			f.release()
			f.release = nil
			if err = f.j.Close(); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, append(bytes.Join(lines, []byte{'\n'}), '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			if j, err := openTestJournal(t, dir, a); err == nil {
				j.Close()
				t.Fatal("stored invalid prelude reopened despite unchanged create reference")
			}
		})
	}
}

func TestCreateContinuationDoesNotRequireCompleteOrdinaryReopen(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	directory := privateDir(t)
	j, err := openTestJournal(t, directory, a)
	if err != nil {
		t.Fatal(err)
	}
	completedHandoffCreate(t, j, a)
	path := j.file.Name()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	// Every prefix, including empty and consumed-but-incomplete create, remains
	// reopenable for ordinary inspection/recovery under the original authority.
	for n := 0; n < 7; n++ {
		if err = os.WriteFile(path, append(bytes.Join(lines[:n+1], []byte{'\n'}), '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		j, err = openTestJournal(t, directory, a)
		if err != nil {
			t.Fatalf("ordinary prefix %d reopen: %v", n, err)
		}
		if len(j.Events()) != n {
			t.Fatal("ordinary prefix changed")
		}
		if err = j.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateContinuationFirstBaselinePhysicalSequence(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	f := newBaselineFixtureWithCreate(t, nil, "", a, func(j *FileJournal) { completedHandoffCreate(t, j, a) })
	events := f.j.Events()
	id, err := f.j.controllerIdentity()
	if err != nil {
		t.Fatal(err)
	}
	creation := controllerEventRef(id, events[6])
	for _, sequence := range []int{0, 1, 7, 8, 9} {
		first := Event{Sequence: sequence, Kind: "baseline", Baseline: &baselineRecord{Version: 1, Stage: "set-observe", Outcome: "intent", SetID: 7, Creation: creation}}
		_, err := replayBaseline(append(cloneCreateEvents(events), first), id, a)
		if (err == nil) != (sequence == 8) {
			t.Errorf("first baseline sequence=%d accepted=%v; only physical sequence 8 is valid", sequence, err == nil)
		}
	}
}
