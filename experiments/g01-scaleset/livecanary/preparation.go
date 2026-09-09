package livecanary

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"slices"
	"syscall"
	"time"
)

// authorizePhase is the canonical local gate shared by preparation and Run.
// It returns a held execution lease. It never records a phase or contacts APIs.
func authorizePhase(a Approval, j Journal, phase string) (state, func(), error) {
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
	s := replay(events)
	invalid := phase != "inspect" && (s.uncertain || s.phaseSeen[phase] || s.deleted)
	// Run has not appended its phase record yet: an eligible create has no events.
	invalid = invalid || (phase == "create" && (s.setID != 0 || len(events) != 0))
	invalid = invalid || (phase != "create" && phase != "inspect" && (s.setID <= 0 || s.reserved))
	invalid = invalid || (phase == "cleanup" && (s.workObserved || len(s.observedJobs) != 0 || s.inventory == ""))
	if invalid {
		release()
		return state{}, nil, ErrQuarantine
	}
	return s, release, nil
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
		if phase == "before-ack" || phase == "after-ack" || phase == "before-acquire" || phase == "acquire-loss" {
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
