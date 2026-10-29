package livecanary

import (
	"bytes"
	"context"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// Private offline composition; no CLI or broker entry point selects this path.
// The injected factory must be network-lazy. It receives validated credentials,
// never the per-open proof; only Driver may use that proof for the single create.
func runControllerHandoffCreate(ctx context.Context, journal *FileJournal, approvalSnapshot, receiptBytes []byte, source string, root handoff.TrustRoot, now func() time.Time, readCredentials func(context.Context) ([]byte, error), apiFactory func(context.Context, Approval, Credentials) (API, error)) error {
	if ctx == nil || now == nil || apiFactory == nil || len(approvalSnapshot) == 0 || len(approvalSnapshot) > maxControllerApprovalBytes {
		return ErrApproval
	}
	// One private snapshot supplies both the receipt request and Driver approval.
	snapshot := bytes.Clone(approvalSnapshot)
	approval, _, err := controllerHandoffRequestOwned(snapshot, "create", source, now())
	if err != nil {
		return ErrApproval
	}
	data, proof, err := consumeControllerHandoffWithProof(ctx, journal, snapshot, "create", source, newControllerHandoffMemorySource(receiptBytes), root, now, readCredentials)
	if err != nil {
		return err
	}
	defer clear(data)
	var c Credentials
	if len(data) > 16<<10 || DecodeStrict(data, &c) != nil || c.validate(approval, now()) != nil || c.PairedBinding != nil || !proof.validAt(ctx, approval, journal, now()) {
		return ErrApproval
	}
	clear(data) // Best effort: decoded Go strings and the adapter retain copies.
	entryContext, cancel := context.WithDeadline(ctx, proof.EffectiveDeadline)
	defer cancel()
	api, err := apiFactory(entryContext, approval, c)
	if err != nil || api == nil || !proof.validAt(entryContext, approval, journal, now()) {
		return ErrApproval
	}
	driver := Driver{approval, journal, api}
	if driver.runWithHandoff(entryContext, "create", &proof) != nil {
		return ErrQuarantine
	}
	return nil
}

// The phase is selected by verified immutable policy before this private entry.
func runControllerHandoffPhase(ctx context.Context, journal *FileJournal, snapshot, receipt []byte, phase, source string, root handoff.TrustRoot, now func() time.Time, read func(context.Context) ([]byte, error), factory func(context.Context, Approval, Credentials) (API, error), paired *PairedTerminalFiles) error {
	if ctx == nil || ctx.Err() != nil || journal == nil || now == nil || factory == nil || phase == "create" || phase == "cleanup" {
		return ErrApproval
	}
	a, _, err := controllerHandoffRequest(snapshot, phase, source, now())
	if err != nil {
		return ErrApproval
	}
	if phase == pairedPreparationPhase {
		if paired == nil || !pairedPreparationReady(a, now()) {
			return ErrApproval
		}
		release, e := journal.authorize(a)
		if e != nil {
			return ErrJournal
		}
		_, e = parseCompletedCreateHistory(journal.Events())
		release()
		if e != nil {
			return ErrQuarantine
		}
	} else if jitBoundaryPhase(phase) {
		release, e := journal.authorize(a)
		if e != nil {
			return ErrJournal
		}
		e = jitBoundaryAdmission(a, journal, handoff.Digest(snapshot), phase, journal.Events(), nil, ctx, false)
		release()
		if e != nil {
			return e
		}
	} else if phase == replacementPhase {
		release, e := journal.authorize(a)
		if e != nil {
			return ErrJournal
		}
		e = replacementAdmission(a, journal, handoff.Digest(snapshot), journal.Events(), nil, ctx, false)
		release()
		if e != nil {
			return e
		}
	} else {
		_, release, e := authorizePhase(a, journal, phase)
		if e != nil {
			return e
		}
		release()
	}
	ctx = context.WithValue(ctx, experimentalPhaseAdmissionKey{}, true)
	ctx = context.WithValue(ctx, jitBoundarySnapshotKey{}, handoff.Digest(snapshot))
	ctx = context.WithValue(ctx, jitBoundaryRawKey{}, snapshot)
	ctx = context.WithValue(ctx, replacementSnapshotKey{}, handoff.Digest(snapshot))
	ctx = context.WithValue(ctx, replacementRawSnapshotKey{}, snapshot)
	data, proof, err := consumeControllerHandoffWithProof(ctx, journal, snapshot, phase, source, newControllerHandoffMemorySource(receipt), root, now, read)
	if err != nil {
		return err
	}
	defer clear(data)
	var c Credentials
	if DecodeStrict(data, &c) != nil || c.validate(a, now()) != nil || !proof.validAt(ctx, a, journal, now()) {
		return ErrApproval
	}
	entry, cancel := context.WithDeadline(ctx, proof.EffectiveDeadline)
	defer cancel()
	entry = context.WithValue(entry, controllerHandoffParentContextKey{}, proof.parent)
	if phase == pairedPreparationPhase {
		if c.PairedBinding == nil || !proof.EffectiveDeadline.After(now().Add(handoff.MinPairedRemaining)) {
			return ErrApproval
		}
		return runPairedTerminalOwned(entry, *paired, c, journal, &proof)
	}
	if c.PairedBinding != nil {
		return ErrApproval
	}
	api, err := factory(entry, a, c)
	if err != nil || api == nil || !proof.validAt(entry, a, journal, now()) {
		return ErrApproval
	}
	return (&Driver{a, journal, api}).runWithHandoff(entry, phase, &proof)
}
