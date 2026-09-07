package livecanary

import (
	"context"
	"errors"
	"testing"

	"github.com/actions/scaleset"
)

type updateResponseAPI struct {
	*fakeAPI
	disableUpdate bool
}

func (f updateResponseAPI) CreateScaleSet(ctx context.Context, set *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error) {
	result, err := f.fakeAPI.CreateScaleSet(ctx, set)
	if result != nil {
		result.RunnerSetting.DisableUpdate = f.disableUpdate
	}
	return result, err
}

func TestCreateRequestsDisabledRunnerUpdate(t *testing.T) {
	_, f, _ := created(t)
	if !f.set.RunnerSetting.DisableUpdate {
		t.Fatal("pinned experiment permits runner self-update")
	}
}

func TestUnconfirmedUpdateSettingQuarantinesCreate(t *testing.T) {
	f := updateResponseAPI{fakeAPI: &fakeAPI{}, disableUpdate: false}
	j := &memoryJournal{}
	d := Driver{approval(), j, f}
	if !errors.Is(d.Run(context.Background(), "create"), ErrQuarantine) || !replay(j.Events()).uncertain {
		t.Fatal("server omitted update setting without retaining uncertain create")
	}
	if !errors.Is(d.Run(context.Background(), "create"), ErrQuarantine) || f.createCalls != 1 {
		t.Fatal("unconfirmed create was retried")
	}
}

func TestUpdateSettingDriftStopsBeforeSessionOrJIT(t *testing.T) {
	for _, phase := range []string{"before-ack", "after-ack", "before-acquire", "acquire-loss", "jit-loss"} {
		t.Run(phase, func(t *testing.T) {
			d, f, j := created(t)
			f.set.RunnerSetting.DisableUpdate = false
			if !errors.Is(d.Run(context.Background(), phase), ErrQuarantine) {
				t.Fatal("server setting drift admitted session or JIT work")
			}
			for _, event := range j.Events() {
				if event.Kind == "intent" && event.Operation != "create" {
					t.Fatal("setting drift reached a side-effect intent")
				}
			}
			if f.jitCalls != 0 || f.session.ack != 0 || f.session.acquire != 0 || f.session.close != 0 {
				t.Fatal("setting drift changed remote work")
			}
		})
	}
}

func TestUpdateSettingDriftDoesNotBlockSafeEmptyCleanup(t *testing.T) {
	d, f, _ := created(t)
	f.set.RunnerSetting.DisableUpdate = false
	if err := d.Run(context.Background(), "cleanup"); err != nil || f.deleteCalls != 1 {
		t.Fatal("update setting alone prevented otherwise verified empty cleanup")
	}
}
