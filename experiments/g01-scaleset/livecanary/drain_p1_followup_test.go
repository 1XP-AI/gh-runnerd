package livecanary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/actions/scaleset"
)

func TestPinnedSDKDrainRejectsPhysicalPollMutationBeforeInner(t *testing.T) {
	a := approval()
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
	}{
		{
			name: "withdrawn capacity header",
			mutate: func(req *http.Request) {
				if req.Method == http.MethodGet && req.URL.Path == "/queue" && req.URL.Query().Get("lastMessageId") != "" {
					req.Header.Set("X-ScaleSetMaxCapacity", "1")
				}
			},
		},
		{
			name: "withdrawn cursor",
			mutate: func(req *http.Request) {
				if req.Method == http.MethodGet && req.URL.Path == "/queue" && req.URL.Query().Get("lastMessageId") != "" {
					q := req.URL.Query()
					q.Set("lastMessageId", "18")
					req.URL.RawQuery = q.Encode()
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, _, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{mutateRequest: tc.mutate})
			observation, err := runDrainListener(context.Background(), session, 7, hook)
			if !errors.Is(err, ErrQuarantine) || observation.Outcome == drainOutcomeObserved {
				t.Fatalf("physical poll mutation was promoted: observation=%+v err=%v", observation, err)
			}
			if got := fixture.polls.Load(); got != 1 {
				t.Fatalf("mutated withdrawn poll reached server: polls=%d, want first poll only", got)
			}
		})
	}
}

func TestDrainCancellationStopsBeforeReleasingHeldResponse(t *testing.T) {
	var order []string
	cancelAndJoinDrain(
		func() { order = append(order, "cancel") },
		func() { order = append(order, "release") },
		func() { order = append(order, "join") },
	)
	if got, want := strings.Join(order, ","), "cancel,release,join"; got != want {
		t.Fatalf("cancellation order = %s, want %s", got, want)
	}
}

func TestPinnedSDKDrainRejectsWithdrawnPollBodyBeforeAbsent(t *testing.T) {
	a := approval()
	job := pinnedDrainJobBody(a)
	body := fmt.Sprintf(`{"messageId":18,"messageType":"RunnerScaleSetJobMessages","statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1},"body":%s}`, mustJSONQuote(job))
	fixture, _, session, hook := newPinnedDrainSessionOptions(t, a, job, pinnedDrainOptions{withdrawnBody: body})
	observation, err := runDrainListener(context.Background(), session, 7, hook)
	if !errors.Is(err, ErrQuarantine) || observation.Outcome == drainOutcomeObserved {
		t.Fatalf("withdrawn 202 body was classified as absent: observation=%+v err=%v", observation, err)
	}
	if fixture.polls.Load() != 2 || fixture.acks.Load() != 1 || fixture.acquires.Load() != 1 {
		t.Fatalf("withdrawn body effects = polls %d ack %d acquire %d, want one bounded poll plus old effects", fixture.polls.Load(), fixture.acks.Load(), fixture.acquires.Load())
	}
}

func TestPinnedSDKDrainRequiresCompletePollStatsBeforeVerifyRun(t *testing.T) {
	a := approval()
	job := pinnedDrainJobBody(a)
	first := fmt.Sprintf(`{"messageId":17,"messageType":"RunnerScaleSetJobMessages","statistics":{"totalRegisteredRunners":1,"totalIdleRunners":1},"body":%s}`, mustJSONQuote(job))
	fixture, base, session, hook := newPinnedDrainSessionOptions(t, a, job, pinnedDrainOptions{firstPollBody: first})
	d := &Driver{Approval: a, Journal: &memoryJournal{}, API: base}
	c := &journaledDrainClient{d: d, inner: session, sessionID: session.Session().SessionID.String(), hook: hook}
	hook.releaseResponse()
	if _, err := c.GetMessage(context.Background(), 0, drainInitialCapacity); !errors.Is(err, ErrQuarantine) {
		t.Fatalf("incomplete poll statistics = %v, want quarantine", err)
	}
	if fixture.verifyCalls.Load() != 0 {
		t.Fatalf("incomplete poll statistics crossed VerifyRun: calls=%d", fixture.verifyCalls.Load())
	}
	if fixture.acks.Load() != 0 || fixture.acquires.Load() != 0 {
		t.Fatalf("incomplete poll statistics reached effects: ack=%d acquire=%d", fixture.acks.Load(), fixture.acquires.Load())
	}
}

func TestPinnedSDKDrainVerifyRunRejectsAmbiguousWireFieldsBeforeEffects(t *testing.T) {
	a := approval()
	for _, tc := range []struct {
		name string
		body string
		good bool
	}{
		{name: "success", body: pinnedDrainRunJSON(a), good: true},
		{name: "duplicate exact head", body: ambiguousRunJSON(a, "duplicate-head"), good: false},
		{name: "duplicate casefold head", body: ambiguousRunJSON(a, "casefold-head"), good: false},
		{name: "null", body: "null", good: false},
		{name: "missing head", body: ambiguousRunJSON(a, "missing-head"), good: false},
		{name: "contradictory head", body: ambiguousRunJSON(a, "wrong-head"), good: false},
		{name: "malformed", body: `{"id":5`, good: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, base, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{verifyBody: tc.body})
			d := &Driver{Approval: a, Journal: &memoryJournal{}, API: base}
			c := &journaledDrainClient{d: d, inner: session, sessionID: session.Session().SessionID.String(), hook: hook}
			hook.releaseResponse()
			_, err := c.GetMessage(context.Background(), 0, drainInitialCapacity)
			if tc.good {
				if err != nil {
					t.Fatalf("valid VerifyRun = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrApproval) && !errors.Is(err, ErrQuarantine) {
				t.Fatalf("ambiguous VerifyRun = %v, want fixed rejection", err)
			}
			if fixture.acks.Load() != 0 || fixture.acquires.Load() != 0 {
				t.Fatalf("ambiguous VerifyRun reached effects: ack=%d acquire=%d", fixture.acks.Load(), fixture.acquires.Load())
			}
		})
	}
}

func TestPinnedSDKDrainAcquisitionRequiresStrictWireResponse(t *testing.T) {
	a := approval()
	for _, tc := range []struct {
		name string
		body string
		good bool
	}{
		{name: "success", body: `{"count":1,"value":[41]}`, good: true},
		{name: "duplicate count", body: `{"count":0,"count":1,"value":[41]}`, good: false},
		{name: "casefold count", body: `{"Count":0,"count":1,"value":[41]}`, good: false},
		{name: "duplicate value", body: `{"count":1,"value":[99],"value":[41]}`, good: false},
		{name: "casefold value", body: `{"count":1,"Value":[99],"value":[41]}`, good: false},
		{name: "missing count", body: `{"value":[41]}`, good: false},
		{name: "count mismatch", body: `{"count":2,"value":[41]}`, good: false},
		{name: "null count", body: `{"count":null,"value":[41]}`, good: false},
		{name: "malformed", body: `{"count":1,"value":[41]`, good: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, base, session, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{acquireBody: tc.body})
			journal := &memoryJournal{}
			d := &Driver{Approval: a, Journal: journal, API: base}
			c := &journaledDrainClient{d: d, inner: session, sessionID: session.Session().SessionID.String(), hook: hook}
			hook.releaseResponse()
			message, err := c.GetMessage(context.Background(), 0, drainInitialCapacity)
			if err != nil || message == nil {
				t.Fatalf("setup poll = message %v err %v", message, err)
			}
			if err := c.DeleteMessage(context.Background(), message.MessageID); err != nil {
				t.Fatalf("setup ACK = %v", err)
			}
			_, err = c.AcquireJobs(context.Background(), []int64{41})
			if tc.good {
				if err != nil {
					t.Fatalf("valid acquisition = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrQuarantine) {
				t.Fatalf("ambiguous acquisition = %v, want quarantine", err)
			}
			if fixture.acquires.Load() != 1 {
				t.Fatalf("ambiguous acquisition request count = %d, want one remote attempt", fixture.acquires.Load())
			}
			if state := replay(journal.Events()); !state.uncertain {
				t.Fatal("ambiguous acquisition did not retain uncertainty")
			}
			for _, event := range journal.Events() {
				if event.Kind == "result" && event.Operation == "acquire" {
					t.Fatal("ambiguous acquisition recorded a successful result")
				}
			}
		})
	}
}

func TestPinnedSDKDrainSnapshotsRequireStrictWireFacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(string) string
	}{
		{name: "duplicate id", mutate: func(body string) string { return strings.Replace(body, `"id":7`, `"id":7,"id":7`, 1) }},
		{name: "casefold id", mutate: func(body string) string { return strings.Replace(body, `"id":7`, `"ID":99,"id":7`, 1) }},
		{name: "duplicate statistics", mutate: func(body string) string {
			return strings.Replace(body, `"totalIdleRunners":1`, `"totalIdleRunners":0,"totalIdleRunners":1`, 1)
		}},
		{name: "casefold statistics", mutate: func(body string) string {
			return strings.Replace(body, `"totalIdleRunners":1`, `"TotalIdleRunners":0,"totalIdleRunners":1`, 1)
		}},
		{name: "missing statistics", mutate: func(body string) string {
			return strings.Replace(body, `,"statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1}`, "", 1)
		}},
		{name: "null statistics", mutate: func(body string) string {
			return strings.Replace(body, `"statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1}`, `"statistics":null`, 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := approval()
			valid := pinnedDrainSnapshotJSON(a)
			fixture, base, _, hook := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{snapshotBodies: []string{tc.mutate(valid)}})
			api := fixtureSDK{base}
			a.Phases = append(a.Phases, "drain")
			j := &memoryJournal{events: []Event{
				{Kind: "phase", Operation: "create"},
				{Kind: "intent", Operation: "create"},
				{Kind: "result", Operation: "create", ID: 7},
			}}
			d := Driver{Approval: a, Journal: j, API: api}
			if err := d.Run(context.Background(), "drain"); !errors.Is(err, ErrQuarantine) {
				t.Fatalf("ambiguous snapshot = %v, want quarantine", err)
			}
			if fixture.polls.Load() != 0 || fixture.acquires.Load() != 0 {
				t.Fatalf("ambiguous snapshot reached session effects: polls=%d acquire=%d", fixture.polls.Load(), fixture.acquires.Load())
			}
			_ = hook
		})
	}
}

func mustJSONQuote(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func pinnedDrainRunJSON(a Approval) string {
	data, _ := json.Marshal(pinnedDrainRun(a))
	return string(data)
}

func ambiguousRunJSON(a Approval, mode string) string {
	if mode == "missing-head" {
		run := pinnedDrainRun(a)
		delete(run, "head_sha")
		data, _ := json.Marshal(run)
		return string(data)
	}
	body := pinnedDrainRunJSON(a)
	switch mode {
	case "duplicate-head":
		return strings.Replace(body, `"head_sha":"`+a.WorkflowSHA+`"`, `"head_sha":"foreign","head_sha":"`+a.WorkflowSHA+`"`, 1)
	case "casefold-head":
		return strings.Replace(body, `"head_sha":"`+a.WorkflowSHA+`"`, `"HEAD_SHA":"foreign","head_sha":"`+a.WorkflowSHA+`"`, 1)
	case "wrong-head":
		return strings.Replace(body, a.WorkflowSHA, strings.Repeat("f", 40), 1)
	default:
		return body
	}
}

type pinnedDrainStatsWire struct {
	Available  int `json:"totalAvailableJobs"`
	Acquired   int `json:"totalAcquiredJobs"`
	Assigned   int `json:"totalAssignedJobs"`
	Running    int `json:"totalRunningJobs"`
	Registered int `json:"totalRegisteredRunners"`
	Busy       int `json:"totalBusyRunners"`
	Idle       int `json:"totalIdleRunners"`
}

type pinnedDrainSnapshotWire struct {
	ID            int                    `json:"id"`
	Name          string                 `json:"name"`
	RunnerGroupID int                    `json:"runnerGroupId"`
	Labels        []scaleset.Label       `json:"labels"`
	RunnerSetting scaleset.RunnerSetting `json:"RunnerSetting"`
	Statistics    pinnedDrainStatsWire   `json:"statistics"`
}

func pinnedDrainSnapshotJSON(a Approval) string {
	data, _ := json.Marshal(pinnedDrainSnapshotWire{
		ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID,
		Labels: []scaleset.Label{{Name: a.setName(), Type: "System"}}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		Statistics: pinnedDrainStatsWire{Registered: 1, Idle: 1},
	})
	return string(data)
}
