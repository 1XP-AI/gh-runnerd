package tooling

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestToolingG01StoragePartitionsUseSeparateProcesses(t *testing.T) {
	root := toolingFixture(t)
	processLog := filepath.Join(root, "storage-processes.log")
	toolingFile(t, root, "experiments/g01-scaleset/livecanary/storage_process_test.go", `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
)

var storageProcessIdentity = func() string {
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(identity[:])
}()

func storageProcess(t *testing.T, name string) {
	t.Helper()
	file, err := os.OpenFile(os.Getenv("TOOLING_STORAGE_PROCESS_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("storage process witness open")
	}
	if _, err := fmt.Fprintf(file, "%s %d %s %s\n", name, os.Getpid(), os.Getenv("TOOLING_STORAGE_PARTITION"), storageProcessIdentity); err != nil {
		t.Fatal("storage process witness write")
	}
	if err := file.Close(); err != nil {
		t.Fatal("storage process witness close")
	}
}

func TestPairedTerminalPostIntentJournalIdentity(t *testing.T) { storageProcess(t, "journal") }
func TestPairedTerminalActualControllerSyncFailures(t *testing.T) { storageProcess(t, "controller") }
func TestPairedTerminalActualWorkerSyncFailures(t *testing.T) { storageProcess(t, "worker") }
func TestPairedTerminalPostIntentAuthorityBoundaries(t *testing.T) { storageProcess(t, "authority") }
func TestPairedTerminalClosedReplayActualFile(t *testing.T) { storageProcess(t, "closed-replay") }
func TestPairedTerminalWorkerReceiptSurvivesControllerWriteFailure(t *testing.T) { storageProcess(t, "worker-receipt") }
`, 0600)

	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	moduleRoot := filepath.Join(root, "experiments/g01-scaleset")
	for _, tc := range []struct {
		partition string
		run       string
		skip      string
	}{
		{partition: "journal", run: g01StorageJournalRegex},
		{partition: "remainder", run: g01TerminalStorageRegex, skip: g01StorageJournalRegex},
	} {
		args := []string{"test", "-race", "-count=1", "-timeout=120s", "-tags=g01_pair_fixture", "-run", tc.run}
		if tc.skip != "" {
			args = append(args, "-skip", tc.skip)
		}
		args = append(args, "./livecanary")
		out, err := toolingRunWithTimeout(t, 90*time.Second, moduleRoot,
			[]string{"TOOLING_STORAGE_PROCESS_LOG=" + processLog, "TOOLING_STORAGE_PARTITION=" + tc.partition},
			append([]string{realGo}, args...)...,
		)
		if err != nil {
			t.Fatalf("%s storage partition failed: %s", tc.partition, out)
		}
	}

	data, err := os.ReadFile(processLog)
	if err != nil {
		t.Fatal(err)
	}
	expectedPartitions := map[string]string{
		"journal":        "journal",
		"controller":     "remainder",
		"worker":         "remainder",
		"authority":      "remainder",
		"closed-replay":  "remainder",
		"worker-receipt": "remainder",
	}
	seen := make(map[string]bool, len(expectedPartitions))
	partitionPIDs := make(map[string]int, 2)
	partitionIDs := make(map[string]string, 2)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			t.Fatalf("unexpected storage process witness: %q", line)
		}
		partition, ok := expectedPartitions[fields[0]]
		if !ok || fields[2] != partition || seen[fields[0]] {
			t.Fatalf("unexpected or duplicate storage witness: %q", line)
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil || pid <= 0 {
			t.Fatalf("invalid storage process PID in witness: %q", line)
		}
		if previous := partitionPIDs[partition]; previous != 0 && previous != pid {
			t.Fatalf("storage partition %s used multiple test processes: %d and %d", partition, previous, pid)
		}
		if previous := partitionIDs[partition]; previous != "" && previous != fields[3] {
			t.Fatalf("storage partition %s used multiple process identities", partition)
		}
		partitionPIDs[partition] = pid
		partitionIDs[partition] = fields[3]
		seen[fields[0]] = true
	}
	if len(seen) != len(expectedPartitions) || len(partitionPIDs) != 2 {
		t.Fatalf("storage witness coverage = %d tests across %d partitions, want %d tests across 2 partitions: %q", len(seen), len(partitionPIDs), len(expectedPartitions), data)
	}
	if partitionIDs["journal"] == partitionIDs["remainder"] {
		t.Fatal("journal and remaining storage families ran in one test process")
	}
}
