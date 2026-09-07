package liveworker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type memoryJournal struct {
	events []Event
	fail   bool
}

func (j *memoryJournal) Events() []Event { return append([]Event(nil), j.events...) }
func (j *memoryJournal) Append(e Event) error {
	if j.fail {
		return ErrState
	}
	e.Sequence = len(j.events) + 1
	j.events = append(j.events, e)
	return nil
}

type fakeRuntime struct {
	Runtime
	creates, starts, deletes int
	preflightErr, createErr  error
	image                    ImageProfile
}

func (f *fakeRuntime) Preflight(context.Context, Approval) (ImageProfile, error) {
	return f.image, f.preflightErr
}
func (f *fakeRuntime) Create(context.Context, string, map[string]any) (string, bool, error) {
	f.creates++
	return strings.Repeat("c", 64), false, f.createErr
}
func approval() Approval {
	return Approval{HarnessSHA: strings.Repeat("1", 40), WorkflowSHA: strings.Repeat("2", 40), OwnerNonce: strings.Repeat("3", 32), Controller: "fixture-controller", Endpoint: "/fixture/docker.sock", DaemonID: "fixture-daemon-1", ImageID: "sha256:" + strings.Repeat("4", 64), Image: ImageReference, ExpiresAt: time.Now().Add(time.Hour), Phases: []string{"create", "start", "inspect", "cleanup"}}
}

const syntheticJIT = "c3ludGhldGljLWppdC1zZWNyZXQ="

func TestNoCreateBeforeDurableIntent(t *testing.T) {
	f := &fakeRuntime{}
	d := Driver{approval(), &memoryJournal{fail: true}, f}
	if d.Run(context.Background(), "create", syntheticJIT) == nil || f.creates != 0 {
		t.Fatal("worker created without durable reservation")
	}
}
func TestUnknownCreateNeverRetriesAfterRestart(t *testing.T) {
	j := &memoryJournal{}
	f := &fakeRuntime{createErr: errors.New("synthetic-secret-response")}
	d := Driver{approval(), j, f}
	_ = d.Run(context.Background(), "create", syntheticJIT)
	restarted := Driver{d.Approval, j, f}
	if !errors.Is(restarted.Run(context.Background(), "create", syntheticJIT), ErrUncertain) || f.creates != 1 {
		t.Fatal("ambiguous create repeated across restart")
	}
}
func TestChangedDaemonCannotCreate(t *testing.T) {
	f := &fakeRuntime{preflightErr: errors.New("changed daemon")}
	d := Driver{approval(), &memoryJournal{}, f}
	if d.Run(context.Background(), "create", syntheticJIT) == nil || f.creates != 0 {
		t.Fatal("wrong runtime identity accepted")
	}
}
