package enrollment

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

type managementScopeProvenanceProbe struct {
	BrokerProvenanceAdapter
	calls int
}

func (p *managementScopeProvenanceProbe) Source() string {
	p.calls++
	return p.BrokerProvenanceAdapter.Source()
}

func TestBrokerManagementScopeRefusesBeforeCredentialRead(t *testing.T) {
	for _, test := range []struct{ broker, controller string }{{"all", ""}, {"organization-only", ""}, {"", "organization-only"}} {
		t.Run(test.broker+"/"+test.controller, func(t *testing.T) {
			e := newPairedBrokerEntryFixture(t)
			a, err := brokerApprovalFromEntryFixture(t, e)
			if err != nil {
				t.Fatal(err)
			}
			var controller controllerApproval
			if _, err := readBrokerPrivateJSON(e.files.ControllerApproval, &controller); err != nil {
				t.Fatal(err)
			}
			a.ManagementScope, controller.ManagementScope = test.broker, test.controller
			data, err := json.Marshal(controller)
			if err != nil || os.WriteFile(e.files.ControllerApproval, data, 0600) != nil {
				t.Fatal("controller scope fixture")
			}
			a.ControllerApprovalSHA256 = brokerBytesDigest(data)
			data, err = json.Marshal(a)
			if err != nil || os.WriteFile(e.files.ApprovalPath, data, 0600) != nil {
				t.Fatal("broker scope fixture")
			}
			probe := &managementScopeProvenanceProbe{BrokerProvenanceAdapter: e.api.provenance}
			e.api.provenance = probe
			input, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			defer writer.Close() // No credential bytes; the held writer would block a read.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := runBrokerWithAPI(ctx, e.files, input, e.api)
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil || probe.calls != 0 || len(e.fixture.calls) != 0 || e.fixture.tokenCalls != 0 {
					t.Fatal("scope refusal crossed credential/provenance/API boundary")
				}
			case <-time.After(300 * time.Millisecond):
				cancel()
				select {
				case <-done:
					t.Fatal("scope mismatch reached credential input")
				case <-time.After(3 * time.Second):
					t.Fatal("scope refusal remained blocked")
				}
			}
		})
	}
}

func TestManagementScopeLegacyBrokerAndControllerGolden(t *testing.T) {
	// Original field order and tags, before management_scope existed. These
	// fixed bytes are also valid input to old readers; no timestamps are renewed.
	legacy := `{"owner_nonce":"` + strings.Repeat("a", 32) + `","mode":"discover-actions-host","app_id":71,"app_name":"synthetic-app","app_owner":"org-a","app_owner_id":101,"installation_id":201,"organization":"org-a","organization_id":101,"repository":"canary","repository_id":501,"runner_group_id":3,"runner_group_name":"synthetic-group","expires_at":"2026-10-25T00:00:00Z","allow_verification_authority":false}`
	var approval BrokerApproval
	if decodeBrokerJSON([]byte(legacy), &approval, true) != nil {
		t.Fatal("legacy broker approval no longer decodes")
	}
	data, err := json.Marshal(approval)
	if err != nil || string(data) != legacy {
		t.Fatal("empty scope changed legacy broker approval bytes")
	}
	resource := strings.Replace(legacy, `"mode":"discover-actions-host"`, `"mode":""`, 1)
	resource = strings.Replace(resource, `"expires_at":"2026-10-25T00:00:00Z"`, `"expires_at":"0001-01-01T00:00:00Z"`, 1)
	if brokerResource(approval) != brokerBytesDigest([]byte(resource)) {
		t.Fatal("empty scope changed legacy experiment resource digest")
	}
	approval.ManagementScope = "organization-only"
	if brokerResource(approval) == brokerBytesDigest([]byte(resource)) {
		t.Fatal("management scope omitted from resource binding")
	}
	controllerJSON := `{"app_id":71,"installation_id":201,"organization":"org-a","repository":"canary","repository_id":501,"runner_group_id":3,"owner_nonce":"` + strings.Repeat("a", 32) + `","harness_sha":"` + strings.Repeat("b", 40) + `","workflow_sha":"` + strings.Repeat("c", 40) + `","workflow_ref":"refs/heads/main","workflow_path":".github/workflows/canary.yml","workflow_run_id":7,"controller":"trusted-controller","expires_at":"2026-10-25T00:00:00Z","actions_hosts":["fixture.actions.githubusercontent.com"],"phases":["create"]}`
	var controller controllerApproval
	if decodeBrokerJSON([]byte(controllerJSON), &controller, true) != nil {
		t.Fatal("legacy controller approval no longer decodes")
	}
	data, err = json.Marshal(controller)
	if err != nil || string(data) != controllerJSON || brokerDigest(controller) != brokerBytesDigest([]byte(controllerJSON)) {
		t.Fatal("empty scope changed legacy controller bytes or digest")
	}
	controller.ManagementScope = "organization-only"
	if brokerDigest(controller) == brokerBytesDigest([]byte(controllerJSON)) {
		t.Fatal("scope omitted from controller approval digest")
	}
}
