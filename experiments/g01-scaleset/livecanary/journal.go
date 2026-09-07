package livecanary

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
		return nil, ErrJournal
	}
	directoryForAdmission, err := admissionDirectoryForAccount(user.LookupId)
	if err != nil {
		return nil, ErrJournal
	}
	return openJournalAtAdmission(directory, a, directoryForAdmission, func(f *os.File) error { return f.Sync() })
}

func openJournalAtAdmission(directory string, a Approval, admissionDirectory string, syncDirectory func(*os.File) error) (*FileJournal, error) {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, ErrJournal
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(s.Uid) != os.Geteuid() {
		return nil, ErrJournal
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, ErrJournal
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
		return nil, ErrJournal
	}
	f, err := root.OpenFile("journal.jsonl", os.O_RDWR|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, ErrJournal
	}
	ok = false
	defer func() {
		if !ok {
			f.Close()
		}
	}()
	info, err = f.Stat()
	if err != nil || !privateFile(info, 0600) || info.Size() > 1<<20 {
		return nil, ErrJournal
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return nil, ErrJournal
	}
	if a.Validate(time.Now()) != nil {
		return nil, ErrJournal
	}
	j := &FileJournal{file: f, root: root, directory: directory, directoryInfo: directoryInfo, fileInfo: info, ownership: ownershipDigest(a)}
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
			return nil, ErrJournal
		}
		var header journalHeader
		if DecodeStrict(line, &header) != nil || header.Version != 1 || header.Ownership != j.ownership || !header.Authority.matchesOwnership(a) {
			return nil, ErrJournal
		}
		j.authority = header.Authority
		for {
			line, err = reader.ReadBytes('\n')
			if err == io.EOF && len(line) == 0 {
				break
			}
			if err != nil {
				return nil, ErrJournal
			}
			var e Event
			if DecodeStrict(line, &e) != nil || e.Sequence != len(j.events)+1 || !validEvent(e) {
				return nil, ErrJournal
			}
			if e.Kind == "authority" {
				if !e.Authority.matchesOwnership(a) || !e.Authority.renews(j.authority) {
					return nil, ErrJournal
				}
				j.authority = *e.Authority
			}
			j.events = append(j.events, e)
		}
	}
	if incoming.Digest != j.authority.Digest {
		if !incoming.renews(j.authority) {
			return nil, ErrJournal
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
		return nil, ErrJournal
	}
	err = syncDirectory(dir)
	dir.Close()
	if err != nil {
		return nil, ErrJournal
	}
	claim, err := openAdmission(admissionDirectory, j, syncDirectory)
	if err != nil {
		return nil, err
	}
	j.claim = claim
	rootKept = true
	ok = true
	return j, nil
}

func validEvent(e Event) bool {
	switch e.Kind {
	case "authority":
		return e.Authority != nil
	case "phase":
		return e.Operation != "" && e.Digest == ""
	case "inventory":
		return len(e.Digest) == 64
	case "observation":
		return e.Operation == "poll" || e.Operation == "inspect" || e.Operation == "inventory"
	case "response":
		return e.Operation == "jit" || e.Operation == "acquire"
	case "barrier":
		return e.Operation == "before-ack" || e.Operation == "after-ack" || e.Operation == "before-acquire"
	case "intent", "result", "unknown":
		switch e.Operation {
		case "create", "session-open", "session-close", "ack", "acquire", "jit", "delete", "probe":
			return true
		}
	}
	return false
}

func (j *FileJournal) write(e any) error {
	data, err := json.Marshal(e)
	if err != nil {
		return ErrJournal
	}
	data = append(data, '\n')
	n, err := j.file.Write(data)
	if err != nil || n != len(data) || j.file.Sync() != nil {
		return ErrJournal
	}
	return nil
}
func (j *FileJournal) Append(e Event) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !validEvent(e) {
		return ErrJournal
	}
	e.Sequence = len(j.events) + 1
	if err := j.write(e); err != nil {
		return err
	}
	j.events = append(j.events, e)
	return nil
}
func (j *FileJournal) Events() []Event {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]Event(nil), j.events...)
}
func (j *FileJournal) Close() error {
	j.life.Lock()
	defer j.life.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	fileErr := j.file.Close()
	rootErr := j.root.Close()
	claimErr := j.claim.close()
	if fileErr != nil || rootErr != nil || claimErr != nil {
		return ErrJournal
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
