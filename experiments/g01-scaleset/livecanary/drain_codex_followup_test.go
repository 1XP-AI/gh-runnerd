package livecanary

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
)

func TestPinnedSDKDrainAcceptsUnrelatedRunnerMetadata(t *testing.T) {
	a := approval()
	runnerBody := `{"count":1,"value":[{"id":19,"name":"` + a.workerName() + `","runnerScaleSetId":7,"status":"online","version":"2.321.0","osDescription":"fixture"}],"status":"online","version":"2.321.0"}`
	_, base, _, _ := newPinnedDrainSessionOptions(t, a, pinnedDrainJobBody(a), pinnedDrainOptions{runnerBody: runnerBody})
	d := &Driver{Approval: a, Journal: &memoryJournal{}, API: fixtureSDK{base}}
	snapshot, err := d.drainSnapshot(context.Background(), 7, "before")
	if err != nil {
		t.Fatalf("runner metadata snapshot = %v, want accepted bounded identity", err)
	}
	if snapshot.Runner == nil || snapshot.Runner.ID != 19 || snapshot.Runner.Name != a.workerName() || snapshot.Runner.ScaleSetID != 7 {
		t.Fatalf("runner metadata snapshot = %+v, want bounded runner identity", snapshot.Runner)
	}
}

type blockedWithdrawalTransport struct {
	entered         chan struct{}
	startWrite      chan struct{}
	callbackStarted <-chan struct{}
	effects         chan string
	polls           atomic.Int32
	enteredOnce     sync.Once
}

func (t *blockedWithdrawalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		switch req.Method {
		case http.MethodDelete:
			t.effects <- "ack"
			return &http.Response{StatusCode: http.StatusNoContent, Status: http.StatusText(http.StatusNoContent), Body: http.NoBody}, nil
		case http.MethodPost:
			t.effects <- "acquire"
			return &http.Response{StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Body: io.NopCloser(strings.NewReader(`{"count":1,"value":[11]}`))}, nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Status: http.StatusText(http.StatusNotFound), Body: http.NoBody}, nil
		}
	}

	poll := t.polls.Add(1)
	trace := httptrace.ContextClientTrace(req.Context())
	if trace == nil || trace.WroteRequest == nil {
		return nil, errors.New("missing poll trace")
	}
	if poll == 1 {
		t.enteredOnce.Do(func() { close(t.entered) })
		<-t.startWrite
		go trace.WroteRequest(httptrace.WroteRequestInfo{})
		select {
		case <-t.callbackStarted:
		case <-time.After(time.Second):
			return nil, errors.New("withdrawal callback did not start")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Body:       io.NopCloser(strings.NewReader(`{"messageId":7,"messageType":"RunnerScaleSetJobMessages","statistics":{"totalAvailableJobs":1,"totalAcquiredJobs":0,"totalAssignedJobs":1,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1},"body":"[{\"messageType\":\"JobAvailable\",\"runnerRequestId\":11}]"}`)),
		}, nil
	}
	trace.WroteRequest(httptrace.WroteRequestInfo{})
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Status:     http.StatusText(http.StatusAccepted),
		Body:       io.NopCloser(strings.NewReader(`{"statistics":{"totalAvailableJobs":0,"totalAcquiredJobs":0,"totalAssignedJobs":0,"totalRunningJobs":0,"totalRegisteredRunners":1,"totalBusyRunners":0,"totalIdleRunners":1}}`)),
	}, nil
}

type blockedDrainResult struct {
	observation drainObservation
	err         error
}

func newBlockedWithdrawalDrain(t *testing.T, ctx context.Context) (*blockedWithdrawalTransport, *drainPollHook, <-chan struct{}, <-chan struct{}, chan<- struct{}, <-chan blockedDrainResult) {
	t.Helper()
	const target = "http://fixture.invalid/queue"
	hook := newDrainPollHook(target)
	callbackStarted := make(chan struct{})
	callbackRelease := make(chan struct{})
	callbackDone := make(chan struct{})
	transport := &blockedWithdrawalTransport{
		entered:         make(chan struct{}),
		startWrite:      make(chan struct{}),
		callbackStarted: callbackStarted,
		effects:         make(chan string, 4),
	}
	hook.inner = transport
	session := &drainSyntheticSession{
		client: &http.Client{Transport: hook},
		initial: scaleset.RunnerScaleSetSession{
			SessionID: uuid.New(), OwnerName: "fixture-owner", MessageQueueURL: target,
			Statistics: &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1, TotalIdleRunners: 1},
		},
		order: new([]string),
	}
	result := make(chan blockedDrainResult, 1)
	go func() {
		observation, err := runDrainListener(ctx, session, 7, hook)
		result <- blockedDrainResult{observation: observation, err: err}
	}()
	select {
	case <-transport.entered:
	case <-time.After(time.Second):
		t.Fatal("drain poll did not enter blocked transport")
	}
	hook.mu.Lock()
	original := hook.onRequestWritten
	hook.onRequestWritten = func() {
		if original != nil {
			original()
		}
		close(callbackStarted)
		<-callbackRelease
		close(callbackDone)
	}
	hook.mu.Unlock()
	close(transport.startWrite)
	return transport, hook, callbackStarted, callbackDone, callbackRelease, result
}

func TestDrainListenerDoesNotReleaseBeforeWithdrawalCompletes(t *testing.T) {
	transport, hook, _, callbackDone, callbackRelease, result := newBlockedWithdrawalDrain(t, context.Background())
	select {
	case <-transport.callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("withdrawal callback did not block")
	}
	if _, _, proven := hook.boundary(); proven {
		close(callbackRelease)
		<-callbackDone
		<-result
		t.Fatal("response-first race was proven before withdrawal completed")
	}
	select {
	case effect := <-transport.effects:
		close(callbackRelease)
		<-callbackDone
		<-result
		t.Fatalf("%s reached the remote effect before withdrawal callback completed", effect)
	case <-time.After(50 * time.Millisecond):
	}
	close(callbackRelease)
	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		t.Fatal("withdrawal callback did not finish")
	}
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("blocked withdrawal drain = %v, observation=%+v", got.err, got.observation)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked withdrawal drain did not finish")
	}
}

func TestDrainListenerCancellationWhileWithdrawalBlockedDoesNotDeadlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport, _, _, callbackDone, callbackRelease, result := newBlockedWithdrawalDrain(t, ctx)
	select {
	case <-transport.callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("withdrawal callback did not block")
	}
	cancel()
	select {
	case got := <-result:
		if !errors.Is(got.err, ErrQuarantine) {
			t.Fatalf("cancelled blocked withdrawal = %v, want quarantine", got.err)
		}
	case <-time.After(time.Second):
		close(callbackRelease)
		<-callbackDone
		t.Fatal("cancelled blocked withdrawal deadlocked")
	}
	select {
	case effect := <-transport.effects:
		t.Fatalf("cancelled blocked withdrawal reached %s", effect)
	default:
	}
	close(callbackRelease)
	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		t.Fatal("cancelled withdrawal callback did not finish")
	}
}
