package livecanary

import (
	"context"
	"errors"
	"testing"
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

func TestCleanupRequiresAtomicOwnershipFreshnessFence(t *testing.T) {
	d, f, _ := created(t)
	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("cleanup without conditional fence was not quarantined: %v", err)
	}
	if f.deleteCalls != 0 {
		t.Fatalf("unsafe cleanup issued %d unconditional deletes", f.deleteCalls)
	}
}
