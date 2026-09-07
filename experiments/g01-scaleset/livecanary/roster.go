package livecanary

import (
	"context"
	"slices"
	"time"
)

type rosterCompleteness string

const (
	rosterUnresolved rosterCompleteness = "unresolved"
	rosterComplete   rosterCompleteness = "strict-stable-total-v1"
)

type rosterObservation struct {
	Organization string             `json:"organization"`
	Digest       string             `json:"digest"`
	Count        *int               `json:"count"`
	Pages        int                `json:"pages"`
	Completeness rosterCompleteness `json:"completeness"`
}

func (a *SDKAPI) observeRoster(ctx context.Context) (rosterObservation, error) {
	out := rosterObservation{Completeness: rosterUnresolved}
	if a == nil || ctx == nil || a.rest == nil {
		return out, ErrApproval
	}
	captured := *a
	captured.approval.Phases = slices.Clone(a.approval.Phases)
	captured.approval.ActionsHosts = slices.Clone(a.approval.ActionsHosts)
	out.Organization = captured.approval.Organization
	ctx, cancel, err := captured.observationContext(ctx, false)
	if err != nil {
		return out, err
	}
	defer cancel()
	guard := func() error {
		if ctx.Err() != nil || a.client != captured.client || a.rest != captured.rest || a.baseURL != captured.baseURL || a.credentials != captured.credentials || approvalDigest(a.approval) != approvalDigest(captured.approval) || captured.approval.Validate(time.Now()) != nil || captured.credentials.validate(captured.approval, time.Now()) != nil {
			return ErrApproval
		}
		return nil
	}
	digest, count, pages, err := captured.enumerateRoster(ctx, guard)
	out.Pages = pages
	if err != nil {
		return out, err
	}
	out.Digest, out.Count, out.Completeness = digest, &count, rosterComplete
	return out, nil
}
