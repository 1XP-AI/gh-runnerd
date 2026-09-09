package enrollment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This child is only the Go test executable, never a controller or worker. The
// production entry point must separately verify G01 build metadata before it
// can construct verifiedBrokerBinary; these pipe tests isolate that handoff.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-paired-journal" {
		if len(os.Args) != 6 || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" {
			os.Exit(3)
		}
		for _, entry := range os.Environ() {
			if entry != "LANG=C" && entry != "LC_ALL=C" {
				os.Exit(4)
			}
		}
		data, e := io.ReadAll(io.LimitReader(os.Stdin, 1))
		if e != nil || len(data) != 0 {
			os.Exit(5)
		}
		var controller controllerApproval
		_, e = readBrokerPrivateJSON(os.Args[3], &controller)
		if e != nil {
			os.Exit(6)
		}
		statePath := os.Args[5]
		admissionPath := filepath.Join(filepath.Dir(statePath), "admission")
		if e = os.Mkdir(admissionPath, 0700); e != nil && !os.IsExist(e) {
			os.Exit(7)
		}
		journalPath := filepath.Join(statePath, "journal.jsonl")
		if _, e = os.Stat(journalPath); os.IsNotExist(e) {
			if e = os.WriteFile(journalPath, []byte("synthetic prepared journal\n"), 0600); e != nil {
				os.Exit(8)
			}
		}
		journalInfo, e := os.Stat(journalPath)
		if e != nil {
			os.Exit(9)
		}
		stateInfo, e := os.Stat(statePath)
		if e != nil {
			os.Exit(10)
		}
		claimPath := filepath.Join(admissionPath, "admission.json")
		if _, e = os.Stat(claimPath); os.IsNotExist(e) {
			stateID := brokerFileIdentity(stateInfo)
			journalID := brokerFileIdentity(journalInfo)
			ownership := controller
			ownership.ExpiresAt = time.Time{}
			ownership.Phases = nil
			claim := map[string]any{"version": 1, "ownership": brokerDigest(ownership), "state_device": stateID.Device, "state_inode": stateID.Inode, "journal_device": journalID.Device, "journal_inode": journalID.Inode}
			claimData, _ := json.Marshal(claim)
			if e = os.WriteFile(claimPath, append(claimData, '\n'), 0600); e != nil {
				os.Exit(11)
			}
		}
		claimInfo, e := os.Stat(claimPath)
		if e != nil {
			os.Exit(12)
		}
		journalData, e := os.ReadFile(journalPath)
		if e != nil {
			os.Exit(13)
		}
		claimData, e := os.ReadFile(claimPath)
		if e != nil {
			os.Exit(14)
		}
		admissionInfo, e := os.Lstat(admissionPath)
		if e != nil {
			os.Exit(15)
		}
		receipt := brokerPreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: "paired-terminal", ApprovalDigest: brokerDigest(controller), State: brokerFileIdentity(stateInfo), Journal: brokerFileIdentity(journalInfo), Claim: brokerFileIdentity(claimInfo), AdmissionDirectory: brokerFileIdentity(admissionInfo), JournalDigest: brokerBytesDigest(journalData), ClaimDigest: brokerBytesDigest(claimData)}
		if json.NewEncoder(os.Stdout).Encode(receipt) != nil {
			os.Exit(13)
		}
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-paired-worker-journal" {
		if len(os.Args) != 6 || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" {
			os.Exit(3)
		}
		for _, entry := range os.Environ() {
			if entry != "LANG=C" && entry != "LC_ALL=C" {
				os.Exit(4)
			}
		}
		data, e := io.ReadAll(io.LimitReader(os.Stdin, 1))
		if e != nil || len(data) != 0 {
			os.Exit(5)
		}
		var worker pairedWorkerApproval
		if _, e = readBrokerPrivateJSON(os.Args[3], &worker); e != nil {
			os.Exit(6)
		}
		statePath := os.Args[5]
		admissionPath := filepath.Join(filepath.Dir(statePath), "worker-admission")
		if e = os.Mkdir(admissionPath, 0700); e != nil && !os.IsExist(e) {
			os.Exit(7)
		}
		journalPath := filepath.Join(statePath, "journal.jsonl")
		if _, e = os.Stat(journalPath); os.IsNotExist(e) {
			if e = os.WriteFile(journalPath, []byte("synthetic prepared worker journal\n"), 0600); e != nil {
				os.Exit(8)
			}
		}
		journalData, e := os.ReadFile(journalPath)
		if e != nil || string(journalData) != "synthetic prepared worker journal\n" {
			os.Exit(9)
		}
		journalInfo, e := os.Stat(journalPath)
		if e != nil {
			os.Exit(10)
		}
		stateInfo, e := os.Stat(statePath)
		if e != nil {
			os.Exit(11)
		}
		claimPath := filepath.Join(admissionPath, "admission.json")
		if _, e = os.Stat(claimPath); os.IsNotExist(e) {
			stateID := brokerFileIdentity(stateInfo)
			journalID := brokerFileIdentity(journalInfo)
			claim := map[string]any{"version": 1, "ownership": brokerDigest(worker), "state_device": stateID.Device, "state_inode": stateID.Inode, "journal_device": journalID.Device, "journal_inode": journalID.Inode}
			claimData, _ := json.Marshal(claim)
			if e = os.WriteFile(claimPath, append(claimData, '\n'), 0600); e != nil {
				os.Exit(12)
			}
		}
		claimData, e := os.ReadFile(claimPath)
		if e != nil {
			os.Exit(13)
		}
		claimInfo, e := os.Stat(claimPath)
		if e != nil {
			os.Exit(14)
		}
		admissionInfo, e := os.Lstat(admissionPath)
		if e != nil {
			os.Exit(16)
		}
		receipt := brokerPreparationReceipt{Version: 1, Status: "worker_journal_prepared", Phase: "paired-worker", ApprovalDigest: brokerDigest(worker), State: brokerFileIdentity(stateInfo), Journal: brokerFileIdentity(journalInfo), Claim: brokerFileIdentity(claimInfo), AdmissionDirectory: brokerFileIdentity(admissionInfo), JournalDigest: brokerBytesDigest(journalData), ClaimDigest: brokerBytesDigest(claimData)}
		if json.NewEncoder(os.Stdout).Encode(receipt) != nil {
			os.Exit(15)
		}
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-journal" {
		if len(os.Args) != 8 || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" || os.Args[6] != "--phase" {
			os.Exit(3)
		}
		for _, entry := range os.Environ() {
			if entry != "LANG=C" && entry != "LC_ALL=C" {
				os.Exit(4)
			}
		}
		data, e := io.ReadAll(io.LimitReader(os.Stdin, 1))
		if e != nil || len(data) != 0 {
			os.Exit(5)
		}
		switch os.Args[7] {
		case "before-ack":
			time.Sleep(time.Minute)
		case "inspect":
			fmt.Fprint(os.Stdout, strings.Repeat("synthetic-private-preparation-output", 1000))
		case "cleanup":
			fmt.Fprint(os.Stdout, `{"version":1,"VERSION":2}`)
		case "after-ack":
		default:
			fmt.Fprint(os.Stdout, `{"version":1,"status":"controller_journal_prepared","phase":"create"}`)
		}
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "--execute-approved-canary" {
		if len(os.Args) != 8 || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" || os.Args[6] != "--phase" {
			os.Exit(3)
		}
		for _, entry := range os.Environ() {
			if entry != "LANG=C" && entry != "LC_ALL=C" {
				os.Exit(4)
			}
		}
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 16385))
		if err != nil || !bytes.Contains(data, []byte("synthetic-private-installation-token")) || bytes.Contains(data, []byte("PRIVATE KEY")) {
			os.Exit(5)
		}
		switch os.Args[7] {
		case "inspect":
			fmt.Fprint(os.Stdout, strings.Repeat("synthetic-private-output", 1000))
		case "before-ack":
			time.Sleep(time.Minute)
		default:
			fmt.Fprintln(os.Stdout, "synthetic-private-output")
			fmt.Fprintln(os.Stderr, "synthetic-private-error")
		}
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--execute-approved-paired-terminal" {
		if len(os.Args) != 12 || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" || os.Args[6] != "--worker-approval" || os.Args[8] != "--worker-state-dir" || os.Args[10] != "--paired-binding" {
			os.Exit(3)
		}
		for _, entry := range os.Environ() {
			if entry != "LANG=C" && entry != "LC_ALL=C" {
				os.Exit(4)
			}
		}
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 16385))
		if err != nil || !bytes.Contains(data, []byte("synthetic-private-installation-token")) || bytes.Contains(data, []byte("PRIVATE KEY")) {
			os.Exit(5)
		}
		var payload map[string]json.RawMessage
		if json.Unmarshal(data, &payload) != nil {
			os.Exit(6)
		}
		if journalData, e := os.ReadFile(filepath.Join(os.Args[9], "journal.jsonl")); e == nil && string(journalData) != "synthetic prepared worker journal\n" {
			os.Exit(8)
		}
		var argvBinding brokerPairedBinding
		var payloadBinding brokerPairedBinding
		bindingData, ok := payload["paired_binding"]
		if !ok || decodeBrokerJSON(bindingData, &payloadBinding, true) != nil || !payloadBinding.valid() || decodeBrokerJSON([]byte(os.Args[11]), &argvBinding, true) != nil || payloadBinding != argvBinding {
			os.Exit(7)
		}
		if journal, e := os.OpenFile(filepath.Join(os.Args[5], "journal.jsonl"), os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW, 0); e == nil {
			_, _ = journal.WriteString("{\"paired_child\":true}\n")
			_ = journal.Sync()
			_ = journal.Close()
		}
		if argvBinding.ControllerApprovalSHA256 == "e"+strings.Repeat("a", 63) {
			fmt.Fprint(os.Stdout, strings.Repeat("synthetic-private-paired-overflow", 1000))
			os.Exit(0)
		}
		if argvBinding.ControllerApprovalSHA256 == "f"+strings.Repeat("a", 63) {
			time.Sleep(time.Minute)
		}
		fmt.Fprintln(os.Stdout, "synthetic-private-paired-output")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func testBrokerBinary(t *testing.T) *verifiedBrokerBinary {
	t.Helper()
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal("fixture executable")
	}
	path := filepath.Join(t.TempDir(), "fixture-controller")
	if os.WriteFile(path, data, 0500) != nil {
		t.Fatal("fixture executable")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal("fixture executable")
	}
	t.Cleanup(func() { f.Close() })
	digest := sha256.Sum256(data)
	return &verifiedBrokerBinary{path: path, file: f, digest: hex.EncodeToString(digest[:])}
}
func TestBrokerPipeUsesFixedArgsMinimalEnvAndDiscardsChildSecrets(t *testing.T) {
	t.Setenv("BROKER_TEST_SECRET", "synthetic-inherited-secret")
	binary := testBrokerBinary(t)
	root := t.TempDir()
	if err := invokeBrokerController(context.Background(), binary, root, filepath.Join(root, "approval.json"), root, "create", []byte(`{"installation_token":"synthetic-private-installation-token"}`)); err != nil {
		t.Fatal("private fixed child handoff failed")
	}
}

func TestBrokerPairedTerminalPipeUsesFixedArgsAndOneControllerInput(t *testing.T) {
	binary := testBrokerBinary(t)
	root := t.TempDir()
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"installation_token":"synthetic-private-installation-token"}`)
	binding := brokerPairedBinding{ControllerApprovalSHA256: strings.Repeat("a", 64), ControllerApprovalDevice: 1, ControllerApprovalInode: 2, ControllerStateDevice: 1, ControllerStateInode: 3, WorkerApprovalSHA256: strings.Repeat("b", 64), WorkerApprovalDevice: 1, WorkerApprovalInode: 4, WorkerStateDevice: 1, WorkerStateInode: 5}
	if err := invokeBrokerPairedTerminal(context.Background(), binary, root, filepath.Join(root, "approval.json"), root, filepath.Join(root, "worker.json"), workerState, binding, data); err != nil {
		t.Fatal("private fixed paired terminal handoff failed")
	}
}

func TestBrokerPairedTerminalRealDigestPrefixDoesNotTriggerFixtureSleep(t *testing.T) {
	binary := testBrokerBinary(t)
	root := t.TempDir()
	workerState := filepath.Join(root, "worker-state")
	if err := os.Mkdir(workerState, 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"installation_token":"synthetic-private-installation-token"}`)
	for _, digest := range []string{"e" + strings.Repeat("b", 63), "f" + strings.Repeat("c", 63)} {
		t.Run(digest[:1], func(t *testing.T) {
			started := time.Now()
			binding := brokerPairedBinding{ControllerApprovalSHA256: digest, ControllerApprovalDevice: 1, ControllerApprovalInode: 2, ControllerStateDevice: 1, ControllerStateInode: 3, WorkerApprovalSHA256: strings.Repeat("b", 64), WorkerApprovalDevice: 1, WorkerApprovalInode: 4, WorkerStateDevice: 1, WorkerStateInode: 5}
			if err := invokeBrokerPairedTerminal(context.Background(), binary, root, filepath.Join(root, "approval.json"), root, filepath.Join(root, "worker.json"), workerState, binding, data); err != nil {
				t.Fatalf("ordinary digest prefix %q refused: %v", digest[:1], err)
			}
			if elapsed := time.Since(started); elapsed > 5*time.Second {
				t.Fatalf("ordinary digest prefix %q held the child for %s", digest[:1], elapsed)
			}
		})
	}
}

func TestBrokerPairedChildBoundsTimeoutOverflowAndCancel(t *testing.T) {
	binary := testBrokerBinary(t)
	root := t.TempDir()
	data := []byte(`{"installation_token":"synthetic-private-installation-token"}`)
	base := brokerPairedBinding{ControllerApprovalDevice: 1, ControllerApprovalInode: 2, ControllerStateDevice: 1, ControllerStateInode: 3, WorkerApprovalDevice: 1, WorkerApprovalInode: 4, WorkerStateDevice: 1, WorkerStateInode: 5}
	for _, tc := range []struct {
		name   string
		prefix string
		ctx    func() (context.Context, context.CancelFunc)
	}{
		{name: "overflow", prefix: "e", ctx: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), time.Second)
		}},
		{name: "timeout", prefix: "f", ctx: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), 100*time.Millisecond)
		}},
		{name: "cancel", prefix: "a", ctx: func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, func() {}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := base
			binding.ControllerApprovalSHA256 = tc.prefix + strings.Repeat("a", 63)
			binding.WorkerApprovalSHA256 = strings.Repeat("b", 64)
			ctx, cancel := tc.ctx()
			defer cancel()
			if err := invokeBrokerPairedTerminal(ctx, binary, root, filepath.Join(root, "approval.json"), root, filepath.Join(root, "worker.json"), filepath.Join(root, "worker-state"), binding, data); err == nil {
				t.Fatal("paired child bound failure accepted")
			}
		})
	}
}

func TestBrokerPairedChildDeadlineIsBoundedAndLeavesCadenceMargin(t *testing.T) {
	now := time.Now()
	deadline, err := pairedChildDeadline(context.Background(), now)
	if err != nil || !deadline.Equal(now.Add(pairedTerminalMaximumChildBudget)) {
		t.Fatalf("background child deadline=%v err=%v", deadline.Sub(now), err)
	}
	parentDeadline := now.Add(2 * time.Minute)
	parent, cancel := context.WithDeadline(context.Background(), parentDeadline)
	defer cancel()
	deadline, err = pairedChildDeadline(parent, now)
	if err != nil || !deadline.Equal(parentDeadline) {
		t.Fatalf("parent authority deadline=%v err=%v", deadline.Sub(now), err)
	}
	short, cancel := context.WithDeadline(context.Background(), now.Add(pairedTerminalMinimumChildBudget))
	defer cancel()
	if _, err := pairedChildDeadline(short, now); err == nil {
		t.Fatal("child deadline accepted without a complete cadence margin")
	}
}

func TestBrokerChildBoundsAndReplacedBinaryRefuse(t *testing.T) {
	binary := testBrokerBinary(t)
	root := t.TempDir()
	data := []byte(`{"installation_token":"synthetic-private-installation-token"}`)
	for _, phase := range []string{"inspect", "before-ack"} {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		err := invokeBrokerController(ctx, binary, root, filepath.Join(root, "approval.json"), root, phase, data)
		cancel()
		if err == nil || strings.Contains(err.Error(), "synthetic-private") {
			t.Fatal("child bound missing or leaked output")
		}
	}
	if os.Chmod(binary.path, 0700) != nil || os.WriteFile(binary.path, []byte("replaced"), 0500) != nil {
		t.Fatal("fixture replacement")
	}
	if err := invokeBrokerController(context.Background(), binary, root, "approval.json", root, "create", data); err == nil {
		t.Fatal("changed executable ran")
	}
}
func TestBrokerBuildMustMatchReviewedController(t *testing.T) {
	sha := strings.Repeat("a", 40)
	good := debug.BuildInfo{GoVersion: "go1.26.8", Path: "github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/cmd/g01-live", Deps: []*debug.Module{{Path: "github.com/actions/scaleset", Version: "v0.4.0"}}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "false"}, {Key: "GOOS", Value: runtime.GOOS}, {Key: "GOARCH", Value: runtime.GOARCH}, {Key: "CGO_ENABLED", Value: "1"}, {Key: "-tags", Value: "g01_live"}}}
	if !validBrokerBuild(&good, sha) {
		t.Fatal("reviewed controller metadata rejected")
	}
	for _, kind := range []string{"revision", "dirty", "SDK", "program"} {
		bad := good
		bad.Settings = append([]debug.BuildSetting(nil), good.Settings...)
		switch kind {
		case "revision":
			bad.Settings[0].Value = strings.Repeat("b", 40)
		case "dirty":
			bad.Settings[1].Value = "true"
		case "SDK":
			bad.Deps = []*debug.Module{{Path: "github.com/actions/scaleset", Version: "v0.4.1"}}
		case "program":
			bad.Path = "arbitrary/program"
		}
		if validBrokerBuild(&bad, sha) {
			t.Errorf("accepted %s", kind)
		}
	}
}

func TestBrokerBuildRejectsFixtureCapability(t *testing.T) {
	sha := strings.Repeat("a", 40)
	fixture := debug.BuildInfo{GoVersion: "go1.26.8", Path: "github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/cmd/g01-live", Deps: []*debug.Module{{Path: "github.com/actions/scaleset", Version: "v0.4.0"}}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "false"}, {Key: "GOOS", Value: runtime.GOOS}, {Key: "GOARCH", Value: runtime.GOARCH}, {Key: "CGO_ENABLED", Value: "1"}, {Key: "-tags", Value: "g01_live,g01_pair_fixture"}}}
	if validBrokerBuild(&fixture, sha) {
		t.Fatal("fixture-enabled controller build accepted by production broker gate")
	}
}

func TestBrokerBuildUnsupportedNativeAdmissionRefuses(t *testing.T) {
	sha := strings.Repeat("a", 40)
	base := debug.BuildInfo{GoVersion: "go1.26.8", Path: "github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/cmd/g01-live", Deps: []*debug.Module{{Path: "github.com/actions/scaleset", Version: "v0.4.0"}}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "false"}, {Key: "GOOS", Value: runtime.GOOS}, {Key: "GOARCH", Value: runtime.GOARCH}, {Key: "CGO_ENABLED", Value: "1"}, {Key: "-tags", Value: "g01_live"}}}
	for _, tc := range []struct{ key, value string }{{"CGO_ENABLED", "0"}, {"CGO_ENABLED", ""}, {"-tags", "g01_live,osusergo"}, {"-tags", "g01_live osusergo"}, {"GOOS", "other"}, {"GOARCH", "other"}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			bad := base
			bad.Settings = append([]debug.BuildSetting(nil), base.Settings...)
			for i := range bad.Settings {
				if bad.Settings[i].Key == tc.key {
					bad.Settings[i].Value = tc.value
				}
			}
			if validBrokerBuild(&bad, sha) {
				t.Fatal("unsupported executable accepted before token mint")
			}
		})
	}
	if !validBrokerBuild(&base, sha) {
		t.Fatal("exact production tag set refused")
	}
	for _, tags := range []string{"g01_live,notosusergo", "g01_live osusergo_extra"} {
		good := base
		good.Settings = append([]debug.BuildSetting(nil), base.Settings...)
		good.Settings[len(good.Settings)-1].Value = tags
		if validBrokerBuild(&good, sha) {
			t.Fatal("unreviewed production tag set accepted")
		}
	}
}
