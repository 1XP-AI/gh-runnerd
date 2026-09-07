//go:build darwin && cgo && g02runtime

package main

import "testing"

func TestUnknownServiceCleanupPreservesRecoveryInventory(t *testing.T) {
	locked, deleted, removed := false, false, false
	complete := finishOwned(false, func() { locked = true }, func() bool { deleted = true; return true }, func() bool { removed = true; return true })
	if complete || deleted || removed || !locked {
		t.Fatalf("unknown service erased recovery: complete=%t keychain_deleted=%t inventory_removed=%t locked=%t", complete, deleted, removed, locked)
	}
}
func TestKeychainCleanupFailurePreservesRecoveryInventory(t *testing.T) {
	removed := false
	complete := finishOwned(true, func() {}, func() bool { return false }, func() bool { removed = true; return true })
	if complete || removed {
		t.Fatal("keychain failure erased recovery inventory")
	}
}
func TestProvenServiceAbsencePermitsOwnedCleanupInOrder(t *testing.T) {
	var calls []string
	complete := finishOwned(true, func() { calls = append(calls, "lock") }, func() bool { calls = append(calls, "keychain"); return true }, func() bool { calls = append(calls, "inventory"); return true })
	if !complete || len(calls) != 2 || calls[0] != "keychain" || calls[1] != "inventory" {
		t.Fatal("incorrect owned cleanup order")
	}
}

func TestLockedCredentialDenialExcludesUnknownFailures(t *testing.T) {
	for _, status := range []int{-25293, -25308} {
		if !knownCredentialDenial(status) {
			t.Errorf("documented credential denial %d rejected", status)
		}
	}
	for _, status := range []int{0, -1, -25300, -26275} {
		if knownCredentialDenial(status) {
			t.Errorf("unrelated status %d misclassified as locked denial", status)
		}
	}
}
