//go:build g01_pair_fixture

package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// WorkflowGrantForFixture is independent synthetic operator policy. Full ref and
// phase are grant authority, not claims about GitHub workflow inputs.
type WorkflowGrantForFixture struct {
	Request                      handoff.Request
	OrganizationID, RepositoryID int64
	ExpiresAt                    time.Time
}

type WorkflowRepositoryForFixture struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	Fork     bool   `json:"fork"`
	Owner    struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"owner"`
}

// WorkflowRunForFixture contains only caller-supplied synthetic response facts.
// Neither this response nor the candidate is used to construct the grant.
type WorkflowRunForFixture struct {
	ID             int64                        `json:"id"`
	HeadSHA        string                       `json:"head_sha"`
	HeadBranch     string                       `json:"head_branch"`
	Event          string                       `json:"event"`
	Path           string                       `json:"path"`
	RunAttempt     int                          `json:"run_attempt"`
	Repository     WorkflowRepositoryForFixture `json:"repository"`
	HeadRepository WorkflowRepositoryForFixture `json:"head_repository"`
}

type WorkflowEffectsForFixture struct{ ObservationGETs, Signatures int }

type workflowMemoryTransport struct {
	grant   reviewedWorkflowGrant
	body    []byte
	effects *WorkflowEffectsForFixture
}

func (t workflowMemoryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Context().Err() != nil || r.Method != "GET" || r.URL.String() != "https://api.github.com"+workflowRunPath(t.grant.Request.Repository, t.grant.Request.WorkflowRunID) || r.Header.Get("Authorization") != "Bearer synthetic-observation-token" || r.Header.Get("Accept-Encoding") != "identity" {
		return nil, errBroker
	}
	t.effects.ObservationGETs++
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(t.body)), ContentLength: int64(len(t.body))}, nil
}

type workflowMemorySigner struct {
	key     ed25519.PrivateKey
	effects *WorkflowEffectsForFixture
}

func (s workflowMemorySigner) Sign(ctx context.Context, b []byte) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, errBroker
	}
	s.effects.Signatures++
	return ed25519.Sign(s.key, b), nil
}

// ObservedWorkflowReceiptForFixture invokes the actual observer/grant adapter
// using an exclusively in-memory HTTP transport and a caller-provided synthetic
// key. It has no root, external credential input, file provisioning or process
// path. It does not model the broker's held process attempt or issue credentials.
// The fixed synthetic token satisfies the adapter seam; nothing reads a secret.
func ObservedWorkflowReceiptForFixture(ctx context.Context, grant WorkflowGrantForFixture, candidate handoff.Request, run WorkflowRunForFixture, keyID string, key ed25519.PrivateKey, now time.Time) ([]byte, WorkflowEffectsForFixture, error) {
	var effects WorkflowEffectsForFixture
	if ctx == nil || ctx.Err() != nil || len(key) != ed25519.PrivateKeySize {
		return nil, effects, errBroker
	}
	body, err := json.Marshal(run)
	if err != nil || len(body) > maxResponseBytes {
		return nil, effects, errBroker
	}
	reviewed := reviewedWorkflowGrant{grant.Request, grant.OrganizationID, grant.RepositoryID, grant.ExpiresAt}
	observer := newWorkflowRunObserver(workflowMemoryTransport{reviewed, body, &effects})
	adapter, err := newWorkflowProvenanceAdapter(reviewed, observer, func(context.Context) (string, error) {
		return "synthetic-observation-token", nil
	}, workflowMemorySigner{bytes.Clone(key), &effects}, keyID, func() time.Time { return now }, nil, func() error { return nil })
	if err != nil {
		return nil, effects, errBroker
	}
	receipt, err := adapter.Attest(ctx, candidate)
	if err != nil {
		return nil, effects, errBroker
	}
	data, err := json.Marshal(receipt)
	if err != nil || len(data) > handoff.MaxReceiptBytes {
		return nil, effects, errBroker
	}
	return data, effects, nil
}
