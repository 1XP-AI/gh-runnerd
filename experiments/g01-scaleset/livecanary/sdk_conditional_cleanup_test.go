//go:build !g01_pair_fixture

package livecanary

import (
	"context"
	"testing"
)

func TestPinnedSDKDoesNotAdvertiseConditionalCleanupWithoutFence(t *testing.T) {
	var api API = &SDKAPI{}
	if _, ok := api.(ConditionalScaleSetDeleter); ok {
		t.Fatal("pinned SDK advertised conditional cleanup without a version/ETag API")
	}
}

func TestSDKAPIDoesNotExposeUnconditionalScaleSetDelete(t *testing.T) {
	type idOnlyDeleter interface {
		DeleteScaleSet(context.Context, int) error
	}
	var api API = &SDKAPI{}
	if _, ok := api.(idOnlyDeleter); ok {
		t.Fatal("SDKAPI exposes the unfenced ID-only Scale Set delete")
	}
}
