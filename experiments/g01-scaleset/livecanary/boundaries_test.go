package livecanary

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/actions/scaleset"
	"github.com/google/uuid"
)

func (f *fakeAPI) GetScaleSet(context.Context, int) (*scaleset.RunnerScaleSet, error) {
	return f.set, nil
}
func (f *fakeAPI) DeleteScaleSet(context.Context, int) error { f.deleteCalls++; return nil }
func (f *fakeAPI) FindRunner(context.Context, string) (*scaleset.RunnerReference, error) {
	return f.findRunner, nil
}
func (f *fakeAPI) GenerateJIT(ctx context.Context, _ int, _ string) (*scaleset.RunnerScaleSetJitRunnerConfig, error) {
	f.jitCalls++
	if _, ok := ctx.Deadline(); !ok {
		panic("missing JIT deadline")
	}
	return &scaleset.RunnerScaleSetJitRunnerConfig{EncodedJITConfig: "synthetic-private-jit"}, nil
}
func (f *fakeAPI) VerifyRun(context.Context, Approval, int64) error { return f.verifyErr }
func (f *fakeAPI) OpenSession(_ context.Context, _ int, owner string) (Session, error) {
	f.session.session.OwnerName = owner
	return f.session, nil
}

type fakeSession struct {
	session             scaleset.RunnerScaleSetSession
	message             *scaleset.RunnerScaleSetMessage
	ack, acquire, close int
	journal             *memoryJournal
	replace             bool
}

func (s *fakeSession) Session() scaleset.RunnerScaleSetSession { return s.session }
func (s *fakeSession) GetMessage(ctx context.Context, last, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	if _, ok := ctx.Deadline(); !ok || last != 0 || capacity != 1 {
		panic("unbounded or unfenced poll")
	}
	return s.message, nil
}
func (s *fakeSession) DeleteMessage(ctx context.Context, _ int) error {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing ACK deadline")
	}
	s.ack++
	if s.replace {
		s.session.SessionID = uuid.New()
	}
	return nil
}
func (s *fakeSession) AcquireJobs(ctx context.Context, ids []int64) ([]int64, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing acquisition deadline")
	}
	s.acquire++
	return ids, nil
}
func (s *fakeSession) Close(context.Context) error { s.close++; return nil }

func created(t *testing.T) (*Driver, *fakeAPI, *memoryJournal) {
	t.Helper()
	a := approval()
	j := &memoryJournal{}
	f := &fakeAPI{}
	f.session = &fakeSession{journal: j, session: scaleset.RunnerScaleSetSession{SessionID: uuid.New(), Statistics: &scaleset.RunnerScaleSetStatistic{}}, message: &scaleset.RunnerScaleSetMessage{MessageID: 9, Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}, JobAvailableMessages: []*scaleset.JobAvailable{{JobMessageBase: scaleset.JobMessageBase{RunnerRequestID: 42, WorkflowRunID: a.WorkflowRunID, OwnerName: a.Organization, RepositoryName: a.Repository}}}}}
	d := &Driver{a, j, f}
	if err := d.Run(context.Background(), "create"); err != nil {
		t.Fatal(err)
	}
	return d, f, j
}

func TestSupportedListenerBarriersAndReservation(t *testing.T) {
	for _, phase := range []string{"before-ack", "after-ack", "before-acquire", "acquire-loss"} {
		t.Run(phase, func(t *testing.T) {
			d, f, j := created(t)
			err := d.Run(context.Background(), phase)
			wantACK := 1
			if phase == "before-ack" {
				wantACK = 0
			}
			wantAcquire, wantClose := 0, 1
			if phase == "acquire-loss" {
				wantAcquire, wantClose = 1, 0
				if !errors.Is(err, ErrQuarantine) {
					t.Fatal("lost acquisition result was not quarantined")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if f.session.ack != wantACK || f.session.acquire != wantAcquire || f.session.close != wantClose {
				t.Fatal("wrong side of supported listener barrier")
			}
			if phase == "acquire-loss" {
				if !replay(j.Events()).reserved {
					t.Fatal("unknown reservation released")
				}
				if d.Run(context.Background(), "jit-loss") == nil || f.jitCalls != 0 {
					t.Fatal("second worker admitted after acquisition ambiguity")
				}
			}
		})
	}
}

func TestNoMessageDoesNotCountAsCompletedBarrier(t *testing.T) {
	d, f, _ := created(t)
	f.session.message = nil
	if d.Run(context.Background(), "before-ack") == nil {
		t.Fatal("no controlled message falsely counted as exercised barrier")
	}
	if f.session.ack != 0 || f.session.close != 1 {
		t.Fatal("empty poll changed work or leaked a known safe session")
	}
}

func TestJITLostResponseIsSecretSafeAndNeverReissued(t *testing.T) {
	d, f, j := created(t)
	err := d.Run(context.Background(), "jit-loss")
	if !errors.Is(err, ErrQuarantine) || f.jitCalls != 1 {
		t.Fatal("JIT suppression not quarantined")
	}
	if d.Run(context.Background(), "jit-loss") == nil || d.Run(context.Background(), "cleanup") == nil || f.jitCalls != 1 || f.deleteCalls != 0 {
		t.Fatal("unknown JIT reservation permitted retry or cleanup")
	}
	data, _ := json.Marshal(j.Events())
	if strings.Contains(string(data), "synthetic-private-jit") || strings.Contains(err.Error(), "synthetic-private-jit") {
		t.Fatal("JIT leaked")
	}
	observed := false
	for _, e := range j.Events() {
		if e.Kind == "response" && e.Operation == "jit" {
			observed = true
		}
	}
	if !observed {
		t.Fatal("server response receipt was not distinguished from application suppression")
	}
}

func TestForeignIdentityAndUnreviewedWorkNeverACKOrDelete(t *testing.T) {
	for _, mutation := range []string{"owner", "group", "label", "run", "batch", "session"} {
		t.Run(mutation, func(t *testing.T) {
			d, f, _ := created(t)
			phase := "after-ack"
			switch mutation {
			case "owner":
				f.set.Name = "manual-runner"
				phase = "cleanup"
			case "group":
				f.set.RunnerGroupID = 99
				phase = "cleanup"
			case "label":
				f.set.Labels = nil
				phase = "cleanup"
			case "run":
				f.verifyErr = errors.New("synthetic-private-response")
			case "batch":
				f.session.message.JobAvailableMessages = append(f.session.message.JobAvailableMessages, f.session.message.JobAvailableMessages[0])
				phase = "acquire-loss"
			case "session":
				f.session.replace = true
			}
			if d.Run(context.Background(), phase) == nil {
				t.Fatal("ownership/authority boundary accepted")
			}
			if f.deleteCalls != 0 || f.session.acquire != 0 {
				t.Fatal("foreign/ambiguous state changed")
			}
			if mutation == "run" && f.session.ack != 0 {
				t.Fatal("unverified job acknowledged")
			}
			if mutation == "session" && f.session.close != 0 {
				t.Fatal("replacement session deleted")
			}
		})
	}
}

func TestCleanupOnlyForNeverIssuedWorkerWithExactReceipt(t *testing.T) {
	d, f, _ := created(t)
	if err := d.Run(context.Background(), "cleanup"); err != nil || f.deleteCalls != 1 {
		t.Fatal("verified empty owned scale set was not deleted")
	}
}
