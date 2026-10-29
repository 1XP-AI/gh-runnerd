package enrollment

import (
	"strings"
	"testing"
	"time"
)

func TestJITBoundaryBrokerFinitePhaseAndVerification(t *testing.T) {
	for _, phase := range []string{"before-jit", "jit-loss"} {
		t.Run(phase, func(t *testing.T) {
			a := brokerApprovalFixture()
			a.Mode = "controller"
			a.Phase = phase
			a.AllowVerificationAuthority = phase == "before-jit"
			a.ControllerHarnessSHA = strings.Repeat("a", 40)
			c := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: a.ControllerHarnessSHA, WorkflowSHA: strings.Repeat("b", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", phase}}
			if c.validate(a, time.Now()) != nil || c.needsVerification() != (phase == "before-jit") {
				t.Fatal("finite JIT credential shape rejected")
			}
			if phase == "before-jit" {
				a.AllowVerificationAuthority = false
				if c.validate(a, time.Now()) == nil {
					t.Fatal("before-jit omitted separate verification")
				}
				a.AllowVerificationAuthority = true
			}
			c.Phases = []string{"create"}
			if c.validate(a, time.Now()) == nil {
				t.Fatal("original phase grant widened")
			}
		})
	}
}
