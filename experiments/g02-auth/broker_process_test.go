package enrollment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// This child is only the Go test executable, never a controller or worker. The
// production entry point must separately verify G01 build metadata before it
// can construct verifiedBrokerBinary; these pipe tests isolate that handoff.
func TestMain(m *testing.M) {
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
	for _, tags := range []string{"g01_live", "g01_live,notosusergo", "g01_live osusergo_extra"} {
		good := base
		good.Settings = append([]debug.BuildSetting(nil), base.Settings...)
		good.Settings[len(good.Settings)-1].Value = tags
		if !validBrokerBuild(&good, sha) {
			t.Fatal("exact supported tags refused")
		}
	}
}
