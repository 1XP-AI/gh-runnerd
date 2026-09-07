package enrollment

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

type verifiedBrokerBinary struct {
	path   string
	file   *os.File
	digest string
}

func validBrokerBuild(info *debug.BuildInfo, expected string) bool {
	if info == nil || info.GoVersion != "go1.26.8" || info.Path != "github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/cmd/g01-live" || !brokerSHA40.MatchString(expected) {
		return false
	}
	revision, clean, sdk := "", false, false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			clean = setting.Value == "false"
		}
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/actions/scaleset" {
			sdk = dep.Version == "v0.4.0" && dep.Replace == nil
		}
	}
	return revision == expected && clean && sdk
}
func openBrokerBinary(path string, a BrokerApproval) (*verifiedBrokerBinary, error) {
	f, err := openBrokerPrivateFile(path, 0500, 128<<20)
	if err != nil {
		return nil, errBroker
	}
	binary := &verifiedBrokerBinary{path: path, file: f, digest: a.ControllerBinarySHA256}
	info, err := buildinfo.Read(f)
	if err != nil || !validBrokerBuild(info, a.ControllerHarnessSHA) || binary.check() != nil {
		f.Close()
		return nil, errBroker
	}
	return binary, nil
}
func (b *verifiedBrokerBinary) check() error {
	if b == nil || b.file == nil || !filepath.IsAbs(b.path) || !brokerSHA256.MatchString(b.digest) {
		return errBroker
	}
	actual, err := b.file.Stat()
	named, e := os.Lstat(b.path)
	if err != nil || e != nil || !os.SameFile(actual, named) || actual.Mode() != 0500 || actual.Size() < 1 || actual.Size() > 128<<20 {
		return errBroker
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, io.NewSectionReader(b.file, 0, actual.Size())); err != nil || hex.EncodeToString(hash.Sum(nil)) != b.digest {
		return errBroker
	}
	return nil
}

type brokerOutputBudget struct {
	mu       sync.Mutex
	n        int
	overflow bool
	cancel   context.CancelFunc
}

func (b *brokerOutputBudget) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n += len(p)
	if b.n > 8192 {
		b.overflow = true
		b.cancel()
		return 0, errBroker
	}
	return len(p), nil
}
func invokeBrokerController(parent context.Context, binary *verifiedBrokerBinary, workingDirectory, approvalPath, stateDirectory, phase string, data []byte) error {
	if !brokerPhases[phase] || len(data) > 16384 || binary.check() != nil {
		return errBroker
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	// No shell, PATH lookup, worker, gh command or inherited credential variables.
	command := exec.CommandContext(ctx, binary.path, "--execute-approved-canary", "--approval", approvalPath, "--state-dir", stateDirectory, "--phase", phase)
	command.Dir = workingDirectory
	command.Env = []string{"LANG=C", "LC_ALL=C"}
	command.WaitDelay = time.Second
	output := &brokerOutputBudget{cancel: cancel}
	command.Stdout = output
	command.Stderr = output
	pipe, err := command.StdinPipe()
	if err != nil {
		return errBroker
	}
	if command.Start() != nil {
		pipe.Close()
		return errBroker
	}
	wrote := make(chan error, 1)
	go func() {
		_, e := pipe.Write(data)
		closeErr := pipe.Close()
		if e == nil {
			e = closeErr
		}
		wrote <- e
	}()
	waited := command.Wait()
	writeErr := <-wrote
	output.mu.Lock()
	overflow := output.overflow
	output.mu.Unlock()
	if waited != nil || writeErr != nil || overflow || ctx.Err() != nil {
		return errBroker
	}
	return nil
}
