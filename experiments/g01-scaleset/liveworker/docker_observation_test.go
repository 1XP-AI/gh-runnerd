package liveworker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type inspectFixtureResponse struct {
	status int
	body   []byte
	serve  func(http.ResponseWriter, *http.Request)
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
					f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusOK, body: data})
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

func createdInspectFixture(t *testing.T) (*dockerFixture, *Docker) {
	t.Helper()
	f := unixFixture(t)
	if f.driver.Run(context.Background(), "create", syntheticJIT) != nil {
		t.Fatal("synthetic create failed")
	}
	return f, f.driver.Runtime.(*Docker)
}

func changeInspectState(t *testing.T, f *dockerFixture, change func(map[string]json.RawMessage)) []byte {
	t.Helper()
	return inspectFixtureBody(t, f, func(body map[string]json.RawMessage) {
		var state map[string]json.RawMessage
		if json.Unmarshal(body["State"], &state) != nil {
			t.Fatal("synthetic state failed")
		}
		change(state)
		body["State"], _ = json.Marshal(state)
	})
}

func assertInspectUnknown(t *testing.T, observation DockerInspectObservation, err error) {
	t.Helper()
	if err != ErrRemote || observation.Outcome != DockerInspectUnknown || observation.State != nil || observation.Container != nil {
		t.Fatal("failed read produced usable observation or non-fixed error")
	}
}

func TestDockerInspectExactKnownStatesAndSerializableFacts(t *testing.T) {
	for _, status := range []ContainerStatus{ContainerStatusCreated, ContainerStatusRunning, ContainerStatusPaused, ContainerStatusRestarting, ContainerStatusRemoving, ContainerStatusExited, ContainerStatusDead} {
		t.Run(string(status), func(t *testing.T) {
			f, client := createdInspectFixture(t)
			data := changeInspectState(t, f, func(state map[string]json.RawMessage) {
				state["Status"], _ = json.Marshal(status)
				state["Running"], state["Paused"] = json.RawMessage("true"), json.RawMessage("true")
				state["ExitCode"] = json.RawMessage("-17")
				state["Error"] = json.RawMessage(`"synthetic-private-state-error"`)
			})
			f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusOK, body: data})
			observation, err := client.InspectExact(context.Background(), f.runtime.container.ID)
			if err != nil || observation.Outcome != DockerInspectPresent || observation.State == nil || observation.Container == nil {
				t.Fatal("known state observation failed")
			}
			state := observation.State
			if state.Status != status || state.Running == nil || !*state.Running || state.Paused == nil || !*state.Paused || state.Restarting == nil || *state.Restarting || state.Dead == nil || *state.Dead || state.ExitCode == nil || *state.ExitCode != -17 {
				t.Fatal("known state or present true/false/signed exit changed")
			}
			if observation.TargetID != f.runtime.container.ID || observation.Method != http.MethodGet || observation.Path != "/v1.45/containers/"+f.runtime.container.ID+"/json" || observation.HTTPStatus != http.StatusOK {
				t.Fatal("exact inspect response provenance changed")
			}
			if f.approval.verify(observation.Container, replay(f.driver.Journal.Events())) != nil {
				t.Fatal("raw container unavailable to existing complete profile verifier")
			}
			serialized, err := json.Marshal(observation)
			if err != nil {
				t.Fatal("facts encoding failed")
			}
			for _, secret := range []string{syntheticJIT, "synthetic-private", "ACTIONS_RUNNER_INPUT_JITCONFIG", "Config", "HostConfig", "Env", f.socket} {
				if strings.Contains(string(serialized), secret) {
					t.Fatal("serialized observation included raw profile, path or secret")
				}
			}
		})
	}
}

func TestDockerInspectExactStatePresenceAndLegacyRequirements(t *testing.T) {
	for _, field := range []string{"Status", "Running", "Paused", "Restarting", "Dead", "ExitCode"} {
		for _, shape := range []string{"missing", "null"} {
			t.Run(field+"/"+shape, func(t *testing.T) {
				f, client := createdInspectFixture(t)
				data := changeInspectState(t, f, func(state map[string]json.RawMessage) {
					if shape == "missing" {
						delete(state, field)
					} else {
						state[field] = json.RawMessage("null")
					}
				})
				f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusOK, body: data})
				observation, err := client.InspectExact(context.Background(), f.runtime.container.ID)
				if err != nil || observation.Outcome != DockerInspectPresent || observation.State == nil {
					t.Fatal("missing/null fact was not observable")
				}
				state := observation.State
				missing := map[string]bool{"Status": state.Status == ContainerStatusUnknown, "Running": state.Running == nil, "Paused": state.Paused == nil, "Restarting": state.Restarting == nil, "Dead": state.Dead == nil, "ExitCode": state.ExitCode == nil}
				if !missing[field] {
					t.Fatal("missing/null fact defaulted to a known value")
				}
				_, err = client.Inspect(context.Background(), f.runtime.container.ID)
				if (err == nil) != (field == "ExitCode") {
					t.Fatal("legacy presence policy changed")
				}
			})
		}
	}
}

func TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy(t *testing.T) {
	for _, status := range []string{"created", "exited"} {
		for _, exit := range []string{"missing", "null", "0", "-1", "9223372036854775807", "-9223372036854775808"} {
			t.Run(status+"/"+exit, func(t *testing.T) {
				f, client := createdInspectFixture(t)
				data := changeInspectState(t, f, func(state map[string]json.RawMessage) {
					state["Status"], _ = json.Marshal(status)
					if exit != "missing" {
						state["ExitCode"] = json.RawMessage(exit)
					}
				})
				f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusOK, body: data})
				observation, err := client.InspectExact(context.Background(), f.runtime.container.ID)
				if err != nil || observation.State == nil {
					t.Fatal("exit observation failed")
				}
				if exit == "null" || exit == "missing" {
					if observation.State.ExitCode != nil {
						t.Fatal("absent exit defaulted to zero")
					}
				} else {
					want, _ := strconv.ParseInt(exit, 10, 64)
					if observation.State.ExitCode == nil || *observation.State.ExitCode != want {
						t.Fatal("signed exit changed")
					}
				}
				if f.driver.Run(context.Background(), "cleanup", "") != nil || f.runtime.deletes.Load() != 1 {
					t.Fatal("legacy created/exited cleanup gained an exit-zero requirement")
				}
			})
		}
	}
}

func TestDockerInspectExactRejectsMalformedOrAmbiguousBodiesBeforeMutation(t *testing.T) {
	for _, action := range []string{"start", "cleanup"} {
		for _, fault := range []string{
			"id-missing", "id-null", "id-wrong", "id-type", "id-alias", "id-folded", "state-folded", "state-type", "status-type", "bool-string", "bool-number", "bool-array", "bool-object", "exit-string", "exit-fraction", "exit-exponent", "exit-overflow", "exit-underflow",
			"duplicate-top", "duplicate-state", "escaped-duplicate", "duplicate-map", "duplicate-label", "duplicate-unknown-array", "state-unicode-folded", "status-folded", "bool-folded", "bool-unicode-folded", "exit-folded", "network-folded", "trailing", "trailing-invalid", "truncated", "null", "array", "invalid-utf8", "too-deep", "oversize",
		} {
			t.Run(action+"/"+fault, func(t *testing.T) {
				f, client := createdInspectFixture(t)
				data := inspectFixtureBody(t, f, func(body map[string]json.RawMessage) {
					var state map[string]json.RawMessage
					_ = json.Unmarshal(body["State"], &state)
					switch fault {
					case "id-missing":
						delete(body, "Id")
					case "id-null":
						body["Id"] = json.RawMessage("null")
					case "id-wrong":
						body["Id"], _ = json.Marshal(strings.Repeat("d", 64))
					case "id-type":
						body["Id"] = json.RawMessage("1")
					case "id-alias":
						body["id"] = body["Id"]
						delete(body, "Id")
					case "id-folded":
						body["ID"] = body["Id"]
					case "state-folded":
						body["state"] = body["State"]
					case "state-unicode-folded":
						body["ſtate"] = body["State"]
					case "status-type":
						state["Status"] = json.RawMessage("false")
					case "bool-string":
						state["Running"] = json.RawMessage(`"synthetic-private-bool"`)
					case "bool-number":
						state["Paused"] = json.RawMessage("0")
					case "bool-array":
						state["Dead"] = json.RawMessage("[]")
					case "bool-object":
						state["Restarting"] = json.RawMessage("{}")
					case "exit-string":
						state["ExitCode"] = json.RawMessage(`"0"`)
					case "exit-fraction":
						state["ExitCode"] = json.RawMessage("0.5")
					case "exit-exponent":
						state["ExitCode"] = json.RawMessage("1e0")
					case "exit-overflow":
						state["ExitCode"] = json.RawMessage("9223372036854775808")
					case "exit-underflow":
						state["ExitCode"] = json.RawMessage("-9223372036854775809")
					case "status-folded":
						state["status"] = state["Status"]
					case "bool-folded":
						state["running"] = state["Running"]
					case "bool-unicode-folded":
						state["Reſtarting"] = state["Restarting"]
					case "exit-folded":
						state["ExitCode"], state["exitcode"] = json.RawMessage("0"), json.RawMessage("0")
					case "network-folded":
						body["NetworkSettings"] = json.RawMessage(`{"Networks":{"bridge":{}},"networks":{"bridge":{}}}`)
					}
					body["State"], _ = json.Marshal(state)
					if fault == "state-type" {
						body["State"] = json.RawMessage("[]")
					}
				})
				text := string(data)
				switch fault {
				case "duplicate-top":
					text = strings.Replace(text, `"Id":`, `"Id":"`+f.runtime.container.ID+`","Id":`, 1)
				case "duplicate-state":
					text = strings.Replace(text, `"Running":`, `"Running":false,"Running":`, 1)
				case "escaped-duplicate":
					text = strings.Replace(text, `"Running":`, `"\u0052unning":false,"Running":`, 1)
				case "duplicate-map":
					text = strings.Replace(text, `"HostConfig":{`, `"HostConfig":{"Extra":null,"Extra":null,`, 1)
				case "duplicate-label":
					text = strings.Replace(text, `"Labels":{`, `"Labels":{"Owner":"one","Owner":"two",`, 1)
				case "duplicate-unknown-array":
					text = `{"Unknown":[{"nested":[{"key":1,"key":2}]}],` + text[1:]
				case "trailing":
					text += " {}"
				case "trailing-invalid":
					text += " synthetic-private-tail"
				case "truncated":
					text = text[:len(text)-1]
				case "null":
					text = "null"
				case "array":
					text = "[]"
				case "invalid-utf8":
					text = `{"Unknown":"` + string([]byte{0xff}) + `",` + text[1:]
				case "too-deep":
					text = `{"Unknown":` + strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66) + "," + text[1:]
				case "oversize":
					text = strings.Repeat(" ", responseLimit) + text
				}
				f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusOK, body: []byte(text)})
				observation, err := client.InspectExact(context.Background(), f.runtime.container.ID)
				assertInspectUnknown(t, observation, err)
				if observation.HTTPStatus != http.StatusOK {
					t.Fatal("received HTTP status was lost")
				}
				if !errors.Is(f.driver.Run(context.Background(), action, ""), ErrUncertain) || f.runtime.starts.Load()+f.runtime.deletes.Load() != 0 {
					t.Fatal("rejected observation authorized a mutating request")
				}
			})
		}
	}
}

func TestDockerInspectUnknownOrAbsentStatusCannotAuthorizeMutation(t *testing.T) {
	for _, fault := range []string{"missing-state", "null-state", "missing-status", "null-status", "unknown-status"} {
		for _, action := range []string{"start", "cleanup"} {
			t.Run(fault+"/"+action, func(t *testing.T) {
				f, client := createdInspectFixture(t)
				data := inspectFixtureBody(t, f, func(body map[string]json.RawMessage) {
					var state map[string]json.RawMessage
					_ = json.Unmarshal(body["State"], &state)
					delete(state, "Status")
					if fault == "null-status" {
						state["Status"] = json.RawMessage("null")
					} else if fault == "unknown-status" {
						state["Status"] = json.RawMessage(`"synthetic-private-unknown-status"`)
					}
					body["State"], _ = json.Marshal(state)
					if fault == "missing-state" {
						delete(body, "State")
					} else if fault == "null-state" {
						body["State"] = json.RawMessage("null")
					}
				})
				f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusOK, body: data})
				observation, err := client.InspectExact(context.Background(), f.runtime.container.ID)
				if err != nil || observation.Outcome != DockerInspectPresent || observation.Container == nil || observation.Container.State.Status != string(ContainerStatusUnknown) {
					t.Fatal("unknown status was not normalized")
				}
				if fault == "missing-state" || fault == "null-state" {
					if observation.State != nil {
						t.Fatal("missing/null state was fabricated")
					}
				} else if observation.State == nil || observation.State.Status != ContainerStatusUnknown {
					t.Fatal("unknown status was lost")
				}
				serialized, _ := json.Marshal(observation)
				if strings.Contains(string(serialized), "synthetic-private") {
					t.Fatal("raw unknown status leaked")
				}
				if !errors.Is(f.driver.Run(context.Background(), action, ""), ErrUncertain) || f.runtime.starts.Load()+f.runtime.deletes.Load() != 0 {
					t.Fatal("unknown/missing status authorized mutation")
				}
			})
		}
	}
}

func TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile(t *testing.T) {
	f := unixFixture(t)
	f.runtime.image.Labels = map[string]string{"Owner": "one", "owner": "two", "S": "three", "ſ": "four"}
	if f.driver.Run(context.Background(), "create", syntheticJIT) != nil {
		t.Fatal("case-sensitive image-label setup failed")
	}
	data := inspectFixtureBody(t, f, func(body map[string]json.RawMessage) {
		var config map[string]json.RawMessage
		_ = json.Unmarshal(body["Config"], &config)
		config["Extension"], config["extension"] = json.RawMessage("null"), json.RawMessage("0")
		body["Config"], _ = json.Marshal(config)
	})
	f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusOK, body: data})
	if f.driver.Run(context.Background(), "start", "") != nil || f.runtime.starts.Load() != 1 {
		t.Fatal("case-sensitive map keys incorrectly folded or complete profile changed")
	}
}

func TestDockerInspectExactNotFoundReportsOnlyTheExactGET(t *testing.T) {
	f := unixFixture(t)
	client := f.driver.Runtime.(*Docker)
	target := strings.Repeat("c", 64)
	wantPath := "/v1.45/containers/" + target + "/json"
	f.inspectResponse.Store(&inspectFixtureResponse{serve: func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1))
		if r.Method != http.MethodGet || r.URL.RequestURI() != wantPath || err != nil || len(body) != 0 {
			t.Error("observation reached a different method/path or supplied a body")
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"synthetic-private-error-body"}`)
	}})
	// No Preflight or create precedes this observation; it cannot imply either.
	observation, err := client.InspectExact(context.Background(), target)
	if err != nil || observation.Outcome != DockerInspectNotFoundReported || observation.HTTPStatus != http.StatusNotFound || observation.TargetID != target || observation.Method != http.MethodGet || observation.Path != wantPath || observation.State != nil || observation.Container != nil || f.requests.Load() != 1 {
		t.Fatal("supported exact GET did not produce only not-found-reported")
	}
	serialized, _ := json.Marshal(observation)
	if strings.Contains(string(serialized), "synthetic-private") {
		t.Fatal("404 error body was exposed")
	}
	if result, err := client.Inspect(context.Background(), target); result != nil || err != ErrRemote {
		t.Fatal("legacy inspect accepted reported absence")
	}
}

func TestDockerInspectExactRejectsOtherResponsesAndInvalidTargets(t *testing.T) {
	for _, status := range []int{201, 204, 301, 302, 307, 400, 401, 403, 409, 500} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			f, client := createdInspectFixture(t)
			f.inspectResponse.Store(&inspectFixtureResponse{status: status, body: []byte(`{"message":"synthetic-private-error"}`)})
			before := f.requests.Load()
			observation, err := client.InspectExact(context.Background(), f.runtime.container.ID)
			assertInspectUnknown(t, observation, err)
			if observation.HTTPStatus != status || f.requests.Load() != before+1 {
				t.Fatal("unsupported status lost or redirect followed")
			}
			if f.driver.Run(context.Background(), "cleanup", "") == nil || f.runtime.deletes.Load() != 0 {
				t.Fatal("unsupported response authorized deletion")
			}
		})
	}
	for _, target := range []string{"", "short", "../synthetic-private-path", strings.Repeat("C", 64), strings.Repeat("c", 63), strings.Repeat("c", 65)} {
		f := unixFixture(t)
		observation, err := f.driver.Runtime.(*Docker).InspectExact(context.Background(), target)
		if err != ErrApproval || observation.TargetID != "" || f.requests.Load() != 0 {
			t.Fatal("invalid target reached the endpoint or was exposed")
		}
	}
}

func TestDockerInspectExactRequiresSupported404Body(t *testing.T) {
	for name, body := range map[string]string{
		"empty": "", "null": "null", "missing-message": "{}", "null-message": `{"message":null}`, "wrong-message": `{"message":1}`,
		"duplicate": `{"message":"one","message":"two"}`, "folded": `{"Message":"one"}`, "extra": `{"message":"one","extra":true}`,
		"trailing": `{"message":"one"} {}`, "truncated": `{"message":`, "oversize": `{"message":"` + strings.Repeat("s", responseLimit) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			f, client := createdInspectFixture(t)
			f.inspectResponse.Store(&inspectFixtureResponse{status: http.StatusNotFound, body: []byte(body)})
			observation, err := client.InspectExact(context.Background(), f.runtime.container.ID)
			assertInspectUnknown(t, observation, err)
			if observation.HTTPStatus != http.StatusNotFound {
				t.Fatal("received 404 provenance lost")
			}
			if f.driver.Run(context.Background(), "cleanup", "") == nil || f.runtime.deletes.Load() != 0 {
				t.Fatal("unsupported 404 body authorized deletion")
			}
		})
	}
}

func TestDockerInspectExactCancellationNeverReportsPresenceOrAbsence(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			f := unixFixture(t)
			client := f.driver.Runtime.(*Docker)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{})
			f.inspectResponse.Store(&inspectFixtureResponse{serve: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"message":"`)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}})
			type result struct {
				observation DockerInspectObservation
				err         error
			}
			done := make(chan result, 1)
			go func() {
				observation, err := client.InspectExact(ctx, strings.Repeat("c", 64))
				done <- result{observation, err}
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("synthetic response did not start")
			}
			cancel()
			select {
			case got := <-done:
				assertInspectUnknown(t, got.observation, got.err)
			case <-time.After(time.Second):
				t.Fatal("cancelled inspect did not return")
			}
		})
	}
	f := unixFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	observation, err := f.driver.Runtime.(*Docker).InspectExact(ctx, strings.Repeat("c", 64))
	assertInspectUnknown(t, observation, err)
	if f.requests.Load() != 0 {
		t.Fatal("already-cancelled observation reached the socket")
	}
}

func TestDockerInspectExactRejectsReplacedSocket(t *testing.T) {
	f, client := createdInspectFixture(t)
	if _, err := client.InspectExact(context.Background(), f.runtime.container.ID); err != nil {
		t.Fatal("initial pinned inspect failed")
	}
	replacement := replaceSocket(t, f.socket)
	observation, err := client.InspectExact(context.Background(), f.runtime.container.ID)
	assertInspectUnknown(t, observation, err)
	if replacement.calls.Load() != 0 {
		t.Fatal("replacement socket received exact observation")
	}
}
