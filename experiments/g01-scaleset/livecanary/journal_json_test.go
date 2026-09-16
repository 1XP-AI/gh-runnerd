package livecanary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadApprovalAcceptsTrustedBrokerWorkflowRef(t *testing.T) {
	a := approval()
	a.WorkflowRef = "refs/heads/main"
	path := filepath.Join(t.TempDir(), "controller-approval.json")
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal("marshal approval")
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal("write approval")
	}
	got, err := ReadApproval(path)
	if err != nil {
		t.Fatalf("trusted broker approval with workflow_ref was rejected: %v", err)
	}
	if got.WorkflowRef != a.WorkflowRef {
		t.Fatalf("workflow_ref = %q, want %q", got.WorkflowRef, a.WorkflowRef)
	}
	if approvalDigest(got) != approvalDigest(a) {
		t.Fatal("workflow_ref was not retained in the approval digest binding")
	}
}

func TestStrictJSONRejectsDecoderEquivalentDuplicateFields(t *testing.T) {
	for _, data := range []string{
		`{"owner_nonce":"first","OWNER_NONCE":"second"}`,
		`{"phases":["inspect"],"PHASES":["create"]}`,
		`{"phases":["inspect"],"phaseſ":["create"]}`,
	} {
		var a Approval
		if DecodeStrict([]byte(data), &a) == nil {
			t.Error("decoder-equivalent duplicate authority fields accepted")
		}
	}
	var a Approval
	if DecodeStrict([]byte(`{"PHASES":["inspect"]}`), &a) != nil || len(a.Phases) != 1 || a.Phases[0] != "inspect" {
		t.Fatal("valid single alias no longer matches decoder")
	}
	var separated []Approval
	if DecodeStrict([]byte(`[{"phases":["inspect"]},{"PHASES":["cleanup"]}]`), &separated) != nil || len(separated) != 2 {
		t.Fatal("independent object scopes incorrectly collide")
	}
}
