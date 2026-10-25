package livecanary

import (
	"context"
	"errors"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/google/uuid"
)

// The high-level listener validates Scale before ACK. This caller explicitly
// acquires after its journaled ACK; faults use the public client boundary,
// never patched SDK code or HTTP response bodies.
type probeClient struct {
	driver           *Driver
	session          Session
	ctx              context.Context
	phase, sessionID string
	initial          scaleset.RunnerScaleSetSession
}

func (p *probeClient) Session() scaleset.RunnerScaleSetSession { return p.initial }
func (p *probeClient) sameSession() bool {
	return p.session.Session().SessionID.String() == p.sessionID
}

func (p *probeClient) GetMessage(_ context.Context, last, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	if !p.sameSession() || last != 0 || capacity != 1 {
		return nil, ErrQuarantine
	}
	var m *scaleset.RunnerScaleSetMessage
	err := p.driver.effect(p.ctx, "observe-poll", nil, func(c context.Context) (Event, error) {
		var err error
		m, err = p.session.GetMessage(c, 0, 1)
		if err != nil {
			return Event{}, err
		}
		if m == nil { // No observation; never erase a previous positive result.
			return Event{}, nil
		}
		return Event{Work: statisticsWork(m.Statistics)}, nil
	})
	if err != nil {
		return nil, err
	}
	if !p.sameSession() {
		return nil, ErrQuarantine
	}
	if m == nil {
		return nil, ErrNoMessage
	}
	if m.Statistics == nil {
		return nil, ErrQuarantine
	}
	if len(m.JobStartedMessages) > 0 || len(m.JobCompletedMessages) > 0 || len(m.JobAssignedMessages) > 0 {
		return nil, ErrQuarantine
	}
	if len(m.JobAvailableMessages) == 0 {
		// Positive counts are unresolved-work evidence, not owned runner IDs.
		// This harness cannot reconcile them against a later stale zero.
		if *m.Statistics != (scaleset.RunnerScaleSetStatistic{}) {
			return nil, ErrQuarantine
		}
		return nil, ErrNoMessage
	}
	ids := make([]int64, 0, len(m.JobAvailableMessages))
	if m.MessageID < 0 || len(m.JobAvailableMessages) > 2 {
		return nil, ErrQuarantine
	}
	// Caller policy acquires after ACK. Validate the single-request acquisition
	// boundary here, before that ACK can occur.
	if (p.phase == "before-acquire" || p.phase == "acquire-loss") && len(m.JobAvailableMessages) != 1 {
		return nil, ErrQuarantine
	}
	for _, j := range m.JobAvailableMessages {
		if j == nil || j.RunnerRequestID <= 0 || j.WorkflowRunID != p.driver.Approval.WorkflowRunID || j.OwnerName != p.driver.Approval.Organization || j.RepositoryName != p.driver.Approval.Repository {
			return nil, ErrQuarantine
		}
		_, err = boundedRead(p.ctx, func(c context.Context) (bool, error) {
			return true, p.driver.API.VerifyRun(c, p.driver.Approval, j.WorkflowRunID)
		})
		if err != nil {
			return nil, ErrApproval
		}
		ids = append(ids, j.RunnerRequestID)
	}
	if err = p.driver.record(Event{Kind: "observation", Operation: "poll", ID: m.MessageID, SessionID: p.sessionID, RequestIDs: ids}); err != nil {
		return nil, err
	}
	return m, nil
}

func (p *probeClient) DeleteMessage(_ context.Context, id int) error {
	if !p.sameSession() {
		return ErrQuarantine
	}
	if p.phase == "before-ack" {
		if err := p.driver.record(Event{Kind: "barrier", Operation: p.phase}); err != nil {
			return err
		}
		return ErrBarrier
	}
	err := p.driver.effect(p.ctx, "ack", nil, func(c context.Context) (Event, error) {
		err := p.session.DeleteMessage(c, id)
		if !p.sameSession() {
			return Event{}, ErrQuarantine
		}
		return Event{ID: id, SessionID: p.sessionID}, err
	})
	if err != nil {
		return err
	}
	if p.phase == "after-ack" {
		if err := p.driver.record(Event{Kind: "barrier", Operation: p.phase}); err != nil {
			return err
		}
		return ErrBarrier
	}
	return nil
}

func (p *probeClient) AcquireJobs(_ context.Context, ids []int64) ([]int64, error) {
	if !p.sameSession() || len(ids) != 1 {
		return nil, ErrQuarantine
	}
	if p.phase == "before-acquire" {
		if err := p.driver.record(Event{Kind: "barrier", Operation: p.phase}); err != nil {
			return nil, err
		}
		return nil, ErrBarrier
	}
	err := p.driver.effect(p.ctx, "acquire", ids, func(c context.Context) (Event, error) {
		_, callErr := p.session.AcquireJobs(c, ids)
		if p.driver.record(Event{Kind: "response", Operation: "acquire", Succeeded: callErr == nil}) != nil {
			return Event{}, ErrJournal
		}
		// The application receives no result. Do not call again, close/recreate
		// this session, or release the reservation after this fault.
		return Event{}, ErrBarrier
	})
	return nil, err
}

type observeOnly struct{}

func (observeOnly) Scale(_ context.Context, msg *scaleset.RunnerScaleSetMessage) error {
	if msg == nil || msg.Statistics == nil || msg.Statistics.TotalAssignedJobs < 0 || len(msg.JobStartedMessages) > 0 || len(msg.JobCompletedMessages) > 0 || len(msg.JobAssignedMessages) > 0 {
		return ErrQuarantine
	}
	return nil
}

type probeListenerClient struct {
	*probeClient
	requestIDs []int64
}

func (p *probeListenerClient) GetMessage(ctx context.Context, last, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	msg, err := p.probeClient.GetMessage(ctx, last, capacity)
	p.requestIDs = nil
	if err == nil && msg != nil {
		for _, job := range msg.JobAvailableMessages {
			p.requestIDs = append(p.requestIDs, job.RunnerRequestID)
		}
	}
	return msg, err
}

func (p *probeListenerClient) DeleteMessage(ctx context.Context, id int) error {
	if err := p.probeClient.DeleteMessage(ctx, id); err != nil {
		return err
	}
	_, err := p.AcquireJobs(p.ctx, p.requestIDs)
	return err
}

func (d *Driver) probe(ctx context.Context, phase string, setID int) error {
	var session Session
	var sid string
	var initial scaleset.RunnerScaleSetSession
	err := d.effect(ctx, "session-open", nil, func(c context.Context) (Event, error) {
		var err error
		session, err = d.API.OpenSession(c, setID, d.Approval.setName())
		if err != nil || session == nil {
			return Event{}, ErrRemote
		}
		s := session.Session()
		if s.SessionID == uuid.Nil || s.OwnerName != d.Approval.setName() {
			return Event{}, ErrQuarantine
		}
		sid = s.SessionID.String()
		initial = s
		work := statisticsWork(s.Statistics)
		if s.RunnerScaleSet != nil {
			work = combinedWork(work, statisticsWork(s.RunnerScaleSet.Statistics))
		}
		return Event{SessionID: sid, Work: work}, nil
	})
	if err != nil {
		return err
	}
	p := &probeClient{d, session, ctx, phase, sid, initial}
	l, err := listener.New(&probeListenerClient{probeClient: p}, listener.Config{ScaleSetID: setID, MaxRunners: 1})
	if err != nil {
		return ErrQuarantine
	}
	err = l.Run(ctx, observeOnly{})
	noMessage := errors.Is(err, ErrNoMessage)
	if !errors.Is(err, ErrBarrier) && !noMessage {
		if recordErr := d.record(Event{Kind: "unknown", Operation: "probe"}); recordErr != nil {
			return recordErr
		}
		return ErrQuarantine
	}
	if !p.sameSession() {
		return ErrQuarantine
	}
	if closeErr := d.effect(ctx, "session-close", nil, func(c context.Context) (Event, error) { return Event{SessionID: sid}, session.Close(c) }); closeErr != nil {
		return closeErr
	}
	if noMessage {
		return ErrNoMessage
	}
	return nil
}
