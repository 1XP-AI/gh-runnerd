//go:build g01_pair_fixture

package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

type processBrokerHTTP struct {
	approval      *Approval
	t             *testing.T
	attempt       string
	calls         []string
	mints         int
	failPreflight bool
}

func (f *processBrokerHTTP) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	repo := map[string]any{"id": 42, "full_name": "fixture-org/canary", "private": true, "fork": false, "owner": map[string]any{"id": 101, "login": "fixture-org"}}
	var payload any
	status := 200
	switch r.URL.Path {
	case "/app":
		payload = map[string]any{"id": 11, "slug": "fixture-app", "owner": map[string]any{"id": 101, "login": "fixture-org", "type": "Organization"}}
	case "/orgs/fixture-org/installation":
		payload = map[string]any{"id": 12, "app_id": 11, "target_id": 101, "target_type": "Organization", "suspended_at": nil, "account": map[string]any{"id": 101, "login": "fixture-org", "type": "Organization"}, "permissions": map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"}}
	case workflowRunFixturePath(f):
		stored, e := os.ReadFile(filepath.Join(f.attempt, "broker.jsonl"))
		if e != nil || len(stored) == 0 {
			f.t.Error("observation preceded durable attempt")
		}
		runID, sha := int64(7), strings.Repeat("b", 40)
		if f.approval != nil {
			runID = f.approval.WorkflowRunID
			sha = f.approval.WorkflowSHA
		}
		payload = map[string]any{"id": runID, "head_sha": sha, "head_branch": "main", "event": "workflow_dispatch", "path": ".github/workflows/canary.yml", "run_attempt": 1, "repository": repo, "head_repository": repo}
	case "/app/installations/12/access_tokens":
		f.mints++
		stored, _ := os.ReadFile(filepath.Join(f.attempt, "broker.jsonl"))
		if !bytes.Contains(stored, []byte("token_request_started")) {
			f.t.Error("mint preceded durable intent")
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"repository_ids":[42]`)) {
			f.t.Error("mint not scoped")
		}
		status = 201
		payload = map[string]any{"token": "synthetic-process-installation-token", "expires_at": time.Now().Add(time.Hour), "permissions": map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"}, "repository_selection": "selected", "repositories": []any{repo}}
	case "/installation/repositories":
		payload = map[string]any{"total_count": 1, "repositories": []any{repo}}
	case "/repos/fixture-org/canary":
		payload = repo
	case "/orgs/fixture-org/actions/runner-groups/3":
		payload = map[string]any{"id": 3, "name": "fixture-group", "visibility": "selected", "default": false, "inherited": false, "allows_public_repositories": f.failPreflight}
	case "/orgs/fixture-org/actions/runner-groups/3/repositories":
		payload = map[string]any{"total_count": 1, "repositories": []any{repo}}
	default:
		return nil, fmt.Errorf("unexpected synthetic route")
	}
	raw, _ := json.Marshal(payload)
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw))}, nil
}
func TestControllerProcessHandoffActualBrokerSeparateProcess(t *testing.T) {
	for _, preflightFail := range []bool{false, true} {
		t.Run(fmt.Sprint(preflightFail), func(t *testing.T) {
			parent, e := filepath.EvalSymlinks(privateDir(t))
			if e != nil {
				t.Fatal(e)
			}
			for _, name := range []string{"admission", "controller-state"} {
				if os.Mkdir(filepath.Join(parent, name), 0700) != nil {
					t.Fatal("private fixture directory")
				}
			}
			a := controllerHandoffApprovalAt(time.Now())
			a.Phases = []string{"create", "inspect", "cleanup"}
			a.WorkflowSHA = strings.Repeat("b", 40)
			a.ExpiresAt = time.Now().Add(time.Hour)
			raw := controllerHandoffSnapshot(t, a)
			executable, e := os.Executable()
			if e != nil {
				t.Fatal(e)
			}
			binaryData, e := os.ReadFile(executable)
			if e != nil {
				t.Fatal(e)
			}
			binaryPath := filepath.Join(parent, "fixture-test")
			if os.WriteFile(binaryPath, binaryData, 0500) != nil {
				t.Fatal("copy fixture executable")
			}
			approvalPath := filepath.Join(parent, "controller.json")
			if os.WriteFile(approvalPath, raw, 0600) != nil {
				t.Fatal("approval")
			}
			ba := enrollment.BrokerApproval{OwnerNonce: a.OwnerNonce, Mode: "controller", Phase: "create", AppID: a.AppID, AppName: "fixture-app", AppOwner: a.Organization, AppOwnerID: 101, InstallationID: a.InstallationID, Organization: a.Organization, OrganizationID: 101, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: int64(a.RunnerGroupID), RunnerGroupName: "fixture-group", ExpiresAt: a.ExpiresAt, ControllerBinarySHA256: handoff.Digest(binaryData), ControllerApprovalSHA256: handoff.Digest(raw), ControllerHarnessSHA: a.HarnessSHA}
			baRaw, _ := json.Marshal(ba)
			baPath := filepath.Join(parent, "broker.json")
			if os.WriteFile(baPath, baRaw, 0600) != nil {
				t.Fatal("broker approval")
			}
			files := enrollment.BrokerFiles{ApprovalPath: baPath, StateDirectory: filepath.Join(parent, "attempt"), ControllerBinary: binaryPath, ControllerApproval: approvalPath, ControllerStateDirectory: filepath.Join(parent, "controller-state")}
			_, request, e := controllerHandoffRequest(raw, "create", "github-rest-workflow-v1", time.Now())
			if e != nil {
				t.Fatal(e)
			}
			grant := enrollment.WorkflowGrantForFixture{Request: request, OrganizationID: 101, RepositoryID: a.RepositoryID, ExpiresAt: a.ExpiresAt}
			transport := &processBrokerHTTP{t: t, attempt: files.StateDirectory, failPreflight: preflightFail}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			result, effects, e := enrollment.RunProcessHandoffForFixture(ctx, files, transport, grant)
			wantSignatures := 1
			if preflightFail {
				wantSignatures = 0
			}
			if (e != nil) != preflightFail || effects.BindingSignatures != wantSignatures || transport.mints != 1 || (!preflightFail && result.Status != "controller_completed") {
				t.Fatalf("broker/process chain: result=%+v err=%v signs=%d mints=%d calls=%v", result, e, effects.BindingSignatures, transport.mints, transport.calls)
			}
			want := []string{"GET /repos/fixture-org/canary/actions/runs/7", "GET /app", "GET /orgs/fixture-org/installation", "POST /app/installations/12/access_tokens", "GET /installation/repositories", "GET /repos/fixture-org/canary", "GET /orgs/fixture-org/actions/runner-groups/3"}
			if !preflightFail {
				want = append(want, "GET /orgs/fixture-org/actions/runner-groups/3/repositories")
			}
			if !reflect.DeepEqual(transport.calls, want) {
				t.Fatalf("operation order changed: %v", transport.calls)
			}
			stored, _ := os.ReadFile(filepath.Join(files.ControllerStateDirectory, "journal.jsonl"))
			if bytes.Contains(stored, []byte("controller-handoff-consumed")) == preflightFail {
				t.Fatal("unexpected child consumption")
			}
			// A second actual entry must not mint or sign again, even after failure.
			mints := transport.mints
			_, again, e := enrollment.RunProcessHandoffForFixture(ctx, files, transport, grant)
			if e == nil || transport.mints != mints || again.BindingSignatures != 0 {
				t.Fatal("permanent broker attempt was retried")
			}
			brokerStored, _ := os.ReadFile(filepath.Join(files.StateDirectory, "broker.jsonl"))
			for _, secret := range []string{"synthetic-process-installation-token", "payload_sha256", "payload_length", "g01-issued-credential-binding", "BEGIN RSA"} {
				if bytes.Contains(brokerStored, []byte(secret)) || bytes.Contains(stored, []byte(secret)) {
					t.Fatal("credential/envelope data persisted")
				}
			}
		})
	}
}

func workflowRunFixturePath(f *processBrokerHTTP) string {
	if f.approval != nil {
		return fmt.Sprintf("/repos/fixture-org/canary/actions/runs/%d", f.approval.WorkflowRunID)
	}
	return "/repos/fixture-org/canary/actions/runs/7"
}
