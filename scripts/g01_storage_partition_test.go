package tooling

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestToolingG01StoragePartitionsUseSeparateProcesses(t *testing.T) {
	root := toolingFixture(t)
	processLog := filepath.Join(root, "storage-processes.log")
	toolingFile(t, root, "experiments/g01-scaleset/livecanary/storage_process_test.go", `//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"fmt"
	"os"
	"testing"
)

func storageProcess(t *testing.T, name string) {
	t.Helper()
	file, err := os.OpenFile(os.Getenv("TOOLING_STORAGE_PROCESS_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("storage process witness open")
	}
	if _, err := fmt.Fprintf(file, "%s %d\n", name, os.Getpid()); err != nil {
		t.Fatal("storage process witness write")
	}
	if err := file.Close(); err != nil {
		t.Fatal("storage process witness close")
	}
}

func TestPairedTerminalPostIntentJournalIdentity(t *testing.T) { storageProcess(t, "journal") }
func TestPairedTerminalActualControllerSyncFailures(t *testing.T) { storageProcess(t, "remaining") }
`, 0600)
	out, err := toolingRun(t, root, []string{"OFFLINE_EXPERIMENT_MODULE=experiments/g01-scaleset", "TOOLING_STORAGE_PROCESS_LOG=" + processLog}, "bash", "scripts/check-offline-experiments.sh")
	if err != nil || !strings.Contains(out, "offline experiment checks passed: 1 module(s)") {
		t.Fatalf("storage witness gate failed: %s", out)
	}
	data, err := os.ReadFile(processLog)
	if err != nil {
		t.Fatal(err)
	}
	processes := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || (fields[0] != "journal" && fields[0] != "remaining") {
			t.Fatal("unexpected storage process witness")
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil || pid <= 0 || processes[fields[0]] != 0 {
			t.Fatal("missing or duplicate storage family execution")
		}
		processes[fields[0]] = pid
	}
	if len(processes) != 2 || processes["journal"] == processes["remaining"] {
		t.Fatal("journal identity and remaining storage families shared one test process")
	}
}
