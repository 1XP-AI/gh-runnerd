package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type manualIdentityAPI struct {
	*driverFake
	identityCalls  int
	beforeIdentity func()
	beforeBindings func()
	identityError  error
}

func (a *manualIdentityAPI) DescribeApp(ctx context.Context, credential Credential) (AppIdentity, error) {
	a.identityCalls++
	if a.beforeIdentity != nil {
		a.beforeIdentity()
	}
	if a.identityError != nil {
		return AppIdentity{}, a.identityError
	}
	return a.driverFake.DescribeApp(ctx, credential)
}

func (a *manualIdentityAPI) App(ctx context.Context, credential Credential) (int64, error) {
	if a.beforeBindings != nil {
		a.beforeBindings()
	}
	return a.fakeAPI.App(ctx, credential)
}

func readAttempt(t *testing.T, path string) (attemptRecord, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(path, "attempt.json"))
	if err != nil {
		t.Fatal("synthetic attempt unavailable")
	}
	var record attemptRecord
	if json.Unmarshal(data, &record) != nil {
		t.Fatal("synthetic attempt malformed")
	}
	return record, data
}

func seedIdentityPhase(t *testing.T, path string, p Proposal, phase string, appID int64) {
	t.Helper()
	j, err := openJournal(path, p, false, 0)
	if err != nil {
		t.Fatal("synthetic journal setup failed")
	}
	defer j.close()
	if j.phase(phase, appID, nil) != nil {
		t.Fatal("synthetic phase setup failed")
	}
}

func TestManualIdentityCorrectionBeforeAuthentication(t *testing.T) {
	candidate := syntheticCandidate(t)
	p := driverProposal()
	p.Organizations = candidate.Organizations
	for _, first := range []string{"valid-key", "invalid-key", "legacy-prepared"} {
		t.Run(first, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "attempt")
			api := &manualIdentityAPI{driverFake: &driverFake{fakeAPI: validAPI(), candidate: candidate}}
			if first == "legacy-prepared" {
				// Version 1 from PR26 persisted this unverified ID before reading PEM.
				seedIdentityPhase(t, path, p, "prepared", 72)
			} else {
				input := string(candidate.PEM)
				if first == "invalid-key" {
					input = "synthetic invalid PEM"
				}
				if _, err := VerifyManual(context.Background(), p, path, 72, io.NopCloser(strings.NewReader(input)), api); err == nil {
					t.Fatal("incorrect App ID unexpectedly verified")
				}
			}
			before, beforeBytes := readAttempt(t, path)
			api.beforeIdentity = func() {
				_, current := readAttempt(t, path)
				if !bytes.Equal(beforeBytes, current) {
					t.Error("correction rewrote the existing record before authentication")
				}
			}
			calls := api.identityCalls
			result, err := VerifyManual(context.Background(), p, path, 71, io.NopCloser(bytes.NewReader(candidate.PEM)), api)
			if err != nil || result.VerifiedOrganizations != 2 || api.identityCalls-calls != 1 {
				t.Fatalf("corrected ID blocked: phase=%s app_id=%d new_identity_checks=%d verified_organizations=%d", before.Phase, before.AppID, api.identityCalls-calls, result.VerifiedOrganizations)
			}
			record, _ := readAttempt(t, path)
			if record.Phase != "verified" || record.AppID != 71 {
				t.Fatal("authenticated correction not pinned")
			}
			assertNoSecretFiles(t, path, string(candidate.PEM), "PRIVATE KEY")
		})
	}
}

func TestManualIdentitySameIDInvalidPEMRecoveryControl(t *testing.T) {
	candidate := syntheticCandidate(t)
	p := driverProposal()
	p.Organizations = candidate.Organizations
	api := &driverFake{fakeAPI: validAPI(), candidate: candidate}
	path := filepath.Join(t.TempDir(), "attempt")
	if _, err := VerifyManual(context.Background(), p, path, 71, io.NopCloser(strings.NewReader("synthetic invalid PEM")), api); err == nil {
		t.Fatal("invalid PEM accepted")
	}
	result, err := VerifyManual(context.Background(), p, path, 71, io.NopCloser(bytes.NewReader(candidate.PEM)), api)
	if err != nil || result.VerifiedOrganizations != 2 {
		t.Fatal("same-ID PEM correction failed")
	}
}

func TestManualIdentityIsUnpinnedUntilAuthenticated(t *testing.T) {
	candidate := syntheticCandidate(t)
	p := driverProposal()
	p.Organizations = candidate.Organizations
	path := filepath.Join(t.TempDir(), "attempt")
	api := &manualIdentityAPI{driverFake: &driverFake{fakeAPI: validAPI(), candidate: candidate}, identityError: errors.New("synthetic identity error")}
	api.beforeIdentity = func() {
		record, _ := readAttempt(t, path)
		if record.Phase != "prepared" || record.AppID != 0 {
			t.Error("candidate App ID persisted before authentication")
		}
	}
	if _, err := VerifyManual(context.Background(), p, path, 71, io.NopCloser(bytes.NewReader(candidate.PEM)), api); err == nil {
		t.Fatal("failed identity lookup accepted")
	}
	record, _ := readAttempt(t, path)
	if record.Phase != "prepared" || record.AppID != 0 {
		t.Error("identity lookup failure pinned the candidate")
	}
	api.identityError = nil
	api.fakeAPI.err = errors.New("synthetic binding error")
	api.beforeBindings = func() {
		record, _ := readAttempt(t, path)
		if record.Phase != "verifying" || record.AppID != 71 {
			t.Error("authenticated identity not durable before binding checks")
		}
	}
	if _, err := VerifyManual(context.Background(), p, path, 71, io.NopCloser(bytes.NewReader(candidate.PEM)), api); err == nil {
		t.Fatal("failed binding lookup accepted")
	}
	record, _ = readAttempt(t, path)
	if record.Phase != "verification_failed" || record.AppID != 71 {
		t.Fatal("binding failure lost authenticated identity")
	}
}

func TestManualIdentityCannotChangeAfterPrepared(t *testing.T) {
	candidate := syntheticCandidate(t)
	p := driverProposal()
	p.Organizations = candidate.Organizations
	for _, phase := range []string{"registration_started", "conversion_started", "conversion_failed", "app_received", "verifying", "verification_failed", "verified"} {
		t.Run(phase, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "attempt")
			seedIdentityPhase(t, path, p, phase, 72)
			_, before := readAttempt(t, path)
			api := &manualIdentityAPI{driverFake: &driverFake{fakeAPI: validAPI(), candidate: candidate}}
			if _, err := VerifyManual(context.Background(), p, path, 71, io.NopCloser(bytes.NewReader(candidate.PEM)), api); err == nil || api.identityCalls != 0 {
				t.Fatal("recorded identity changed or reached authentication")
			}
			_, after := readAttempt(t, path)
			if !bytes.Equal(before, after) {
				t.Fatal("rejected identity changed durable inventory")
			}
		})
	}
}

func TestManualIdentityPinSurvivesLaterJournalWriteFailure(t *testing.T) {
	candidate := syntheticCandidate(t)
	p := driverProposal()
	p.Organizations = candidate.Organizations
	path := filepath.Join(t.TempDir(), "attempt")
	api := &manualIdentityAPI{driverFake: &driverFake{fakeAPI: validAPI(), candidate: candidate}}
	api.beforeBindings = func() {
		record, _ := readAttempt(t, path)
		if record.Phase != "verifying" || record.AppID != 71 {
			t.Fatal("identity not pinned before synthetic write failure")
		}
		// Simulate a leftover write after the authenticated pin became durable.
		// The verification_failed transition must now fail closed without erasing it.
		if os.WriteFile(filepath.Join(path, "record.next"), []byte("synthetic interrupted write"), 0600) != nil {
			t.Fatal("synthetic interruption setup failed")
		}
	}
	api.fakeAPI.err = errors.New("synthetic binding failure")
	if _, err := VerifyManual(context.Background(), p, path, 71, io.NopCloser(bytes.NewReader(candidate.PEM)), api); err == nil {
		t.Fatal("interrupted verification accepted")
	}
	record, before := readAttempt(t, path)
	if record.Phase != "verifying" || record.AppID != 71 {
		t.Fatal("failed write lost the authenticated identity")
	}
	calls := api.identityCalls
	if _, err := VerifyManual(context.Background(), p, path, 72, io.NopCloser(bytes.NewReader(candidate.PEM)), api); err == nil || api.identityCalls != calls {
		t.Fatal("write ambiguity allowed a different App identity")
	}
	_, after := readAttempt(t, path)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected correction changed pinned inventory")
	}
}

func TestManualIdentityPreparedCorrectionPreservesOwnershipAndNoRegistration(t *testing.T) {
	candidate := syntheticCandidate(t)
	p := driverProposal()
	p.Organizations = candidate.Organizations
	path := filepath.Join(t.TempDir(), "attempt")
	seedIdentityPhase(t, path, p, "prepared", 72)
	api := &manualIdentityAPI{driverFake: &driverFake{fakeAPI: validAPI(), candidate: candidate}}
	for _, field := range []string{"owner", "name", "organizations"} {
		changed := p
		switch field {
		case "owner":
			changed.Owner = "org-b"
		case "name":
			changed.AppName = "different-app"
		case "organizations":
			changed.Organizations = append([]Binding(nil), p.Organizations...)
			changed.Organizations[1].OrganizationID = 999
		}
		if _, err := VerifyManual(context.Background(), changed, path, 71, io.NopCloser(bytes.NewReader(candidate.PEM)), api); err == nil || api.identityCalls != 0 {
			t.Fatal("prepared correction changed ownership")
		}
	}
	if d, err := StartManifest(context.Background(), p, path, api, time.Minute); err == nil {
		d.Close()
		t.Fatal("prepared correction reopened automatic registration")
	}
}
