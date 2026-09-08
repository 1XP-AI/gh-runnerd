package enrollment

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This models a trusted preparer's local receipt, not G01's replay parser. The
// canonical parser is tested in G01 and through the optional executable fixture.
func brokerSyntheticPreparation(t *testing.T, p *brokerControllerPlan, directory string) (brokerPreparationReceipt, error) {
	t.Helper()
	path := filepath.Join(p.statePath, "journal.jsonl")
	if _, e := os.Stat(path); os.IsNotExist(e) {
		if os.WriteFile(path, []byte("synthetic prepared journal\n"), 0600) != nil {
			t.Fatal("fixture journal")
		}
	}
	jb, e := os.ReadFile(path)
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	ji, e := os.Stat(path)
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	binding, e := p.binding()
	if e != nil {
		return brokerPreparationReceipt{}, e
	}
	id := brokerFileIdentity(ji)
	claimPath := filepath.Join(directory, "admission.json")
	if _, e := os.Lstat(claimPath); os.IsNotExist(e) {
		data, _ := json.Marshal(map[string]any{"version": 1, "ownership": binding.Ownership, "state_device": binding.State.Device, "state_inode": binding.State.Inode, "journal_device": id.Device, "journal_inode": id.Inode})
		if os.WriteFile(claimPath, data, 0600) != nil {
			t.Fatal("fixture claim")
		}
	}
	cb, e := os.ReadFile(claimPath)
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	ci, e := os.Stat(claimPath)
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	return brokerPreparationReceipt{1, "controller_journal_prepared", p.approval.Phase, brokerDigest(p.controller), binding.State, id, brokerFileIdentity(ci), brokerBytesDigest(jb), brokerBytesDigest(cb)}, nil
}

// This models the explicit nonproduction worker-preparer seam used by direct
// broker unit tests. The real entrypoint invokes G01's canonical worker parser;
// this helper only supplies bounded receipt bytes for tests that do not spawn
// the reviewed executable.
func brokerSyntheticWorkerPreparation(t *testing.T, p *brokerWorkerPlan, directory string) (brokerPreparationReceipt, error) {
	t.Helper()
	if p == nil || p.check() != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
		return brokerPreparationReceipt{}, errBroker
	}
	p.claimDirectory = directory
	path := filepath.Join(p.statePath, "journal.jsonl")
	if _, e := os.Stat(path); os.IsNotExist(e) {
		if os.WriteFile(path, []byte("synthetic prepared worker journal\n"), 0600) != nil {
			return brokerPreparationReceipt{}, errBroker
		}
	}
	journalData, e := os.ReadFile(path)
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	journalInfo, e := os.Stat(path)
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	stateInfo, e := os.Stat(p.statePath)
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	worker, e := p.binding()
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	claimPath := filepath.Join(directory, "admission.json")
	if _, e = os.Stat(claimPath); os.IsNotExist(e) {
		claim := map[string]any{"version": 1, "ownership": brokerBytesDigest(p.raw), "state_device": worker.State.Device, "state_inode": worker.State.Inode, "journal_device": brokerFileIdentity(journalInfo).Device, "journal_inode": brokerFileIdentity(journalInfo).Inode}
		claimData, _ := json.Marshal(claim)
		if os.WriteFile(claimPath, append(claimData, '\n'), 0600) != nil {
			return brokerPreparationReceipt{}, errBroker
		}
	}
	claimData, e := os.ReadFile(claimPath)
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	claimInfo, e := os.Stat(claimPath)
	if e != nil {
		return brokerPreparationReceipt{}, errBroker
	}
	return brokerPreparationReceipt{Version: 1, Status: "worker_journal_prepared", Phase: "paired-worker", ApprovalDigest: brokerDigest(p.approval), State: brokerFileIdentity(stateInfo), Journal: brokerFileIdentity(journalInfo), Claim: brokerFileIdentity(claimInfo), JournalDigest: brokerBytesDigest(journalData), ClaimDigest: brokerBytesDigest(claimData)}, nil
}

func brokerAttachSyntheticWorkerPreparation(t *testing.T, p *brokerControllerPlan, directory string) {
	t.Helper()
	if p == nil || p.worker == nil {
		t.Fatal("missing paired worker plan")
	}
	p.worker.prepare = func(context.Context) (brokerPreparationReceipt, error) {
		return brokerSyntheticWorkerPreparation(t, p.worker, directory)
	}
}
func TestBrokerPreparedReceiptBindsValidatedBytesBeforeMint(t *testing.T) {
	for _, kind := range []string{"journal changed before capture", "claim changed before capture", "wrong phase", "wrong approval", "missing identity", "wrong version", "wrong status"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			launched := false
			p := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { launched = true; return nil })
			prepare := p.localPrepare
			p.localPrepare = func(ctx context.Context, path string) (brokerPreparationReceipt, error) {
				r, e := prepare(ctx, path)
				if e != nil {
					return r, e
				}
				switch kind {
				case "journal changed before capture":
					os.WriteFile(filepath.Join(p.statePath, "journal.jsonl"), []byte("changed after canonical receipt\n"), 0600)
				case "claim changed before capture":
					name := filepath.Join(f.admissionRoot, "admission.json")
					b, _ := os.ReadFile(name)
					os.WriteFile(name, append(b, '\n'), 0600)
				case "wrong phase":
					r.Phase = "inspect"
				case "wrong approval":
					r.ApprovalDigest = strings.Repeat("f", 64)
				case "missing identity":
					r.Journal = brokerInode{}
				case "wrong version":
					r.Version = 0
				case "wrong status":
					r.Status = "other"
				}
				return r, nil
			}
			if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, p); e == nil || f.tokenCalls != 0 || len(f.calls) != 0 || launched {
				t.Fatal("changed or mismatched prepared receipt reached authenticated effects")
			}
		})
	}
}

func TestBrokerCanonicalPreparationExecutableBridge(t *testing.T) {
	path := os.Getenv("G01_BROKER_CANONICAL_PREPARER_FIXTURE")
	if path == "" {
		t.Skip("requires G01 local-preparation Go test executable")
	}
	f, e := openBrokerPrivateFile(path, 0500, 128<<20)
	if e != nil {
		t.Fatal("private fixture executable")
	}
	defer f.Close()
	bytes, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("fixture bytes")
	}
	binary := &verifiedBrokerBinary{path: path, file: f, digest: brokerBytesDigest(bytes)}
	for _, kind := range []string{"fresh positive", "locked", "malformed", "oversized", "permission", "authority", "used phase", "pending intent", "changed after receipt"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, httpFixture, root := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			launched := false
			p := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { launched = true; return nil })
			p.localPrepare = func(ctx context.Context, snapshot string) (brokerPreparationReceipt, error) {
				r, e := invokeBrokerPreparation(ctx, binary, filepath.Dir(root), snapshot, p.statePath, a.Phase)
				if e == nil && kind == "changed after receipt" {
					os.WriteFile(filepath.Join(p.statePath, "journal.jsonl"), []byte("invalid after canonical receipt\n"), 0600)
				}
				return r, e
			}
			if kind != "fresh positive" && kind != "changed after receipt" {
				source := filepath.Join(filepath.Dir(root), "initial-approved.json")
				if os.WriteFile(source, p.raw, 0600) != nil {
					t.Fatal("fixture source")
				}
				receipt, e := invokeBrokerPreparation(context.Background(), binary, filepath.Dir(root), source, p.statePath, a.Phase)
				if e != nil || !receipt.valid(p) {
					t.Fatal("actual canonical initial preparation failed")
				}
				journal := filepath.Join(p.statePath, "journal.jsonl")
				data, _ := os.ReadFile(journal)
				switch kind {
				case "locked":
					handle, e := os.OpenFile(journal, os.O_RDWR, 0)
					if e != nil {
						t.Fatal("fixture lease")
					}
					defer handle.Close()
					if syscall.Flock(int(handle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
						t.Fatal("fixture lock")
					}
				case "malformed":
					data = append(data, []byte("{invalid-event}\n")...)
				case "oversized":
					data = []byte(strings.Repeat("x", (1<<20)+1))
				case "permission":
					os.Chmod(journal, 0644)
				case "authority":
					var header map[string]any
					json.Unmarshal(data, &header)
					header["authority"].(map[string]any)["digest"] = strings.Repeat("f", 64)
					data, _ = json.Marshal(header)
					data = append(data, '\n')
				case "used phase":
					data = append(data, []byte(`{"sequence":1,"kind":"phase","operation":"create"}`+"\n")...)
				case "pending intent":
					data = append(data, []byte(`{"sequence":1,"kind":"intent","operation":"create"}`+"\n")...)
				}
				if kind != "locked" && kind != "permission" {
					if os.WriteFile(journal, data, 0600) != nil {
						t.Fatal("fixture invalid journal")
					}
				}
			}
			_, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, p)
			if kind == "fresh positive" {
				if e != nil || httpFixture.tokenCalls != 1 || !launched {
					t.Fatal("canonical preparation did not permit one synthetic issuance/handoff")
				}
			} else if e == nil || len(httpFixture.calls) != 0 || httpFixture.tokenCalls != 0 || launched {
				t.Fatalf("canonical %s reached authenticated effect", kind)
			}
		})
	}
}

func TestBrokerPreparationProcessRequiresBoundedTypedResultAndEmptyInput(t *testing.T) {
	binary := testBrokerBinary(t)
	root := t.TempDir()
	receipt, e := invokeBrokerPreparation(context.Background(), binary, root, filepath.Join(root, "approval.json"), root, "create")
	if e != nil || receipt.Version != 1 || receipt.Status != "controller_journal_prepared" {
		t.Fatal("fixed preparation argv/env/empty input/JSON route failed")
	}
	for _, phase := range []string{"before-ack", "inspect", "cleanup", "after-ack"} {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		_, e := invokeBrokerPreparation(ctx, binary, root, filepath.Join(root, "approval.json"), root, phase)
		cancel()
		if e == nil || strings.Contains(e.Error(), "synthetic-private") {
			t.Fatal("unbounded/duplicate/empty preparation response accepted or leaked")
		}
	}
}

func TestBrokerPreparedStateLeaseFailureReleasesOtherLeases(t *testing.T) {
	a, _, _, f, root := newBrokerFixture(t)
	a.Mode = "controller"
	a.Phase = "create"
	p := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { return nil })
	receipt, e := p.localPrepare(context.Background(), "")
	if e != nil {
		t.Fatal("fixture preparation")
	}
	p.preparationReceipt = receipt
	admission, e := openBrokerPrivateDirectory(f.admissionRoot)
	if e != nil {
		t.Fatal("fixture root")
	}
	defer admission.Close()
	defer p.close()
	if p.preparedState(admission, true) != nil {
		t.Fatal("capture positive")
	}
	handle, e := os.Open(filepath.Join(p.statePath, "journal.jsonl"))
	if e != nil {
		t.Fatal("fixture lock")
	}
	defer handle.Close()
	if syscall.Flock(int(handle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		t.Fatal("fixture lock")
	}
	if p.preparedState(admission, false) == nil {
		t.Fatal("held journal lease ignored")
	}
	syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
	if p.preparedState(admission, false) != nil {
		t.Fatal("failed check leaked another lease")
	}
}
func TestBrokerPreparedFilesChangeAfterCaptureStopsBeforeMint(t *testing.T) {
	for _, kind := range []string{"journal write", "journal replacement", "claim write", "claim replacement"} {
		t.Run(kind, func(t *testing.T) {
			a, c, _, f, root := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			p := brokerTestPlan(t, &a, filepath.Dir(root), func(context.Context, []byte, string) error { t.Fatal("changed prepared state handed off"); return nil })
			api := newBrokerAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
				response, e := f.RoundTrip(r)
				if r.URL.Path == "/app" {
					path := filepath.Join(p.statePath, "journal.jsonl")
					if strings.HasPrefix(kind, "claim") {
						path = filepath.Join(f.admissionRoot, "admission.json")
					}
					data, _ := os.ReadFile(path)
					if strings.HasSuffix(kind, "replacement") {
						os.Rename(path, path+"-retained")
					}
					os.WriteFile(path, append(data, '\n'), 0600)
				}
				return response, e
			}))
			api.admissionDirectory = func() (string, error) { return f.admissionRoot, nil }
			if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, p); e == nil || f.tokenCalls != 0 || len(f.calls) != 1 {
				t.Fatal("prepared state mutation reached mint")
			}
		})
	}
}
