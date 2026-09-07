package liveworker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryJournal struct {
	mu     sync.Mutex
	events []Event
	fail   bool
}

func (j *memoryJournal) Events() []Event {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]Event(nil), j.events...)
}
func (j *memoryJournal) Append(e Event) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.fail {
		return ErrState
	}
	e.Sequence = len(j.events) + 1
	j.events = append(j.events, e)
	return nil
}

type fakeRuntime struct {
	container           *Container
	startErr, deleteErr error
	warnings            bool
	afterStart          func()
	Runtime
	creates, starts, deletes atomic.Int64
	preflightErr, createErr  error
	image                    ImageProfile
}

func (f *fakeRuntime) Preflight(context.Context, Approval) (ImageProfile, error) {
	return f.image, f.preflightErr
}
func (f *fakeRuntime) Create(ctx context.Context, name string, payload map[string]any) (string, bool, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("unbounded create")
	}
	f.creates.Add(1)
	c := &Container{ID: strings.Repeat("c", 64), Name: "/" + name, ImageID: approval().ImageID, Path: "/home/runner/bin/Runner.Listener", Args: []string{"run", "--once"}}
	if payload != nil {
		c.Config = normalized(payload).(map[string]any)
		c.HostConfig = c.Config["HostConfig"].(map[string]any)
		delete(c.Config, "HostConfig")
		env := []any{}
		for _, entry := range f.image.Env {
			env = append(env, entry)
		}
		env = append(env, c.Config["Env"].([]any)...)
		c.Config["Env"] = env
		labels := c.Config["Labels"].(map[string]any)
		for key, value := range f.image.Labels {
			labels[key] = value
		}
	}
	c.State.Status = "created"
	c.NetworkSettings.Networks = map[string]any{"bridge": map[string]any{}}
	f.container = c
	return c.ID, f.warnings, f.createErr
}
func (f *fakeRuntime) Inspect(context.Context, string) (*Container, error) { return f.container, nil }
func (f *fakeRuntime) Start(ctx context.Context, _ string) error {
	if _, ok := ctx.Deadline(); !ok {
		panic("unbounded start")
	}
	f.starts.Add(1)
	f.container.State.Running = true
	f.container.State.Status = "running"
	if f.afterStart != nil {
		f.afterStart()
	}
	return f.startErr
}
func (f *fakeRuntime) Delete(context.Context, string) error { f.deletes.Add(1); return f.deleteErr }

func approval() Approval {
	return Approval{RunnerUpdatesDisabled: true, HarnessSHA: strings.Repeat("1", 40), WorkflowSHA: strings.Repeat("2", 40), OwnerNonce: strings.Repeat("3", 32), Controller: "fixture-controller", Endpoint: "/fixture/docker.sock", DaemonID: "fixture-daemon-1", ImageID: "sha256:" + strings.Repeat("4", 64), Image: ImageReference, ExpiresAt: time.Now().Add(time.Hour), Phases: []string{"create", "start", "inspect", "cleanup"}}
}

const syntheticJIT = "c3ludGhldGljLWppdC1zZWNyZXQ="

func TestNoCreateBeforeDurableIntent(t *testing.T) {
	f := &fakeRuntime{}
	d := Driver{approval(), &memoryJournal{fail: true}, f}
	if d.Run(context.Background(), "create", syntheticJIT) == nil || f.creates.Load() != 0 {
		t.Fatal("worker created without durable reservation")
	}
}
func TestUnknownCreateNeverRetriesAfterRestart(t *testing.T) {
	j := &memoryJournal{}
	f := &fakeRuntime{createErr: errors.New("synthetic-secret-response")}
	d := Driver{approval(), j, f}
	_ = d.Run(context.Background(), "create", syntheticJIT)
	restarted := Driver{d.Approval, j, f}
	if !errors.Is(restarted.Run(context.Background(), "create", syntheticJIT), ErrUncertain) || f.creates.Load() != 1 {
		t.Fatal("ambiguous create repeated across restart")
	}
}
func TestChangedDaemonCannotCreate(t *testing.T) {
	f := &fakeRuntime{preflightErr: errors.New("changed daemon")}
	d := Driver{approval(), &memoryJournal{}, f}
	if d.Run(context.Background(), "create", syntheticJIT) == nil || f.creates.Load() != 0 {
		t.Fatal("wrong runtime identity accepted")
	}
}

func (*memoryJournal) authorize(Approval) (func(), error) { return func() {}, nil }
