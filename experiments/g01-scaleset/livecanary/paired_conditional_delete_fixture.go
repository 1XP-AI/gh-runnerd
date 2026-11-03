//go:build g01_pair_fixture && !g01_worker

package livecanary

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/actions/scaleset"
)

// This fixture-tagged adapter talks only to the generated loopback provider
// fixture, which enforces If-Match atomically. pairedFixtureScaleSetURL rejects
// non-loopback endpoints. The untagged pinned production SDK deliberately does
// not implement ConditionalScaleSetDeleter.
func (a *SDKAPI) PrepareScaleSetDeletion(ctx context.Context, expected ScaleSetDeletionExpectation) (ScaleSetDeletionFence, error) {
	if a == nil || ctx == nil || ctx.Err() != nil || a.rest == nil || a.approval.Validate(time.Now()) != nil || a.credentials.validate(a.approval, time.Now()) != nil {
		return ScaleSetDeletionFence{}, ErrQuarantine
	}
	target, err := pairedFixtureScaleSetURL(a, expected.ScaleSetID)
	if err != nil {
		return ScaleSetDeletionFence{}, ErrQuarantine
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ScaleSetDeletionFence{}, ErrQuarantine
	}
	request.Header.Set("Authorization", "Bearer "+a.credentials.InstallationToken)
	response, err := a.rest.Do(request)
	if err != nil {
		return ScaleSetDeletionFence{}, ErrQuarantine
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ScaleSetDeletionFence{}, ErrQuarantine
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return ScaleSetDeletionFence{}, ErrQuarantine
	}
	var set scaleset.RunnerScaleSet
	if json.Unmarshal(data, &set) != nil || !expected.matches(a.approval, &set) {
		return ScaleSetDeletionFence{}, ErrQuarantine
	}
	etag := response.Header.Get("ETag")
	if etag == "" || strings.ContainsAny(etag, "\r\n\x00") {
		return ScaleSetDeletionFence{}, ErrQuarantine
	}
	return ScaleSetDeletionFence{
		ScaleSetID: expected.ScaleSetID, ScaleSetName: expected.ScaleSetName,
		RunnerGroupID: expected.RunnerGroupID, OwnerNonce: expected.OwnerNonce,
		OwnershipLabel: expected.OwnershipLabel, Statistics: expected.Statistics,
		InventoryDigest: expected.InventoryDigest, ETag: etag,
	}, nil
}

func (a *SDKAPI) DeleteScaleSetIfOwned(ctx context.Context, fence ScaleSetDeletionFence) error {
	expected := ScaleSetDeletionExpectation{
		ScaleSetID: fence.ScaleSetID, ScaleSetName: fence.ScaleSetName,
		RunnerGroupID: fence.RunnerGroupID, OwnerNonce: fence.OwnerNonce,
		OwnershipLabel: fence.OwnershipLabel, Statistics: fence.Statistics,
		InventoryDigest: fence.InventoryDigest,
	}
	if a == nil || ctx == nil || ctx.Err() != nil || a.rest == nil || a.approval.Validate(time.Now()) != nil || a.credentials.validate(a.approval, time.Now()) != nil || !fence.matches(expected) {
		return ErrQuarantine
	}
	target, err := pairedFixtureScaleSetURL(a, fence.ScaleSetID)
	if err != nil {
		return ErrQuarantine
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return ErrQuarantine
	}
	request.Header.Set("Authorization", "Bearer "+a.credentials.InstallationToken)
	request.Header.Set("If-Match", fence.ETag)
	response, err := a.rest.Do(request)
	if err != nil {
		return ErrQuarantine
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return ErrQuarantine
	}
	return nil
}

func pairedFixtureScaleSetURL(a *SDKAPI, id int) (string, error) {
	if a == nil || id <= 0 {
		return "", ErrQuarantine
	}
	u, err := url.Parse(a.baseURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return "", ErrQuarantine
	}
	host := u.Hostname()
	if net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || u.Port() == "" {
		return "", ErrQuarantine
	}
	u.Path = "/tenant/_apis/runtime/runnerscalesets/" + strconv.Itoa(id)
	u.RawQuery = "api-version=6.0-preview"
	return u.String(), nil
}
