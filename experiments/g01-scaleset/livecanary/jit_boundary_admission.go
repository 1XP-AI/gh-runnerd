package livecanary

import (
	"context"
	"os"
	"reflect"
	"slices"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

const beforeJITPhase = "before-jit"

func jitBoundaryPhase(p string) bool { return p == beforeJITPhase || p == "jit-loss" }

type jitBoundarySnapshotKey struct{}
type jitBoundaryRawKey struct{}
type jitBoundaryGuardKey struct{}

func jitBoundaryCreatePrefix(a Approval, j *FileJournal, raw, phase string, events []Event) (completedCreateHistory, error) {
	if j == nil || !jitBoundaryPhase(phase) || !slices.Contains(a.Phases, phase) || len(events) != 7 || !validHandoffSHA256(raw) || j.authority.Digest != approvalDigest(a) || events[0].ControllerHandoff == nil || events[0].ControllerHandoff.Phase != "create" || events[0].ControllerHandoff.ApprovalSHA256 != raw {
		return completedCreateHistory{}, ErrQuarantine
	}
	for _, e := range events {
		if e.Authority != nil {
			return completedCreateHistory{}, ErrQuarantine
		}
	}
	return parseCompletedCreateHistory(events)
}
func jitBoundaryAdmission(a Approval, j *FileJournal, raw, phase string, events []Event, proof *controllerHandoffProof, ctx context.Context, post bool) error {
	n := 7
	if proof != nil {
		n++
		if !proof.validAt(ctx, a, j, time.Now()) || proof.Phase != phase || proof.RawApprovalSHA256 != raw || len(events) < n || !proof.matchesEvent(events[7]) {
			return ErrQuarantine
		}
	}
	if post {
		n++
	}
	if len(events) != n {
		return ErrQuarantine
	}
	if _, e := jitBoundaryCreatePrefix(a, j, raw, phase, events[:7]); e != nil {
		return e
	}
	if post && (events[8].Kind != "phase" || events[8].Operation != phase || events[8].Sequence != 9) {
		return ErrQuarantine
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if !reflect.DeepEqual(events, j.events) || !j.jitBoundaryPhysicalLocked() {
		return ErrJournal
	}
	return nil
}

// Both modes require the original serialized approval, before any remint.
func PrepareJITBoundaryJournal(directory string, a Approval, approvalPath, phase string) (PreparationReceipt, error) {
	return prepareJITBoundaryJournal(directory, a, approvalPath, phase, OpenJournal)
}
func prepareJITBoundaryJournal(directory string, a Approval, path, phase string, open func(string, Approval) (*FileJournal, error)) (PreparationReceipt, error) {
	var current Approval
	snap, err := readPairedApproval(path, &current, DecodeStrict)
	if err != nil || !jitBoundaryPhase(phase) || approvalDigest(a) != approvalDigest(current) {
		return PreparationReceipt{}, ErrApproval
	}
	receipt, err := prepareJournalSnapshot(directory, a, phase, open, handoff.Digest(snap.raw))
	if err != nil {
		return PreparationReceipt{}, err
	}
	var again Approval
	live, err := readPairedApproval(path, &again, DecodeStrict)
	if err != nil || !os.SameFile(snap.info, live.info) || handoff.Digest(snap.raw) != handoff.Digest(live.raw) || approvalDigest(again) != approvalDigest(a) {
		return PreparationReceipt{}, ErrApproval
	}
	return receipt, nil
}
