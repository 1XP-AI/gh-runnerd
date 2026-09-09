package livecanary

import (
	"context"
	"errors"
	"slices"

	"github.com/actions/scaleset"
)

type drainSessionOpener interface {
	OpenDrainSession(context.Context, int, string, *drainPollHook) (Session, error)
}

type journaledDrainClient struct {
	d         *Driver
	inner     Session
	sessionID string
	phaseCtx  context.Context
}

func (c *journaledDrainClient) bindDrainContext(ctx context.Context) { c.phaseCtx = ctx }

func (c *journaledDrainClient) operationContext(fallback context.Context) (context.Context, error) {
	if c.phaseCtx != nil {
		if c.phaseCtx.Err() != nil {
			return nil, ErrQuarantine
		}
		return c.phaseCtx, nil
	}
	if fallback == nil || fallback.Err() != nil {
		return nil, ErrQuarantine
	}
	return fallback, nil
}

func (c *journaledDrainClient) reject(operation string) error {
	if c.d.record(Event{Kind: "unknown", Operation: operation}) != nil {
		return ErrJournal
	}
	return ErrQuarantine
}

func (c *journaledDrainClient) Session() scaleset.RunnerScaleSetSession { return c.inner.Session() }

func (c *journaledDrainClient) GetMessage(ctx context.Context, last, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	callCtx, err := c.operationContext(ctx)
	if err != nil {
		return nil, err
	}
	var message *scaleset.RunnerScaleSetMessage
	err = c.d.effect(callCtx, "observe-poll", nil, func(call context.Context) (Event, error) {
		var callErr error
		message, callErr = c.inner.GetMessage(call, last, capacity)
		if callErr != nil {
			return Event{}, callErr
		}
		if message == nil {
			return Event{SessionID: c.sessionID}, nil
		}
		if message.MessageID <= 0 || len(message.JobAvailableMessages) != 1 || len(message.JobAssignedMessages) != 0 || len(message.JobStartedMessages) != 0 || len(message.JobCompletedMessages) != 0 || message.Statistics == nil {
			return Event{}, ErrRemote
		}
		if _, callErr = newDrainStatistics(message.Statistics); callErr != nil {
			return Event{}, ErrRemote
		}
		for _, job := range message.JobAvailableMessages {
			if job == nil || job.RunnerRequestID <= 0 || job.WorkflowRunID != c.d.Approval.WorkflowRunID || job.OwnerName != c.d.Approval.Organization || job.RepositoryName != c.d.Approval.Repository {
				return Event{}, ErrApproval
			}
			if _, err := boundedRead(call, func(verify context.Context) (bool, error) {
				return true, c.d.API.VerifyRun(verify, c.d.Approval, job.WorkflowRunID)
			}); err != nil {
				return Event{}, ErrApproval
			}
		}
		requestID := message.JobAvailableMessages[0].RunnerRequestID
		return Event{ID: message.MessageID, SessionID: c.sessionID, RequestIDs: []int64{requestID}, Work: workDemand}, nil
	})
	if err != nil {
		return nil, err
	}
	return message, nil
}

func (c *journaledDrainClient) DeleteMessage(ctx context.Context, id int) error {
	callCtx, err := c.operationContext(ctx)
	if err != nil {
		return err
	}
	return c.d.effect(callCtx, "ack", nil, func(call context.Context) (Event, error) {
		if err := c.inner.DeleteMessage(call, id); err != nil {
			return Event{}, err
		}
		return Event{ID: id, SessionID: c.sessionID}, nil
	})
}

func (c *journaledDrainClient) AcquireJobs(ctx context.Context, ids []int64) ([]int64, error) {
	callCtx, err := c.operationContext(ctx)
	if err != nil {
		return nil, err
	}
	ids = slices.Clone(ids)
	var got []int64
	err = c.d.effect(callCtx, "acquire", ids, func(call context.Context) (Event, error) {
		var err error
		got, err = c.inner.AcquireJobs(call, slices.Clone(ids))
		if err != nil || !slices.Equal(got, ids) {
			return Event{}, errors.New("acquisition response did not match the one-shot request")
		}
		return Event{RequestIDs: slices.Clone(ids), SessionID: c.sessionID}, nil
	})
	if err != nil {
		return nil, err
	}
	return slices.Clone(got), nil
}

func (d *Driver) drainSnapshot(ctx context.Context, setID int, stage string) (drainSnapshot, error) {
	var snapshot drainSnapshot
	var set *scaleset.RunnerScaleSet
	if err := d.effect(ctx, "observe-owned", nil, func(call context.Context) (Event, error) {
		var err error
		set, err = d.API.GetScaleSet(call, setID)
		if err != nil || set == nil || set.ID != setID || set.Name != d.Approval.setName() || set.RunnerGroupID != d.Approval.RunnerGroupID || !set.RunnerSetting.DisableUpdate {
			return Event{}, ErrRemote
		}
		label := ""
		for _, candidate := range set.Labels {
			if candidate.Name == d.Approval.setName() {
				label = candidate.Name
			}
		}
		stats, err := newDrainStatistics(set.Statistics)
		if err != nil || label == "" {
			return Event{}, ErrRemote
		}
		snapshot.Set = drainSetIdentity{ID: set.ID, Name: set.Name, RunnerGroupID: set.RunnerGroupID, Label: label}
		snapshot.Statistics = stats
		snapshot.StatsKnown = true
		return Event{ID: set.ID}, nil
	}); err != nil {
		return drainSnapshot{}, err
	}
	var runner *scaleset.RunnerReference
	if err := d.effect(ctx, "observe-runner", nil, func(call context.Context) (Event, error) {
		var err error
		runner, err = d.API.FindRunner(call, d.Approval.workerName())
		if err != nil {
			return Event{}, err
		}
		if runner == nil {
			return Event{DrainSnapshot: &snapshot}, nil
		}
		if runner.ID <= 0 || runner.Name != d.Approval.workerName() || runner.RunnerScaleSetID != setID {
			return Event{DrainSnapshot: &snapshot, Work: workUnresolved}, nil
		}
		snapshot.Runner = &drainRunnerIdentity{ID: runner.ID, Name: runner.Name, ScaleSetID: runner.RunnerScaleSetID}
		return Event{ID: runner.ID, DrainSnapshot: &snapshot}, nil
	}); err != nil {
		return drainSnapshot{}, err
	}
	_ = stage // Stage is retained by the caller's before/after final payload.
	return snapshot, nil
}

func validDrainIdlePrerequisite(s drainSnapshot) bool {
	return s.StatsKnown && s.Runner != nil && s.Statistics.Available == 0 && s.Statistics.Acquired == 0 && s.Statistics.Assigned == 0 && s.Statistics.Running == 0 && s.Statistics.Registered == 1 && s.Statistics.Busy == 0 && s.Statistics.Idle == 1
}

func drainMarkerFor(ctx context.Context, err error) string {
	if ctx != nil {
		switch ctx.Err() {
		case context.DeadlineExceeded:
			return drainMarkerDeadline
		case context.Canceled:
			return drainMarkerCancelled
		}
	}
	if errors.Is(err, ErrQuarantine) || errors.Is(err, ErrNoMessage) {
		return drainMarkerQuarantine
	}
	return drainMarkerQuarantine
}

func (d *Driver) recordDrainMarker(marker string) error {
	if marker == "" {
		marker = drainMarkerQuarantine
	}
	return d.record(Event{Kind: "observation", Operation: "drain-marker", DrainMarker: marker})
}

func (d *Driver) drain(ctx context.Context, setID int) error {
	opener, ok := d.API.(drainSessionOpener)
	if !ok {
		return ErrApproval
	}
	before, err := d.drainSnapshot(ctx, setID, "before")
	if err != nil {
		return err
	}
	if !validDrainIdlePrerequisite(before) {
		if markerErr := d.recordDrainMarker(drainMarkerPrerequisiteFailed); markerErr != nil {
			return markerErr
		}
		return ErrQuarantine
	}
	hook := newDrainPollHook("")
	var session Session
	var sessionID string
	err = d.effect(ctx, "session-open", nil, func(call context.Context) (Event, error) {
		var err error
		session, err = opener.OpenDrainSession(call, setID, d.Approval.setName(), hook)
		if err != nil || session == nil {
			return Event{}, ErrRemote
		}
		current := session.Session()
		if current.SessionID == [16]byte{} || current.OwnerName != d.Approval.setName() || current.RunnerScaleSet == nil || current.RunnerScaleSet.ID != setID || current.RunnerScaleSet.Name != d.Approval.setName() || current.RunnerScaleSet.RunnerGroupID != d.Approval.RunnerGroupID || !current.RunnerScaleSet.RunnerSetting.DisableUpdate || !slices.ContainsFunc(current.RunnerScaleSet.Labels, func(label scaleset.Label) bool { return label.Name == d.Approval.setName() }) || current.Statistics == nil || current.RunnerScaleSet.Statistics == nil {
			return Event{}, ErrQuarantine
		}
		stats, err := newDrainStatistics(current.Statistics)
		embeddedStats, embeddedErr := newDrainStatistics(current.RunnerScaleSet.Statistics)
		if err != nil || embeddedErr != nil || stats != before.Statistics || embeddedStats != before.Statistics {
			return Event{}, ErrQuarantine
		}
		sessionID = current.SessionID.String()
		return Event{SessionID: sessionID}, nil
	})
	if err != nil {
		if markerErr := d.recordDrainMarker(drainMarkerFor(ctx, err)); markerErr != nil {
			return markerErr
		}
		return err
	}
	journaled := &journaledDrainClient{d: d, inner: session, sessionID: sessionID}
	obs, runErr := runDrainListener(ctx, journaled, setID, hook)
	if ctx.Err() != nil {
		if markerErr := d.recordDrainMarker(drainMarkerFor(ctx, runErr)); markerErr != nil {
			return markerErr
		}
		if runErr != nil {
			return runErr
		}
		return ErrQuarantine
	}
	if runErr != nil {
		// An unknown ACK/acquisition or a timing-miss leaves the session as a
		// live reservation. Closing it can requeue or otherwise change the
		// remote state before an operator can inspect the journal.
		if markerErr := d.recordDrainMarker(drainMarkerFor(ctx, runErr)); markerErr != nil {
			return markerErr
		}
		after, afterErr := d.drainSnapshot(ctx, setID, "after")
		if afterErr != nil {
			if markerErr := d.recordDrainMarker(drainMarkerFor(ctx, afterErr)); markerErr != nil {
				return markerErr
			}
			return afterErr
		}
		obs.Before, obs.After = before, after
		obs.Outcome = drainOutcomeInconclusive
		if recordErr := d.record(Event{Kind: "observation", Operation: "drain", Drain: &obs}); recordErr != nil {
			return recordErr
		}
		return runErr
	}
	closeErr := d.effect(ctx, "session-close", nil, func(call context.Context) (Event, error) {
		if err := session.Close(call); err != nil {
			return Event{}, err
		}
		return Event{SessionID: sessionID}, nil
	})
	if closeErr != nil {
		if markerErr := d.recordDrainMarker(drainMarkerFor(ctx, closeErr)); markerErr != nil {
			return markerErr
		}
		return closeErr
	}
	after, afterErr := d.drainSnapshot(ctx, setID, "after")
	if afterErr != nil {
		if markerErr := d.recordDrainMarker(drainMarkerFor(ctx, afterErr)); markerErr != nil {
			return markerErr
		}
		return afterErr
	}
	obs.Before, obs.After = before, after
	if runErr == nil && !sameDrainRunner(before.Runner, after.Runner) {
		obs.Outcome = drainOutcomeInconclusive
		runErr = ErrNoMessage
	}
	if err := d.record(Event{Kind: "observation", Operation: "drain", Drain: &obs}); err != nil {
		return err
	}
	return runErr
}
