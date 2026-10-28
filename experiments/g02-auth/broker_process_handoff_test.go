package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func runBrokerHandoffTestChild() int {
	mode := strings.TrimPrefix(filepath.Base(os.Args[3]), "handoff-")
	switch mode {
	case "early-exit":
		return 9
	case "blocked-control":
		time.Sleep(time.Minute)
		return 9
	case "overflow":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), 8193))
		time.Sleep(time.Minute)
		return 9
	}
	control := os.NewFile(3, "control")
	defer control.Close()
	receipt, binding, e := handoff.ReadControl(control)
	if e != nil || len(receipt) == 0 || len(binding) == 0 {
		return 5
	}
	if mode == "blocked-input" {
		time.Sleep(time.Minute)
		return 9
	}
	payload, e := io.ReadAll(io.LimitReader(os.Stdin, handoff.MaxCredentialBytes+1))
	if e != nil || (len(payload) != handoff.MaxCredentialBytes && !bytes.Contains(payload, []byte("synthetic-private-installation-token"))) {
		return 6
	}
	if mode == "child-fail" {
		return 9
	}
	return 0
}
func TestBrokerHandoffCarrierBoundedChildFailures(t *testing.T) {
	binary := testBrokerBinary(t)
	parent, _ := filepath.EvalSymlinks(t.TempDir())
	for _, mode := range []string{"positive", "early-exit", "blocked-control", "blocked-input", "overflow", "child-fail", "start-fail", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			budget := 200 * time.Millisecond
			if mode == "positive" {
				budget = 3 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			root := parent
			if mode == "start-fail" {
				root = filepath.Join(parent, "absent")
			}
			h := &issuedControllerHandoff{receipt: []byte("receipt"), envelope: []byte("binding"), payload: bytes.Repeat([]byte("x"), handoff.MaxCredentialBytes), deadline: time.Now().Add(3 * time.Second)}
			// Fill FD3 past pipe capacity for the blocked-control case.
			if mode == "blocked-control" {
				h.receipt = bytes.Repeat([]byte("r"), handoff.MaxReceiptBytes)
				h.envelope = bytes.Repeat([]byte("b"), handoff.MaxBindingBytes)
			}
			started := time.Now()
			e := invokeBrokerControllerHandoff(ctx, binary, root, filepath.Join(parent, "handoff-"+mode), parent, h)
			if (e == nil) != (mode == "positive") || time.Since(started) > 4*time.Second {
				t.Fatalf("carrier outcome=%v elapsed=%v", e, time.Since(started))
			}
		})
	}
}

type handoffTestSigner struct {
	key    ed25519.PrivateKey
	calls  int
	fail   bool
	before func()
}

func (s *handoffTestSigner) Sign(ctx context.Context, b []byte) ([]byte, error) {
	s.calls++
	prefix := []byte("gh-runnerd/g01-issued-credential-binding/v1\x00")
	if !bytes.HasPrefix(b, prefix) {
		return nil, errBroker
	}
	var claims handoff.BindingClaims
	if json.Unmarshal(b[len(prefix):], &claims) != nil {
		return nil, errBroker
	}
	deadline, ok := ctx.Deadline()
	if !ok || !deadline.Equal(claims.ExpiresAt) || deadline.After(time.Now().Add(handoff.MaxProcessDuration)) {
		return nil, errBroker
	}
	if s.before != nil {
		s.before()
	}
	if s.fail || ctx.Err() != nil {
		return nil, errBroker
	}
	return ed25519.Sign(s.key, b), nil
}
func testHandoffAssembly(t *testing.T, api *brokerAPI) (*brokerHandoffAssembly, *handoffTestSigner) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{71}, 32))
	issuer, e := handoff.NewIssuerRoot("fixture-issuer", key.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	signer := &handoffTestSigner{key: key}
	return &brokerHandoffAssembly{signer: signer, issuer: issuer, workflow: api.provenanceRoot, source: api.provenance.Source(), launch: func(context.Context, *issuedControllerHandoff, string) error { return errBroker }}, signer
}
func TestBrokerHandoffAssemblyRefusesBeforeManagementInput(t *testing.T) {
	for _, fault := range []string{"missing signer", "missing issuer", "missing workflow", "same role key", "same ID different key"} {
		t.Run(fault, func(t *testing.T) {
			entry := newPairedBrokerEntryFixture(t)
			assembly, signer := testHandoffAssembly(t, entry.api)
			var a BrokerApproval
			raw, _ := os.ReadFile(entry.files.ApprovalPath)
			if json.Unmarshal(raw, &a) != nil {
				t.Fatal("approval")
			}
			a.Mode = "controller"
			a.Phase = "create"
			raw, _ = json.Marshal(a)
			if os.WriteFile(entry.files.ApprovalPath, raw, 0600) != nil {
				t.Fatal("approval")
			}
			files := entry.files
			files.WorkerApproval = ""
			files.WorkerStateDirectory = ""
			switch fault {
			case "missing signer":
				assembly.signer = nil
			case "missing issuer":
				assembly.issuer = handoff.IssuerRoot{}
			case "missing workflow":
				assembly.workflow = handoff.TrustRoot{}
			case "same role key":
				adapter := entry.api.provenance.(*signedBrokerFixtureAdapter)
				assembly.issuer, _ = handoff.NewIssuerRoot("other-role", adapter.public)
			case "same ID different key":
				other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{72}, 32))
				assembly.workflow, _ = handoff.NewTrustRoot(entry.api.provenanceRoot.KeyID(), other.Public().(ed25519.PublicKey))
			}
			inputPath := filepath.Join(entry.parent, "unread-management")
			if os.WriteFile(inputPath, entry.input, 0600) != nil {
				t.Fatal("input")
			}
			input, e := os.Open(inputPath)
			if e != nil {
				t.Fatal(e)
			}
			defer input.Close()
			if _, e := runBrokerWithPrivateHandoff(context.Background(), files, input, entry.api, assembly); e == nil {
				t.Fatal("invalid private assembly accepted")
			}
			pos, e := input.Seek(0, io.SeekCurrent)
			if e != nil || pos != 0 || len(entry.fixture.calls) != 0 || signer.calls != 0 {
				t.Fatal("invalid roots read management input or invoked effects")
			}
		})
	}
}
func TestBrokerHandoffPostMintGuardAndPermanentAttempt(t *testing.T) {
	binary := testBrokerBinary(t)
	for _, fault := range []string{"positive", "preflight", "claim loss", "sign failure", "claim loss during sign", "launch failure", "child early-exit", "child blocked-control", "child blocked-input", "child overflow", "child canceled", "child start-fail", "child failure"} {
		t.Run(fault, func(t *testing.T) {
			a, c, api, f, path := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			legacy := 0
			plan := brokerTestPlan(t, &a, filepath.Dir(path), func(context.Context, []byte, string) error { legacy++; return nil })
			plan.controller.WorkflowRunID = 7
			plan.controller.WorkflowRef = "refs/heads/main"
			plan.raw, _ = json.Marshal(plan.controller)
			a.ControllerApprovalSHA256 = handoff.Digest(plan.raw)
			plan.approval = a
			request, e := brokerProvenanceRequest(a, plan.controller, api.provenance.Source())
			if e != nil {
				t.Fatal(e)
			}
			receipt, e := api.provenance.Attest(context.Background(), request)
			if e != nil {
				t.Fatal(e)
			}
			plan.receiptRaw, _ = json.Marshal(receipt)
			assembly, signer := testHandoffAssembly(t, api)
			plan.handoff = assembly
			launches := 0
			assembly.launch = func(ctx context.Context, h *issuedControllerHandoff, _ string) error {
				launches++
				b, e := handoff.DecodeStrictBinding(h.envelope)
				deadline, ok := ctx.Deadline()
				if e != nil || !ok || !b.ExpiresAt.Equal(h.deadline) || !deadline.Equal(h.deadline) || b.ExpiresAt.After(time.Now().Add(handoff.MaxProcessDuration)) || b.ExpiresAt.After(receipt.ExpiresAt) || b.ExpiresAt.After(a.ExpiresAt) || len(h.payload) != b.PayloadLength || handoff.Digest(h.payload) != b.PayloadSHA256 || !bytes.Equal(h.receipt, plan.receiptRaw) {
					t.Fatal("issued exact bytes/deadline lost")
				}
				if strings.HasPrefix(fault, "child ") {
					mode := strings.TrimPrefix(fault, "child ")
					if mode == "failure" {
						mode = "child-fail"
					}
					runCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
					defer cancel()
					if mode == "canceled" {
						cancel()
					}
					root := filepath.Dir(path)
					if mode == "start-fail" {
						root = filepath.Join(root, "absent")
					}
					started := time.Now()
					e := invokeBrokerControllerHandoff(runCtx, binary, root, filepath.Join(filepath.Dir(path), "handoff-"+mode), plan.statePath, h)
					if e == nil || time.Since(started) > 2*time.Second {
						t.Fatal("child failure did not return bounded refusal")
					}
					return e
				}
				if fault == "launch failure" {
					return errBroker
				}
				return nil
			}
			if fault == "preflight" {
				f.badGroup = true
			}
			if fault == "sign failure" {
				signer.fail = true
			}
			if fault == "claim loss" {
				// Lose held admission after preflight's last response, before signing.
				api.client.Transport = handoffClaimLossTransport{inner: api.client.Transport, after: func() { os.Rename(f.admissionRoot, f.admissionRoot+"-lost") }}
			}
			if fault == "claim loss during sign" {
				signer.before = func() { os.Rename(f.admissionRoot, f.admissionRoot+"-lost") }
			}
			result, e := brokerExecuteWithProvenance(context.Background(), a, brokerInput{PEM: string(c.PEM)}, path, api, plan, &receipt)
			if (e == nil) != (fault == "positive") || f.tokenCalls != 1 || legacy != 0 {
				t.Fatalf("unsafe broker path: result=%+v err=%v mints=%d legacy=%d", result, e, f.tokenCalls, legacy)
			}
			wantSigns, wantLaunches := 1, 1
			if fault == "preflight" || fault == "claim loss" {
				wantSigns, wantLaunches = 0, 0
			}
			if fault == "sign failure" || fault == "claim loss during sign" {
				wantLaunches = 0
			}
			if signer.calls != wantSigns || launches != wantLaunches {
				t.Fatalf("unguarded handoff: signs=%d launches=%d", signer.calls, launches)
			}
			before := f.tokenCalls
			if _, e := brokerExecuteWithProvenance(context.Background(), a, brokerInput{PEM: string(c.PEM)}, path, api, plan, &receipt); e == nil || f.tokenCalls != before || signer.calls != wantSigns || launches != wantLaunches {
				t.Fatal("ambiguous attempt retried")
			}
			stored, _ := os.ReadFile(filepath.Join(path, "broker.jsonl"))
			for _, forbidden := range []string{f.token, "payload_sha256", "payload_length", "g01-issued-credential-binding"} {
				if bytes.Contains(stored, []byte(forbidden)) {
					t.Fatal("issuance secrets persisted")
				}
			}
		})
	}
}

type handoffClaimLossTransport struct {
	inner interface {
		RoundTrip(*http.Request) (*http.Response, error)
	}
	after func()
}

func (t handoffClaimLossTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	res, e := t.inner.RoundTrip(r)
	if strings.HasSuffix(r.URL.Path, "/runner-groups/3/repositories") {
		t.after()
	}
	return res, e
}
