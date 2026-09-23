package livecanary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"reflect"
	"slices"
	"syscall"
	"time"
)

// authorizePhase is the canonical local gate shared by preparation and Run.
// It returns a held execution lease. It never records a phase or contacts APIs.
func authorizePhase(a Approval, j Journal, phase string) (state, func(), error) {
	return authorizePhaseWithProof(a, j, phase, context.Background(), nil)
}

func authorizePhaseWithProof(a Approval, j Journal, phase string, ctx context.Context, proof *controllerHandoffProof) (state, func(), error) {
	if a.Validate(time.Now()) != nil || !slices.Contains(a.Phases, phase) {
		return state{}, nil, ErrApproval
	}
	if j == nil {
		return state{}, nil, ErrJournal
	}
	release, e := j.authorize(a)
	if e != nil {
		return state{}, nil, ErrJournal
	}
	events := j.Events()
	if err := validateCreatePrefix(a, j, phase, ctx, proof, false, events); err != nil {
		release()
		return state{}, nil, err
	}
	s := replayWithApproval(events, &a)
	// Every approved phase, including the read-only inspect slot, is consumed
	// by its durable phase record. A second inspect must not become a fresh
	// authority merely because its implementation has no external write. One
	// inspect remains available to examine an uncertain or durably deleted
	// state; its own phase record then prevents another attempt. Deletion still
	// fences every phase that could perform a further effect.
	invalid := (phase != "inspect" && s.uncertain) || s.phaseSeen[phase] || (s.deleted && phase != "inspect")
	// The create prefix has already been checked. A verified handoff is the
	// sole exact one-event exception to the empty legacy prefix.
	invalid = invalid || (phase == "create" && s.setID != 0)
	invalid = invalid || (phase != "create" && phase != "inspect" && (s.setID <= 0 || s.reserved))
	invalid = invalid || (phase == "cleanup" && (s.workObserved || len(s.observedJobs) != 0 || s.inventory == ""))
	if invalid {
		release()
		return state{}, nil, ErrQuarantine
	}
	return s, release, nil
}

// validateCreatePrefix is the exact create-event grammar shared by the
// pre-phase authorization check and Driver's post-phase check.
func validateCreatePrefix(a Approval, j Journal, phase string, ctx context.Context, proof *controllerHandoffProof, post bool, events []Event) error {
	if phase != "create" {
		if proof != nil {
			return ErrQuarantine
		}
		return nil
	}
	if proof == nil {
		if !post && len(events) == 0 {
			return nil
		}
		if post && len(events) == 1 && reflect.DeepEqual(events[0], Event{Sequence: 1, Kind: "phase", Operation: "create"}) {
			return nil
		}
		return ErrQuarantine
	}
	if !proof.validAt(ctx, a, j, time.Now()) {
		return ErrQuarantine
	}
	if !post {
		if len(events) == 1 && proof.matchesEvent(events[0]) {
			return nil
		}
		return ErrQuarantine
	}
	if len(events) == 2 && proof.matchesEvent(events[0]) && reflect.DeepEqual(events[1], Event{Sequence: 2, Kind: "phase", Operation: "create"}) {
		return nil
	}
	return ErrQuarantine
}

// validateCreatePreEffectHistory fences observations made after the phase
// gate. It is checked before and after the durable create intent so an
// unrelated or uncertain event cannot be hidden by inventory/discovery.
func validateCreatePreEffectHistory(a Approval, j Journal, ctx context.Context, proof *controllerHandoffProof, inventory string, createIntentRecorded bool, events []Event) error {
	offset := 0
	if proof != nil {
		if !proof.validAt(ctx, a, j, time.Now()) || len(events) == 0 || !proof.matchesEvent(events[0]) {
			return ErrQuarantine
		}
		offset = 1
	}
	expected := []Event{
		{Sequence: offset + 1, Kind: "phase", Operation: "create"},
		{Sequence: offset + 2, Kind: "inventory", Digest: inventory},
		{Sequence: offset + 3, Kind: "intent", Operation: "observe-discovery"},
		{Sequence: offset + 4, Kind: "result", Operation: "observe-discovery"},
	}
	if createIntentRecorded {
		expected = append(expected, Event{Sequence: offset + 5, Kind: "intent", Operation: "create"})
	}
	if len(events) != offset+len(expected) {
		return ErrQuarantine
	}
	for index, event := range expected {
		if !reflect.DeepEqual(events[offset+index], event) {
			return ErrQuarantine
		}
	}
	return nil
}

type PreparedIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}
type PreparationReceipt struct {
	Version            int              `json:"version"`
	Status             string           `json:"status"`
	Phase              string           `json:"phase"`
	ApprovalDigest     string           `json:"approval_digest"`
	State              PreparedIdentity `json:"state"`
	Journal            PreparedIdentity `json:"journal"`
	Claim              PreparedIdentity `json:"claim"`
	AdmissionDirectory PreparedIdentity `json:"admission_directory"`
	JournalDigest      string           `json:"journal_digest"`
	ClaimDigest        string           `json:"claim_digest"`
}

const pairedPreparationPhase = "paired-terminal"

func preparedIdentity(i os.FileInfo) PreparedIdentity {
	s := i.Sys().(*syscall.Stat_t)
	return PreparedIdentity{uint64(s.Dev), s.Ino}
}
func preparedDigest(f *os.File, limit int64) (string, error) {
	i, e := f.Stat()
	if e != nil || i.Size() < 1 || i.Size() > limit {
		return "", ErrJournal
	}
	h := sha256.New()
	n, e := io.Copy(h, io.NewSectionReader(f, 0, limit+1))
	if e != nil || n != i.Size() {
		return "", ErrJournal
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// PrepareJournal performs local state preparation only. OpenJournal may create
// and pin a journal or append an explicitly permitted authority renewal. It
// never reads credentials, constructs APIs, or records a phase/remote intent.
// The receipt is captured under the canonical lease, which is released before
// return. Consumers must compare these exact receipts before subsequent effects.
func PrepareJournal(directory string, a Approval, phase string) (PreparationReceipt, error) {
	return prepareJournal(directory, a, phase, OpenJournal)
}

// PreparePairedJournal is the paired terminal's explicit local preparation
// contract. It proves the already-completed controller create prerequisite and
// its admission claim under controller authority; it does not borrow cleanup
// authority and never reads credentials, worker input or contacts a remote
// service.
func PreparePairedJournal(directory string, a Approval) (PreparationReceipt, error) {
	return preparePairedJournal(directory, a, OpenJournal)
}

func pairedPreparationReady(a Approval, now time.Time) bool {
	if a.Validate(now) != nil || a.WorkflowRunID <= 0 {
		return false
	}
	want := map[string]bool{"create": false, "inspect": false, "cleanup": false}
	verification := false
	for _, phase := range a.Phases {
		if _, ok := want[phase]; ok {
			want[phase] = true
		}
		if phase == "before-ack" || phase == "after-ack" || phase == "before-acquire" || phase == "acquire-loss" || phase == "drain" {
			verification = true
		}
	}
	return verification && want["create"] && want["inspect"] && want["cleanup"]
}

// pairedControllerPrerequisite accepts only the canonical controller-create
// prefix consumed by newBaselineListenerHeld. The paired preparation phase is
// deliberately not a second create/cleanup authority: it may inspect the
// completed create and inventory records, but it cannot reset, discard or
// append to them. Authority renewal records are structural and are ignored by
// this prefix parser; every other event must be one of the exact create
// protocol records below.
func pairedControllerPrerequisite(events []Event) (state, error) {
	s := replay(events)
	if s.uncertain || s.deleted || s.reserved || s.workObserved || s.setID <= 0 || s.inventory == "" {
		return state{}, ErrQuarantine
	}
	phase, inventory, discoveryIntent, discoveryResult, createIntent, createResult := 0, 0, 0, 0, 0, 0
	phaseAt, inventoryAt, discoveryIntentAt, discoveryResultAt, createIntentAt, createResultAt := 0, 0, 0, 0, 0, 0
	for index, e := range events {
		if e.Kind == "authority" {
			continue
		}
		position := index + 1
		switch {
		case e.Kind == "phase" && e.Operation == "create":
			phase++
			phaseAt = position
		case e.Kind == "inventory" && len(e.Digest) == 64 && isLowerHex(e.Digest):
			inventory++
			inventoryAt = position
		case e.Kind == "intent" && e.Operation == "observe-discovery":
			discoveryIntent++
			discoveryIntentAt = position
		case e.Kind == "result" && e.Operation == "observe-discovery" && e.Work == "":
			discoveryResult++
			discoveryResultAt = position
		case e.Kind == "intent" && e.Operation == "create":
			createIntent++
			createIntentAt = position
		case e.Kind == "result" && e.Operation == "create" && e.ID > 0 && e.Work == "":
			createResult++
			createResultAt = position
		default:
			return state{}, ErrQuarantine
		}
	}
	if phase != 1 || inventory != 1 || discoveryIntent != 1 || discoveryResult != 1 || createIntent != 1 || createResult != 1 || !(phaseAt < inventoryAt && inventoryAt < discoveryIntentAt && discoveryIntentAt < discoveryResultAt && discoveryResultAt < createIntentAt && createIntentAt < createResultAt) {
		return state{}, ErrQuarantine
	}
	return s, nil
}

func preparePairedJournal(directory string, a Approval, open func(string, Approval) (*FileJournal, error)) (receipt PreparationReceipt, err error) {
	if !pairedPreparationReady(a, time.Now()) || open == nil {
		return receipt, ErrApproval
	}
	j, e := open(directory, a)
	if e != nil {
		return receipt, ErrJournal
	}
	defer func() {
		if j.Close() != nil {
			receipt = PreparationReceipt{}
			err = ErrJournal
		}
	}()
	release, e := j.authorize(a)
	if e != nil {
		return receipt, ErrJournal
	}
	defer release()
	if _, e = pairedControllerPrerequisite(j.Events()); e != nil {
		return receipt, ErrQuarantine
	}
	jd, e := preparedDigest(j.file, 1<<20)
	if e != nil {
		return receipt, e
	}
	cd, e := preparedDigest(j.claim.file, 4096)
	if e != nil {
		return receipt, e
	}
	ji, e := j.file.Stat()
	if e != nil {
		return receipt, ErrJournal
	}
	ci, e := j.claim.file.Stat()
	if e != nil || !j.ownsCurrentJournal() || !j.claim.matches(j) {
		return receipt, ErrJournal
	}
	receipt = PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: pairedPreparationPhase, ApprovalDigest: approvalDigest(a), State: preparedIdentity(j.directoryInfo), Journal: preparedIdentity(ji), Claim: preparedIdentity(ci), AdmissionDirectory: preparedIdentity(j.claim.rootInfo), JournalDigest: jd, ClaimDigest: cd}
	return receipt, nil
}

func prepareJournal(directory string, a Approval, phase string, open func(string, Approval) (*FileJournal, error)) (receipt PreparationReceipt, err error) {
	if a.Validate(time.Now()) != nil || !slices.Contains(a.Phases, phase) || open == nil {
		return receipt, ErrApproval
	}
	j, e := open(directory, a)
	if e != nil {
		return receipt, ErrJournal
	}
	defer func() {
		if j.Close() != nil {
			receipt = PreparationReceipt{}
			err = ErrJournal
		}
	}()
	_, release, e := authorizePhase(a, j, phase)
	if e != nil {
		return receipt, e
	}
	defer release()
	jd, e := preparedDigest(j.file, 1<<20)
	if e != nil {
		return receipt, e
	}
	cd, e := preparedDigest(j.claim.file, 4096)
	if e != nil {
		return receipt, e
	}
	ji, e := j.file.Stat()
	if e != nil {
		return receipt, ErrJournal
	}
	ci, e := j.claim.file.Stat()
	if e != nil || !j.ownsCurrentJournal() || !j.claim.matches(j) {
		return receipt, ErrJournal
	}
	receipt = PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: phase, ApprovalDigest: approvalDigest(a), State: preparedIdentity(j.directoryInfo), Journal: preparedIdentity(ji), Claim: preparedIdentity(ci), AdmissionDirectory: preparedIdentity(j.claim.rootInfo), JournalDigest: jd, ClaimDigest: cd}
	return receipt, nil
}
