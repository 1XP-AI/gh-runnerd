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
