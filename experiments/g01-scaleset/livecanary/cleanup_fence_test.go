package livecanary

import (
	"context"
	"errors"
	"testing"

	"github.com/actions/scaleset"
)

// cleanupFenceAPI models the adapter contract that a real provider must satisfy.
// Its conditional operation compares the captured version and owner fields in
// the same critical section as deletion; the legacy DeleteScaleSet counter is
// intentionally never used by these tests.
type cleanupFenceAPI struct {
	*fakeAPI
	approval            Approval
	currentVersion      string
	currentETag         string
	replaceBeforeDelete bool
	cancelDuringPrepare context.CancelFunc
	prepareCalls        int
	conditionalCalls    int
	mutateFence         func(*ScaleSetDeletionFence)
	mutateBeforePrepare func(*scaleset.RunnerScaleSet)
	deleteErr           error
	gotExpectation      ScaleSetDeletionExpectation
	gotFence            ScaleSetDeletionFence
}

type authorizationCountingJournal struct {
	Journal
	authorizeCalls int
}

func (j *authorizationCountingJournal) authorize(a Approval) (func(), error) {
	j.authorizeCalls++
	return j.Journal.authorize(a)
}

func (a *cleanupFenceAPI) PrepareScaleSetDeletion(ctx context.Context, expected ScaleSetDeletionExpectation) (ScaleSetDeletionFence, error) {
	a.prepareCalls++
	a.gotExpectation = expected
	if a.mutateBeforePrepare != nil {
		a.mutateBeforePrepare(a.fakeAPI.set)
	}
	current, err := a.fakeAPI.GetScaleSet(ctx, expected.ScaleSetID)
	if err != nil || ctx.Err() != nil || !expected.matches(a.approval, current) {
		return ScaleSetDeletionFence{}, ErrQuarantine
	}
	fence := ScaleSetDeletionFence{
		ScaleSetID:      expected.ScaleSetID,
		ScaleSetName:    expected.ScaleSetName,
		RunnerGroupID:   expected.RunnerGroupID,
		OwnerNonce:      expected.OwnerNonce,
		OwnershipLabel:  expected.OwnershipLabel,
		Statistics:      expected.Statistics,
		InventoryDigest: expected.InventoryDigest,
		Version:         a.currentVersion,
		ETag:            a.currentETag,
	}
	if a.mutateFence != nil {
		a.mutateFence(&fence)
	}
	if a.cancelDuringPrepare != nil {
		a.cancelDuringPrepare()
	}
	return fence, nil
}

func (a *cleanupFenceAPI) DeleteScaleSetIfOwned(_ context.Context, fence ScaleSetDeletionFence) error {
	a.conditionalCalls++
	a.gotFence = fence
	if a.replaceBeforeDelete {
		a.currentVersion = "v2"
		a.currentETag = "etag-v2"
		a.fakeAPI.set.Name = "foreign-scale-set"
	}
	if a.deleteErr != nil {
		return a.deleteErr
	}
	revisionMatches := fence.Version != "" && fence.Version == a.currentVersion || fence.ETag != "" && fence.ETag == a.currentETag
	if a.fakeAPI.set == nil || fence.ScaleSetID != a.fakeAPI.set.ID || fence.ScaleSetName != a.fakeAPI.set.Name || fence.RunnerGroupID != a.fakeAPI.set.RunnerGroupID || fence.OwnerNonce != a.approval.OwnerNonce || fence.OwnershipLabel != a.approval.setName() || fence.Statistics != (scaleset.RunnerScaleSetStatistic{}) || a.fakeAPI.set.Statistics == nil || *a.fakeAPI.set.Statistics != fence.Statistics || !hasScaleSetLabel(a.fakeAPI.set, fence.OwnershipLabel) || !revisionMatches {
		return ErrQuarantine
	}
	return nil
}

type cancellationAfterPrepareCleanupAPI struct {
	*cleanupFenceAPI
	cancel context.CancelFunc
}

func (a *cancellationAfterPrepareCleanupAPI) PrepareScaleSetDeletion(ctx context.Context, expected ScaleSetDeletionExpectation) (ScaleSetDeletionFence, error) {
	fence, err := a.cleanupFenceAPI.PrepareScaleSetDeletion(ctx, expected)
	a.cancel()
	return fence, err
}

func TestCleanupConditionalFenceRechecksCancellationBeforeDelete(t *testing.T) {
	d, f, j := created(t)
	ctx, cancel := context.WithCancel(context.Background())
	base := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval, currentVersion: "v1"}
	adapter := &cancellationAfterPrepareCleanupAPI{cleanupFenceAPI: base, cancel: cancel}
	d.API = adapter

	if err := d.Run(ctx, "cleanup"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("cancellation after conditional prepare = %v, want quarantine", err)
	}
	if adapter.prepareCalls != 1 || adapter.conditionalCalls != 0 || f.deleteCalls != 0 {
		t.Fatalf("cancellation after prepare calls = prepare %d conditional %d unconditional %d, want 1/0/0", adapter.prepareCalls, adapter.conditionalCalls, f.deleteCalls)
	}
	if !replay(j.Events()).uncertain {
		t.Fatal("cancellation after prepare did not retain an uncertain deletion reservation")
	}
}

func TestCleanupConditionalFenceAcceptsExactOwnerAndFreshnessMatch(t *testing.T) {
	d, f, j := created(t)
	adapter := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval, currentVersion: "v1"}
	d.API = adapter

	if err := d.Run(context.Background(), "cleanup"); err != nil {
		t.Fatalf("exact conditional cleanup: %v", err)
	}
	if adapter.prepareCalls != 1 || adapter.conditionalCalls != 1 {
		t.Fatalf("conditional cleanup calls = prepare %d delete %d, want one each", adapter.prepareCalls, adapter.conditionalCalls)
	}
	if f.deleteCalls != 0 {
		t.Fatalf("conditional cleanup called legacy unconditional delete %d times", f.deleteCalls)
	}
	want := ScaleSetDeletionExpectation{ScaleSetID: 7, ScaleSetName: d.Approval.setName(), RunnerGroupID: d.Approval.RunnerGroupID, OwnerNonce: d.Approval.OwnerNonce, OwnershipLabel: d.Approval.setName(), Statistics: scaleset.RunnerScaleSetStatistic{}, InventoryDigest: "fixture-inventory"}
	if adapter.gotExpectation != want || adapter.gotFence.Version != "v1" || adapter.gotFence.ScaleSetID != want.ScaleSetID || adapter.gotFence.ScaleSetName != want.ScaleSetName || adapter.gotFence.RunnerGroupID != want.RunnerGroupID || adapter.gotFence.OwnerNonce != want.OwnerNonce || adapter.gotFence.OwnershipLabel != want.OwnershipLabel || adapter.gotFence.Statistics != want.Statistics || adapter.gotFence.InventoryDigest != want.InventoryDigest {
		t.Fatalf("conditional cleanup fence = expectation=%+v fence=%+v, want %+v and exact v1 fence", adapter.gotExpectation, adapter.gotFence, want)
	}
	if !replay(j.Events()).deleted {
		t.Fatal("successful conditional delete was not durably recorded")
	}
}

func TestCleanupPreparationRereadsFinalOwnershipAndZeroStatistics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*scaleset.RunnerScaleSet)
	}{
		{name: "ownership label", mutate: func(set *scaleset.RunnerScaleSet) { set.Labels = nil }},
		{name: "idle statistics", mutate: func(set *scaleset.RunnerScaleSet) {
			set.Statistics = &scaleset.RunnerScaleSetStatistic{TotalIdleRunners: 1}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, f, j := created(t)
			adapter := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval, currentVersion: "v1", mutateBeforePrepare: tc.mutate}
			d.API = adapter

			if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) {
				t.Fatalf("cleanup after final predicate changed = %v, want quarantine", err)
			}
			if adapter.prepareCalls != 1 || adapter.conditionalCalls != 0 || f.deleteCalls != 0 || !replay(j.Events()).uncertain {
				t.Fatalf("final predicate change crossed deletion fence: prepare=%d conditional=%d unconditional=%d uncertain=%t", adapter.prepareCalls, adapter.conditionalCalls, f.deleteCalls, replay(j.Events()).uncertain)
			}
		})
	}
}

func TestCleanupMissingConditionalDeleterStopsBeforeJournalAuthorization(t *testing.T) {
	d, f, j := created(t)
	counting := &authorizationCountingJournal{Journal: j}
	d.Journal = counting
	before := len(j.Events())

	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("missing conditional deleter cleanup = %v, want quarantine", err)
	}
	if counting.authorizeCalls != 0 || len(j.Events()) != before || f.deleteCalls != 0 {
		t.Fatalf("missing conditional deleter changed journal/authorization: authorizations=%d events=%d before=%d unconditional=%d", counting.authorizeCalls, len(j.Events()), before, f.deleteCalls)
	}
}

func TestCleanupConditionalFenceRejectsConcurrentReplacement(t *testing.T) {
	d, f, _ := created(t)
	adapter := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval, currentVersion: "v1", replaceBeforeDelete: true}
	d.API = adapter

	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("concurrent replacement cleanup = %v, want quarantine", err)
	}
	if adapter.prepareCalls != 1 || adapter.conditionalCalls != 1 || f.deleteCalls != 0 {
		t.Fatalf("replacement cleanup calls = prepare %d conditional %d unconditional %d, want 1/1/0", adapter.prepareCalls, adapter.conditionalCalls, f.deleteCalls)
	}
}

func TestCleanupConditionalFenceAcceptsETagFreshness(t *testing.T) {
	d, f, _ := created(t)
	adapter := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval, currentETag: "etag-v1"}
	d.API = adapter

	if err := d.Run(context.Background(), "cleanup"); err != nil {
		t.Fatalf("exact ETag conditional cleanup: %v", err)
	}
	if adapter.prepareCalls != 1 || adapter.conditionalCalls != 1 || adapter.gotFence.Version != "" || adapter.gotFence.ETag != "etag-v1" || f.deleteCalls != 0 {
		t.Fatalf("ETag cleanup state = prepare %d conditional %d version %q etag %q unconditional %d, want 1/1/empty/etag-v1/0", adapter.prepareCalls, adapter.conditionalCalls, adapter.gotFence.Version, adapter.gotFence.ETag, f.deleteCalls)
	}
}

func TestCleanupConditionalFenceRejectsMissingFreshness(t *testing.T) {
	d, f, j := created(t)
	adapter := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval}
	d.API = adapter

	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("missing freshness cleanup = %v, want quarantine", err)
	}
	if adapter.prepareCalls != 1 || adapter.conditionalCalls != 0 || f.deleteCalls != 0 || !replay(j.Events()).uncertain {
		t.Fatalf("missing freshness state = prepare %d conditional %d unconditional %d uncertain %t, want 1/0/0/true", adapter.prepareCalls, adapter.conditionalCalls, f.deleteCalls, replay(j.Events()).uncertain)
	}
	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) || adapter.prepareCalls != 1 || f.deleteCalls != 0 {
		t.Fatalf("missing freshness cleanup retried: error=%v prepare=%d unconditional=%d", err, adapter.prepareCalls, f.deleteCalls)
	}
}

func TestCleanupConditionalFenceRejectsMismatchedOwnerFence(t *testing.T) {
	d, f, j := created(t)
	adapter := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval, currentVersion: "v1", mutateFence: func(fence *ScaleSetDeletionFence) {
		fence.OwnerNonce = "fedcba9876543210fedcba9876543210"
	}}
	d.API = adapter

	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("mismatched owner cleanup = %v, want quarantine", err)
	}
	if adapter.prepareCalls != 1 || adapter.conditionalCalls != 0 || f.deleteCalls != 0 || !replay(j.Events()).uncertain {
		t.Fatalf("mismatched owner state = prepare %d conditional %d unconditional %d uncertain %t, want 1/0/0/true", adapter.prepareCalls, adapter.conditionalCalls, f.deleteCalls, replay(j.Events()).uncertain)
	}
}

func TestCleanupConditionalFenceFailureRetainsUnknownAndCannotRetry(t *testing.T) {
	d, f, j := created(t)
	adapter := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval, currentVersion: "v1", deleteErr: errors.New("synthetic conditional refusal")}
	d.API = adapter

	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("conditional failure cleanup = %v, want quarantine", err)
	}
	if adapter.prepareCalls != 1 || adapter.conditionalCalls != 1 || f.deleteCalls != 0 || !replay(j.Events()).uncertain {
		t.Fatalf("conditional failure state = prepare %d conditional %d unconditional %d uncertain %t, want 1/1/0/true", adapter.prepareCalls, adapter.conditionalCalls, f.deleteCalls, replay(j.Events()).uncertain)
	}
	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) || adapter.prepareCalls != 1 || adapter.conditionalCalls != 1 || f.deleteCalls != 0 {
		t.Fatalf("conditional failure cleanup retried: error=%v prepare=%d conditional=%d unconditional=%d", err, adapter.prepareCalls, adapter.conditionalCalls, f.deleteCalls)
	}
}

func TestCleanupCancellationDuringPrepareDoesNotDeleteAndRetainsUncertainty(t *testing.T) {
	d, f, j := created(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := &cleanupFenceAPI{fakeAPI: f, approval: d.Approval, currentVersion: "v1", cancelDuringPrepare: cancel}
	d.API = adapter

	if err := d.Run(ctx, "cleanup"); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("canceled prepare cleanup = %v, want quarantine", err)
	}
	if adapter.prepareCalls != 1 || adapter.conditionalCalls != 0 || f.deleteCalls != 0 || !replay(j.Events()).uncertain {
		t.Fatalf("canceled prepare state = prepare %d conditional %d unconditional %d uncertain %t, want 1/0/0/true", adapter.prepareCalls, adapter.conditionalCalls, f.deleteCalls, replay(j.Events()).uncertain)
	}
	if err := d.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) || adapter.prepareCalls != 1 || adapter.conditionalCalls != 0 || f.deleteCalls != 0 || !replay(j.Events()).uncertain {
		t.Fatalf("canceled prepare cleanup retried or lost uncertainty: error=%v prepare=%d conditional=%d unconditional=%d uncertain=%t", err, adapter.prepareCalls, adapter.conditionalCalls, f.deleteCalls, replay(j.Events()).uncertain)
	}
}

func TestPinnedSDKDoesNotAdvertiseConditionalCleanupWithoutFence(t *testing.T) {
	var api API = &SDKAPI{}
	if _, ok := api.(ConditionalScaleSetDeleter); ok {
		t.Fatal("pinned SDK advertised conditional cleanup without a version/ETag API")
	}
}
