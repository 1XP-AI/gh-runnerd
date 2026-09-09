package tooling

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	const pairedCollectionSentinel = "tagged-pair-fixture-paired-collection-regression"
	const listenerSentinel = "tagged-pair-fixture-listener-regression"
	const exampleSentinel = "tagged-pair-fixture-example-regression"
	const fuzzSentinel = "tagged-pair-fixture-fuzz-seed-regression"
	const terminalSentinel = "tagged-pair-fixture-terminal-regression"
	const storageSentinel = "tagged-pair-fixture-storage-regression"
	pairedCollectionPath := "experiments/g01-scaleset/livecanary/pair_fixture_paired_collection_regression_test.go"
	listenerPath := "experiments/g01-scaleset/livecanary/pair_fixture_listener_regression_test.go"
	examplePath := "experiments/g01-scaleset/livecanary/pair_fixture_example_regression_test.go"
	fuzzPath := "experiments/g01-scaleset/livecanary/pair_fixture_fuzz_regression_test.go"
	terminalPath := "experiments/g01-scaleset/livecanary/pair_fixture_terminal_regression_test.go"
	storagePath := "experiments/g01-scaleset/livecanary/pair_fixture_storage_regression_test.go"
	remainingCollectionInvocation := "go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -skip ^TestPaired ./livecanary"
	taggedPositiveSource := func(testName, marker string) string {
		return `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"os"
	"testing"
)

func ` + testName + `(t *testing.T) {
	path := os.Getenv("TOOLING_SENTINEL_LOG")
	if path == "" {
		t.Fatal("TOOLING_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("` + marker + `\n"); err != nil {
		t.Fatal(err)
	}
}
`
	}
	pairedCollectionPositiveSource := taggedPositiveSource("TestPairedCollectionFixturePass", "paired-collection-pass")
	listenerPositiveSource := taggedPositiveSource("TestListenerFixturePass", "listener-pass")
	taggedExampleSource := func(exampleName, marker, expected string) string {
		return `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"fmt"
	"os"
)

func ` + exampleName + `() {
	path := os.Getenv("TOOLING_SENTINEL_LOG")
	if path == "" {
		panic("TOOLING_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if _, err := file.WriteString("` + marker + `\n"); err != nil {
		panic(err)
	}
	fmt.Println("` + marker + `")
	// Output: ` + expected + `
}
`
	}
	taggedFuzzSource := func(fuzzName, marker, failure string) string {
		failureLine := ""
		if failure != "" {
			failureLine = `
	t.Fatal("` + failure + `")`
		}
		return `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"os"
	"testing"
)

func ` + fuzzName + `(f *testing.F) {
	f.Add("fixture-seed")
	f.Fuzz(func(t *testing.T, _ string) {
		path := os.Getenv("TOOLING_SENTINEL_LOG")
		if path == "" {
			t.Fatal("TOOLING_SENTINEL_LOG is not set")
		}
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := file.WriteString("` + marker + `\n"); err != nil {
			t.Fatal(err)
		}` + failureLine + `
	})
}
`
	}
	examplePositiveSource := taggedExampleSource("Example_fixturePass", "example-pass", "example-pass")
	exampleSource := taggedExampleSource("Example_fixtureFailure", exampleSentinel, "unexpected-example-output")
	fuzzPositiveSource := taggedFuzzSource("FuzzRemainingFixturePass", "fuzz-pass", "")
	fuzzSource := taggedFuzzSource("FuzzRemainingFixtureFailure", fuzzSentinel, fuzzSentinel)
	terminalPositiveSource := taggedPositiveSource("TestPairedTerminalFixturePass", "terminal-pass")
	storagePositiveSource := taggedPositiveSource("TestPairedTerminalFixtureStorageFailure", "storage-pass")
	pairedCollectionSource := `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import "testing"

func TestPairedCollectionFixtureFailure(t *testing.T) {
	t.Fatal("tagged-pair-fixture-paired-collection-regression")
}
`
	listenerSource := `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import "testing"

func TestListenerFixtureFailure(t *testing.T) {
	t.Fatal("tagged-pair-fixture-listener-regression")
}
`
	terminalSource := `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import "testing"

func TestPairedTerminalFixtureFailure(t *testing.T) {
	t.Fatal("tagged-pair-fixture-terminal-regression")
}
`
	storageSource := `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import "testing"

func TestPairedTerminalFixtureStorageFailure(t *testing.T) {
	t.Fatal("tagged-pair-fixture-storage-regression")
}
`
	wrapper, logPath, realGo := toolingGoWrapper(t, root)
	sentinelLogPath := filepath.Join(root, "tagged-sentinel.log")
	env := []string{"GO=" + wrapper, "TOOLING_REAL_GO=" + realGo, "TOOLING_GO_LOG=" + logPath, "TOOLING_SENTINEL_LOG=" + sentinelLogPath}
	// Each generated witness must fail through its own reviewed partition. The
	// terminal witnesses retain their established behavior while the log
	// assertions below prove the four-way partition contract.
	partitions := []struct {
		path, sentinel, name, source, positiveSource, invocation string
	}{
		{
			pairedCollectionPath,
			pairedCollectionSentinel,
			"paired-collection",
			pairedCollectionSource,
			pairedCollectionPositiveSource,
			"go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPaired -skip ^TestPairedTerminal ./livecanary",
		},
		{
			listenerPath,
			listenerSentinel,
			"listener",
			listenerSource,
			listenerPositiveSource,
			remainingCollectionInvocation,
		},
		{
			examplePath,
			exampleSentinel,
			"example",
			exampleSource,
			examplePositiveSource,
			remainingCollectionInvocation,
		},
		{
			fuzzPath,
			fuzzSentinel,
			"fuzz",
			fuzzSource,
			fuzzPositiveSource,
			remainingCollectionInvocation,
		},
		{
			terminalPath,
			terminalSentinel,
			"terminal",
			terminalSource,
			terminalPositiveSource,
			"go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPairedTerminal -skip ^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$ ./livecanary",
		},
		{
			storagePath,
			storageSentinel,
			"storage",
			storageSource,
			storagePositiveSource,
			"go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$ ./livecanary",
		},
	}
	for _, tc := range partitions {
		toolingFile(t, root, tc.path, tc.source, 0600)
		if err := os.WriteFile(logPath, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := toolingRun(t, root, env, "bash", "scripts/check-offline-experiments.sh"); err == nil || !strings.Contains(out, tc.sentinel) {
			t.Errorf("tagged %s failure was skipped: %s", tc.name, out)
		}
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		count := 0
		for _, line := range lines {
			if line == tc.invocation {
				count++
			}
		}
		if count != 1 {
			t.Errorf("tagged %s failure used invocation %q %d times; wrapper log:\n%s", tc.name, tc.invocation, count, data)
		}
		if err := os.Remove(filepath.Join(root, tc.path)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range partitions {
		toolingFile(t, root, tc.path, tc.positiveSource, 0600)
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
	sentinelData, err := os.ReadFile(sentinelLogPath)
	if err != nil {
		t.Fatal(err)
	}
	sentinelLines := strings.Split(strings.TrimSpace(string(sentinelData)), "\n")
	for _, marker := range []string{"paired-collection-pass", "listener-pass", "example-pass", "fuzz-pass", "terminal-pass", "storage-pass"} {
		count := 0
		for _, line := range sentinelLines {
			if line == marker {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("positive sentinel %q ran %d times; sentinel log:\n%s", marker, count, sentinelData)
		}
	}
	log := string(data)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for _, invocation := range []string{
		"go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPaired -skip ^TestPairedTerminal ./livecanary",
		remainingCollectionInvocation,
		"go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPairedTerminal -skip ^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$ ./livecanary",
		"go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$ ./livecanary",
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
	legacyCollectionInvocation := "go1.26.8\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -skip ^TestPairedTerminal ./livecanary"
	for _, line := range lines {
		if line == legacyCollectionInvocation {
			t.Fatalf("offline gate retained the unsplit collection invocation: %s", line)
		}
	}
}

func TestToolingDefaultG01PartitionsRun(t *testing.T) {
	root := toolingFixture(t)
	const heavyName = "TestBaselineStatisticsPresenceAndEligibility"
	const heavySentinel = "default-g01-heavy-regression"
	const remainderSentinel = "default-g01-remainder-regression"
	const otherPackageSentinel = "default-g01-other-package-regression"
	const sameNameOtherPackageSentinel = "default-g01-same-name-other-package-regression"
	const exampleSentinel = "default-g01-example-output-regression"
	const fuzzSentinel = "default-g01-fuzz-seed-regression"
	livecanaryBase := "experiments/g01-scaleset/livecanary"
	otherPackageBase := "experiments/g01-scaleset/otherfixture"
	defaultTestSource := func(pkg, testName, marker, failure string) string {
		failureLine := ""
		if failure != "" {
			failureLine = "\n\tt.Fatal(\"" + failure + "\")"
		}
		return `//go:build !g01_pair_fixture && !g01_live && !g01_worker

package ` + pkg + `

import (
	"os"
	"testing"
)

func ` + testName + `(t *testing.T) {
	path := os.Getenv("TOOLING_SENTINEL_LOG")
	if path == "" {
		t.Fatal("TOOLING_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("` + marker + `\n"); err != nil {
		t.Fatal(err)
	}` + failureLine + `
}
`
	}
	defaultExampleSource := func(marker, expected string) string {
		return `//go:build !g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"fmt"
	"os"
)

func Example_defaultG01Fixture() {
	path := os.Getenv("TOOLING_SENTINEL_LOG")
	if path == "" {
		panic("TOOLING_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if _, err := file.WriteString("` + marker + `\n"); err != nil {
		panic(err)
	}
	fmt.Println("` + marker + `")
	// Output: ` + expected + `
}
`
	}
	defaultFuzzSource := func(marker, failure string) string {
		failureLine := ""
		if failure != "" {
			failureLine = "\n\t\tt.Fatal(\"" + failure + "\")"
		}
		return `//go:build !g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"os"
	"testing"
)

func FuzzDefaultG01Fixture(f *testing.F) {
	f.Add("fixture-seed")
	f.Fuzz(func(t *testing.T, _ string) {
		path := os.Getenv("TOOLING_SENTINEL_LOG")
		if path == "" {
			t.Fatal("TOOLING_SENTINEL_LOG is not set")
		}
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := file.WriteString("` + marker + `\n"); err != nil {
			t.Fatal(err)
		}` + failureLine + `
	})
}
`
	}
	type fixture struct {
		name, path, marker, positiveSource, failureSource string
	}
	fixtures := []fixture{
		{
			name:           "heavy",
			path:           livecanaryBase + "/default_heavy_regression_test.go",
			marker:         heavySentinel,
			positiveSource: defaultTestSource("livecanary", heavyName, heavySentinel, ""),
			failureSource:  defaultTestSource("livecanary", heavyName, heavySentinel, heavySentinel),
		},
		{
			name:           "remainder",
			path:           livecanaryBase + "/default_remainder_regression_test.go",
			marker:         remainderSentinel,
			positiveSource: defaultTestSource("livecanary", "TestDefaultG01RemainderFixture", remainderSentinel, ""),
			failureSource:  defaultTestSource("livecanary", "TestDefaultG01RemainderFixture", remainderSentinel, remainderSentinel),
		},
		{
			name:           "other package",
			path:           otherPackageBase + "/default_other_package_regression_test.go",
			marker:         otherPackageSentinel,
			positiveSource: defaultTestSource("otherfixture", "TestDefaultG01OtherPackageFixture", otherPackageSentinel, ""),
			failureSource:  defaultTestSource("otherfixture", "TestDefaultG01OtherPackageFixture", otherPackageSentinel, otherPackageSentinel),
		},
		{
			name:           "same-name other package",
			path:           otherPackageBase + "/default_same_name_regression_test.go",
			marker:         sameNameOtherPackageSentinel,
			positiveSource: defaultTestSource("otherfixture", heavyName, sameNameOtherPackageSentinel, ""),
			failureSource:  defaultTestSource("otherfixture", heavyName, sameNameOtherPackageSentinel, sameNameOtherPackageSentinel),
		},
		{
			name:           "Example Output",
			path:           livecanaryBase + "/default_example_regression_test.go",
			marker:         exampleSentinel,
			positiveSource: defaultExampleSource(exampleSentinel, exampleSentinel),
			failureSource:  defaultExampleSource(exampleSentinel, "unexpected-default-g01-example-output"),
		},
		{
			name:           "Fuzz seed",
			path:           livecanaryBase + "/default_fuzz_regression_test.go",
			marker:         fuzzSentinel,
			positiveSource: defaultFuzzSource(fuzzSentinel, ""),
			failureSource:  defaultFuzzSource(fuzzSentinel, fuzzSentinel),
		},
	}
	toolingFile(t, root, otherPackageBase+"/fixture.go", "package otherfixture\n", 0600)
	for _, tc := range fixtures {
		toolingFile(t, root, tc.path, tc.positiveSource, 0600)
	}
	wrapper, logPath, realGo := toolingGoWrapper(t, root)
	sentinelLogPath := filepath.Join(root, "default-sentinel.log")
	env := []string{
		"GO=" + wrapper,
		"TOOLING_REAL_GO=" + realGo,
		"TOOLING_GO_LOG=" + logPath,
		"TOOLING_SENTINEL_LOG=" + sentinelLogPath,
	}
	if out, err := toolingRun(t, root, env, "bash", "scripts/check-offline-experiments.sh"); err != nil {
		t.Fatalf("default G01 positive control: %s", out)
	}
	sentinelData, err := os.ReadFile(sentinelLogPath)
	if err != nil {
		t.Fatal(err)
	}
	sentinelLines := strings.Split(strings.TrimSpace(string(sentinelData)), "\n")
	for _, tc := range fixtures {
		count := 0
		for _, line := range sentinelLines {
			if line == tc.marker {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("positive %s sentinel %q ran %d times; sentinel log:\n%s", tc.name, tc.marker, count, sentinelData)
		}
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logData)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for _, invocation := range []string{
		"go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestBaselineStatisticsPresenceAndEligibility$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -skip ^TestBaselineStatisticsPresenceAndEligibility$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerPrepareReviewedG01LiveBinary$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestPaired -skip ^TestPairedBroker(PrepareReviewedG01LiveBinary|RealCadenceChildExceedsThirtySeconds)$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -skip ^TestPaired ./...",
	} {
		count := 0
		for _, line := range lines {
			if line == invocation {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("default G01 partition invocation %q ran %d times; wrapper log:\n%s", invocation, count, log)
		}
	}
	legacyInvocation := "go1.26.8\ttest -race -count=1 -timeout=45s ./..."
	legacyCount := 0
	for _, line := range lines {
		if line == legacyInvocation {
			legacyCount++
		}
	}
	if legacyCount != 0 {
		t.Fatalf("offline gate retained %d unsplit G02 invocations; wrapper log:\n%s", legacyCount, log)
	}
	for _, invocation := range []string{
		"go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestPaired -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=120s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
	} {
		for _, line := range lines {
			if line == invocation {
				t.Fatalf("offline gate retained forbidden G02 invocation %q; wrapper log:\n%s", invocation, log)
			}
		}
	}
	for _, tc := range fixtures {
		toolingFile(t, root, tc.path, tc.failureSource, 0600)
		if err := os.WriteFile(logPath, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sentinelLogPath, nil, 0600); err != nil {
			t.Fatal(err)
		}
		out, runErr := toolingRun(t, root, env, "bash", "scripts/check-offline-experiments.sh")
		if runErr == nil || !strings.Contains(out, tc.marker) {
			t.Errorf("default G01 %s failure was skipped: %s", tc.name, out)
		}
		data, readErr := os.ReadFile(sentinelLogPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		count := 0
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == tc.marker {
				count++
			}
		}
		if count != 1 {
			t.Errorf("default G01 %s sentinel %q ran %d times; sentinel log:\n%s", tc.name, tc.marker, count, data)
		}
		toolingFile(t, root, tc.path, tc.positiveSource, 0600)
	}
}

func TestToolingDefaultG02PartitionsRun(t *testing.T) {
	root := toolingFixture(t)
	const prepName = "TestPairedBrokerPrepareReviewedG01LiveBinary"
	const prepSentinel = "default-g02-prep-regression"
	const heavyName = "TestPairedBrokerRealCadenceChildExceedsThirtySeconds"
	const heavySentinel = "default-g02-heavy-regression"
	const pairedFamilyName = "TestPairedBrokerClaimFenceFixture"
	const pairedFamilySentinel = "default-g02-paired-family-regression"
	const remainderSentinel = "default-g02-remainder-regression"
	const otherPackageSentinel = "default-g02-other-package-regression"
	const sameNameOtherPackageSentinel = "default-g02-same-name-other-package-regression"
	const sameNamePairedFamilySentinel = "default-g02-same-name-paired-family-regression"
	const sameNamePrepSentinel = "default-g02-same-name-prep-regression"
	const exampleSentinel = "default-g02-example-output-regression"
	const fuzzSentinel = "default-g02-fuzz-seed-regression"
	g02Base := "experiments/g02-auth"
	remainderPackageBase := g02Base + "/remainderfixture"
	otherPackageBase := g02Base + "/otherfixture"
	sameNamePackageBase := g02Base + "/samefixture"
	pairedFamilyPackageBase := g02Base + "/pairedfixture"
	prepPackageBase := g02Base + "/prepfxture"
	defaultTestSource := func(pkg, testName, marker, failure string) string {
		failureLine := ""
		if failure != "" {
			failureLine = "\n\tt.Fatal(\"" + failure + "\")"
		}
		return `package ` + pkg + `

import (
	"os"
	"testing"
)

func ` + testName + `(t *testing.T) {
	path := os.Getenv("TOOLING_SENTINEL_LOG")
	if path == "" {
		t.Fatal("TOOLING_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("` + marker + `\n"); err != nil {
		t.Fatal(err)
	}` + failureLine + `
}
`
	}
	defaultExampleSource := func(marker, expected string) string {
		return `package fixture

import (
	"fmt"
	"os"
)

func Example_g02FixtureOutput() {
	path := os.Getenv("TOOLING_SENTINEL_LOG")
	if path == "" {
		panic("TOOLING_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if _, err := file.WriteString("` + marker + `\n"); err != nil {
		panic(err)
	}
	fmt.Println("` + marker + `")
	// Output: ` + expected + `
}
`
	}
	defaultFuzzSource := func(marker, failure string) string {
		failureLine := ""
		if failure != "" {
			failureLine = "\n\t\tt.Fatal(\"" + failure + "\")"
		}
		return `package fixture

import (
	"os"
	"testing"
)

func FuzzG02Fixture(f *testing.F) {
	f.Add("fixture-seed")
	f.Fuzz(func(t *testing.T, _ string) {
		path := os.Getenv("TOOLING_SENTINEL_LOG")
		if path == "" {
			t.Fatal("TOOLING_SENTINEL_LOG is not set")
		}
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := file.WriteString("` + marker + `\n"); err != nil {
			t.Fatal(err)
		}` + failureLine + `
	})
}
`
	}
	type fixture struct {
		name, path, marker, positiveSource, failureSource string
	}
	fixtures := []fixture{
		{
			name:           "prep",
			path:           g02Base + "/default_prep_regression_test.go",
			marker:         prepSentinel,
			positiveSource: defaultTestSource("fixture", prepName, prepSentinel, ""),
			failureSource:  defaultTestSource("fixture", prepName, prepSentinel, prepSentinel),
		},
		{
			name:           "heavy",
			path:           g02Base + "/default_heavy_regression_test.go",
			marker:         heavySentinel,
			positiveSource: defaultTestSource("fixture", heavyName, heavySentinel, ""),
			failureSource:  defaultTestSource("fixture", heavyName, heavySentinel, heavySentinel),
		},
		{
			name:           "paired family",
			path:           g02Base + "/default_paired_family_regression_test.go",
			marker:         pairedFamilySentinel,
			positiveSource: defaultTestSource("fixture", pairedFamilyName, pairedFamilySentinel, ""),
			failureSource:  defaultTestSource("fixture", pairedFamilyName, pairedFamilySentinel, pairedFamilySentinel),
		},
		{
			name:           "remainder",
			path:           remainderPackageBase + "/default_remainder_regression_test.go",
			marker:         remainderSentinel,
			positiveSource: defaultTestSource("remainderfixture", "TestDefaultG02RemainderFixture", remainderSentinel, ""),
			failureSource:  defaultTestSource("remainderfixture", "TestDefaultG02RemainderFixture", remainderSentinel, remainderSentinel),
		},
		{
			name:           "other package",
			path:           otherPackageBase + "/default_other_package_regression_test.go",
			marker:         otherPackageSentinel,
			positiveSource: defaultTestSource("otherfixture", "TestDefaultG02OtherPackageFixture", otherPackageSentinel, ""),
			failureSource:  defaultTestSource("otherfixture", "TestDefaultG02OtherPackageFixture", otherPackageSentinel, otherPackageSentinel),
		},
		{
			name:           "same-name other package",
			path:           sameNamePackageBase + "/default_same_name_regression_test.go",
			marker:         sameNameOtherPackageSentinel,
			positiveSource: defaultTestSource("samefixture", heavyName, sameNameOtherPackageSentinel, ""),
			failureSource:  defaultTestSource("samefixture", heavyName, sameNameOtherPackageSentinel, sameNameOtherPackageSentinel),
		},
		{
			name:           "same-name paired family other package",
			path:           pairedFamilyPackageBase + "/default_same_name_paired_family_regression_test.go",
			marker:         sameNamePairedFamilySentinel,
			positiveSource: defaultTestSource("pairedfixture", pairedFamilyName, sameNamePairedFamilySentinel, ""),
			failureSource:  defaultTestSource("pairedfixture", pairedFamilyName, sameNamePairedFamilySentinel, sameNamePairedFamilySentinel),
		},
		{
			name:           "same-name prep other package",
			path:           prepPackageBase + "/default_same_name_prep_regression_test.go",
			marker:         sameNamePrepSentinel,
			positiveSource: defaultTestSource("prepfixture", prepName, sameNamePrepSentinel, ""),
			failureSource:  defaultTestSource("prepfixture", prepName, sameNamePrepSentinel, sameNamePrepSentinel),
		},
		{
			name:           "Example Output",
			path:           g02Base + "/default_example_regression_test.go",
			marker:         exampleSentinel,
			positiveSource: defaultExampleSource(exampleSentinel, exampleSentinel),
			failureSource:  defaultExampleSource(exampleSentinel, "unexpected-default-g02-example-output"),
		},
		{
			name:           "Fuzz seed",
			path:           g02Base + "/default_fuzz_regression_test.go",
			marker:         fuzzSentinel,
			positiveSource: defaultFuzzSource(fuzzSentinel, ""),
			failureSource:  defaultFuzzSource(fuzzSentinel, fuzzSentinel),
		},
	}
	toolingFile(t, root, remainderPackageBase+"/fixture.go", "package remainderfixture\n", 0600)
	toolingFile(t, root, otherPackageBase+"/fixture.go", "package otherfixture\n", 0600)
	toolingFile(t, root, sameNamePackageBase+"/fixture.go", "package samefixture\n", 0600)
	toolingFile(t, root, pairedFamilyPackageBase+"/fixture.go", "package pairedfixture\n", 0600)
	toolingFile(t, root, prepPackageBase+"/fixture.go", "package prepfixture\n", 0600)
	for _, tc := range fixtures {
		toolingFile(t, root, tc.path, tc.positiveSource, 0600)
	}
	wrapper, logPath, realGo := toolingGoWrapper(t, root)
	sentinelLogPath := filepath.Join(root, "default-g02-sentinel.log")
	env := []string{
		"GO=" + wrapper,
		"TOOLING_REAL_GO=" + realGo,
		"TOOLING_GO_LOG=" + logPath,
		"TOOLING_SENTINEL_LOG=" + sentinelLogPath,
	}
	if out, err := toolingRun(t, root, env, "bash", "scripts/check-offline-experiments.sh"); err != nil {
		t.Fatalf("default G02 positive control: %s", out)
	}
	sentinelData, err := os.ReadFile(sentinelLogPath)
	if err != nil {
		t.Fatal(err)
	}
	sentinelLines := strings.Split(strings.TrimSpace(string(sentinelData)), "\n")
	for _, tc := range fixtures {
		count := 0
		for _, line := range sentinelLines {
			if line == tc.marker {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("positive %s sentinel %q ran %d times; sentinel log:\n%s", tc.name, tc.marker, count, sentinelData)
		}
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logData)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for _, invocation := range []string{
		"go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerPrepareReviewedG01LiveBinary$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestPaired -skip ^TestPairedBroker(PrepareReviewedG01LiveBinary|RealCadenceChildExceedsThirtySeconds)$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -skip ^TestPaired ./...",
	} {
		count := 0
		for _, line := range lines {
			if line == invocation {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("default G02 partition invocation %q ran %d times; wrapper log:\n%s", invocation, count, log)
		}
	}
	for _, invocation := range []string{
		"go1.26.8\ttest -race -count=1 -timeout=45s ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestPaired -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=45s -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=120s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.26.8\ttest -race -count=1 -timeout=120s -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
	} {
		for _, line := range lines {
			if line == invocation {
				t.Fatalf("offline gate retained forbidden G02 invocation %q; wrapper log:\n%s", invocation, log)
			}
		}
	}
	for _, tc := range fixtures {
		toolingFile(t, root, tc.path, tc.failureSource, 0600)
		if err := os.WriteFile(logPath, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sentinelLogPath, nil, 0600); err != nil {
			t.Fatal(err)
		}
		out, runErr := toolingRun(t, root, env, "bash", "scripts/check-offline-experiments.sh")
		if runErr == nil || !strings.Contains(out, tc.marker) {
			t.Errorf("default G02 %s failure was skipped: %s", tc.name, out)
		}
		data, readErr := os.ReadFile(sentinelLogPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		count := 0
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == tc.marker {
				count++
			}
		}
		if count != 1 {
			t.Errorf("default G02 %s sentinel %q ran %d times; sentinel log:\n%s", tc.name, tc.marker, count, data)
		}
		toolingFile(t, root, tc.path, tc.positiveSource, 0600)
	}
}

func TestG02OwnedPrepDirRemovedAfterPrepFailure(t *testing.T) {
	root := toolingFixture(t)
	script, err := os.ReadFile("check-offline-experiments.sh")
	if err != nil {
		t.Fatal(err)
	}
	body := string(script)
	if !strings.Contains(body, `trap 'rm -rf -- "${g02_prep_dir}"' EXIT`) {
		t.Fatal("G02 prep dir has no exact owned EXIT trap")
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "timeout=120s") && strings.Contains(line, "real_pair_cadence_regex") {
			t.Fatal("cadence partition timeout was widened instead of isolating fixture preparation")
		}
	}
	tmp := filepath.Join(root, "owned-tmp")
	if err := os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	const prepSentinel = "default-g02-prep-failure-cleanup"
	const cadenceSentinel = "default-g02-cadence-should-not-run"
	toolingFile(t, root, "experiments/g02-auth/default_prep_regression_test.go", `package fixture

import (
	"os"
	"testing"
)

func TestPairedBrokerPrepareReviewedG01LiveBinary(t *testing.T) {
	path := os.Getenv("TOOLING_SENTINEL_LOG")
	if path == "" {
		t.Fatal("TOOLING_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("`+prepSentinel+`\n"); err != nil {
		t.Fatal(err)
	}
	t.Fatal("`+prepSentinel+`")
}
`, 0600)
	toolingFile(t, root, "experiments/g02-auth/default_heavy_regression_test.go", `package fixture

import (
	"os"
	"testing"
)

func TestPairedBrokerRealCadenceChildExceedsThirtySeconds(t *testing.T) {
	path := os.Getenv("TOOLING_SENTINEL_LOG")
	if path == "" {
		t.Fatal("TOOLING_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("`+cadenceSentinel+`\n"); err != nil {
		t.Fatal(err)
	}
}
`, 0600)
	wrapper, logPath, realGo := toolingGoWrapper(t, root)
	sentinelLogPath := filepath.Join(root, "prep-failure-sentinel.log")
	env := []string{
		"GO=" + wrapper,
		"TOOLING_REAL_GO=" + realGo,
		"TOOLING_GO_LOG=" + logPath,
		"TOOLING_SENTINEL_LOG=" + sentinelLogPath,
		"TMPDIR=" + tmp,
	}
	out, runErr := toolingRun(t, root, env, "bash", "scripts/check-offline-experiments.sh")
	if runErr == nil {
		t.Fatalf("prep failure passed: %s", out)
	}
	if !strings.Contains(out, prepSentinel) {
		t.Fatalf("prep failure did not propagate: %s", out)
	}
	sentinelData, readErr := os.ReadFile(sentinelLogPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	sentinels := string(sentinelData)
	if strings.Count(sentinels, prepSentinel+"\n") != 1 {
		t.Fatalf("prep sentinel ran %d times; sentinel log:\n%s", strings.Count(sentinels, prepSentinel+"\n"), sentinelData)
	}
	if strings.Contains(sentinels, cadenceSentinel) {
		t.Fatalf("cadence ran after prep failure; sentinel log:\n%s", sentinelData)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	var leftover []string
	for _, entry := range entries {
		if entry.IsDir() {
			leftover = append(leftover, entry.Name())
		}
	}
	if len(leftover) != 0 {
		t.Fatalf("owned prep dir leaked after prep failure: %q", leftover)
	}
}

func TestG02OwnedPrepCleanupContract(t *testing.T) {
	script, err := os.ReadFile("check-offline-experiments.sh")
	if err != nil {
		t.Fatal(err)
	}
	body := string(script)
	mktempAt := strings.Index(body, "g02_prep_dir=$(mktemp -d)")
	exitTrapAt := strings.Index(body, `trap 'rm -rf -- "${g02_prep_dir}"' EXIT`)
	intTrapAt := strings.Index(body, `trap 'trap - EXIT; rm -rf -- "${g02_prep_dir}"; exit 130' INT`)
	termTrapAt := strings.Index(body, `trap 'trap - EXIT; rm -rf -- "${g02_prep_dir}"; exit 143' TERM`)
	hupTrapAt := strings.Index(body, `trap 'trap - EXIT; rm -rf -- "${g02_prep_dir}"; exit 129' HUP`)
	chmodAt := strings.Index(body, `chmod 0700 "${g02_prep_dir}"`)
	if mktempAt < 0 || exitTrapAt < 0 || intTrapAt < 0 || termTrapAt < 0 || hupTrapAt < 0 || chmodAt < 0 {
		t.Fatal("G02 prep dir is missing owned EXIT/INT/TERM/HUP cleanup traps")
	}
	if !(mktempAt < exitTrapAt && exitTrapAt < intTrapAt && intTrapAt < termTrapAt && termTrapAt < hupTrapAt && hupTrapAt < chmodAt) {
		t.Fatal("cleanup traps must be registered immediately after mktemp and before chmod")
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "timeout=120s") && strings.Contains(line, "real_pair_cadence_regex") {
			t.Fatal("cadence partition timeout was widened instead of isolating fixture preparation")
		}
	}
}

func TestG02OwnedPrepDirRemovedAfterSuccess(t *testing.T) {
	root, tmp, env := toolingG02CleanupEnv(t, "")
	out, err := toolingRun(t, root, env, "bash", "scripts/check-offline-experiments.sh")
	if err != nil {
		t.Fatalf("success cleanup failed: %s", out)
	}
	if leftover := toolingLeftoverDirs(t, tmp); len(leftover) != 0 {
		t.Fatalf("owned prep dir leaked after success: %q", leftover)
	}
}

func TestG02OwnedPrepDirRemovedAfterExit91(t *testing.T) {
	root, tmp, env := toolingG02CleanupEnv(t, `#!/bin/sh
set -eu
printf '%s\t%s\n' "${GOTOOLCHAIN:-}" "$*" >> "$TOOLING_GO_LOG"
if [ -n "${G01_PAIR_BRIDGE_PREP_DIR:-}" ]; then
	case "$*" in
	*" -run ^TestPairedBrokerPrepareReviewedG01LiveBinary$ "*)
		printf '%s\n' "${G01_PAIR_BRIDGE_PREP_DIR}" > "$TOOLING_PREP_DIR_FILE"
		exit 91
		;;
	esac
fi
exec "$TOOLING_REAL_GO" "$@"
`)
	out, err := toolingRun(t, root, env, "bash", "scripts/check-offline-experiments.sh")
	if toolingExitCode(err) != 91 {
		t.Fatalf("exit 91 status=%d err=%v out=%s", toolingExitCode(err), err, out)
	}
	toolingMustRemoveOwnedPrep(t, env, tmp)
	toolingMustNotInvokeCadence(t, env)
}

func TestG02OwnedPrepDirRemovedAfterSignal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		signal string
		status int
	}{
		{name: "TERM", signal: "TERM", status: 143},
		{name: "INT", signal: "INT", status: 130},
		{name: "HUP", signal: "HUP", status: 129},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, tmp, env := toolingG02CleanupEnv(t, `#!/bin/sh
set -eu
printf '%s\t%s\n' "${GOTOOLCHAIN:-}" "$*" >> "$TOOLING_GO_LOG"
if [ -n "${G01_PAIR_BRIDGE_PREP_DIR:-}" ]; then
	case "$*" in
	*" -run ^TestPairedBrokerPrepareReviewedG01LiveBinary$ "*)
		printf '%s\n' "${G01_PAIR_BRIDGE_PREP_DIR}" > "$TOOLING_PREP_DIR_FILE"
		printf '%s\n' "$PPID" > "$TOOLING_G02_SHELL_PID_FILE"
		printf '%s\n' "$$" > "$TOOLING_WRAPPER_PID_FILE"
		while [ ! -f "$TOOLING_HOLD_RELEASE" ]; do
			sleep 1
		done
		exit 0
		;;
	esac
fi
exec "$TOOLING_REAL_GO" "$@"
`)
			holdRelease := toolingEnvValue(env, "TOOLING_HOLD_RELEASE")
			if holdRelease == "" {
				t.Fatal("hold release path is missing")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "scripts/check-offline-experiments.sh")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.26.8", "GOFLAGS=", "GOWORK=off")
			cmd.Env = append(cmd.Env, env...)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			reaped := false
			t.Cleanup(func() {
				_ = os.WriteFile(holdRelease, []byte("1\n"), 0600)
				if !reaped && cmd.Process != nil {
					_ = cmd.Process.Kill()
					_, _ = cmd.Process.Wait()
				}
			})
			g02PID := toolingWaitPIDFile(t, toolingEnvValue(env, "TOOLING_G02_SHELL_PID_FILE"), 40*time.Second)
			toolingRequireOwnedG02Shell(t, cmd.Process.Pid, g02PID)
			toolingSignalExactPID(t, g02PID, tc.signal)
			if err := os.WriteFile(holdRelease, []byte("1\n"), 0600); err != nil {
				t.Fatal(err)
			}
			waitErr := cmd.Wait()
			reaped = true
			if toolingExitCode(waitErr) != tc.status {
				t.Fatalf("%s status=%d err=%v out=%s", tc.name, toolingExitCode(waitErr), waitErr, out.String())
			}
			toolingMustRemoveOwnedPrep(t, env, tmp)
			toolingMustNotInvokeCadence(t, env)
		})
	}
}

func toolingG02CleanupEnv(t *testing.T, wrapperBody string) (root, tmp string, env []string) {
	t.Helper()
	root = toolingFixture(t)
	tmp = filepath.Join(root, "owned-tmp")
	if err := os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "go-wrapper.log")
	prepDirFile := filepath.Join(root, "g02-prep-dir")
	g02PIDFile := filepath.Join(root, "g02-shell.pid")
	wrapperPIDFile := filepath.Join(root, "g02-wrapper.pid")
	holdRelease := filepath.Join(root, "g02-hold-release")
	if wrapperBody == "" {
		wrapper, _, _ := toolingGoWrapper(t, root)
		env = []string{
			"GO=" + wrapper,
			"TOOLING_REAL_GO=" + realGo,
			"TOOLING_GO_LOG=" + logPath,
			"TOOLING_PREP_DIR_FILE=" + prepDirFile,
			"TOOLING_G02_SHELL_PID_FILE=" + g02PIDFile,
			"TOOLING_WRAPPER_PID_FILE=" + wrapperPIDFile,
			"TOOLING_HOLD_RELEASE=" + holdRelease,
			"TMPDIR=" + tmp,
		}
		return root, tmp, env
	}
	toolingFile(t, root, "logging-go", wrapperBody, 0700)
	env = []string{
		"GO=" + filepath.Join(root, "logging-go"),
		"TOOLING_REAL_GO=" + realGo,
		"TOOLING_GO_LOG=" + logPath,
		"TOOLING_PREP_DIR_FILE=" + prepDirFile,
		"TOOLING_G02_SHELL_PID_FILE=" + g02PIDFile,
		"TOOLING_WRAPPER_PID_FILE=" + wrapperPIDFile,
		"TOOLING_HOLD_RELEASE=" + holdRelease,
		"TMPDIR=" + tmp,
	}
	return root, tmp, env
}

func toolingEnvValue(env []string, key string) string {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return strings.TrimPrefix(kv, prefix)
		}
	}
	return ""
}

func toolingExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func toolingLeftoverDirs(t *testing.T, tmp string) []string {
	t.Helper()
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	var leftover []string
	for _, entry := range entries {
		if entry.IsDir() {
			leftover = append(leftover, entry.Name())
		}
	}
	return leftover
}

func toolingMustRemoveOwnedPrep(t *testing.T, env []string, tmp string) {
	t.Helper()
	prepDir := strings.TrimSpace(toolingReadFile(t, toolingEnvValue(env, "TOOLING_PREP_DIR_FILE")))
	if prepDir == "" {
		t.Fatal("owned prep dir path was not recorded")
	}
	if _, err := os.Stat(prepDir); !os.IsNotExist(err) {
		t.Fatalf("owned prep dir leaked: %s err=%v", prepDir, err)
	}
	if leftover := toolingLeftoverDirs(t, tmp); len(leftover) != 0 {
		t.Fatalf("owned prep dir leaked under TMPDIR: %q", leftover)
	}
}

func toolingMustNotInvokeCadence(t *testing.T, env []string) {
	t.Helper()
	logPath := toolingEnvValue(env, "TOOLING_GO_LOG")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cadence := "go1.26.8\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./..."
	for _, line := range strings.Split(string(data), "\n") {
		if line == cadence {
			t.Fatalf("cadence ran after G02 prep cleanup path; wrapper log:\n%s", data)
		}
	}
}

func toolingReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func toolingWaitPIDFile(t *testing.T, path string, d time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			text := strings.TrimSpace(string(data))
			if text != "" {
				pid, convErr := strconv.Atoi(text)
				if convErr != nil {
					t.Fatal(convErr)
				}
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for pid file %s", path)
	return 0
}

func toolingRequireOwnedG02Shell(t *testing.T, scriptPID, g02PID int) {
	t.Helper()
	if scriptPID <= 1 || g02PID <= 1 {
		t.Fatalf("refusing to signal pid script=%d g02=%d", scriptPID, g02PID)
	}
	self := os.Getpid()
	if g02PID == self || scriptPID == self {
		t.Fatal("refusing to signal the test process")
	}
	out, err := exec.Command("ps", "-o", "pid=,ppid=,command=", "-p", strconv.Itoa(g02PID)).CombinedOutput()
	if err != nil {
		t.Fatalf("ps g02 pid %d: %s", g02PID, out)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		t.Fatalf("unexpected ps output for %d: %q", g02PID, out)
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		t.Fatal(err)
	}
	if ppid != scriptPID {
		t.Fatalf("g02 pid %d parent=%d, want script pid %d; ps=%q", g02PID, ppid, scriptPID, out)
	}
	cmd := strings.ToLower(strings.Join(fields[2:], " "))
	for _, forbidden := range []string{"runner.listener", "actions-runner", "launchd", "docker", "lima", "colima", "keychain"} {
		if strings.Contains(cmd, forbidden) {
			t.Fatalf("refusing to signal non-owned process %d command=%q", g02PID, cmd)
		}
	}
	if !strings.Contains(cmd, "check-offline-experiments.sh") {
		t.Fatalf("g02 pid %d is not the offline-experiment subshell: %q", g02PID, cmd)
	}
}

func toolingSignalExactPID(t *testing.T, pid int, spec string) {
	t.Helper()
	if pid <= 1 {
		t.Fatalf("refusing to signal pid %d", pid)
	}
	switch spec {
	case "TERM", "INT", "HUP":
	default:
		t.Fatalf("unsupported signal spec %q", spec)
	}
	out, err := exec.Command("kill", "-s", spec, strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		t.Fatalf("kill -s %s %d: %s", spec, pid, out)
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
