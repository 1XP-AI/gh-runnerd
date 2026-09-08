package livecanary

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Optional cross-module fixture is ONLY the Go test executable. Production has
// no injected root/entry point. It calls the canonical local preparation helper,
// never Driver.Run or an API, and must receive an empty stdin and fixed argv.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-paired-journal" {
		if len(os.Args) != 6 || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" {
			os.Exit(2)
		}
		input, e := io.ReadAll(io.LimitReader(os.Stdin, 1))
		if e != nil || len(input) != 0 {
			os.Exit(3)
		}
		for _, entry := range os.Environ() {
			if entry != "LANG=C" && entry != "LC_ALL=C" {
				os.Exit(4)
			}
		}
		a, e := ReadApproval(os.Args[3])
		if e != nil {
			os.Exit(5)
		}
		directory, e := filepath.EvalSymlinks(filepath.Join(filepath.Dir(os.Args[5]), "admission"))
		if e != nil {
			os.Exit(6)
		}
		receipt, e := preparePairedJournal(os.Args[5], a, func(path string, a Approval) (*FileJournal, error) {
			return openJournalAtAdmission(path, a, directory, func(f *os.File) error { return f.Sync() })
		})
		if e != nil {
			os.Exit(7)
		}
		if json.NewEncoder(os.Stdout).Encode(receipt) != nil {
			os.Exit(8)
		}
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-journal" {
		if len(os.Args) != 8 || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" || os.Args[6] != "--phase" {
			os.Exit(2)
		}
		input, e := io.ReadAll(io.LimitReader(os.Stdin, 1))
		if e != nil || len(input) != 0 {
			os.Exit(3)
		}
		for _, entry := range os.Environ() {
			if entry != "LANG=C" && entry != "LC_ALL=C" {
				os.Exit(4)
			}
		}
		a, e := ReadApproval(os.Args[3])
		if e != nil {
			os.Exit(5)
		}
		directory, e := filepath.EvalSymlinks(filepath.Join(filepath.Dir(os.Args[5]), "admission"))
		if e != nil {
			os.Exit(6)
		}
		receipt, e := prepareJournal(os.Args[5], a, os.Args[7], func(path string, a Approval) (*FileJournal, error) {
			return openJournalAtAdmission(path, a, directory, func(f *os.File) error { return f.Sync() })
		})
		if e != nil {
			os.Exit(7)
		}
		if json.NewEncoder(os.Stdout).Encode(receipt) != nil {
			os.Exit(8)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
