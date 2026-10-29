package enrollment

import (
	"strings"
	"testing"
	"time"
)

func TestSessionReplacementBrokerRequiresOriginalPhaseAndVerification(t *testing.T) {
	a := brokerApprovalFixture()
	a.Mode = "controller"
	a.Phase = "session-replacement"
	a.AllowVerificationAuthority = true
	a.ControllerHarnessSHA = strings.Repeat("a", 40)
	c := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: a.OwnerNonce, HarnessSHA: a.ControllerHarnessSHA, WorkflowSHA: strings.Repeat("b", 40), WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 7, Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "session-replacement"}}
	if c.validate(a, time.Now()) != nil || !c.needsVerification() {
		t.Fatal("replacement contract refused")
	}
	a.AllowVerificationAuthority = false
	if c.validate(a, time.Now()) == nil {
		t.Fatal("replacement omitted independent workflow verification")
	}
	a.AllowVerificationAuthority = true
	c.Phases = []string{"create"}
	if c.validate(a, time.Now()) == nil {
		t.Fatal("old phase set authorized replacement")
	}
}
