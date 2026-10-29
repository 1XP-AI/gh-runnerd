package livecanary

import (
	"github.com/actions/scaleset"
	"log/slog"
	"net/http"
)

// A fresh dedicated client binds the fixed physical gate to this invocation.
// Context stripping never converts a replacement request into unmarked traffic.
func (a *SDKAPI) replacementAPI(s *replacementScope) (*SDKAPI, error) {
	if a == nil || a.physical == nil || s == nil {
		return nil, ErrApproval
	}
	physical := a.physical.Clone()
	// net/http can retry a failed idempotent request below RoundTrip when a
	// connection was reused. Fresh HTTP/1 connections keep the physical budget
	// visible to the one-shot gate, including failures on first write.
	physical.DisableKeepAlives = true
	physical.MaxConnsPerHost = 1
	httpClient := &http.Client{Transport: withOwnedResponseBudget(physical, s), Timeout: operationTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	options := []scaleset.HTTPOption{scaleset.WithHTTPClient(httpClient), scaleset.WithRetry(scaleset.RetryConfig{Max: 0}), scaleset.WithLogger(slog.New(slog.DiscardHandler))}
	config := a.baseURL + "/" + a.approval.Organization
	if a.baseURL == "https://api.github.com" {
		config = "https://github.com/" + a.approval.Organization
	}
	client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: config, PersonalAccessToken: a.credentials.InstallationToken}, options...)
	if err != nil {
		physical.CloseIdleConnections()
		return nil, ErrApproval
	}
	return &SDKAPI{physical: physical, client: client, rest: httpClient, baseURL: a.baseURL, approval: a.approval, credentials: a.credentials, options: options}, nil
}
