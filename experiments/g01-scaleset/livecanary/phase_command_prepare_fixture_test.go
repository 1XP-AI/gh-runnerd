//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// One bounded preparation test compiles current-source, race-instrumented TEST
// dispatchers. Both have fixture capabilities refused by production admission.
func TestExperimentalPrepareCommandHelpers(t *testing.T) {
	dir := os.Getenv("G01_PHASE_HELPER_DIR")
	if dir == "" {
		t.Skip("separate owned offline preparation partition")
	}
	if !privateStateDirectory(dir) {
		t.Fatal("private helper output directory required")
	}
	goTool := os.Getenv("G01_PHASE_GO")
	if goTool == "" {
		goTool = "go"
	}
	goTool, e := exec.LookPath(goTool)
	if e != nil {
		t.Fatal("Go helper tool unavailable")
	}
	// Hash and invoke the same tool even after changing the child working directory.
	goTool, e = filepath.Abs(goTool)
	if e != nil {
		t.Fatal("Go helper tool unavailable")
	}
	source := phaseHelperSourceFingerprint(t)
	tool := phaseHelperFileDigest(t, goTool)
	manifestPath := filepath.Join(dir, "provenance.json")
	if entries, e := os.ReadDir(dir); e != nil {
		t.Fatal("helper directory unavailable")
	} else if len(entries) > 0 {
		var prior phaseHelperProvenance
		raw, e := os.ReadFile(manifestPath)
		if e != nil || DecodeStrict(raw, &prior) != nil || prior.SourceSHA256 != source || prior.GoToolSHA256 != tool || prior.ControllerSHA256 != phaseHelperFileDigest(t, filepath.Join(dir, "controller.test")) || prior.BrokerSHA256 != phaseHelperFileDigest(t, filepath.Join(dir, "broker.test")) {
			t.Fatal("helper files lack exact current-source provenance")
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	for _, step := range []struct{ work, tags, pkg, name string }{{"..", "g01_live,g01_pair_fixture", "./cmd/g01-live", "controller.test"}, {"../../g02-auth", "g01_pair_fixture", "./cmd/g01-broker", "broker.test"}} {
		cmd := exec.CommandContext(ctx, goTool, "test", "-race", "-p=2", "-c", "-tags="+step.tags, "-o", filepath.Join(dir, step.name), step.pkg)
		cmd.Dir = step.work
		cmd.Env = append(os.Environ(), "GOMAXPROCS=2")
		if output, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("current-source helper build %s failed: %v %s", step.name, e, output)
		}
	}
	if phaseHelperSourceFingerprint(t) != source {
		t.Fatal("source changed during helper compilation")
	}
	manifest := phaseHelperProvenance{source, tool, phaseHelperFileDigest(t, filepath.Join(dir, "controller.test")), phaseHelperFileDigest(t, filepath.Join(dir, "broker.test"))}
	raw, _ := json.Marshal(manifest)
	if os.WriteFile(manifestPath, append(raw, '\n'), 0600) != nil {
		t.Fatal("helper provenance write failed")
	}
}

type phaseHelperProvenance struct {
	SourceSHA256     string `json:"source_sha256"`
	GoToolSHA256     string `json:"go_tool_sha256"`
	ControllerSHA256 string `json:"controller_sha256"`
	BrokerSHA256     string `json:"broker_sha256"`
}

func phaseHelperFileDigest(t *testing.T, path string) string {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("helper input/artifact unreadable")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func phaseHelperSourceFingerprint(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	root, e := cmd.Output()
	if e != nil {
		t.Fatal("helper source root unavailable")
	}
	cmd = exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = strings.TrimSpace(string(root))
	raw, e := cmd.Output()
	if e != nil {
		t.Fatal("helper source inventory unavailable")
	}
	paths := bytes.Split(bytes.TrimSuffix(raw, []byte{0}), []byte{0})
	sort.Slice(paths, func(i, j int) bool { return bytes.Compare(paths[i], paths[j]) < 0 })
	h := sha256.New()
	for _, p := range paths {
		path := string(p)
		name := filepath.Base(path)
		if !strings.HasSuffix(path, ".go") && name != "go.mod" && name != "go.sum" {
			continue
		}
		data, e := os.ReadFile(filepath.Join(cmd.Dir, path))
		if e != nil {
			t.Fatal("helper source input unavailable")
		}
		h.Write(p)
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
