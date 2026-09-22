package livecanary

import (
	"context"

	"github.com/actions/scaleset"
)

// ScaleSetDeletionExpectation is the immutable identity, final ownership proof
// and fresh runner inventory that a cleanup adapter must bind to its
// conditional delete. Statistics is the exact all-zero predicate captured by
// the final owned read. OwnerNonce is kept separate from ScaleSetName so an
// adapter cannot silently reduce the owner check to a provider name comparison.
type ScaleSetDeletionExpectation struct {
	ScaleSetID      int
	ScaleSetName    string
	RunnerGroupID   int
	OwnerNonce      string
	OwnershipLabel  string
	Statistics      scaleset.RunnerScaleSetStatistic
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
	OwnershipLabel  string
	Statistics      scaleset.RunnerScaleSetStatistic
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
	if set == nil || e.ScaleSetID <= 0 || e.ScaleSetID != set.ID || e.ScaleSetName == "" || e.ScaleSetName != set.Name || e.ScaleSetName != a.setName() || e.RunnerGroupID <= 0 || e.RunnerGroupID != set.RunnerGroupID || e.RunnerGroupID != a.RunnerGroupID || e.OwnerNonce == "" || e.OwnerNonce != a.OwnerNonce || e.OwnershipLabel == "" || e.OwnershipLabel != e.ScaleSetName || e.Statistics != (scaleset.RunnerScaleSetStatistic{}) || set.Statistics == nil || *set.Statistics != e.Statistics || e.InventoryDigest == "" {
		return false
	}
	return hasScaleSetLabel(set, e.OwnershipLabel)
}

func hasScaleSetLabel(set *scaleset.RunnerScaleSet, name string) bool {
	if set == nil || name == "" {
		return false
	}
	for _, label := range set.Labels {
		if label.Name == name {
			return true
		}
	}
	return false
}

func (f ScaleSetDeletionFence) matches(e ScaleSetDeletionExpectation) bool {
	return e.ScaleSetID > 0 && e.ScaleSetName != "" && e.RunnerGroupID > 0 && e.OwnerNonce != "" && e.OwnershipLabel == e.ScaleSetName && e.Statistics == (scaleset.RunnerScaleSetStatistic{}) && e.InventoryDigest != "" && f.ScaleSetID == e.ScaleSetID && f.ScaleSetName == e.ScaleSetName && f.RunnerGroupID == e.RunnerGroupID && f.OwnerNonce == e.OwnerNonce && f.OwnershipLabel == e.OwnershipLabel && f.Statistics == e.Statistics && f.InventoryDigest == e.InventoryDigest && (f.Version != "" || f.ETag != "")
}
