package livecanary

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
)

var errBaselineCollected = errors.New("baseline callback collection complete")

// Private protocol collection under the caller's concrete controller lease.
// No current phase/CLI calls this. The future pair orchestrator must prove
// completed pairing and host preflight before invoking this experiment slice.
type baselineListener struct {
	finalizer                *pairedBaselineScope
	finalizing               bool
	pairGuard                func() error
	mu                       sync.Mutex
	ctx                      context.Context
	cancel                   context.CancelFunc
	approval                 Approval
	journal                  *FileJournal
	api                      SDKAPI
	identity                 controllerJournalIdentity
	creation                 controllerRecordRef
	setID                    int
	session                  Session
	initial                  scaleset.RunnerScaleSetSession
	sessionID, queue, origin string
	runtimePathPrefix        string
	runtimePathPrefixSet     bool
	running, used, invalid   bool
	after                    func(context.Context, baselineAcquisition) error
}

func newBaselineListenerHeld(ctx context.Context, a Approval, j *FileJournal, api *SDKAPI, setID int) (*baselineListener, error) {
	if ctx == nil || ctx.Err() != nil || j == nil || api == nil || api.client == nil || api.rest == nil || setID <= 0 || a.WorkflowRunID <= 0 || !j.authorityHeld(a) || approvalDigest(a) != approvalDigest(api.approval) {
		return nil, ErrApproval
	}
	_, cancel, err := api.observationContext(ctx, true)
	if err != nil {
		return nil, ErrApproval
	}
	cancel()
	id, err := j.controllerIdentity()
	if err != nil {
		return nil, err
	}
	events := j.Events()
	s := replay(events)
	if s.setID != setID || s.uncertain || s.reserved || s.deleted || len(s.observedJobs) != 0 {
		return nil, ErrQuarantine
	}
	var creation controllerRecordRef
	for _, e := range events {
		if e.Baseline != nil || (e.Kind == "phase" && e.Operation != "create") || e.Operation == "jit" || e.Operation == "acquire" || e.Operation == "session-open" {
			return nil, ErrQuarantine
		}
		if e.Kind == "result" && e.Operation == "create" && e.ID == setID {
			if creation.Sequence != 0 {
				return nil, ErrQuarantine
			}
			creation = controllerEventRef(id, e)
		}
	}
	if creation.Sequence == 0 {
		return nil, ErrQuarantine
	}
	a.Phases = slices.Clone(a.Phases)
	a.ActionsHosts = slices.Clone(a.ActionsHosts)
	captured := *api
	captured.approval = a
	captured.options = slices.Clone(api.options)
	deadline := time.Now().Add(10 * time.Minute)
	if a.ExpiresAt.Before(deadline) {
		deadline = a.ExpiresAt
	}
	if api.credentials.ExpiresAt.Before(deadline) {
		deadline = api.credentials.ExpiresAt
	}
	original, stop := context.WithDeadline(ctx, deadline)
	return &baselineListener{ctx: original, cancel: stop, approval: a, journal: j, api: captured, identity: id, creation: creation, setID: setID}, nil
}
func (b *baselineListener) check() error {
	if b == nil || !b.running || b.invalid || b.finalizing || b.ctx.Err() != nil || !b.journal.authorityHeld(b.approval) {
		return ErrQuarantine
	}
	id, err := b.journal.controllerIdentity()
	if err != nil || id != b.identity {
		return ErrJournal
	}
	if b.pairGuard != nil {
		if err := b.pairGuard(); err != nil {
			return err
		}
	}
	if b.session != nil && b.session.Session().SessionID.String() != b.sessionID {
		return ErrQuarantine
	}
	return nil
}
func (b *baselineListener) state() (baselineHistory, error) {
	return replayBaseline(b.journal.Events(), b.identity, b.approval)
}
func (b *baselineListener) enter() error {
	if b == nil || !b.mu.TryLock() {
		return ErrQuarantine
	}
	if err := b.check(); err != nil {
		b.mu.Unlock()
		return err
	}
	return nil
}
func (b *baselineListener) record(r baselineRecord) (controllerRecordRef, error) {
	r.Version = 1
	r.SetID = b.setID
	r.Creation = b.creation
	ref, err := b.journal.appendRecord(Event{Kind: "baseline", Baseline: &r})
	if err != nil {
		b.invalid = true
	}
	return ref, err
}
func (b *baselineListener) begin(stage string, configure func(*baselineRecord)) (baselineRecord, error) {
	if err := b.check(); err != nil {
		return baselineRecord{}, err
	}
	if err := b.journal.baselineCapacity(); err != nil {
		return baselineRecord{}, err
	}
	r := baselineRecord{Stage: stage, Outcome: "intent", SessionID: b.sessionID}
	if configure != nil {
		configure(&r)
	}
	ref, err := b.record(r)
	r.Intent = ref
	if err == nil {
		err = b.check()
	} // Intent fsync may outlive authority or file identity.
	return r, err
}

// Known responses survive cancellation; every subsequent operation rechecks
// the original authority before its request or continuation.
func (b *baselineListener) finish(r baselineRecord, known bool) (controllerRecordRef, error) {
	r.Outcome = "result"
	if !known {
		r.Outcome = "unknown"
	}
	ref, err := b.record(r)
	if err != nil {
		return controllerRecordRef{}, ErrJournal
	}
	if !known {
		b.invalid = true
		return ref, ErrQuarantine
	}
	return ref, nil
}
func (b *baselineListener) wire(stage string) *baselineWireCapture {
	return &baselineWireCapture{stage: stage, setID: b.setID, organization: b.approval.Organization, owner: b.approval.setName(), queue: b.queue, origin: b.origin, runtimePathPrefix: b.runtimePathPrefix, runtimePathPrefixSet: b.runtimePathPrefixSet, allowedHosts: baselineWireAllowedHosts(b.approval, b.api.drainEndpointHost())}
}

func (b *baselineListener) capturedOrigin() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.origin
}

func (b *baselineListener) capturedRuntimePathPrefix() (string, bool) {
	if b == nil {
		return "", false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.runtimePathPrefix, b.runtimePathPrefixSet && !b.invalid
}
func (b *baselineListener) run(after func(context.Context, baselineAcquisition) error) error {
	if b == nil || b.ctx == nil || b.cancel == nil || !b.mu.TryLock() {
		return ErrQuarantine
	}
	if b.used || after == nil {
		b.cancel()
		b.mu.Unlock()
		return ErrApproval
	}
	b.used = true
	b.running = true
	b.after = after
	b.mu.Unlock()
	defer func() {
		b.cancel()
		b.mu.Lock()
		b.running = false
		b.invalid = true
		b.after = nil
		b.mu.Unlock()
	}()
	if err := b.initialize(); err != nil {
		return err
	}
	l, err := listener.New(b, listener.Config{ScaleSetID: b.setID, MaxRunners: 1})
	if err != nil {
		return ErrQuarantine
	}
	err = l.Run(b.ctx, b)
	if errors.Is(err, errBaselineCollected) {
		if b.finalizer != nil {
			b.mu.Lock()
			if b.check() != nil {
				b.mu.Unlock()
				return ErrQuarantine
			}
			b.finalizing = true
			scope := b.finalizer
			b.mu.Unlock()
			return scope.finalizeTerminal()
		}
		return nil
	}
	return ErrQuarantine
}
func (b *baselineListener) initialize() error {
	if err := b.enter(); err != nil {
		return err
	}
	defer b.mu.Unlock()
	r, err := b.begin("set-observe", nil)
	if err != nil {
		return err
	}
	c := b.wire("set-observe")
	ctx, cancel := context.WithTimeout(b.ctx, operationTimeout)
	set, callErr := b.api.GetScaleSet(c.context(ctx), b.setID)
	cancel()
	r.Set = c.set
	r.HTTPStatus = c.status
	prefix, prefixKnown := c.requestRuntimePathPrefix()
	known := c.observed() && prefixKnown && r.Set.eligible(b.approval, b.setID) && ((callErr != nil && b.ctx.Err() != nil) || (callErr == nil && set != nil && set.ID == r.Set.ID && set.Name == r.Set.Name && set.RunnerGroupID == r.Set.GroupID && set.RunnerSetting.DisableUpdate && r.Set.Statistics.matches(set.Statistics)))
	if prefixKnown {
		b.runtimePathPrefix = prefix
		b.runtimePathPrefixSet = true
	}
	if _, err = b.finish(r, known); err != nil {
		return err
	}
	r, err = b.begin("session-open", nil)
	if err != nil {
		return err
	}
	c = b.wire("session-open")
	ctx, cancel = context.WithTimeout(b.ctx, operationTimeout)
	session, callErr := b.api.OpenSession(c.context(ctx), b.setID, b.approval.setName())
	cancel()
	sf, _, _, status := c.facts()
	origin := c.requestOrigin()
	sessionPrefix, sessionPrefixKnown := c.requestRuntimePathPrefix()
	r.Session = sf
	r.HTTPStatus = status
	known = c.observed() && sessionPrefixKnown && sf.eligible(b.approval, b.setID) && ((callErr != nil && b.ctx.Err() != nil) || (callErr == nil && session != nil))
	var initial scaleset.RunnerScaleSetSession
	if callErr == nil && session != nil {
		initial = session.Session()
	}
	if known && callErr == nil {
		known = initial.SessionID.String() == sf.SessionID && initial.OwnerName == sf.Owner && sf.Statistics.matches(initial.Statistics) && initial.MessageQueueURL != ""
	}
	if known && (origin == "" || !b.runtimePathPrefixSet || sessionPrefix != b.runtimePathPrefix) {
		known = false
	}
	if known && origin != "" {
		b.origin = origin
	}
	if callErr == nil && session != nil && sf != nil && initial.SessionID.String() == sf.SessionID && initial.OwnerName == b.approval.setName() {
		b.session = session
		b.sessionID = sf.SessionID
		b.queue = initial.MessageQueueURL
	}
	if _, err = b.finish(r, known); err != nil {
		return err
	}
	if callErr != nil || b.check() != nil {
		return ErrQuarantine
	}
	b.session = session
	b.sessionID = sf.SessionID
	b.queue = initial.MessageQueueURL
	b.initial = scaleset.RunnerScaleSetSession{SessionID: initial.SessionID, OwnerName: initial.OwnerName, Statistics: initial.Statistics}
	return nil
}
func (b *baselineListener) Session() scaleset.RunnerScaleSetSession {
	if b == nil {
		return scaleset.RunnerScaleSetSession{}
	}
	if !b.mu.TryLock() {
		return scaleset.RunnerScaleSetSession{}
	}
	defer b.mu.Unlock()
	if b.check() != nil {
		return scaleset.RunnerScaleSetSession{}
	}
	copy := b.initial
	if copy.Statistics != nil {
		x := *copy.Statistics
		copy.Statistics = &x
	}
	return copy
}
func (b *baselineListener) GetMessage(_ context.Context, last, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	if err := b.enter(); err != nil {
		return nil, err
	}
	defer b.mu.Unlock()
	s, err := b.state()
	if err != nil || last != s.cursor || capacity != 1 || s.polls >= 16 {
		return nil, ErrQuarantine
	}
	r, err := b.begin("poll", func(r *baselineRecord) { r.Cursor = last })
	if err != nil {
		return nil, err
	}
	c := b.wire("poll")
	c.cursor = last
	ctx, cancel := context.WithTimeout(b.ctx, operationTimeout)
	m, callErr := b.session.GetMessage(c.context(ctx), last, 1)
	cancel()
	_, batch, _, status := c.facts()
	r.Batch = batch
	r.HTTPStatus = status
	r.NoMessage = status == 202 && m == nil
	known := c.observed() && (callErr == nil || b.ctx.Err() != nil) && (r.NoMessage || (batch != nil && (batch.matches(m) || callErr != nil && b.ctx.Err() != nil) && batch.MessageID > 0 && !s.messageIDs[batch.MessageID] && batch.Statistics.eligible(s.acquired)))
	if known && !r.NoMessage {
		copyState := s
		if err := copyState.admitBatch(batch, b.approval); err != nil {
			known = false
		}
	}
	batchRef, err := b.finish(r, known)
	if err != nil {
		return nil, err
	}
	if callErr != nil || b.check() != nil {
		return nil, ErrQuarantine
	}
	if r.NoMessage {
		return nil, nil
	}
	r, err = b.begin("source", func(r *baselineRecord) { r.BatchRef = batchRef })
	if err != nil {
		return nil, err
	}
	// The strict source reader requires this separate credential/context gate.
	vctx, vcancel, callErr := b.api.observationContext(b.ctx, true)
	var response observationResponse
	if callErr == nil {
		response, callErr = b.api.observeApprovedRun(vctx)
		vcancel()
	}
	r.HTTPStatus = response.Status
	if callErr == nil && response.Status == 200 {
		r.Source = baselineApprovedSource(b.approval)
	}
	if _, err = b.finish(r, r.Source != nil); err != nil {
		return nil, err
	}
	if err = b.check(); err != nil {
		return nil, err
	}
	data, _ := json.Marshal(m)
	var copy scaleset.RunnerScaleSetMessage
	if json.Unmarshal(data, &copy) != nil {
		return nil, ErrQuarantine
	}
	return &copy, nil
}
func (b *baselineListener) DeleteMessage(_ context.Context, id int) error {
	if err := b.enter(); err != nil {
		return err
	}
	defer b.mu.Unlock()
	s, err := b.state()
	if err != nil || s.batch == nil || id != s.batch.MessageID {
		return ErrQuarantine
	}
	r, err := b.begin("ack", func(r *baselineRecord) { r.MessageID = id; r.BatchRef = s.batchRef; r.SourceRef = s.sourceRef })
	if err != nil {
		return err
	}
	c := b.wire("ack")
	c.cursor = id
	ctx, cancel := context.WithTimeout(b.ctx, operationTimeout)
	callErr := b.session.DeleteMessage(c.context(ctx), id)
	cancel()
	_, _, _, status := c.facts()
	_, err = b.finish(r, c.observed() && status == 204 && (callErr == nil || b.ctx.Err() != nil) && b.session.Session().SessionID.String() == b.sessionID)
	return err
}
func (b *baselineListener) AcquireJobs(_ context.Context, ids []int64) ([]int64, error) {
	if err := b.enter(); err != nil {
		return nil, err
	}
	defer b.mu.Unlock()
	ids = slices.Clone(ids) // Response checks and the receipt use the same entry snapshot.

	s, err := b.state()
	if err != nil || s.anchor == nil || len(ids) != 1 || ids[0] != s.anchor.RequestID || s.acquired {
		return nil, ErrQuarantine
	}
	r, err := b.begin("acquire", func(r *baselineRecord) { r.BatchRef = s.batchRef; r.SourceRef = s.sourceRef; r.ACKRef = s.ackRef })
	if err != nil {
		return nil, err
	}
	c := b.wire("acquire")
	c.requestIDs = slices.Clone(ids)
	ctx, cancel := context.WithTimeout(b.ctx, operationTimeout)
	got, callErr := b.session.AcquireJobs(c.context(ctx), slices.Clone(ids))
	cancel()
	_, _, accepted, status := c.facts()
	r.Accepted = accepted
	r.HTTPStatus = status
	known := c.observed() && status == 200 && accepted.matches(ids) && ((callErr == nil && slices.Equal(got, ids)) || (callErr != nil && b.ctx.Err() != nil)) && b.session.Session().SessionID.String() == b.sessionID
	resultRef, err := b.finish(r, known)
	if err != nil {
		return nil, err
	}
	receipt := baselineAcquisition{b.sessionID, s.batch.MessageID, ids[0], s.anchor.JobID, s.anchor.Index, s.batchRef, s.sourceRef, s.ackRef, r.Intent, resultRef}
	continuation, err := b.begin("continuation", func(r *baselineRecord) { r.AcquireRef = resultRef })
	if err != nil {
		return nil, err
	}
	// The SDK has returned and released its session mutex. The continuation
	// never runs in a transport hook; reentrant adapter calls promptly refuse.
	callErr = b.after(b.ctx, receipt)
	if _, err = b.finish(continuation, callErr == nil); err != nil {
		return nil, err
	}
	return slices.Clone(got), nil
}
func (b *baselineListener) callback(stage string, value any) error {
	if err := b.enter(); err != nil {
		return err
	}
	defer b.mu.Unlock()
	s, err := b.state()
	if err != nil || s.batch == nil {
		return ErrQuarantine
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ErrQuarantine
	}
	for i, x := range s.batch.Items {
		kind := "JobStarted"
		if stage == "completed" {
			kind = "JobCompleted"
		}
		if x.Kind != kind || s.callbacks[i] {
			continue
		}
		got, e := decodeBaselineItem(data, i)
		if e != nil || !baselineSDKItemMatches(x, got) {
			return ErrQuarantine
		}

		_, err = b.record(baselineRecord{Stage: stage, Outcome: "observed", SessionID: b.sessionID, BatchRef: s.batchRef, ACKRef: s.ackRef, ItemIndex: &i})
		return err
	}
	return ErrQuarantine
}
func (b *baselineListener) HandleJobStarted(_ context.Context, x *scaleset.JobStarted) error {
	return b.callback("started", x)
}
func (b *baselineListener) HandleJobCompleted(_ context.Context, x *scaleset.JobCompleted) error {
	return b.callback("completed", x)
}
func (b *baselineListener) HandleDesiredRunnerCount(_ context.Context, n int) (int, error) {
	if err := b.enter(); err != nil {
		return 0, err
	}
	defer b.mu.Unlock()
	s, err := b.state()
	if err != nil || s.latest == nil || s.latest.Assigned == nil || *s.latest.Assigned != n {
		return 0, ErrQuarantine
	}
	_, err = b.record(baselineRecord{Stage: "desired", Outcome: "observed", SessionID: b.sessionID, BatchRef: s.batchRef, Desired: &n})
	if err != nil {
		return 0, err
	}
	if s.complete {
		return min(n, 1), errBaselineCollected
	}
	return min(n, 1), nil
}
