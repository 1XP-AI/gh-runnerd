package livecanary

import (
	"context"
	"errors"
	"testing"

	"github.com/actions/scaleset"
)

func TestInspectConsumesSingleDurablePhaseSlot(t *testing.T) {
	d, _, j := created(t)
	if err := d.Run(context.Background(), "inspect"); err != nil {
		t.Fatalf("first inspect: %v", err)
	}
	if err := d.Run(context.Background(), "inspect"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("repeated inspect was not quarantined: %v", err)
	}
	count := 0
	for _, event := range j.Events() {
		if event.Kind == "phase" && event.Operation == "inspect" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("inspect phase slot count = %d, want 1", count)
	}
}

func TestInspectSlotIsConsumedBeforeFailedPreflight(t *testing.T) {
	d, f, j := created(t)
	f.preflightErr = errors.New("synthetic preflight refusal")
	if err := d.Run(context.Background(), "inspect"); !errors.Is(err, ErrApproval) {
		t.Fatalf("failed inspect preflight returned %v", err)
	}
	f.preflightErr = nil
	if err := d.Run(context.Background(), "inspect"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("inspect retried after failed preflight: %v", err)
	}
	count := 0
	for _, event := range j.Events() {
		if event.Kind == "phase" && event.Operation == "inspect" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("failed inspect phase slot count = %d, want 1", count)
	}
}

func TestInspectAllowsOneRecoveryReadAfterDurableDelete(t *testing.T) {
	d, f, j := created(t)
	if err := j.Append(Event{Kind: "intent", Operation: "delete"}); err != nil {
		t.Fatal("durable delete intent: ", err)
	}
	if err := j.Append(Event{Kind: "result", Operation: "delete"}); err != nil {
		t.Fatal("durable delete result: ", err)
	}
	// The remote set is absent, so recovery must perform the read and retain
	// the unresolved result rather than treating durable deletion as proof.
	f.set = nil
	if err := d.Run(context.Background(), "inspect"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("post-delete recovery inspect returned %v, want quarantine after read", err)
	}
	events := j.Events()
	inspectPhases, ownedReads := 0, 0
	for _, event := range events {
		if event.Kind == "phase" && event.Operation == "inspect" {
			inspectPhases++
		}
		if event.Kind == "result" && event.Operation == "observe-owned" {
			ownedReads++
		}
	}
	if inspectPhases != 1 || ownedReads != 1 {
		t.Fatalf("post-delete recovery did not consume one inspect/read slot: phases=%d reads=%d events=%+v", inspectPhases, ownedReads, events)
	}
	got := len(events)
	if err := d.Run(context.Background(), "inspect"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("repeated post-delete inspect returned %v, want quarantine", err)
	}
	if len(j.Events()) != got {
		t.Fatal("repeated post-delete inspect appended durable evidence or retried the read")
	}
}

func TestInspectQuarantinesSurvivingSetAfterDurableDelete(t *testing.T) {
	d, f, j := created(t)
	adapter := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval, currentVersion: "v1"}
	d.API = adapter
	if err := d.Run(context.Background(), "cleanup"); err != nil {
		t.Fatalf("fenced cleanup: %v", err)
	}
	if f.set == nil || f.set.Statistics == nil || *f.set.Statistics != (scaleset.RunnerScaleSetStatistic{}) {
		t.Fatalf("fenced cleanup did not leave the zero-stat scale set for inspection: %+v", f.set)
	}
	ownedReadsBefore := 0
	for _, event := range j.Events() {
		if event.Kind == "result" && event.Operation == "observe-owned" {
			ownedReadsBefore++
		}
	}

	if err := d.Run(context.Background(), "inspect"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("inspect accepted a surviving set after durable delete: %v", err)
	}

	ownedReads, inspectObservations := 0, 0
	for _, event := range j.Events() {
		if event.Kind == "result" && event.Operation == "observe-owned" {
			ownedReads++
		}
		if event.Kind == "observation" && event.Operation == "inspect" {
			inspectObservations++
		}
	}
	if ownedReads != ownedReadsBefore+1 {
		t.Fatalf("surviving-set inspect did not preserve its owned read evidence: reads=%d before=%d", ownedReads, ownedReadsBefore)
	}
	if inspectObservations != 0 {
		t.Fatalf("surviving set was recorded as a safe inspection: observations=%d", inspectObservations)
	}
}

func TestCleanupRequiresAtomicOwnershipFreshnessFence(t *testing.T) {
	d, f, _ := created(t)
	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("cleanup without conditional fence was not quarantined: %v", err)
	}
	if f.deleteCalls != 0 {
		t.Fatalf("unsafe cleanup issued %d unconditional deletes", f.deleteCalls)
	}
}
