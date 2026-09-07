package liveworker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/user"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
)

// Journal is private controller state, not a publishable evidence report. The
// single locked inode is retained even after crashes. A damaged tail is rejected,
// never truncated, repaired or interpreted as permission to retry.
type FileJournal struct {
	claim         *admissionClaim
	root          *os.Root
	directory     string
	directoryInfo os.FileInfo
	fileInfo      os.FileInfo
	life          sync.Mutex
	closed        bool
	ownership     string
	authority     phaseAuthority
	file          *os.File
	events        []Event
	mu            sync.Mutex
	paired        pairedState
	bytes         int64
	poisoned      bool
	// Private fault injection preserves the real file write/fsync boundary.
	recordSync func(*os.File) error
}

func privateFile(info os.FileInfo, mode os.FileMode) bool {
	s, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(s.Uid) == os.Geteuid() && info.Mode().Perm() == mode && info.Mode().IsRegular() && s.Nlink == 1
}

func DecodeStrict(data []byte, target any) error {
	// Reject ambiguous duplicate keys rather than silently accepting the last
	// value in a reviewed approval or a broker's authority metadata.
	if !uniqueKeys(json.NewDecoder(bytes.NewReader(data))) {
		return ErrApproval
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return ErrApproval
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrApproval
	}
	return nil
}

func uniqueKeys(d *json.Decoder) bool {
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return false
			}
			s, ok := key.(string)
			if !ok {
				return false
			}
			s = foldedJSONName(s)
			if seen[s] {
				return false
			}
			seen[s] = true
			if !uniqueKeys(d) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for d.More() {
			if !uniqueKeys(d) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

// ReadApproval rejects symlinks, nonregular/shared files and permissive modes.
// The caller keeps approval content and its private names out of public output.
func ReadApproval(path string) (Approval, error) {
	var a Approval
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return a, ErrApproval
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !privateFile(info, 0600) || info.Size() > 16384 {
		return a, ErrApproval
	}
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(data) > 16384 || DecodeStrict(data, &a) != nil {
		return Approval{}, ErrApproval
	}
	return a, nil
}

func OpenJournal(directory string, a Approval) (*FileJournal, error) {
	if !nativeAccountLookup {
		return nil, ErrState
	}
	admissionDirectory, err := admissionDirectoryForAccount(user.LookupId)
	if err != nil {
		return nil, ErrState
	}
	return openJournalAtAdmission(directory, a, admissionDirectory, func(f *os.File) error { return f.Sync() })
}

func openJournalAtAdmission(directory string, a Approval, admissionDirectory string, syncDirectory func(*os.File) error) (*FileJournal, error) {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, ErrState
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(s.Uid) != os.Geteuid() {
		return nil, ErrState
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, ErrState
	}
	rootKept := false
	defer func() {
		if !rootKept {
			_ = root.Close()
		}
	}()
	directoryInfo := info
	captured, err := root.Stat(".")
	if err != nil || !os.SameFile(directoryInfo, captured) {
		return nil, ErrState
	}
	f, err := root.OpenFile("journal.jsonl", os.O_RDWR|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, ErrState
	}
	ok = false
	defer func() {
		if !ok {
			f.Close()
		}
	}()
	info, err = f.Stat()
	if err != nil || !privateFile(info, 0600) || info.Size() > 1<<20 {
		return nil, ErrState
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return nil, ErrState
	}
	if a.Validate(time.Now()) != nil {
		return nil, ErrState
	}
	j := &FileJournal{file: f, root: root, directory: directory, directoryInfo: directoryInfo, fileInfo: info, ownership: ownershipDigest(a), bytes: info.Size()}
	incoming := authorityFor(a)
	if info.Size() == 0 {
		j.authority = incoming
		if err = j.write(journalHeader{1, j.ownership, incoming}); err != nil {
			return nil, err
		}

	} else {
		reader := bufio.NewReader(io.LimitReader(f, 1<<20+1))
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, ErrState
		}
		var header journalHeader
		if DecodeStrict(line, &header) != nil || header.Version != 1 || header.Ownership != j.ownership || !header.Authority.matchesOwnership(a) {
			return nil, ErrState
		}
		j.authority = header.Authority
		for {
			line, err = reader.ReadBytes('\n')
			if err == io.EOF && len(line) == 0 {
				break
			}
			if err != nil {
				return nil, ErrState
			}
			var e Event
			if DecodeStrict(line, &e) != nil || e.Sequence != len(j.events)+1 || (e.Paired != nil && len(line) > maxPairRecord) || !validEvent(e) || !j.paired.step(e) {
				return nil, ErrState
			}
			if e.Kind == "authority" {
				if !e.Authority.matchesOwnership(a) || !e.Authority.renews(j.authority) {
					return nil, ErrState
				}
				j.authority = *e.Authority
			}
			j.events = append(j.events, e)
		}
	}
	if incoming.Digest != j.authority.Digest {
		if !incoming.renews(j.authority) {
			return nil, ErrState
		}
		if err = j.Append(Event{Kind: "authority", Authority: &incoming}); err != nil {
			return nil, err
		}
		j.authority = incoming
	}

	// Retry directory-entry durability even when a prior failed sync left a
	// valid header. File contents alone never prove its entry survived a crash.
	dir, err := root.Open(".")
	if err != nil {
		return nil, ErrState
	}
	err = syncDirectory(dir)
	dir.Close()
	if err != nil {
		return nil, ErrState
	}
	j.claim, err = openAdmission(admissionDirectory, j, syncDirectory)
	if err != nil {
		return nil, err
	}
	if j.paired.binding != nil && j.paired.binding.Worker != j.pairedIdentity() {
		_ = j.claim.close()
		return nil, ErrState
	}
	rootKept = true
	ok = true
	return j, nil
}

func validEvent(e Event) bool {
	if e.Paired != nil || e.Kind == "paired" {
		return validPairedShape(e)
	}
	switch e.Kind {
	case "authority":
		return e.Authority != nil
	case "observation":
		return e.Operation == "inspect"
	case "intent", "result", "unknown":
		return e.Operation == "create" || e.Operation == "start" || e.Operation == "delete"
	}
	return false
}

func (j *FileJournal) write(e any) error {
	data, err := json.Marshal(e)
	if err != nil {
		return ErrState
	}
	data = append(data, '\n')
	if j.poisoned || j.bytes+int64(len(data)) > maxJournal {
		return ErrState
	}
	info, err := j.file.Stat()
	if err != nil || info.Size() != j.bytes {
		j.poisoned = true
		return ErrState
	}
	n, err := j.file.Write(data)
	if err != nil || n != len(data) {
		j.poisoned = true
		return ErrState
	}
	syncFile := j.recordSync
	if syncFile == nil {
		syncFile = func(f *os.File) error { return f.Sync() }
	}
	if syncFile(j.file) != nil {
		j.poisoned = true
		return ErrState
	}
	j.bytes += int64(len(data))
	return nil
}
func (j *FileJournal) Append(e Event) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	_, err := j.appendStored(e)
	return err
}

// appendStored works during pre-claim renewal too. Only appendRecord needs a
// fully initialized claim to produce a domain-bound public receipt.
func (j *FileJournal) appendStored(e Event) (Event, error) {
	e = cloneEvent(e)
	e.Sequence = len(j.events) + 1
	next := j.paired
	if !validEvent(e) || !next.step(e) {
		return Event{}, ErrState
	}
	if e.Paired != nil && (j.claim == nil || next.binding == nil || next.binding.Worker != j.pairedIdentity()) {
		return Event{}, ErrState
	}
	if err := j.write(e); err != nil {
		return Event{}, err
	}
	j.events = append(j.events, e)
	j.paired = next
	return cloneEvent(e), nil
}

func (j *FileJournal) appendRecord(e Event) (RecordRef, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.claim == nil {
		return RecordRef{}, ErrState
	}
	assigned, err := j.appendStored(e)
	if err != nil {
		return RecordRef{}, err
	}
	return workerEventRef(j.pairedIdentity(), assigned), nil
}

func (j *FileJournal) hasRoom(required int64) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	info, err := j.file.Stat()
	return err == nil && !j.poisoned && info.Size() == j.bytes && required >= 0 && j.bytes+required <= maxJournal
}

func (j *FileJournal) pairState() pairedState {
	j.mu.Lock()
	defer j.mu.Unlock()
	// Internal callers only read this snapshot. Exported receipts are cloned.
	return j.paired
}
func (j *FileJournal) Events() []Event {
	j.mu.Lock()
	defer j.mu.Unlock()
	events := make([]Event, len(j.events))
	for i, e := range j.events {
		events[i] = cloneEvent(e)
	}
	return events
}
func (j *FileJournal) Close() error {
	j.life.Lock()
	defer j.life.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	claimErr := j.claim.close()
	fileErr := j.file.Close()
	rootErr := j.root.Close()
	if claimErr != nil || fileErr != nil || rootErr != nil {
		return ErrState
	}
	return nil
}

// encoding/json matches struct fields with Unicode simple-fold equivalence.
// Canonicalize once per key, avoiding quadratic pairwise key comparisons.
// DecodeStrict serves fixed authority/journal schemas, not case-sensitive maps.
func foldedJSONName(input string) string {
	var result strings.Builder
	result.Grow(len(input))
	for _, r := range input {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		result.WriteRune(minimum)
	}
	return result.String()
}
