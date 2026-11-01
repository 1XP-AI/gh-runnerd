package enrollment

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newLegacyControllerEntryFixture(t *testing.T) pairedBrokerEntryFixture {
	t.Helper()
	entry := newPairedBrokerEntryFixture(t)
	var approval BrokerApproval
	raw, err := os.ReadFile(entry.files.ApprovalPath)
	if err != nil || json.Unmarshal(raw, &approval) != nil {
		t.Fatal("broker approval")
	}
	approval.Mode, approval.Phase = "controller", "create"
	raw, err = json.Marshal(approval)
	if err != nil || os.WriteFile(entry.files.ApprovalPath, raw, 0600) != nil {
		t.Fatal("controller approval")
	}
	entry.files.WorkerApproval = ""
	entry.files.WorkerStateDirectory = ""
	return entry
}

func assertLegacyMalformedInputLeftNoDurableAdmission(t *testing.T, entry pairedBrokerEntryFixture) {
	t.Helper()
	ledgerPath := filepath.Join(entry.fixture.admissionRoot, "broker-admission.jsonl")
	if data, err := os.ReadFile(ledgerPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			var event brokerClaimEvent
			if json.Unmarshal([]byte(line), &event) == nil && event.Kind == "claim" {
				t.Fatalf("malformed input left a durable broker claim: %s", line)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read broker admission ledger: %v", err)
	}
	paths := []string{
		ledgerPath,
		filepath.Join(entry.fixture.admissionRoot, "broker-admission.lock"),
		filepath.Join(entry.files.ControllerStateDirectory, "journal.jsonl"),
		filepath.Join(entry.files.ControllerStateDirectory, "admission.json"),
	}
	if entry.files.WorkerStateDirectory != "" {
		paths = append(paths,
			filepath.Join(entry.files.WorkerStateDirectory, "journal.jsonl"),
			filepath.Join(entry.files.WorkerStateDirectory, "admission.json"),
		)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("malformed input left durable preparation state %q: %v", path, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(entry.files.StateDirectory, "broker.jsonl"))
	if err != nil {
		t.Fatalf("read retained attempt journal: %v", err)
	}
	if strings.Contains(string(data), `"phase":"controller_preparation_started"`) || strings.Contains(string(data), `"phase":"token_request_started"`) {
		t.Fatalf("malformed input reached preparation or management intent: %s", data)
	}
}

func retryLegacyEntryOnFreshAttempt(t *testing.T, entry *pairedBrokerEntryFixture) {
	t.Helper()
	entry.files.StateDirectory = filepath.Join(entry.parent, "retry-attempt")
	entry.fixture.root = entry.files.StateDirectory
	result, err := entry.run(t, entry.files.ControllerStateDirectory, entry.files.WorkerStateDirectory)
	if err != nil || result.Status == "" || entry.fixture.tokenCalls != 1 {
		t.Fatalf("fresh approved attempt did not recover after malformed input: result=%+v err=%v mints=%d", result, err, entry.fixture.tokenCalls)
	}
}

func TestLegacyControllerEntryRejectsMalformedInputBeforeAdmission(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "malformed JSON", data: []byte(`{"pem":`)},
		{name: "malformed PEM", data: func() []byte {
			data, _ := json.Marshal(brokerInput{PEM: "not a PEM credential", VerificationToken: "synthetic-private-verification-token"})
			return data
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := newLegacyControllerEntryFixture(t)
			validInput := append([]byte(nil), entry.input...)
			entry.input = tc.data
			if _, err := entry.run(t, entry.files.ControllerStateDirectory, entry.files.WorkerStateDirectory); err == nil {
				t.Fatal("malformed legacy controller input was accepted")
			}
			if entry.fixture.tokenCalls != 0 || len(entry.fixture.calls) != 0 {
				t.Fatalf("malformed legacy controller input reached authenticated effects: mints=%d calls=%v", entry.fixture.tokenCalls, entry.fixture.calls)
			}
			assertLegacyMalformedInputLeftNoDurableAdmission(t, entry)

			// The consumed attempt remains permanently non-retryable; a fresh
			// approved attempt is the only recovery path.
			entry.input = validInput
			if _, err := entry.run(t, entry.files.ControllerStateDirectory, entry.files.WorkerStateDirectory); err == nil || entry.fixture.tokenCalls != 0 {
				t.Fatalf("malformed attempt was retried in place: err=%v mints=%d", err, entry.fixture.tokenCalls)
			}
			entry.input = validInput
			retryLegacyEntryOnFreshAttempt(t, &entry)
		})
	}
}

func TestLegacyPairedEntryRejectsMalformedInputBeforeAdmission(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "malformed JSON", data: []byte(`{"pem":`)},
		{name: "malformed PEM", data: func() []byte {
			data, _ := json.Marshal(brokerInput{PEM: "not a PEM credential", VerificationToken: "synthetic-private-verification-token"})
			return data
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := newPairedBrokerEntryFixture(t)
			validInput := append([]byte(nil), entry.input...)
			entry.input = tc.data
			if _, err := entry.run(t, entry.files.ControllerStateDirectory, entry.files.WorkerStateDirectory); err == nil {
				t.Fatal("malformed legacy paired input was accepted")
			}
			if entry.fixture.tokenCalls != 0 || len(entry.fixture.calls) != 0 {
				t.Fatalf("malformed legacy paired input reached authenticated effects: mints=%d calls=%v", entry.fixture.tokenCalls, entry.fixture.calls)
			}
			assertLegacyMalformedInputLeftNoDurableAdmission(t, entry)

			entry.input = validInput
			if _, err := entry.run(t, entry.files.ControllerStateDirectory, entry.files.WorkerStateDirectory); err == nil || entry.fixture.tokenCalls != 0 {
				t.Fatalf("malformed paired attempt was retried in place: err=%v mints=%d", err, entry.fixture.tokenCalls)
			}
			entry.input = validInput
			retryLegacyEntryOnFreshAttempt(t, &entry)
		})
	}
}
