//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	if _, e := os.Stat(filepath.Join(dir, "controller.test")); e == nil {
		if _, e := os.Stat(filepath.Join(dir, "broker.test")); e == nil {
			return
		}
	}
	if !privateStateDirectory(dir) {
		t.Fatal("private helper output directory required")
	}
	goTool := os.Getenv("G01_PHASE_GO")
	if goTool == "" {
		var e error
		goTool, e = exec.LookPath("go")
		if e != nil {
			t.Fatal("Go helper tool unavailable")
		}
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
}
