//go:build g01_pair_fixture

package livecanary

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

const observedHandoffSource = "github-rest-workflow-v1"

func observedHandoffInputs(t *testing.T, f controllerHandoffFixture) (enrollment.WorkflowGrantForFixture, enrollment.WorkflowRunForFixture) {
	t.Helper()
	_, request, err := controllerHandoffRequest(f.snapshot, "create", observedHandoffSource, f.now)
	if err != nil {
		t.Fatal(err)
	}
	grant := enrollment.WorkflowGrantForFixture{Request: request, OrganizationID: 101, RepositoryID: f.approval.RepositoryID, ExpiresAt: f.now.Add(time.Hour)}
	repo := enrollment.WorkflowRepositoryForFixture{ID: grant.RepositoryID, FullName: request.Repository, Private: true}
	repo.Owner.ID, repo.Owner.Login = grant.OrganizationID, f.approval.Organization
	run := enrollment.WorkflowRunForFixture{ID: request.WorkflowRunID, HeadSHA: request.WorkflowSHA, HeadBranch: "main", Event: "workflow_dispatch", Path: request.WorkflowPath, RunAttempt: 1, Repository: repo, HeadRepository: repo}
	return grant, run
}

func observedHandoffReceipt(t *testing.T, f controllerHandoffFixture) []byte {
	t.Helper()
	grant, run := observedHandoffInputs(t, f)
	b, effects, err := enrollment.ObservedWorkflowReceiptForFixture(context.Background(), grant, grant.Request, run, f.root.KeyID(), f.privateKey, f.now)
	if err != nil || effects.ObservationGETs != 1 || effects.Signatures != 1 {
		t.Fatalf("actual adapter did not observe then sign: %v %+v", err, effects)
	}
	return b
}

func TestControllerHandoffObservedAdapterCreatesOnce(t *testing.T) {
	f, j := openCreateControllerHandoff(t)
	receipt := observedHandoffReceipt(t, f)
	r, err := handoff.DecodeStrictReceipt(receipt)
	grant, _ := observedHandoffInputs(t, f)
	if err != nil || f.root.Verify(grant.Request, r, f.now) != nil {
		t.Fatal("actual producer receipt failed independently pinned root")
	}
	data := handoffCredentialBytes(t, f.approval)
	reads, factories := 0, 0
	fake := &fakeAPI{}
	err = runControllerHandoffCreate(context.Background(), j, f.snapshot, receipt, observedHandoffSource, f.root, time.Now, func(context.Context) ([]byte, error) {
		reads++
		stored, err := j.root.ReadFile("journal.jsonl")
		var event Event
		lines := bytes.Split(bytes.TrimSpace(stored), []byte{'\n'})
		if err != nil || len(lines) != 2 || DecodeStrict(lines[1], &event) != nil || event.Sequence != 1 || event.Kind != "controller-handoff-consumed" || event.ControllerHandoff == nil {
			t.Fatal("credential read before stored nonce event")
		}
		hash := sha256.Sum256(receipt)
		if event.ControllerHandoff.ReceiptNonce != r.ReceiptNonce || event.ControllerHandoff.ApprovalSHA256 != grant.Request.ControllerApprovalSHA256 || event.ControllerHandoff.ReceiptSHA256 != hex.EncodeToString(hash[:]) || event.ControllerHandoff.KeyID != f.root.KeyID() {
			t.Fatal("durable event lost original receipt/approval binding")
		}
		// Callers can mutate their buffers after entry; entry owns its snapshots.
		clear(f.snapshot)
		clear(receipt)
		return data, nil
	}, func(ctx context.Context, a Approval, c Credentials) (API, error) {
		factories++
		deadline, ok := ctx.Deadline()
		if !ok || deadline.After(r.ExpiresAt) || a.Controller != f.approval.Controller || c.validate(a, time.Now()) != nil || !bytes.Equal(data, make([]byte, len(data))) {
			t.Fatal("factory preceded strict credentials/proof deadline or used caller-mutated bytes")
		}
		return handoffValidInventoryAPI{fake}, nil
	})
	if err != nil || reads != 1 || factories != 1 || fake.createCalls != 1 {
		t.Fatalf("observed source-to-create failed: %v reads=%d factories=%d creates=%d", err, reads, factories, fake.createCalls)
	}
	// New valid receipt bytes from the same adapter cannot retry a used create.
	f.snapshot = controllerHandoffSnapshot(t, f.approval)
	fresh := observedHandoffReceipt(t, f)
	err = runControllerHandoffCreate(context.Background(), j, f.snapshot, fresh, observedHandoffSource, f.root, time.Now, func(context.Context) ([]byte, error) { t.Fatal("fresh nonce reread credentials"); return nil, nil }, func(context.Context, Approval, Credentials) (API, error) {
		t.Fatal("fresh nonce rebuilt API")
		return nil, nil
	})
	if err == nil || fake.createCalls != 1 {
		t.Fatal("fresh actual-adapter nonce retried create")
	}
}

func TestControllerHandoffObservedProducerRejectsIndependentMismatches(t *testing.T) {
	for _, fault := range []string{"candidate phase", "candidate digest", "candidate source", "run", "sha", "branch", "event", "path", "attempt", "repository", "head repository", "owner", "public", "fork"} {
		t.Run(fault, func(t *testing.T) {
			f, j := openCreateControllerHandoff(t)
			grant, run := observedHandoffInputs(t, f)
			candidate := grant.Request
			switch fault {
			case "candidate phase":
				candidate.Phase = "cleanup"
			case "candidate digest":
				candidate.ControllerApprovalSHA256 = strings.Repeat("a", 64)
			case "candidate source":
				candidate.Source = "other-source"
			case "run":
				run.ID++
			case "sha":
				run.HeadSHA = strings.Repeat("f", 40)
			case "branch":
				run.HeadBranch = "other"
			case "event":
				run.Event = "push"
			case "path":
				run.Path = ".github/workflows/other.yml"
			case "attempt":
				run.RunAttempt++
			case "repository":
				run.Repository.ID++
			case "head repository":
				run.HeadRepository.ID++
			case "owner":
				run.Repository.Owner.ID++
			case "public":
				run.Repository.Private = false
			case "fork":
				run.HeadRepository.Fork = true
			}
			b, effects, err := enrollment.ObservedWorkflowReceiptForFixture(context.Background(), grant, candidate, run, f.root.KeyID(), f.privateKey, f.now)
			wantGETs := 1
			if strings.HasPrefix(fault, "candidate ") {
				wantGETs = 0
			}
			if err == nil || len(b) != 0 || effects.ObservationGETs != wantGETs || effects.Signatures != 0 {
				t.Fatalf("unchecked independent facts reached signing: %v %+v", err, effects)
			}
			// A failed producer gives the consumer no fallback authority.
			if runControllerHandoffCreate(context.Background(), j, f.snapshot, b, observedHandoffSource, f.root, time.Now, func(context.Context) ([]byte, error) {
				t.Fatal("failed observation read controller credentials")
				return nil, nil
			}, func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("failed observation built API")
				return nil, nil
			}) == nil || len(j.Events()) != 0 {
				t.Fatal("unobserved run consumed controller attempt")
			}
		})
	}
}

func TestControllerHandoffObservedConsumerRejectsBeforeCredentials(t *testing.T) {
	for _, fault := range []string{"raw approval", "approval digest", "phase", "source", "root", "signature", "duplicate receipt", "unknown receipt", "journal approval", "journal closed", "cancellation"} {
		t.Run(fault, func(t *testing.T) {
			f, j := openCreateControllerHandoff(t)
			grant, run := observedHandoffInputs(t, f)
			if fault == "phase" {
				grant.Request.Phase = "before-ack"
			}
			receipt, _, err := enrollment.ObservedWorkflowReceiptForFixture(context.Background(), grant, grant.Request, run, f.root.KeyID(), f.privateKey, f.now)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, source, root := f.snapshot, observedHandoffSource, f.root
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch fault {
			case "raw approval":
				snapshot = append(bytes.Clone(snapshot), '\n')
			case "approval digest":
				// An authentic receipt for changed approval must still fail the
				// independently opened journal's original canonical authority.
				a := f.approval
				a.Controller = "other-controller"
				snapshot = controllerHandoffSnapshot(t, a)
				_, request, e := controllerHandoffRequest(snapshot, "create", source, f.now)
				if e != nil {
					t.Fatal(e)
				}
				grant.Request = request
				receipt, _, err = enrollment.ObservedWorkflowReceiptForFixture(ctx, grant, request, run, f.root.KeyID(), f.privateKey, f.now)
				if err != nil {
					t.Fatal(err)
				}
			case "source":
				source = "other-source"
			case "root":
				other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize))
				root, err = handoff.NewTrustRoot(f.root.KeyID(), other.Public().(ed25519.PublicKey))
				if err != nil {
					t.Fatal(err)
				}
			case "signature":
				var r handoff.Receipt
				if json.Unmarshal(receipt, &r) != nil {
					t.Fatal("decode fixture")
				}
				r.Signature = strings.Repeat("A", len(r.Signature))
				receipt, _ = json.Marshal(r)
			case "duplicate receipt":
				receipt = append([]byte(`{"phase":"create",`), receipt[1:]...)
			case "unknown receipt":
				receipt = append([]byte(`{"unexpected":1,`), receipt[1:]...)
			case "journal approval":
				if j.Close() != nil {
					t.Fatal("close original journal")
				}
				a := f.approval
				a.OwnerNonce = strings.Repeat("e", 32)
				admission, e := filepath.EvalSymlinks(privateDir(t))
				if e != nil {
					t.Fatal(e)
				}
				other, e := openJournalAtAdmission(privateDir(t), a, admission, func(f *os.File) error { return f.Sync() })
				if e != nil {
					t.Fatal(e)
				}
				defer other.Close()
				j = other
			case "journal closed":
				if j.Close() != nil {
					t.Fatal("close journal")
				}
			case "cancellation":
				cancel()
			}
			err = runControllerHandoffCreate(ctx, j, snapshot, receipt, source, root, time.Now, func(context.Context) ([]byte, error) { t.Fatal("invalid binding read credentials"); return nil, nil }, func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("invalid binding built API")
				return nil, nil
			})
			if err == nil || len(j.Events()) != 0 {
				t.Fatalf("invalid receipt reached durable consumption: %v", err)
			}
		})
	}
}

func TestControllerHandoffObservedFailureRetainsConsumptionAcrossReopen(t *testing.T) {
	for _, fault := range []string{"malformed credentials", "cancel reader", "journal closed by reader", "journal reopened by factory"} {
		t.Run(fault, func(t *testing.T) {
			f, j := openCreateControllerHandoff(t)
			directory := j.directory
			receipt := observedHandoffReceipt(t, f)
			data := handoffCredentialBytes(t, f.approval)
			if fault == "malformed credentials" {
				data = []byte(`{"installation_token":"synthetic"}`)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			factories, reads := 0, 0
			fake := &fakeAPI{}
			var reopened *FileJournal
			err := runControllerHandoffCreate(ctx, j, f.snapshot, receipt, observedHandoffSource, f.root, time.Now, func(context.Context) ([]byte, error) {
				reads++
				if fault == "cancel reader" {
					cancel()
				}
				if fault == "journal closed by reader" {
					if j.Close() != nil {
						t.Fatal("close consumed journal")
					}
				}
				return data, nil
			}, func(context.Context, Approval, Credentials) (API, error) {
				factories++
				if j.Close() != nil {
					t.Fatal("close old journal")
				}
				var e error
				reopened, e = openTestJournal(t, directory, f.approval)
				if e != nil {
					t.Fatal(e)
				}
				return handoffValidInventoryAPI{fake}, nil
			})
			wantFactories := 0
			if fault == "journal reopened by factory" {
				wantFactories = 1
			}
			if err == nil || reads != 1 || factories != wantFactories || fake.createCalls != 0 || !bytes.Equal(data, make([]byte, len(data))) {
				t.Fatalf("failed handoff crossed API boundary: %v reads=%d factories=%d creates=%d", err, reads, factories, fake.createCalls)
			}
			if reopened == nil {
				_ = j.Close()
				var e error
				reopened, e = openTestJournal(t, directory, f.approval)
				if e != nil {
					t.Fatal(e)
				}
			}
			defer reopened.Close()
			if len(reopened.Events()) != 1 || reopened.Events()[0].ControllerHandoff == nil {
				t.Fatal("consumption did not survive journal reopen")
			}
			fresh := observedHandoffReceipt(t, f)
			err = runControllerHandoffCreate(context.Background(), reopened, f.snapshot, fresh, observedHandoffSource, f.root, time.Now, func(context.Context) ([]byte, error) {
				t.Fatal("failed create reread credentials after reopen")
				return nil, nil
			}, func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("failed create recreated API after reopen")
				return nil, nil
			})
			if err == nil || len(reopened.Events()) != 1 {
				t.Fatal("fresh nonce reset consumed attempt")
			}
		})
	}
}
