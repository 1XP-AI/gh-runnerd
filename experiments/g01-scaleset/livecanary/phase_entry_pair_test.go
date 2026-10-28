//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExperimentalPhaseReceiverFullPairedTerminal(t *testing.T) {
	f := newTerminalFixtureWithCreate(t, []string{"create", "start", "inspect", "cleanup"}, nil, func(j *FileJournal, a Approval) { completedHandoffCreate(t, j, a) })
	fixture := newExportedPairedFixture(t, f)
	// Pin a ref in the raw current snapshot without changing immutable authority.
	// Initial fixture approval already owns this same reference in its journal.
	if f.c.a.WorkflowRef == "" {
		t.Fatal("fixture requires ref-backed create")
	}
	j, err := pairedTerminalFixtureAdapters.openController(fixture.files.ControllerStateDirectory, f.c.a)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	approval := controllerHandoffFixtureForApproval(t, f.c.a, pairedPreparationPhase, time.Now().UTC().Truncate(time.Second))
	approval.receipt.ReceiptNonce = strings.Repeat("e", 32)
	approval.receiptBytes = approval.encodeReceipt(t, approval.receipt, true)
	payload, _ := json.Marshal(fixture.credentials)
	cr, ir, root := phasePipes(t, approval, payload, "paired-terminal")
	err = runControllerProcessHandoffPhase(context.Background(), j, approval.snapshot, cr, ir, "paired-terminal", pairedPreparationPhase, &fixture.files, approval.request.Source, approval.root, root, time.Now, func(context.Context, Approval, Credentials) (API, error) {
		t.Fatal("paired path used controller factory")
		return nil, ErrApproval
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.c.j != j {
		t.Fatal("paired receiver reopened controller journal")
	}
	if f.c.acks.Load() != 2 || f.c.acquires.Load() != 1 || f.jit.Load() != 1 || f.creates.Load() != 1 || f.starts.Load() != 1 || f.sessionDeletes.Load() != 1 || f.workerDeletes.Load() != 1 || f.setDeletes.Load() != 1 || f.workerAbsences.Load() != 1 || f.setAbsences.Load() != 1 || f.rosters.Load() != 4 || f.cleanup.Load() != 0 || f.c.forbidden.Load() != 0 {
		t.Fatal("paired terminal effects incomplete")
	}
}

func TestExperimentalPhasePairedFinalBytesSubstitutionBurns(t *testing.T) {
	f := newTerminalFixtureWithCreate(t, []string{"create", "start", "inspect", "cleanup"}, nil, func(j *FileJournal, a Approval) { completedHandoffCreate(t, j, a) })
	fixture := newExportedPairedFixture(t, f)
	j, e := pairedTerminalFixtureAdapters.openController(fixture.files.ControllerStateDirectory, f.c.a)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	receipt := controllerHandoffFixtureForApproval(t, f.c.a, pairedPreparationPhase, time.Now().UTC().Truncate(time.Second))
	receipt.receipt.ReceiptNonce = strings.Repeat("e", 32)
	receipt.receiptBytes = receipt.encodeReceipt(t, receipt.receipt, true)
	original, _ := json.Marshal(fixture.credentials)
	substitute := fixture.credentials
	binding := *substitute.PairedBinding
	binding.WorkerApprovalInode++
	substitute.PairedBinding = &binding
	final, _ := json.Marshal(substitute)
	cr, ir, issuer := phasePipesDelivered(t, receipt, original, final, "paired-terminal")
	e = runControllerProcessHandoffPhase(context.Background(), j, receipt.snapshot, cr, ir, "paired-terminal", pairedPreparationPhase, &fixture.files, receipt.request.Source, receipt.root, issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) {
		t.Fatal("substituted final bytes built factory")
		return nil, ErrApproval
	})
	if e == nil || len(j.Events()) != 8 || f.c.requests.Load() != 0 || f.creates.Load() != 0 || f.jit.Load() != 0 {
		t.Fatal("substitution crossed durable consume/effects boundary", e)
	}
	if j.Close() != nil {
		t.Fatal("close")
	}
	if _, e := preparePairedJournal(fixture.files.ControllerStateDirectory, f.c.a, pairedTerminalFixtureAdapters.openController); e == nil {
		t.Fatal("burned paired slot admitted preparer")
	}
}
