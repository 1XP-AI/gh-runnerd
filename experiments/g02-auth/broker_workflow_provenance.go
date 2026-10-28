package enrollment

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

const workflowProvenanceSource = "github-rest-workflow-v1"

// This exact run/phase grant is independent reviewed configuration, never built
// from an Attest argument or inferred from GitHub workflow inputs.
type reviewedWorkflowGrant struct {
	Request                      BrokerProvenanceRequest
	OrganizationID, RepositoryID int64
	ExpiresAt                    time.Time
}

func (g reviewedWorkflowGrant) valid() bool {
	return g.Request.Valid() && g.Request.Source == workflowProvenanceSource && brokerWorkflowRunHeadBranchMatchesRef(strings.TrimPrefix(g.Request.WorkflowRef, "refs/heads/"), g.Request.WorkflowRef) && g.OrganizationID > 0 && g.RepositoryID > 0 && !g.ExpiresAt.IsZero()
}

type receiptSigner interface {
	Sign(context.Context, []byte) ([]byte, error)
}

type workflowProvenanceAdapter struct {
	grant          reviewedWorkflowGrant
	observer       *workflowRunObserver
	readCredential func(context.Context) (string, error)
	signer         receiptSigner
	keyID          string
	now            func() time.Time
	nonce          func() ([16]byte, error)
	checkAttempt   func() error
}

func newWorkflowProvenanceAdapter(grant reviewedWorkflowGrant, observer *workflowRunObserver, readCredential func(context.Context) (string, error), signer receiptSigner, keyID string, now func() time.Time, nonce func() ([16]byte, error), checkAttempt func() error) (BrokerProvenanceAdapter, error) {
	if !grant.valid() || observer == nil || observer.client == nil || readCredential == nil || signer == nil || !brokerComponent.MatchString(keyID) || now == nil || checkAttempt == nil {
		return nil, errBroker
	}
	instant := now()
	if instant.IsZero() || !grant.ExpiresAt.After(instant) || grant.ExpiresAt.After(instant.Add(24*time.Hour)) {
		return nil, errBroker
	}
	if nonce == nil {
		nonce = func() ([16]byte, error) {
			var b [16]byte
			_, err := rand.Read(b[:])
			return b, err
		}
	}
	return &workflowProvenanceAdapter{grant: grant, observer: observer, readCredential: readCredential, signer: signer, keyID: keyID, now: now, nonce: nonce, checkAttempt: checkAttempt}, nil
}
func (*workflowProvenanceAdapter) Source() string { return workflowProvenanceSource }
func (a *workflowProvenanceAdapter) check(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || a.checkAttempt() != nil {
		return errBroker
	}
	instant := a.now()
	if instant.IsZero() || !a.grant.ExpiresAt.After(instant) || ctx.Err() != nil {
		return errBroker
	}
	if deadline, ok := ctx.Deadline(); ok && !deadline.After(instant) {
		return errBroker
	}
	return nil
}

func (a *workflowProvenanceAdapter) Attest(parent context.Context, request BrokerProvenanceRequest) (BrokerProvenanceReceipt, error) {
	fail := BrokerProvenanceReceipt{}
	if request != a.grant.Request || a.check(parent) != nil {
		return fail, errBroker
	}
	// The broker supplies a deadline bound to both approvals; this also bounds
	// direct experimental use and never extends the independently reviewed grant.
	ctx, cancel := context.WithDeadline(parent, minTime(a.grant.ExpiresAt, a.now().Add(brokerProvenanceMaximumDuration)))
	defer cancel()
	if a.check(ctx) != nil {
		return fail, errBroker
	}
	token, err := a.readCredential(ctx)
	if err != nil || a.check(ctx) != nil {
		return fail, errBroker
	}
	if _, err = a.observer.observe(ctx, a.grant, token); err != nil || a.check(ctx) != nil {
		return fail, errBroker
	}
	nonce, err := a.nonce()
	nonceHex := hex.EncodeToString(nonce[:])
	if err != nil || nonce == ([16]byte{}) || nonceHex == request.OwnerNonce || a.check(ctx) != nil {
		return fail, errBroker
	}
	receipt := BrokerProvenanceReceipt{
		Version: 1, Algorithm: "ed25519", KeyID: a.keyID,
		ControllerApprovalSHA256: request.ControllerApprovalSHA256,
		Repository:               request.Repository, WorkflowRunID: request.WorkflowRunID,
		WorkflowRef: request.WorkflowRef, WorkflowSHA: request.WorkflowSHA,
		WorkflowPath: request.WorkflowPath, Phase: request.Phase,
		OwnerNonce: request.OwnerNonce, ReceiptNonce: nonceHex,
		Source: workflowProvenanceSource, IssuedAt: a.now().UTC(), ExpiresAt: a.grant.ExpiresAt,
	}
	if a.check(ctx) != nil {
		return fail, errBroker
	}
	signature, err := a.signer.Sign(ctx, receipt.SigningBytes())
	if err != nil || len(signature) != ed25519.SignatureSize || a.check(ctx) != nil {
		return fail, errBroker
	}
	receipt.Signature = base64.RawURLEncoding.EncodeToString(signature)
	if a.Verify(ctx, request, receipt) != nil {
		return fail, errBroker
	}
	return receipt, nil
}

// This verifies adapter policy/shape only. The broker separately verifies the
// signature using its independently pinned root before reading management input.
func (a *workflowProvenanceAdapter) Verify(ctx context.Context, request BrokerProvenanceRequest, receipt BrokerProvenanceReceipt) error {
	if request != a.grant.Request || a.check(ctx) != nil || receipt.KeyID != a.keyID || receipt.ExpiresAt.After(a.grant.ExpiresAt) || receipt.Validate(request, a.now()) != nil {
		return errBroker
	}
	return nil
}
