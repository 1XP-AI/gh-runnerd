package livecanary

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/actions/scaleset"
)

func TestBaselineMessageZeroIsNotMissingOrNegative(t *testing.T) {
	for _, kind := range []string{"zero", "missing", "null", "negative"} {
		t.Run(kind, func(t *testing.T) {
			items, _ := json.Marshal([]any{baselineFixtureItem(approval(), "JobAvailable")})
			body := map[string]any{"messageId": 0, "messageType": "RunnerScaleSetJobMessages", "body": string(items), "statistics": scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}}
			switch kind {
			case "missing":
				delete(body, "messageId")
			case "null":
				body["messageId"] = nil
			case "negative":
				body["messageId"] = -1
			}
			data, _ := json.Marshal(body)
			batch, err := decodeBaselineBatch(data)
			if kind == "zero" {
				if err != nil || batch == nil || batch.MessageID != 0 {
					t.Fatal("present message zero rejected")
				}
			} else if err == nil {
				t.Fatal("missing/null/negative message identity accepted as zero")
			}
		})
	}
}

func TestBaselineMalformedMessageIdentityNeverACKs(t *testing.T) {
	for _, kind := range []string{"missing", "null", "negative", "repeated-zero"} {
		t.Run(kind, func(t *testing.T) {
			f := newBaselineFixture(t, func(stage string, value any) any {
				if stage == "poll1" || kind == "repeated-zero" && stage == "poll2" {
					body := value.(map[string]any)
					switch kind {
					case "missing":
						delete(body, "messageId")
					case "null":
						body["messageId"] = nil
					case "negative":
						body["messageId"] = -1
					case "repeated-zero":
						body["messageId"] = 0
					}
				}
				return value
			})
			wantACK, wantAcquire := int32(0), int32(0)
			if kind == "repeated-zero" {
				wantACK, wantAcquire = 1, 1
			}
			if err := f.run(t); err == nil || f.acks.Load() != wantACK || f.acquires.Load() != wantAcquire || f.forbidden.Load() != 0 {
				t.Fatal("invalid or repeated identity was ACKed/acquired")
			}
		})
	}
}

func TestDurableBatchRequiresPresentNonnegativeMessageIdentity(t *testing.T) {
	for _, field := range []string{`"message_id":0,`, ``, `"message_id":null,`, `"message_id":-1,`, `"message_id":0,"Message_ID":0,`} {
		var b baselineBatch
		err := DecodeStrict([]byte(`{`+field+`"statistics":null,"items":[]}`), &b)
		if (err == nil) != (field == `"message_id":0,`) {
			t.Fatal("durable batch lost the distinction between present zero and invalid identity")
		}
	}
}

func TestPinnedSDKDrainMessageZeroAndMalformedIdentity(t *testing.T) {
	for _, field := range []string{`"messageId":0,`, ``, `"messageId":null,`, `"messageId":-1,`, `"messageId":0,"MessageId":0,`} {
		t.Run(field, func(t *testing.T) {
			a := approval()
			body, _ := json.Marshal(pinnedDrainJobBody(a))
			stats, _ := json.Marshal(pinnedDrainStatistics(1, 1))
			wire := `{` + field + `"messageType":"RunnerScaleSetJobMessages","body":` + string(body) + `,"statistics":` + string(stats) + `}`
			f, _, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{firstPollBody: wire, acceptAnyACK: true, mutateRequest: func(req *http.Request) {
				if req.Method == http.MethodDelete && strings.HasPrefix(req.URL.Path, "/queue/") && req.URL.Path != "/queue/0" {
					t.Error("ACK did not target message zero")
				}
			}})
			observation, err := runDrainListener(context.Background(), session, 7, hook)
			if field != `"messageId":0,` {
				if err == nil || f.acks.Load() != 0 || f.acquires.Load() != 0 {
					t.Fatal("invalid identity reached drain effects")
				}
				return
			}
			if err != nil || observation.Outcome != drainOutcomeObserved || f.acks.Load() != 1 || f.acquires.Load() != 1 || f.polls.Load() != 2 || !slices.Equal(f.cursorsSnapshot(), []string{"", ""}) || !slices.Equal(f.capacitiesSnapshot(), []string{"1", "0"}) {
				t.Fatalf("zero drain did not ACK/acquire/withdraw with the SDK's omitted zero cursor: err=%v", err)
			}
		})
	}
}

func TestQueueIdentityGuardPreservesBytesAndRejectsAmbiguity(t *testing.T) {
	for _, field := range []string{`"messageId":0,`, `"messageId":8,`, ``, `"messageId":null,`, `"messageId":-1,`, `"messageId":0,"messageId":0,`, `"messageId":0,"MessageId":0,`, `"messageId":0.1,`, `"messageId":9223372036854775808,`} {
		body := `{ ` + field + ` "other": "untouched bytes" }`
		req, _ := http.NewRequest(http.MethodGet, "http://fixture.invalid/queue", nil)
		req.Header.Set(scaleset.HeaderScaleSetMaxCapacity, "1")
		source := &baselineCloseErrorBody{Reader: strings.NewReader(body)}
		response, err := guardQueueMessageIdentity(req, &http.Response{StatusCode: http.StatusOK, Body: source})
		valid := field == `"messageId":0,` || field == `"messageId":8,`
		if (err == nil) != valid {
			t.Fatal("queue identity presence/domain/ambiguity not enforced")
		}
		if valid {
			data, err := readBaselineRequestBody(response.Body)
			if err != nil || string(data) != body {
				t.Fatal("identity guard altered SDK response bytes")
			}
		}
	}
}

func TestPinnedSDKJournaledDrainZeroIdentity(t *testing.T) {
	a := approval()
	body, _ := json.Marshal(pinnedDrainJobBody(a))
	stats, _ := json.Marshal(pinnedDrainStatistics(1, 1))
	wire := `{"messageId":0,"messageType":"RunnerScaleSetJobMessages","body":` + string(body) + `,"statistics":` + string(stats) + `}`
	f, base, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{firstPollBody: wire, acceptAnyACK: true})
	j := &memoryJournal{}
	d := &Driver{Approval: a, Journal: j, API: fixtureSDK{base}}
	client := &journaledDrainClient{d: d, inner: session, sessionID: session.Session().SessionID.String(), setID: 7, hook: hook}
	observation, err := runDrainListener(context.Background(), client, 7, hook)
	if err != nil || observation.Outcome != drainOutcomeObserved || f.acks.Load() != 1 || f.acquires.Load() != 1 {
		t.Fatalf("journaled drain rejected a validated zero identity: %v", err)
	}
	var knownACK bool
	for _, e := range j.Events() {
		if e.Kind == "result" && e.Operation == "ack" && e.ID == 0 && e.SessionID == client.sessionID {
			knownACK = true
		}
	}
	if !knownACK {
		t.Fatal("zero ACK was not durably classified")
	}
}

func TestQueueIdentityGuardRetainsBodyErrorAndBudgetGates(t *testing.T) {
	for _, kind := range []string{"read-error", "close-error", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			data := `{"messageId":0}`
			var source io.ReadCloser = &sdkRequestBodyFault{data: []byte(data), err: io.ErrUnexpectedEOF}
			if kind == "close-error" {
				source = &baselineCloseErrorBody{Reader: strings.NewReader(data), err: io.ErrClosedPipe}
			}
			if kind == "oversize" {
				source = io.NopCloser(strings.NewReader(`{"messageId":0,"extra":"` + strings.Repeat("x", int(responseBodyLimit)) + `"}`))
			}
			req, _ := http.NewRequest(http.MethodGet, "http://fixture.invalid/queue", nil)
			req.Header.Set(scaleset.HeaderScaleSetMaxCapacity, "1")
			transport := responseBudgetTransport{inner: baselineRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: source}, nil
			})}
			if response, err := transport.RoundTrip(req); err == nil || response != nil {
				t.Fatal("incomplete/unbounded identity response admitted")
			}
		})
	}
}

func TestZeroACKRequiresExplicitKnownCursor(t *testing.T) {
	c := &baselineWireCapture{stage: "ack", setID: 7, sessionID: "fixture", queue: "https://api.example/queue", origin: "https://api.example:443", runtimePathPrefix: "/tenant", runtimePathPrefixSet: true}
	req, _ := http.NewRequest(http.MethodDelete, "https://api.example/queue/0", nil)
	if c.target(req) {
		t.Fatal("unset cursor authorized zero ACK")
	}
	c.cursorKnown = true
	if !c.target(req) {
		t.Fatal("known zero cursor was rejected")
	}
	c.cursor = -1
	if c.target(req) {
		t.Fatal("negative cursor authorized ACK")
	}
}

func TestBaselineZeroMessageTraversesJournalACKAndContinuation(t *testing.T) {
	f := newBaselineFixture(t, func(stage string, value any) any {
		if stage == "poll1" {
			value.(map[string]any)["messageId"] = 0
		}
		return value
	})
	if err := f.run(t); err != nil || f.acks.Load() != 2 || f.acquires.Load() != 1 || f.continuations.Load() != 1 || f.forbidden.Load() != 0 {
		t.Fatalf("zero message did not traverse validated journal/ACK/continuation: err=%v ACK=%d acquire=%d continuation=%d", err, f.acks.Load(), f.acquires.Load(), f.continuations.Load())
	}
}
