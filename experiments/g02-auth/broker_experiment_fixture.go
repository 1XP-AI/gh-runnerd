//go:build g01_pair_fixture

package enrollment

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// This test-only setter is absent from every admissible production binary.
func ConfigureBrokerExperimentForFixture(policy string, transport http.RoundTripper, admission string, workerAdmission ...string) error {
	if _, _, _, e := handoff.DecodePublicPolicy([]byte(policy), "broker", time.Now()); e != nil {
		return errBroker
	}
	if transport == nil || !filepath.IsAbs(admission) || filepath.Clean(admission) != admission {
		return errBroker
	}
	if len(workerAdmission) == 1 && workerAdmission[0] != "" {
		root := workerAdmission[0]
		resolveWorkerClaimDirectory = func() (string, error) { return root, nil }
	}
	brokerExperimentPolicyJSON = policy
	brokerExperimentFixtureAPI = func(root handoff.TrustRoot) *brokerAPI {
		api := newBrokerAPIWithProvenanceRoot(time.Now, transport, root)
		api.admissionDirectory = func() (string, error) { return admission, nil }
		return api
	}
	brokerBinaryOpener = func(path string, a BrokerApproval) (*verifiedBrokerBinary, error) {
		f, e := openBrokerPrivateFile(path, 0500, 128<<20)
		if e != nil {
			return nil, errBroker
		}
		b := &verifiedBrokerBinary{path, f, a.ControllerBinarySHA256}
		if b.check() != nil {
			f.Close()
			return nil, errBroker
		}
		return b, nil
	}
	return nil
}
