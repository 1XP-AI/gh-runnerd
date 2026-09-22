//go:build g01_live && g01_pair_fixture

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

type fixtureEndpointConfig struct {
	BaseURL            string `json:"base_url"`
	CAPEM              string `json:"ca_pem"`
	AdmissionDirectory string `json:"admission_directory"`
}

func readFixtureEndpointConfig(stateDirectory string) (fixtureEndpointConfig, error) {
	var config fixtureEndpointConfig
	path := filepath.Join(stateDirectory, "paired-fixture.json")
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return config, livecanary.ErrApproval
	}
	defer file.Close()
	info, err := file.Stat()
	stat, ok := info.Sys().(*syscall.Stat_t)
	if err != nil || !ok || int(stat.Uid) != os.Geteuid() || info.Mode().Perm() != 0600 || !info.Mode().IsRegular() || stat.Nlink != 1 || info.Size() > 8192 {
		return config, livecanary.ErrApproval
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil || len(data) > 8192 || livecanary.DecodeStrict(data, &config) != nil || config.BaseURL == "" || config.CAPEM == "" || config.AdmissionDirectory == "" || !filepath.IsAbs(config.AdmissionDirectory) || filepath.Clean(config.AdmissionDirectory) != config.AdmissionDirectory {
		return fixtureEndpointConfig{}, livecanary.ErrApproval
	}
	return config, nil
}

func init() {
	openJournalForCommand = func(stateDirectory string, a livecanary.Approval) (*livecanary.FileJournal, error) {
		config, err := readFixtureEndpointConfig(stateDirectory)
		if err != nil {
			return nil, err
		}
		return livecanary.OpenJournalForPairedFixtureAt(stateDirectory, a, config.AdmissionDirectory)
	}
	pairedPrepareJournalForCommand = func(stateDirectory string, a livecanary.Approval) (livecanary.PreparationReceipt, error) {
		config, err := readFixtureEndpointConfig(stateDirectory)
		if err != nil {
			return livecanary.PreparationReceipt{}, err
		}
		return livecanary.PreparePairedJournalForFixtureAt(stateDirectory, a, config.AdmissionDirectory)
	}
	prepareWorkerJournalForCommand = func(stateDirectory string, a liveworker.Approval) (liveworker.PreparationReceipt, error) {
		// Worker admission is a distinct disposable fixture root. It is derived
		// from the worker state identity, never supplied by production approval
		// or the broker's controller admission root.
		workerStateReal, err := filepath.EvalSymlinks(stateDirectory)
		if err != nil || !filepath.IsAbs(workerStateReal) || filepath.Clean(workerStateReal) != workerStateReal {
			return liveworker.PreparationReceipt{}, liveworker.ErrState
		}
		admissionDirectory := filepath.Join(filepath.Dir(workerStateReal), "worker-admission")
		if err := os.Mkdir(admissionDirectory, 0700); err != nil && !os.IsExist(err) {
			return liveworker.PreparationReceipt{}, liveworker.ErrState
		}
		return livecanary.PrepareWorkerJournalForPairedFixtureAt(stateDirectory, a, admissionDirectory)
	}
	newSDKAPIForCommand = func(a livecanary.Approval, c livecanary.Credentials, stateDirectory string) (*livecanary.SDKAPI, error) {
		config, err := readFixtureEndpointConfig(stateDirectory)
		if err != nil {
			return nil, err
		}
		return livecanary.NewSDKAPIForPairedFixture(a, c, config.BaseURL, []byte(config.CAPEM))
	}
	runPairedTerminalForCommand = func(ctx context.Context, files livecanary.PairedTerminalFiles, c livecanary.Credentials) error {
		config, err := readFixtureEndpointConfig(files.ControllerStateDirectory)
		if err != nil {
			return err
		}
		return livecanary.RunPairedTerminalForFixture(ctx, files, c, config.BaseURL, []byte(config.CAPEM), config.AdmissionDirectory)
	}
}
