package livecanary

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
	"github.com/actions/scaleset"
	"github.com/google/uuid"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// A failed decode must consume the phase, not merely the particular nonce.
func TestExperimentalPhaseFreshNonceBurnsBeforeInput(t *testing.T) {
	f := newControllerHandoffFixture(t, "before-ack")
	j, err := openTestJournal(t, privateDir(t), f.approval)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	reads := 0
	for _, nonce := range []string{strings.Repeat("d", 32), strings.Repeat("e", 32)} {
		r := f.receipt
		r.ReceiptNonce = nonce
		_, _, err := consumeControllerHandoffWithProof(context.Background(), j, f.snapshot, "before-ack", f.request.Source, newControllerHandoffMemorySource(f.encodeReceipt(t, r, true)), f.root, time.Now, func(context.Context) ([]byte, error) { reads++; return nil, ErrApproval })
		if err == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
	if reads != 1 || len(j.Events()) != 1 {
		t.Fatalf("phase retried credential boundary: reads=%d consumes=%d", reads, len(j.Events()))
	}
}

func phasePipes(t *testing.T, f controllerHandoffFixture, payload []byte, mode string) (*os.File, *os.File, handoff.IssuerRoot) {
	return phasePipesDelivered(t, f, payload, payload, mode)
}
func phasePipesDelivered(t *testing.T, f controllerHandoffFixture, payload, delivered []byte, mode string, deadline ...time.Time) (*os.File, *os.File, handoff.IssuerRoot) {
	t.Helper()
	b, root, key := processBinding(t, f, payload)
	phase := handoff.PhaseBinding{BindingClaims: b.BindingClaims, Mode: mode}
	phase.Version = 2
	phase.Kind = handoff.PhaseBindingKind
	phase.Audience = handoff.PhaseAudience(mode, f.request.Phase)
	phase.ExpiresAt = time.Now().Add(5 * time.Minute)
	if len(deadline) == 1 && deadline[0].Before(phase.ExpiresAt) {
		phase.ExpiresAt = deadline[0]
	}
	phase.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, phase.SigningBytes()))
	raw, _ := json.Marshal(phase)
	frame, e := handoff.EncodeControl(f.receiptBytes, raw)
	if e != nil {
		t.Fatal(e)
	}
	cr, cw, _ := os.Pipe()
	ir, iw, _ := os.Pipe()
	done := make(chan struct{})
	go func() { defer close(done); cw.Write(frame); cw.Close(); iw.Write(delivered); iw.Close() }()
	t.Cleanup(func() { cr.Close(); ir.Close(); cw.Close(); iw.Close(); <-done })
	return cr, ir, root
}
func TestExperimentalPhaseReceiverFaultAndInspect(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	j, e := openTestJournal(t, privateDir(t), a)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	_, fake := completedHandoffCreate(t, j, a)
	fake.session = &fakeSession{session: scaleset.RunnerScaleSetSession{SessionID: uuid.New(), Statistics: &scaleset.RunnerScaleSetStatistic{}}, message: &scaleset.RunnerScaleSetMessage{MessageID: 9, Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}, JobAvailableMessages: []*scaleset.JobAvailable{{JobMessageBase: scaleset.JobMessageBase{RunnerRequestID: 42, WorkflowRunID: a.WorkflowRunID, OwnerName: a.Organization, RepositoryName: a.Repository}}}}}
	for _, phase := range []string{"before-ack", "inspect"} {
		f := controllerHandoffFixtureForApproval(t, a, phase, time.Now().UTC().Truncate(time.Second))
		// Separate transport nonces, while the exact phase slots remain singleton.
		f.receipt.ReceiptNonce = strings.Repeat(map[string]string{"before-ack": "e", "inspect": "f"}[phase], 32)
		f.receiptBytes = f.encodeReceipt(t, f.receipt, true)
		cr, ir, root := phasePipes(t, f, handoffCredentialBytes(t, a), "controller")
		factories := 0
		e = runControllerProcessHandoffPhase(context.Background(), j, f.snapshot, cr, ir, "controller", phase, nil, f.request.Source, f.root, root, time.Now, func(context.Context, Approval, Credentials) (API, error) {
			factories++
			return completedCreateAPI{fake}, nil
		})
		if e != nil || factories != 1 {
			t.Fatalf("%s receiver: %v factories=%d", phase, e, factories)
		}
	}
	barriers := 0
	consumed := 0
	for _, e := range j.Events() {
		if e.Kind == "barrier" && e.Operation == "before-ack" {
			barriers++
		}
		if e.ControllerHandoff != nil {
			consumed++
		}
	}
	if barriers != 1 || consumed != 3 || fake.createCalls != 1 || fake.session.ack != 0 || fake.session.acquire != 0 {
		t.Fatalf("wrong effects: barriers=%d consumed=%d", barriers, consumed)
	}
}

func TestExperimentalPhaseInvalidInputBurnsReopenAndPreparer(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	dir := privateDir(t)
	j, e := openTestJournal(t, dir, a)
	if e != nil {
		t.Fatal(e)
	}
	completedHandoffCreate(t, j, a)
	f := controllerHandoffFixtureForApproval(t, a, "inspect", time.Now().UTC().Truncate(time.Second))
	f.receipt.ReceiptNonce = strings.Repeat("e", 32)
	f.receiptBytes = f.encodeReceipt(t, f.receipt, true)
	factories := 0
	run := func(j *FileJournal, f controllerHandoffFixture) (error, []byte) {
		cr, ir, issuer := phasePipes(t, f, []byte("malformed"), "controller")
		unread := processUnreadCopy(t, ir)
		e := runControllerProcessHandoffPhase(context.Background(), j, f.snapshot, cr, ir, "controller", "inspect", nil, f.request.Source, f.root, issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) { factories++; return nil, ErrApproval })
		data, _ := io.ReadAll(unread)
		return e, data
	}
	if e, _ := run(j, f); e == nil || factories != 0 || len(j.Events()) != 8 {
		t.Fatal("invalid input did not burn durable phase", e)
	}
	if j.Close() != nil {
		t.Fatal("close")
	}
	if _, e := prepareJournal(dir, a, "inspect", func(path string, a Approval) (*FileJournal, error) { return openTestJournal(t, path, a) }); e == nil {
		t.Fatal("burned phase authorized preparer")
	}
	j, e = openTestJournal(t, dir, a)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	f.receipt.ReceiptNonce = strings.Repeat("f", 32)
	f.receiptBytes = f.encodeReceipt(t, f.receipt, true)
	if e, unread := run(j, f); e == nil || string(unread) != "malformed" || factories != 0 || len(j.Events()) != 8 {
		t.Fatal("fresh nonce/reopen retried credential input", e)
	}
}

func TestExperimentalPhaseOriginalParentCanceledBeforeFactory(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	j, e := openTestJournal(t, privateDir(t), a)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	completedHandoffCreate(t, j, a)
	f := controllerHandoffFixtureForApproval(t, a, "inspect", time.Now().UTC().Truncate(time.Second))
	f.receipt.ReceiptNonce = strings.Repeat("e", 32)
	f.receiptBytes = f.encodeReceipt(t, f.receipt, true)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	e = runControllerHandoffPhase(parent, j, f.snapshot, f.receiptBytes, "inspect", f.request.Source, f.root, time.Now, func(context.Context) ([]byte, error) { cancel(); return handoffCredentialBytes(t, a), nil }, func(context.Context, Approval, Credentials) (API, error) { calls++; return nil, ErrApproval }, nil)
	if e == nil || calls != 0 || len(j.Events()) != 8 {
		t.Fatal("original cancellation crossed factory", e, calls)
	}
}
