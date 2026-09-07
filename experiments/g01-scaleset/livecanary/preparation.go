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
	Version        int              `json:"version"`
	Status         string           `json:"status"`
	Phase          string           `json:"phase"`
	ApprovalDigest string           `json:"approval_digest"`
	State          PreparedIdentity `json:"state"`
	Journal        PreparedIdentity `json:"journal"`
	Claim          PreparedIdentity `json:"claim"`
	JournalDigest  string           `json:"journal_digest"`
	ClaimDigest    string           `json:"claim_digest"`
}

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
	receipt = PreparationReceipt{1, "controller_journal_prepared", phase, approvalDigest(a), preparedIdentity(j.directoryInfo), preparedIdentity(ji), preparedIdentity(ci), jd, cd}
	return receipt, nil
}
