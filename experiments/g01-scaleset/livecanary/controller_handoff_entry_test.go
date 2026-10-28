package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func handoffCredentialBytes(t *testing.T, a Approval) []byte {
	t.Helper()
	b, err := json.Marshal(credentials(a))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestControllerHandoffEntryGuardsBeforeCredentialReader(t *testing.T) {
	for _, fault := range []string{"rootless", "source", "receipt", "nil factory", "nil journal", "nil context", "nil clock", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			f, j := openCreateControllerHandoff(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var entryContext context.Context = ctx
			now := time.Now
			factory := func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("invalid entry built API")
				return nil, nil
			}
			switch fault {
			case "rootless":
				f.root = handoff.TrustRoot{}
			case "source":
				f.request.Source = "untrusted-source"
			case "receipt":
				f.receiptBytes = []byte("invalid")
			case "nil factory":
				factory = nil
			case "nil journal":
				j = nil
			case "nil context":
				entryContext = nil
			case "nil clock":
				now = nil
			case "canceled":
				cancel()
			}
			if runControllerHandoffCreate(entryContext, j, f.snapshot, f.receiptBytes, f.request.Source, f.root, now, func(context.Context) ([]byte, error) { t.Fatal("invalid entry read credentials"); return nil, nil }, factory) == nil {
				t.Fatal("invalid entry accepted")
			}
		})
	}
}

func TestControllerHandoffEntryFreshNonceCannotRetryFailedCreate(t *testing.T) {
	f, j := openCreateControllerHandoff(t)
	reads := 0
	read := func(context.Context) ([]byte, error) { reads++; return []byte("malformed"), nil }
	factory := func(context.Context, Approval, Credentials) (API, error) {
		t.Fatal("invalid input built API")
		return nil, nil
	}
	if runControllerHandoffCreate(context.Background(), j, f.snapshot, f.receiptBytes, f.request.Source, f.root, time.Now, read, factory) == nil {
		t.Fatal("invalid credentials accepted")
	}
	r := f.receipt
	r.ReceiptNonce = strings.Repeat("e", 32)
	if runControllerHandoffCreate(context.Background(), j, f.snapshot, f.encodeReceipt(t, r, true), f.request.Source, f.root, time.Now, read, factory) == nil || reads != 1 || len(j.Events()) != 1 {
		t.Fatalf("fresh nonce retried consumed create: reads=%d events=%d", reads, len(j.Events()))
	}
}

func TestControllerHandoffEntryConcurrentFreshNoncesReadOnce(t *testing.T) {
	f, j := openCreateControllerHandoff(t)
	r := f.receipt
	r.ReceiptNonce = strings.Repeat("e", 32)
	receipts := [][]byte{f.receiptBytes, f.encodeReceipt(t, r, true)}
	var reads atomic.Int32
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, receipt := range receipts {
		go func() {
			<-start
			done <- runControllerHandoffCreate(context.Background(), j, f.snapshot, receipt, f.request.Source, f.root, time.Now, func(context.Context) ([]byte, error) { reads.Add(1); return []byte("malformed"), nil }, func(context.Context, Approval, Credentials) (API, error) {
				return nil, errors.New("unexpected API factory")
			})
		}()
	}
	close(start)
	for range receipts {
		if <-done == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
	if reads.Load() != 1 || len(j.Events()) != 1 {
		t.Fatalf("concurrent creates crossed credential boundary: reads=%d events=%d", reads.Load(), len(j.Events()))
	}
}

func TestControllerHandoffEntryCreatesOnceAfterDurableConsumption(t *testing.T) {
	f, j := openCreateControllerHandoff(t)
	data := handoffCredentialBytes(t, f.approval)
	fake := &fakeAPI{}
	reads, factories := 0, 0
	err := runControllerHandoffCreate(context.Background(), j, f.snapshot, f.receiptBytes, f.request.Source, f.root, time.Now, func(context.Context) ([]byte, error) {
		reads++
		events := j.Events()
		if len(events) != 1 || events[0].ControllerHandoff == nil || events[0].ControllerHandoff.ApprovalSHA256 != f.request.ControllerApprovalSHA256 {
			t.Fatal("credential callback preceded durable nonce fence")
		}
		// Read the stored journal independently, rather than trusting only Events.
		stored, err := j.root.ReadFile("journal.jsonl")
		if err != nil || !bytes.Contains(stored, []byte(f.receipt.ReceiptNonce)) {
			t.Fatal("nonce not present in stored journal")
		}
		return data, nil
	}, func(_ context.Context, a Approval, c Credentials) (API, error) {
		factories++
		if approvalDigest(a) != approvalDigest(f.approval) || c.validate(a, time.Now()) != nil {
			t.Fatal("factory received unvalidated approval/credentials")
		}
		return handoffValidInventoryAPI{fake}, nil
	})
	if err != nil || reads != 1 || factories != 1 || fake.createCalls != 1 {
		t.Fatalf("entry did not perform one authorized create: err=%v reads=%d factories=%d creates=%d", err, reads, factories, fake.createCalls)
	}
	if !bytes.Equal(data, make([]byte, len(data))) {
		t.Fatal("raw credential bytes were retained")
	}
}

func TestControllerHandoffEntryValidatesCredentialsBeforeFactory(t *testing.T) {
	for _, fault := range []string{"malformed", "unknown", "duplicate", "case duplicate", "trailing", "oversize", "identity", "scope", "token", "expiry", "permissions", "paired binding", "reader error", "cancel reader", "factory error", "cancel factory", "nil API"} {
		t.Run(fault, func(t *testing.T) {
			f, j := openCreateControllerHandoff(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := credentials(f.approval)
			switch fault {
			case "identity":
				c.InstallationID++
			case "scope":
				c.ManagementScope = "organization-only"
			case "token":
				c.InstallationToken = "bad"
			case "expiry":
				c.ExpiresAt = time.Now()
			case "permissions":
				c.SelfHostedRunners = "read"
			case "paired binding":
				c.PairedBinding = &PairedTerminalBinding{}
			}
			data, _ := json.Marshal(c)
			switch fault {
			case "malformed":
				data = []byte("{")
			case "unknown":
				data = append([]byte(`{"unknown":1,`), data[1:]...)
			case "duplicate":
				data = append([]byte(`{"app_id":11,`), data[1:]...)
			case "case duplicate":
				data = append([]byte(`{"APP_ID":11,`), data[1:]...)
			case "trailing":
				data = append(data, []byte(` {}`)...)
			case "oversize":
				data = append(data, []byte(strings.Repeat(" ", 16385))...)
			}
			reads, factories := 0, 0
			fake := &fakeAPI{}
			err := runControllerHandoffCreate(ctx, j, f.snapshot, f.receiptBytes, f.request.Source, f.root, time.Now, func(context.Context) ([]byte, error) {
				reads++
				if fault == "cancel reader" {
					cancel()
				}
				if fault == "reader error" {
					return data, errors.New("private detail")
				}
				return data, nil
			}, func(context.Context, Approval, Credentials) (API, error) {
				factories++
				if fault == "factory error" {
					return nil, errors.New("private detail")
				}
				if fault == "cancel factory" {
					cancel()
				}
				if fault == "nil API" {
					return nil, nil
				}
				return handoffValidInventoryAPI{fake}, nil
			})
			wantFactories := 0
			if fault == "factory error" || fault == "cancel factory" || fault == "nil API" {
				wantFactories = 1
			}
			if err == nil || reads != 1 || factories != wantFactories || fake.createCalls != 0 || !bytes.Equal(data, make([]byte, len(data))) {
				t.Fatalf("unsafe credential boundary: err=%v reads=%d factories=%d creates=%d cleared=%v", err, reads, factories, fake.createCalls, bytes.Equal(data, make([]byte, len(data))))
			}
			if err.Error() == "private detail" {
				t.Fatal("raw dependency error escaped")
			}
			// Failures must leave the consumed attempt permanently unavailable.
			if len(j.Events()) != 1 || j.Events()[0].ControllerHandoff == nil {
				t.Fatal("consumption was reset")
			}
			err = runControllerHandoffCreate(context.Background(), j, f.snapshot, f.receiptBytes, f.request.Source, f.root, time.Now, func(context.Context) ([]byte, error) { t.Fatal("replay read credentials"); return nil, nil }, func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("replay built API")
				return nil, nil
			})
			if err == nil {
				t.Fatal("consumed receipt replayed")
			}
		})
	}
}
