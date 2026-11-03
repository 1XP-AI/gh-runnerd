package enrollment

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrokerExperimentV3ManagementTokenRevokedBeforeClaimCompletion(t *testing.T) {
	fixture := newBrokerV3RunFixture(t, "create", false)
	result, runErr := executeBrokerV3Fixture(t, context.Background(), fixture)
	if runErr != nil || result.Status != "controller_completed" {
		t.Fatalf("controller phase did not complete: result=%+v err=%v", result, runErr)
	}
	if got := brokerFixtureCallCount(fixture.fixture.calls, "DELETE /installation/token"); got != 2 {
		t.Fatalf("read-only and management installation tokens revoked=%d, want 2; calls=%v", got, fixture.fixture.calls)
	}

	ledger, err := os.ReadFile(filepath.Join(fixture.fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ledger, []byte(`"kind":"management_token_lifecycle_started"`)) || !bytes.Contains(ledger, []byte(`"kind":"management_token_lifecycle_confirmed"`)) {
		t.Fatalf("management-token lifecycle was not durably paired: %s", ledger)
	}
	if bytes.Contains(ledger, []byte("synthetic-private-installation-token")) {
		t.Fatal("management token was persisted in the account ledger")
	}

	journal, err := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(journal, []byte(`"management_token_revocation_started"`)) || !bytes.Contains(journal, []byte(`"management_token_revoked"`)) {
		t.Fatalf("management-token revocation lacks attempt-journal evidence: %s", journal)
	}
	if bytes.Contains(journal, []byte("synthetic-private-installation-token")) {
		t.Fatal("management token was persisted in the attempt journal")
	}
	journalText := string(journal)
	revocationStarted := strings.Index(journalText, "management_token_revocation_started")
	revocationConfirmed := strings.Index(journalText, "management_token_revoked")
	controllerCompleted := strings.Index(journalText, "controller_completed")
	if revocationStarted < 0 || revocationConfirmed <= revocationStarted || controllerCompleted <= revocationConfirmed {
		t.Fatalf("controller completion was not journaled after token cleanup: %s", journal)
	}
}

func TestBrokerExperimentV3RevokesKnownRejectedManagementTokens(t *testing.T) {
	for _, fault := range []string{"read-only-scope", "expired"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newBrokerV3RunFixture(t, "create", false)
			fixture.fixture.managementTokenFault = fault
			result, runErr := executeBrokerV3Fixture(t, context.Background(), fixture)
			if runErr == nil || result.Status != "" || fixture.fixture.tokenCalls != 2 {
				t.Fatalf("rejected management-token response crossed the phase boundary: result=%+v err=%v mints=%d calls=%v", result, runErr, fixture.fixture.tokenCalls, fixture.fixture.calls)
			}
			if got := brokerFixtureCallCount(fixture.fixture.calls, "DELETE /installation/token"); got != 2 {
				t.Fatalf("known rejected tokens revoked=%d, want read plus management token: %v", got, fixture.fixture.calls)
			}
			if got := brokerFixtureCallCount(fixture.fixture.calls, "GET /installation/repositories"); got != 1 {
				t.Fatalf("rejected management token reached provider preflight: repository reads=%d calls=%v", got, fixture.fixture.calls)
			}
			journal, err := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(journal, []byte(`"management_token_revocation_started"`)) || !bytes.Contains(journal, []byte(`"management_token_revoked"`)) || bytes.Contains(journal, []byte(`"controller_handoff_started"`)) {
				t.Fatalf("rejected management-token result lacks cleanup evidence or crossed handoff: %s", journal)
			}
		})
	}
}

func TestBrokerExperimentV3LostManagementTokenResponseBlocksLaterInspection(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	create.fixture.loseManagementTokenReply = true
	result, createErr := executeBrokerV3Fixture(t, context.Background(), create)
	if createErr == nil || result.Status != "" || create.fixture.tokenCalls != 2 || brokerFixtureCallCount(create.fixture.calls, "DELETE /installation/token") != 1 {
		t.Fatalf("lost management-token response was not retained as uncertain: result=%+v err=%v mints=%d calls=%v", result, createErr, create.fixture.tokenCalls, create.fixture.calls)
	}
	ledger, err := os.ReadFile(filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !bytes.Contains(ledger, []byte(`"kind":"management_token_lifecycle_started"`)) || bytes.Contains(ledger, []byte(`"kind":"management_token_lifecycle_confirmed"`)) {
		t.Fatalf("lost management-token response did not leave account lifecycle pending: err=%v ledger=%s", err, ledger)
	}

	inspect := newBrokerV3RunFixture(t, "inspect", false)
	inspect.fixture.admissionRoot = create.fixture.admissionRoot
	inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
	inspectResult, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
	if inspectErr == nil || inspectResult.Status != "" || inspect.fixture.tokenCalls != 0 || len(inspect.fixture.calls) != 0 {
		t.Fatalf("inspection crossed unresolved management-token lifecycle: result=%+v err=%v mints=%d calls=%v", inspectResult, inspectErr, inspect.fixture.tokenCalls, inspect.fixture.calls)
	}
}

func TestBrokerExperimentV3UncertainManagementTokenRevocationBlocksLaterInspection(t *testing.T) {
	create := newBrokerV3RunFixture(t, "create", false)
	create.fixture.managementTokenRevokeError = true
	result, createErr := executeBrokerV3Fixture(t, context.Background(), create)
	if createErr == nil || result.Status != "" || create.fixture.tokenCalls != 2 || brokerFixtureCallCount(create.fixture.calls, "DELETE /installation/token") != 2 {
		t.Fatalf("uncertain management-token revocation was not fail-stop: result=%+v err=%v mints=%d calls=%v", result, createErr, create.fixture.tokenCalls, create.fixture.calls)
	}
	ledger, err := os.ReadFile(filepath.Join(create.fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !bytes.Contains(ledger, []byte(`"kind":"management_token_lifecycle_started"`)) || bytes.Contains(ledger, []byte(`"kind":"management_token_lifecycle_confirmed"`)) {
		t.Fatalf("uncertain management-token revocation did not retain account intent: err=%v ledger=%s", err, ledger)
	}
	journal, err := os.ReadFile(filepath.Join(create.files.StateDirectory, "broker.jsonl"))
	if err != nil || !bytes.Contains(journal, []byte(`"management_token_revocation_uncertain"`)) {
		t.Fatalf("uncertain management-token revocation lacks attempt evidence: err=%v journal=%s", err, journal)
	}

	inspect := newBrokerV3RunFixture(t, "inspect", false)
	inspect.fixture.admissionRoot = create.fixture.admissionRoot
	inspect.files.ControllerStateDirectory = create.files.ControllerStateDirectory
	inspectResult, inspectErr := executeBrokerV3Fixture(t, context.Background(), inspect)
	if inspectErr == nil || inspectResult.Status != "" || inspect.fixture.tokenCalls != 0 || len(inspect.fixture.calls) != 0 {
		t.Fatalf("inspection crossed uncertain management-token revocation: result=%+v err=%v mints=%d calls=%v", inspectResult, inspectErr, inspect.fixture.tokenCalls, inspect.fixture.calls)
	}
}

func TestBrokerExperimentV3DoesNotRevokeCallerVerificationTokenCollision(t *testing.T) {
	fixture := newBrokerV3RunFixture(t, "create", false)
	fixture.fixture.token = "synthetic-private-workflow-token"
	result, runErr := executeBrokerV3Fixture(t, context.Background(), fixture)
	if runErr == nil || result.Status != "" || fixture.fixture.tokenCalls != 2 || brokerFixtureCallCount(fixture.fixture.calls, "DELETE /installation/token") != 1 {
		t.Fatalf("verification-token collision was not withheld from revocation: result=%+v err=%v mints=%d calls=%v", result, runErr, fixture.fixture.tokenCalls, fixture.fixture.calls)
	}
	journal, err := os.ReadFile(filepath.Join(fixture.files.StateDirectory, "broker.jsonl"))
	if err != nil || !bytes.Contains(journal, []byte(`"management_token_revocation_skipped"`)) || !bytes.Contains(journal, []byte("verification_token_collision")) {
		t.Fatalf("verification-token collision lacks safe skip evidence: err=%v journal=%s", err, journal)
	}
	ledger, err := os.ReadFile(filepath.Join(fixture.fixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !bytes.Contains(ledger, []byte(`"kind":"management_token_lifecycle_started"`)) || bytes.Contains(ledger, []byte(`"kind":"management_token_lifecycle_confirmed"`)) {
		t.Fatalf("verification-token collision did not keep account lifecycle pending: err=%v ledger=%s", err, ledger)
	}
}
