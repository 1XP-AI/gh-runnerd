package livecanary

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
)

func credentials(a Approval) Credentials {
	return Credentials{InstallationToken: "synthetic-installation-token", VerificationToken: "synthetic-actions-read-token", AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, ExpiresAt: time.Now().Add(time.Hour), SelfHostedRunners: "write", Metadata: "read"}
}

func TestAuthoritySplitAndPolicyRejection(t *testing.T) {
	for _, fault := range []string{"", "public", "other-repo", "all-repos", "default-group", "public-group", "other-group-repo", "workflow-sha", "workflow-repo", "workflow-event"} {
		t.Run(fault, func(t *testing.T) {
			a := approval()
			c := credentials(a)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				repo := map[string]any{"id": a.RepositoryID, "full_name": a.Organization + "/" + a.Repository, "private": true, "fork": false}
				if fault == "public" {
					repo["private"] = false
				}
				if fault == "other-repo" {
					repo["id"] = 99
				}
				var body any
				wantToken := c.InstallationToken
				switch {
				case r.URL.Path == "/installation/repositories":
					body = map[string]any{"total_count": 1, "repositories": []any{repo}}
				case strings.Contains(r.URL.Path, "/actions/runs/"):
					wantToken = c.VerificationToken
					sha, event := a.WorkflowSHA, "workflow_dispatch"
					if fault == "workflow-sha" {
						sha = strings.Repeat("3", 40)
					}
					if fault == "workflow-event" {
						event = "pull_request"
					}
					if fault == "workflow-repo" {
						repo["id"] = 99
					}
					body = map[string]any{"id": a.WorkflowRunID, "head_sha": sha, "path": a.WorkflowPath, "event": event, "run_attempt": 1, "repository": repo, "head_repository": repo}
				case strings.HasSuffix(r.URL.Path, "/repositories"):
					if fault == "other-group-repo" {
						repo["id"] = 99
					}
					body = map[string]any{"total_count": 1, "repositories": []any{repo}}
				case strings.Contains(r.URL.Path, "/runner-groups/"):
					visibility := "selected"
					if fault == "all-repos" {
						visibility = "all"
					}
					body = map[string]any{"id": a.RunnerGroupID, "visibility": visibility, "default": fault == "default-group", "allows_public_repositories": fault == "public-group", "inherited": false}
				default:
					body = repo
				}
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer "+wantToken {
					t.Error("authority crossed endpoint boundary or preflight wrote")
				}
				_ = json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			api := &SDKAPI{rest: server.Client(), baseURL: server.URL, approval: a, credentials: c}
			err := api.Preflight(context.Background(), a)
			if strings.HasPrefix(fault, "workflow-") {
				if err != nil {
					t.Fatal(err)
				}
				err = api.VerifyRun(context.Background(), a, a.WorkflowRunID)
			}
			if (err == nil) != (fault == "") {
				t.Fatal("remote approval boundary failed")
			}
		})
	}
}

func TestCredentialAttestationMismatchAndExpiredTokenRejected(t *testing.T) {
	for _, fault := range []string{"app", "installation", "org", "permission", "expiry", "same-token"} {
		t.Run(fault, func(t *testing.T) {
			a := approval()
			c := credentials(a)
			switch fault {
			case "app":
				c.AppID++
			case "installation":
				c.InstallationID++
			case "org":
				c.Organization = "other"
			case "permission":
				c.SelfHostedRunners = "read"
			case "expiry":
				c.ExpiresAt = time.Now()
			case "same-token":
				c.VerificationToken = c.InstallationToken
			}
			if _, err := NewSDKAPI(a, c); err == nil {
				t.Fatal("wrong broker authority accepted")
			}
		})
	}
}

func TestValidDrainSessionWireRequiresExactQueueAuthorization(t *testing.T) {
	a := approval()
	token := strings.Repeat("q", 24)
	other := strings.Repeat("w", 24)
	zero, one := 0, 1
	wireStats := &baselineStatistics{Available: &zero, Acquired: &zero, Assigned: &zero, Running: &zero, Registered: &one, Busy: &zero, Idle: &one}
	sdkStats := &scaleset.RunnerScaleSetStatistic{TotalAvailableJobs: 0, TotalAcquiredJobs: 0, TotalAssignedJobs: 0, TotalRunningJobs: 0, TotalRegisteredRunners: 1, TotalBusyRunners: 0, TotalIdleRunners: 1}
	set := &scaleset.RunnerScaleSet{
		ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID,
		Labels:        []scaleset.Label{{Name: a.setName()}},
		RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true}, Statistics: sdkStats,
	}
	sessionID := uuid.MustParse("00000000-0000-4000-8000-000000000031")
	session := scaleset.RunnerScaleSetSession{
		SessionID: sessionID, OwnerName: a.setName(), MessageQueueURL: "https://fixture.actions.githubusercontent.com/queue",
		MessageQueueAccessToken: token, RunnerScaleSet: set, Statistics: sdkStats,
	}
	wire := &baselineSessionFacts{
		SessionID: sessionID.String(), Owner: a.setName(), Statistics: wireStats, NestedStatistics: wireStats,
		NestedSet: true, SetID: 7, SetName: a.setName(), GroupID: a.RunnerGroupID,
		queueURL: session.MessageQueueURL, authorization: token,
	}
	if !validDrainSessionWire(a, 7, a.setName(), wire, session) {
		t.Fatal("matching queue authorization rejected")
	}
	wire.authorization = other
	if validDrainSessionWire(a, 7, a.setName(), wire, session) {
		t.Fatal("different queue authorization accepted")
	}
	wire.authorization = ""
	if validDrainSessionWire(a, 7, a.setName(), wire, session) {
		t.Fatal("missing queue authorization accepted")
	}
}

func TestTransportRejectsPlaintextOffHostAndProxyBeforeNetwork(t *testing.T) {
	a := approval()
	api, err := NewSDKAPI(a, credentials(a))
	if err != nil {
		t.Fatal(err)
	}
	transport := api.rest.Transport.(*http.Transport)
	for _, target := range []string{"http://api.github.com:443/path", "https://evil.example/path", "https://api.github.com:444/path", "https://user:password@api.github.com/path"} {
		req, _ := http.NewRequest(http.MethodGet, target, nil)
		if _, err := transport.Proxy(req); err == nil {
			t.Fatal("unsafe authenticated destination accepted")
		}
	}
	for _, address := range []string{"evil.example:443", "127.0.0.1:443", "api.github.com:80"} {
		if conn, err := transport.DialContext(context.Background(), "tcp", address); err == nil {
			conn.Close()
			t.Fatal("unsafe network destination accepted")
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/path", nil)
	if proxy, err := transport.Proxy(req); err != nil || proxy != nil {
		t.Fatal("direct approved HTTPS refused or proxy used")
	}
	if api.rest.CheckRedirect(req, nil) != http.ErrUseLastResponse {
		t.Fatal("redirect may forward credentials")
	}
}

func TestHTTPErrorsDoNotReturnSecretResponseBody(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				_, _ = w.Write([]byte("synthetic-secret-body"))
			}))
			defer server.Close()
			a := approval()
			api := &SDKAPI{rest: server.Client(), baseURL: server.URL, approval: a, credentials: credentials(a)}
			err := api.Preflight(context.Background(), a)
			if err == nil || strings.Contains(err.Error(), "synthetic-secret") || calls != 1 {
				t.Fatal("unsafe error output or automatic HTTP retry")
			}
		})
	}
}
