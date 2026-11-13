//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

type v3ProcessFixtureTransport struct {
	base   *processBrokerHTTP
	callee handoff.CalleeIdentity
}

func (f *v3ProcessFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := f.base.RoundTrip(r)
	if err != nil || r.URL.Path != workflowRunFixturePath(f.base) {
		return response, err
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	_ = response.Body.Close()
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	payload["referenced_workflows"] = []any{map[string]any{
		"path": f.callee.Repository + "/" + f.callee.WorkflowPath + "@" + f.callee.SHA,
		"sha":  f.callee.SHA,
	}}
	data, err = json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
	return response, nil
}

type controllerV3ProcessFixture struct {
	controller controllerV3HandoffFixture
	files      enrollment.BrokerFiles
	grant      enrollment.WorkflowGrantForFixture
	transport  *v3ProcessFixtureTransport
}

func canonicalFixtureTempRoot(root string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", os.ErrInvalid
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(canonical) || filepath.Clean(canonical) != canonical {
		return "", os.ErrInvalid
	}
	return canonical, nil
}

func newControllerV3ProcessFixture(t *testing.T) controllerV3ProcessFixture {
	t.Helper()
	controller := newControllerV3HandoffFixture(t, "create")
	var policy handoff.PublicPolicy
	if err := json.Unmarshal(controller.policyRaw, &policy); err != nil {
		t.Fatal(err)
	}
	// The copied TEST child carries these independently compiled fixture root
	// IDs; keep the policy's public keys unchanged while matching those pins.
	policy.WorkflowKeyID = "fixture-ed25519-v1"
	policy.IssuerKeyID = "fixture-issuer-v1"
	var err error
	controller.policyRaw, err = json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	controller.workflow, err = handoff.NewTrustRoot(policy.WorkflowKeyID, controller.workflowKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	issuerKey := ed25519.NewKeyFromSeed(stringsToBytes(71, ed25519.SeedSize))
	controller.issuer, err = handoff.NewIssuerRoot(policy.IssuerKeyID, issuerKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	tempRoot, err := canonicalFixtureTempRoot(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := os.MkdirTemp(tempRoot, "g01-v3-process-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(parent) })
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"admission", "controller-state"} {
		if err := os.Mkdir(filepath.Join(parent, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	state := filepath.Join(parent, "controller-state")
	attempt := filepath.Join(parent, "attempt")
	approvalPath := filepath.Join(parent, "controller-approval.json")
	brokerApprovalPath := filepath.Join(parent, "broker-approval.json")
	policyPath := filepath.Join(state, "controller-policy.json")
	if err := os.WriteFile(approvalPath, controller.snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, controller.policyRaw, 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binaryData, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(parent, "fixture-controller")
	if err := os.WriteFile(binaryPath, binaryData, 0500); err != nil {
		t.Fatal(err)
	}
	approval := controller.approval
	brokerApproval := enrollment.BrokerApproval{
		GroupPolicy:              approval.GroupPolicy,
		ManagementScope:          approval.ManagementScope,
		OwnerNonce:               approval.OwnerNonce,
		Mode:                     "controller",
		AppID:                    approval.AppID,
		AppName:                  "fixture-app",
		AppOwner:                 approval.Organization,
		AppOwnerID:               101,
		InstallationID:           approval.InstallationID,
		Organization:             approval.Organization,
		OrganizationID:           approval.GroupPolicy.OrganizationID,
		Repository:               approval.Repository,
		RepositoryID:             approval.RepositoryID,
		RunnerGroupID:            int64(approval.RunnerGroupID),
		RunnerGroupName:          "fixture-group",
		ExpiresAt:                approval.ExpiresAt,
		Phase:                    "create",
		ControllerBinarySHA256:   handoff.Digest(binaryData),
		ControllerApprovalSHA256: handoff.Digest(controller.snapshot),
		ControllerHarnessSHA:     approval.HarnessSHA,
	}
	brokerRaw, err := json.Marshal(brokerApproval)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brokerApprovalPath, brokerRaw, 0600); err != nil {
		t.Fatal(err)
	}
	base := &processBrokerHTTP{
		t: t, attempt: attempt, admissionDirectory: filepath.Join(parent, "admission"), approval: &approval,
		expectedWorkflowAuthorizations: []string{
			"Bearer synthetic-observation-token",
			"Bearer synthetic-process-verification-token",
			"Bearer synthetic-process-verification-token",
		},
	}
	return controllerV3ProcessFixture{
		controller: controller,
		files: enrollment.BrokerFiles{
			ApprovalPath:             brokerApprovalPath,
			StateDirectory:           attempt,
			ControllerBinary:         binaryPath,
			ControllerApproval:       approvalPath,
			ControllerStateDirectory: state,
		},
		grant:     enrollment.WorkflowGrantForFixture{Request: controller.request, OrganizationID: 101, RepositoryID: approval.RepositoryID, ExpiresAt: approval.ExpiresAt},
		transport: &v3ProcessFixtureTransport{base: base, callee: controller.request.Callee},
	}
}

func controllerV3ProcessEffects(t *testing.T, fixture controllerV3ProcessFixture) map[string]int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixture.files.ControllerStateDirectory, "controller-effects.json"))
	if err != nil {
		t.Fatal(err)
	}
	var effects map[string]int
	if err := json.Unmarshal(raw, &effects); err != nil {
		t.Fatal(err)
	}
	return effects
}

func controllerV3ProcessEvents(t *testing.T, fixture controllerV3ProcessFixture) []Event {
	t.Helper()
	admission := filepath.Join(filepath.Dir(fixture.files.ControllerStateDirectory), "admission")
	j, err := openJournalAtAdmission(fixture.files.ControllerStateDirectory, fixture.controller.approval, admission, func(file *os.File) error { return file.Sync() })
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	return j.Events()
}

func TestCanonicalFixtureTempRootResolvesSymlink(t *testing.T) {
	realRoot := t.TempDir()
	aliasParent := t.TempDir()
	alias := filepath.Join(aliasParent, "tmp")
	if err := os.Symlink(realRoot, alias); err != nil {
		t.Fatal(err)
	}
	got, err := canonicalFixtureTempRoot(alias)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("fixture temp root was not canonical: got %q want %q", got, want)
	}
}

func TestControllerProcessHandoffV3ActualBrokerChild(t *testing.T) {
	for _, tc := range []struct {
		name        string
		wantSuccess bool
	}{
		{name: "selected-callee", wantSuccess: true},
		{name: "changed-callee", wantSuccess: false},
		{name: "forged-receipt", wantSuccess: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newControllerV3ProcessFixture(t)
			var observedReceipt, observedBinding []byte
			options := enrollment.ProcessHandoffFixtureOptions{
				ObserveHandoff: func(receipt, envelope []byte) {
					observedReceipt, observedBinding = receipt, envelope
				},
				ObserveAppJWTKey: func(key *rsa.PublicKey) {
					fixture.transport.base.appPublicKey = key
				},
			}
			switch tc.name {
			case "changed-callee":
				options.MutateReceipt = func(raw []byte) []byte {
					receipt, err := handoff.DecodeStrictReceipt(raw)
					if err != nil {
						t.Fatalf("decode receipt for changed-callee fault: %v", err)
					}
					receipt.Callee.SHA = strings.Repeat("f", 40)
					return signV3Receipt(t, receipt, fixture.controller.workflowKey)
				}
			case "forged-receipt":
				options.MutateReceipt = func(raw []byte) []byte {
					receipt, err := handoff.DecodeStrictReceipt(raw)
					if err != nil {
						t.Fatalf("decode receipt for forged fault: %v", err)
					}
					receipt.Signature = strings.Repeat("A", len(receipt.Signature))
					mutated, err := json.Marshal(receipt)
					if err != nil {
						t.Fatalf("encode forged receipt: %v", err)
					}
					return mutated
				}
			}
			// Fault injection is at the broker-to-child boundary: broker issuance
			// and minting remain observable, while the child must quarantine before
			// its journal consumption, factory, or create effect.
			result, effects, err := enrollment.RunProcessHandoffForFixtureWithOptions(context.Background(), fixture.files, fixture.transport, fixture.grant, options)
			controllerEffects := controllerV3ProcessEffects(t, fixture)
			events := controllerV3ProcessEvents(t, fixture)
			assertProcessBrokerTrace(t, fixture.transport.base, expectedProcessBrokerV3Trace(*fixture.transport.base.approval, false), "")
			assertProcessBrokerTokenLifecycle(t, fixture.files.StateDirectory, filepath.Join(filepath.Dir(fixture.files.StateDirectory), "admission"), true, true)
			if tc.wantSuccess {
				receipt, receiptErr := handoff.DecodeStrictReceipt(observedReceipt)
				binding, bindingErr := handoff.DecodeStrictBinding(observedBinding)
				if err != nil || result.Status != "controller_completed" || effects.WorkflowSignatures != 1 || effects.BindingSignatures != 1 || fixture.transport.base.mints != 2 || fixture.transport.base.revocations != 2 || fixture.transport.base.readOnlyRevocations != 1 || fixture.transport.base.managementRevocations != 1 || receiptErr != nil || bindingErr != nil || receipt.Callee != fixture.controller.request.Callee || binding.Request != fixture.controller.request || controllerEffects["factory_calls"] != 1 || controllerEffects["create_calls"] != 1 || len(events) != 7 {
					t.Fatalf("positive child handoff: result=%+v err=%v effects=%+v mints=%d calls=%v controller_effects=%v journal_events=%d receipt_err=%v binding_err=%v", result, err, effects, fixture.transport.base.mints, fixture.transport.base.calls, controllerEffects, len(events), receiptErr, bindingErr)
				}
				return
			}
			if err == nil || result.Status == "controller_completed" || effects.WorkflowSignatures != 1 || effects.BindingSignatures != 1 || fixture.transport.base.mints != 2 || fixture.transport.base.revocations != 2 || fixture.transport.base.readOnlyRevocations != 1 || fixture.transport.base.managementRevocations != 1 || controllerEffects["factory_calls"] != 0 || controllerEffects["create_calls"] != 0 || len(events) != 0 {
				t.Fatalf("rejected child handoff crossed effect boundary: result=%+v err=%v effects=%+v mints=%d calls=%v controller_effects=%v journal_events=%d", result, err, effects, fixture.transport.base.mints, fixture.transport.base.calls, controllerEffects, len(events))
			}
		})
	}
}

func TestControllerProcessHandoffV3FixtureUsesSelectedCallee(t *testing.T) {
	fixture := newControllerV3ProcessFixture(t)
	if fixture.controller.request.Source != handoff.ReusableCalleeSource || fixture.controller.request.Callee.WorkflowPath != ".github/workflows/runner.yml" || !strings.HasPrefix(fixture.controller.request.Callee.SHA, "d") {
		t.Fatal("fixture did not select the reusable callee")
	}
}
