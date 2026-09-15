package livecanary

import (
	"context"

	"github.com/actions/scaleset"
)

// ScaleSetDeletionExpectation is the immutable identity and fresh runner
// inventory that a cleanup adapter must bind to its conditional delete.
// OwnerNonce is kept separate from ScaleSetName so an adapter cannot silently
// reduce the owner check to a provider name comparison.
type ScaleSetDeletionExpectation struct {
	ScaleSetID      int
	ScaleSetName    string
	RunnerGroupID   int
	OwnerNonce      string
	InventoryDigest string
}

// ScaleSetDeletionFence is a provider version/ETag (or equivalent opaque
// revision) captured by the conditional-delete adapter. A fence without a
// freshness value is not a deletion capability.
type ScaleSetDeletionFence struct {
	ScaleSetID      int
	ScaleSetName    string
	RunnerGroupID   int
	OwnerNonce      string
	InventoryDigest string
	Version         string
	ETag            string
}

// ConditionalScaleSetDeleter is the only cleanup deletion contract. Prepare
// must read the current provider object and return a revision that is fresh for
// the exact expectation; DeleteScaleSetIfOwned must send that revision as a
// server-side condition in the same operation as deletion. Implementations
// cannot fall back to API.DeleteScaleSet, an observe-then-delete sequence, or
// an inventory-after-delete check. If the provider has no conditional version,
// ETag, or equivalent atomic capability, the adapter must be unavailable.
type ConditionalScaleSetDeleter interface {
	PrepareScaleSetDeletion(context.Context, ScaleSetDeletionExpectation) (ScaleSetDeletionFence, error)
	DeleteScaleSetIfOwned(context.Context, ScaleSetDeletionFence) error
}

func (e ScaleSetDeletionExpectation) matches(a Approval, set *scaleset.RunnerScaleSet) bool {
	return set != nil && e.ScaleSetID > 0 && e.ScaleSetID == set.ID && e.ScaleSetName != "" && e.ScaleSetName == set.Name && e.ScaleSetName == a.setName() && e.RunnerGroupID > 0 && e.RunnerGroupID == set.RunnerGroupID && e.RunnerGroupID == a.RunnerGroupID && e.OwnerNonce != "" && e.OwnerNonce == a.OwnerNonce && e.InventoryDigest != ""
}

func (f ScaleSetDeletionFence) matches(e ScaleSetDeletionExpectation) bool {
	return f.ScaleSetID == e.ScaleSetID && f.ScaleSetName == e.ScaleSetName && f.RunnerGroupID == e.RunnerGroupID && f.OwnerNonce == e.OwnerNonce && f.InventoryDigest == e.InventoryDigest && (f.Version != "" || f.ETag != "")
}
