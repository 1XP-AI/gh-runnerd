package livecanary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

const maxControllerApprovalBytes = 16 << 10

type controllerHandoffParentContextKey struct{}

func controllerHandoffParentCanceled(ctx context.Context) bool {
	if ctx == nil {
		return true
	}
	parent, ok := ctx.Value(controllerHandoffParentContextKey{}).(context.Context)
	return ok && parent.Err() != nil
}

type controllerHandoffConsumption struct {
	ReceiptNonce   string `json:"receipt_nonce"`
	Phase          string `json:"phase"`
	ApprovalSHA256 string `json:"approval_sha256"`
	ReceiptSHA256  string `json:"receipt_sha256"`
	KeyID          string `json:"key_id"`
}

// A proof is an in-memory, per-open capability. Journal replay can establish
// that a nonce was consumed, but can never reconstruct this authorization.
type controllerHandoffProof struct {
	RawApprovalSHA256       string
	CanonicalApprovalDigest string
	ReceiptNonce            string
	ReceiptSHA256           string
	ReceiptExpiresAt        time.Time
	EffectiveDeadline       time.Time
	ReturnedAt              time.Time
	ContextActiveAtReturn   bool
	CredentialReadSucceeded bool
	Phase                   string
	KeyID                   string
	HandoffSequence         int
	JournalIdentity         controllerJournalIdentity

	journal *FileJournal
	parent  context.Context
}

// Receipt input is deliberately concrete: production's offline path owns a
// bounded memory snapshot; tests may supply an owned in-memory pipe.
type controllerHandoffReceiptSource struct {
	memory     []byte
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	closeOnce  sync.Once
}

func newControllerHandoffMemorySource(data []byte) *controllerHandoffReceiptSource {
	if len(data) > handoff.MaxReceiptBytes+1 {
		data = data[:handoff.MaxReceiptBytes+1]
	}
	return &controllerHandoffReceiptSource{memory: bytes.Clone(data)}
}

func (source *controllerHandoffReceiptSource) read() ([]byte, error) {
	if source == nil {
		return nil, ErrApproval
	}
	var reader io.Reader = bytes.NewReader(source.memory)
	if source.pipeReader != nil {
		reader = source.pipeReader
	}
	return io.ReadAll(io.LimitReader(reader, handoff.MaxReceiptBytes+1))
}

func (source *controllerHandoffReceiptSource) close() {
	if source == nil {
		return
	}
	source.closeOnce.Do(func() {
		if source.pipeReader != nil {
			_ = source.pipeReader.Close()
		}
		if source.pipeWriter != nil {
			_ = source.pipeWriter.Close()
		}
	})
}

func (p controllerHandoffProof) validAt(ctx context.Context, approval Approval, journal Journal, now time.Time) bool {
	if ctx == nil || ctx.Err() != nil || p.parent == nil || p.parent.Err() != nil || p.journal == nil {
		return false
	}
	fileJournal, ok := journal.(*FileJournal)
	if !ok || fileJournal != p.journal || !p.ContextActiveAtReturn || !p.CredentialReadSucceeded || p.HandoffSequence != 1 || p.Phase != "create" {
		return false
	}
	if !component.MatchString(p.KeyID) || !nonce.MatchString(p.ReceiptNonce) || !validHandoffSHA256(p.RawApprovalSHA256) || !validHandoffSHA256(p.ReceiptSHA256) {
		return false
	}
	if p.CanonicalApprovalDigest != approvalDigest(approval) || !p.ReceiptExpiresAt.After(now) || !p.EffectiveDeadline.After(now) || p.ReturnedAt.IsZero() || p.ReturnedAt.After(now) || p.EffectiveDeadline.After(approval.ExpiresAt) || p.EffectiveDeadline.After(p.ReceiptExpiresAt) {
		return false
	}
	identity, err := fileJournal.controllerIdentity()
	return err == nil && identity == p.JournalIdentity
}

func (p controllerHandoffProof) matchesEvent(event Event) bool {
	if event.Sequence != p.HandoffSequence || !validControllerHandoffEvent(event) || event.ControllerHandoff == nil {
		return false
	}
	consumption := event.ControllerHandoff
	return consumption.ReceiptNonce == p.ReceiptNonce && consumption.Phase == p.Phase && consumption.ApprovalSHA256 == p.RawApprovalSHA256 && consumption.ReceiptSHA256 == p.ReceiptSHA256 && consumption.KeyID == p.KeyID
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

func (j *FileJournal) consumeControllerHandoff(approval Approval, consumption controllerHandoffConsumption) (Event, error) {
	return j.consumeControllerHandoffAt(context.Background(), approval, consumption, approval.ExpiresAt, time.Now, func(now time.Time) bool {
		return approval.Validate(now) == nil
	})
}

// Fresh authorization is sampled only after acquiring mu, immediately before
// the fsynced one-shot nonce append. Callers may not pass a timestamp captured
// before waiting for this critical section.
func (j *FileJournal) consumeControllerHandoffAt(ctx context.Context, approval Approval, consumption controllerHandoffConsumption, deadline time.Time, now func() time.Time, validate func(time.Time) bool) (Event, error) {
	if now == nil || validate == nil {
		return Event{}, ErrJournal
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	appendNow := now()
	event := Event{Kind: "controller-handoff-consumed", ControllerHandoff: &consumption}
	if ctx == nil || ctx.Err() != nil || !deadline.After(appendNow) || !j.authorityHeldAt(approval, appendNow) || !validate(appendNow) || !validEvent(event) || controllerHandoffNonceUsed(j.events, consumption.ReceiptNonce) {
		return Event{}, ErrJournal
	}
	return j.appendStored(event)
}

func controllerHandoffRequest(approvalSnapshot []byte, phase, source string, now time.Time) (Approval, handoff.Request, error) {
	if len(approvalSnapshot) == 0 || len(approvalSnapshot) > maxControllerApprovalBytes {
		return Approval{}, handoff.Request{}, ErrApproval
	}
	return controllerHandoffRequestOwned(bytes.Clone(approvalSnapshot), phase, source, now)
}

func controllerHandoffRequestOwned(approvalSnapshot []byte, phase, source string, now time.Time) (Approval, handoff.Request, error) {
	var approval Approval
	if len(approvalSnapshot) == 0 || len(approvalSnapshot) > maxControllerApprovalBytes || DecodeStrict(approvalSnapshot, &approval) != nil || approval.Validate(now) != nil || approval.WorkflowRunID <= 0 || approval.WorkflowRef == "" || !slices.Contains(approval.Phases, phase) {
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

func controllerHandoffAcquisitionDeadline(ctx context.Context, approvalExpiry, started time.Time) time.Time {
	deadline := started.Add(operationTimeout)
	if approvalExpiry.Before(deadline) {
		deadline = approvalExpiry
	}
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	return deadline
}

// readControllerHandoffReceipt keeps the potentially blocking read in the
// caller goroutine. The owned pipe is closed on cancellation and its watcher
// is joined before return; the source applies the 16 KiB+1 framing limit.
func readControllerHandoffReceipt(ctx context.Context, source *controllerHandoffReceiptSource) ([]byte, error) {
	if ctx == nil || source == nil {
		return nil, ErrApproval
	}
	if ctx.Err() != nil {
		return nil, ErrApproval
	}
	stopWatcher := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			source.close()
		case <-stopWatcher:
		}
	}()
	defer func() {
		close(stopWatcher)
		source.close()
		<-watcherDone
	}()
	data, err := source.read()
	if err != nil {
		return nil, ErrApproval
	}
	return data, nil
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

func controllerHandoffInputsMatch(ctx context.Context, journal *FileJournal, approvalSnapshot []byte, approval Approval, request handoff.Request, phase, source string, receiptBytes []byte, receipt handoff.Receipt, root handoff.TrustRoot, effectiveDeadline, now time.Time) bool {
	if ctx == nil || ctx.Err() != nil || !effectiveDeadline.After(now) || !controllerHandoffActive(ctx, approval.ExpiresAt, receipt.ExpiresAt, now) {
		return false
	}
	derivedApproval, derivedRequest, err := controllerHandoffRequestOwned(approvalSnapshot, phase, source, now)
	if err != nil || !reflect.DeepEqual(derivedApproval, approval) || derivedRequest != request || root.Verify(request, receipt, now) != nil || !journal.authorityHeldAt(approval, now) {
		return false
	}
	approvalHash := sha256.Sum256(approvalSnapshot)
	return request.ControllerApprovalSHA256 == hex.EncodeToString(approvalHash[:]) && len(receiptBytes) <= handoff.MaxReceiptBytes
}

func consumeControllerHandoff(ctx context.Context, journal *FileJournal, approvalSnapshot []byte, phase, source string, receiptSource *controllerHandoffReceiptSource, root handoff.TrustRoot, now func() time.Time, readCredentials func(context.Context) ([]byte, error)) ([]byte, error) {
	credentials, _, err := consumeControllerHandoffWithProof(ctx, journal, approvalSnapshot, phase, source, receiptSource, root, now, readCredentials)
	return credentials, err
}

func consumeControllerHandoffWithProof(ctx context.Context, journal *FileJournal, approvalSnapshot []byte, phase, source string, receiptSource *controllerHandoffReceiptSource, root handoff.TrustRoot, now func() time.Time, readCredentials func(context.Context) ([]byte, error)) ([]byte, controllerHandoffProof, error) {
	var zeroProof controllerHandoffProof
	defer receiptSource.close()
	if ctx == nil || journal == nil || receiptSource == nil || !root.Valid() || now == nil || readCredentials == nil || ctx.Err() != nil {
		return nil, zeroProof, ErrApproval
	}
	started := now()
	if len(approvalSnapshot) == 0 || len(approvalSnapshot) > maxControllerApprovalBytes {
		return nil, zeroProof, ErrApproval
	}
	ownedApprovalSnapshot := bytes.Clone(approvalSnapshot)
	approval, request, err := controllerHandoffRequestOwned(ownedApprovalSnapshot, phase, source, started)
	if err != nil {
		return nil, zeroProof, err
	}
	acquisitionDeadline := controllerHandoffAcquisitionDeadline(ctx, approval.ExpiresAt, started)
	if !acquisitionDeadline.After(started) {
		return nil, zeroProof, ErrApproval
	}
	readerContext, cancelReader := context.WithDeadline(ctx, acquisitionDeadline)
	receiptBytes, err := readControllerHandoffReceipt(readerContext, receiptSource)
	cancelReader()
	if err != nil || len(receiptBytes) == 0 || len(receiptBytes) > handoff.MaxReceiptBytes || ctx.Err() != nil {
		return nil, zeroProof, ErrApproval
	}
	receipt, err := handoff.DecodeStrictReceipt(receiptBytes)
	verifiedAt := now()
	if err != nil || root.Verify(request, receipt, verifiedAt) != nil {
		return nil, zeroProof, ErrApproval
	}
	effectiveDeadline := acquisitionDeadline
	if receipt.ExpiresAt.Before(effectiveDeadline) {
		effectiveDeadline = receipt.ExpiresAt
	}
	if !effectiveDeadline.After(verifiedAt) || !controllerHandoffActive(ctx, approval.ExpiresAt, receipt.ExpiresAt, verifiedAt) {
		return nil, zeroProof, ErrApproval
	}
	leaseNow := now()
	release, err := journal.authorizeAt(approval, leaseNow)
	if err != nil {
		return nil, zeroProof, ErrJournal
	}
	leaseHeld := true
	defer func() {
		if leaseHeld {
			release()
		}
	}()
	approvalHash, _ := hex.DecodeString(request.ControllerApprovalSHA256)
	receiptHash := sha256.Sum256(receiptBytes)
	consumption := controllerHandoffConsumption{
		ReceiptNonce:   receipt.ReceiptNonce,
		Phase:          phase,
		ApprovalSHA256: hex.EncodeToString(approvalHash),
		ReceiptSHA256:  hex.EncodeToString(receiptHash[:]),
		KeyID:          root.KeyID(),
	}
	validateAtAppend := func(appendAt time.Time) bool {
		return controllerHandoffInputsMatch(ctx, journal, ownedApprovalSnapshot, approval, request, phase, source, receiptBytes, receipt, root, effectiveDeadline, appendAt)
	}
	consumedEvent, err := journal.consumeControllerHandoffAt(ctx, approval, consumption, effectiveDeadline, now, validateAtAppend)
	if err != nil {
		return nil, zeroProof, ErrJournal
	}
	postAppendNow := now()
	if ctx.Err() != nil || !effectiveDeadline.After(postAppendNow) || !controllerHandoffActive(ctx, approval.ExpiresAt, receipt.ExpiresAt, postAppendNow) {
		return nil, zeroProof, ErrApproval
	}
	// The durable nonce fence is committed before credentials are read, but the
	// journal lifecycle lease is deliberately not held across that input.
	release()
	leaseHeld = false
	credentialContext, cancelCredentials := context.WithDeadline(ctx, effectiveDeadline)
	credentials, credentialErr := readCredentials(credentialContext)
	credentialContextErr := credentialContext.Err()
	cancelCredentials()
	if credentialErr != nil || len(credentials) == 0 || ctx.Err() != nil || credentialContextErr != nil || !effectiveDeadline.After(now()) || !controllerHandoffActive(ctx, approval.ExpiresAt, receipt.ExpiresAt, now()) {
		clear(credentials)
		return nil, zeroProof, ErrApproval
	}
	finalNow := now()
	finalRelease, err := journal.authorizeAt(approval, finalNow)
	if err != nil {
		clear(credentials)
		return nil, zeroProof, ErrJournal
	}
	finalOK := controllerHandoffInputsMatch(ctx, journal, ownedApprovalSnapshot, approval, request, phase, source, receiptBytes, receipt, root, effectiveDeadline, now())
	if finalOK {
		events := journal.Events()
		finalOK = consumedEvent.Sequence > 0 && len(events) >= consumedEvent.Sequence && reflect.DeepEqual(events[consumedEvent.Sequence-1], consumedEvent)
	}
	identity, identityErr := journal.controllerIdentity()
	finalRelease()
	if !finalOK || identityErr != nil {
		clear(credentials)
		return nil, zeroProof, ErrApproval
	}
	returnedAt := now()
	if ctx.Err() != nil || !effectiveDeadline.After(returnedAt) || !receipt.ExpiresAt.After(returnedAt) || !controllerHandoffActive(ctx, approval.ExpiresAt, receipt.ExpiresAt, returnedAt) {
		clear(credentials)
		return nil, zeroProof, ErrApproval
	}
	proof := controllerHandoffProof{
		RawApprovalSHA256:       request.ControllerApprovalSHA256,
		CanonicalApprovalDigest: approvalDigest(approval),
		ReceiptNonce:            receipt.ReceiptNonce,
		ReceiptSHA256:           hex.EncodeToString(receiptHash[:]),
		ReceiptExpiresAt:        receipt.ExpiresAt,
		EffectiveDeadline:       effectiveDeadline,
		ReturnedAt:              returnedAt,
		ContextActiveAtReturn:   true,
		CredentialReadSucceeded: true,
		Phase:                   phase,
		KeyID:                   root.KeyID(),
		HandoffSequence:         consumedEvent.Sequence,
		JournalIdentity:         identity,
		journal:                 journal,
		parent:                  ctx,
	}
	return credentials, proof, nil
}
