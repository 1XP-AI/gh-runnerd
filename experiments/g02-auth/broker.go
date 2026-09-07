package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

// BrokerApproval describes one future, explicitly approved credential issuance
// and either hostname discovery or one already-reviewed controller phase.
type BrokerApproval struct {
	Mode                       string    `json:"mode"`
	AppID                      int64     `json:"app_id"`
	AppName                    string    `json:"app_name"`
	AppOwner                   string    `json:"app_owner"`
	AppOwnerID                 int64     `json:"app_owner_id"`
	InstallationID             int64     `json:"installation_id"`
	Organization               string    `json:"organization"`
	OrganizationID             int64     `json:"organization_id"`
	Repository                 string    `json:"repository"`
	RepositoryID               int64     `json:"repository_id"`
	RunnerGroupID              int64     `json:"runner_group_id"`
	RunnerGroupName            string    `json:"runner_group_name"`
	ExpiresAt                  time.Time `json:"expires_at"`
	Phase                      string    `json:"phase,omitempty"`
	AllowVerificationAuthority bool      `json:"allow_verification_authority"`
	ControllerBinarySHA256     string    `json:"controller_binary_sha256,omitempty"`
	ControllerApprovalSHA256   string    `json:"controller_approval_sha256,omitempty"`
	ControllerHarnessSHA       string    `json:"controller_harness_sha,omitempty"`
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
var brokerPhases = map[string]bool{"create": true, "before-ack": true, "after-ack": true, "before-acquire": true, "acquire-loss": true, "jit-loss": true, "inspect": true, "cleanup": true}

func (a BrokerApproval) validate(now time.Time) error {
	if a.AppID < 1 || a.AppOwnerID < 1 || a.InstallationID < 1 || a.OrganizationID < 1 || a.RepositoryID < 1 || a.RunnerGroupID < 1 || !appSlug.MatchString(a.AppName) || !organizationLogin.MatchString(a.AppOwner) || !organizationLogin.MatchString(a.Organization) || !brokerComponent.MatchString(a.Repository) || !brokerComponent.MatchString(a.RunnerGroupName) || !a.ExpiresAt.After(now.Add(time.Minute)) || a.ExpiresAt.After(now.Add(24*time.Hour)) {
		return errBroker
	}
	if a.Mode == "discover-actions-host" {
		if a.Phase != "" || a.AllowVerificationAuthority {
			return errBroker
		}
	} else if a.Mode != "controller" || !brokerPhases[a.Phase] {
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
func brokerExecute(parent context.Context, a BrokerApproval, input brokerInput, path string, api *brokerAPI, launch func(context.Context, []byte) error) (BrokerResult, error) {
	if parent == nil || api == nil || a.validate(api.now()) != nil || (input.VerificationToken != "" && (!a.AllowVerificationAuthority || !validBrokerToken(input.VerificationToken))) || (a.Mode == "controller" && launch == nil) {
		return BrokerResult{}, errBroker
	}
	ctx, cancel := context.WithDeadline(parent, minTime(a.ExpiresAt, api.now().Add(10*time.Minute)))
	defer cancel()
	candidate := Candidate{AppID: a.AppID, PEM: []byte(input.PEM)}
	defer clear(candidate.PEM)
	cred, err := parseCredential(candidate)
	if err != nil {
		return BrokerResult{}, errBroker
	}
	j, err := openBrokerJournal(path, a)
	if err != nil {
		return BrokerResult{}, errBroker
	}
	defer j.close()
	// JWT identity verification precedes the one token mint. Private repository
	// and runner-group APIs require that installation token, so scope preflight
	// necessarily follows minting and precedes every subsequent auth/child effect.
	identity, err := api.github.DescribeApp(ctx, cred)
	if err != nil || identity.ID != a.AppID || identity.Slug != a.AppName || identity.OwnerID != a.AppOwnerID || !strings.EqualFold(identity.OwnerLogin, a.AppOwner) || identity.OwnerType != "Organization" {
		return BrokerResult{}, errBroker
	}
	install, err := api.github.OrganizationInstallation(ctx, cred, a.Organization)
	if err != nil || install.ID != a.InstallationID || install.AppID != identity.ID || install.AccountID != a.OrganizationID || install.TargetID != a.OrganizationID || !strings.EqualFold(install.Login, a.Organization) || install.AccountType != "Organization" || install.TargetType != "Organization" || !install.SuspensionKnown || install.Suspended || !minimalPermissions(install.Permissions) {
		return BrokerResult{}, errBroker
	}
	if j.append("token_request_started", nil) != nil {
		return BrokerResult{}, errBroker
	}
	issued, err := api.mint(ctx, a, cred)
	if err != nil {
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
	if a.Mode == "discover-actions-host" {
		host, err := api.discover(ctx, a, issued.Token, j)
		if err != nil || j.append("actions_host_observed", map[string]any{"actions_host": host}) != nil {
			return BrokerResult{}, errBroker
		}
		return BrokerResult{Status: "actions_host_observed", ActionsHost: host}, nil
	}
	// These facts are from authenticated identity reads and the actual mint
	// response. The broker does not accept user-provided token issuance claims.
	payload := struct {
		InstallationToken string    `json:"installation_token"`
		VerificationToken string    `json:"verification_token"`
		AppID             int64     `json:"app_id"`
		InstallationID    int64     `json:"installation_id"`
		Organization      string    `json:"organization"`
		ExpiresAt         time.Time `json:"expires_at"`
		SelfHostedRunners string    `json:"organization_self_hosted_runners"`
		Metadata          string    `json:"metadata"`
	}{issued.Token, input.VerificationToken, identity.ID, install.ID, install.Login, issued.ExpiresAt, issued.Permissions["organization_self_hosted_runners"], issued.Permissions["metadata"]}
	data, _ := json.Marshal(payload)
	defer clear(data)
	if len(data) > 16384 || j.append("controller_handoff_started", nil) != nil {
		return BrokerResult{}, errBroker
	}
	if launch(ctx, data) != nil || j.append("controller_completed", nil) != nil {
		return BrokerResult{}, errBroker
	}
	return BrokerResult{Status: "controller_completed"}, nil
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
