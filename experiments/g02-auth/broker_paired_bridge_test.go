package enrollment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const pairedBridgeJIT = "c3ludGhldGljLXByaXZhdGUtaml0LWNvbmZpZw=="

// pairedBrokerBridge is an offline-only private transport. It serves the
// controller's real REST/Actions calls over a generated TLS root and the
// worker's real Docker calls over a generated Unix socket. It retains only
// bounded counters and protocol state; request bodies and credentials are
// never recorded.
type pairedBrokerBridge struct {
	mu                  sync.Mutex
	server              *httptest.Server
	dockerServer        *http.Server
	dockerListener      net.Listener
	dockerEndpoint      string
	installationToken   string
	verificationToken   string
	adminToken          string
	queueToken          string
	organization        string
	repository          string
	workflowSHA         string
	workflowPath        string
	workflowRunID       int64
	runnerGroupID       int64
	setName             string
	workerName          string
	workerImage         string
	workerImageID       string
	workerDaemonID      string
	setExists           bool
	setCreated          bool
	setDeleted          bool
	container           map[string]any
	containerStarted    bool
	containerDeleted    bool
	polls               int
	lastAck             int
	unexpected          int
	createCalls         int
	startCalls          int
	workerDeleteCalls   int
	workerAbsenceCalls  int
	setCreateCalls      int
	setDeleteCalls      int
	setAbsenceCalls     int
	sessionOpenCalls    int
	sessionCloseCalls   int
	jitCalls            int
	acquireCalls        int
	ackCalls            int
	rosterCalls         int
	controllerInventory int
	registrationCalls   int
	exchangeCalls       int
	jobListCalls        int
	jobDetailCalls      int
	sdkRunnerCalls      int
	restRunnerCalls     int
}

func newPairedBrokerBridge(t *testing.T) *pairedBrokerBridge {
	t.Helper()
	// Unix socket paths are capped at 108 bytes on the supported hosts. Use a
	// short private /tmp directory so the fixture remains robust under the
	// workspace's long per-test temporary path.
	root, err := os.MkdirTemp("/tmp", "g01p-")
	if err != nil {
		t.Fatal("private bridge root")
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal("private bridge root")
	}
	f := &pairedBrokerBridge{
		installationToken: "synthetic-private-installation-token",
		verificationToken: "synthetic-private-workflow-token",
		queueToken:        "synthetic-private-queue-token",
		organization:      "org-a",
		repository:        "canary",
		workflowSHA:       strings.Repeat("b", 40),
		workflowPath:      ".github/workflows/canary.yml",
		workflowRunID:     7,
		runnerGroupID:     3,
		setName:           "g01-" + strings.Repeat("a", 32),
		workerName:        "g01-" + strings.Repeat("a", 32) + "-worker-1",
		workerImage:       pairedWorkerImage,
		workerImageID:     "sha256:" + strings.Repeat("d", 64),
		workerDaemonID:    "fixture-daemon",
	}
	listener, err := net.Listen("unix", filepath.Join(root, "docker.sock"))
	if err != nil {
		t.Fatal("private Docker socket")
	}
	if err := os.Chmod(listener.Addr().String(), 0600); err != nil {
		_ = listener.Close()
		t.Fatal("private Docker socket mode")
	}
	f.dockerListener = listener
	f.dockerEndpoint = listener.Addr().String()
	f.dockerServer = &http.Server{Handler: http.HandlerFunc(f.handleDocker)}
	go func() { _ = f.dockerServer.Serve(listener) }()

	f.server = httptest.NewTLSServer(http.HandlerFunc(f.handleGitHub))
	t.Cleanup(func() {
		f.server.Close()
		_ = f.dockerServer.Close()
		_ = f.dockerListener.Close()
	})
	return f
}

func (f *pairedBrokerBridge) markUnexpected() {
	f.unexpected++
}

func writeBridgeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	if status != http.StatusNoContent && value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func bridgeRepository() map[string]any {
	return map[string]any{
		"id": 501, "full_name": "org-a/canary", "private": true, "fork": false,
		"owner": map[string]any{"id": 101, "login": "org-a", "type": "Organization"},
	}
}

func (f *pairedBrokerBridge) actionPath(path string) string {
	if strings.HasPrefix(path, "/tenant/") {
		return strings.TrimPrefix(path, "/tenant")
	}
	return path
}

func (f *pairedBrokerBridge) auth(r *http.Request, want string) bool {
	return r.Header.Get("Authorization") == want
}

func (f *pairedBrokerBridge) scaleSet() map[string]any {
	return map[string]any{
		"id": 7, "name": f.setName, "runnerGroupId": f.runnerGroupID,
		"labels":        []map[string]any{{"name": f.setName, "type": "System"}},
		"RunnerSetting": map[string]any{"disableUpdate": true},
		"statistics":    map[string]int{"totalAvailableJobs": 0, "totalAcquiredJobs": 0, "totalAssignedJobs": 0, "totalRunningJobs": 0, "totalRegisteredRunners": 0, "totalBusyRunners": 0, "totalIdleRunners": 0},
	}
}

func (f *pairedBrokerBridge) job() map[string]any {
	status := "in_progress"
	var conclusion any
	if f.polls >= 2 {
		status = "completed"
		conclusion = "success"
	}
	return map[string]any{
		"id": 701, "run_id": f.workflowRunID, "run_attempt": 1,
		"head_sha": f.workflowSHA, "status": status, "conclusion": conclusion,
		"runner_id": 9001, "runner_name": f.workerName, "runner_group_id": f.runnerGroupID,
	}
}

func (f *pairedBrokerBridge) queueItems() []map[string]any {
	base := map[string]any{
		"runnerRequestId": int64(42), "jobId": "fixture-job", "ownerName": f.organization,
		"repositoryName": f.repository, "workflowRunId": f.workflowRunID,
		"eventName": "workflow_dispatch", "jobWorkflowRef": "fixture-workflow-ref",
		"acquireJobUrl": "https://invalid.example/acquire", "jobDisplayName": "fixture-job",
	}
	if f.polls < 2 {
		item := cloneBridgeMap(base)
		item["messageType"] = "JobAvailable"
		return []map[string]any{item}
	}
	assigned := cloneBridgeMap(base)
	assigned["messageType"] = "JobAssigned"
	completed := cloneBridgeMap(base)
	completed["messageType"] = "JobCompleted"
	completed["runnerId"], completed["runnerName"], completed["result"] = 81, f.workerName, "succeeded"
	started := cloneBridgeMap(base)
	started["messageType"] = "JobStarted"
	started["runnerId"], started["runnerName"] = 81, f.workerName
	return []map[string]any{assigned, completed, started}
}

func cloneBridgeMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+2)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (f *pairedBrokerBridge) handleGitHub(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.actionPath(r.URL.Path)

	// These are the only REST calls that carry the temporary installation or
	// verification authorities. The bridge compares them but never records them.
	if strings.HasSuffix(path, "/installation/repositories") && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.installationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		writeBridgeJSON(w, http.StatusOK, map[string]any{"total_count": 1, "repositories": []any{bridgeRepository()}})
		return
	}
	if path == "/repos/"+f.organization+"/"+f.repository && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.installationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		writeBridgeJSON(w, http.StatusOK, bridgeRepository())
		return
	}
	groupPath := "/orgs/" + f.organization + "/actions/runner-groups/3"
	if path == groupPath && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.installationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		writeBridgeJSON(w, http.StatusOK, map[string]any{"id": 3, "name": "fixture-group", "visibility": "selected", "default": false, "inherited": false, "allows_public_repositories": false})
		return
	}
	if path == groupPath+"/repositories" && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.installationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		writeBridgeJSON(w, http.StatusOK, map[string]any{"total_count": 1, "repositories": []any{bridgeRepository()}})
		return
	}
	if strings.HasSuffix(path, "/actions/runners/registration-token") && r.Method == http.MethodPost {
		if !f.auth(r, "Bearer "+f.installationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		f.registrationCalls++
		writeBridgeJSON(w, http.StatusCreated, map[string]any{"token": "synthetic-private-registration-token", "expires_at": time.Now().Add(time.Hour)})
		return
	}
	if strings.HasSuffix(path, "/actions/runner-registration") && r.Method == http.MethodPost {
		if !f.auth(r, "RemoteAuth synthetic-private-registration-token") {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		f.exchangeCalls++
		claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
		f.adminToken = "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."
		writeBridgeJSON(w, http.StatusOK, map[string]string{"url": f.server.URL + "/tenant/", "token": f.adminToken})
		return
	}

	if strings.HasSuffix(path, "/actions/runners") && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.installationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		if f.setCreated {
			f.rosterCalls++
		} else {
			f.controllerInventory++
		}
		writeBridgeJSON(w, http.StatusOK, map[string]any{"total_count": 0, "runners": []any{}})
		return
	}
	if path == "/orgs/"+f.organization+"/actions/runners/9001" && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.installationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		f.restRunnerCalls++
		if f.polls >= 2 {
			writeBridgeJSON(w, http.StatusNotFound, map[string]string{"message": "runner absent"})
			return
		}
		writeBridgeJSON(w, http.StatusOK, map[string]any{"id": 9001, "name": f.workerName, "status": "online", "busy": true})
		return
	}
	workflowPath := "/repos/" + f.organization + "/" + f.repository + "/actions/runs/7"
	if path == workflowPath && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.verificationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		repo := bridgeRepository()
		writeBridgeJSON(w, http.StatusOK, map[string]any{"id": f.workflowRunID, "head_sha": f.workflowSHA, "path": f.workflowPath, "event": "workflow_dispatch", "run_attempt": 1, "repository": repo, "head_repository": repo})
		return
	}
	jobsPath := "/repos/" + f.organization + "/" + f.repository + "/actions/runs/7/attempts/1/jobs"
	if path == jobsPath && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.verificationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		f.jobListCalls++
		writeBridgeJSON(w, http.StatusOK, map[string]any{"total_count": 1, "jobs": []any{f.job()}})
		return
	}
	if path == "/repos/"+f.organization+"/"+f.repository+"/actions/jobs/701" && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.verificationToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		f.jobDetailCalls++
		writeBridgeJSON(w, http.StatusOK, f.job())
		return
	}

	// All remaining routes are Actions service calls. The SDK pins the admin
	// token in the generated private client and never follows redirects.
	if strings.HasPrefix(path, "/_apis/") || path == "/queue" || strings.HasPrefix(path, "/queue/") {
		queueCredentialPath := strings.HasSuffix(path, "/acquirejobs")
		if f.adminToken != "" && !f.auth(r, "Bearer "+f.adminToken) && !queueCredentialPath && path != "/queue" && !strings.HasPrefix(path, "/queue/") {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		f.handleActions(w, r, path)
		return
	}
	f.markUnexpected()
	writeBridgeJSON(w, http.StatusForbidden, nil)
}

func (f *pairedBrokerBridge) handleActions(w http.ResponseWriter, r *http.Request, path string) {
	if path == "/queue" && r.Method == http.MethodGet {
		if !f.auth(r, "Bearer "+f.queueToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		f.polls++
		lastMessageID := r.URL.Query().Get("lastMessageId")
		if f.lastAck == 0 {
			if lastMessageID != "" {
				f.markUnexpected()
			}
		} else if lastMessageID != strconv.Itoa(f.lastAck) {
			f.markUnexpected()
		}
		items, _ := json.Marshal(f.queueItems())
		writeBridgeJSON(w, http.StatusOK, map[string]any{"messageId": 8 + f.polls, "messageType": "RunnerScaleSetJobMessages", "body": string(items), "statistics": map[string]int{"totalAvailableJobs": 0, "totalAcquiredJobs": 0, "totalAssignedJobs": 1, "totalRunningJobs": 0, "totalRegisteredRunners": 0, "totalBusyRunners": 0, "totalIdleRunners": 0}})
		return
	}
	if strings.HasPrefix(path, "/queue/") && r.Method == http.MethodDelete {
		if !f.auth(r, "Bearer "+f.queueToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		id, err := strconv.Atoi(strings.TrimPrefix(path, "/queue/"))
		if err != nil || id != 8+f.polls {
			f.markUnexpected()
		}
		f.lastAck, f.ackCalls = id, f.ackCalls+1
		writeBridgeJSON(w, http.StatusNoContent, nil)
		return
	}
	if strings.HasSuffix(path, "/runnerscalesets") {
		if r.Method == http.MethodGet {
			if f.setExists {
				writeBridgeJSON(w, http.StatusOK, map[string]any{"count": 1, "value": []any{f.scaleSet()}})
			} else {
				writeBridgeJSON(w, http.StatusOK, map[string]any{"count": 0, "value": []any{}})
			}
			return
		}
		if r.Method == http.MethodPost {
			f.setCreated, f.setExists, f.setCreateCalls = true, true, f.setCreateCalls+1
			writeBridgeJSON(w, http.StatusOK, f.scaleSet())
			return
		}
	}
	if strings.HasSuffix(path, "/runnerscalesets/7") {
		switch r.Method {
		case http.MethodGet:
			if !f.setExists {
				f.setAbsenceCalls++
				writeBridgeJSON(w, http.StatusNotFound, map[string]string{"message": "scale set absent"})
				return
			}
			writeBridgeJSON(w, http.StatusOK, f.scaleSet())
		case http.MethodDelete:
			if !f.setExists {
				f.markUnexpected()
				writeBridgeJSON(w, http.StatusNotFound, nil)
				return
			}
			f.setExists, f.setDeleted, f.setDeleteCalls = false, true, f.setDeleteCalls+1
			writeBridgeJSON(w, http.StatusNoContent, nil)
		default:
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
		}
		return
	}
	if strings.HasSuffix(path, "/runnerscalesets/7/sessions") && r.Method == http.MethodPost {
		f.sessionOpenCalls++
		writeBridgeJSON(w, http.StatusOK, map[string]any{"sessionId": "00000000-0000-4000-8000-000000000001", "ownerName": f.setName, "messageQueueUrl": f.server.URL + "/queue", "messageQueueAccessToken": f.queueToken, "statistics": map[string]int{"totalAvailableJobs": 0, "totalAcquiredJobs": 0, "totalAssignedJobs": 1, "totalRunningJobs": 0, "totalRegisteredRunners": 0, "totalBusyRunners": 0, "totalIdleRunners": 0}})
		return
	}
	if strings.Contains(path, "/runnerscalesets/7/sessions/") && r.Method == http.MethodDelete {
		f.sessionCloseCalls++
		writeBridgeJSON(w, http.StatusNoContent, nil)
		return
	}
	if strings.HasSuffix(path, "/runnerscalesets/7/generatejitconfig") && r.Method == http.MethodPost {
		f.jitCalls++
		writeBridgeJSON(w, http.StatusOK, map[string]any{"runner": map[string]any{"id": 81, "name": f.workerName, "runnerScaleSetId": 7}, "encodedJITConfig": pairedBridgeJIT})
		return
	}
	if strings.HasSuffix(path, "/acquirejobs") && r.Method == http.MethodPost {
		if !f.auth(r, "Bearer "+f.queueToken) {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		f.acquireCalls++
		writeBridgeJSON(w, http.StatusOK, map[string]any{"count": 1, "value": []int64{42}})
		return
	}
	if strings.HasSuffix(path, "/agents/81") && r.Method == http.MethodGet {
		f.sdkRunnerCalls++
		if f.polls >= 2 {
			// The real SDK only classifies this as RunnerNotFoundError when the
			// service error includes its canonical typeName.
			writeBridgeJSON(w, http.StatusNotFound, map[string]string{"typeName": "AgentNotFoundException", "message": "synthetic missing runner"})
			return
		}
		writeBridgeJSON(w, http.StatusOK, map[string]any{"id": 81, "name": f.workerName, "runnerScaleSetId": 7})
		return
	}
	if strings.HasSuffix(path, "/agents") && r.Method == http.MethodGet {
		writeBridgeJSON(w, http.StatusOK, map[string]any{"count": 0, "value": []any{}})
		return
	}
	f.markUnexpected()
	writeBridgeJSON(w, http.StatusForbidden, nil)
}

func (f *pairedBrokerBridge) handleDocker(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := r.URL.Path
	if path == "/version" && r.Method == http.MethodGet {
		writeBridgeJSON(w, http.StatusOK, map[string]string{"ApiVersion": "1.51", "MinAPIVersion": "1.24"})
		return
	}
	if path == "/v1.45/info" && r.Method == http.MethodGet {
		writeBridgeJSON(w, http.StatusOK, map[string]any{"ID": f.workerDaemonID, "OSType": "linux", "Architecture": "aarch64", "NCPU": 4, "MemTotal": int64(4 << 30), "MemoryLimit": true, "SwapLimit": true, "CpuCfsQuota": true, "PidsLimit": true, "Warnings": []string{}})
		return
	}
	if strings.HasPrefix(path, "/v1.45/images/") && strings.HasSuffix(path, "/json") && r.Method == http.MethodGet {
		writeBridgeJSON(w, http.StatusOK, map[string]any{"Id": f.workerImageID, "Os": "linux", "Architecture": "arm64", "RepoDigests": []string{f.workerImage}, "Config": map[string]any{"Env": []string{"PATH=/usr/bin"}, "Labels": map[string]string{}, "Volumes": map[string]any{}, "ExposedPorts": map[string]any{}}})
		return
	}
	containerPath := "/v1.45/containers/" + strings.Repeat("c", 64)
	if path == "/v1.45/containers/create" && r.Method == http.MethodPost {
		var payload map[string]any
		if json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&payload) != nil || r.URL.Query().Get("name") != f.workerName {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusBadRequest, nil)
			return
		}
		env, ok := payload["Env"].([]any)
		if !ok || len(env) != 1 {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusBadRequest, nil)
			return
		}
		host := payload["HostConfig"]
		delete(payload, "HostConfig")
		payload["Env"] = append([]any{"PATH=/usr/bin"}, env...)
		f.container = map[string]any{"Id": strings.Repeat("c", 64), "Name": "/" + f.workerName, "Image": f.workerImageID, "Path": "/home/runner/bin/Runner.Listener", "Args": []string{"run", "--once"}, "Config": payload, "HostConfig": host, "Mounts": []any{}, "State": map[string]any{"Status": "created", "Running": false, "Paused": false, "Restarting": false, "Dead": false, "ExitCode": 0}, "NetworkSettings": map[string]any{"Networks": map[string]any{"bridge": map[string]any{}}}}
		f.createCalls++
		writeBridgeJSON(w, http.StatusCreated, map[string]any{"Id": strings.Repeat("c", 64), "Warnings": []any{}})
		return
	}
	if path == containerPath+"/start" && r.Method == http.MethodPost {
		if f.container == nil {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusNotFound, nil)
			return
		}
		f.containerStarted = true
		f.startCalls++
		state := f.container["State"].(map[string]any)
		state["Status"], state["Running"] = "running", true
		writeBridgeJSON(w, http.StatusNoContent, nil)
		return
	}
	if path == containerPath+"/json" && r.Method == http.MethodGet {
		if f.containerDeleted || f.container == nil {
			f.workerAbsenceCalls++
			writeBridgeJSON(w, http.StatusNotFound, map[string]string{"message": "container absent"})
			return
		}
		if f.containerStarted && f.polls >= 2 {
			state := f.container["State"].(map[string]any)
			state["Status"], state["Running"] = "exited", false
		}
		writeBridgeJSON(w, http.StatusOK, f.container)
		return
	}
	if path == containerPath && r.Method == http.MethodDelete {
		if r.URL.Query().Get("force") != "false" || r.URL.Query().Get("v") != "false" || f.container == nil {
			f.markUnexpected()
			writeBridgeJSON(w, http.StatusForbidden, nil)
			return
		}
		f.containerDeleted, f.workerDeleteCalls = true, f.workerDeleteCalls+1
		writeBridgeJSON(w, http.StatusNoContent, nil)
		return
	}
	f.markUnexpected()
	writeBridgeJSON(w, http.StatusForbidden, nil)
}

func bridgeCA(t *testing.T, server *httptest.Server) string {
	t.Helper()
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
}

func bridgeRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("bridge test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func buildPairedG01Binary(t *testing.T) (string, string, string) {
	t.Helper()
	repo := bridgeRepoRoot(t)
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = repo
	harnessBytes, err := command.Output()
	if err != nil {
		t.Fatal("read reviewed bridge head")
	}
	harness := strings.TrimSpace(string(harnessBytes))
	if len(harness) != 40 {
		t.Fatal("invalid reviewed bridge head")
	}
	// The checkout contains nested Go modules and the active worktree's .git
	// indirection is intentionally not used for build stamping. Build from a
	// clean temporary clone so Go records vcs.revision and vcs.modified=false
	// in the executable that the broker verifies.
	cloneRoot := filepath.Join(t.TempDir(), "repo")
	command = exec.Command("git", "clone", "--no-hardlinks", "--quiet", repo, cloneRoot)
	if output, err := command.CombinedOutput(); err != nil {
		_ = output
		t.Fatal("clone reviewed g01 bridge source")
	}
	out := filepath.Join(t.TempDir(), "g01-live")
	command = exec.Command("go", "build", "-buildvcs=true", "-tags", "g01_live,g01_pair_fixture", "-o", out, "./cmd/g01-live")
	command.Dir = filepath.Join(cloneRoot, "experiments", "g01-scaleset")
	command.Env = append(os.Environ(), "GOTOOLCHAIN=go1.26.8")
	if output, err := command.CombinedOutput(); err != nil {
		_ = output
		t.Fatal("build reviewed g01 bridge binary")
	}
	if err := os.Chmod(out, 0500); err != nil {
		t.Fatal("pin reviewed bridge binary mode")
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal("read reviewed bridge binary")
	}
	digest := sha256.Sum256(data)
	return out, harness, hexDigest(digest[:])
}

func hexDigest(data []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(data)*2)
	for i, value := range data {
		out[2*i], out[2*i+1] = hex[value>>4], hex[value&15]
	}
	return string(out)
}

func runBridgeCommand(t *testing.T, ctx context.Context, binary string, args []string, input []byte) []byte {
	t.Helper()
	command := exec.CommandContext(ctx, binary, args...)
	command.Env = []string{"LANG=C", "LC_ALL=C"}
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		// g01-live's output boundary is fixed text; retaining it here makes a
		// local fixture failure diagnosable without exposing credentials or SDK
		// response bodies.
		t.Fatalf("reviewed g01 bridge command failed: %q", strings.TrimSpace(string(output)))
	}
	return output
}

func writePrivateBridgeJSON(t *testing.T, path string, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil || os.WriteFile(path, data, 0600) != nil {
		t.Fatal("private bridge JSON")
	}
	return data
}

func TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private Unix fixture requires a Unix host")
	}
	binaryPath, harness, binaryDigest := buildPairedG01Binary(t)
	bridge := newPairedBrokerBridge(t)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal("private bridge state root")
	}
	controllerState := filepath.Join(parent, "controller-state")
	workerState := filepath.Join(parent, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal("controller bridge state")
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal("worker bridge state")
	}
	admissionConfig := map[string]string{"base_url": bridge.server.URL, "ca_pem": bridgeCA(t, bridge.server), "admission_directory": ""}
	_, candidate, api, brokerFixture, attempt := newBrokerFixture(t)
	admissionConfig["admission_directory"] = brokerFixture.admissionRoot
	configPath := filepath.Join(controllerState, "paired-fixture.json")
	configData := writePrivateBridgeJSON(t, configPath, admissionConfig)
	// The worker-preparation command receives only its worker state path. The
	// fixture-only adapter therefore carries the same generated loopback
	// endpoint config in that disposable state root; production preparation
	// never reads this file or accepts a caller-selected admission root.
	writePrivateBridgeJSON(t, filepath.Join(workerState, "paired-fixture.json"), json.RawMessage(configData))

	now := time.Now()
	expires := now.Add(20 * time.Minute)
	workflow := strings.Repeat("b", 40)
	controller := controllerApproval{AppID: 71, InstallationID: 201, Organization: bridge.organization, Repository: bridge.repository, RepositoryID: 501, RunnerGroupID: bridge.runnerGroupID, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: harness, WorkflowSHA: workflow, WorkflowPath: bridge.workflowPath, WorkflowRunID: bridge.workflowRunID, Controller: "trusted-controller", ExpiresAt: expires, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "inspect", "cleanup"}}
	controllerPath := filepath.Join(parent, "controller-approval.json")
	controllerData := writePrivateBridgeJSON(t, controllerPath, controller)
	credentials := map[string]any{"installation_token": bridge.installationToken, "verification_token": bridge.verificationToken, "app_id": 71, "installation_id": 201, "organization": bridge.organization, "expires_at": expires, "organization_self_hosted_runners": "write", "metadata": "read"}
	credentialData, err := json.Marshal(credentials)
	if err != nil {
		t.Fatal("controller credentials")
	}
	createCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	runBridgeCommand(t, createCtx, binaryPath, []string{"--execute-approved-canary", "--approval", controllerPath, "--state-dir", controllerState, "--phase", "create"}, credentialData)
	cancel()
	journalData, err := os.ReadFile(filepath.Join(controllerState, "journal.jsonl"))
	if err != nil {
		t.Fatal("controller journal")
	}
	var operations []string
	for _, line := range strings.Split(strings.TrimSpace(string(journalData)), "\n")[1:] {
		var event struct {
			Kind      string `json:"kind"`
			Operation string `json:"operation"`
			ID        int    `json:"id"`
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			t.Fatal("controller journal event")
		}
		if event.Kind == "phase" || event.Kind == "inventory" || event.Kind == "intent" || event.Kind == "result" {
			operations = append(operations, event.Kind+":"+event.Operation)
		}
	}
	want := []string{"phase:create", "inventory:", "intent:observe-discovery", "result:observe-discovery", "intent:create", "result:create"}
	if len(operations) != len(want) {
		t.Fatalf("controller create did not produce canonical history: %v", operations)
	}
	for i := range want {
		if operations[i] != want[i] {
			t.Fatalf("controller create history[%d]=%q want %q", i, operations[i], want[i])
		}
	}
	if _, err := os.Stat(filepath.Join(brokerFixture.admissionRoot, "admission.json")); err != nil {
		t.Fatal("controller admission claim")
	}
	worker := pairedWorkerApproval{RunnerUpdatesDisabled: true, HarnessSHA: harness, WorkflowSHA: workflow, OwnerNonce: controller.OwnerNonce, Controller: controller.Controller, Endpoint: bridge.dockerEndpoint, DaemonID: bridge.workerDaemonID, ImageID: bridge.workerImageID, Image: pairedWorkerImage, ExpiresAt: expires, Phases: []string{"create", "start", "inspect", "cleanup"}}
	workerPath := filepath.Join(parent, "worker-approval.json")
	writePrivateBridgeJSON(t, workerPath, worker)
	brokerApproval := brokerApprovalFixture()
	brokerApproval.Mode, brokerApproval.Phase = "paired-terminal", "paired-terminal"
	brokerApproval.AllowVerificationAuthority = true
	brokerApproval.ExpiresAt = expires
	brokerApproval.OwnerNonce = controller.OwnerNonce
	brokerApproval.ControllerHarnessSHA = harness
	brokerApproval.ControllerBinarySHA256 = binaryDigest
	controllerDigest := sha256.Sum256(controllerData)
	brokerApproval.ControllerApprovalSHA256 = hexDigest(controllerDigest[:])
	approvalPath := filepath.Join(parent, "broker-approval.json")
	writePrivateBridgeJSON(t, approvalPath, brokerApproval)
	inputPath := filepath.Join(parent, "broker-input.json")
	inputData := writePrivateBridgeJSON(t, inputPath, brokerInput{PEM: string(candidate.PEM), VerificationToken: bridge.verificationToken})

	oldOpener := brokerBinaryOpener
	brokerBinaryOpener = func(path string, a BrokerApproval) (*verifiedBrokerBinary, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, errBroker
		}
		return &verifiedBrokerBinary{path: path, file: file, digest: a.ControllerBinarySHA256}, nil
	}
	defer func() { brokerBinaryOpener = oldOpener }()
	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatal("broker input")
	}
	brokerCtx, brokerCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	result, err := runBrokerWithAPI(brokerCtx, BrokerFiles{ApprovalPath: approvalPath, StateDirectory: attempt, ControllerBinary: binaryPath, ControllerApproval: controllerPath, ControllerStateDirectory: controllerState, WorkerApproval: workerPath, WorkerStateDirectory: workerState}, input, api)
	brokerCancel()
	if err != nil || result.Status != "paired_terminal_completed" {
		t.Fatalf("real paired bridge did not complete: status=%q err=%v", result.Status, err)
	}
	journalData, err = os.ReadFile(filepath.Join(controllerState, "journal.jsonl"))
	if err != nil {
		t.Fatal("controller journal after paired terminal")
	}
	bridge.mu.Lock()
	counts := []int{bridge.createCalls, bridge.startCalls, bridge.jitCalls, bridge.acquireCalls, bridge.ackCalls, bridge.sessionOpenCalls, bridge.sessionCloseCalls, bridge.workerDeleteCalls, bridge.workerAbsenceCalls, bridge.setCreateCalls, bridge.setDeleteCalls, bridge.setAbsenceCalls, bridge.rosterCalls, bridge.unexpected}
	bridge.mu.Unlock()
	if want := []int{1, 1, 1, 1, 2, 1, 1, 1, 1, 1, 1, 1, 4, 0}; !slices.Equal(counts, want) {
		t.Fatalf("real paired effect counts=%v want=%v", counts, want)
	}
	if brokerFixture.tokenCalls != 1 {
		t.Fatalf("real paired bridge minted %d tokens", brokerFixture.tokenCalls)
	}
	ledger, err := os.ReadFile(filepath.Join(brokerFixture.admissionRoot, "broker-admission.jsonl"))
	if err != nil || !strings.Contains(string(ledger), `"slot":"paired-terminal"`) || !strings.Contains(string(ledger), `"worker"`) {
		t.Fatal("paired broker ledger missing controller/worker claim")
	}
	for _, root := range []string{attempt, controllerState, workerState, brokerFixture.admissionRoot} {
		assertNoSecretFiles(t, root, string(candidate.PEM), "PRIVATE KEY", bridge.installationToken, bridge.verificationToken, pairedBridgeJIT, "synthetic-private-registration-token", "synthetic-private-admin-token", bridge.queueToken)
	}
	if !strings.Contains(string(journalData), `"kind":"baseline"`) {
		t.Fatal("terminal child did not append to the original controller journal")
	}
	workerJournal, err := os.ReadFile(filepath.Join(workerState, "journal.jsonl"))
	if err != nil || !strings.Contains(string(workerJournal), `"kind":"paired"`) {
		t.Fatal("terminal child did not create the worker paired journal")
	}

	// A completed paired claim is one-shot. Reopening the real broker entrypoint
	// must stop before the second mint or any terminal effect.
	retryPath := filepath.Join(parent, "broker-input-retry.json")
	if err := os.WriteFile(retryPath, inputData, 0600); err != nil {
		t.Fatal("retry input")
	}
	retryInput, err := os.Open(retryPath)
	if err != nil {
		t.Fatal("retry input open")
	}
	_, retryErr := runBrokerWithAPI(context.Background(), BrokerFiles{ApprovalPath: approvalPath, StateDirectory: attempt, ControllerBinary: binaryPath, ControllerApproval: controllerPath, ControllerStateDirectory: controllerState, WorkerApproval: workerPath, WorkerStateDirectory: workerState}, retryInput, api)
	if retryErr == nil || brokerFixture.tokenCalls != 1 {
		t.Fatal("completed paired claim replayed effects")
	}
}
