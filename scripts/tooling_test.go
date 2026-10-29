package tooling

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	g01TerminalHeavyRegex          = `^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks)$`
	g01TerminalRemainderSkipRegex  = `^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks|Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$`
	g01TerminalStorageRegex        = `^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$`
	g01StorageJournalRegex         = `^TestPairedTerminalPostIntentJournalIdentity$`
	g01TerminalHeavyInvocation     = "go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run " + g01TerminalHeavyRegex + " ./livecanary"
	g01TerminalRemainderInvocation = "go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPairedTerminal -skip " + g01TerminalRemainderSkipRegex + " ./livecanary"
	g01StorageJournalInvocation    = "go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run " + g01StorageJournalRegex + " ./livecanary"
	g01TerminalStorageInvocation   = "go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run " + g01TerminalStorageRegex + " -skip " + g01StorageJournalRegex + " ./livecanary"
	g01LegacyStorageInvocation     = "go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run " + g01TerminalStorageRegex + " ./livecanary"
	g01LegacyTerminalInvocation    = "go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPairedTerminal -skip " + g01TerminalStorageRegex + " ./livecanary"
	g01ReplacementInvocation       = "go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestSessionReplacement ./livecanary"
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
	env := make([]string, 0, len(os.Environ())+3+len(extra))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "FAST_MODULE=") || strings.HasPrefix(entry, "FAST_PACKAGE=") || strings.HasPrefix(entry, "FAST_TEST=") {
			continue
		}
		env = append(env, entry)
	}
	cmd.Env = append(env, "GOTOOLCHAIN=go1.27.3", "GOFLAGS=", "GOWORK=off")
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
if [ "${1:-}" != run ]; then
	printf '%s\t%s\n' "${GOTOOLCHAIN:-}" "$*" >> "$TOOLING_GO_LOG"
fi
exec "$TOOLING_REAL_GO" "$@"
`, 0700)
	return wrapperPath, logPath, realGo
}

func toolingFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"Makefile", "go.mod", "LICENSE", "docs/DEPENDENCIES.md", "scripts/check-toolchain.sh", "scripts/check-licenses.sh", "scripts/check-offline-experiments.sh", "scripts/gofmt.sh", "scripts/fuzz-smoke.sh", "scripts/fast-check.sh", "scripts/fast-check-events/main.go"} {
		data, err := os.ReadFile(filepath.Join("..", name))
		if err != nil {
			t.Fatal(err)
		}
		toolingFile(t, root, name, string(data), 0600)
	}
	toolingFile(t, root, "cmd/gh-runnerd/main.go", "package main\n\nfunc main() {}\n", 0600)
	toolingFile(t, root, "scripts/fixture_test.go", "package tooling\n\nimport \"testing\"\n\nfunc TestToolingTaggedPairFixturePartitionsRun(t *testing.T) {}\n", 0600)
	for _, module := range []string{"g01-scaleset", "g02-auth", "r1-credentials"} {
		base := "experiments/" + module
		toolingFile(t, root, base+"/go.mod", "module example.test/"+module+"\n\ngo 1.27.3\n", 0600)
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

func TestToolingRootTestPartitions(t *testing.T) {
	const heavy = `^TestToolingTaggedPairFixturePartitionsRun$`
	const owned = `^TestG02OwnedPrepDir`
	const remainderSkip = `^Test(ToolingTaggedPairFixturePartitionsRun$|G02OwnedPrepDir)`
	for _, target := range []struct{ name, flags string }{
		{"test", "-count=1"},
		{"test-race", "-race -count=1"},
	} {
		for _, failCall := range []int{0, 1, 2, 3} {
			t.Run(target.name+"/fail="+strconv.Itoa(failCall), func(t *testing.T) {
				root := t.TempDir()
				makefile, err := os.ReadFile("../Makefile")
				if err != nil {
					t.Fatal(err)
				}
				toolingFile(t, root, "Makefile", string(makefile), 0600)
				// Record actual make execution without running the inherited suites.
				// The spaced path also checks that all recipes honor the GO override.
				toolingFile(t, root, "recording go", `#!/bin/sh
set -eu
printf '%s\t%s\t%s\n' "$GOTOOLCHAIN" "$GOFLAGS" "$*" >> "$TOOLING_GO_LOG"
ordinal=0
if [ -f "$TOOLING_GO_LOG.count" ]; then read -r ordinal < "$TOOLING_GO_LOG.count"; fi
ordinal=$((ordinal + 1))
printf '%s\n' "$ordinal" > "$TOOLING_GO_LOG.count"
if [ "$ordinal" = "$TOOLING_FAIL_CALL" ]; then exit 23; fi
printf 'done\n' >> "$TOOLING_GO_LOG"
`, 0700)
				logPath := filepath.Join(root, "calls.log")
				out, err := toolingRun(t, root, []string{
					"TOOLING_GO_LOG=" + logPath,
					"TOOLING_FAIL_CALL=" + strconv.Itoa(failCall),
					"GOTOOLCHAIN=auto",
				}, "make", "GO="+filepath.Join(root, "recording go"), "GOFLAGS=-mod=readonly -p=2", target.name)
				if failCall == 0 && err != nil {
					t.Fatalf("partitioned make failed: %v\n%s", err, out)
				}
				if failCall != 0 && (err == nil || !strings.Contains(out, "Error 23")) {
					t.Fatalf("partition failure was not propagated: %v\n%s", err, out)
				}
				var want string
				for i, selection := range []string{
					"-skip " + remainderSkip + " ./...",
					"-run " + owned + " ./scripts",
					"-run " + heavy + " ./scripts",
				} {
					want += "go1.27.3\t-mod=readonly -p=2\ttest " + target.flags + " " + selection + "\n"
					if failCall == i+1 {
						break
					}
					want += "done\n"
				}
				got, err := os.ReadFile(logPath)
				if err != nil || string(got) != want {
					t.Fatalf("root partitions: err=%v\ngot  %q\nwant %q\nmake output: %s", err, got, want, out)
				}
				if failCall != 0 {
					return
				}
				// Evaluate the actual selectors received by Go, after Make's dollar
				// expansion, so a future name cannot fall into a gap or run twice.
				var selectors []struct{ run, skip *regexp.Regexp }
				for _, line := range strings.Split(strings.TrimSpace(string(got)), "\n") {
					if line == "done" {
						continue
					}
					selection := struct{ run, skip *regexp.Regexp }{regexp.MustCompile(""), regexp.MustCompile("a^")}
					args := strings.Fields(strings.Split(line, "\t")[2])
					for i, arg := range args {
						switch arg {
						case "-run":
							selection.run = regexp.MustCompile(args[i+1])
						case "-skip":
							selection.skip = regexp.MustCompile(args[i+1])
						}
					}
					selectors = append(selectors, selection)
				}
				for _, tc := range []struct {
					name  string
					stage int
				}{
					{"TestToolingRootTestPartitions", 1},
					{"TestG02OwnedPrepCleanupContract", 1},
					{"TestFutureUnrelated", 1},
					{"ExampleFuture", 1},
					{"FuzzFuture", 1},
					{"TestToolingTaggedPairFixturePartitionsRunExtension", 1},
					{"TestG02OwnedPrepDirRemovedAfterPrepFailure", 2},
					{"TestG02OwnedPrepDirRemovedAfterSuccess", 2},
					{"TestG02OwnedPrepDirRemovedAfterExit91", 2},
					{"TestG02OwnedPrepDirRemovedAfterSignal", 2},
					{"TestG02OwnedPrepDirFuture", 2},
					{"TestToolingTaggedPairFixturePartitionsRun", 3},
				} {
					var stages []int
					for i, selection := range selectors {
						if selection.run.MatchString(tc.name) && !selection.skip.MatchString(tc.name) {
							stages = append(stages, i+1)
						}
					}
					if len(stages) != 1 || stages[0] != tc.stage {
						t.Errorf("%s selected by stages %v; want only %d", tc.name, stages, tc.stage)
					}
				}
			})
		}
	}
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
	toolingFile(t, root, "newer-go", "#!/bin/sh\nif [ \"${GOTOOLCHAIN:-auto}\" = go1.27.3 ]; then exec \"$TOOLING_REAL_GO\" \"$@\"; fi\nif [ \"$1\" = version ]; then echo 'go version go1.28.1 linux/amd64'; else exit 88; fi\n", 0700)
	for _, args := range [][]string{{"make", "GO=" + filepath.Join(root, "newer-go"), "toolchain"}, {"bash", "scripts/check-toolchain.sh"}} {
		env := []string{"GOTOOLCHAIN=auto", "GO=" + filepath.Join(root, "newer-go"), "TOOLING_REAL_GO=" + realGo}
		if out, err := toolingRun(t, root, env, args...); err != nil || !strings.Contains(out, "toolchain: go1.27.3") {
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

func TestToolingCheckToolchainRequiresReleaseDefaults(t *testing.T) {
	for _, experiment := range []string{"", "none"} {
		t.Run("GOEXPERIMENT="+experiment, func(t *testing.T) {
			root := toolingFixture(t)
			out, err := toolingRun(t, root, []string{"GOEXPERIMENT=" + experiment}, "bash", "scripts/check-toolchain.sh")
			if experiment == "" {
				if err != nil {
					t.Fatalf("release defaults rejected: %s", out)
				}
			} else if err == nil || !strings.Contains(out, "GOEXPERIMENT") {
				t.Fatalf("non-default compiler profile accepted by exact-toolchain check: %s", out)
			}
		})
	}
}

func TestToolingCheckToolchainRequiresGo1273(t *testing.T) {
	for _, tc := range []struct {
		name, reportedVersion, want string
		wantError                   bool
	}{
		{name: "accept exact patch", reportedVersion: "go1.27.3", want: "toolchain: go1.27.3"},
		{name: "reject prior shared pin", reportedVersion: "go1.26.9", want: "selected go1.26.9, want go1.27.3", wantError: true},
		{name: "reject previous patch", reportedVersion: "go1.27.2", want: "selected go1.27.2, want go1.27.3", wantError: true},
		{name: "reject nondefault build stamp", reportedVersion: "go1.27.3-X:nojsonv2", want: "selected go1.27.3-X:nojsonv2, want go1.27.3", wantError: true},
		{name: "reject different newer patch", reportedVersion: "go1.27.4", want: "selected go1.27.4, want go1.27.3", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := toolingFixture(t)
			toolingFile(t, root, "go.mod", "module example.test/root\n\ngo 1.27.3\n", 0600)
			toolingFile(t, root, "reported-go", `#!/bin/sh
set -eu
if [ "${1:-}" = version ]; then
	printf 'go version %s darwin/arm64\n' "$TOOLING_REPORTED_GO_VERSION"
elif [ "${1:-}" = env ] && [ "${2:-}" = GOTOOLCHAIN ]; then
	printf '%s\n' "$GOTOOLCHAIN"
else
	exit 88
fi
`, 0700)
			goPath := filepath.Join(root, "reported-go")
			out, err := toolingRun(t, root, []string{
				"GO=" + goPath,
				"TOOLING_REPORTED_GO_VERSION=" + tc.reportedVersion,
			}, "bash", "scripts/check-toolchain.sh")
			if tc.wantError {
				if err == nil || !strings.Contains(out, tc.want) {
					t.Fatalf("old Go toolchain was not rejected explicitly: err=%v output=%s", err, out)
				}
				return
			}
			if err != nil || !strings.Contains(out, tc.want) {
				t.Fatalf("Go 1.27.3 did not pass exact toolchain admission: err=%v output=%s", err, out)
			}
		})
	}
}

func assertFastCheckRequiresExplicitSelectors(t *testing.T, root, fastCheck string) {
	t.Helper()
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{name: "missing module", env: []string{"FAST_PACKAGE=./cmd/gh-runnerd", "FAST_TEST=^TestFastSelected$"}, want: "FAST_MODULE"},
		{name: "missing package", env: []string{"FAST_MODULE=.", "FAST_TEST=^TestFastSelected$"}, want: "FAST_PACKAGE"},
		{name: "missing test", env: []string{"FAST_MODULE=.", "FAST_PACKAGE=./cmd/gh-runnerd"}, want: "FAST_TEST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := toolingRun(t, root, tc.env, "bash", fastCheck)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("fast check accepted %s: err=%v output=%s", tc.name, err, out)
			}
		})
	}
}

func TestFastCheckHandlesJSONGOFLAGS(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import "testing"

func TestFastSelected(t *testing.T) {}
`, 0600)
	out, err := toolingRun(t, root, []string{"GOFLAGS=-json -race"}, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./cmd/gh-runnerd",
		"FAST_TEST=^TestFastSelected$$",
		"fast",
	)
	if err != nil {
		t.Fatalf("make fast did not normalize JSON output while preserving other GOFLAGS: %s", out)
	}
}

func TestFastCheckPreservesEnvironmentSelectorDollar(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import "testing"

func TestB(t *testing.T) {}
`, 0600)
	out, err := toolingRun(t, root, []string{
		"FAST_MODULE=.",
		"FAST_PACKAGE=./cmd/gh-runnerd",
		"FAST_TEST=^TestA$.*",
	}, "make", "fast")
	if err == nil || !strings.Contains(out, "FAST_TEST matched no compiled test") {
		t.Fatalf("environment selector was changed before matching: err=%v output=%s", err, out)
	}
}

func TestFastCheckExecutesSelectedTestWithListGOFLAGS(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import "testing"

func TestFastSelected(t *testing.T) {
	t.Fatal("selected-test-ran")
}
`, 0600)
	for _, tc := range []struct {
		name, test, want string
	}{
		{name: "selected test runs", test: "^TestFastSelected$", want: "selected-test-ran"},
		{name: "no matching test remains an error", test: "^NoSuchTest$", want: "fast check failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := toolingRun(t, root, []string{"GOFLAGS=-list=."}, "make",
				"FAST_MODULE=.",
				"FAST_PACKAGE=./cmd/gh-runnerd",
				"FAST_TEST="+strings.ReplaceAll(tc.test, "$", "$$"),
				"fast",
			)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("GOFLAGS=-list=. accepted %s: err=%v output=%s", tc.name, err, out)
			}
		})
	}
}

func TestFastCheckRequiresCompleteSubtestSelector(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import "testing"

func TestFastCheckParent(t *testing.T) {
	t.Run("ActualChild", func(t *testing.T) {})
	t.Run("Actual", func(t *testing.T) {})
	t.Run("Other", func(t *testing.T) {})
	t.Run("hello world", func(t *testing.T) {})
}
`, 0600)
	for _, tc := range []struct {
		name, selector, wantOutput string
		wantErr                    bool
	}{
		{
			name:       "missing child",
			selector:   "^TestFastCheckParent$/^MissingChild$",
			wantErr:    true,
			wantOutput: "FAST_TEST matched no compiled test",
		},
		{
			name:     "actual child",
			selector: "^TestFastCheckParent$/^ActualChild$",
		},
		{
			name:     "subtest whitespace",
			selector: "^TestFastCheckParent$/^hello world$",
		},
		{
			name:     "slash in character class",
			selector: "^TestFastCheckParent$/^Actual[/]?$",
		},
		{
			name:     "slash in parentheses",
			selector: "^TestFastCheckParent$/^Actual(/)?$",
		},
		{
			name:     "grouped alternation",
			selector: "^TestFastCheckParent$/^(Actual|Other)$",
		},
		{
			name:     "top-level alternation",
			selector: "^TestFastCheckParent$/^ActualChild$|^TestFastCheckParent$/^Other$",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := toolingRun(t, root, nil, "make",
				"FAST_MODULE=.",
				"FAST_PACKAGE=./cmd/gh-runnerd",
				"FAST_TEST="+strings.ReplaceAll(tc.selector, "$", "$$"),
				"fast",
			)
			if (err != nil) != tc.wantErr {
				t.Fatalf("selector %q returned err=%v, want error=%t; output=%s", tc.selector, err, tc.wantErr, out)
			}
			if tc.wantOutput != "" && !strings.Contains(out, tc.wantOutput) {
				t.Fatalf("selector %q output lacks %q: %s", tc.selector, tc.wantOutput, out)
			}
		})
	}
}

func TestFastCheckRejectsSyntheticTestMainRun(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("=== RUN   NoSuchTest")
	os.Exit(m.Run())
}

func TestFastActual(t *testing.T) {}
`, 0600)
	out, err := toolingRun(t, root, []string{"GOFLAGS=-v"}, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./cmd/gh-runnerd",
		"FAST_TEST=^NoSuchTest$$",
		"fast",
	)
	if err == nil || !strings.Contains(out, "FAST_TEST matched no compiled test") {
		t.Fatalf("synthetic TestMain run was treated as selected-test evidence: err=%v output=%s", err, out)
	}
}

func TestFastCheckRejectsSpoofedTestMainEvents(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("=== RUN   NoSuchTest")
	fmt.Println("--- PASS: NoSuchTest (0.00s)")
	os.Exit(m.Run())
}

func TestFastActual(t *testing.T) {}
`, 0600)
	out, err := toolingRun(t, root, []string{"GOFLAGS=-json"}, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./cmd/gh-runnerd",
		"FAST_TEST=^NoSuchTest$$",
		"fast",
	)
	if err == nil || !strings.Contains(out, "FAST_TEST matched no compiled test") {
		t.Fatalf("spoofed TestMain run/pass events were treated as selected-test evidence: err=%v output=%s", err, out)
	}
}

func TestFastCheckRejectsSkippedSelectedTest(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import "testing"

func TestFastSkipped(t *testing.T) {
	t.Skip("bounded fixture skip")
}
`, 0600)
	out, err := toolingRun(t, root, nil, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./cmd/gh-runnerd",
		"FAST_TEST=^TestFastSkipped$$",
		"fast",
	)
	if err == nil || !strings.Contains(out, "FAST_TEST matched no compiled test") {
		t.Fatalf("skipped test was treated as passing evidence: err=%v output=%s", err, out)
	}
}

func TestFastCheckAcceptsPassingSameNameAcrossPackages(t *testing.T) {
	for _, tc := range []struct {
		name, firstDir, firstPackage, firstBody, secondDir, secondPackage, secondBody, wantOutput string
		wantErr                                                                                   bool
	}{
		{
			name:          "skip then pass",
			firstDir:      "a-skip",
			firstPackage:  "skipfixture",
			firstBody:     "\tt.Skip(\"bounded fixture skip\")\n",
			secondDir:     "b-pass",
			secondPackage: "passfixture",
			secondBody:    "",
		},
		{
			name:          "pass then skip",
			firstDir:      "a-pass",
			firstPackage:  "passfixture",
			firstBody:     "",
			secondDir:     "b-skip",
			secondPackage: "skipfixture",
			secondBody:    "\tt.Skip(\"bounded fixture skip\")\n",
		},
		{
			name:          "pass then fail",
			firstDir:      "a-pass",
			firstPackage:  "passfixture",
			firstBody:     "",
			secondDir:     "b-fail",
			secondPackage: "failfixture",
			secondBody:    "\tt.Fatal(\"bounded fixture failure\")\n",
			wantOutput:    "fast check failed: selected test command exited",
			wantErr:       true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := toolingFixture(t)
			toolingFile(t, root, "internal/fast-check-multi/"+tc.firstDir+"/fast_check_test.go", `package `+tc.firstPackage+`

import "testing"

func TestFastSameName(t *testing.T) {
`+tc.firstBody+`}
`, 0600)
			toolingFile(t, root, "internal/fast-check-multi/"+tc.secondDir+"/fast_check_test.go", `package `+tc.secondPackage+`

import "testing"

func TestFastSameName(t *testing.T) {
`+tc.secondBody+`}
`, 0600)
			out, err := toolingRun(t, root, []string{"GOFLAGS=-p=1"}, "make",
				"FAST_MODULE=.",
				"FAST_PACKAGE=./internal/fast-check-multi/...",
				"FAST_TEST=^TestFastSameName$$",
				"fast",
			)
			if (err != nil) != tc.wantErr {
				t.Fatalf("same-name package order %s returned err=%v, want error=%t: %s", tc.name, err, tc.wantErr, out)
			}
			if tc.wantOutput != "" && !strings.Contains(out, tc.wantOutput) {
				t.Fatalf("same-name package order %s output lacks %q: %s", tc.name, tc.wantOutput, out)
			}
		})
	}
}

func TestFastCheckNeutralizesGoTestExecutionFlags(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import (
	"os"
	"testing"
)

func fastCheckRecord(t testing.TB, marker string) {
	t.Helper()
	path := os.Getenv("TOOLING_FAST_SENTINEL_LOG")
	if path == "" {
		t.Fatal("TOOLING_FAST_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(marker + "\n"); err != nil {
		t.Fatal(err)
	}
}

func TestFastCheckSelected(t *testing.T) {
	fastCheckRecord(t, "selected-test")
}

func BenchmarkFastCheckUnexpected(b *testing.B) {
	fastCheckRecord(b, "unexpected-benchmark")
	b.Fatal("unexpected benchmark ran")
}

func BenchmarkFastCheckFalseSuccess(b *testing.B) {
	fastCheckRecord(b, "false-success-benchmark")
}

func FuzzFastCheckUnexpected(f *testing.F) {
	f.Add("fixture-seed")
	f.Fuzz(func(t *testing.T, _ string) {
		fastCheckRecord(t, "unexpected-fuzz")
		t.Fatal("unexpected fuzz ran")
	})
}

func FuzzFastCheckFalseSuccess(f *testing.F) {
	f.Add("fixture-seed")
	f.Fuzz(func(t *testing.T, _ string) {
		fastCheckRecord(t, "false-success-fuzz")
	})
}
`, 0600)
	sentinelLog := filepath.Join(root, "fast-check-sentinel.log")
	for _, tc := range []struct {
		name, goflags, test, wantOutput string
		wantErr                         bool
		wantSentinel                    string
	}{
		{
			name:         "benchmark cannot expand selected test",
			goflags:      "-bench=. -benchtime=1x",
			test:         "^TestFastCheckSelected$",
			wantSentinel: "selected-test\n",
		},
		{
			name:         "inherited run and count cannot replace bounded selector",
			goflags:      "-run=^NoSuchTest$ -count=5",
			test:         "^TestFastCheckSelected$",
			wantSentinel: "selected-test\n",
		},
		{
			name:         "fuzz cannot expand selected test",
			goflags:      "-fuzz=^FuzzFastCheckUnexpected$ -fuzztime=1x",
			test:         "^TestFastCheckSelected$",
			wantSentinel: "selected-test\n",
		},
		{
			name:         "skip and build-only cannot suppress selected test",
			goflags:      "-skip=^TestFastCheckSelected$ -c -count=5",
			test:         "^TestFastCheckSelected$",
			wantSentinel: "selected-test\n",
		},
		{
			name:       "benchmark cannot satisfy missing test",
			goflags:    "-bench=^BenchmarkFastCheckFalseSuccess$ -benchtime=1x",
			test:       "^NoSuchTest$",
			wantErr:    true,
			wantOutput: "FAST_TEST matched no compiled test",
		},
		{
			name:       "fuzz cannot satisfy missing test",
			goflags:    "-fuzz=^FuzzFastCheckFalseSuccess$ -fuzztime=1x",
			test:       "^NoSuchTest$",
			wantErr:    true,
			wantOutput: "FAST_TEST matched no compiled test",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(sentinelLog, nil, 0600); err != nil {
				t.Fatal(err)
			}
			env := []string{
				"GOFLAGS=" + tc.goflags,
				"TOOLING_FAST_SENTINEL_LOG=" + sentinelLog,
			}
			out, err := toolingRun(t, root, env, "make",
				"FAST_MODULE=.",
				"FAST_PACKAGE=./cmd/gh-runnerd",
				"FAST_TEST="+strings.ReplaceAll(tc.test, "$", "$$"),
				"fast",
			)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GOFLAGS=%q test=%q returned err=%v, want error=%t; output=%s", tc.goflags, tc.test, err, tc.wantErr, out)
			}
			if tc.wantOutput != "" && !strings.Contains(out, tc.wantOutput) {
				t.Fatalf("GOFLAGS=%q test=%q output lacks %q: %s", tc.goflags, tc.test, tc.wantOutput, out)
			}
			gotSentinel := toolingReadFile(t, sentinelLog)
			if tc.wantSentinel != "" && gotSentinel != tc.wantSentinel {
				t.Fatalf("GOFLAGS=%q test=%q sentinel log = %q, want %q; output=%s", tc.goflags, tc.test, gotSentinel, tc.wantSentinel, out)
			}
			if tc.wantSentinel == "" && gotSentinel != "" {
				t.Fatalf("GOFLAGS=%q test=%q ran an execution-expanding target: sentinel log = %q; output=%s", tc.goflags, tc.test, gotSentinel, out)
			}
		})
	}
}

func TestFastCheckNeutralizesExecAndCPUControls(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import (
	"os"
	"testing"
)

func TestFastCheckSelected(t *testing.T) {
	path := os.Getenv("TOOLING_FAST_SENTINEL_LOG")
	if path == "" {
		t.Fatal("TOOLING_FAST_SENTINEL_LOG is not set")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("selected-test\n"); err != nil {
		t.Fatal(err)
	}
	t.Fatal("selected-test-ran")
}
`, 0600)
	sentinelLog := filepath.Join(root, "fast-check-sentinel.log")
	for _, tc := range []struct {
		name, goflags, wantSentinel string
		wantErr                     bool
	}{
		{
			name:         "exec cannot bypass selected test",
			goflags:      "-exec=true",
			wantErr:      true,
			wantSentinel: "selected-test\n",
		},
		{
			name:         "cpu list cannot repeat selected test",
			goflags:      "-cpu=1,2",
			wantErr:      true,
			wantSentinel: "selected-test\n",
		},
		{
			name:         "dry run cannot satisfy execution evidence",
			goflags:      "-n",
			wantErr:      true,
			wantSentinel: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(sentinelLog, nil, 0600); err != nil {
				t.Fatal(err)
			}
			out, err := toolingRun(t, root, []string{
				"GOFLAGS=" + tc.goflags,
				"TOOLING_FAST_SENTINEL_LOG=" + sentinelLog,
			}, "make",
				"FAST_MODULE=.",
				"FAST_PACKAGE=./cmd/gh-runnerd",
				"FAST_TEST=^TestFastCheckSelected$$",
				"fast",
			)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GOFLAGS=%q returned err=%v, want error=%t; output=%s", tc.goflags, err, tc.wantErr, out)
			}
			if got := toolingReadFile(t, sentinelLog); got != tc.wantSentinel {
				t.Fatalf("GOFLAGS=%q sentinel log = %q, want %q; output=%s", tc.goflags, got, tc.wantSentinel, out)
			}
		})
	}
}

func TestFastCheckIgnoresTestMainOutputForMatchEvidence(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("ok\tfrom TestMain")
	os.Exit(m.Run())
}
`, 0600)
	out, err := toolingRun(t, root, []string{"GOFLAGS=-v"}, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./cmd/gh-runnerd",
		"FAST_TEST=^NoSuchTest$$",
		"fast",
	)
	if err == nil || !strings.Contains(out, "FAST_TEST matched no compiled test") {
		t.Fatalf("TestMain output was treated as selected-test evidence: err=%v output=%s", err, out)
	}
}

func TestFastCheckPreservesRaceGOFLAGS(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import "testing"

var fastCheckRaceValue int

func TestFastCheckRaceTarget(t *testing.T) {
	start := make(chan struct{})
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			for j := 0; j < 1000; j++ {
				fastCheckRaceValue = j
			}
			done <- struct{}{}
		}()
	}
	close(start)
	<-done
	<-done
}
`, 0600)
	out, err := toolingRun(t, root, []string{"GOFLAGS=-race"}, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./cmd/gh-runnerd",
		"FAST_TEST=^TestFastCheckRaceTarget$$",
		"fast",
	)
	if err == nil || !strings.Contains(out, "DATA RACE") {
		t.Fatalf("GOFLAGS=-race was not preserved by fast check: err=%v output=%s", err, out)
	}
}

func TestFastCheckResolvesPackageSymlinks(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "linked/selected/fast_check_test.go", `package selected

import "testing"

func TestFastSymlinkSelected(t *testing.T) {}
`, 0600)
	toolingFile(t, root, "linked/other/fast_check_test.go", `package other

import "testing"

func TestFastSymlinkNotSelected(t *testing.T) {
	t.Fatal("wrong test ran")
}
`, 0600)
	toolingFile(t, root, "linked/selected/capacity/fast_check_test.go", `package capacity

import "testing"

func TestFastWildcardSelected(t *testing.T) {}
`, 0600)
	if err := os.Symlink("../linked", filepath.Join(root, "scripts", "package-link")); err != nil {
		t.Fatal(err)
	}
	out, err := toolingRun(t, root, nil, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./scripts/package-link/...",
		"FAST_TEST=^TestFastSymlinkSelected$$",
		"fast",
	)
	if err != nil {
		t.Fatalf("valid in-module package symlink failed: %s", out)
	}
	out, err = toolingRun(t, root, nil, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./.../capacity",
		"FAST_TEST=^TestFastWildcardSelected$$",
		"fast",
	)
	if err != nil {
		t.Fatalf("valid general package wildcard failed: %s", out)
	}

	outside := t.TempDir()
	toolingFile(t, outside, "escape_test.go", `package external

import "testing"

func TestFastSymlinkEscape(t *testing.T) {}
`, 0600)
	if err := os.Symlink(outside, filepath.Join(root, "scripts", "outside-link")); err != nil {
		t.Fatal(err)
	}
	out, err = toolingRun(t, root, nil, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./scripts/outside-link",
		"FAST_TEST=^TestFastSymlinkEscape$$",
		"fast",
	)
	if err == nil || !strings.Contains(out, "FAST_PACKAGE must resolve inside selected module") {
		t.Fatalf("package symlink escaping module was accepted: err=%v output=%s", err, out)
	}
}

func TestFastCheckRunsOnlyTheExplicitSelector(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import "testing"

func TestFastSelected(t *testing.T) {}

func TestFastNotSelected(t *testing.T) {
	t.Fatal("fast check ran an unselected test")
}
`, 0600)
	toolingFile(t, root, "internal/no_tests/empty.go", "package no_tests\n", 0600)
	toolingFile(t, root, "experiments/g01-scaleset/fast_check_test.go", `package fixture

import "testing"

func TestNestedFastSelected(t *testing.T) {}
`, 0600)

	wrapper, logPath, realGo := toolingGoWrapper(t, root)
	env := []string{"GO=" + wrapper, "TOOLING_REAL_GO=" + realGo, "TOOLING_GO_LOG=" + logPath}
	for _, tc := range []struct {
		name, module, packagePath, test, invocation string
	}{
		{
			name:        "root module",
			module:      ".",
			packagePath: "./...",
			test:        "^TestFastSelected$",
			invocation:  "go1.27.3\tlist -json=false -f {{.Dir}} ./...\ngo1.27.3\ttest -v -json=false -list= -bench= -fuzz= -skip= -c=false -count=1 -cpu=1 -exec= -run ^TestFastSelected$ ./... -args -test.v=test2json",
		},
		{
			name:        "nested module",
			module:      "./experiments/g01-scaleset",
			packagePath: ".",
			test:        "^TestNestedFastSelected$",
			invocation:  "go1.27.3\tlist -json=false -f {{.Dir}} .\ngo1.27.3\ttest -v -json=false -list= -bench= -fuzz= -skip= -c=false -count=1 -cpu=1 -exec= -run ^TestNestedFastSelected$ . -args -test.v=test2json",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(logPath, nil, 0600); err != nil {
				t.Fatal(err)
			}
			out, err := toolingRun(t, root, env, "make",
				"FAST_MODULE="+tc.module,
				"FAST_PACKAGE="+tc.packagePath,
				"FAST_TEST="+strings.ReplaceAll(tc.test, "$", "$$"),
				"fast",
			)
			if err != nil {
				t.Fatalf("focused selector failed: %s", out)
			}
			if !strings.Contains(out, "focused selector only") || !strings.Contains(out, "complete public validation remains make check/CI") {
				t.Fatalf("focused selector output did not identify its bounded scope: %s", out)
			}
			log := strings.TrimSpace(toolingReadFile(t, logPath))
			if log != tc.invocation {
				t.Fatalf("focused selector invocation = %q, want %q", log, tc.invocation)
			}
		})
	}
}

func TestFastCheckResolvesRelativeGoOverride(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "experiments/g01-scaleset/fast_check_test.go", `package fixture

import "testing"

func TestNestedFastSelected(t *testing.T) {}
`, 0600)
	wrapper, logPath, realGo := toolingGoWrapper(t, root)
	data, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	toolingFile(t, root, "tools/go", string(data), 0700)
	out, err := toolingRun(t, root, []string{
		"GO=./tools/go",
		"TOOLING_REAL_GO=" + realGo,
		"TOOLING_GO_LOG=" + logPath,
	}, "make",
		"FAST_MODULE=./experiments/g01-scaleset",
		"FAST_PACKAGE=.",
		"FAST_TEST=^TestNestedFastSelected$$",
		"fast",
	)
	if err != nil {
		t.Fatalf("relative Go override failed after changing directories: %s", out)
	}
}

func TestFastCheckDisablesDependencyExpansionForPackageResolution(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/main.go", `package main

import "fmt"

var _ = fmt.Sprint

func main() {}
`, 0600)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import "testing"

func TestFastSelected(t *testing.T) {}
`, 0600)
	out, err := toolingRun(t, root, []string{"GOFLAGS=-deps"}, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./cmd/gh-runnerd",
		"FAST_TEST=^TestFastSelected$$",
		"fast",
	)
	if err != nil {
		t.Fatalf("dependency expansion escaped selected module during package resolution: %s", out)
	}
}

func TestFastCheckRequiresExplicitSelectors(t *testing.T) {
	root := toolingFixture(t)
	fastCheck, err := filepath.Abs("fast-check.sh")
	if err != nil {
		t.Fatal(err)
	}
	assertFastCheckRequiresExplicitSelectors(t, root, fastCheck)

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := toolingRun(t, repoRoot, nil, "make",
		"FAST_MODULE=.",
		"FAST_PACKAGE=./scripts",
		"FAST_TEST=^TestFastCheckRequiresExplicitSelectorsUnderMake$$",
		"fast",
	); err != nil {
		t.Fatalf("make fast negative-selector regression failed: %s", out)
	}
}

func TestFastCheckRequiresExplicitSelectorsUnderMake(t *testing.T) {
	root := toolingFixture(t)
	fastCheck, err := filepath.Abs("fast-check.sh")
	if err != nil {
		t.Fatal(err)
	}
	assertFastCheckRequiresExplicitSelectors(t, root, fastCheck)
}

func TestFastCheckRejectsInvalidSelectors(t *testing.T) {
	root := toolingFixture(t)
	toolingFile(t, root, "cmd/gh-runnerd/fast_check_test.go", `package main

import "testing"

func TestFastSelected(t *testing.T) {}
`, 0600)
	wrapper, logPath, realGo := toolingGoWrapper(t, root)
	env := []string{"GO=" + wrapper, "TOOLING_REAL_GO=" + realGo, "TOOLING_GO_LOG=" + logPath}
	for _, tc := range []struct {
		name, module, packagePath, test string
	}{
		{name: "missing module directory", module: "./missing", packagePath: "./cmd/gh-runnerd", test: "^TestFastSelected$"},
		{name: "module without go.mod", module: "./cmd/gh-runnerd", packagePath: "./", test: "^TestFastSelected$"},
		{name: "escaping module path", module: "./../", packagePath: "./cmd/gh-runnerd", test: "^TestFastSelected$"},
		{name: "missing package directory", module: ".", packagePath: "./missing", test: "^TestFastSelected$"},
		{name: "escaping package path", module: ".", packagePath: "./../outside", test: "^TestFastSelected$"},
		{name: "no matching test", module: ".", packagePath: "./cmd/gh-runnerd", test: "^NoSuchTest$"},
		{name: "malformed test regexp", module: ".", packagePath: "./cmd/gh-runnerd", test: "["},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(logPath, nil, 0600); err != nil {
				t.Fatal(err)
			}
			out, err := toolingRun(t, root, env, "make",
				"FAST_MODULE="+tc.module,
				"FAST_PACKAGE="+tc.packagePath,
				"FAST_TEST="+strings.ReplaceAll(tc.test, "$", "$$"),
				"fast",
			)
			if err == nil || !strings.Contains(out, "fast check failed") {
				t.Fatalf("invalid %s was accepted: err=%v output=%s", tc.name, err, out)
			}
		})
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

func toolingActiveYAMLLine(line string) string {
	line = strings.TrimRight(line, "\r\n")
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return ""
	}
	if comment := strings.Index(line, " #"); comment >= 0 {
		line = line[:comment]
	}
	if comment := strings.Index(line, "\t#"); comment >= 0 {
		line = line[:comment]
	}
	return strings.TrimRight(line, " \t")
}

func toolingActiveYAMLLineStart(block, wanted string) int {
	offset := 0
	for _, line := range strings.SplitAfter(block, "\n") {
		if line == "" {
			continue
		}
		if toolingActiveYAMLLine(line) == wanted {
			return offset
		}
		offset += len(line)
	}
	return -1
}

func toolingHasActiveYAMLLine(block, wanted string) bool {
	return toolingActiveYAMLLineStart(block, wanted) >= 0
}

func toolingG01CanaryMappingEntry(line string) (string, string, bool) {
	key, value, found := strings.Cut(strings.TrimSpace(line), ":")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if len(key) >= 2 && key[0] == '"' && key[len(key)-1] == '"' {
		decoded, err := strconv.Unquote(key)
		if err != nil {
			return "", "", false
		}
		key = decoded
	} else if len(key) >= 2 && key[0] == '\'' && key[len(key)-1] == '\'' {
		key = key[1 : len(key)-1]
	}
	return key, strings.TrimSpace(value), true
}

func toolingG01CanaryDispatchStart(workflow string) (int, int, bool) {
	onFound := false
	onChildIndent := -1
	offset := 0
	for _, rawLine := range strings.SplitAfter(workflow, "\n") {
		line := toolingG01CanaryActiveYAMLLine(rawLine)
		if strings.TrimSpace(line) == "" {
			offset += len(rawLine)
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		key, value, found := toolingG01CanaryMappingEntry(line)
		if !onFound {
			if indent == 0 && found && key == "on" {
				if value != "" {
					return 0, 0, false
				}
				onFound = true
			}
			offset += len(rawLine)
			continue
		}
		if indent == 0 {
			return 0, 0, false
		}
		if onChildIndent < 0 {
			onChildIndent = indent
		}
		if indent == onChildIndent && found && key == "workflow_dispatch" {
			if value != "" {
				return 0, 0, false
			}
			return offset, indent, true
		}
		offset += len(rawLine)
	}
	return 0, 0, false
}

func toolingG01CanaryWorkflowPhaseAgnostic(workflow string) bool {
	dispatchStart, dispatchIndent, found := toolingG01CanaryDispatchStart(workflow)
	if !found {
		return false
	}

	dispatch := workflow[dispatchStart:]
	for _, rawLine := range strings.Split(dispatch, "\n")[1:] {
		line := toolingG01CanaryActiveYAMLLine(rawLine)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent <= dispatchIndent {
			break
		}
		// This canary forbids all dispatch inputs; do not partially parse YAML child keys.
		return false
	}

	if toolingG01CanaryHasPhaseInterpolation(workflow) {
		return false
	}
	return true
}

func TestG01CanaryWorkflowIsPhaseAgnostic(t *testing.T) {
	workflowData, err := os.ReadFile("../experiments/g01-canary-assets/canary.yml.template")
	if err != nil {
		t.Fatal(err)
	}
	if !toolingG01CanaryWorkflowPhaseAgnostic(string(workflowData)) {
		t.Fatal("G01 canary template must keep workflow_dispatch without inputs or phase interpolation")
	}
}

func TestToolingG01CanaryPhaseDetectorHandlesQuotedKeysAndComments(t *testing.T) {
	workflowDispatchJobWithoutTrigger := `on:
  push:
jobs:
  workflow_dispatch:
    runs-on: ubuntu-latest
    steps: []`
	if toolingG01CanaryWorkflowPhaseAgnostic(workflowDispatchJobWithoutTrigger) {
		t.Error("G01 canary contract accepted a job identifier as a workflow_dispatch trigger")
	}

	quotedPhaseInput := `on:
  workflow_dispatch:
    inputs:
      "phase":
        required: true
        type: choice`
	if toolingG01CanaryWorkflowPhaseAgnostic(quotedPhaseInput) {
		t.Error("G01 canary contract accepted a quoted phase input key")
	}

	escapedQuotedPhaseInput := `on:
  workflow_dispatch:
    "inp\u0075ts":
      phase:
        required: true
        type: choice`
	if toolingG01CanaryWorkflowPhaseAgnostic(escapedQuotedPhaseInput) {
		t.Error("G01 canary contract accepted a YAML-escaped quoted inputs key")
	}

	explicitPhaseInputKey := `on:
  workflow_dispatch:
    ? inputs
    :
      phase:
        required: true
        type: choice`
	if toolingG01CanaryWorkflowPhaseAgnostic(explicitPhaseInputKey) {
		t.Error("G01 canary contract accepted a phase input with an explicit YAML mapping key")
	}

	mergedPhaseInputs := `on:
  workflow_call: &shared_phase_inputs
    inputs:
      phase:
        required: true
        type: string
  workflow_dispatch:
    <<: *shared_phase_inputs`
	if toolingG01CanaryWorkflowPhaseAgnostic(mergedPhaseInputs) {
		t.Error("G01 canary contract accepted phase inputs merged through a YAML alias")
	}

	aliasedPhaseInputKey := `name: &phase_input_key inputs
on:
  workflow_dispatch:
    *phase_input_key:
      phase:
        required: true
        type: choice`
	if toolingG01CanaryWorkflowPhaseAgnostic(aliasedPhaseInputKey) {
		t.Error("G01 canary contract accepted a phase input whose YAML mapping key is an alias")
	}

	deeplyIndentedPhaseInput := `on:
  workflow_dispatch:
      inputs:
        phase:
          required: true
          type: choice`
	if toolingG01CanaryWorkflowPhaseAgnostic(deeplyIndentedPhaseInput) {
		t.Error("G01 canary contract accepted a phase input at a valid non-default indentation")
	}

	commentedInterpolation := `# run-name: G01 ${{ inputs.phase }}
on:
  workflow_dispatch:`
	if !toolingG01CanaryWorkflowPhaseAgnostic(commentedInterpolation) {
		t.Error("G01 canary contract treated a commented phase interpolation as active YAML")
	}

	quotedScalarInterpolation := `run-name: "G01 # ${{ inputs.phase }}"
on:
  workflow_dispatch:`
	if toolingG01CanaryWorkflowPhaseAgnostic(quotedScalarInterpolation) {
		t.Error("G01 canary contract missed phase interpolation after # inside a quoted scalar")
	}

	blockScalarInterpolation := `on:
  workflow_dispatch:
run: |
  # ${{ inputs.phase }}`
	if toolingG01CanaryWorkflowPhaseAgnostic(blockScalarInterpolation) {
		t.Error("G01 canary contract missed phase interpolation inside a YAML block scalar")
	}

	nestedBraceInterpolation := `run-name: ${{ format('{0}', inputs.phase) }}
on:
  workflow_dispatch:`
	if toolingG01CanaryWorkflowPhaseAgnostic(nestedBraceInterpolation) {
		t.Error("G01 canary contract missed a phase reference after a nested expression brace")
	}

	bracketStyleContextInterpolation := `run-name: ${{ github['event']['inputs']['phase'] }}
on:
  workflow_dispatch:`
	if toolingG01CanaryWorkflowPhaseAgnostic(bracketStyleContextInterpolation) {
		t.Error("G01 canary contract missed a phase reference through bracket-style context paths")
	}

	multilineInterpolation := `run-name: "${{
  inputs.phase
}}"
on:
  workflow_dispatch:`
	if toolingG01CanaryWorkflowPhaseAgnostic(multilineInterpolation) {
		t.Error("G01 canary contract missed a phase interpolation folded across quoted YAML lines")
	}

	multilineQuotedCommentInterpolation := `run-name: "G01
  # ${{ inputs.phase }}"
on:
  workflow_dispatch:`
	if toolingG01CanaryWorkflowPhaseAgnostic(multilineQuotedCommentInterpolation) {
		t.Error("G01 canary contract treated a comment marker inside a multiline quoted scalar as YAML comment")
	}

	stringLiteralOnly := `run-name: ${{ format('{0}', 'inputs.phase') }}
on:
  workflow_dispatch:`
	if !toolingG01CanaryWorkflowPhaseAgnostic(stringLiteralOnly) {
		t.Error("G01 canary contract treated a string literal as a phase reference")
	}
}

func toolingG01CanaryActiveYAMLLine(line string) string {
	var quote byte
	var escaped bool
	return toolingG01CanaryActiveYAMLLineWithQuoteState(line, &quote, &escaped)
}

func toolingG01CanaryActiveYAMLLineWithQuoteState(line string, quote *byte, escaped *bool) string {
	line = strings.TrimRight(line, "\r\n")
	for i := 0; i < len(line); i++ {
		current := line[i]
		if *quote == '"' {
			if *escaped {
				*escaped = false
				continue
			}
			switch current {
			case '\\':
				*escaped = true
			case '"':
				*quote = 0
			}
			continue
		}
		if *quote == '\'' {
			if current == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				*quote = 0
			}
			continue
		}

		switch current {
		case '"':
			if i == 0 || strings.ContainsRune(" \t:[{,-", rune(line[i-1])) {
				*quote = '"'
			}
		case '\'':
			if i == 0 || strings.ContainsRune(" \t:[{,-", rune(line[i-1])) {
				*quote = '\''
			}
		case '#':
			if i == 0 || line[i-1] == ' ' || line[i-1] == '\t' {
				return strings.TrimRight(line[:i], " \t")
			}
		}
	}
	return strings.TrimRight(line, " \t")
}

func toolingG01CanaryBlockScalarIndicator(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	indicator := fields[len(fields)-1]
	if indicator[0] != '|' && indicator[0] != '>' {
		return false
	}
	for _, char := range indicator[1:] {
		if char != '+' && char != '-' && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

func toolingG01CanaryWordByte(char byte) bool {
	return char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
}

func toolingG01CanaryExpressionPathSegment(expression string, start int, segment string) (int, bool) {
	for start < len(expression) && (expression[start] == ' ' || expression[start] == '\t' || expression[start] == '\n') {
		start++
	}
	if start < len(expression) && expression[start] == '.' {
		start++
		for start < len(expression) && (expression[start] == ' ' || expression[start] == '\t' || expression[start] == '\n') {
			start++
		}
		end := start + len(segment)
		if end > len(expression) || expression[start:end] != segment || (end < len(expression) && toolingG01CanaryWordByte(expression[end])) {
			return start, false
		}
		return end, true
	}
	if start < len(expression) && expression[start] == '[' {
		start++
		for start < len(expression) && (expression[start] == ' ' || expression[start] == '\t' || expression[start] == '\n') {
			start++
		}
		value, end, ok := toolingG01CanaryReadExpressionString(expression, start)
		if !ok || value != segment {
			return start, false
		}
		for end < len(expression) && (expression[end] == ' ' || expression[end] == '\t' || expression[end] == '\n') {
			end++
		}
		if end >= len(expression) || expression[end] != ']' {
			return end, false
		}
		return end + 1, true
	}
	return start, false
}

func toolingG01CanaryGitHubEventInputPhase(expression string, start int) bool {
	const context = "github"
	if !strings.HasPrefix(expression[start:], context) {
		return false
	}
	next := start + len(context)
	if next < len(expression) && toolingG01CanaryWordByte(expression[next]) {
		return false
	}
	for _, segment := range []string{"event", "inputs", "phase"} {
		var ok bool
		next, ok = toolingG01CanaryExpressionPathSegment(expression, next, segment)
		if !ok {
			return false
		}
	}
	return true
}

func toolingG01CanaryReadExpressionString(source string, start int) (string, int, bool) {
	if start >= len(source) || (source[start] != '\'' && source[start] != '"') {
		return "", start, false
	}
	quote := source[start]
	var value strings.Builder
	for i := start + 1; i < len(source); {
		char := source[i]
		if quote == '"' && char == '\\' && i+1 < len(source) {
			value.WriteByte(source[i+1])
			i += 2
			continue
		}
		if char == quote {
			if quote == '\'' && i+1 < len(source) && source[i+1] == '\'' {
				value.WriteByte('\'')
				i += 2
				continue
			}
			return value.String(), i + 1, true
		}
		value.WriteByte(char)
		i++
	}
	return "", len(source), false
}

func toolingG01CanaryExpressionReferencesPhase(expression string) bool {
	for i := 0; i < len(expression); {
		if expression[i] == '\'' || expression[i] == '"' {
			_, next, ok := toolingG01CanaryReadExpressionString(expression, i)
			if !ok {
				return false
			}
			i = next
			continue
		}
		if (i == 0 || !toolingG01CanaryWordByte(expression[i-1])) && toolingG01CanaryGitHubEventInputPhase(expression, i) {
			return true
		}
		if !strings.HasPrefix(expression[i:], "inputs") || (i > 0 && toolingG01CanaryWordByte(expression[i-1])) {
			i++
			continue
		}
		next := i + len("inputs")
		if next < len(expression) && toolingG01CanaryWordByte(expression[next]) {
			i++
			continue
		}
		for next < len(expression) && (expression[next] == ' ' || expression[next] == '\t' || expression[next] == '\n') {
			next++
		}
		if next < len(expression) && expression[next] == '.' {
			next++
			for next < len(expression) && (expression[next] == ' ' || expression[next] == '\t' || expression[next] == '\n') {
				next++
			}
			phaseEnd := next + len("phase")
			if phaseEnd <= len(expression) && expression[next:phaseEnd] == "phase" && (phaseEnd == len(expression) || !toolingG01CanaryWordByte(expression[phaseEnd])) {
				return true
			}
		} else if next < len(expression) && expression[next] == '[' {
			next++
			for next < len(expression) && (expression[next] == ' ' || expression[next] == '\t' || expression[next] == '\n') {
				next++
			}
			value, afterValue, ok := toolingG01CanaryReadExpressionString(expression, next)
			if ok && value == "phase" {
				next = afterValue
				for next < len(expression) && (expression[next] == ' ' || expression[next] == '\t' || expression[next] == '\n') {
					next++
				}
				if next < len(expression) && expression[next] == ']' {
					return true
				}
			}
		}
		i++
	}
	return false
}

func toolingG01CanaryExpressionEnd(line string, start int) int {
	for i := start; i < len(line); {
		if line[i] == '\'' || line[i] == '"' {
			_, next, ok := toolingG01CanaryReadExpressionString(line, i)
			if !ok {
				return -1
			}
			i = next
			continue
		}
		if line[i] == '}' && i+1 < len(line) && line[i+1] == '}' {
			return i
		}
		i++
	}
	return -1
}

func toolingG01CanaryLineHasPhaseInterpolation(line string) bool {
	for offset := 0; offset < len(line); {
		start := strings.Index(line[offset:], "${{")
		if start < 0 {
			return false
		}
		bodyStart := offset + start + len("${{")
		bodyEnd := toolingG01CanaryExpressionEnd(line, bodyStart)
		if bodyEnd < 0 {
			return toolingG01CanaryExpressionReferencesPhase(line[bodyStart:])
		}
		if toolingG01CanaryExpressionReferencesPhase(line[bodyStart:bodyEnd]) {
			return true
		}
		offset = bodyEnd + len("}}")
	}
	return false
}

func toolingG01CanaryHasPhaseInterpolation(workflow string) bool {
	blockIndent := -1
	var yamlQuote byte
	var yamlEscaped bool
	var activeDocument strings.Builder
	for _, rawLine := range strings.Split(workflow, "\n") {
		line := strings.TrimRight(rawLine, "\r\n")
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if blockIndent >= 0 {
			if strings.TrimSpace(line) == "" || indent > blockIndent {
				activeDocument.WriteString(line)
				activeDocument.WriteByte('\n')
				continue
			}
			blockIndent = -1
		}

		activeLine := toolingG01CanaryActiveYAMLLineWithQuoteState(line, &yamlQuote, &yamlEscaped)
		activeDocument.WriteString(activeLine)
		activeDocument.WriteByte('\n')
		if toolingG01CanaryBlockScalarIndicator(activeLine) {
			blockIndent = indent
		}
	}
	return toolingG01CanaryLineHasPhaseInterpolation(activeDocument.String())
}

func toolingYAMLDirectMappingLines(document, parent string) []string {
	parentIndent := -1
	var entries []string
	for _, rawLine := range strings.Split(document, "\n") {
		line := toolingActiveYAMLLine(rawLine)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if parentIndent < 0 {
			if indent == 0 && strings.TrimSpace(line) == parent+":" {
				parentIndent = indent
			}
			continue
		}
		if indent <= parentIndent {
			break
		}
		if indent == parentIndent+2 {
			entries = append(entries, strings.TrimSpace(line))
		}
	}
	return entries
}

func toolingPublicWorkflowTriggersValid(workflow string) bool {
	events := toolingYAMLDirectMappingLines(workflow, "on")
	if len(events) != 2 {
		return false
	}
	push, dispatch := false, false
	for _, event := range events {
		key, _, found := strings.Cut(event, ":")
		if !found {
			return false
		}
		switch strings.TrimSpace(key) {
		case "push":
			push = true
		case "workflow_dispatch":
			dispatch = true
		default:
			return false
		}
	}
	return push && dispatch
}

func toolingPRWorkflowCancelsInProgress(workflow string) bool {
	for _, entry := range toolingYAMLDirectMappingLines(workflow, "concurrency") {
		key, value, found := strings.Cut(entry, ":")
		if found && strings.TrimSpace(key) == "cancel-in-progress" && strings.TrimSpace(value) == "true" {
			return true
		}
	}
	return false
}

func TestToolingPublicWorkflowTriggersRejectExtraEvents(t *testing.T) {
	workflow := `name: Public CI
on:
  push:
  workflow_dispatch:
  schedule:
    - cron: '0 0 * * *'`
	if toolingPublicWorkflowTriggersValid(workflow) {
		t.Fatal("Public CI trigger contract accepted the extra schedule event")
	}
}

func TestToolingPRWorkflowCancellationIgnoresCommentedTrue(t *testing.T) {
	workflow := `name: Pull Request Checks
concurrency:
  group: pr-checks
  # cancel-in-progress: true
  cancel-in-progress: false`
	if toolingPRWorkflowCancelsInProgress(workflow) {
		t.Fatal("PR quick cancellation contract accepted commented true with active false")
	}
}

func toolingWorkflowSequenceItems(t *testing.T, block, key string) []string {
	t.Helper()
	marker := "    " + key + ":"
	lines := strings.Split(block, "\n")
	start := -1
	for i, line := range lines {
		if toolingActiveYAMLLine(line) == marker {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("workflow block is missing %q sequence", key)
	}
	var items []string
	for _, line := range lines[start:] {
		line = toolingActiveYAMLLine(line)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent <= 4 {
			break
		}
		if strings.HasPrefix(line, "      - ") {
			items = append(items, strings.Trim(strings.TrimPrefix(line, "      - "), "'\""))
		}
	}
	if len(items) == 0 {
		t.Fatalf("workflow %q sequence is empty", key)
	}
	return items
}

func TestToolingWorkflowSequenceItemsIgnoresCommentedYAML(t *testing.T) {
	block := `#     branches:
#       - fake-commented-branch
    branches:
      - main
      # - fake-commented-item
      - release
    paths:
      - "**.go"`

	got := toolingWorkflowSequenceItems(t, block, "branches")
	if strings.Join(got, ",") != "main,release" {
		t.Fatalf("workflow branches = %v, want [main release]", got)
	}
}

func TestToolingActiveYAMLLineIgnoresComments(t *testing.T) {
	fixture := `#   workflow_dispatch:
  workflow_dispatch: # active manual trigger
#   cancel-in-progress: true
  cancel-in-progress: false # preserve active run`
	for _, line := range []string{"  workflow_dispatch:", "  cancel-in-progress: false"} {
		if !toolingHasActiveYAMLLine(fixture, line) {
			t.Errorf("active YAML line %q was not found", line)
		}
	}
	for _, line := range []string{"  cancel-in-progress: true", "  workflow_dispatch: # commented out"} {
		if toolingHasActiveYAMLLine("#"+line, line) {
			t.Errorf("commented YAML line %q was treated as active", line)
		}
	}
}

func toolingContainsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestPublicWorkflowCapacityContract(t *testing.T) {
	workflowData, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowData)
	pushStart := toolingActiveYAMLLineStart(workflow, "  push:")
	dispatchStart := toolingActiveYAMLLineStart(workflow, "  workflow_dispatch:")
	if pushStart < 0 || dispatchStart < 0 || dispatchStart <= pushStart {
		t.Fatal("Public CI must keep push and workflow_dispatch triggers")
	}
	pushTrigger := workflow[pushStart:dispatchStart]
	branches := toolingWorkflowSequenceItems(t, pushTrigger, "branches")
	if len(branches) != 1 || branches[0] != "main" {
		t.Errorf("Public CI push branches = %v, want [main]", branches)
	}
	paths := toolingWorkflowSequenceItems(t, pushTrigger, "paths")
	expectedPaths := []string{
		"**.go",
		"go.mod",
		"go.sum",
		"go.work",
		"go.work.sum",
		"**/go.mod",
		"**/go.sum",
		"**/go.work",
		"**/go.work.sum",
		"LICENSE",
		"docs/DEPENDENCIES.md",
		"Makefile",
		"scripts/**",
		"experiments/**",
		".github/workflows/**",
	}
	for _, expectedPath := range expectedPaths {
		if !toolingContainsString(paths, expectedPath) {
			t.Errorf("Public CI push path filters omit %q", expectedPath)
		}
	}
	if len(paths) != len(expectedPaths) {
		t.Errorf("Public CI push path filters changed unexpectedly: got %v, want %v", paths, expectedPaths)
	}
	for _, path := range paths {
		if !toolingContainsString(expectedPaths, path) {
			t.Errorf("Public CI push path filters contain unexpected pattern %q", path)
		}
		if strings.HasPrefix(path, "docs/") && path != "docs/DEPENDENCIES.md" {
			t.Errorf("Public CI must filter docs-only paths other than docs/DEPENDENCIES.md; found %q", path)
		}
	}

	concurrencyStart := toolingActiveYAMLLineStart(workflow, "concurrency:")
	jobsStart := toolingActiveYAMLLineStart(workflow, "jobs:")
	if concurrencyStart < 0 || jobsStart <= concurrencyStart {
		t.Fatal("Public CI is missing its workflow concurrency block")
	}
	concurrency := workflow[concurrencyStart:jobsStart]
	if !toolingHasActiveYAMLLine(concurrency, "  group: public-ci-${{ github.workflow }}-${{ github.ref }}") {
		t.Error("Public CI must preserve its workflow/ref concurrency group")
	}
	if !toolingHasActiveYAMLLine(concurrency, "  cancel-in-progress: false") {
		t.Error("Public CI must not cancel an active run when a newer main push arrives")
	}
	if regexp.MustCompile(`(?m)^\s+queue:`).MatchString(concurrency) {
		t.Error("Public CI must keep the default one-running/one-pending concurrency queue")
	}

	jobs := toolingWorkflowJobs(t, workflow)
	requiredJobs := []string{"root", "race", "offline", "vuln", "checks"}
	for _, job := range requiredJobs {
		if _, ok := jobs[job]; !ok {
			t.Fatalf("workflow is missing required job %q; got jobs %v", job, toolingWorkflowJobNames(jobs))
		}
	}
	if len(jobs) != len(requiredJobs) {
		t.Fatalf("workflow has unexpected jobs: %v", toolingWorkflowJobNames(jobs))
	}

	commands := []string{
		"toolchain",
		"build",
		"fmt-check",
		"vet",
		"test",
		"test-race",
		"fuzz-smoke",
		"deps",
		"licenses",
		"experiments",
		"vuln",
	}
	for _, command := range commands {
		pattern := regexp.MustCompile(`(?m)^\s*run: make ` + regexp.QuoteMeta(command) + `$`)
		if got := len(pattern.FindAllString(workflow, -1)); got != 1 {
			t.Errorf("make %s appears %d times in hosted workflow, want exactly once", command, got)
		}
	}
	for _, command := range []string{"toolchain", "build", "fmt-check", "vet", "test", "fuzz-smoke", "deps", "licenses"} {
		if !strings.Contains(jobs["root"], "run: make "+command) {
			t.Errorf("root job omits make %s", command)
		}
	}
	if !strings.Contains(jobs["race"], "run: make test-race") {
		t.Error("race job omits make test-race")
	}
	if strings.Contains(jobs["root"], "run: make test-race") {
		t.Error("root job redundantly runs make test-race")
	}
	if !strings.Contains(jobs["offline"], "run: make experiments") {
		t.Error("offline job omits make experiments")
	}
	for _, module := range []string{"experiments/g01-scaleset", "experiments/g02-auth", "experiments/r1-credentials"} {
		if !strings.Contains(jobs["offline"], module) {
			t.Errorf("offline matrix omits %s", module)
		}
	}
	if !strings.Contains(jobs["offline"], "OFFLINE_EXPERIMENT_MODULE: ${{ matrix.module }}") {
		t.Error("offline matrix does not select its module")
	}
	const offlineMatrix = `      matrix:
        include:
          - module: experiments/g01-scaleset
            profile: core
          - module: experiments/g01-scaleset
            profile: jit
          - module: experiments/g02-auth
            profile: all
          - module: experiments/r1-credentials
            profile: all
`
	offline := jobs["offline"]
	matrixStart := strings.Index(offline, "      matrix:\n")
	envStart := strings.Index(offline, "    env:\n")
	if matrixStart < 0 || envStart < matrixStart || offline[matrixStart:envStart] != offlineMatrix {
		t.Error("offline matrix must contain exactly G01 core/jit, G02 all, and R1 all")
	}
	for _, line := range []string{
		"    name: Offline experiment checks (${{ matrix.module }}, ${{ matrix.profile }})",
		"      max-parallel: 3", "      fail-fast: false",
		"      OFFLINE_EXPERIMENT_PROFILE: ${{ matrix.profile }}",
	} {
		if !toolingHasActiveYAMLLine(offline, line) {
			t.Errorf("offline job must preserve %q", line)
		}
	}
	if strings.Contains(offline, "continue-on-error:") || strings.Contains(offline, "    if:") {
		t.Error("every offline matrix row must remain required")
	}
	if !strings.Contains(jobs["vuln"], "run: make vuln") {
		t.Error("vulnerability job omits make vuln")
	}

	const (
		checkout = "uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"
		setupGo  = "uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"
		mainHead = "ref: ${{ github.sha }}"
	)
	for _, job := range []string{"root", "race", "offline", "vuln"} {
		block := jobs[job]
		for _, required := range []string{checkout, setupGo, "timeout-minutes: 15", "persist-credentials: false", "cache: false", mainHead} {
			if !strings.Contains(block, required) {
				t.Errorf("job %s is missing %q", job, required)
			}
		}
	}
	if !strings.Contains(workflow, "permissions:\n  contents: read") {
		t.Error("hosted workflow must grant only read-only contents permission")
	}
	for _, forbidden := range []string{
		"contents: write",
		"pull_request_target:",
		"self-hosted",
		"secrets.",
		"actions/cache@",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("hosted workflow contains forbidden %q", forbidden)
		}
	}
	if !toolingPublicWorkflowTriggersValid(workflow) {
		t.Error("Public CI must run on main pushes/manual dispatch, not pull requests")
	}

	aggregator := jobs["checks"]
	if !strings.Contains(aggregator, "name: Go checks") {
		t.Error("required public check name Go checks is not preserved")
	}
	if !strings.Contains(aggregator, "needs: [root, race, offline, vuln]") {
		t.Error("Go checks aggregator does not depend on every required job")
	}
	if !strings.Contains(aggregator, "if: ${{ always() }}") {
		t.Error("Go checks aggregator must inspect dependencies after failure, cancellation, or skip")
	}
	for _, job := range []string{"root", "race", "offline", "vuln"} {
		if !strings.Contains(aggregator, "needs."+job+".result") {
			t.Errorf("Go checks aggregator does not inspect %s result", job)
		}
	}
	if !strings.Contains(aggregator, `[ "$result" != success ]`) || !strings.Contains(aggregator, "exit 1") {
		t.Error("Go checks aggregator does not fail when a required job is not successful")
	}
	if strings.Contains(aggregator, "continue-on-error: true") {
		t.Error("Go checks aggregator must not ignore a required-job failure")
	}
	if !strings.Contains(aggregator, "timeout-minutes: 15") {
		t.Error("Go checks aggregator must remain bounded to 15 minutes")
	}
	aggregatorScript := toolingWorkflowRunScript(t, aggregator)
	for _, tc := range []struct {
		name                      string
		root, race, offline, vuln string
		succeeds                  bool
	}{
		{name: "all success", root: "success", race: "success", offline: "success", vuln: "success", succeeds: true},
		{name: "root failure", root: "failure", race: "success", offline: "success", vuln: "success", succeeds: false},
		{name: "race cancellation", root: "success", race: "cancelled", offline: "success", vuln: "success", succeeds: false},
		{name: "offline cancellation", root: "success", race: "success", offline: "cancelled", vuln: "success", succeeds: false},
		{name: "vulnerability skipped", root: "success", race: "success", offline: "success", vuln: "skipped", succeeds: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := toolingRun(t, t.TempDir(), []string{
				"ROOT_RESULT=" + tc.root,
				"RACE_RESULT=" + tc.race,
				"OFFLINE_RESULT=" + tc.offline,
				"VULN_RESULT=" + tc.vuln,
			}, "bash", "-c", aggregatorScript)
			if got := err == nil; got != tc.succeeds {
				t.Fatalf("aggregator status with root=%s offline=%s vuln=%s: success=%t err=%v output=%s", tc.root, tc.offline, tc.vuln, got, err, out)
			}
		})
	}
}

func TestPullRequestQuickWorkflowContract(t *testing.T) {
	workflowData, err := os.ReadFile("../.github/workflows/pr-fast.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowData)
	for _, required := range []string{
		"name: Pull Request Checks",
		"name: Go checks",
		"  pull_request:",
		"types: [opened, synchronize, reopened]",
		"permissions:\n  contents: read",
		"run: git diff --check \"$BASE_SHA...$HEAD_SHA\"",
		"git diff --name-only --no-renames \"$BASE_SHA...$HEAD_SHA\"",
		"module_change_pattern='^(experiments/g01-scaleset/|experiments/g02-auth/handoff/|experiments/g02-auth/(go\\.mod|go\\.sum)$)'",
		"cd \"$module\" && GOTOOLCHAIN=\"$GOTOOLCHAIN\" go test -run '^$' -count=1 ./...",
		"go.work",
		"go.work.sum",
		"run: make toolchain",
		"run: make build",
		"run: make fmt-check",
		"run: make vet",
		"go test -run '^$' -count=1 ./...",
		"run: GOTOOLCHAIN=\"$GOTOOLCHAIN\" go test -run '^(TestPublicWorkflowCapacityContract|TestPullRequestQuickWorkflowContract|TestG01WorkflowModuleSelectionPredicate)$' -count=1 ./scripts",
		"run: make deps",
		"run: make licenses",
		"run: make vuln",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("PR quick workflow is missing %q", required)
		}
	}
	if !toolingPRWorkflowCancelsInProgress(workflow) {
		t.Error("PR quick workflow must cancel in-progress runs")
	}
	for _, forbidden := range []string{
		"run: make test\n",
		"run: make test-race\n",
		"run: make fuzz-smoke\n",
		"run: make experiments\n",
		"pull_request_target:",
		"self-hosted",
		"secrets.",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("PR quick workflow contains full/live check %q", forbidden)
		}
	}
}

func TestG01WorkflowModuleSelectionPredicate(t *testing.T) {
	workflowData, err := os.ReadFile("../.github/workflows/pr-fast.yml")
	if err != nil {
		t.Fatal(err)
	}
	assignment := regexp.MustCompile(`(?m)^\s*module_change_pattern='([^']+)'`).FindStringSubmatch(string(workflowData))
	if len(assignment) != 2 || assignment[1] == "" {
		t.Fatal("could not extract the G01 module_change_pattern assignment")
	}
	matches := func(path string) bool {
		t.Helper()
		_, err := toolingRun(t, "..", []string{
			"G01_MODULE_CHANGE_PATTERN=" + assignment[1],
			"G01_SYNTHETIC_PATH=" + path,
		}, "bash", "-c", `printf '%s\n' "$G01_SYNTHETIC_PATH" | grep -Eq "$G01_MODULE_CHANGE_PATTERN"`)
		return err == nil
	}
	for _, path := range []string{
		"experiments/g01-scaleset/livecanary/controller_handoff.go",
		"experiments/g02-auth/handoff/handoff.go",
		"experiments/g02-auth/go.mod",
		"experiments/g02-auth/go.sum",
	} {
		if !matches(path) {
			t.Errorf("G01 selection predicate rejected required path %q", path)
		}
	}
	for _, path := range []string{
		"experiments/g02-auth/broker.go",
		"experiments/r1-credentials/go.mod",
		"docs/EXECUTION.md",
		"experiments/g02-auth/go.mod.bak",
		"experiments/g02-auth/go.sum.sig",
		"experiments/g02-auth/go.mod/nested",
		"experiments/g02-auth/go.sum/nested",
		"experiments/g02-auth/nested/go.mod",
	} {
		if matches(path) {
			t.Errorf("G01 selection predicate accepted forbidden path %q", path)
		}
	}
}

func TestG01SDKComparisonDropsUnusedLocalHandoffModuleEdge(t *testing.T) {
	scriptData, err := os.ReadFile("../experiments/g01-scaleset/compare-sdk.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptData)
	dropEdge := "go mod edit -droprequire=github.com/1XP-AI/gh-runnerd/experiments/g02-auth -dropreplace=github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
	dropIndex := strings.Index(script, dropEdge)
	tidyIndex := strings.Index(script, "go mod tidy")
	if dropIndex < 0 || tidyIndex < 0 || dropIndex > tidyIndex {
		t.Fatal("SDK comparison must remove the unused local G02 edge before tidying its standalone copy")
	}
	if !strings.Contains(script, `go mod edit -require="github.com/actions/scaleset@$sdk_version"`) {
		t.Fatal("SDK comparison no longer selects the requested SDK version")
	}
}

func toolingWorkflowJobs(t *testing.T, workflow string) map[string]string {
	t.Helper()
	lines := strings.Split(workflow, "\n")
	jobsLine := -1
	for index, line := range lines {
		if line == "jobs:" {
			jobsLine = index
			break
		}
	}
	if jobsLine < 0 {
		t.Fatal("workflow is missing jobs mapping")
	}
	starts := make(map[string]int)
	var names []string
	for index := jobsLine + 1; index < len(lines); index++ {
		line := lines[index]
		if len(line) > 0 && line[0] != ' ' && strings.TrimSpace(line) != "" {
			break
		}
		if len(line)-len(strings.TrimLeft(line, " ")) != 2 {
			continue
		}
		name := strings.TrimSpace(line)
		if !strings.HasSuffix(name, ":") || strings.ContainsAny(name, " \t") {
			continue
		}
		name = strings.TrimSuffix(name, ":")
		starts[name] = index
		names = append(names, name)
	}
	if len(starts) == 0 {
		t.Fatal("workflow has no jobs")
	}
	blocks := make(map[string]string, len(starts))
	for index, name := range names {
		start := starts[name]
		end := len(lines)
		if index+1 < len(names) {
			end = starts[names[index+1]]
		}
		blocks[name] = strings.Join(lines[start:end], "\n")
	}
	return blocks
}

func toolingWorkflowJobNames(jobs map[string]string) []string {
	names := make([]string, 0, len(jobs))
	for name := range jobs {
		names = append(names, name)
	}
	return names
}

func toolingWorkflowRunScript(t *testing.T, job string) string {
	t.Helper()
	lines := strings.Split(job, "\n")
	for index, line := range lines {
		if line != "        run: |" {
			continue
		}
		var script []string
		for _, bodyLine := range lines[index+1:] {
			if bodyLine != "" && len(bodyLine)-len(strings.TrimLeft(bodyLine, " ")) <= 8 {
				break
			}
			if bodyLine == "" {
				script = append(script, "")
				continue
			}
			if !strings.HasPrefix(bodyLine, "          ") {
				t.Fatalf("workflow run body is not indented consistently: %q", bodyLine)
			}
			script = append(script, strings.TrimPrefix(bodyLine, "          "))
		}
		if len(script) == 0 {
			t.Fatal("workflow run block is empty")
		}
		return strings.Join(script, "\n")
	}
	t.Fatal("workflow job is missing a literal run block")
	return ""
}

func TestToolingEstablishedModulesAreRequired(t *testing.T) {
	for _, module := range []string{"g01-scaleset", "g02-auth", "r1-credentials"} {
		root := toolingFixture(t)
		if err := os.Rename(filepath.Join(root, "experiments", module, "go.mod"), filepath.Join(root, "experiments", module, "absent.mod")); err != nil {
			t.Fatal(err)
		}
		if out, err := toolingRun(t, root, nil, "bash", "scripts/check-offline-experiments.sh"); err == nil {
			t.Errorf("missing established %s passed: %s", module, out)
		}
	}
}

func TestToolingOfflineModuleSelection(t *testing.T) {
	root := toolingFixture(t)
	out, err := toolingRun(t, root, []string{"OFFLINE_EXPERIMENT_MODULE=experiments/r1-credentials"}, "bash", "scripts/check-offline-experiments.sh")
	if err != nil {
		t.Fatalf("selected offline module failed: %s", out)
	}
	if !strings.Contains(out, "offline experiment: experiments/r1-credentials") || !strings.Contains(out, "offline experiment checks passed: 1 module(s)") {
		t.Fatalf("selected offline module was not isolated: %s", out)
	}
	if strings.Contains(out, "g01-scaleset") || strings.Contains(out, "g02-auth") {
		t.Fatalf("selected offline module ran an unrelated module: %s", out)
	}
	if out, err := toolingRun(t, root, []string{"OFFLINE_EXPERIMENT_MODULE=unsupported"}, "bash", "scripts/check-offline-experiments.sh"); err == nil || !strings.Contains(out, "unsupported module") {
		t.Fatalf("unsupported offline module was accepted: %s", out)
	}
	if err := os.Rename(filepath.Join(root, "experiments", "g01-scaleset", "go.mod"), filepath.Join(root, "experiments", "g01-scaleset", "absent.mod")); err != nil {
		t.Fatal(err)
	}
	if out, err := toolingRun(t, root, []string{"OFFLINE_EXPERIMENT_MODULE=experiments/r1-credentials"}, "bash", "scripts/check-offline-experiments.sh"); err == nil || !strings.Contains(out, "required module experiments/g01-scaleset is missing") {
		t.Fatalf("selected offline module skipped inventory validation: %s", out)
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
	const terminalHeavySentinel = "tagged-pair-fixture-terminal-heavy-regression"
	const terminalFutureSentinel = "tagged-pair-fixture-terminal-future-regression"
	const storageSentinel = "tagged-pair-fixture-storage-regression"
	const storageJournalSentinel = "tagged-pair-fixture-storage-journal-regression"
	pairedCollectionPath := "experiments/g01-scaleset/livecanary/pair_fixture_paired_collection_regression_test.go"
	listenerPath := "experiments/g01-scaleset/livecanary/pair_fixture_listener_regression_test.go"
	examplePath := "experiments/g01-scaleset/livecanary/pair_fixture_example_regression_test.go"
	fuzzPath := "experiments/g01-scaleset/livecanary/pair_fixture_fuzz_regression_test.go"
	terminalPath := "experiments/g01-scaleset/livecanary/pair_fixture_terminal_regression_test.go"
	terminalHeavyPath := "experiments/g01-scaleset/livecanary/pair_fixture_terminal_heavy_regression_test.go"
	terminalFuturePath := "experiments/g01-scaleset/livecanary/pair_fixture_terminal_future_regression_test.go"
	storagePath := "experiments/g01-scaleset/livecanary/pair_fixture_storage_regression_test.go"
	storageJournalPath := "experiments/g01-scaleset/livecanary/pair_fixture_storage_journal_regression_test.go"
	remainingCollectionInvocation := "go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -skip ^Test(Paired|SessionReplacement|JITBoundary|ExperimentalAcquireLossCommand$|ExperimentalDrainCommand$|ExperimentalACKBoundaryCommand$) ./livecanary"
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
	terminalHeavyPositiveSource := taggedPositiveSource("TestPairedTerminalFinalResultCapacity", "terminal-heavy-pass")
	terminalFuturePositiveSource := taggedPositiveSource("TestPairedTerminalFutureCoverage", "terminal-future-pass")
	storagePositiveSource := taggedPositiveSource("TestPairedTerminalFixtureStorageFailure", "storage-pass")
	storageJournalPositiveSource := taggedPositiveSource("TestPairedTerminalPostIntentJournalIdentity", "storage-journal-pass")
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
	storageJournalSource := `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import "testing"

func TestPairedTerminalPostIntentJournalIdentity(t *testing.T) {
	t.Fatal("tagged-pair-fixture-storage-journal-regression")
}
`
	terminalHeavySource := `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import "testing"

func TestPairedTerminalFinalResultCapacity(t *testing.T) {
	t.Fatal("tagged-pair-fixture-terminal-heavy-regression")
}
`
	terminalFutureSource := `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import "testing"

func TestPairedTerminalFutureCoverage(t *testing.T) {
	t.Fatal("tagged-pair-fixture-terminal-future-regression")
}
`
	wrapper, logPath, realGo := toolingGoWrapper(t, root)
	sentinelLogPath := filepath.Join(root, "tagged-sentinel.log")
	env := []string{"GO=" + wrapper, "TOOLING_REAL_GO=" + realGo, "TOOLING_GO_LOG=" + logPath, "TOOLING_SENTINEL_LOG=" + sentinelLogPath}
	// Each generated witness must fail through its own reviewed partition. The
	// log assertions prove the bounded tagged livecanary partitions, including a future
	// TestPairedTerminal name in the complementary remainder.
	partitions := []struct {
		path, sentinel, name, source, positiveSource, invocation string
	}{
		{
			pairedCollectionPath,
			pairedCollectionSentinel,
			"paired-collection",
			pairedCollectionSource,
			pairedCollectionPositiveSource,
			"go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPaired -skip ^TestPairedTerminal ./livecanary",
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
			terminalHeavyPath,
			terminalHeavySentinel,
			"terminal-heavy",
			terminalHeavySource,
			terminalHeavyPositiveSource,
			g01TerminalHeavyInvocation,
		},
		{
			terminalPath,
			terminalSentinel,
			"terminal-remainder",
			terminalSource,
			terminalPositiveSource,
			g01TerminalRemainderInvocation,
		},
		{
			terminalFuturePath,
			terminalFutureSentinel,
			"terminal-future",
			terminalFutureSource,
			terminalFuturePositiveSource,
			g01TerminalRemainderInvocation,
		},
		{
			storageJournalPath,
			storageJournalSentinel,
			"storage-journal",
			storageJournalSource,
			storageJournalPositiveSource,
			g01StorageJournalInvocation,
		},
		{
			storagePath,
			storageSentinel,
			"storage",
			storageSource,
			storagePositiveSource,
			g01TerminalStorageInvocation,
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
	for _, marker := range []string{"paired-collection-pass", "listener-pass", "example-pass", "fuzz-pass", "terminal-heavy-pass", "terminal-pass", "terminal-future-pass", "storage-journal-pass", "storage-pass"} {
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
		"go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -run ^TestPaired -skip ^TestPairedTerminal ./livecanary",
		remainingCollectionInvocation,
		g01ReplacementInvocation,
		g01TerminalHeavyInvocation,
		g01TerminalRemainderInvocation,
		g01StorageJournalInvocation,
		g01TerminalStorageInvocation,
		"go1.27.3\tvet -tags=g01_pair_fixture ./livecanary",
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
	legacyCollectionInvocation := "go1.27.3\ttest -race -count=1 -timeout=120s -tags=g01_pair_fixture -skip ^TestPairedTerminal ./livecanary"
	for _, line := range lines {
		if line == legacyCollectionInvocation {
			t.Fatalf("offline gate retained the unsplit collection invocation: %s", line)
		}
		if line == g01LegacyTerminalInvocation {
			t.Fatalf("offline gate retained the unsplit terminal non-storage invocation: %s", line)
		}
		if line == g01LegacyStorageInvocation {
			t.Fatalf("offline gate retained the unsplit storage invocation: %s", line)
		}
	}
}

func TestG01PairedTerminalPartitionRegistry(t *testing.T) {
	script, err := os.ReadFile("check-offline-experiments.sh")
	if err != nil {
		t.Fatal(err)
	}
	body := string(script)
	heavy := toolingScriptAssignment(t, body, "terminal_heavy_regex")
	remainderSkip := toolingScriptAssignment(t, body, "terminal_remainder_skip_regex")
	storage := toolingScriptAssignment(t, body, "storage_regex")
	journal := toolingScriptAssignment(t, body, "storage_journal_regex")
	if heavy != g01TerminalHeavyRegex {
		t.Fatalf("terminal_heavy_regex = %q", heavy)
	}
	if remainderSkip != g01TerminalRemainderSkipRegex {
		t.Fatalf("terminal_remainder_skip_regex = %q", remainderSkip)
	}
	if storage != g01TerminalStorageRegex {
		t.Fatalf("storage_regex = %q", storage)
	}
	if journal != g01StorageJournalRegex {
		t.Fatalf("storage_journal_regex = %q", journal)
	}
	if !strings.Contains(body, `chmod 0700 "${g02_prep_dir}"`) || !strings.Contains(body, `trap 'trap - EXIT; rm -rf -- "${g02_prep_dir}"; exit 143' TERM`) {
		t.Fatal("G02 owned signal cleanup traps were removed")
	}
	all := toolingListLivecanary(t, "^TestPairedTerminal")
	heavyNames := toolingListLivecanary(t, heavy)
	storageNames := toolingListLivecanary(t, storage)
	journalNames := toolingListLivecanary(t, journal)
	heavySet := map[string]bool{}
	for _, name := range heavyNames {
		heavySet[name] = true
	}
	storageSet := map[string]bool{}
	for _, name := range storageNames {
		storageSet[name] = true
	}
	journalSet := map[string]bool{}
	for _, name := range journalNames {
		if !storageSet[name] {
			t.Fatalf("journal partition contains a non-storage test: %s", name)
		}
		journalSet[name] = true
	}
	var storageRemainderNames []string
	for _, name := range storageNames {
		if !journalSet[name] {
			storageRemainderNames = append(storageRemainderNames, name)
		}
	}
	var remainderNames []string
	for _, name := range all {
		if heavySet[name] || storageSet[name] {
			continue
		}
		remainderNames = append(remainderNames, name)
	}
	if out := toolingLivecanarySkipCheck(t, "^TestPairedTerminalFinalResultCapacity$", remainderSkip); !strings.Contains(out, "[no tests to run]") {
		t.Fatalf("remainder skip still selects a heavy name: %s", out)
	}
	if out := toolingLivecanarySkipCheck(t, "^TestPairedTerminalClosedReplayActualFile$", remainderSkip); !strings.Contains(out, "[no tests to run]") {
		t.Fatalf("remainder skip still selects a storage name: %s", out)
	}
	if len(all) != 26 {
		t.Fatalf("listed %d TestPairedTerminal names, want 26: %q", len(all), all)
	}
	if len(heavyNames) != 5 {
		t.Fatalf("heavy partition listed %d names, want 5: %q", len(heavyNames), heavyNames)
	}
	if len(remainderNames) != 15 {
		t.Fatalf("remainder partition listed %d names, want 15: %q", len(remainderNames), remainderNames)
	}
	if len(storageNames) != 6 {
		t.Fatalf("storage partition listed %d names, want 6: %q", len(storageNames), storageNames)
	}
	if len(journalNames) != 1 || len(storageRemainderNames) != 5 {
		t.Fatalf("storage split listed journal=%d remainder=%d, want 1 and 5", len(journalNames), len(storageRemainderNames))
	}
	if out := toolingLivecanarySkipCheck(t, journal, journal); !strings.Contains(out, "[no tests to run]") {
		t.Fatalf("storage skip still selects journal identity: %s", out)
	}
	seen := map[string]string{}
	assign := func(partition string, names []string) {
		t.Helper()
		for _, name := range names {
			if previous, ok := seen[name]; ok {
				t.Fatalf("%s selected in %s and %s", name, previous, partition)
			}
			seen[name] = partition
		}
	}
	assign("heavy", heavyNames)
	assign("remainder", remainderNames)
	assign("storage-journal", journalNames)
	assign("storage-remainder", storageRemainderNames)
	if len(seen) != 26 {
		t.Fatalf("partitions covered %d names, want 26", len(seen))
	}
	for _, name := range all {
		if _, ok := seen[name]; !ok {
			t.Fatalf("%s is not selected by any terminal partition", name)
		}
	}
	wantHeavy := []string{
		"TestPairedTerminalFinalResultCapacity",
		"TestPairedTerminalPendingChildCapacity",
		"TestPairedTerminalEligibilityUsesFreshExactFacts",
		"TestPairedTerminalCapturedAcknowledgementCancellation",
		"TestPairedTerminalMissingAcknowledgementsAndPostchecks",
	}
	if !toolingSameNames(heavyNames, wantHeavy) {
		t.Fatalf("heavy names = %q, want %q", heavyNames, wantHeavy)
	}
}

func toolingSameNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := map[string]int{}
	for _, name := range want {
		counts[name]++
	}
	for _, name := range got {
		counts[name]--
		if counts[name] < 0 {
			return false
		}
	}
	return true
}

func toolingScriptAssignment(t *testing.T, body, name string) string {
	t.Helper()
	prefix := name + "='"
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) && strings.HasSuffix(trimmed, "'") {
			return strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), "'")
		}
	}
	t.Fatalf("script missing %s assignment", name)
	return ""
}

func toolingListLivecanary(t *testing.T, run string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-tags=g01_pair_fixture", "-list", run, "./livecanary")
	cmd.Dir = filepath.Join("..", "experiments", "g01-scaleset")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.3", "GOFLAGS=", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list run=%q: %s", run, out)
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Test") {
			names = append(names, line)
		}
	}
	return names
}

func toolingLivecanarySkipCheck(t *testing.T, run, skip string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-timeout=15s", "-tags=g01_pair_fixture", "-run", run, "-skip", skip, "./livecanary")
	cmd.Dir = filepath.Join("..", "experiments", "g01-scaleset")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.3", "GOFLAGS=", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("skip check run=%q skip=%q: %s", run, skip, out)
	}
	return string(out)
}

func TestToolingDefaultG01PartitionsRun(t *testing.T) {
	root := toolingFixture(t)
	const heavyName = "TestBaselineStatisticsPresenceAndEligibility"
	const handoffName = "TestControllerHandoffFixture"
	const heavySentinel = "default-g01-heavy-regression"
	const handoffSentinel = "default-g01-handoff-regression"
	const remainderSentinel = "default-g01-remainder-regression"
	const otherPackageSentinel = "default-g01-other-package-regression"
	const sameNameOtherPackageSentinel = "default-g01-same-name-other-package-regression"
	const sameNameHandoffOtherPackageSentinel = "default-g01-same-name-handoff-other-package-regression"
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
	defaultHandoffTestSource := func(pkg, marker, failure string) string {
		failureLine := ""
		if failure != "" {
			failureLine = "\n\tt.Fatal(\"" + failure + "\")"
		}
		return `//go:build !g01_pair_fixture && !g01_live && !g01_worker

package ` + pkg + `

import (
	"os"
	"strings"
	"testing"
)

func ` + handoffName + `(t *testing.T) {
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.run=^TestControllerHandoff") {
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
			return
		}
	}
	t.Fatal("controller handoff fixture did not run in its isolated partition")
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
			name:           "handoff",
			path:           livecanaryBase + "/default_handoff_regression_test.go",
			marker:         handoffSentinel,
			positiveSource: defaultHandoffTestSource("livecanary", handoffSentinel, ""),
			failureSource:  defaultHandoffTestSource("livecanary", handoffSentinel, handoffSentinel),
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
			name:           "same-name handoff other package",
			path:           otherPackageBase + "/default_same_name_handoff_regression_test.go",
			marker:         sameNameHandoffOtherPackageSentinel,
			positiveSource: defaultHandoffTestSource("otherfixture", sameNameHandoffOtherPackageSentinel, ""),
			failureSource:  defaultHandoffTestSource("otherfixture", sameNameHandoffOtherPackageSentinel, sameNameHandoffOtherPackageSentinel),
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
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestBaselineStatisticsPresenceAndEligibility$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestControllerHandoff ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -skip ^Test(BaselineStatisticsPresenceAndEligibility|ControllerHandoff.*)$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerPrepareReviewedG01LiveBinary$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestPaired -skip ^TestPairedBroker(PrepareReviewedG01LiveBinary|RealCadenceChildExceedsThirtySeconds)$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -skip ^TestPaired ./...",
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
	r1Invocation := "go1.27.3\ttest -race -count=1 -timeout=45s ./..."
	r1Count := 0
	for _, line := range lines {
		if line == r1Invocation {
			r1Count++
		}
	}
	if r1Count != 1 {
		t.Fatalf("offline gate ran the R1 fixture invocation %d times; wrapper log:\n%s", r1Count, log)
	}
	for _, invocation := range []string{
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestPaired -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=120s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
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
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerPrepareReviewedG01LiveBinary$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestPaired -skip ^TestPairedBroker(PrepareReviewedG01LiveBinary|RealCadenceChildExceedsThirtySeconds)$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -skip ^TestPaired ./...",
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
		"go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestPaired -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=45s -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=120s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
		"go1.27.3\ttest -race -count=1 -timeout=120s -skip ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./...",
	} {
		for _, line := range lines {
			if line == invocation {
				t.Fatalf("offline gate retained forbidden G02 invocation %q; wrapper log:\n%s", invocation, log)
			}
		}
	}
	r1Invocation := "go1.27.3\ttest -race -count=1 -timeout=45s ./..."
	r1Count := 0
	for _, line := range lines {
		if line == r1Invocation {
			r1Count++
		}
	}
	if r1Count != 1 {
		t.Fatalf("offline gate ran the R1 fixture invocation %d times; wrapper log:\n%s", r1Count, log)
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
			cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.3", "GOFLAGS=", "GOWORK=off")
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
	cadence := "go1.27.3\ttest -race -count=1 -timeout=45s -run ^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$ ./..."
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
	toolingFile(t, root, "go.mod", "module example.test/audit\n\ngo 1.27.3\n\nrequire example.test/dependency v1.2.0\n\nreplace example.test/dependency => ./replacement\n", 0600)
	toolingFile(t, root, "replacement/go.mod", "module example.test/dependency\n\ngo 1.27.3\n", 0600)
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

func TestToolingExperimentalCommandHelpersOwnedRouting(t *testing.T) {
	for _, failPrep := range []bool{false, true} {
		t.Run(strconv.FormatBool(failPrep), func(t *testing.T) {
			root := toolingFixture(t)
			log := filepath.Join(root, "phase-routing.log")
			toolingFile(t, root, "phase-go", `#!/bin/sh
set -eu
case "$*" in
 *'TestExperimentalPrepareCommandHelpers$'*)
  test -n "${G01_PHASE_HELPER_DIR:-}"
  test -d "$G01_PHASE_HELPER_DIR"
  test "$G01_PHASE_CONTROLLER_TEST_BINARY" = "$G01_PHASE_HELPER_DIR/controller.test"
  test "$G01_PHASE_BROKER_TEST_BINARY" = "$G01_PHASE_HELPER_DIR/broker.test"
  printf 'prepare %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  if [ "$PHASE_FAIL_PREP" = 1 ]; then exit 29; fi
  printf '#!/bin/sh\nexit 0\n' > "$G01_PHASE_CONTROLLER_TEST_BINARY"
  cp "$G01_PHASE_CONTROLLER_TEST_BINARY" "$G01_PHASE_BROKER_TEST_BINARY"
  chmod 0700 "$G01_PHASE_CONTROLLER_TEST_BINARY" "$G01_PHASE_BROKER_TEST_BINARY"
  ;;
 *'-tags=g01_pair_fixture -run ^TestSessionReplacement ./livecanary'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  printf 'replacement %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
 *'-tags=g01_pair_fixture -run ^TestExperimentalAcquireLossCommand$ ./livecanary'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  printf 'acquire-loss %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
 *'-tags=g01_pair_fixture -run ^TestExperimentalDrainCommand$ ./livecanary'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  printf 'drain %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
 *'-tags=g01_pair_fixture -run ^TestExperimentalACKBoundaryCommand$ ./livecanary'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  printf 'ack %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
 *'-tags=g01_pair_fixture -run ^TestJITBoundaryStorageAndCancellationEveryAppend$ ./livecanary'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  printf 'jit-storage %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
 *'-tags=g01_pair_fixture -run ^TestJITBoundaryExpiryAtEveryAppend$ ./livecanary'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  printf 'jit-expiry %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
 *'-tags=g01_pair_fixture -run ^TestJITBoundary(PreparationRequiresOriginalFreshCreate|AtomicPrefixBeforeCredentialInput|ConcurrentConsumesOnce|HistoryCannotResumeOrRenew|StrictJournalSchemaAndRawReferences|ProofCannotCrossOpen|DirectRunAndCredentialShapes|CloneOwnsNestedPayload)$ ./livecanary'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  printf 'jit-admission %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
 *'-tags=g01_pair_fixture -run ^TestJITBoundaryPhysicalMutationsStop$ ./livecanary'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  printf 'jit-physical %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
 *'-tags=g01_pair_fixture -run ^TestJITBoundary -skip ^TestJITBoundary(StorageAndCancellationEveryAppend|ExpiryAtEveryAppend|PreparationRequiresOriginalFreshCreate|AtomicPrefixBeforeCredentialInput|ConcurrentConsumesOnce|HistoryCannotResumeOrRenew|StrictJournalSchemaAndRawReferences|ProofCannotCrossOpen|DirectRunAndCredentialShapes|CloneOwnsNestedPayload|PhysicalMutationsStop)$ ./livecanary'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  printf 'jit-boundary %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
 *'-tags=g01_pair_fixture -skip ^Test(Paired|SessionReplacement|JITBoundary|ExperimentalAcquireLossCommand$|ExperimentalDrainCommand$|ExperimentalACKBoundaryCommand$) ./livecanary'*)
  if [ ! -x "${G01_PHASE_CONTROLLER_TEST_BINARY:-}" ] || [ ! -x "${G01_PHASE_BROKER_TEST_BINARY:-}" ]; then
   printf 'missing-current-source-command-helpers\n' >&2
   exit 31
  fi
  printf 'consume %s\n' "$G01_PHASE_HELPER_DIR" >> "$PHASE_ROUTE_LOG"
  ;;
esac
exit 0
`, 0700)
			fail := "0"
			if failPrep {
				fail = "1"
			}
			output, err := toolingRun(t, root, []string{"OFFLINE_EXPERIMENT_MODULE=experiments/g01-scaleset", "GO=" + filepath.Join(root, "phase-go"), "PHASE_ROUTE_LOG=" + log, "PHASE_FAIL_PREP=" + fail}, "bash", "scripts/check-offline-experiments.sh")
			if (err != nil) != failPrep {
				t.Fatalf("helper preparation routing: failure=%t err=%v output=%s", failPrep, err, output)
			}
			data, e := os.ReadFile(log)
			if e != nil {
				t.Fatalf("no helper preparation: %v output=%s", e, output)
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			want := 11
			if failPrep {
				want = 1
			}
			if len(lines) != want || !strings.HasPrefix(lines[0], "prepare ") {
				t.Fatalf("helper routing %s", data)
			}
			dir := strings.TrimPrefix(lines[0], "prepare ")
			if _, e := os.Stat(dir); !os.IsNotExist(e) {
				t.Fatal("owned helper directory survived completion")
			}
			if !failPrep && (lines[1] != "replacement "+dir || lines[2] != "acquire-loss "+dir || lines[3] != "drain "+dir || lines[4] != "ack "+dir || lines[5] != "jit-storage "+dir || lines[6] != "jit-expiry "+dir || lines[7] != "jit-admission "+dir || lines[8] != "jit-physical "+dir || lines[9] != "jit-boundary "+dir || lines[10] != "consume "+dir) {
				t.Fatal("fixture partition consumed different helpers")
			}
		})
	}
}

func TestToolingSessionReplacementPartitionDisjoint(t *testing.T) {
	data, err := os.ReadFile("check-offline-experiments.sh")
	if err != nil {
		t.Fatal(err)
	}
	separate := regexp.MustCompile(toolingScriptAssignment(t, string(data), "replacement_regex"))
	acquireLoss := regexp.MustCompile(toolingScriptAssignment(t, string(data), "acquire_loss_command_regex"))
	drain := regexp.MustCompile(toolingScriptAssignment(t, string(data), "drain_command_regex"))
	ack := regexp.MustCompile(toolingScriptAssignment(t, string(data), "ack_command_regex"))
	jit := regexp.MustCompile(toolingScriptAssignment(t, string(data), "jit_boundary_regex"))
	storage := regexp.MustCompile(toolingScriptAssignment(t, string(data), "jit_boundary_storage_regex"))
	expiry := regexp.MustCompile(toolingScriptAssignment(t, string(data), "jit_boundary_expiry_regex"))
	admission := regexp.MustCompile(toolingScriptAssignment(t, string(data), "jit_boundary_admission_regex"))
	physical := regexp.MustCompile(toolingScriptAssignment(t, string(data), "jit_boundary_physical_regex"))
	heavy := regexp.MustCompile(toolingScriptAssignment(t, string(data), "jit_boundary_heavy_regex"))
	skip := regexp.MustCompile(toolingScriptAssignment(t, string(data), "non_paired_skip_regex"))
	paired := regexp.MustCompile(toolingScriptAssignment(t, string(data), "paired_collection_regex"))
	for _, name := range []string{"TestJITBoundaryPhysicalMutationsStop", "TestJITBoundaryPreparationRequiresOriginalFreshCreate", "TestJITBoundaryStrictJournalSchemaAndRawReferences", "TestJITBoundaryStorageAndCancellationEveryAppend", "TestJITBoundaryExpiryAtEveryAppend", "TestJITBoundaryProtocol", "TestJITBoundaryFutureNegative", "TestSessionReplacementProtocol", "TestSessionReplacementFutureNegative", "TestExperimentalActualCommandBrokerReceiver", "TestExperimentalAcquireLossCommand", "TestExperimentalAcquireLossCommandFuture", "TestExperimentalDrainCommand", "TestExperimentalDrainCommandFuture", "TestExperimentalACKBoundaryCommand", "TestExperimentalACKBoundaryCommandFuture", "TestPairedTerminalFutureCoverage", "ExampleFuture", "FuzzFuture"} {
		count := 0
		if ack.MatchString(name) {
			count++
		}
		if drain.MatchString(name) {
			count++
		}
		if acquireLoss.MatchString(name) {
			count++
		}
		if separate.MatchString(name) {
			count++
		}
		if jit.MatchString(name) && !heavy.MatchString(name) {
			count++
		}
		if storage.MatchString(name) {
			count++
		}
		if expiry.MatchString(name) {
			count++
		}
		if admission.MatchString(name) {
			count++
		}
		if physical.MatchString(name) {
			count++
		}
		if !skip.MatchString(name) {
			count++
		}
		if paired.MatchString(name) {
			count++
		}
		if count != 1 {
			t.Fatalf("%s covered %d times", name, count)
		}
	}
	if strings.Count(string(data), `-run "${ack_command_regex}" ./livecanary`) != 1 || strings.Count(string(data), `-run "${drain_command_regex}" ./livecanary`) != 1 || strings.Count(string(data), `-run "${acquire_loss_command_regex}" ./livecanary`) != 1 || strings.Count(string(data), `-run "${replacement_regex}" ./livecanary`) != 1 || strings.Count(string(data), `-run "${jit_boundary_storage_regex}" ./livecanary`) != 1 || strings.Count(string(data), `-run "${jit_boundary_expiry_regex}" ./livecanary`) != 1 || strings.Count(string(data), `-run "${jit_boundary_admission_regex}" ./livecanary`) != 1 || strings.Count(string(data), `-run "${jit_boundary_physical_regex}" ./livecanary`) != 1 || strings.Count(string(data), `-run "${jit_boundary_regex}" -skip "${jit_boundary_heavy_regex}" ./livecanary`) != 1 || strings.Count(string(data), `-skip "${non_paired_skip_regex}" ./livecanary`) != 1 {
		t.Fatal("replacement partition not consumed exactly once")
	}
}

// Exercise the real checker with an inert recording Go. No inherited behavioral
// suite runs here; argv and owned helper lifetime are the observable contract.
func toolingG01ProfileCalls(t *testing.T, profile, module, fail string) ([]string, error) {
	t.Helper()
	root := toolingFixture(t)
	log := filepath.Join(root, "profile.log")
	toolingFile(t, root, "recording go", `#!/bin/sh
set -eu
printf '%s\t%s\t' "$GOTOOLCHAIN" "${G01_PHASE_HELPER_DIR:-}" >> "$PROFILE_LOG"
printf '%s\t' "$@" >> "$PROFILE_LOG"
printf '\n' >> "$PROFILE_LOG"
case "$*" in
 *'TestExperimentalPrepareCommandHelpers$'*)
  test -d "$G01_PHASE_HELPER_DIR"
  test "$G01_PHASE_GO" = "$0"
  test "$G01_PHASE_CONTROLLER_TEST_BINARY" = "$G01_PHASE_HELPER_DIR/controller.test"
  test "$G01_PHASE_BROKER_TEST_BINARY" = "$G01_PHASE_HELPER_DIR/broker.test"
  case "$(LC_ALL=C ls -ld "$G01_PHASE_HELPER_DIR")" in drwx------*) ;; *) exit 38;; esac
  printf '#!/bin/sh\nexit 0\n' > "$G01_PHASE_CONTROLLER_TEST_BINARY"
  cp "$G01_PHASE_CONTROLLER_TEST_BINARY" "$G01_PHASE_BROKER_TEST_BINARY"
  chmod 0700 "$G01_PHASE_CONTROLLER_TEST_BINARY" "$G01_PHASE_BROKER_TEST_BINARY"
  ;;
 *'-tags=g01_pair_fixture'*)
  test -x "$G01_PHASE_CONTROLLER_TEST_BINARY"
  test -x "$G01_PHASE_BROKER_TEST_BINARY"
  ;;
esac
if [ -n "$PROFILE_FAIL" ] && [ "$*" = "$PROFILE_FAIL" ]; then exit 37; fi
`, 0700)
	out, runErr := toolingRun(t, root, []string{
		"GO=" + filepath.Join(root, "recording go"), "PROFILE_LOG=" + log,
		"PROFILE_FAIL=" + fail, "OFFLINE_EXPERIMENT_PROFILE=" + profile,
		"OFFLINE_EXPERIMENT_MODULE=" + module,
	}, "bash", "scripts/check-offline-experiments.sh")
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) && runErr != nil {
		return nil, runErr
	}
	if err != nil {
		t.Fatalf("profile log: %v output=%s", err, out)
	}
	var calls []string
	prepDir := ""
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.Split(strings.TrimRight(line, "\t"), "\t")
		if len(parts) < 3 || parts[0] != "go1.27.3" {
			t.Fatalf("unbounded/unpinned profile call %q", line)
		}
		if parts[1] != "" {
			if prepDir != "" && prepDir != parts[1] {
				t.Fatal("profile crossed helper directories")
			}
			prepDir = parts[1]
		}
		calls = append(calls, strings.Join(parts[2:], "\t"))
	}
	if prepDir != "" {
		if _, err := os.Stat(prepDir); !os.IsNotExist(err) {
			t.Fatalf("profile left owned helper directory %s", prepDir)
		}
		t.Logf("profile=%s cleaned owned helper directory %s", profile, prepDir)
	}
	return calls, runErr
}

func toolingG01ProfilePreparation(call string) bool {
	return strings.Contains(call, "\t^TestExperimentalPrepareCommandHelpers$\t")
}

func TestToolingG01ProfileRouting(t *testing.T) {
	profiles := map[string][]string{}
	for _, profile := range []string{"", "all", "core", "jit"} {
		calls, err := toolingG01ProfileCalls(t, profile, "experiments/g01-scaleset", "")
		if err != nil {
			t.Fatalf("profile %q failed: %v", profile, err)
		}
		prep := 0
		for _, call := range calls {
			if toolingG01ProfilePreparation(call) {
				prep++
				continue
			}
			profiles[profile] = append(profiles[profile], call)
		}
		if prep != 1 {
			t.Fatalf("profile %q prepares helpers %d times", profile, prep)
		}
		t.Logf("profile=%q behavioral argv (%d):\n%s", profile, len(profiles[profile]), strings.Join(profiles[profile], "\n"))
	}
	if !toolingSameNames(profiles[""], profiles["all"]) {
		t.Fatal("default no longer executes the complete all profile")
	}
	union := append(append([]string{}, profiles["core"]...), profiles["jit"]...)
	if !toolingSameNames(profiles["all"], union) {
		t.Error("core+jit behavioral command multiset differs from all")
	}
	if len(profiles["jit"]) != 5 {
		t.Errorf("jit has %d behavioral calls, want exactly five", len(profiles["jit"]))
	}
	for profile, calls := range profiles {
		acquireLoss, drain, ack := 0, 0, 0
		for _, call := range calls {
			if strings.Contains(call, "\t^TestExperimentalACKBoundaryCommand$\t") {
				ack++
				if !strings.Contains(call, "test\t-race\t-count=1\t-timeout=120s\t-tags=g01_pair_fixture\t-run\t") {
					t.Error("unbounded ACK command")
				}
			}
			if strings.Contains(call, "\t^TestExperimentalDrainCommand$\t") {
				drain++
				if !strings.Contains(call, "test\t-race\t-count=1\t-timeout=120s\t-tags=g01_pair_fixture\t-run\t") {
					t.Errorf("unbounded drain command %q", call)
				}
			}
			if strings.Contains(call, "\t^TestExperimentalAcquireLossCommand$\t") {
				acquireLoss++
				if !strings.Contains(call, "test\t-race\t-count=1\t-timeout=120s\t-tags=g01_pair_fixture\t-run\t") {
					t.Errorf("unbounded acquire-loss command %q", call)
				}
			}
			jit := strings.Contains(call, "\t^TestJITBoundary")
			if profile == "jit" && (!jit || !strings.Contains(call, "-timeout=120s\t-tags=g01_pair_fixture\t")) || profile == "core" && jit {
				t.Errorf("misrouted %s command %q", profile, call)
			}
		}
		want := 1
		if profile == "jit" {
			want = 0
		}
		if ack != want {
			t.Errorf("profile %q ACK calls=%d want=%d", profile, ack, want)
		}
		if drain != want {
			t.Errorf("profile %q drain calls=%d want=%d", profile, drain, want)
		}
		if acquireLoss != want {
			t.Errorf("profile %q acquire-loss calls=%d want=%d", profile, acquireLoss, want)
		}
	}
	// Name discovery compiles/lists current tagged tests without executing them.
	// Include future family members and Example/Fuzz names to guard complements.
	names := toolingListLivecanary(t, "^Test")
	names = append(names, "TestJITBoundaryFuture", "TestSessionReplacementFuture", "TestPairedTerminalFuture", "TestFuture", "TestExperimentalACKBoundaryCommandFuture", "ExampleFuture", "FuzzFuture")
	for _, name := range names {
		if name == "TestExperimentalPrepareCommandHelpers" {
			continue // Independent setup; also validates provenance in the remainder.
		}
		counts := map[string]int{}
		for _, profile := range []string{"core", "jit"} {
			for _, call := range profiles[profile] {
				args := strings.Split(call, "\t")
				if len(args) == 0 || args[0] != "test" || !toolingContainsString(args, "-tags=g01_pair_fixture") {
					continue
				}
				run, skip := ".", "^$"
				for i := 0; i+1 < len(args); i++ {
					if args[i] == "-run" {
						run = args[i+1]
					}
					if args[i] == "-skip" {
						skip = args[i+1]
					}
				}
				if regexp.MustCompile(run).MatchString(name) && !regexp.MustCompile(skip).MatchString(name) {
					counts[profile]++
				}
			}
		}
		want := "core"
		if strings.HasPrefix(name, "TestJITBoundary") {
			want = "jit"
		}
		if counts[want] != 1 || counts["core"]+counts["jit"] != 1 {
			t.Errorf("tagged name %s routes %v, want once in %s", name, counts, want)
		}
	}
	t.Logf("routed %d current/synthetic tagged names", len(names))
}

func TestToolingG01ProfileRejectsBeforeGo(t *testing.T) {
	for _, profile := range []string{"unknown", "core", "jit"} {
		for _, module := range []string{"", "experiments/g01-scaleset", "experiments/g02-auth", "experiments/r1-credentials", "unsupported"} {
			if profile != "unknown" && module == "experiments/g01-scaleset" {
				continue
			}
			calls, err := toolingG01ProfileCalls(t, profile, module, "")
			if err == nil || len(calls) != 0 {
				t.Errorf("profile=%q module=%q: err=%v calls=%v; want rejection before Go", profile, module, err, calls)
			}
		}
	}
}

func TestToolingG01ProfileFailuresCleanUp(t *testing.T) {
	for _, profile := range []string{"core", "jit"} {
		calls, err := toolingG01ProfileCalls(t, profile, "experiments/g01-scaleset", "")
		if err != nil {
			t.Fatal(err)
		}
		// Every call after helper allocation, including preparation and vet, must
		// stop the checker immediately and remove only its owned helper pair.
		prepared, acquireLoss, drain, ack := false, false, false, false
		for i, call := range calls {
			acquireLoss = acquireLoss || strings.Contains(call, "\t^TestExperimentalAcquireLossCommand$\t")
			drain = drain || strings.Contains(call, "\t^TestExperimentalDrainCommand$\t")
			ack = ack || strings.Contains(call, "\t^TestExperimentalACKBoundaryCommand$\t")
			prepared = prepared || toolingG01ProfilePreparation(call)
			if !prepared {
				continue
			}
			fail := strings.TrimSpace(strings.ReplaceAll(call, "\t", " "))
			got, err := toolingG01ProfileCalls(t, profile, "experiments/g01-scaleset", fail)
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 37 || len(got) != i+1 {
				t.Fatalf("%s call %d failure: err=%v count=%d want=%d", profile, i, err, len(got), i+1)
			}
		}
		if ack != (profile == "core") {
			t.Error("ACK failure cleanup coverage missing or leaked into jit")
		}
		if drain != (profile == "core") {
			t.Error("drain failure cleanup coverage missing or leaked into jit")
		}
		if acquireLoss != (profile == "core") {
			t.Error("acquire-loss failure cleanup coverage missing or leaked into jit")
		}
	}
}
