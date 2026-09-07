package enrollment

import (
	"os"
	"path/filepath"
	"regexp"
	"syscall"
)

var brokerSHA40 = regexp.MustCompile(`^[a-f0-9]{40}$`)
var brokerSHA256 = regexp.MustCompile(`^[a-f0-9]{64}$`)

func openBrokerPrivateFile(path string, mode os.FileMode, limit int64) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, errBroker
	}
	parent, err := openBrokerPrivateDirectory(filepath.Dir(path))
	if err != nil {
		return nil, errBroker
	}
	defer parent.Close()
	f, err := parent.OpenFile(filepath.Base(path), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errBroker
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, errBroker
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Getuid()) || s.Nlink != 1 || info.Mode() != mode || info.Size() > limit || info.Size() < 1 {
		f.Close()
		return nil, errBroker
	}
	return f, nil
}
func openBrokerPrivateDirectory(path string) (*os.Root, error) {
	if !filepath.IsAbs(path) {
		return nil, errBroker
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, errBroker
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Getuid()) {
		return nil, errBroker
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, errBroker
	}
	actual, err := root.Stat(".")
	if err != nil || !os.SameFile(actual, info) {
		root.Close()
		return nil, errBroker
	}
	return root, nil
}

type BrokerFiles struct{ ApprovalPath, StateDirectory, ControllerBinary, ControllerApproval, ControllerStateDirectory string }
