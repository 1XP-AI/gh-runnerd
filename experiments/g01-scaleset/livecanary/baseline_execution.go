package livecanary

import (
	"context"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

func (s *pairedBaselineScope) afterAcquire(ctx context.Context, acquisition baselineAcquisition) error {
	if ctx.Err() != nil {
		return ErrQuarantine
	}
	state, err := s.state()
	if err != nil || state.pending == nil || state.pending.Stage != "continuation" || state.acquireRef != acquisition.Result {
		return ErrQuarantine
	}
	parent := state.pendingRef
	r, err := s.begin(baselineRecord{Stage: "jit", SessionID: state.sessionID, JIT: &baselineJIT{Parent: parent, Pair: state.pairRef, Acquire: state.acquireRef, RequestID: acquisition.RequestID, ExpectedName: s.approval.workerName()}})
	if err != nil {
		return err
	}
	if s.listener == nil {
		return ErrQuarantine
	}
	// AcquireJobs invokes this continuation while the listener mutex is held.
	// wire reads the already-captured session tuple directly for this handoff;
	// taking the listener accessors here would recursively lock that mutex.
	capture := s.listener.wire("jit")
	capture.runnerName = s.approval.workerName()
	if capture.origin == "" || !capture.runtimePathPrefixSet || capture.runtimePathPrefix == "" || capture.runnerName == "" {
		return ErrQuarantine
	}
	request, cancel := context.WithTimeout(ctx, operationTimeout)
	got, callErr := s.captured.GenerateJIT(capture.context(request), s.setID, s.approval.workerName())
	cancel()
	wire, status, observed := capture.takeJIT()
	r.HTTPStatus = status
	known := callErr == nil && observed && status == 200 && wire != nil && wire.Runner != nil && got != nil && got.Runner != nil && *wire.Runner == *got.Runner && wire.EncodedJITConfig == got.EncodedJITConfig && wire.Runner.ID > 0 && wire.Runner.Name == s.approval.workerName() && wire.Runner.RunnerScaleSetID == s.setID
	// The transport already observed this bounded response. Cancellation may
	// stop SDK decoding; preserve its expected runner tuple as unknown evidence.
	// Only full SDK agreement below supplies a transient handoff secret.
	if observed && status == 200 && wire != nil && wire.Runner != nil && wire.Runner.ID > 0 && wire.Runner.Name == s.approval.workerName() && wire.Runner.RunnerScaleSetID == s.setID {
		r.JIT.Runner = &liveworker.SDKRunnerIdentity{ID: liveworker.SDKRunnerID(wire.Runner.ID), Name: wire.Runner.Name, ScaleSetID: wire.Runner.RunnerScaleSetID}
	}
	secret := ""
	if known {
		secret = got.EncodedJITConfig
	}
	if got != nil {
		got.EncodedJITConfig = ""
	}
	if wire != nil {
		wire.EncodedJITConfig = ""
	}
	defer func() { secret = "" }()
	_, err = s.finish(r, known)
	if err != nil {
		return err
	}
	state, err = s.state()
	if err != nil {
		return err
	}
	r, err = s.begin(baselineRecord{Stage: "handoff", SessionID: state.sessionID, Handoff: &baselineHandoff{Parent: parent, Input: state.expectedHandoff(controllerRecordRef{})}})
	if err != nil {
		return err
	}
	input := state.expectedHandoff(r.Intent)
	container, callErr := s.worker.Create(input, secret)
	secret = ""
	r.Handoff.Input = input
	if container.CreateResult.Sequence != 0 {
		r.Handoff.Container = &container
	}
	_, err = s.finish(r, callErr == nil)
	if err != nil {
		return err
	}
	state, err = s.state()
	if err != nil {
		return err
	}
	r, err = s.begin(baselineRecord{Stage: "worker-start", SessionID: state.sessionID, Start: &baselineStart{Parent: parent, Handoff: state.handoffRef, Container: container}})
	if err != nil {
		return err
	}
	started, callErr := s.worker.Start(container, workerRef(state.handoffRef))
	r.Start.WorkerResult = started
	_, err = s.finish(r, callErr == nil)
	if err != nil {
		return err
	}
	return s.sampleRounds(1, 4)
}

func (s *pairedBaselineScope) sampleRounds(first, last int) error {
	for number := first; number <= last; number++ {
		if !s.lastRound.IsZero() {
			wait := s.lastRound.Add(5 * time.Second).Sub(s.cadence.now())
			if wait > 0 {
				if err := s.cadence.wait(s.ctx, wait); err != nil {
					return err
				}
			}
			if s.cadence.now().Before(s.lastRound.Add(5 * time.Second)) {
				return ErrQuarantine
			}
		}
		if err := s.boundary(); err != nil {
			return err
		}
		state, err := s.state()
		if err != nil {
			return err
		}
		f := &baselineSample{Round: number, Start: state.startRef, Previous: state.lastSample}
		if number <= 4 {
			f.Parent = state.pendingRef
		} else {
			f.Completed = state.completedRef
		}
		r, err := s.begin(baselineRecord{Stage: "identity-sample", SessionID: state.sessionID, Sample: f})
		if err != nil {
			return err
		}
		request, cancel := context.WithTimeout(s.ctx, operationTimeout)
		callErr := s.sample(request, f, state)
		cancel()
		// Replay is also the single consistency policy used before accepting a round.
		consistent := callErr == nil && state.acceptSample(f, s.approval)
		_, err = s.finish(r, consistent)
		if err != nil {
			return err
		}
		s.lastRound = s.cadence.now()
	}
	return nil
}
func (s *pairedBaselineScope) sample(ctx context.Context, f *baselineSample, state baselineHistory) error {
	boundary := func() error {
		if ctx.Err() != nil {
			return ErrQuarantine
		}
		return s.boundary()
	}
	if err := boundary(); err != nil {
		return err
	}
	sdk, err := s.captured.observeSDKRunner(ctx, sdkRunnerID(state.jit.Runner.ID), state.jit.Runner.Name, s.setID)
	f.SDK = &sdk
	trial := state
	if err == nil && !trial.acceptSampleThrough(f, s.approval, 1) {
		err = ErrQuarantine
	}
	if err != nil {
		return err
	}
	if err = boundary(); err != nil {
		return err
	}
	previous := restJobID(0)
	if state.lastJob != nil {
		previous = state.lastJob.ID
	}
	job, err := s.captured.observeRESTJob(ctx, 1, previous)
	f.Job = &job
	trial = state
	if err == nil && !trial.acceptSampleThrough(f, s.approval, 2) {
		err = ErrQuarantine
	}
	if err != nil {
		return err
	}
	// A current association or a retained earlier positive association selects
	// REST's own ID; the SDK ID is never cast into a REST target.
	association := job
	if state.lastJob != nil {
		if association.RunnerID == nil || *association.RunnerID == 0 {
			association.RunnerID = state.lastJob.RunnerID
		}
		if association.RunnerName == nil || *association.RunnerName == "" {
			association.RunnerName = state.lastJob.RunnerName
		}
		if association.RunnerGroupID == nil || *association.RunnerGroupID == 0 {
			association.RunnerGroupID = state.lastJob.RunnerGroupID
		}
	}
	if association.RunnerID != nil && *association.RunnerID > 0 && association.RunnerName != nil && *association.RunnerName == state.jit.Runner.Name && association.RunnerGroupID != nil && *association.RunnerGroupID == int64(s.approval.RunnerGroupID) {
		f.RESTAddressable = true
		if err = boundary(); err != nil {
			return err
		}
		runner, err := s.captured.observeRESTRunner(ctx, *association.RunnerID, state.jit.Runner.Name)
		f.REST = &runner
		if err != nil {
			return err
		}
	}
	trial = state
	if !trial.acceptSampleThrough(f, s.approval, 3) {
		return ErrQuarantine
	}
	if err = boundary(); err != nil {
		return err
	}
	// W.Observe uses its original scope and existing separate network deadlines.
	local, err := s.worker.Observe()
	f.Local = &local
	return err
}
