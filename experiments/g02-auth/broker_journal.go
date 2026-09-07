package enrollment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
)

type brokerJournal struct {
	file *os.File
	root *os.Root
}

func openBrokerJournal(path string, a BrokerApproval) (j *brokerJournal, err error) {
	path = filepath.Clean(path)
	name := filepath.Base(path)
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return nil, errBroker
	}
	parent, e := openBrokerPrivateDirectory(filepath.Dir(path))
	if e != nil {
		return nil, errBroker
	}
	defer parent.Close()
	if e = parent.Mkdir(name, 0700); e != nil && !os.IsExist(e) {
		return nil, errBroker
	}
	info, e := parent.Lstat(name)
	if e != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, errBroker
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return nil, errBroker
	}
	root, e := parent.OpenRoot(name)
	if e != nil {
		return nil, errBroker
	}
	j = &brokerJournal{root: root}
	defer func() {
		if err != nil {
			j.close()
		}
	}()
	actual, e := root.Stat(".")
	if e != nil || !os.SameFile(info, actual) || syncDirectory(parent) != nil {
		return j, errBroker
	}
	// The same attempt is never silently retried, even after success. A separate
	// reviewed phase uses a new exact approval/attempt; prior intent remains.
	f, e := root.OpenFile("broker.jsonl", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return j, errBroker
	}
	j.file = f
	if !privateFile(f) || syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return j, errBroker
	}
	data, _ := json.Marshal(a)
	digest := sha256.Sum256(data)
	if j.append("prepared", map[string]any{"approval_sha256": hex.EncodeToString(digest[:]), "app_id": a.AppID, "installation_id": a.InstallationID, "organization_id": a.OrganizationID, "repository_id": a.RepositoryID, "runner_group_id": a.RunnerGroupID, "mode": a.Mode, "phase": a.Phase}) != nil || syncDirectory(root) != nil {
		return j, errBroker
	}
	return j, nil
}
func (j *brokerJournal) append(phase string, metadata map[string]any) error {
	record := struct {
		Phase    string         `json:"phase"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}{phase, metadata}
	data, _ := json.Marshal(record)
	data = append(data, '\n')
	if _, err := j.file.Write(data); err != nil {
		return errBroker
	}
	if j.file.Sync() != nil {
		return errBroker
	}
	return nil
}
func (j *brokerJournal) close() {
	if j == nil {
		return
	}
	if j.file != nil {
		_ = j.file.Close()
	}
	if j.root != nil {
		_ = j.root.Close()
	}
}
