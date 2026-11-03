//go:build g01_live

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

func TestBuildRevisionAdmissionRequiresGo1273(t *testing.T) {
	revision := strings.Repeat("a", 40)
	good := &debug.BuildInfo{
		Path:      "github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/cmd/g01-live",
		GoVersion: "go1.27.3",
		Deps:      []*debug.Module{{Path: "github.com/actions/scaleset", Version: "v0.5.0"}},
		Settings: []debug.BuildSetting{
			{Key: "GOOS", Value: runtime.GOOS},
			{Key: "GOARCH", Value: runtime.GOARCH},
			{Key: "CGO_ENABLED", Value: "1"},
			{Key: "-tags", Value: "g01_live"},
			{Key: "vcs.revision", Value: revision},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	if got, ok := buildRevisionFromInfo(good, true); !ok || got != revision {
		t.Fatalf("Go 1.27.3 reviewed build rejected: revision=%q admitted=%v", got, ok)
	}
	for _, version := range []string{"go1.26.8", "go1.26.9", "go1.27.2", "go1.27.3-X:nojsonv2", "go1.28.1", ""} {
		bad := *good
		bad.GoVersion = version
		if _, ok := buildRevisionFromInfo(&bad, true); ok {
			t.Errorf("build toolchain %q admitted", version)
		}
	}
	for _, version := range []string{"v0.4.0", "v0.5.1", ""} {
		bad := *good
		bad.Deps = []*debug.Module{{Path: "github.com/actions/scaleset", Version: version}}
		if _, ok := buildRevisionFromInfo(&bad, true); ok {
			t.Errorf("unselected SDK %q admitted", version)
		}
	}
	wrongGOOS := "windows"
	if runtime.GOOS == wrongGOOS {
		wrongGOOS = "linux"
	}
	wrongGOARCH := "amd64"
	if runtime.GOARCH == wrongGOARCH {
		wrongGOARCH = "arm64"
	}
	for _, tc := range []struct {
		name   string
		mutate func(*debug.BuildInfo)
	}{
		{name: "wrong path", mutate: func(info *debug.BuildInfo) { info.Path = "example.com/other" }},
		{name: "missing platform", mutate: func(info *debug.BuildInfo) { info.Settings = removeBuildSetting(info.Settings, "GOOS") }},
		{name: "wrong platform", mutate: func(info *debug.BuildInfo) { setBuildSetting(info, "GOOS", wrongGOOS) }},
		{name: "wrong architecture", mutate: func(info *debug.BuildInfo) { setBuildSetting(info, "GOARCH", wrongGOARCH) }},
		{name: "cgo disabled", mutate: func(info *debug.BuildInfo) { setBuildSetting(info, "CGO_ENABLED", "0") }},
		{name: "missing tags", mutate: func(info *debug.BuildInfo) { info.Settings = removeBuildSetting(info.Settings, "-tags") }},
		{name: "fixture tag", mutate: func(info *debug.BuildInfo) { setBuildSetting(info, "-tags", "g01_live,g01_pair_fixture") }},
		{name: "dirty source", mutate: func(info *debug.BuildInfo) { setBuildSetting(info, "vcs.modified", "true") }},
		{name: "replaced SDK", mutate: func(info *debug.BuildInfo) {
			info.Deps[0].Replace = &debug.Module{Path: "github.com/actions/scaleset", Version: "v0.5.0"}
		}},
	} {
		bad := *good
		bad.Settings = append([]debug.BuildSetting(nil), good.Settings...)
		bad.Deps = append([]*debug.Module(nil), good.Deps...)
		for i, dep := range bad.Deps {
			if dep != nil {
				copy := *dep
				bad.Deps[i] = &copy
			}
		}
		tc.mutate(&bad)
		if _, ok := buildRevisionFromInfo(&bad, true); ok {
			t.Errorf("unsupported build identity admitted: %s", tc.name)
		}
	}
}

func setBuildSetting(info *debug.BuildInfo, key, value string) {
	for i := range info.Settings {
		if info.Settings[i].Key == key {
			info.Settings[i].Value = value
			return
		}
	}
	info.Settings = append(info.Settings, debug.BuildSetting{Key: key, Value: value})
}

func removeBuildSetting(settings []debug.BuildSetting, key string) []debug.BuildSetting {
	result := settings[:0]
	for _, setting := range settings {
		if setting.Key != key {
			result = append(result, setting)
		}
	}
	return result
}

type unreadable struct{ t *testing.T }

func (r unreadable) Read([]byte) (int, error) {
	r.t.Fatal("credentials read before explicit exact-build approval")
	return 0, nil
}

type countedInput struct {
	io.Reader
	reads         int
	eofs          int
	readsAfterEOF int
}

func pairedBindingFixture(t *testing.T, controllerPath, controllerState, workerPath, workerState string) livecanary.PairedTerminalBinding {
	t.Helper()
	identity := func(path string) (uint64, uint64) {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			t.Fatal("fixture identity")
		}
		return uint64(stat.Dev), stat.Ino
	}
	digest := func(path string) string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	controllerDevice, controllerInode := identity(controllerPath)
	controllerStateDevice, controllerStateInode := identity(controllerState)
	workerDevice, workerInode := identity(workerPath)
	workerStateDevice, workerStateInode := identity(workerState)
	return livecanary.PairedTerminalBinding{ControllerApprovalSHA256: digest(controllerPath), ControllerApprovalDevice: controllerDevice, ControllerApprovalInode: controllerInode, ControllerStateDevice: controllerStateDevice, ControllerStateInode: controllerStateInode, WorkerApprovalSHA256: digest(workerPath), WorkerApprovalDevice: workerDevice, WorkerApprovalInode: workerInode, WorkerStateDevice: workerStateDevice, WorkerStateInode: workerStateInode}
}

func (r *countedInput) Close() error { return nil }
func (r *countedInput) Read(p []byte) (int, error) {
	r.reads++
	if r.eofs != 0 {
		r.readsAfterEOF++
	}
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.eofs++
	}
	return n, err
}

type chunkedControllerInput struct {
	chunks        [][]byte
	index         int
	reads         int
	eofs          int
	readsAfterEOF int
}

func (r *chunkedControllerInput) Read(p []byte) (int, error) {
	r.reads++
	if r.eofs != 0 {
		r.readsAfterEOF++
		return 0, io.EOF
	}
	if r.index == len(r.chunks) {
		r.eofs++
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.index])
	r.index++
	return n, nil
}

func (r *chunkedControllerInput) Close() error { return nil }

func TestPlanAndRefusalsNeverReadCredentialsOrEchoInputs(t *testing.T) {
	for _, args := range [][]string{{"--plan"}, {}, {"--synthetic-secret=do-not-print"}, {"--execute-approved-canary", "--approval=synthetic-secret", "--state-dir=synthetic-secret", "--phase=create"}} {
		var out bytes.Buffer
		code := run(args, unreadable{t}, &out)
		if len(args) == 1 && args[0] == "--plan" {
			if code != 0 {
				t.Fatal("offline plan failed")
			}
		} else if code == 0 {
			t.Fatal("unsafe invocation accepted")
		}
		if strings.Contains(out.String(), "synthetic-secret") || strings.Contains(out.String(), "do-not-print") {
			t.Fatal("caller input leaked")
		}
	}
}

func TestPublicEvidenceSanitizerPublishesOnlyAllowlistedFields(t *testing.T) {
	input := `[{"elapsed_ms":1250,"attempt":1,"session_generation":2,"phase":"after-ack","operation":"ack","outcome":"unknown","resources":[{"kind":"session","id":"private-session-identifier"},{"kind":"message","id":"private-message-identifier"}],"http_status":503,"succeeded":false,"counts":{"available":0,"assigned":1}}]`
	var out bytes.Buffer
	code := runWithPreparation([]string{"--sanitize-public-evidence"}, strings.NewReader(input), &out, func() (string, bool) {
		t.Fatal("sanitization must not inspect live build provenance")
		return "", false
	}, nil)
	if code != 0 {
		t.Fatalf("sanitization failed: output=%q", out.String())
	}
	for _, forbidden := range []string{"private-session-identifier", "private-message-identifier", "http_status\":503", "authorization"} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatalf("sanitizer emitted private input %q: %s", forbidden, out.String())
		}
	}
	for _, required := range []string{`"schema":"gh-runnerd/g01-public-evidence/v1"`, `"elapsed_ms":1250`, `"http_status_class":"5xx"`, `"succeeded":false`, `"session-001"`, `"message-001"`, `"available":0`, `"assigned":1`} {
		if !strings.Contains(out.String(), required) {
			t.Fatalf("sanitizer omitted %s: %s", required, out.String())
		}
	}
}

func TestPublicEvidenceSanitizerRejectsInvalidOrMixedInputWithoutPublishing(t *testing.T) {
	valid := `[{"elapsed_ms":1,"phase":"baseline","operation":"inspect","outcome":"observed"}]`
	oversized := valid + strings.Repeat(" ", maxPublicEvidenceInputBytes+1-len(valid))
	for _, tc := range []struct {
		name  string
		args  []string
		input string
	}{
		{name: "unknown private field", args: []string{"--sanitize-public-evidence"}, input: `[{"elapsed_ms":1,"phase":"baseline","operation":"inspect","outcome":"observed","private":"synthetic-secret"}]`},
		{name: "trailing document", args: []string{"--sanitize-public-evidence"}, input: valid + ` {"private":"synthetic-secret"}`},
		{name: "malformed input", args: []string{"--sanitize-public-evidence"}, input: `[{"phase":`},
		{name: "oversized input", args: []string{"--sanitize-public-evidence"}, input: oversized},
		{name: "elapsed exceeds bound", args: []string{"--sanitize-public-evidence"}, input: `[{"elapsed_ms":86400001,"phase":"baseline","operation":"inspect","outcome":"observed"}]`},
		{name: "case-folded event field", args: []string{"--sanitize-public-evidence"}, input: `[{"elapsed_ms":1,"Phase":"baseline","operation":"inspect","outcome":"observed"}]`},
		{name: "duplicate event field", args: []string{"--sanitize-public-evidence"}, input: `[{"elapsed_ms":1,"phase":"baseline","phase":"cleanup","operation":"inspect","outcome":"observed"}]`},
		{name: "duplicate nested resource field", args: []string{"--sanitize-public-evidence"}, input: `[{"elapsed_ms":1,"phase":"baseline","operation":"inspect","outcome":"observed","resources":[{"kind":"session","id":"first","id":"second"}]}]`},
		{name: "null required number", args: []string{"--sanitize-public-evidence"}, input: `[{"elapsed_ms":null,"phase":"baseline","operation":"inspect","outcome":"observed"}]`},
		{name: "null optional number", args: []string{"--sanitize-public-evidence"}, input: `[{"elapsed_ms":1,"attempt":null,"phase":"baseline","operation":"inspect","outcome":"observed"}]`},
		{name: "null optional container", args: []string{"--sanitize-public-evidence"}, input: `[{"elapsed_ms":1,"phase":"baseline","operation":"inspect","outcome":"observed","resources":null}]`},
		{name: "mixed plan", args: []string{"--sanitize-public-evidence", "--plan"}, input: valid},
		{name: "approval path", args: []string{"--sanitize-public-evidence", "--approval", "/private/approval.json"}, input: valid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code := runWithPreparation(tc.args, strings.NewReader(tc.input), &out, func() (string, bool) {
				t.Fatal("sanitization refusal must not inspect live build provenance")
				return "", false
			}, nil)
			if code == 0 || strings.Contains(out.String(), "synthetic-secret") || strings.Contains(out.String(), "g01-public-evidence/v1") {
				t.Fatalf("invalid input was published: code=%d output=%q", code, out.String())
			}
		})
	}
}

func TestPublicEvidenceJSONScannerBoundsNesting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		depth   int
		wantErr bool
	}{
		{name: "at limit", depth: maxPublicEvidenceJSONDepth},
		{name: "over limit", depth: maxPublicEvidenceJSONDepth + 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := strings.Repeat("[", tc.depth) + "null" + strings.Repeat("]", tc.depth)
			decoder := json.NewDecoder(strings.NewReader(input))
			decoder.UseNumber()
			err := consumePublicEvidenceJSONValue(decoder)
			if (err != nil) != tc.wantErr {
				t.Fatalf("nesting depth %d error=%v, wantErr=%v", tc.depth, err, tc.wantErr)
			}
		})
	}
}

func TestPreparationCommandNeverReadsCredentialsOrRunsRemotePhase(t *testing.T) {
	a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create"}}
	parent := t.TempDir()
	path := filepath.Join(parent, "approval.json")
	data, _ := json.Marshal(a)
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("fixture")
	}
	args := []string{"--prepare-approved-journal", "--approval", path, "--state-dir", parent, "--phase", "create"}
	for _, kind := range []string{"success", "failed preparation", "mixed execution", "mixed plan", "unapproved phase"} {
		t.Run(kind, func(t *testing.T) {
			argv := append([]string(nil), args...)
			called := 0
			switch kind {
			case "mixed execution":
				argv = append(argv, "--execute-approved-canary")
			case "mixed plan":
				argv = append(argv, "--plan")
			case "unapproved phase":
				argv[len(argv)-1] = "cleanup"
			}
			var out bytes.Buffer
			code := runWithPreparation(argv, unreadable{t}, &out, func() (string, bool) { return a.HarnessSHA, true }, func(state string, got livecanary.Approval, phase string) (livecanary.PreparationReceipt, error) {
				called++
				if state != parent || got.HarnessSHA != a.HarnessSHA || phase != "create" {
					t.Fatal("preparation identity changed")
				}
				if kind == "failed preparation" {
					return livecanary.PreparationReceipt{}, livecanary.ErrJournal
				}
				return livecanary.PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: phase}, nil
			})
			if kind == "success" {
				if code != 0 || called != 1 || !strings.Contains(out.String(), `"status":"controller_journal_prepared"`) {
					t.Fatal("fixed local success missing")
				}
			} else {
				if code == 0 {
					t.Fatal("ambiguous preparation accepted")
				}
				if kind != "failed preparation" && called != 0 {
					t.Fatal("invalid flags/phase reached state preparation")
				}
			}
			if strings.Contains(out.String(), parent) {
				t.Fatal("private path disclosed")
			}
		})
	}
}

func TestPairedTerminalModeQuarantinesBeforeControllerInput(t *testing.T) {
	a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 5, Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "inspect", "cleanup", "jit-loss"}}
	worker := liveworker.Approval{RunnerUpdatesDisabled: true, HarnessSHA: a.HarnessSHA, WorkflowSHA: a.WorkflowSHA, OwnerNonce: a.OwnerNonce, Controller: a.Controller, Endpoint: "/tmp/g01-paired-docker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: liveworker.ImageReference, ExpiresAt: a.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	root := t.TempDir()
	controllerState := filepath.Join(root, "controller-state")
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	controllerPath := filepath.Join(root, "controller.json")
	workerPath := filepath.Join(root, "worker.json")
	controllerData, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	workerData, err := json.Marshal(worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal(err)
	}
	binding := pairedBindingFixture(t, controllerPath, controllerState, workerPath, workerState)
	input := &chunkedControllerInput{chunks: [][]byte{[]byte(`{`), []byte(`}`)}}
	var out bytes.Buffer
	bindingData, _ := json.Marshal(binding)
	code := runWithPreparation([]string{"--execute-approved-paired-terminal", "--approval", controllerPath, "--state-dir", controllerState, "--worker-approval", workerPath, "--worker-state-dir", workerState, "--paired-binding", string(bindingData)}, input, &out, func() (string, bool) { return a.HarnessSHA, true }, func(string, livecanary.Approval, string) (livecanary.PreparationReceipt, error) {
		t.Fatal("paired mode entered controller-only preparation")
		return livecanary.PreparationReceipt{}, nil
	})
	if code == 0 || input.reads != 0 || input.eofs != 0 || input.readsAfterEOF != 0 {
		t.Fatalf("paired mode crossed quarantined controller-input boundary: code=%d reads=%d eofs=%d rereads=%d output=%q", code, input.reads, input.eofs, input.readsAfterEOF, out.String())
	}
}

func TestPairedTerminalModeRejectsUnusedPhaseAndControllerFlagsBeforeInput(t *testing.T) {
	a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 5, Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "before-ack", "after-ack", "before-acquire", "acquire-loss", "inspect", "cleanup", "jit-loss"}}
	worker := liveworker.Approval{RunnerUpdatesDisabled: true, HarnessSHA: a.HarnessSHA, WorkflowSHA: a.WorkflowSHA, OwnerNonce: a.OwnerNonce, Controller: a.Controller, Endpoint: "/tmp/g01-paired-docker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: liveworker.ImageReference, ExpiresAt: a.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	root := t.TempDir()
	controllerState := filepath.Join(root, "controller-state")
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	controllerPath := filepath.Join(root, "controller.json")
	workerPath := filepath.Join(root, "worker.json")
	controllerData, _ := json.Marshal(a)
	workerData, _ := json.Marshal(worker)
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal(err)
	}
	bindingData, _ := json.Marshal(pairedBindingFixture(t, controllerPath, controllerState, workerPath, workerState))
	base := []string{"--execute-approved-paired-terminal", "--approval", controllerPath, "--state-dir", controllerState, "--worker-approval", workerPath, "--worker-state-dir", workerState, "--paired-binding", string(bindingData)}
	for _, extra := range [][]string{{"--phase", "cleanup"}, {"--execute-approved-canary"}} {
		input := &countedInput{Reader: strings.NewReader(`{}`)}
		args := append(append([]string(nil), base...), extra...)
		var out bytes.Buffer
		if code := runWithPreparation(args, input, &out, func() (string, bool) { return a.HarnessSHA, true }, nil); code == 0 || input.reads != 0 {
			t.Fatalf("incompatible paired flags reached input: extra=%v code=%d reads=%d", extra, code, input.reads)
		}
	}
}

func TestPairedTerminalModeRequiresWorkflowVerificationAuthorityBeforeInput(t *testing.T) {
	a := livecanary.Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: strings.Repeat("b", 40), WorkflowSHA: strings.Repeat("c", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 5, Controller: "fixture-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "inspect", "cleanup"}}
	worker := liveworker.Approval{RunnerUpdatesDisabled: true, HarnessSHA: a.HarnessSHA, WorkflowSHA: a.WorkflowSHA, OwnerNonce: a.OwnerNonce, Controller: a.Controller, Endpoint: "/tmp/g01-paired-docker.sock", DaemonID: "fixture-daemon", ImageID: "sha256:" + strings.Repeat("d", 64), Image: liveworker.ImageReference, ExpiresAt: a.ExpiresAt, Phases: []string{"create", "start", "inspect", "cleanup"}}
	root := t.TempDir()
	controllerState := filepath.Join(root, "controller-state")
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(controllerState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	controllerPath := filepath.Join(root, "controller.json")
	workerPath := filepath.Join(root, "worker.json")
	controllerData, _ := json.Marshal(a)
	workerData, _ := json.Marshal(worker)
	if err := os.WriteFile(controllerPath, controllerData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workerPath, workerData, 0600); err != nil {
		t.Fatal(err)
	}
	bindingData, _ := json.Marshal(pairedBindingFixture(t, controllerPath, controllerState, workerPath, workerState))
	input := &countedInput{Reader: strings.NewReader(`{}`)}
	var out bytes.Buffer
	args := []string{"--execute-approved-paired-terminal", "--approval", controllerPath, "--state-dir", controllerState, "--worker-approval", workerPath, "--worker-state-dir", workerState, "--paired-binding", string(bindingData)}
	code := runWithPreparation(args, input, &out, func() (string, bool) { return a.HarnessSHA, true }, nil)
	if code == 0 || input.reads != 0 {
		t.Fatalf("paired mode accepted missing verification authority or read input: code=%d reads=%d output=%q", code, input.reads, out.String())
	}
}
