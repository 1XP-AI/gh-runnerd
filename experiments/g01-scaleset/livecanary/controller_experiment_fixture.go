//go:build g01_pair_fixture

package livecanary

import (
	"path/filepath"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// ConfigureControllerExperimentForFixture exists only in builds refused by the
// production broker's exact tag admission. It selects concrete offline
// constructors, with no callback dispatcher or production authority setter.
func ConfigureControllerExperimentForFixture(policy, baseURL string, caPEM []byte, admission string, workerAdmission ...string) error {
	if _, _, _, e := handoff.DecodePublicPolicy([]byte(policy), "controller", time.Now()); e != nil {
		return ErrApproval
	}
	if _, _, _, e := fixtureEndpoint(baseURL, caPEM); e != nil {
		return ErrApproval
	}
	if !filepath.IsAbs(admission) || filepath.Clean(admission) != admission {
		return ErrApproval
	}
	controllerExperimentPolicyJSON = policy
	controllerExperimentFixtureOpen = func(path string, a Approval) (*FileJournal, error) {
		return OpenJournalForPairedFixtureAt(path, a, admission)
	}
	controllerExperimentFixtureAPI = func(a Approval, c Credentials) (API, error) { return NewSDKAPIForPairedFixture(a, c, baseURL, caPEM) }
	pairedTerminalFixtureAdapters = &pairedTerminalAdapters{
		openController: controllerExperimentFixtureOpen,
		openWorker: func(path string, a liveworker.Approval) (*liveworker.FileJournal, error) {
			root := filepath.Join(filepath.Dir(path), "worker-admission")
			if len(workerAdmission) == 1 && workerAdmission[0] != "" {
				root = workerAdmission[0]
			}
			return liveworker.OpenJournalForPairedFixture(path, a, root)
		},
		newAPI: func(a Approval, c Credentials) (*SDKAPI, error) {
			return NewSDKAPIForPairedFixture(a, c, baseURL, caPEM)
		},
		newDocker: liveworker.NewDocker,
	}
	pairedTerminalFixtureCadence = fixtureFastPairCadence
	return nil
}

func PrepareJournalForExperimentFixture(path string, a Approval, phase, admission string) (PreparationReceipt, error) {
	return prepareJournal(path, a, phase, func(path string, a Approval) (*FileJournal, error) {
		return OpenJournalForPairedFixtureAt(path, a, admission)
	})
}
