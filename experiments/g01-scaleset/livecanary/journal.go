package livecanary

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"sync"
	"syscall"
)

// Journal is private controller state, not a publishable evidence report. The
// single locked inode is retained even after crashes. A damaged tail is rejected,
// never truncated, repaired or interpreted as permission to retry.
type FileJournal struct {
	file   *os.File
	events []Event
	mu     sync.Mutex
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
			if !ok || seen[s] {
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
	defer root.Close()
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
	encoded, err := json.Marshal(a)
	if err != nil {
		return nil, ErrJournal
	}
	digest := sha256.Sum256(encoded)
	want := hex.EncodeToString(digest[:])
	j := &FileJournal{file: f}
	if info.Size() == 0 {
		if err = j.write(Event{Kind: "approval", Digest: want}); err != nil {
			return nil, err
		}
		// Sync both the new file and its directory entry before permitting any
		// external effect. A zero-length/torn journal can never authorize reuse.
		dir, err := root.Open(".")
		if err != nil {
			return nil, ErrJournal
		}
		err = dir.Sync()
		dir.Close()
		if err != nil {
			return nil, ErrJournal
		}
	} else {
		reader := bufio.NewReader(io.LimitReader(f, 1<<20+1))
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, ErrJournal
		}
		var header Event
		if DecodeStrict(line, &header) != nil || header.Kind != "approval" || header.Sequence != 0 || header.Digest != want {
			return nil, ErrJournal
		}
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
			j.events = append(j.events, e)
		}
	}
	ok = true
	return j, nil
}

func validEvent(e Event) bool {
	switch e.Kind {
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

func (j *FileJournal) write(e Event) error {
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
func (j *FileJournal) Close() error { return j.file.Close() }
