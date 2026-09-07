//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPairedReplayRejectsForgedUnknownFacts(t *testing.T) {
	f := newPairedIntegrationFixture(t)
	if _, err := runFastPair(f); err != nil {
		t.Fatal("completed fixture")
	}
	dir, admission := f.c.j.directory, f.c.j.claim.directory
	data, err := os.ReadFile(filepath.Join(dir, "journal.jsonl"))
	if err != nil {
		t.Fatal("fixture file")
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	_ = f.c.j.Close()
	cases := []struct {
		name, stage string
		change      func(*baselineRecord)
	}{
		{"handoff-self-ref", "handoff", func(r *baselineRecord) { r.Handoff.Input.HandoffIntent.Sequence++ }},
		{"jit-negative-id", "jit", func(r *baselineRecord) { r.JIT.Runner.ID = -1 }},
		{"pair-wrong-receipt", "pair", func(r *baselineRecord) { r.Pair.Receipt.PairSHA256 = strings.Repeat("f", 64) }},
		{"host-unprefixed-image", "host-preflight", func(r *baselineRecord) { r.Host.ImageID = strings.TrimPrefix(r.Host.ImageID, "sha256:") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copyLines := make([][]byte, len(lines))
			for i := range lines {
				copyLines[i] = bytes.Clone(lines[i])
			}
			found := false
			for i := 1; i < len(copyLines); i++ {
				var e Event
				if json.Unmarshal(copyLines[i], &e) != nil || e.Baseline == nil || e.Baseline.Stage != tc.stage || e.Baseline.Outcome != "result" {
					continue
				}
				tc.change(e.Baseline)
				e.Baseline.Outcome = "unknown"
				copyLines[i], _ = json.Marshal(e)
				copyLines = copyLines[:i+1]
				found = true
				break
			}
			if !found {
				t.Fatal("selected receipt missing")
			}
			altered := append(bytes.Join(copyLines, []byte{'\n'}), '\n')
			if os.WriteFile(filepath.Join(dir, "journal.jsonl"), altered, 0600) != nil {
				t.Fatal("rewrite fixture inode")
			}
			j, err := openJournalAtAdmission(dir, f.c.a, admission, func(file *os.File) error { return file.Sync() })
			if err == nil {
				_ = j.Close()
				t.Fatal("forged unknown receipt accepted")
			}
		})
	}
}
