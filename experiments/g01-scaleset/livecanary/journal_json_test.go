package livecanary

import "testing"

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
