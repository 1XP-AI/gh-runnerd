//go:build g01_pair_fixture

package livecanary

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

type processBrokerHTTP struct {
	approval                       *Approval
	t                              *testing.T
	attempt                        string
	calls                          []string
	expectedWorkflowAuthorizations []string
	workflowAuthorizationsChecked  int
	mints                          int
	readOnlyMints                  int
	managementMints                int
	revocations                    int
	readOnlyRevocations            int
	managementRevocations          int
	failPreflight                  bool
	groupFault                     bool
}

func TestProcessBrokerHTTPRejectsUnrecognizedAuthorization(t *testing.T) {
	attempt := t.TempDir()
	if err := os.WriteFile(filepath.Join(attempt, "broker.jsonl"), []byte("attempt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	approval := &Approval{AppID: 11, WorkflowRunID: 7, Organization: "fixture-org", Repository: "canary"}
	now := time.Now()
	wrongClaims, _ := json.Marshal(struct {
		Issuer    string `json:"iss"`
		IssuedAt  int64  `json:"iat"`
		ExpiresAt int64  `json:"exp"`
	}{"12", now.Add(-time.Minute).Unix(), now.Add(9 * time.Minute).Unix()})
	wrongAppJWT := "Bearer " + base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(wrongClaims) + ".c2ln"
	cases := []struct {
		name          string
		path          string
		method        string
		authorization string
	}{
		{name: "app JWT identity", path: "/app", method: http.MethodGet, authorization: wrongAppJWT},
		{name: "mint JWT identity", path: "/app/installations/12/access_tokens", method: http.MethodPost, authorization: wrongAppJWT},
		{name: "workflow observation", path: workflowRunFixturePath(&processBrokerHTTP{approval: approval}), method: http.MethodGet, authorization: "Bearer synthetic-process-verification-token"},
		{name: "read-only preflight", path: "/installation/repositories", method: http.MethodGet, authorization: "Bearer synthetic-process-installation-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &processBrokerHTTP{approval: approval, t: t, attempt: attempt, readOnlyMints: 1, expectedWorkflowAuthorizations: []string{"Bearer synthetic-observation-token"}}
			request, err := http.NewRequest(tc.method, "https://api.github.com"+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", tc.authorization)
			response, err := transport.RoundTrip(request)
			if response != nil {
				response.Body.Close()
			}
			if err == nil {
				t.Fatal("unrecognized request authority was accepted")
			}
			if len(transport.calls) != 0 || transport.mints != 0 || transport.revocations != 0 {
				t.Fatalf("rejected authority crossed fixture effect boundary: calls=%v mints=%d revocations=%d", transport.calls, transport.mints, transport.revocations)
			}
		})
	}
}

func (f *processBrokerHTTP) validateAuthorization(r *http.Request) error {
	if f.approval == nil {
		return nil
	}
	authorization := r.Header.Get("Authorization")
	switch {
	case r.URL.Path == workflowRunFixturePath(f):
		if r.Method != http.MethodGet || f.workflowAuthorizationsChecked >= len(f.expectedWorkflowAuthorizations) || authorization != f.expectedWorkflowAuthorizations[f.workflowAuthorizationsChecked] {
			return fmt.Errorf("unexpected synthetic workflow authority")
		}
		f.workflowAuthorizationsChecked++
		return nil
	case r.URL.Path == "/app" || r.URL.Path == "/orgs/fixture-org/installation" || r.URL.Path == "/app/installations/12/access_tokens":
		if !processAppJWTMatches(authorization, f.approval.AppID, time.Now()) {
			return fmt.Errorf("unexpected synthetic App authority")
		}
		return nil
	case r.URL.Path == "/installation/token":
		if r.Method != http.MethodDelete {
			return fmt.Errorf("unexpected synthetic revocation method")
		}
		switch authorization {
		case "Bearer synthetic-process-read-installation-token":
			if f.readOnlyRevocations >= f.readOnlyMints || f.managementRevocations != f.managementMints {
				return fmt.Errorf("unexpected synthetic read-token revocation order")
			}
		case "Bearer synthetic-process-installation-token":
			if f.managementRevocations >= f.managementMints {
				return fmt.Errorf("unexpected synthetic management-token revocation order")
			}
		default:
			return fmt.Errorf("unexpected synthetic revocation authority")
		}
		return nil
	case isProcessBrokerPreflightPath(r.URL.Path):
		expected := ""
		if f.managementMints > f.managementRevocations {
			expected = "Bearer synthetic-process-installation-token"
		} else if f.readOnlyMints > f.readOnlyRevocations {
			expected = "Bearer synthetic-process-read-installation-token"
		}
		if r.Method != http.MethodGet || expected == "" || authorization != expected {
			return fmt.Errorf("unexpected synthetic preflight authority")
		}
		return nil
	default:
		return nil
	}
}

func isProcessBrokerPreflightPath(path string) bool {
	switch path {
	case "/installation/repositories", "/repos/fixture-org/canary", "/orgs/fixture-org/actions/runner-groups/3", "/orgs/fixture-org/actions/runner-groups/3/repositories":
		return true
	default:
		return false
	}
}

func expectedProcessBrokerV3Trace(approval Approval, readGroupReject bool) []string {
	workflowGet := "GET " + workflowRunFixturePath(&processBrokerHTTP{approval: &approval})
	trace := []string{workflowGet, "GET /app", "GET /orgs/fixture-org/installation", "POST /app/installations/12/access_tokens"}
	appendPreflight := func(rejectGroup bool) {
		if approval.ManagementScope != "organization-only" {
			trace = append(trace, "GET /installation/repositories", "GET /repos/fixture-org/canary")
		}
		trace = append(trace, "GET /orgs/fixture-org/actions/runner-groups/3")
		if !rejectGroup {
			trace = append(trace, "GET /orgs/fixture-org/actions/runner-groups/3/repositories")
		}
	}
	appendPreflight(readGroupReject)
	trace = append(trace, "DELETE /installation/token")
	if readGroupReject {
		return trace
	}
	trace = append(trace, workflowGet, "POST /app/installations/12/access_tokens")
	appendPreflight(false)
	trace = append(trace, workflowGet, "DELETE /installation/token")
	return trace
}

func assertProcessBrokerTrace(t *testing.T, transport *processBrokerHTTP, want []string) {
	t.Helper()
	if !reflect.DeepEqual(transport.calls, want) {
		t.Fatalf("broker request order differs: got=%v want=%v", transport.calls, want)
	}
	workflowGets := 0
	for _, call := range want {
		if call == "GET "+workflowRunFixturePath(transport) {
			workflowGets++
		}
	}
	if transport.workflowAuthorizationsChecked != workflowGets {
		t.Fatalf("workflow authority count=%d, want one checked identity per GET (%d)", transport.workflowAuthorizationsChecked, workflowGets)
	}
}

type processTokenLifecycleRecord struct {
	Phase   string          `json:"phase"`
	Kind    string          `json:"kind"`
	Slot    string          `json:"slot"`
	Attempt json.RawMessage `json:"attempt"`
	Journal json.RawMessage `json:"journal"`
}

func assertProcessBrokerTokenLifecycle(t *testing.T, attemptDirectory, admissionDirectory string, wantRead, wantManagement bool) {
	t.Helper()
	readLifecycleRecords := func(path string, include map[string]bool) []processTokenLifecycleRecord {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("durable broker lifecycle unavailable: %v", err)
		}
		var records []processTokenLifecycleRecord
		for _, line := range bytes.Split(data, []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var record processTokenLifecycleRecord
			if err := json.Unmarshal(line, &record); err != nil {
				t.Fatalf("durable lifecycle record malformed: %v", err)
			}
			name := record.Kind
			if record.Phase != "" {
				name = record.Phase
			}
			if include[name] {
				record.Kind = name
				records = append(records, record)
			}
		}
		return records
	}
	brokerKinds := map[string]bool{
		"read_only_token_revocation_started":    true,
		"read_only_token_revoked":               true,
		"read_only_token_revocation_uncertain":  true,
		"read_only_token_revocation_skipped":    true,
		"management_token_revocation_started":   true,
		"management_token_revoked":              true,
		"management_token_revocation_uncertain": true,
		"management_token_revocation_skipped":   true,
	}
	wantBroker := make([]string, 0, 4)
	if wantRead {
		wantBroker = append(wantBroker, "read_only_token_revocation_started", "read_only_token_revoked")
	}
	if wantManagement {
		wantBroker = append(wantBroker, "management_token_revocation_started", "management_token_revoked")
	}
	gotBroker := readLifecycleRecords(filepath.Join(attemptDirectory, "broker.jsonl"), brokerKinds)
	var gotBrokerKinds []string
	for _, record := range gotBroker {
		gotBrokerKinds = append(gotBrokerKinds, record.Kind)
	}
	if !reflect.DeepEqual(gotBrokerKinds, wantBroker) {
		t.Fatalf("durable broker revocation order differs: got=%v want=%v", gotBrokerKinds, wantBroker)
	}

	admissionKinds := map[string]bool{
		"read_token_lifecycle_started":            true,
		"read_token_lifecycle_confirmed":          true,
		"read_token_revocation_started":           true,
		"read_token_revocation_confirmed":         true,
		"read_token_request_not_dispatched":       true,
		"management_token_lifecycle_started":      true,
		"management_token_lifecycle_confirmed":    true,
		"management_token_request_not_dispatched": true,
	}
	wantAdmission := make([]string, 0, 5)
	if wantRead {
		wantAdmission = append(wantAdmission, "read_token_lifecycle_started", "read_token_lifecycle_confirmed")
	}
	if wantManagement {
		wantAdmission = append(wantAdmission, "management_token_lifecycle_started", "management_token_lifecycle_confirmed")
	} else {
		wantAdmission = append(wantAdmission, "management_token_request_not_dispatched")
	}
	gotAdmission := readLifecycleRecords(filepath.Join(admissionDirectory, "broker-admission.jsonl"), admissionKinds)
	var gotAdmissionKinds []string
	for _, record := range gotAdmission {
		gotAdmissionKinds = append(gotAdmissionKinds, record.Kind)
	}
	if !reflect.DeepEqual(gotAdmissionKinds, wantAdmission) {
		t.Fatalf("durable admission lifecycle differs: got=%v want=%v", gotAdmissionKinds, wantAdmission)
	}
	for index := 0; index+1 < len(gotAdmission); index += 2 {
		started, confirmed := gotAdmission[index], gotAdmission[index+1]
		if !strings.HasSuffix(started.Kind, "_started") || !strings.HasSuffix(confirmed.Kind, "_confirmed") || started.Slot == "" || started.Slot != confirmed.Slot || !bytes.Equal(started.Attempt, confirmed.Attempt) || !bytes.Equal(started.Journal, confirmed.Journal) {
			t.Fatalf("durable token lifecycle identity differs: started=%+v confirmed=%+v", started, confirmed)
		}
	}
	if wantRead && wantManagement && (gotAdmission[0].Slot != gotAdmission[2].Slot || !bytes.Equal(gotAdmission[0].Attempt, gotAdmission[2].Attempt) || !bytes.Equal(gotAdmission[0].Journal, gotAdmission[2].Journal)) {
		t.Fatal("read and management token lifecycle records do not share one exact broker claim")
	}
	if wantRead && !wantManagement {
		last := gotAdmission[len(gotAdmission)-1]
		if last.Kind != "management_token_request_not_dispatched" || last.Slot != gotAdmission[0].Slot || !bytes.Equal(last.Attempt, gotAdmission[0].Attempt) || !bytes.Equal(last.Journal, gotAdmission[0].Journal) {
			t.Fatal("unissued management token record is not bound to the read-token claim")
		}
	}
}

// The adapter's focused unit test verifies the RS256 signature against its
// synthetic key. Process fixtures additionally bind provider requests to the
// intended App identity and the broker's bounded ten-minute JWT lifetime.
func processAppJWTMatches(authorization string, appID int64, now time.Time) bool {
	if appID < 1 || !strings.HasPrefix(authorization, "Bearer ") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(authorization, "Bearer "), ".")
	if len(parts) != 3 {
		return false
	}
	headerBytes, headerErr := base64.RawURLEncoding.DecodeString(parts[0])
	claimsBytes, claimsErr := base64.RawURLEncoding.DecodeString(parts[1])
	signature, signatureErr := base64.RawURLEncoding.DecodeString(parts[2])
	if headerErr != nil || claimsErr != nil || signatureErr != nil || len(signature) == 0 {
		return false
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	var claims struct {
		Issuer    string `json:"iss"`
		IssuedAt  int64  `json:"iat"`
		ExpiresAt int64  `json:"exp"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || json.Unmarshal(claimsBytes, &claims) != nil {
		return false
	}
	nowUnix := now.Unix()
	return header.Algorithm == "RS256" && header.Type == "JWT" && claims.Issuer == strconv.FormatInt(appID, 10) &&
		claims.IssuedAt <= nowUnix && claims.IssuedAt >= now.Add(-2*time.Minute).Unix() &&
		claims.ExpiresAt > nowUnix && claims.ExpiresAt-nowUnix <= 10*60 && claims.ExpiresAt-claims.IssuedAt == 10*60
}

func (f *processBrokerHTTP) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := f.validateAuthorization(r); err != nil {
		return nil, err
	}
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
		run := map[string]any{"id": runID, "head_sha": sha, "head_branch": "main", "event": "workflow_dispatch", "status": "in_progress", "path": ".github/workflows/canary.yml", "run_attempt": 1, "repository": repo, "head_repository": repo}
		if f.approval != nil && f.approval.GroupPolicy != (handoff.GroupPolicy{}) {
			workflow := f.approval.GroupPolicy.Workflow
			at := strings.LastIndexByte(workflow, '@')
			if at <= 0 || at == len(workflow)-1 {
				f.t.Error("fixture reusable workflow pin is malformed")
			} else {
				run["referenced_workflows"] = []any{map[string]any{"path": workflow, "sha": workflow[at+1:]}}
			}
		}
		payload = run
	case "/app/installations/12/access_tokens":
		f.mints++
		stored, _ := os.ReadFile(filepath.Join(f.attempt, "broker.jsonl"))
		if !bytes.Contains(stored, []byte("token_request_started")) {
			f.t.Error("mint preceded durable intent")
		}
		body, _ := io.ReadAll(r.Body)
		var request struct {
			RepositoryIDs []int64           `json:"repository_ids"`
			Permissions   map[string]string `json:"permissions"`
		}
		if json.Unmarshal(body, &request) != nil || (request.Permissions["organization_self_hosted_runners"] != "read" && request.Permissions["organization_self_hosted_runners"] != "write") {
			f.t.Error("mint permission set was invalid")
		}
		if request.Permissions["organization_self_hosted_runners"] == "read" {
			f.readOnlyMints++
		} else {
			f.managementMints++
		}
		organizationOnly := f.approval != nil && f.approval.ManagementScope == "organization-only"
		if !organizationOnly && !reflect.DeepEqual(request.RepositoryIDs, []int64{42}) {
			f.t.Error("mint not scoped")
		}
		status = 201
		token := "synthetic-process-installation-token"
		if request.Permissions["organization_self_hosted_runners"] == "read" {
			token = "synthetic-process-read-installation-token"
		}
		payload = map[string]any{"token": token, "expires_at": time.Now().Add(time.Hour), "permissions": request.Permissions, "repository_selection": "selected", "repositories": []any{repo}}
		if organizationOnly {
			var organizationRequest map[string]any
			_ = json.Unmarshal(body, &organizationRequest)
			if len(organizationRequest) != 1 {
				f.t.Error("organization-only mint widened")
			}
			payload = map[string]any{"token": token, "expires_at": time.Now().Add(time.Hour), "permissions": request.Permissions}
		}
	case "/installation/token":
		f.revocations++
		if r.Method != http.MethodDelete {
			f.t.Error("installation-token revocation used the wrong method")
		}
		stored, e := os.ReadFile(filepath.Join(f.attempt, "broker.jsonl"))
		switch r.Header.Get("Authorization") {
		case "Bearer synthetic-process-read-installation-token":
			f.readOnlyRevocations++
			if e != nil || !bytes.Contains(stored, []byte("read_only_token_revocation_started")) {
				f.t.Error("read-token revocation preceded durable intent")
			}
		case "Bearer synthetic-process-installation-token":
			f.managementRevocations++
			if e != nil || !bytes.Contains(stored, []byte("management_token_revocation_started")) {
				f.t.Error("management-token revocation preceded durable intent")
			}
		default:
			f.t.Error("installation-token revocation used an unexpected authority")
		}
		status = http.StatusNoContent
	case "/installation/repositories":
		payload = map[string]any{"total_count": 1, "repositories": []any{repo}}
	case "/repos/fixture-org/canary":
		payload = repo
	case "/orgs/fixture-org/actions/runner-groups/3":
		payload = map[string]any{"id": 3, "name": "fixture-group", "visibility": "selected", "default": false, "inherited": false, "allows_public_repositories": f.failPreflight}
		if f.approval != nil && f.approval.GroupPolicy.Version == 2 {
			expected := f.approval.GroupPolicy.Workflow
			if f.groupFault {
				expected = strings.Replace(expected, "dddd", "eeee", 1)
			}
			payload.(map[string]any)["restricted_to_workflows"] = true
			payload.(map[string]any)["selected_workflows"] = []string{expected}
		}
	case "/orgs/fixture-org/actions/runner-groups/3/repositories":
		payload = map[string]any{"total_count": 1, "repositories": []any{repo}}
	default:
		return nil, fmt.Errorf("unexpected synthetic route")
	}
	var raw []byte
	if status != http.StatusNoContent {
		raw, _ = json.Marshal(payload)
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw))}, nil
}

func TestProcessBrokerWorkflowRunFixtureProvidesInProgressStatus(t *testing.T) {
	attempt := t.TempDir()
	if err := os.WriteFile(filepath.Join(attempt, "broker.jsonl"), []byte("attempt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	transport := &processBrokerHTTP{t: t, attempt: attempt}
	request, err := http.NewRequest(http.MethodGet, "https://api.github.com"+workflowRunFixturePath(transport), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var workflow struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&workflow); err != nil {
		t.Fatal(err)
	}
	if workflow.Status != "in_progress" {
		t.Fatalf("workflow fixture status = %q, want in_progress", workflow.Status)
	}
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
			transport := &processBrokerHTTP{t: t, attempt: files.StateDirectory, approval: &a, expectedWorkflowAuthorizations: []string{"Bearer synthetic-observation-token"}, failPreflight: preflightFail}
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
			want = append(want, "DELETE /installation/token")
			if !reflect.DeepEqual(transport.calls, want) {
				t.Fatalf("operation order changed: %v", transport.calls)
			}
			assertProcessBrokerTokenLifecycle(t, files.StateDirectory, filepath.Join(parent, "admission"), false, true)
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
