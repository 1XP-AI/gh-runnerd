package liveworker

import (
	"encoding/json"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// This permanent experiment pin is deliberately not configurable by approval,
// flags, environment or the supplied state directory. The operator prepares
// this one private directory explicitly; this code never creates its parents.
func admissionDirectoryForAccount(lookup func(string) (*user.User, error)) (string, error) {
	if lookup == nil {
		return "", ErrState
	}
	uid := strconv.Itoa(os.Geteuid())
	account, err := lookup(uid)
	if err != nil || account == nil || account.Uid != uid || !filepath.IsAbs(account.HomeDir) || filepath.Clean(account.HomeDir) != account.HomeDir {
		return "", ErrState
	}
	info, err := os.Lstat(account.HomeDir)
	canonical, canonicalErr := filepath.EvalSymlinks(account.HomeDir)
	if err != nil || canonicalErr != nil || canonical != account.HomeDir || !ownedDirectory(info, false) {
		return "", ErrState
	}
	return filepath.Join(account.HomeDir, ".gh-runnerd-g01-worker-experiment"), nil
}

func ownedDirectory(info os.FileInfo, private bool) bool {
	if info == nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 || (private && info.Mode().Perm() != 0700) {
		return false
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(s.Uid) == os.Geteuid()
}

type admissionRecord struct {
	Version       int    `json:"version"`
	Ownership     string `json:"ownership"`
	StateDevice   uint64 `json:"state_device"`
	StateInode    uint64 `json:"state_inode"`
	JournalDevice uint64 `json:"journal_device"`
	JournalInode  uint64 `json:"journal_inode"`
}

func admissionFor(j *FileJournal) admissionRecord {
	directory := j.directoryInfo.Sys().(*syscall.Stat_t)
	file := j.fileInfo.Sys().(*syscall.Stat_t)
	return admissionRecord{1, j.ownership, uint64(directory.Dev), directory.Ino, uint64(file.Dev), file.Ino}
}

type admissionClaim struct {
	file      *os.File
	root      *os.Root
	directory string
	rootInfo  os.FileInfo
	fileInfo  os.FileInfo
}

func openAdmission(directory string, j *FileJournal, syncDirectory func(*os.File) error) (*admissionClaim, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return nil, ErrState
	}
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil || canonical != directory {
		return nil, ErrState
	}
	info, err := os.Lstat(directory)
	if err != nil || !ownedDirectory(info, true) {
		return nil, ErrState
	}
	parentInfo, err := os.Lstat(filepath.Dir(directory))
	if err != nil || !ownedDirectory(parentInfo, false) {
		return nil, ErrState
	}
	parent, err := os.Open(filepath.Dir(directory))
	if err != nil {
		return nil, ErrState
	}
	defer parent.Close()
	capturedParent, err := parent.Stat()
	if err != nil || !os.SameFile(parentInfo, capturedParent) || syncDirectory(parent) != nil {
		return nil, ErrState
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, ErrState
	}
	kept := false
	defer func() {
		if !kept {
			_ = root.Close()
		}
	}()
	captured, err := root.Stat(".")
	if err != nil || !os.SameFile(info, captured) {
		return nil, ErrState
	}
	// Serialize the empty-file creation window before taking the claim's
	// lifetime flock. Otherwise a second opener can lock the creator's empty
	// claim, causing both contenders to refuse and strand an empty claim.
	dir, err := root.Open(".")
	if err != nil {
		return nil, ErrState
	}
	defer dir.Close()
	lockedDirectory, err := dir.Stat()
	if err != nil || !os.SameFile(info, lockedDirectory) || syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return nil, ErrState
	}
	created := true
	file, err := root.OpenFile("admission.json", os.O_RDWR|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if os.IsExist(err) {
		created = false
		file, err = root.OpenFile("admission.json", os.O_RDWR|syscall.O_NOFOLLOW, 0)
	}
	if err != nil {
		return nil, ErrState
	}
	defer func() {
		if !kept {
			_ = file.Close()
		}
	}()
	fileInfo, err := file.Stat()
	if err != nil || !privateFile(fileInfo, 0600) || fileInfo.Size() > 4096 || syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return nil, ErrState
	}
	want := admissionFor(j)
	if created {
		data, err := json.Marshal(want)
		if err != nil {
			return nil, ErrState
		}
		data = append(data, '\n')
		if n, err := file.Write(data); err != nil || n != len(data) {
			return nil, ErrState
		}
	}
	claim := &admissionClaim{file, root, directory, info, fileInfo}
	if !claim.matches(j) || file.Sync() != nil {
		return nil, ErrState
	}
	if syncDirectory(dir) != nil {
		return nil, ErrState
	}
	kept = true
	return claim, nil
}

// Called only while FileJournal's exclusive execution lease holds these exact
// handles open. The global flock lasts for the entire FileJournal lifetime.
func (c *admissionClaim) matches(j *FileJournal) bool {
	if c == nil {
		return false
	}
	directory, err := os.Lstat(c.directory)
	if err != nil || !ownedDirectory(directory, true) || !os.SameFile(directory, c.rootInfo) {
		return false
	}
	named, err := c.root.Lstat("admission.json")
	if err != nil || !privateFile(named, 0600) || !os.SameFile(named, c.fileInfo) {
		return false
	}
	opened, err := c.file.Stat()
	if err != nil || !privateFile(opened, 0600) || !os.SameFile(opened, c.fileInfo) || opened.Size() > 4096 {
		return false
	}
	data, err := io.ReadAll(io.NewSectionReader(c.file, 0, 4097))
	var record admissionRecord
	return err == nil && len(data) <= 4096 && DecodeStrict(data, &record) == nil && record == admissionFor(j)
}

func (c *admissionClaim) close() error {
	fileErr := c.file.Close()
	rootErr := c.root.Close()
	if fileErr != nil || rootErr != nil {
		return ErrState
	}
	return nil // The claim file is never deleted or reassigned here.
}
