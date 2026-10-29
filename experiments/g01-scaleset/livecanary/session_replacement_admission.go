package livecanary

import (
	"context"
	"slices"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

const replacementPhase = "session-replacement"

type replacementSnapshotKey struct{}

// Replacement never borrows renewal or replay authority. Both canonical and
// original serialized approval identities must match the first create grant.
func replacementCreatePrefix(a Approval, j *FileJournal, raw string, events []Event) (completedCreateHistory, error) {
	if j == nil || len(events) != 7 || raw == "" || !slices.Contains(a.Phases, replacementPhase) || j.authority.Digest != approvalDigest(a) || events[0].ControllerHandoff == nil || events[0].ControllerHandoff.Phase != "create" || events[0].ControllerHandoff.ApprovalSHA256 != raw {
		return completedCreateHistory{}, ErrQuarantine
	}
	for _, e := range events {
		if e.Authority != nil {
			return completedCreateHistory{}, ErrQuarantine
		}
	}
	return parseCompletedCreateHistory(events)
}

func replacementAdmission(a Approval, j *FileJournal, raw string, events []Event, proof *controllerHandoffProof, ctx context.Context, post bool) error {
	n := 7
	if proof != nil {
		n++
		if !proof.validAt(ctx, a, j, time.Now()) || proof.Phase != replacementPhase || proof.RawApprovalSHA256 != raw || len(events) < n || !proof.matchesEvent(events[7]) {
			return ErrQuarantine
		}
	}
	if post {
		n++
	}
	if len(events) != n {
		return ErrQuarantine
	}
	if _, e := replacementCreatePrefix(a, j, raw, events[:7]); e != nil {
		return e
	}
	if post && (events[n-1].Kind != "phase" || events[n-1].Operation != replacementPhase || events[n-1].Sequence != n) {
		return ErrQuarantine
	}
	return nil
}

// The command must retain the original bytes; a decoded approval alone cannot
// prove that its raw hash is the approval consumed by create.
func PrepareSessionReplacementJournal(directory string, a Approval, approvalPath string) (PreparationReceipt, error) {
	return prepareSessionReplacementJournal(directory, a, approvalPath, OpenJournal)
}
func prepareSessionReplacementJournal(directory string, a Approval, path string, open func(string, Approval) (*FileJournal, error)) (PreparationReceipt, error) {
	var current Approval
	snap, err := readPairedApproval(path, &current, DecodeStrict)
	if err != nil || approvalDigest(a) != approvalDigest(current) {
		return PreparationReceipt{}, ErrApproval
	}
	return prepareJournalSnapshot(directory, a, replacementPhase, open, handoff.Digest(snap.raw))
}
