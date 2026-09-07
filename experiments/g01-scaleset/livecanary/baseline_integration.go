package livecanary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

type pairedBaselineCollection struct {
	Result             controllerRecordRef `json:"result"`
	Outcome            collectionOutcome   `json:"outcome"`
	Rounds             int                 `json:"rounds"`
	OutstandingSession sessionOutstanding  `json:"outstanding_session"`
	SessionIntent      controllerRecordRef `json:"session_intent"`
	SessionResult      controllerRecordRef `json:"session_result"`
}
type pairedBaselineCadence struct {
	now  func() time.Time
	wait func(context.Context, time.Duration) error
}

func realBaselineCadence() pairedBaselineCadence {
	return pairedBaselineCadence{now: time.Now, wait: func(ctx context.Context, d time.Duration) error {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ErrQuarantine
		case <-timer.C:
			return nil
		}
	}}
}

type pairedBaselineScope struct {
	cadence pairedBaselineCadence

	ctx                 context.Context
	driver              *Driver
	workerDriver        *liveworker.Driver
	approval            Approval
	workerApproval      liveworker.Approval
	api                 *SDKAPI
	captured            SDKAPI
	docker              *liveworker.Docker
	journal             *FileJournal
	workerJournal       *liveworker.FileJournal
	identity            controllerJournalIdentity
	creation, inventory controllerRecordRef
	setID               int
	listener            *baselineListener
	worker              *liveworker.PairedWorker
	binding             liveworker.PairBinding
	pair                *liveworker.PairReceipt
	pairIntent          controllerRecordRef
	lastRound           time.Time
}

func runPairedBaseline(ctx context.Context, d *Driver, w *liveworker.Driver) (pairedBaselineCollection, error) {
	return runPairedBaselineWithCadence(ctx, d, w, realBaselineCadence())
}

// The production entry fixes the real clock. Tests can advance only cadence;
// approval, network and scope deadlines always use their original real context.
func runPairedBaselineWithCadence(ctx context.Context, d *Driver, w *liveworker.Driver, cadence pairedBaselineCadence) (out pairedBaselineCollection, err error) {
	out = pairedBaselineCollection{Outcome: collectionUnresolved, OutstandingSession: sessionNone}
	if ctx == nil || ctx.Err() != nil || d == nil || w == nil || cadence.now == nil || cadence.wait == nil {
		return out, ErrApproval
	}
	j, ok := d.Journal.(*FileJournal)
	wj, wok := w.Journal.(*liveworker.FileJournal)
	api, aok := d.API.(*SDKAPI)
	docker, dok := w.Runtime.(*liveworker.Docker)
	if !ok || j == nil || !wok || wj == nil || !aok || api == nil || !dok || docker == nil {
		return out, ErrApproval
	}
	a := d.Approval
	a.Phases = slices.Clone(a.Phases)
	a.ActionsHosts = slices.Clone(a.ActionsHosts)
	wa := w.Approval
	wa.Phases = slices.Clone(wa.Phases)
	release, err := j.authorize(a)
	if err != nil {
		return out, ErrJournal
	}
	defer release()
	history := replay(j.Events())
	if history.workObserved {
		return out, ErrQuarantine
	}
	initial, err := newBaselineListenerHeld(ctx, a, j, api, history.setID)
	if err != nil {
		return out, err
	}
	defer initial.cancel()
	deadline := time.Now().Add(10 * time.Minute)
	for _, limit := range []time.Time{a.ExpiresAt, wa.ExpiresAt, api.credentials.ExpiresAt} {
		if limit.Before(deadline) {
			deadline = limit
		}
	}
	outer, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	s := &pairedBaselineScope{cadence: cadence, ctx: outer, driver: d, workerDriver: w, approval: a, workerApproval: wa, api: api, captured: initial.api, docker: docker, journal: j, workerJournal: wj, identity: initial.identity, creation: initial.creation, setID: history.setID, listener: initial}
	for _, e := range j.Events() {
		if e.Kind == "inventory" {
			if s.inventory.Sequence != 0 {
				return out, ErrQuarantine
			}
			s.inventory = controllerEventRef(s.identity, e)
		}
	}
	if !refPresent(s.inventory) {
		return out, ErrQuarantine
	}
	// Captured concrete dependencies are the ones used by every later call.
	capturedW := *w
	capturedW.Approval = wa
	err = capturedW.WithPairedExecution(outer, pairInput(s.identity, a, s.setID, s.creation), s.checkControllerOnly, func(worker *liveworker.PairedWorker) error {
		s.worker = worker
		s.binding = worker.Binding()
		operationErr := s.execute()
		var storageErr error
		out, storageErr = s.summarize(operationErr)
		if operationErr != nil {
			return operationErr
		}
		return storageErr
	})
	if err != nil {
		return out, ErrQuarantine
	}
	return out, nil
}
func (s *pairedBaselineScope) current() error {
	if s.ctx.Err() != nil || s.driver.Journal != s.journal || s.driver.API != s.api || s.workerDriver.Journal != s.workerJournal || s.workerDriver.Runtime != s.docker || approvalDigest(s.driver.Approval) != approvalDigest(s.approval) || workerApprovalDigest(s.workerDriver.Approval) != workerApprovalDigest(s.workerApproval) || s.api.client != s.captured.client || s.api.rest != s.captured.rest || s.api.baseURL != s.captured.baseURL || s.api.credentials != s.captured.credentials || approvalDigest(s.api.approval) != approvalDigest(s.approval) {
		return ErrQuarantine
	}
	s.journal.mu.Lock()
	defer s.journal.mu.Unlock()
	id, err := s.journal.controllerIdentity()
	if err != nil || id != s.identity || !s.journal.authorityHeld(s.approval) {
		return ErrJournal
	}
	return nil
}
func workerApprovalDigest(a liveworker.Approval) string {
	data, _ := json.Marshal(a)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
func (s *pairedBaselineScope) state() (baselineHistory, error) {
	return replayBaseline(s.journal.Events(), s.identity, s.approval)
}
func (s *pairedBaselineScope) event(ref liveworker.RecordRef) *Event {
	events := s.journal.Events()
	if ref.Sequence <= 0 || ref.Sequence > len(events) {
		return nil
	}
	e := events[ref.Sequence-1]
	if controllerEventRef(s.identity, e) != controllerRef(ref) {
		return nil
	}
	return &e
}

// This checker never calls W or the listener, including from b.mu-held afterAcquire.
func (s *pairedBaselineScope) checkControllerOnly(c liveworker.ControllerCheck) error {
	if err := s.current(); err != nil {
		return err
	}
	event := s.event(c.ControllerRecord)
	if event == nil {
		return ErrJournal
	}
	if c.Stage == liveworker.CheckScope {
		if c.ControllerRecord != workerRef(s.creation) || event.Kind != "result" || event.Operation != "create" || event.ID != s.setID {
			return ErrQuarantine
		}
		if s.pair != nil && (c.PairSHA256 != s.pair.PairSHA256 || c.WorkerRecord != s.pair.WorkerBound) {
			return ErrQuarantine
		}
		return nil
	}
	if s.binding.Version != 1 || c.PairSHA256 != pairDigest(s.binding) {
		return ErrQuarantine
	}
	r := event.Baseline
	if c.Stage == liveworker.CheckBind {
		if r == nil || r.Stage != "pair" || r.Outcome != "intent" || r.Pair == nil || r.Pair.Binding != s.binding || controllerRef(c.ControllerRecord) != s.pairIntent {
			return ErrQuarantine
		}
		if s.pair != nil && c.WorkerRecord != s.pair.WorkerBound {
			return ErrQuarantine
		}
		return nil
	}
	state, err := s.state()
	if err != nil || state.uncertain || s.pair == nil {
		return ErrQuarantine
	}
	switch c.Stage {
	case liveworker.CheckCreate:
		if r == nil || r.Stage != "handoff" || r.Outcome != "intent" || r.Handoff == nil || c.Handoff == nil || *c.Handoff != state.expectedHandoff(controllerRef(c.ControllerRecord)) || c.WorkerRecord != s.pair.WorkerBound {
			return ErrQuarantine
		}
	case liveworker.CheckStart:
		if r == nil || r.Stage != "handoff" || r.Outcome != "result" || r.Handoff == nil || r.Handoff.Container == nil || c.ControllerRecord != workerRef(state.handoffRef) || c.WorkerRecord != r.Handoff.Container.CreateResult {
			return ErrQuarantine
		}
	default:
		return ErrQuarantine
	}
	return nil
}
func (s *pairedBaselineScope) checkPair() error {
	if err := s.current(); err != nil {
		return err
	}
	if s.pair == nil || s.worker == nil {
		return ErrQuarantine
	}
	receipt, err := s.worker.Bind(workerRef(s.pairIntent))
	if err != nil || receipt != *s.pair {
		return ErrQuarantine
	}
	return s.current()
}
func (s *pairedBaselineScope) store(r baselineRecord) (controllerRecordRef, error) {
	r.Version = 1
	r.SetID = s.setID
	r.Creation = s.creation
	return s.journal.appendRecord(Event{Kind: "baseline", Baseline: &r})
}
func (s *pairedBaselineScope) begin(r baselineRecord) (baselineRecord, error) {
	if err := s.checkPair(); err != nil {
		return r, err
	}
	if err := s.journal.baselineCapacity(); err != nil {
		return r, err
	}
	r.Outcome = "intent"
	ref, err := s.store(r)
	r.Intent = ref
	if err == nil {
		err = s.boundary()
	}
	return r, err
}
func (s *pairedBaselineScope) boundary() error {
	if err := s.checkPair(); err != nil {
		return err
	}
	return s.journal.baselineCapacity()
}
func (s *pairedBaselineScope) finish(r baselineRecord, known bool) (controllerRecordRef, error) {
	r.Outcome = "unknown"
	if known {
		r.Outcome = "result"
	}
	ref, err := s.store(r)
	if err != nil {
		return ref, ErrJournal
	}
	if !known {
		return ref, ErrQuarantine
	}
	return ref, nil
}
func (s *pairedBaselineScope) execute() error {
	if err := s.current(); err != nil {
		return err
	}
	if err := s.journal.baselineCapacity(); err != nil {
		return err
	}
	r := baselineRecord{Stage: "pair", Outcome: "intent", Pair: &baselinePair{Binding: s.binding}}
	intent, err := s.store(r)
	s.pairIntent = intent
	if err != nil {
		return err
	}
	if err = s.current(); err != nil {
		return err
	}
	if err = s.journal.baselineCapacity(); err != nil {
		return err
	}
	receipt, callErr := s.worker.Bind(workerRef(intent))
	r.Intent = intent
	if callErr == nil {
		r.Pair.Receipt = &receipt
	}
	_, err = s.finish(r, callErr == nil)
	if err != nil {
		return err
	}
	s.pair = &receipt
	state, err := s.state()
	if err != nil {
		return err
	}
	r, err = s.begin(baselineRecord{Stage: "host-preflight", Host: &baselineHost{Pair: state.pairRef, ApprovalSHA256: workerApprovalDigest(s.workerApproval), DaemonID: s.workerApproval.DaemonID, ImageID: s.workerApproval.ImageID, Image: s.workerApproval.Image}})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
	_, callErr = s.docker.Preflight(ctx, s.workerApproval)
	cancel()
	_, err = s.finish(r, callErr == nil)
	if err != nil {
		return err
	}
	if err = s.boundary(); err != nil {
		return err
	}
	state, err = s.state()
	if err != nil {
		return err
	}
	r, err = s.begin(baselineRecord{Stage: "roster-anchor", Roster: &baselineRoster{Pair: state.pairRef, Host: state.hostRef, Inventory: s.inventory}})
	if err != nil {
		return err
	}
	observation, callErr := s.captured.observeRoster(s.ctx)
	r.Roster.Observation = &observation
	prior := s.event(workerRef(s.inventory))
	known := callErr == nil && observation.Completeness == rosterComplete && prior != nil && prior.Digest == observation.Digest
	_, err = s.finish(r, known)
	if err != nil {
		return err
	}
	if err = s.boundary(); err != nil {
		return err
	}
	b, err := newPairedBaselineListenerHeld(s)
	if err != nil {
		return err
	}
	err = b.run(s.afterAcquire)
	if err != nil {
		return err
	}
	state, err = s.state()
	if err != nil || !state.complete || state.runnerID != sdkRunnerID(state.jit.Runner.ID) || state.runnerName != state.jit.Runner.Name {
		return ErrQuarantine
	}
	return s.sampleRounds(5, 8)
}
func newPairedBaselineListenerHeld(s *pairedBaselineScope) (*baselineListener, error) {
	if err := s.boundary(); err != nil {
		return nil, err
	}
	state, err := s.state()
	if err != nil || state.uncertain || state.pending != nil || state.pairRef.Sequence == 0 || state.hostRef.Sequence == 0 || state.rosterRef.Sequence == 0 || state.setObserved || state.sessionID != "" || s.listener.used || state.pairIntent != s.pairIntent || state.pair.Binding != s.binding || *state.pair.Receipt != *s.pair {
		return nil, ErrQuarantine
	}
	b := s.listener
	b.cancel()
	b.ctx, b.cancel = context.WithCancel(s.ctx)
	b.pairGuard = s.boundary
	return b, nil
}
func (s *pairedBaselineScope) summarize(callErr error) (pairedBaselineCollection, error) {
	state, err := s.state()
	out := pairedBaselineCollection{Outcome: collectionUnresolved, OutstandingSession: state.outstanding(), Rounds: state.rounds, SessionIntent: state.sessionIntent, SessionResult: state.sessionResult}
	if err != nil {
		return out, ErrJournal
	}
	if callErr == nil {
		out.Outcome = collectionIncomplete
		if state.collected() {
			out.Outcome = collectionCollected
		}
	}
	r := baselineRecord{Stage: "collection", Outcome: "observed", SessionID: state.sessionID, Collection: &baselineCollectionFacts{Outcome: out.Outcome, Pair: state.pairRef, Start: state.startRef, Completed: state.completedRef, LastSample: state.lastSample, Rounds: state.rounds, OutstandingSession: state.outstanding(), SessionIntent: state.sessionIntent, SessionResult: state.sessionResult}}
	if state.seen && state.pending == nil && state.child == nil {
		out.Result, err = s.store(r)
		if err != nil {
			out.Outcome = collectionUnresolved
			return out, ErrJournal
		}
	}
	return out, nil
}
