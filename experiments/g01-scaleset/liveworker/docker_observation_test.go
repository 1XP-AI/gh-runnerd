package liveworker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

type inspectFixtureResponse struct {
	status int
	body   []byte
}

func inspectFixtureBody(t *testing.T, f *dockerFixture, change func(map[string]json.RawMessage)) []byte {
	t.Helper()
	data, err := json.Marshal(f.runtime.container)
	if err != nil {
		t.Fatal("synthetic inspect encoding failed")
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(data, &body) != nil {
		t.Fatal("synthetic inspect object failed")
	}
	change(body)
	data, err = json.Marshal(body)
	if err != nil {
		t.Fatal("synthetic inspect encoding failed")
	}
	return data
}

// Full valid profile receipts reach the actual Unix HTTP adapter; only one
// decision field is removed/null. A Go struct fixture would erase this fault.
func TestUnixInspectRequiresStateFlagsBeforeMutation(t *testing.T) {
	for _, action := range []struct{ phase, status string }{{"start", "created"}, {"cleanup", "created"}, {"cleanup", "exited"}} {
		for _, field := range []string{"Running", "Paused", "Restarting", "Dead"} {
			for _, shape := range []string{"complete", "missing", "null"} {
				t.Run(action.phase+"/"+action.status+"/"+field+"/"+shape, func(t *testing.T) {
					f := unixFixture(t)
					if f.driver.Run(context.Background(), "create", syntheticJIT) != nil {
						t.Fatal("synthetic create failed")
					}
					data := inspectFixtureBody(t, f, func(body map[string]json.RawMessage) {
						var state map[string]json.RawMessage
						if json.Unmarshal(body["State"], &state) != nil {
							t.Fatal("synthetic state failed")
						}
						state["Status"], _ = json.Marshal(action.status)
						if shape == "missing" {
							delete(state, field)
						} else if shape == "null" {
							state[field] = json.RawMessage("null")
						}
						body["State"], _ = json.Marshal(state)
					})
					f.inspectResponse.Store(&inspectFixtureResponse{http.StatusOK, data})
					err := f.driver.Run(context.Background(), action.phase, "")
					mutations := f.runtime.starts.Load() + f.runtime.deletes.Load()
					if shape == "complete" {
						if err != nil || mutations != 1 {
							t.Fatal("complete-state positive control refused")
						}
					} else if !errors.Is(err, ErrUncertain) || mutations != 0 {
						t.Errorf("missing/null state authorized mutation: error=%v mutations=%d", err, mutations)
					}
				})
			}
		}
	}
}
