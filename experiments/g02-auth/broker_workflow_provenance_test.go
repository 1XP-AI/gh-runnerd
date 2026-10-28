package enrollment

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type workflowSignerFunc func(context.Context, []byte) ([]byte, error)

func (f workflowSignerFunc) Sign(ctx context.Context, b []byte) ([]byte, error) { return f(ctx, b) }
func workflowGrantFixture() reviewedWorkflowGrant {
	return reviewedWorkflowGrant{Request: BrokerProvenanceRequest{ControllerApprovalSHA256: strings.Repeat("a", 64), Repository: "org-a/canary", WorkflowRunID: 7, WorkflowRef: "refs/heads/main", WorkflowSHA: strings.Repeat("b", 40), WorkflowPath: ".github/workflows/canary.yml", Phase: "create", OwnerNonce: strings.Repeat("c", 32), Source: workflowProvenanceSource}, OrganizationID: 101, RepositoryID: 501, ExpiresAt: time.Now().UTC().Add(time.Hour)}
}
func workflowRunJSON(t *testing.T, grant reviewedWorkflowGrant) string {
	t.Helper()
	repo := map[string]any{"id": grant.RepositoryID, "full_name": grant.Request.Repository, "private": true, "fork": false, "owner": map[string]any{"id": grant.OrganizationID, "login": strings.Split(grant.Request.Repository, "/")[0]}}
	run := map[string]any{"id": grant.Request.WorkflowRunID, "head_sha": grant.Request.WorkflowSHA, "head_branch": strings.TrimPrefix(grant.Request.WorkflowRef, "refs/heads/"), "path": grant.Request.WorkflowPath, "event": "workflow_dispatch", "run_attempt": 1, "repository": repo, "head_repository": repo}
	b, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestWorkflowProvenanceSignsObservedExactGrant(t *testing.T) {
	grant := workflowGrantFixture()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := NewBrokerProvenanceTrustRoot("reviewed-key", public)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	observer := newWorkflowRunObserver(transportFunc(func(r *http.Request) (*http.Response, error) {
		order = append(order, "GET")
		if r.Method != "GET" || r.URL.String() != "https://api.github.com/repos/org-a/canary/actions/runs/7" || r.Header.Get("Authorization") != "Bearer synthetic-observation-token" || r.Header.Get("Accept-Encoding") != "identity" || r.URL.RawQuery != "" {
			t.Fatal("unexpected observation request")
		}
		return response(200, workflowRunJSON(t, grant)), nil
	}))
	adapter, err := newWorkflowProvenanceAdapter(grant, observer, func(context.Context) (string, error) {
		order = append(order, "token")
		return "synthetic-observation-token", nil
	}, workflowSignerFunc(func(_ context.Context, b []byte) ([]byte, error) {
		order = append(order, "sign")
		return ed25519.Sign(private, b), nil
	}), "reviewed-key", time.Now, nil, func() error { return nil })
	if err != nil {
		t.Fatal("construct adapter", err)
	}
	receipt, err := adapter.Attest(context.Background(), grant.Request)
	if err != nil || root.Verify(grant.Request, receipt, time.Now()) != nil || adapter.Verify(context.Background(), grant.Request, receipt) != nil {
		t.Fatal("observed authorized request did not yield independently verifiable receipt")
	}
	if strings.Join(order, ",") != "token,GET,sign" || receipt.ExpiresAt != grant.ExpiresAt || receipt.ReceiptNonce == grant.Request.OwnerNonce {
		t.Fatal("receipt/order diverged from exact grant")
	}
	next, err := adapter.Attest(context.Background(), grant.Request)
	if err != nil || next.ReceiptNonce == receipt.ReceiptNonce {
		t.Fatal("receipt nonce was not fresh")
	}
}

func TestWorkflowProvenanceRejectsRequestOutsideIndependentGrantBeforeCallbacks(t *testing.T) {
	grant := workflowGrantFixture()
	callbacks := 0
	observer := newWorkflowRunObserver(transportFunc(func(*http.Request) (*http.Response, error) { callbacks++; return nil, errBroker }))
	adapter, err := newWorkflowProvenanceAdapter(grant, observer, func(context.Context) (string, error) { callbacks++; return "synthetic-observation-token", nil }, workflowSignerFunc(func(context.Context, []byte) ([]byte, error) { callbacks++; return nil, errBroker }), "reviewed-key", time.Now, nil, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"digest", "repository", "run", "ref", "sha", "path", "phase", "owner", "source"} {
		t.Run(field, func(t *testing.T) {
			request := grant.Request
			switch field {
			case "digest":
				request.ControllerApprovalSHA256 = strings.Repeat("d", 64)
			case "repository":
				request.Repository = "org-a/other"
			case "run":
				request.WorkflowRunID++
			case "ref":
				request.WorkflowRef = "refs/heads/other"
			case "sha":
				request.WorkflowSHA = strings.Repeat("d", 40)
			case "path":
				request.WorkflowPath = ".github/workflows/other.yml"
			case "phase":
				request.Phase = "cleanup"
			case "owner":
				request.OwnerNonce = strings.Repeat("d", 32)
			case "source":
				request.Source = "other-source"
			}
			if _, err := adapter.Attest(context.Background(), request); err == nil || callbacks != 0 {
				t.Fatal("request outside exact grant reached callback")
			}
		})
	}
}

func TestWorkflowProvenanceRechecksAuthorityAtEveryBoundary(t *testing.T) {
	for _, boundary := range []string{"before token", "after token", "after HTTP", "after nonce", "after sign"} {
		for _, fault := range []string{"claim", "cancellation", "expiry", "deadline"} {
			t.Run(boundary+"/"+fault, func(t *testing.T) {
				grant := workflowGrantFixture()
				instant := time.Now().UTC()
				claimLost := false
				ctx, cancel := context.WithDeadline(context.Background(), instant.Add(time.Minute))
				defer cancel()
				invalidate := func() {
					switch fault {
					case "claim":
						claimLost = true
					case "cancellation":
						cancel()
					case "expiry":
						instant = grant.ExpiresAt
					case "deadline":
						instant = instant.Add(2 * time.Minute)
					}
				}
				tokens, gets, signs := 0, 0, 0
				observer := newWorkflowRunObserver(transportFunc(func(*http.Request) (*http.Response, error) {
					gets++
					if boundary == "after HTTP" {
						invalidate()
					}
					return response(200, workflowRunJSON(t, grant)), nil
				}))
				adapter, err := newWorkflowProvenanceAdapter(grant, observer, func(context.Context) (string, error) {
					tokens++
					if boundary == "after token" {
						invalidate()
					}
					return "synthetic-observation-token", nil
				}, workflowSignerFunc(func(context.Context, []byte) ([]byte, error) {
					signs++
					if boundary == "after sign" {
						invalidate()
					}
					return make([]byte, ed25519.SignatureSize), nil
				}), "reviewed-key", func() time.Time { return instant }, func() ([16]byte, error) {
					if boundary == "after nonce" {
						invalidate()
					}
					return [16]byte{1}, nil
				}, func() error {
					if claimLost {
						return errBroker
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if boundary == "before token" {
					invalidate()
				}
				if _, err := adapter.Attest(ctx, grant.Request); err == nil {
					t.Fatal("lost authority yielded receipt")
				}
				expectedToken, expectedGET, expectedSign := 1, 1, 0
				if boundary == "before token" {
					expectedToken, expectedGET = 0, 0
				}
				if boundary == "after token" {
					expectedGET = 0
				}
				if boundary == "after sign" {
					expectedSign = 1
				}
				if tokens != expectedToken || gets != expectedGET || signs != expectedSign {
					t.Fatalf("crossed revoked boundary: token=%d GET=%d sign=%d", tokens, gets, signs)
				}
			})
		}
	}
}

func TestWorkflowProvenanceRejectsFailedDependenciesAndUnsafeNonce(t *testing.T) {
	for _, fault := range []string{"credential error", "invalid credential", "HTTP error", "nonce error", "owner nonce", "zero nonce", "sign error", "short signature"} {
		t.Run(fault, func(t *testing.T) {
			grant := workflowGrantFixture()
			signs := 0
			observer := newWorkflowRunObserver(transportFunc(func(*http.Request) (*http.Response, error) {
				if fault == "HTTP error" {
					return nil, errBroker
				}
				return response(200, workflowRunJSON(t, grant)), nil
			}))
			adapter, err := newWorkflowProvenanceAdapter(grant, observer, func(context.Context) (string, error) {
				if fault == "credential error" {
					return "", errBroker
				}
				if fault == "invalid credential" {
					return "invalid\ntoken", nil
				}
				return "synthetic-observation-token", nil
			}, workflowSignerFunc(func(context.Context, []byte) ([]byte, error) {
				signs++
				if fault == "sign error" {
					return nil, errBroker
				}
				if fault == "short signature" {
					return []byte{1}, nil
				}
				return make([]byte, 64), nil
			}), "reviewed-key", time.Now, func() ([16]byte, error) {
				if fault == "nonce error" {
					return [16]byte{}, errBroker
				}
				if fault == "zero nonce" {
					return [16]byte{}, nil
				}
				var b [16]byte
				if fault == "owner nonce" {
					for i := range b {
						b[i] = 0xcc
					}
					return b, nil
				}
				return [16]byte{1}, nil
			}, func() error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.Attest(context.Background(), grant.Request); err == nil {
				t.Fatal("unsafe dependency yielded receipt")
			}
			wantSigns := 0
			if fault == "sign error" || fault == "short signature" {
				wantSigns = 1
			}
			if signs != wantSigns {
				t.Fatal("signed before prerequisite succeeded")
			}
		})
	}
}

func TestWorkflowProvenanceRejectsInvalidConfiguration(t *testing.T) {
	for _, fault := range []string{"zero grant", "tag ref", "pull ref", "wrong source", "organization ID", "repository ID", "expired", "overlong grant", "observer", "credential reader", "signer", "key ID", "clock", "attempt guard"} {
		t.Run(fault, func(t *testing.T) {
			grant := workflowGrantFixture()
			observer := newWorkflowRunObserver(transportFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("construction contacted HTTP")
				return nil, errBroker
			}))
			reader := func(context.Context) (string, error) { t.Fatal("construction read token"); return "", errBroker }
			var signer receiptSigner = workflowSignerFunc(func(context.Context, []byte) ([]byte, error) { t.Fatal("construction signed"); return nil, errBroker })
			keyID := "reviewed-key"
			clock := time.Now
			guard := func() error { return nil }
			switch fault {
			case "zero grant":
				grant = reviewedWorkflowGrant{}
			case "tag ref":
				grant.Request.WorkflowRef = "refs/tags/main"
			case "pull ref":
				grant.Request.WorkflowRef = "refs/pull/1/head"
			case "wrong source":
				grant.Request.Source = "other"
			case "organization ID":
				grant.OrganizationID = 0
			case "repository ID":
				grant.RepositoryID = 0
			case "expired":
				grant.ExpiresAt = time.Now().Add(-time.Second)
			case "overlong grant":
				grant.ExpiresAt = time.Now().Add(25 * time.Hour)
			case "observer":
				observer = nil
			case "credential reader":
				reader = nil
			case "signer":
				signer = nil
			case "key ID":
				keyID = "bad/key"
			case "clock":
				clock = nil
			case "attempt guard":
				guard = nil
			}
			if _, err := newWorkflowProvenanceAdapter(grant, observer, reader, signer, keyID, clock, nil, guard); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
}

func TestWorkflowProvenanceBrokerFactoryEnforcesCredentialBoundary(t *testing.T) {
	for _, fault := range []string{"valid", "rootless", "conflict", "replacement signer", "ungranted phase", "insufficient grant expiry", "observation failure", "factory failure", "lost claimed attempt"} {
		t.Run(fault, func(t *testing.T) {
			e := newPairedBrokerEntryFixture(t)
			approval, err := brokerApprovalFromEntryFixture(t, e)
			if err != nil {
				t.Fatal(err)
			}
			var controller controllerApproval
			if _, err := readBrokerPrivateJSON(e.files.ControllerApproval, &controller); err != nil {
				t.Fatal(err)
			}
			request, err := brokerProvenanceRequest(approval, controller, workflowProvenanceSource)
			if err != nil {
				t.Fatal(err)
			}
			grant := reviewedWorkflowGrant{Request: request, OrganizationID: 101, RepositoryID: 501, ExpiresAt: approval.ExpiresAt}
			if fault == "ungranted phase" {
				grant.Request.Phase = "cleanup"
			}
			if fault == "insufficient grant expiry" {
				grant.ExpiresAt = approval.ExpiresAt.Add(-time.Minute)
			}
			public, private, err := ed25519.GenerateKey(nil)
			if err != nil {
				t.Fatal(err)
			}
			root, err := NewBrokerProvenanceTrustRoot("reviewed-key", public)
			if err != nil {
				t.Fatal(err)
			}
			e.api.provenanceRoot = root
			e.api.provenance = nil
			if fault == "rootless" {
				e.api.provenanceRoot = BrokerProvenanceTrustRoot{}
			}
			if fault == "conflict" {
				e.api.provenance = newSignedBrokerFixtureAdapter(t)
			}
			if fault == "replacement signer" {
				_, private, err = ed25519.GenerateKey(nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			factories, tokens, gets, signs := 0, 0, 0, 0
			observer := newWorkflowRunObserver(transportFunc(func(*http.Request) (*http.Response, error) {
				gets++
				if fault == "observation failure" {
					return response(403, `{}`), nil
				}
				return response(200, workflowRunJSON(t, grant)), nil
			}))
			e.api.provenanceFactory = func(guard func() error) (BrokerProvenanceAdapter, error) {
				factories++
				probe := attemptClaimProbeAdapter{attemptPath: e.files.StateDirectory}
				if guard == nil || guard() != nil || !probe.claimHeld() {
					t.Fatal("factory did not receive actual held claim")
				}
				if fault == "factory failure" {
					return nil, errBroker
				}
				return newWorkflowProvenanceAdapter(grant, observer, func(context.Context) (string, error) {
					tokens++
					if fault == "lost claimed attempt" {
						if os.Rename(filepath.Join(e.files.StateDirectory, "broker.jsonl"), filepath.Join(e.files.StateDirectory, "replaced.jsonl")) != nil {
							t.Fatal("replace attempt")
						}
					}
					return "synthetic-observation-token", nil
				}, workflowSignerFunc(func(_ context.Context, b []byte) ([]byte, error) { signs++; return ed25519.Sign(private, b), nil }), "reviewed-key", time.Now, nil, guard)
			}
			inputPath := filepath.Join(e.parent, "input-probe.json")
			if err := os.WriteFile(inputPath, []byte(`invalid-input`), 0600); err != nil {
				t.Fatal(err)
			}
			input, err := os.Open(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			if _, err := runBrokerWithAPI(context.Background(), e.files, input, e.api); err == nil {
				t.Fatal("invalid management input accepted")
			}
			_, statErr := input.Stat()
			if fault == "valid" {
				if !errors.Is(statErr, os.ErrClosed) || tokens != 1 || gets != 1 || signs != 1 {
					t.Fatal("valid observed receipt did not reach credential input exactly once")
				}
			} else {
				if statErr != nil {
					t.Fatal("failed provenance closed management input")
				}
				offset, err := input.Seek(0, 1)
				if err != nil || offset != 0 {
					t.Fatal("failed provenance consumed management input")
				}
			}
			if fault == "rootless" || fault == "conflict" {
				if factories != 0 || tokens != 0 || gets != 0 || signs != 0 {
					t.Fatal("invalid adapter/root configuration invoked callbacks")
				}
			}
			if fault == "ungranted phase" && tokens != 0 {
				t.Fatal("phase outside exact grant read observation token")
			}
			if fault == "lost claimed attempt" && (gets != 0 || signs != 0) {
				t.Fatal("lost claimed attempt reached HTTP/signing")
			}
			if len(e.fixture.calls) != 0 || e.fixture.tokenCalls != 0 {
				t.Fatal("provenance reached management API")
			}
			if fault == "observation failure" {
				// Failed observation permanently consumes this attempt, even though no
				// management input was consumed; a second invocation cannot retry it.
				before := factories
				if _, err := runBrokerWithAPI(context.Background(), e.files, input, e.api); err == nil || factories != before {
					t.Fatal("failed attempt was retried")
				}
			}
		})
	}
}

func TestWorkflowProvenanceFactoryRequiresOneClaimBeforeCallbacks(t *testing.T) {
	now := time.Now()
	approval, controller := brokerProvenanceDeadlineInputs(now)
	fixture := newSignedBrokerFixtureAdapter(t)
	api := newBrokerAPIWithProvenanceRoot(time.Now, nil, fixture.root)
	calls := 0
	api.provenanceFactory = func(func() error) (BrokerProvenanceAdapter, error) { calls++; return fixture, nil }
	for _, claims := range [][]*brokerJournal{nil, {nil}, {nil, nil}} {
		if _, err := brokerWorkflowReceipt(context.Background(), api, approval, controller, approval.ControllerApprovalSHA256, claims...); err == nil {
			t.Fatal("factory accepted absent/invalid held attempt")
		}
	}
	if calls != 0 {
		t.Fatal("invalid attempt invoked factory")
	}
	ordinary := newBrokerAPI(time.Now, nil)
	if ordinary.provenance != nil || ordinary.provenanceFactory != nil || ordinary.provenanceRoot.Valid() {
		t.Fatal("normal constructor enabled provenance")
	}
}

func TestWorkflowProvenanceRejectsCancellationDuringAttemptCheck(t *testing.T) {
	grant := workflowGrantFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tokens, checks := 0, 0
	adapter, err := newWorkflowProvenanceAdapter(grant, newWorkflowRunObserver(transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("canceled claim reached HTTP")
		return nil, errBroker
	})), func(context.Context) (string, error) { tokens++; return "synthetic-observation-token", nil }, workflowSignerFunc(func(context.Context, []byte) ([]byte, error) {
		t.Fatal("canceled claim reached signing")
		return nil, errBroker
	}), "reviewed-key", time.Now, nil, func() error {
		checks++
		if checks == 2 {
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Attest(ctx, grant.Request); err == nil || tokens != 0 {
		t.Fatal("attempt check canceled context but token reader still ran")
	}
}

func TestWorkflowProvenanceBoundsCallbacksToGrantDeadline(t *testing.T) {
	for _, boundary := range []string{"token", "HTTP", "sign"} {
		t.Run(boundary, func(t *testing.T) {
			grant := workflowGrantFixture()
			grant.ExpiresAt = time.Now().UTC().Add(40 * time.Millisecond)
			wait := func(ctx context.Context) {
				deadline, ok := ctx.Deadline()
				if !ok || deadline.After(grant.ExpiresAt) {
					t.Error("callback deadline exceeded grant")
				}
				<-ctx.Done()
			}
			gets, signs := 0, 0
			observer := newWorkflowRunObserver(transportFunc(func(r *http.Request) (*http.Response, error) {
				gets++
				if boundary == "HTTP" {
					wait(r.Context())
				}
				return response(200, workflowRunJSON(t, grant)), nil
			}))
			adapter, err := newWorkflowProvenanceAdapter(grant, observer, func(ctx context.Context) (string, error) {
				if boundary == "token" {
					wait(ctx)
				}
				return "synthetic-observation-token", nil
			}, workflowSignerFunc(func(ctx context.Context, _ []byte) ([]byte, error) {
				signs++
				if boundary == "sign" {
					wait(ctx)
				}
				return make([]byte, ed25519.SignatureSize), nil
			}), "reviewed-key", time.Now, nil, func() error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.Attest(context.Background(), grant.Request); err == nil {
				t.Fatal("callback completed after grant expiry but yielded receipt")
			}
			if boundary == "token" && gets != 0 || boundary != "sign" && signs != 0 {
				t.Fatal("expired callback crossed next boundary")
			}
		})
	}
}
