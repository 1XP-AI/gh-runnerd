package enrollment

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// BrokerApproval describes one future, explicitly approved credential issuance
// and either hostname discovery or one already-reviewed controller phase.
type BrokerApproval struct {
	GroupPolicy                handoff.GroupPolicy `json:"group_policy,omitzero"`
	ManagementScope            string              `json:"management_scope,omitempty"`
	OwnerNonce                 string              `json:"owner_nonce"`
	Mode                       string              `json:"mode"`
	AppID                      int64               `json:"app_id"`
	AppName                    string              `json:"app_name"`
	AppOwner                   string              `json:"app_owner"`
	AppOwnerID                 int64               `json:"app_owner_id"`
	InstallationID             int64               `json:"installation_id"`
	Organization               string              `json:"organization"`
	OrganizationID             int64               `json:"organization_id"`
	Repository                 string              `json:"repository"`
	RepositoryID               int64               `json:"repository_id"`
	RunnerGroupID              int64               `json:"runner_group_id"`
	RunnerGroupName            string              `json:"runner_group_name"`
	ExpiresAt                  time.Time           `json:"expires_at"`
	Phase                      string              `json:"phase,omitempty"`
	AllowVerificationAuthority bool                `json:"allow_verification_authority"`
	ControllerBinarySHA256     string              `json:"controller_binary_sha256,omitempty"`
	ControllerApprovalSHA256   string              `json:"controller_approval_sha256,omitempty"`
	ControllerHarnessSHA       string              `json:"controller_harness_sha,omitempty"`
}
type brokerInput struct {
	PEM               string `json:"pem"`
	VerificationToken string `json:"verification_token,omitempty"`
}

func (brokerInput) String() string   { return "[redacted broker input]" }
func (brokerInput) GoString() string { return "[redacted broker input]" }

type BrokerResult struct {
	Status      string `json:"status"`
	ActionsHost string `json:"actions_host,omitempty"`
}

var errBroker = errors.New("broker stopped; retain private intent and review; no automatic retry")
var brokerComponent = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
var brokerWorkerComponent = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var brokerPhases = map[string]bool{"create": true, "before-ack": true, "after-ack": true, "before-acquire": true, "acquire-loss": true, "jit-loss": true, "drain": true, "inspect": true, "cleanup": true, "session-replacement": true, "before-jit": true}
var brokerSpecialSlots = map[string]bool{"discover-actions-host": true, "paired-terminal": true}

const organizationOnlyManagementScope = "organization-only"

func brokerSlotAllowed(slot string) bool { return brokerPhases[slot] || brokerSpecialSlots[slot] }

// One header, one claim and one completion for every finite schema slot, and
// the final empty split element from the required trailing newline. This is a
// structural bound derived from the actual slot schema, not an unbounded log.
func brokerLedgerMaxLines() int { return 1 + 2*(len(brokerPhases)+len(brokerSpecialSlots)) + 1 }

const (
	// Terminal collection has seven five-second cadence gaps in production.
	// Keep a complete cadence plus a separate execution margin available to the
	// child, and reserve one more minute for the short-lived installation token
	// context that precedes it.
	pairedTerminalCadenceBudget      = 35 * time.Second
	pairedTerminalChildMargin        = 25 * time.Second
	pairedTerminalMinimumChildBudget = pairedTerminalCadenceBudget + pairedTerminalChildMargin
	pairedTerminalCredentialMargin   = time.Minute
	pairedTerminalMinimumAuthority   = pairedTerminalMinimumChildBudget + pairedTerminalCredentialMargin
	pairedTerminalMaximumChildBudget = 10 * time.Minute
)

func (a BrokerApproval) validate(now time.Time) error {
	if !a.GroupPolicy.Valid(a.Organization+"/"+a.Repository, "") || (a.GroupPolicy.Version == 2 && (a.GroupPolicy.OrganizationID != a.OrganizationID || a.Mode == "discover-actions-host")) {
		return errBroker
	}
	if a.ManagementScope != "" && a.ManagementScope != organizationOnlyManagementScope {
		return errBroker
	}
	minimumLifetime := time.Minute
	if a.Mode == "paired-terminal" {
		minimumLifetime = pairedTerminalMinimumAuthority
	}
	if !brokerNonce.MatchString(a.OwnerNonce) || a.AppID < 1 || a.AppOwnerID < 1 || a.InstallationID < 1 || a.OrganizationID < 1 || a.RepositoryID < 1 || a.RunnerGroupID < 1 || !appSlug.MatchString(a.AppName) || !organizationLogin.MatchString(a.AppOwner) || !organizationLogin.MatchString(a.Organization) || !brokerComponent.MatchString(a.Repository) || !brokerComponent.MatchString(a.RunnerGroupName) || !a.ExpiresAt.After(now.Add(minimumLifetime)) || a.ExpiresAt.After(now.Add(24*time.Hour)) {
		return errBroker
	}
	if a.Mode == "discover-actions-host" {
		if a.Phase != "" || a.AllowVerificationAuthority {
			return errBroker
		}
	} else if a.Mode == "controller" {
		if !brokerPhases[a.Phase] {
			return errBroker
		}
	} else if a.Mode == "paired-terminal" {
		if a.Phase != "paired-terminal" {
			return errBroker
		}
	} else {
		return errBroker
	}
	return nil
}
func validBrokerToken(token string) bool {
	if len(token) < 20 || len(token) > 16384 {
		return false
	}
	for _, c := range token {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

func brokerPairedAuthorityDeadline(parent context.Context, a BrokerApproval, plan *brokerControllerPlan, now time.Time) (time.Time, error) {
	if parent == nil || parent.Err() != nil {
		return time.Time{}, errBroker
	}
	deadline := a.ExpiresAt
	if plan != nil {
		if plan.controller.ExpiresAt.IsZero() {
			return time.Time{}, errBroker
		}
		deadline = minTime(deadline, plan.controller.ExpiresAt)
		if plan.worker != nil {
			if plan.worker.approval.ExpiresAt.IsZero() {
				return time.Time{}, errBroker
			}
			deadline = minTime(deadline, plan.worker.approval.ExpiresAt)
		}
	}
	if parentDeadline, ok := parent.Deadline(); ok {
		deadline = minTime(deadline, parentDeadline)
	}
	if !deadline.After(now.Add(pairedTerminalMinimumAuthority)) {
		return time.Time{}, errBroker
	}
	return deadline, nil
}

func revokeBrokerReadOnlyToken(ctx context.Context, api *brokerAPI, j *brokerJournal, token, verificationToken string) error {
	if !validBrokerToken(token) || len(token) > 1024 {
		return errBroker
	}
	if token == verificationToken {
		if j.append("read_only_token_revocation_skipped", map[string]any{"reason": "verification_token_collision"}) != nil {
			return errBroker
		}
		return errBroker
	}
	if j.append("read_only_token_revocation_started", nil) != nil {
		return errBroker
	}
	if api.revokeInstallationToken(ctx, token) != nil {
		_ = j.append("read_only_token_revocation_uncertain", map[string]any{"reason": "revoke_request_failed_or_ambiguous"})
		return errBroker
	}
	if j.append("read_only_token_revoked", nil) != nil {
		// The revoked record may have reached disk without a durable sync. A
		// later uncertainty record is terminal and overrides that observation.
		_ = j.append("read_only_token_revocation_uncertain", map[string]any{"reason": "revocation_result_persistence_failed"})
		return errBroker
	}
	return nil
}

func brokerExecute(parent context.Context, a BrokerApproval, input brokerInput, path string, api *brokerAPI, plan *brokerControllerPlan) (BrokerResult, error) {
	return brokerExecuteWithProvenance(parent, a, input, path, api, plan, nil)
}

func brokerExecuteWithProvenance(parent context.Context, a BrokerApproval, input brokerInput, path string, api *brokerAPI, plan *brokerControllerPlan, provenance *BrokerProvenanceReceipt, claimed ...*brokerJournal) (BrokerResult, error) {
	v3ReusableCallee := plan != nil && plan.handoff != nil && plan.handoff.experimental && plan.handoff.request.Version == 2 && plan.handoff.request.Source == handoff.ReusableCalleeSource
	deferredInput := plan != nil && plan.inputSource != nil
	if parent == nil || api == nil || !brokerCanonicalPath(path) || a.validate(api.now()) != nil || (!deferredInput && ((a.AllowVerificationAuthority || v3ReusableCallee) && input.VerificationToken == "" || input.VerificationToken != "" && (!validBrokerToken(input.VerificationToken) || (!a.AllowVerificationAuthority && !v3ReusableCallee)))) || ((a.Mode == "controller" || a.Mode == "paired-terminal") && plan == nil) || (a.Mode != "controller" && a.Mode != "paired-terminal" && plan != nil) {
		return BrokerResult{}, errBroker
	}
	if len(claimed) > 1 || (len(claimed) == 1 && (claimed[0] == nil || claimed[0].path != path || claimed[0].check() != nil)) {
		return BrokerResult{}, errBroker
	}
	if provenance != nil {
		if plan == nil || (a.Mode != "controller" && a.Mode != "paired-terminal") {
			return BrokerResult{}, errBroker
		}
		request, err := brokerHandoffRequest(a, plan, provenance.Source)
		if err != nil || provenance.Validate(request, api.now()) != nil || provenance.ExpiresAt.Before(a.ExpiresAt) {
			return BrokerResult{}, errBroker
		}
		plan.provenance = provenance
	}
	if plan != nil && plan.handoff != nil && ((!plan.handoff.experimental && (a.Mode != "controller" || a.Phase != "create")) || !plan.handoff.valid(api.provenanceRoot) || provenance == nil || plan.handoff.source != provenance.Source || len(plan.receiptRaw) == 0) {
		return BrokerResult{}, errBroker
	}
	now := api.now()
	deadline := minTime(a.ExpiresAt, now.Add(10*time.Minute))
	if a.Mode == "paired-terminal" {
		var err error
		deadline, err = brokerPairedAuthorityDeadline(parent, a, plan, now)
		if err != nil {
			return BrokerResult{}, errBroker
		}
		deadline = minTime(deadline, now.Add(10*time.Minute))
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	var j *brokerJournal
	if len(claimed) == 1 {
		j = claimed[0]
	} else {
		var err error
		j, err = openBrokerJournalForExecution(parent, path, a)
		if err != nil {
			return BrokerResult{}, errBroker
		}
		defer j.close()
	}
	var cred Credential
	parseInput := func(value brokerInput) error {
		if (a.AllowVerificationAuthority || v3ReusableCallee) && value.VerificationToken == "" || value.VerificationToken != "" && (!validBrokerToken(value.VerificationToken) || (!a.AllowVerificationAuthority && !v3ReusableCallee)) {
			return errBroker
		}
		candidate := Candidate{AppID: a.AppID, PEM: []byte(value.PEM)}
		parsed, err := parseCredential(candidate)
		clear(candidate.PEM)
		if err != nil {
			return errBroker
		}
		cred = parsed
		return nil
	}
	// Non-deferred broker input is already present at this boundary. Parse it
	// before resolving or creating the persistent admission claim so malformed
	// credentials cannot burn a finite slot. Deferred v3 input remains behind
	// controller preparation and its independent root/receipt guards below.
	if !deferredInput && parseInput(input) != nil {
		return BrokerResult{}, errBroker
	}
	if api.admissionDirectory == nil {
		return BrokerResult{}, errBroker
	}
	directory, e := api.admissionDirectory()
	if e != nil {
		return BrokerResult{}, errBroker
	}
	if plan != nil {
		defer plan.close()
		if plan.prepare(a, j, api.now()) != nil {
			return BrokerResult{}, errBroker
		}
	}
	claim, e := openBrokerAdmission(directory, a, j, plan, api.syncDirectory)
	if e != nil {
		return BrokerResult{}, errBroker
	}
	defer claim.close()
	guard := func(checkPrepared bool) error {
		if claim.check() != nil || j.check() != nil {
			return errBroker
		}
		if plan != nil && (plan.check() != nil || plan.compatibleControllerClaim(claim.root) != nil || (checkPrepared && plan.prepared != nil && plan.preparedState(claim.root, false) != nil) || (checkPrepared && plan.prepared != nil && a.Mode == "paired-terminal" && (plan.worker == nil || plan.worker.checkPrepared(ctx) != nil))) {
			return errBroker
		}
		return nil
	}
	guardLive := func() error { return guard(true) }
	guardPostChild := func() error { return guard(false) }
	if guardLive() != nil {
		return BrokerResult{}, errBroker
	}
	var err error

	if plan != nil {
		if j.append("controller_preparation_started", nil) != nil {
			return BrokerResult{}, errBroker
		}
		receipt, err := plan.localPrepare(ctx, filepath.Join(path, "controller-approval.json"))
		if err != nil || !receipt.valid(plan) {
			return BrokerResult{}, errBroker
		}
		plan.preparationReceipt = receipt
		if guardLive() != nil || plan.preparedState(claim.root, true) != nil {
			return BrokerResult{}, errBroker
		}
		if j.append("controller_state_prepared", map[string]any{"receipt": receipt}) != nil || guardLive() != nil {
			return BrokerResult{}, errBroker
		}
	}
	if deferredInput {
		input, err = plan.inputSource(ctx)
		if err != nil || parseInput(input) != nil {
			if guardLive() != nil || j.append("management_input_rejected", nil) != nil || guardLive() != nil {
				return BrokerResult{}, errBroker
			}
			return BrokerResult{}, errBroker
		}
	}

	// Every authenticated call revalidates the still-held durable claim.
	scoped := newBrokerAPI(api.now, brokerGuardTransport{api.client.Transport, guardLive})
	api = scoped
	// JWT identity verification precedes token issuance. The v3 reusable-callee
	// route first uses an exact read-only installation token for provider-state
	// preflight, then repeats the final callee GET immediately before write-token
	// issuance. Other internal fixture routes retain their historical sequence.
	identity, err := api.github.DescribeApp(ctx, cred)
	if err != nil || identity.ID != a.AppID || identity.Slug != a.AppName || identity.OwnerID != a.AppOwnerID || !strings.EqualFold(identity.OwnerLogin, a.AppOwner) || identity.OwnerType != "Organization" {
		return BrokerResult{}, errBroker
	}
	install, err := api.github.OrganizationInstallation(ctx, cred, a.Organization)
	if err != nil || install.ID != a.InstallationID || install.AppID != identity.ID || install.AccountID != a.OrganizationID || install.TargetID != a.OrganizationID || install.Login != a.Organization || install.AccountType != "Organization" || install.TargetType != "Organization" || !install.SuspensionKnown || install.Suspended || !minimalPermissions(install.Permissions) {
		return BrokerResult{}, errBroker
	}
	if v3ReusableCallee {
		if j.append("token_request_started", nil) != nil {
			return BrokerResult{}, errBroker
		}
		readOnly, err := api.mintReadOnly(ctx, a, cred)
		if err != nil {
			if readOnly.Token != "" {
				_ = revokeBrokerReadOnlyToken(ctx, api, j, readOnly.Token, input.VerificationToken)
			}
			return BrokerResult{}, errBroker
		}
		if readOnly.Token != "" && readOnly.Token == input.VerificationToken {
			_ = revokeBrokerReadOnlyToken(ctx, api, j, readOnly.Token, input.VerificationToken)
			return BrokerResult{}, errBroker
		}
		readCtx, stopRead := context.WithDeadline(ctx, readOnly.ExpiresAt.Add(-time.Minute))
		preflightErr := api.preflight(readCtx, a, readOnly.Token)
		stopRead()
		readOnlyVerified := preflightErr == nil
		verifiedJournaled := true
		if readOnlyVerified {
			verifiedJournaled = j.append("read_only_token_verified", map[string]any{"expires_at": readOnly.ExpiresAt, "app_id": identity.ID, "installation_id": install.ID}) == nil
		}
		if revokeBrokerReadOnlyToken(ctx, api, j, readOnly.Token, input.VerificationToken) != nil || !readOnlyVerified || !verifiedJournaled {
			return BrokerResult{}, errBroker
		}
		// The final workflow GET is deliberately after read-only provider
		// inspection and immediately before the management write-token request.
		if api.verifyFinalReusableWorkflow(ctx, a, plan.controller, input.VerificationToken, plan.handoff.grant) != nil {
			return BrokerResult{}, errBroker
		}
		if j.append("management_token_request_started", nil) != nil {
			return BrokerResult{}, errBroker
		}
	} else {
		// Workflow identity is the final controller-authority check for the
		// internal fixture routes, using the supplied verification credential.
		if a.Mode == "paired-terminal" || (plan != nil && plan.controller.needsVerification()) {
			if provenance != nil {
				if api.verifyWorkflowWithReceipt(ctx, a, plan.controller, input.VerificationToken, provenance) != nil {
					return BrokerResult{}, errBroker
				}
			} else if api.verifyWorkflow(ctx, a, plan.controller, input.VerificationToken) != nil {
				return BrokerResult{}, errBroker
			}
		}
		if j.append("token_request_started", nil) != nil {
			return BrokerResult{}, errBroker
		}
	}
	issued, err := api.mint(ctx, a, cred)
	if err != nil {
		return BrokerResult{}, errBroker
	}
	if a.Mode == "paired-terminal" && !issued.ExpiresAt.After(api.now().Add(pairedTerminalMinimumAuthority)) {
		return BrokerResult{}, errBroker
	}
	tokenCtx, stopToken := context.WithDeadline(ctx, issued.ExpiresAt.Add(-time.Minute))
	defer stopToken()
	ctx = tokenCtx
	if input.VerificationToken == issued.Token || api.preflight(ctx, a, issued.Token) != nil {
		return BrokerResult{}, errBroker
	}
	if j.append("token_verified", map[string]any{"expires_at": issued.ExpiresAt, "app_id": identity.ID, "installation_id": install.ID}) != nil {
		return BrokerResult{}, errBroker
	}
	if v3ReusableCallee && api.verifyFinalReusableWorkflow(ctx, a, plan.controller, input.VerificationToken, plan.handoff.grant) != nil {
		return BrokerResult{}, errBroker
	}
	if a.Mode == "discover-actions-host" {
		host, err := api.discover(ctx, a, issued.Token, j)
		if err != nil || j.append("actions_host_observed", map[string]any{"actions_host": host}) != nil || claim.complete() != nil {
			return BrokerResult{}, errBroker
		}
		return BrokerResult{Status: "actions_host_observed", ActionsHost: host}, nil
	}
	// These facts are from authenticated identity reads and the actual mint
	// response. The broker does not accept user-provided token issuance claims.
	payload := struct {
		ManagementScope   string               `json:"management_scope,omitempty"`
		InstallationToken string               `json:"installation_token"`
		VerificationToken string               `json:"verification_token"`
		AppID             int64                `json:"app_id"`
		InstallationID    int64                `json:"installation_id"`
		Organization      string               `json:"organization"`
		ExpiresAt         time.Time            `json:"expires_at"`
		SelfHostedRunners string               `json:"organization_self_hosted_runners"`
		Metadata          string               `json:"metadata"`
		PairedBinding     *brokerPairedBinding `json:"paired_binding,omitempty"`
	}{a.ManagementScope, issued.Token, input.VerificationToken, identity.ID, install.ID, install.Login, issued.ExpiresAt, issued.Permissions["organization_self_hosted_runners"], issued.Permissions["metadata"], nil}
	if a.Mode == "paired-terminal" && plan.handoff != nil {
		binding, e := plan.pairedBinding()
		if e != nil {
			return BrokerResult{}, errBroker
		}
		payload.PairedBinding = &binding
	}
	data, _ := json.Marshal(payload)
	defer clear(data)
	if len(data) > 16384 || j.append("controller_handoff_started", nil) != nil {
		return BrokerResult{}, errBroker
	}
	var launchErr error
	if plan.handoff != nil {
		assembly := plan.handoff
		request, e := brokerHandoffRequest(a, plan, assembly.source)
		rawReceipt, e2 := handoff.DecodeStrictReceipt(plan.receiptRaw)
		deadline, ok := ctx.Deadline()
		if e != nil || e2 != nil || !ok || provenance == nil || (!bytes.Equal(rawReceipt.SigningBytes(), provenance.SigningBytes()) || rawReceipt.Signature != provenance.Signature) || assembly.workflow.Verify(request, rawReceipt, api.now()) != nil || guardLive() != nil || ctx.Err() != nil {
			return BrokerResult{}, errBroker
		}
		deadline = minTime(deadline, minTime(rawReceipt.ExpiresAt, plan.controller.ExpiresAt))
		if a.Mode == "controller" && a.Phase == "create" {
			deadline = minTime(deadline, api.now().Add(handoff.MaxProcessDuration))
		}
		if a.Mode == "paired-terminal" && !deadline.After(api.now().Add(handoff.MinPairedRemaining)) {
			return BrokerResult{}, errBroker
		}
		ctx, stopHandoff := context.WithDeadline(ctx, deadline)
		defer stopHandoff()
		binding := handoff.CredentialBinding{BindingClaims: handoff.BindingClaims{
			Request:       request,
			Version:       1,
			Kind:          handoff.BindingKind,
			Algorithm:     "ed25519",
			Audience:      handoff.BindingAudience,
			KeyID:         assembly.issuer.KeyID(),
			IssuerSource:  handoff.IssuerSource,
			WorkflowKeyID: rawReceipt.KeyID,
			ReceiptSHA256: handoff.Digest(plan.receiptRaw),
			PayloadSHA256: handoff.Digest(data),
			PayloadLength: len(data),
			ReceiptNonce:  rawReceipt.ReceiptNonce,
			IssuedAt:      api.now(),
			ExpiresAt:     deadline,
		}}

		expected := handoff.BindingExpected{Request: request, Receipt: rawReceipt, ReceiptBytes: plan.receiptRaw, ApprovalExpiresAt: plan.controller.ExpiresAt, Deadline: deadline}
		var envelope []byte
		if a.Mode == "controller" && a.Phase == "create" {
			signature, e := assembly.signer.Sign(ctx, binding.SigningBytes())
			if e != nil || guardLive() != nil || ctx.Err() != nil {
				clear(signature)
				return BrokerResult{}, errBroker
			}
			binding.Signature = base64.RawURLEncoding.EncodeToString(signature)
			clear(signature)
			if assembly.issuer.VerifyBinding(expected, binding, api.now()) != nil {
				return BrokerResult{}, errBroker
			}
			envelope, e = json.Marshal(binding)
			if e != nil {
				return BrokerResult{}, errBroker
			}
		} else {
			phase := handoff.PhaseBinding{BindingClaims: binding.BindingClaims, Mode: a.Mode}
			phase.Version = 2
			phase.Kind = handoff.PhaseBindingKind
			phase.Audience = handoff.PhaseAudience(a.Mode, request.Phase)
			signature, e := assembly.signer.Sign(ctx, phase.SigningBytes())
			if e != nil || guardLive() != nil || ctx.Err() != nil {
				clear(signature)
				return BrokerResult{}, errBroker
			}
			phase.Signature = base64.RawURLEncoding.EncodeToString(signature)
			clear(signature)
			if assembly.issuer.VerifyPhaseBinding(expected, phase, api.now()) != nil {
				return BrokerResult{}, errBroker
			}
			envelope, e = json.Marshal(phase)
			if e != nil {
				return BrokerResult{}, errBroker
			}
		}
		if len(envelope) > handoff.MaxBindingBytes || guardLive() != nil || ctx.Err() != nil {
			clear(envelope)
			return BrokerResult{}, errBroker
		}

		owned := &issuedControllerHandoff{receipt: bytes.Clone(plan.receiptRaw), envelope: envelope, payload: bytes.Clone(data), deadline: deadline}
		defer owned.clear()
		if guardLive() != nil || ctx.Err() != nil {
			return BrokerResult{}, errBroker
		}
		launchErr = assembly.launch(ctx, owned, filepath.Join(path, "controller-approval.json"))
	} else {
		if guardLive() != nil {
			return BrokerResult{}, errBroker
		}
		launchErr = plan.launch(ctx, data, filepath.Join(path, "controller-approval.json"))
	}
	if launchErr != nil || guardPostChild() != nil || j.append("controller_completed", nil) != nil || claim.complete() != nil || guardPostChild() != nil {
		return BrokerResult{}, errBroker
	}
	if a.Mode == "paired-terminal" {
		return BrokerResult{Status: "paired_terminal_completed"}, nil
	}
	return BrokerResult{Status: "controller_completed"}, nil
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
