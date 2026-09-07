package tooling

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func toolingFile(t *testing.T, root, name, data string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func toolingRun(t *testing.T, root string, extra []string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.26.8", "GOFLAGS=", "GOWORK=off")
	cmd.Env = append(cmd.Env, extra...)
	data, err := cmd.CombinedOutput()
	return string(data), err
}

func toolingGoWrapper(t *testing.T, root string) (string, string, string) {
	t.Helper()
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "go-wrapper.log")
	wrapperPath := filepath.Join(root, "logging-go")
	toolingFile(t, root, "logging-go", `#!/bin/sh
set -eu
printf '%s\t%s\n' "${GOTOOLCHAIN:-}" "$*" >> "$TOOLING_GO_LOG"
exec "$TOOLING_REAL_GO" "$@"
`, 0700)
	return wrapperPath, logPath, realGo
}

func toolingFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"Makefile", "go.mod", "LICENSE", "docs/DEPENDENCIES.md", "scripts/check-toolchain.sh", "scripts/check-licenses.sh", "scripts/check-offline-experiments.sh", "scripts/gofmt.sh", "scripts/fuzz-smoke.sh"} {
		data, err := os.ReadFile(filepath.Join("..", name))
		if err != nil {
			t.Fatal(err)
		}
		toolingFile(t, root, name, string(data), 0600)
	}
	toolingFile(t, root, "cmd/gh-runnerd/main.go", "package main\n\nfunc main() {}\n", 0600)
	for _, module := range []string{"g01-scaleset", "g02-auth"} {
		base := "experiments/" + module
		toolingFile(t, root, base+"/go.mod", "module example.test/"+module+"\n\ngo 1.26.8\n", 0600)
		toolingFile(t, root, base+"/fixture.go", "package fixture\n", 0600)
	}
	toolingFile(t, root, "experiments/g01-scaleset/livecanary/fixture.go", "package livecanary\n\nfunc fixtureValue() string { return \"livecanary-fixture\" }\n", 0600)
	toolingFile(t, root, "experiments/g01-scaleset/livecanary/fixture_test.go", `package livecanary

import "testing"

func TestLivecanaryFixturePositiveControl(t *testing.T) {
	if got := fixtureValue(); got != "livecanary-fixture" {
		t.Fatalf("fixture value = %q", got)
	}
}
`, 0600)
	for _, command := range []string{"g01-live", "g01-worker"} {
		toolingFile(t, root, "experiments/g01-scaleset/cmd/"+command+"/main.go", "package main\n\nfunc main() {}\n", 0600)
	}
	// This fixture tests check orchestration, not vulnerability database access.
	toolingFile(t, root, "scripts/govulncheck.sh", "#!/bin/sh\nexit 0\n", 0600)
	if out, err := toolingRun(t, root, nil, "git", "init", "--quiet"); err != nil {
		t.Fatalf("fixture Git initialization: %s", out)
	}
	return root
}

func TestToolingGOFLAGSUsesGoEnvironment(t *testing.T) {
	root := toolingFixture(t)
	for _, target := range []string{"build", "vet", "test", "test-race"} {
		if out, err := toolingRun(t, root, nil, "make", "GOFLAGS=-mod=readonly", target); err != nil {
			t.Errorf("conventional GOFLAGS failed for %s: %s", target, out)
		}
	}
}

func TestToolingPinsNewerSystemGo(t *testing.T) {
	root := toolingFixture(t)
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	// Controlled newer-installation fixture: only an explicit exact selection
	// delegates to the actual installed Go; auto reports a newer bundled version.
	toolingFile(t, root, "newer-go", "#!/bin/sh\nif [ \"${GOTOOLCHAIN:-auto}\" = go1.26.8 ]; then exec \"$TOOLING_REAL_GO\" \"$@\"; fi\nif [ \"$1\" = version ]; then echo 'go version go1.27.1 linux/amd64'; else exit 88; fi\n", 0700)
	for _, args := range [][]string{{"make", "GO=" + filepath.Join(root, "newer-go"), "toolchain"}, {"bash", "scripts/check-toolchain.sh"}} {
		env := []string{"GOTOOLCHAIN=auto", "GO=" + filepath.Join(root, "newer-go"), "TOOLING_REAL_GO=" + realGo}
		if out, err := toolingRun(t, root, env, args...); err != nil || !strings.Contains(out, "toolchain: go1.26.8") {
			t.Errorf("newer system selection did not use pin: %s", out)
		}
	}
	// The standalone checker setting its own environment must not mask a Make
	// regression: a separate build recipe must also receive the exact selection.
	env := []string{"GOTOOLCHAIN=auto", "TOOLING_REAL_GO=" + realGo}
	if out, err := toolingRun(t, root, env, "make", "GO="+filepath.Join(root, "newer-go"), "build"); err != nil {
		t.Errorf("build did not receive the exact toolchain selection: %s", out)
	}
}

func TestToolingCheckRequiresExecutableLink(t *testing.T) {
	root := toolingFixture(t)
	if out, err := toolingRun(t, root, nil, "make", "check"); err != nil {
		t.Fatalf("positive check fixture: %s", out)
	}
	toolingFile(t, root, "cmd/gh-runnerd/main.go", "package main\n\nfunc missingLinkSymbol()\n\nfunc main() {\n\tmissingLinkSymbol()\n}\n", 0600)
	toolingFile(t, root, "cmd/gh-runnerd/fixture.s", "// No definition of missingLinkSymbol.\n", 0600)
	if out, err := toolingRun(t, root, nil, "make", "check"); err == nil || !strings.Contains(out, "missingLinkSymbol") {
		t.Fatalf("check accepted an un-linkable command: %s", out)
	}
	workflow, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil || !strings.Contains(string(workflow), "run: make build") {
		t.Fatal("hosted workflow omits executable build")
	}
}

func TestToolingEstablishedModulesAreRequired(t *testing.T) {
	for _, module := range []string{"g01-scaleset", "g02-auth"} {
		root := toolingFixture(t)
		if err := os.Rename(filepath.Join(root, "experiments", module, "go.mod"), filepath.Join(root, "experiments", module, "absent.mod")); err != nil {
			t.Fatal(err)
		}
		if out, err := toolingRun(t, root, nil, "bash", "scripts/check-offline-experiments.sh"); err == nil {
			t.Errorf("missing established %s passed: %s", module, out)
		}
	}
}

func TestToolingTaggedCLIRegressionRuns(t *testing.T) {
	root := toolingFixture(t)
	for _, command := range []string{"g01-live", "g01-worker"} {
		name := "experiments/g01-scaleset/cmd/" + command + "/regression_test.go"
		toolingFile(t, root, name, "//go:build g01_live && g01_worker\n\npackage main\n\nimport \"testing\"\n\nfunc TestTaggedFailure(t *testing.T) { t.Fatal(\"tagged-CLI-regression\") }\n", 0600)
		if out, err := toolingRun(t, root, nil, "bash", "scripts/check-offline-experiments.sh"); err == nil || !strings.Contains(out, "tagged-CLI-regression") {
			t.Fatalf("tagged %s failure was skipped: %s", command, out)
		}
		if err := os.Remove(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestToolingTaggedPairFixturePartitionsRun(t *testing.T) {
	root := toolingFixture(t)
	const collectionSentinel = "tagged-pair-fixture-collection-regression"
	const terminalSentinel = "tagged-pair-fixture-terminal-regression"
	collectionPath := "experiments/g01-scaleset/livecanary/pair_fixture_collection_regression_test.go"
	terminalPath := "experiments/g01-scaleset/livecanary/pair_fixture_terminal_regression_test.go"
	toolingFile(t, root, collectionPath, `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import "testing"

func TestTaggedPairFixtureCollectionFailure(t *testing.T) {
	t.Fatal("tagged-pair-fixture-collection-regression")
}
`, 0600)
	toolingFile(t, root, terminalPath, `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import "testing"

func TestPairedTerminalFixtureFailure(t *testing.T) {
	t.Fatal("tagged-pair-fixture-terminal-regression")
}
`, 0600)
	wrapper, logPath, realGo := toolingGoWrapper(t, root)
	env := []string{"GO=" + wrapper, "TOOLING_REAL_GO=" + realGo, "TOOLING_GO_LOG=" + logPath}
	if out, err := toolingRun(t, root, env, "make", "check"); err == nil || !strings.Contains(out, collectionSentinel) {
		t.Fatalf("tagged collection failure was skipped: %s", out)
	}
	if err := os.Remove(filepath.Join(root, collectionPath)); err != nil {
		t.Fatal(err)
	}
	if out, err := toolingRun(t, root, env, "make", "check"); err == nil || !strings.Contains(out, terminalSentinel) {
		t.Fatalf("tagged terminal failure was skipped: %s", out)
	}
	if err := os.Remove(filepath.Join(root, terminalPath)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	env = append(env, "GOTOOLCHAIN=auto")
	if out, err := toolingRun(t, root, env, "bash", "scripts/check-offline-experiments.sh"); err != nil {
		t.Fatalf("pair fixture positive control: %s", out)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for _, invocation := range []string{
		"go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -skip ^TestPairedTerminal ./livecanary",
		"go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPairedTerminal ./livecanary",
		"go1.26.8\tvet -tags=g01_pair_fixture ./livecanary",
	} {
		count := 0
		for _, line := range lines {
			if line == invocation {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("offline gate logged %q %d times; wrapper log:\n%s", invocation, count, log)
		}
	}
}

func TestToolingLicenseIdentityAndStaleRows(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "go.mod", "module example.test/audit\n\ngo 1.26.8\n\nrequire example.test/dependency v1.2.0\n\nreplace example.test/dependency => ./replacement\n", 0600)
	toolingFile(t, root, "replacement/go.mod", "module example.test/dependency\n\ngo 1.26.8\n", 0600)
	toolingFile(t, root, "replacement/LICENSE", "Synthetic license fixture\n", 0600)
	base := "## Runtime modules\n\n| Module | Version | Replacement | License | Source |\n| --- | --- | --- | --- | --- |\n| `example.test/audit` | `local` | `none` | MIT | root |\n"
	good := "| `example.test/dependency` | `v1.2.0` | `./replacement` | MIT | local fixture |\n"
	for _, tc := range []struct {
		name, row string
		valid     bool
	}{{"matching", good, true}, {"old version", strings.ReplaceAll(good, "v1.2.0", "v1.1.0"), false}, {"unrecorded replacement", strings.ReplaceAll(good, "./replacement", "none"), false}, {"stale module", good + "| `example.test/removed` | `v1.0.0` | `none` | MIT | stale |\n", false}, {"duplicate", good + good, false}} {
		toolingFile(t, root, "docs/DEPENDENCIES.md", base+tc.row, 0600)
		out, err := toolingRun(t, root, nil, "bash", "scripts/check-licenses.sh")
		if (err == nil) != tc.valid {
			t.Errorf("license %s acceptance=%t: %s", tc.name, err == nil, out)
		}
	}
}
