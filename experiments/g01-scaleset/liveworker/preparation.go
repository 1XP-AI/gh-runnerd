package liveworker

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"time"
)

// PreparationReceipt is a credential-free snapshot of the canonical worker
// journal/admission boundary. It is returned only after OpenJournal has replayed
// the complete history and held the worker authority lease.
type PreparationReceipt struct {
	Version            int          `json:"version"`
	Status             string       `json:"status"`
	Phase              string       `json:"phase"`
	ApprovalDigest     string       `json:"approval_digest"`
	State              FileIdentity `json:"state"`
	Journal            FileIdentity `json:"journal"`
	Claim              FileIdentity `json:"claim"`
	AdmissionDirectory FileIdentity `json:"admission_directory"`
	JournalDigest      string       `json:"journal_digest"`
	ClaimDigest        string       `json:"claim_digest"`
}

func preparationDigest(file *os.File, limit int64) (string, error) {
	info, err := file.Stat()
	if err != nil || info.Size() < 1 || info.Size() > limit {
		return "", ErrState
	}
	hash := sha256.New()
	read, err := io.Copy(hash, io.NewSectionReader(file, 0, limit+1))
	if err != nil || read != info.Size() {
		return "", ErrState
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func prepareJournal(directory string, a Approval, open func(string, Approval) (*FileJournal, error)) (receipt PreparationReceipt, err error) {
	if open == nil || a.Validate(time.Now()) != nil {
		return receipt, ErrApproval
	}
	j, err := open(directory, a)
	if err != nil {
		return receipt, ErrState
	}
	defer func() {
		if j.Close() != nil {
			receipt = PreparationReceipt{}
			err = ErrState
		}
	}()
	release, err := j.authorize(a)
	if err != nil {
		return receipt, ErrState
	}
	defer release()
	// The paired broker is preparing the first worker handoff. A prior effect,
	// reservation, uncertainty or paired binding is retained for inspection but
	// cannot be adopted as fresh authority for another mint/launch.
	for _, event := range j.Events() {
		if event.Kind != "authority" {
			return receipt, ErrState
		}
	}
	if !j.ownsCurrentJournal() || j.claim == nil || !j.claim.matches(j) {
		return receipt, ErrState
	}
	journalInfo, err := j.file.Stat()
	if err != nil {
		return receipt, ErrState
	}
	claimInfo, err := j.claim.file.Stat()
	if err != nil {
		return receipt, ErrState
	}
	journalDigest, err := preparationDigest(j.file, maxJournal)
	if err != nil {
		return receipt, ErrState
	}
	claimDigest, err := preparationDigest(j.claim.file, 4096)
	if err != nil {
		return receipt, ErrState
	}
	if !j.ownsCurrentJournal() || !j.claim.matches(j) {
		return receipt, ErrState
	}
	return PreparationReceipt{
		Version:            1,
		Status:             "worker_journal_prepared",
		Phase:              "paired-worker",
		ApprovalDigest:     approvalDigest(a),
		State:              identityOf(j.directoryInfo),
		Journal:            identityOf(journalInfo),
		Claim:              identityOf(claimInfo),
		AdmissionDirectory: identityOf(j.claim.rootInfo),
		JournalDigest:      journalDigest,
		ClaimDigest:        claimDigest,
	}, nil
}

// PrepareJournal validates the worker approval, replays its canonical journal,
// and checks the permanent admission claim without reading credentials or
// contacting a runtime. It may initialize a new worker journal, but never
// records a worker effect.
func PrepareJournal(directory string, a Approval) (PreparationReceipt, error) {
	return prepareJournal(directory, a, OpenJournal)
}
