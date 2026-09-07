package livecanary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"syscall"
	"time"
)

type phaseAuthority struct {
	Digest    string    `json:"digest"`
	ExpiresAt time.Time `json:"expires_at"`
	Phases    []string  `json:"phases"`
}
type journalHeader struct {
	Version   int            `json:"version"`
	Ownership string         `json:"ownership"`
	Authority phaseAuthority `json:"authority"`
}

func approvalDigest(a Approval) string {
	data, _ := json.Marshal(a)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func ownershipDigest(a Approval) string {
	a.ExpiresAt = time.Time{}
	a.Phases = nil
	return approvalDigest(a)
}
func authorityFor(a Approval) phaseAuthority {
	return phaseAuthority{approvalDigest(a), a.ExpiresAt, slices.Clone(a.Phases)}
}
func (p phaseAuthority) matchesOwnership(a Approval) bool {
	if p.ExpiresAt.IsZero() {
		return false
	}
	a.ExpiresAt = p.ExpiresAt
	a.Phases = p.Phases
	return a.Validate(p.ExpiresAt.Add(-time.Nanosecond)) == nil && p.Digest == approvalDigest(a)
}
func (p phaseAuthority) renews(previous phaseAuthority) bool {
	if !p.ExpiresAt.After(previous.ExpiresAt) || len(p.Phases) == 0 {
		return false
	}
	for _, phase := range p.Phases {
		if phase != "inspect" && phase != "cleanup" {
			return false
		}
	}
	return true
}

// The lease keeps journal/claim ownership held until this one Driver.Run exits.
// This is approved-code discipline, not isolation against hostile Go callers.
func (j *FileJournal) authorize(a Approval) (func(), error) {
	if !j.life.TryLock() {
		return nil, ErrJournal
	}
	if j.closed || !j.ownsCurrentJournal() || a.Validate(time.Now()) != nil || j.ownership != ownershipDigest(a) || j.authority.Digest != approvalDigest(a) {
		j.life.Unlock()
		return nil, ErrJournal
	}
	return j.life.Unlock, nil
}

func (j *FileJournal) ownsCurrentJournal() bool {
	if j.root == nil || j.file == nil {
		return false
	}
	directory, err := os.Lstat(j.directory)
	if err != nil || !directory.IsDir() || directory.Mode().Perm() != 0700 || !os.SameFile(directory, j.directoryInfo) {
		return false
	}
	stat, ok := directory.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return false
	}
	named, err := j.root.Lstat("journal.jsonl")
	if err != nil || !privateFile(named, 0600) || !os.SameFile(named, j.fileInfo) {
		return false
	}
	opened, err := j.file.Stat()
	return err == nil && privateFile(opened, 0600) && os.SameFile(opened, j.fileInfo)
}
