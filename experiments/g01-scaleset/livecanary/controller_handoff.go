package livecanary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"reflect"
	"slices"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

type controllerHandoffConsumption struct {
	ReceiptNonce   string `json:"receipt_nonce"`
	Phase          string `json:"phase"`
	ApprovalSHA256 string `json:"approval_sha256"`
	ReceiptSHA256  string `json:"receipt_sha256"`
	KeyID          string `json:"key_id"`
}

func validControllerHandoffEvent(event Event) bool {
	if event.Sequence < 0 || event.Kind != "controller-handoff-consumed" || event.ControllerHandoff == nil {
		return false
	}
	other := event
	other.ControllerHandoff = nil
	if !reflect.DeepEqual(other, Event{Sequence: event.Sequence, Kind: event.Kind}) {
		return false
	}
	consumption := event.ControllerHandoff
	if !nonce.MatchString(consumption.ReceiptNonce) || !slices.Contains(phases, consumption.Phase) || !component.MatchString(consumption.KeyID) {
		return false
	}
	return validHandoffSHA256(consumption.ApprovalSHA256) && validHandoffSHA256(consumption.ReceiptSHA256)
}

func validHandoffSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func controllerHandoffNonceUsed(events []Event, receiptNonce string) bool {
	for _, event := range events {
		if event.ControllerHandoff != nil && event.ControllerHandoff.ReceiptNonce == receiptNonce {
			return true
		}
	}
	return false
}

func validateControllerHandoffReplay(events []Event) error {
	seen := make(map[string]struct{})
	for _, event := range events {
		if event.ControllerHandoff == nil {
			continue
		}
		if !validControllerHandoffEvent(event) {
			return ErrJournal
		}
		nonce := event.ControllerHandoff.ReceiptNonce
		if _, exists := seen[nonce]; exists {
			return ErrJournal
		}
		seen[nonce] = struct{}{}
	}
	return nil
}

func (j *FileJournal) consumeControllerHandoff(approval Approval, consumption controllerHandoffConsumption) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	event := Event{Kind: "controller-handoff-consumed", ControllerHandoff: &consumption}
	if !j.authorityHeld(approval) || !validEvent(event) || controllerHandoffNonceUsed(j.events, consumption.ReceiptNonce) {
		return ErrJournal
	}
	if _, err := j.appendStored(event); err != nil {
		return ErrJournal
	}
	return nil
}

func controllerHandoffRequest(approvalSnapshot []byte, phase, source string, now time.Time) (Approval, handoff.Request, error) {
	var approval Approval
	if len(approvalSnapshot) == 0 || len(approvalSnapshot) > 16384 || DecodeStrict(approvalSnapshot, &approval) != nil || approval.Validate(now) != nil || approval.WorkflowRunID <= 0 || approval.WorkflowRef == "" || !slices.Contains(approval.Phases, phase) {
		return Approval{}, handoff.Request{}, ErrApproval
	}
	approvalHash := sha256.Sum256(approvalSnapshot)
	request := handoff.Request{
		ControllerApprovalSHA256: hex.EncodeToString(approvalHash[:]),
		Repository:               approval.Organization + "/" + approval.Repository,
		WorkflowRunID:            approval.WorkflowRunID,
		WorkflowRef:              approval.WorkflowRef,
		WorkflowSHA:              approval.WorkflowSHA,
		WorkflowPath:             approval.WorkflowPath,
		Phase:                    phase,
		OwnerNonce:               approval.OwnerNonce,
		Source:                   source,
	}
	if !request.Valid() {
		return Approval{}, handoff.Request{}, ErrApproval
	}
	return approval, request, nil
}

func controllerHandoffDeadline(ctx context.Context, approvalExpiry, receiptExpiry time.Time) time.Time {
	deadline := approvalExpiry
	if receiptExpiry.Before(deadline) {
		deadline = receiptExpiry
	}
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	return deadline
}

func controllerHandoffActive(ctx context.Context, approvalExpiry, receiptExpiry, now time.Time) bool {
	if ctx == nil || ctx.Err() != nil || !approvalExpiry.After(now) || !receiptExpiry.After(now) {
		return false
	}
	if deadline, ok := ctx.Deadline(); ok && !deadline.After(now) {
		return false
	}
	return true
}

func consumeControllerHandoff(ctx context.Context, journal *FileJournal, approvalSnapshot []byte, phase, source string, receiptSource io.Reader, root handoff.TrustRoot, now func() time.Time, readCredentials func(context.Context) ([]byte, error)) ([]byte, error) {
	if ctx == nil || journal == nil || receiptSource == nil || !root.Valid() || now == nil || readCredentials == nil || ctx.Err() != nil {
		return nil, ErrApproval
	}
	approval, request, err := controllerHandoffRequest(approvalSnapshot, phase, source, now())
	if err != nil {
		return nil, err
	}
	release, err := journal.authorize(approval)
	if err != nil {
		return nil, ErrJournal
	}
	defer release()
	receiptBytes, err := io.ReadAll(io.LimitReader(receiptSource, handoff.MaxReceiptBytes+1))
	if err != nil || len(receiptBytes) == 0 || len(receiptBytes) > handoff.MaxReceiptBytes {
		return nil, ErrApproval
	}
	receipt, err := handoff.DecodeStrictReceipt(receiptBytes)
	if err != nil || root.Verify(request, receipt, now()) != nil {
		return nil, ErrApproval
	}
	if !controllerHandoffActive(ctx, approval.ExpiresAt, receipt.ExpiresAt, now()) || !journal.authorityHeld(approval) {
		return nil, ErrApproval
	}
	approvalHash, _ := hex.DecodeString(request.ControllerApprovalSHA256)
	receiptHash := sha256.Sum256(receiptBytes)
	consumption := controllerHandoffConsumption{
		ReceiptNonce:   receipt.ReceiptNonce,
		Phase:          phase,
		ApprovalSHA256: hex.EncodeToString(approvalHash),
		ReceiptSHA256:  hex.EncodeToString(receiptHash[:]),
		KeyID:          root.KeyID(),
	}
	if err := journal.consumeControllerHandoff(approval, consumption); err != nil {
		return nil, ErrJournal
	}
	if !controllerHandoffActive(ctx, approval.ExpiresAt, receipt.ExpiresAt, now()) || !journal.authorityHeld(approval) {
		return nil, ErrApproval
	}
	readerContext, cancel := context.WithDeadline(ctx, controllerHandoffDeadline(ctx, approval.ExpiresAt, receipt.ExpiresAt))
	defer cancel()
	if !controllerHandoffActive(readerContext, approval.ExpiresAt, receipt.ExpiresAt, now()) || !journal.authorityHeld(approval) {
		return nil, ErrApproval
	}
	credentials, err := readCredentials(readerContext)
	if err != nil || !controllerHandoffActive(readerContext, approval.ExpiresAt, receipt.ExpiresAt, now()) || !journal.authorityHeld(approval) {
		clear(credentials)
		return nil, ErrApproval
	}
	return credentials, nil
}
