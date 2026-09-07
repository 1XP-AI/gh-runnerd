package liveworker

import (
	"context"
	"errors"
	"testing"
)

func TestAuditPR28MissingBridgeMustNotStart(t *testing.T) {
	for _, shape := range []string{"nil", "empty", "bridge"} {
		t.Run(shape, func(t *testing.T) {
			a := approval()
			f := &fakeRuntime{}
			d := Driver{a, &memoryJournal{}, f}
			if d.Run(context.Background(), "create", syntheticJIT) != nil {
				t.Fatal("fixture create failed")
			}
			if shape == "nil" {
				f.container.NetworkSettings.Networks = nil
			}
			if shape == "empty" {
				f.container.NetworkSettings.Networks = map[string]any{}
			}
			if shape == "bridge" {
				f.container.NetworkSettings.Networks = map[string]any{"bridge": map[string]any{}}
			}
			err := d.Run(context.Background(), "start", "")
			if shape == "bridge" {
				if err != nil || f.starts.Load() != 1 {
					t.Fatal("approved bridge fixture refused")
				}
				return
			}
			if !errors.Is(err, ErrUncertain) || f.starts.Load() != 0 {
				t.Fatalf("missing bridge consumed start: error=%v start_calls=%d", err, f.starts.Load())
			}
		})
	}
}
