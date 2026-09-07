package enrollment

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"syscall"
)

// Only this non-secret inventory survives a process exit. Its presence, including
// an interrupted or completed attempt, forbids another automatic registration.
type attemptRecord struct {
	Version       int       `json:"version"`
	Owner         string    `json:"owner"`
	AppName       string    `json:"app_name"`
	Organizations []Binding `json:"organizations"`
	Phase         string    `json:"phase"`
	AppID         int64     `json:"app_id,omitempty"`
}
type journal struct {
	root   *os.Root
	lock   *os.File
	record attemptRecord
}

var errJournal = errors.New("private attempt journal unavailable; inspect existing App and use manual import")

func privateFile(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().IsRegular() && info.Mode().Perm() == 0600 && stat.Uid == uint32(os.Getuid()) && stat.Nlink == 1
}
func openJournal(path string, p Proposal, manual bool, appID int64) (j *journal, err error) {
	if e := os.Mkdir(path, 0700); e != nil && !os.IsExist(e) {
		return nil, errJournal
	}
	info, e := os.Lstat(path)
	if e != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, errJournal
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return nil, errJournal
	}
	root, e := os.OpenRoot(path)
	if e != nil {
		return nil, errJournal
	}
	j = &journal{root: root}
	defer func() {
		if err != nil {
			j.close()
		}
	}()
	actual, e := root.Stat(".")
	if e != nil || !os.SameFile(info, actual) {
		return j, errJournal
	}
	lock, e := root.OpenFile("active.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return j, errJournal
	}
	j.lock = lock
	if !privateFile(lock) || syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return j, errJournal
	}
	j.record = attemptRecord{Version: 1, Owner: p.Owner, AppName: p.AppName, Organizations: append([]Binding(nil), p.Organizations...), Phase: "prepared", AppID: appID}
	existing, e := root.OpenFile("attempt.json", os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e == nil {
		defer existing.Close()
		if !manual || !privateFile(existing) {
			return j, errJournal
		}
		data, e := io.ReadAll(io.LimitReader(existing, 16385))
		if e != nil || len(data) > 16384 {
			return j, errJournal
		}
		var old attemptRecord
		if json.Unmarshal(data, &old) != nil || old.Version != 1 || old.Owner != p.Owner || old.AppName != p.AppName || old.AppID < 0 || (old.AppID != 0 && old.AppID != appID) || !sameOrganizations(old.Organizations, p.Organizations) {
			return j, errJournal
		}
		switch old.Phase {
		case "prepared", "registration_started", "conversion_started", "conversion_failed", "app_received", "verifying", "verification_failed", "verified":
		default:
			return j, errJournal
		}
		// Keep ownership and identity on recovery; imported IDs must still be checked
		// with the App's JWT before they can become verified bindings.
		j.record = old
		return j, nil
	}
	if !os.IsNotExist(e) {
		return j, errJournal
	}
	f, e := root.OpenFile("attempt.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return j, errJournal
	}
	data, _ := json.Marshal(j.record)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || j.syncDir() != nil {
		return j, errJournal
	}
	return j, nil
}
func sameOrganizations(a, b []Binding) bool {
	a = append([]Binding(nil), a...)
	b = append([]Binding(nil), b...)
	for i := range a {
		a[i].InstallationID = 0
	}
	for i := range b {
		b[i].InstallationID = 0
	}
	return reflect.DeepEqual(a, b)
}
func (j *journal) syncDir() error {
	f, e := j.root.Open(".")
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (j *journal) phase(phase string, appID int64, bindings []Binding) error {
	next := j.record
	next.Phase = phase
	if appID > 0 {
		next.AppID = appID
	}
	if bindings != nil {
		next.Organizations = append([]Binding(nil), bindings...)
	}
	// A leftover temp file from a crash is a fail-closed recovery condition.
	f, e := j.root.OpenFile("record.next", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return errJournal
	}
	data, _ := json.Marshal(next)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errJournal
	}
	if j.root.Rename("record.next", "attempt.json") != nil || j.syncDir() != nil {
		return errJournal
	}
	j.record = next
	return nil
}
func (j *journal) close() {
	if j == nil {
		return
	}
	if j.lock != nil {
		_ = j.lock.Close()
	}
	if j.root != nil {
		_ = j.root.Close()
	}
}
