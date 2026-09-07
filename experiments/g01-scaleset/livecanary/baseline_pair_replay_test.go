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
		{"sample-unrecognized-outcome", "identity-sample", func(r *baselineRecord) { r.Sample.SDK.Response.Outcome = "arbitrary-remote-text" }},
		{"sample-negative-job", "identity-sample", func(r *baselineRecord) { r.Sample.Job.ID = -1 }},
		{"sample-invalid-local-ref", "identity-sample", func(r *baselineRecord) { r.Sample.Local.Result.Sequence = -1 }},
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

func TestPairedReplayRESTJobNormalization(t *testing.T) {
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
	zero := func(endpoint string, status int, outcome observationOutcome) func(*restJobObservation) {
		return func(job *restJobObservation) {
			*job = restJobObservation{Response: observationResponse{Endpoint: endpoint, Status: status, Outcome: outcome}}
		}
	}
	cases := []struct {
		name   string
		valid  bool
		change func(*restJobObservation)
	}{
		{"present-without-ID", false, zero("rest_job", 200, observationPresent)},
		{"present-without-association", false, func(job *restJobObservation) { job.RunnerID = nil; job.RunnerName = nil; job.RunnerGroupID = nil }},
		{"pending-with-full-association", false, func(job *restJobObservation) { job.Response.Outcome = observationPending }},
		{"positive-ID-at-source", false, func(job *restJobObservation) { job.Response.Endpoint = "rest_run" }},
		{"positive-ID-at-list", false, func(job *restJobObservation) { job.Response.Endpoint = "rest_attempt_jobs" }},
		{"empty-pending-at-detail", false, zero("rest_job", 200, observationPending)},
		{"empty-pending-at-source", false, zero("rest_run", 200, observationPending)},
		{"positive-present-detail", true, func(*restJobObservation) {}},
		{"positive-pending-partial", true, func(job *restJobObservation) {
			job.Response.Outcome = observationPending
			job.RunnerName = nil
			job.RunnerGroupID = nil
		}},
		{"positive-pending-unassigned", true, func(job *restJobObservation) {
			job.Response.Outcome = observationPending
			job.RunnerID = nil
			job.RunnerName = nil
			job.RunnerGroupID = nil
		}},
		{"empty-pending-list", true, zero("rest_attempt_jobs", 200, observationPending)},
	}
	for _, endpoint := range []string{"rest_run", "rest_attempt_jobs", "rest_job"} {
		cases = append(cases, struct {
			name   string
			valid  bool
			change func(*restJobObservation)
		}{endpoint + "-unresolved", true, zero(endpoint, 200, observationUnresolved)}, struct {
			name   string
			valid  bool
			change func(*restJobObservation)
		}{endpoint + "-not-found", true, zero(endpoint, 404, observationNotFound)})
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
				if json.Unmarshal(copyLines[i], &e) != nil || e.Baseline == nil || e.Baseline.Stage != "identity-sample" || e.Baseline.Outcome != "result" {
					continue
				}
				tc.change(e.Baseline.Sample.Job)
				// A partial unknown round stops after this actual job-reader slot.
				e.Baseline.Sample.REST = nil
				e.Baseline.Sample.Local = nil
				e.Baseline.Sample.RESTAddressable = false
				e.Baseline.Outcome = "unknown"
				copyLines[i], _ = json.Marshal(e)
				copyLines = copyLines[:i+1]
				found = true
				break
			}
			if !found {
				t.Fatal("actual sample missing")
			}
			if os.WriteFile(filepath.Join(dir, "journal.jsonl"), append(bytes.Join(copyLines, []byte{'\n'}), '\n'), 0600) != nil {
				t.Fatal("rewrite fixture inode")
			}
			j, err := openJournalAtAdmission(dir, f.c.a, admission, func(file *os.File) error { return file.Sync() })
			if j != nil {
				_ = j.Close()
			}
			if tc.valid && err != nil {
				t.Fatal("valid normalized partial job rejected")
			}
			if !tc.valid && err == nil {
				t.Fatal("impossible normalized job accepted")
			}
		})
	}
}
