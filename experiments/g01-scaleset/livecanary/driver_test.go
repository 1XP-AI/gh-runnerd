package livecanary

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/actions/scaleset"
)

type memoryJournal struct {
	events []Event
	fail   bool
}

func (j *memoryJournal) Events() []Event { return append([]Event(nil), j.events...) }
func (j *memoryJournal) Append(e Event) error {
	if j.fail {
		return errors.New("synthetic journal failure")
	}
	e.Sequence = len(j.events) + 1
	j.events = append(j.events, e)
	return nil
}

type fakeAPI struct {
	API
	createCalls             int
	set                     *scaleset.RunnerScaleSet
	session                 *fakeSession
	jitCalls, deleteCalls   int
	findRunner              *scaleset.RunnerReference
	verifyErr               error
	preflightErr, createErr error
}

func (f *fakeAPI) Preflight(context.Context, Approval) error { return f.preflightErr }
func (f *fakeAPI) Inventory(context.Context) (string, error) { return "fixture-inventory", nil }
func (f *fakeAPI) FindScaleSet(context.Context, string, int) (*scaleset.RunnerScaleSet, error) {
	return nil, nil
}
func (f *fakeAPI) CreateScaleSet(_ context.Context, s *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error) {
	f.createCalls++
	if f.createErr != nil {
		return nil, f.createErr
	}
	copy := *s
	copy.ID = 7
	copy.Statistics = &scaleset.RunnerScaleSetStatistic{}
	f.set = &copy
	return &copy, nil
}

func approval() Approval {
	return Approval{AppID: 11, InstallationID: 12, Organization: "fixture-org", Repository: "canary", RepositoryID: 42, RunnerGroupID: 3, OwnerNonce: "0123456789abcdef0123456789abcdef", HarnessSHA: "1111111111111111111111111111111111111111", WorkflowSHA: "2222222222222222222222222222222222222222", WorkflowPath: ".github/workflows/canary.yml", WorkflowRunID: 5, Controller: "disposable-controller", ExpiresAt: time.Now().Add(time.Hour), ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create", "inspect", "cleanup", "before-ack", "after-ack", "before-acquire", "acquire-loss", "jit-loss"}}
}

func TestJournalFailureStopsBeforeCreate(t *testing.T) {
	f := &fakeAPI{}
	d := Driver{approval(), &memoryJournal{fail: true}, f}
	if d.Run(context.Background(), "create") == nil || f.createCalls != 0 {
		t.Fatal("external create ran without durable intent")
	}
}

func TestAmbiguousCreateNeverRetriesAfterRestart(t *testing.T) {
	j := &memoryJournal{}
	f := &fakeAPI{createErr: errors.New("synthetic-token-response-body")}
	d := Driver{approval(), j, f}
	_ = d.Run(context.Background(), "create")
	restarted := Driver{d.Approval, j, f}
	err := restarted.Run(context.Background(), "create")
	if !errors.Is(err, ErrQuarantine) || f.createCalls != 1 {
		t.Fatal("uncertain side effect was retried across restart")
	}
}

func TestAuthorityMismatchStopsBeforeCreate(t *testing.T) {
	f := &fakeAPI{preflightErr: errors.New("wrong installation or public repository")}
	d := Driver{approval(), &memoryJournal{}, f}
	if d.Run(context.Background(), "create") == nil || f.createCalls != 0 {
		t.Fatal("creation bypassed remote authority/policy verification")
	}
}

func (*memoryJournal) authorize(Approval) (func(), error) { return func() {}, nil }
