package livecanary

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/actions/scaleset"
)

type drainSessionOpener interface {
	OpenDrainSession(context.Context, int, string, *drainPollHook) (Session, error)
}

type drainScaleSetWireReader interface {
	drainGetScaleSet(context.Context, int, *baselineWireCapture) (*scaleset.RunnerScaleSet, error)
}

type drainRunnerWireReader interface {
	drainFindRunner(context.Context, string, *baselineWireCapture) (*scaleset.RunnerReference, error)
}

type drainEndpointHostReader interface {
	drainEndpointHost() string
}

type journaledDrainClient struct {
	d         *Driver
	inner     Session
	sessionID string
	setID     int
	hook      *drainPollHook
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
	if c.hook != nil {
		callCtx = c.hook.markPoll(callCtx)
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
		messageStats, statsErr := newDrainStatistics(message.Statistics)
		if statsErr != nil {
			return Event{}, ErrRemote
		}
		if c.hook != nil {
			pollIndex := 1
			if capacity == drainWithdrawnCapacity {
				pollIndex = 2
			}
			wireStats, statsKnown := c.hook.pollStatistics(pollIndex)
			if !statsKnown || wireStats != messageStats {
				return Event{}, ErrRemote
			}
		}
		if c.hook != nil {
			pollIndex := 1
			if capacity == drainWithdrawnCapacity {
				pollIndex = 2
			}
			batch, batchKnown := c.hook.pollBatch(pollIndex)
			if !batchKnown || !batch.matches(message) {
				return Event{}, ErrRemote
			}
		}
		if message.MessageID <= 0 || len(message.JobAvailableMessages) != 1 || len(message.JobAssignedMessages) != 0 || len(message.JobStartedMessages) != 0 || len(message.JobCompletedMessages) != 0 || message.Statistics == nil {
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
		var wire *baselineWireCapture
		if c.hook != nil {
			c.hook.mu.Lock()
			queue := c.hook.target
			origin := c.hook.origin
			runtimePathPrefix := c.hook.runtimePathPrefix
			runtimePathPrefixSet := c.hook.runtimePathPrefixSet
			c.hook.mu.Unlock()
			if !runtimePathPrefixSet {
				return Event{}, c.reject("ack")
			}
			setID := c.setID
			sessionID := c.sessionID
			if c.inner != nil {
				current := c.inner.Session()
				if setID <= 0 && current.RunnerScaleSet != nil {
					setID = current.RunnerScaleSet.ID
				}
				if sessionID == "" {
					sessionID = current.SessionID.String()
				}
			}
			apiHost := ""
			if c.d != nil {
				if reader, ok := c.d.API.(drainEndpointHostReader); ok {
					apiHost = reader.drainEndpointHost()
				}
			}
			var approval Approval
			if c.d != nil {
				approval = c.d.Approval
			}
			wire = &baselineWireCapture{stage: "ack", setID: setID, sessionID: sessionID, queue: queue, cursor: id, origin: origin, runtimePathPrefix: runtimePathPrefix, runtimePathPrefixSet: runtimePathPrefixSet, allowedHosts: baselineWireAllowedHosts(approval, apiHost)}
			call = wire.context(call)
		}
		if err := c.inner.DeleteMessage(call, id); err != nil {
			return Event{}, err
		}
		if wire != nil {
			_, _, _, status := wire.facts()
			if !wire.observed() || status != http.StatusNoContent {
				return Event{}, errors.New("ack response did not match the one-shot request")
			}
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
		var wire *baselineWireCapture
		if c.hook != nil {
			setID := c.setID
			if setID <= 0 {
				if session := c.inner.Session(); session.RunnerScaleSet != nil {
					setID = session.RunnerScaleSet.ID
				}
			}
			c.hook.mu.Lock()
			queue, origin := c.hook.target, c.hook.origin
			runtimePathPrefix := c.hook.runtimePathPrefix
			runtimePathPrefixSet := c.hook.runtimePathPrefixSet
			c.hook.mu.Unlock()
			if !runtimePathPrefixSet {
				return Event{}, c.reject("acquire")
			}
			apiHost := ""
			if reader, ok := c.d.API.(drainEndpointHostReader); ok {
				apiHost = reader.drainEndpointHost()
			}
			wire = &baselineWireCapture{stage: "acquire", setID: setID, queue: queue, requestIDs: slices.Clone(ids), origin: origin, runtimePathPrefix: runtimePathPrefix, runtimePathPrefixSet: runtimePathPrefixSet, allowedHosts: baselineWireAllowedHosts(c.d.Approval, apiHost)}
			call = wire.context(call)
		}
		got, err = c.inner.AcquireJobs(call, slices.Clone(ids))
		if err != nil {
			return Event{}, errors.New("acquisition response did not match the one-shot request")
		}
		if wire != nil {
			_, _, accepted, status := wire.facts()
			if !wire.requestObserved() || !wire.observed() || status != 200 || !accepted.matches(ids) {
				return Event{}, errors.New("acquisition response did not match the one-shot request")
			}
		}
		if !slices.Equal(got, ids) {
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
	snapshot, _, err := d.drainSnapshotWithOrigin(ctx, setID, stage, "")
	return snapshot, err
}

func (d *Driver) drainSnapshotWithOrigin(ctx context.Context, setID int, stage, expectedOrigin string) (drainSnapshot, string, error) {
	snapshot, origin, _, err := d.drainSnapshotWithBinding(ctx, setID, stage, expectedOrigin, "")
	return snapshot, origin, err
}

// drainSnapshotWithBinding captures the first approved runtime tenant prefix
// from the scale-set request and requires the runner request to use that same
// prefix. A non-empty expected prefix binds a later snapshot to the prefix
// already established by the drain phase.
func (d *Driver) drainSnapshotWithBinding(ctx context.Context, setID int, stage, expectedOrigin, expectedRuntimePathPrefix string) (drainSnapshot, string, string, error) {
	var snapshot drainSnapshot
	var set *scaleset.RunnerScaleSet
	setReader, setWire := d.API.(drainScaleSetWireReader)
	runnerReader, runnerWire := d.API.(drainRunnerWireReader)
	if setWire != runnerWire || (expectedOrigin != "" && !setWire) {
		return drainSnapshot{}, "", "", ErrQuarantine
	}
	origin := expectedOrigin
	runtimePathPrefix := expectedRuntimePathPrefix
	allowedHosts := []string(nil)
	if reader, ok := d.API.(drainEndpointHostReader); ok {
		allowedHosts = baselineWireAllowedHosts(d.Approval, reader.drainEndpointHost())
	}
	if err := d.effect(ctx, "observe-owned", nil, func(call context.Context) (Event, error) {
		var err error
		if setWire {
			wire := &baselineWireCapture{stage: "set-observe", setID: setID, organization: d.Approval.Organization, origin: expectedOrigin, runtimePathPrefix: expectedRuntimePathPrefix, runtimePathPrefixSet: expectedRuntimePathPrefix != "", allowedHosts: allowedHosts}
			set, err = setReader.drainGetScaleSet(call, setID, wire)
			wireSet, status := wire.setFacts()
			wireOrigin := wire.requestOrigin()
			wireRuntimePathPrefix, prefixKnown := wire.requestRuntimePathPrefix()
			if err != nil || !wire.observed() || status != 200 || wireOrigin == "" || !prefixKnown || (origin != "" && wireOrigin != origin) || !wireSet.eligibleForDrain(d.Approval, setID) || !wireSet.matches(set) {
				return Event{}, ErrQuarantine
			}
			origin = wireOrigin
			runtimePathPrefix = wireRuntimePathPrefix
		} else {
			// Synthetic API fakes used by state-machine tests have no physical
			// request origin. Production SDKAPI always takes the wire branch.
			set, err = d.API.GetScaleSet(call, setID)
		}
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
		return drainSnapshot{}, "", "", err
	}
	var runner *scaleset.RunnerReference
	if err := d.effect(ctx, "observe-runner", nil, func(call context.Context) (Event, error) {
		var err error
		if runnerWire {
			wire := &baselineWireCapture{stage: "runner-observe", runnerName: d.Approval.workerName(), organization: d.Approval.Organization, origin: origin, runtimePathPrefix: runtimePathPrefix, runtimePathPrefixSet: runtimePathPrefix != "", allowedHosts: allowedHosts}
			runner, err = runnerReader.drainFindRunner(call, d.Approval.workerName(), wire)
			wireRunner, status := wire.runnerFacts()
			wireOrigin := wire.requestOrigin()
			wireRuntimePathPrefix, prefixKnown := wire.requestRuntimePathPrefix()
			if err != nil || !wire.observed() || status != http.StatusOK || origin == "" || wireOrigin != origin || !prefixKnown || wireRuntimePathPrefix != runtimePathPrefix || !wireRunner.matches(runner) {
				return Event{}, ErrQuarantine
			}
		} else {
			// Synthetic API fakes used by state-machine tests have no physical
			// request origin. Production SDKAPI always takes the wire branch.
			runner, err = d.API.FindRunner(call, d.Approval.workerName())
		}
		if err != nil {
			return Event{}, err
		}
		if runner == nil {
			return Event{DrainSnapshot: &snapshot, DrainSnapshotStage: stage}, nil
		}
		if runner.ID <= 0 || runner.Name != d.Approval.workerName() || runner.RunnerScaleSetID != setID {
			return Event{DrainSnapshot: &snapshot, DrainSnapshotStage: stage, Work: workUnresolved}, nil
		}
		snapshot.Runner = &drainRunnerIdentity{ID: runner.ID, Name: runner.Name, ScaleSetID: runner.RunnerScaleSetID}
		return Event{ID: runner.ID, DrainSnapshot: &snapshot, DrainSnapshotStage: stage}, nil
	}); err != nil {
		return drainSnapshot{}, "", "", err
	}
	_ = stage // Stage is retained by the caller's before/after final payload.
	return snapshot, origin, runtimePathPrefix, nil
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
	phaseState := replayWithApproval(d.Journal.Events(), &d.Approval)
	if !phaseState.drainPhasePending || phaseState.drainPhaseSequence <= 0 || phaseState.drainPhaseSetID != setID {
		return ErrQuarantine
	}
	phaseSequence := phaseState.drainPhaseSequence
	before, origin, runtimePathPrefix, err := d.drainSnapshotWithBinding(ctx, setID, "before", "", "")
	if err != nil {
		if markerErr := d.recordDrainMarker(drainMarkerFor(ctx, err)); markerErr != nil {
			return markerErr
		}
		return err
	}
	if !validDrainIdlePrerequisite(before) {
		if markerErr := d.recordDrainMarker(drainMarkerPrerequisiteFailed); markerErr != nil {
			return markerErr
		}
		return ErrQuarantine
	}
	hook := newDrainPollHookWithOriginAndPrefix("", origin, runtimePathPrefix)
	var session Session
	var sessionID string
	err = d.effect(ctx, "session-open", nil, func(call context.Context) (Event, error) {
		var err error
		session, err = opener.OpenDrainSession(call, setID, d.Approval.setName(), hook)
		if err != nil || session == nil {
			return Event{}, ErrRemote
		}
		hook.mu.Lock()
		sessionOrigin := hook.origin
		sessionRuntimePathPrefix := hook.runtimePathPrefix
		sessionRuntimePathPrefixSet := hook.runtimePathPrefixSet
		hook.mu.Unlock()
		if sessionOrigin != origin || (runtimePathPrefix != "" && (!sessionRuntimePathPrefixSet || sessionRuntimePathPrefix != runtimePathPrefix)) {
			return Event{}, ErrQuarantine
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
	journaled := &journaledDrainClient{d: d, inner: session, sessionID: sessionID, setID: setID, hook: hook}
	obs, runErr := runDrainListener(ctx, journaled, setID, hook)
	// The bounded observation sequence is phase-local: it must identify the
	// exact durable drain phase that was active when the listener ran.
	obs.Sequence = phaseSequence
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
		after, _, _, afterErr := d.drainSnapshotWithBinding(ctx, setID, "after", origin, runtimePathPrefix)
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
		origin := ""
		runtimePathPrefix := ""
		runtimePathPrefixSet := false
		if hook != nil {
			hook.mu.Lock()
			origin = hook.origin
			runtimePathPrefix = hook.runtimePathPrefix
			runtimePathPrefixSet = hook.runtimePathPrefixSet
			hook.mu.Unlock()
		}
		if origin == "" || !runtimePathPrefixSet {
			return Event{}, ErrQuarantine
		}
		allowedHosts := []string(nil)
		if reader, ok := d.API.(drainEndpointHostReader); ok {
			allowedHosts = baselineWireAllowedHosts(d.Approval, reader.drainEndpointHost())
		}
		wire := &baselineWireCapture{stage: "terminal-session-close", setID: setID, sessionID: sessionID, origin: origin, runtimePathPrefix: runtimePathPrefix, runtimePathPrefixSet: runtimePathPrefixSet, allowedHosts: allowedHosts}
		call = wire.context(call)
		if err := session.Close(call); err != nil {
			return Event{}, err
		}
		_, _, _, status := wire.facts()
		if !wire.observed() || status != http.StatusNoContent {
			return Event{}, errors.New("session close response did not match the one-shot request")
		}
		return Event{SessionID: sessionID}, nil
	})
	if closeErr != nil {
		if markerErr := d.recordDrainMarker(drainMarkerFor(ctx, closeErr)); markerErr != nil {
			return markerErr
		}
		return closeErr
	}
	after, _, _, afterErr := d.drainSnapshotWithBinding(ctx, setID, "after", origin, runtimePathPrefix)
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
