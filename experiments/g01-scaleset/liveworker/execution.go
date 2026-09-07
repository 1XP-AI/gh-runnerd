package liveworker

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// PairedWorker is scoped to WithPairedExecution. All value copies share private
// lifetime state; it grants no authority against hostile Go callers.
type PairedWorker struct{ scope *pairedScope }
type exactRuntime interface {
	Runtime
	InspectExact(context.Context, string) (DockerInspectObservation, error)
}

type pairedScope struct {
	op        sync.Mutex
	active    atomic.Bool
	ctx       context.Context
	cancel    context.CancelFunc
	approval  Approval
	journal   *FileJournal
	runtime   exactRuntime
	checker   func(ControllerCheck) error
	binding   PairBinding
	pairSHA   string
	original  bool
	boundHere bool
	image     *ImageProfile
	failure   error
}

// WithPairedExecution requires the caller's real controller lease/checker.
// Fresh entry checks only captured controller inputs. The caller learns the
// full binding in the callback and durably records it before Bind. No entry
// check authorizes controller acquisition/JIT. The actual controller checker
// and two-journal integration are separate from this worker-local API.
func (d *Driver) WithPairedExecution(ctx context.Context, in PairInput, checkController func(ControllerCheck) error, fn func(*PairedWorker) error) error {
	if d == nil || ctx == nil || checkController == nil || fn == nil || !validPairInput(in) {
		return ErrApproval
	}
	a, candidate, runtime := d.Approval, d.Journal, d.Runtime
	a.Phases = slices.Clone(a.Phases)
	if in.OwnerNonce != a.OwnerNonce || in.ControllerName != a.Controller || in.HarnessSHA != a.HarnessSHA || in.Source.HeadSHA != a.WorkflowSHA {
		return ErrApproval
	}
	j, ok := candidate.(*FileJournal)
	r, supported := runtime.(exactRuntime)
	if !ok || j == nil || !supported || r == nil {
		return ErrState
	}
	// An interface can contain a nil pointer (or another nilable receiver).
	value := reflect.ValueOf(r)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return ErrState
		}
	}
	release, err := j.authorize(a)
	if err != nil {
		return err
	}
	defer release()
	state := j.pairState()
	if state.legacy {
		return ErrUncertain
	}
	binding := PairBinding{1, in, j.pairedIdentity()}
	if !validBinding(binding) || (state.binding != nil && *state.binding != binding) {
		return ErrState
	}
	deadline := minTime(time.Now().Add(10*time.Minute), a.ExpiresAt)
	bounded, cancel := context.WithDeadline(ctx, deadline)
	s := &pairedScope{ctx: bounded, cancel: cancel, approval: a, journal: j, runtime: r, checker: checkController, binding: binding, pairSHA: pairHash(binding), original: state.binding == nil}
	s.active.Store(true)
	defer func() {
		s.active.Store(false)
		cancel()
		s.op.Lock() // Drain an already-started operation's result recording.
		s.op.Unlock()
	}()
	if err := s.guard(ControllerCheck{Stage: CheckScope, PairSHA256: s.pairSHA, ControllerRecord: in.SetCreation, WorkerRecord: state.pair.WorkerBound}); err != nil {
		return err
	}
	err = fn(&PairedWorker{s})
	// Establish the wrapper's lifetime boundary before inspecting final failure.
	s.active.Store(false)
	cancel()
	s.op.Lock()
	defer s.op.Unlock()
	if err != nil {
		return err
	}
	if s.failure != nil {
		return s.failure
	}
	if ctx.Err() != nil || !time.Now().Before(deadline) {
		return ErrState
	}
	return nil
}

func (w *PairedWorker) Binding() PairBinding {
	if w == nil || w.scope == nil {
		return PairBinding{}
	}
	return w.scope.binding
}

func (w *PairedWorker) enter(phase string) (*pairedScope, error) {
	if w == nil || w.scope == nil {
		return nil, ErrState
	}
	s := w.scope
	if !s.op.TryLock() {
		return nil, ErrState
	}
	if !s.active.Load() || s.ctx.Err() != nil || s.failure != nil || !slices.Contains(s.approval.Phases, phase) {
		s.op.Unlock()
		return nil, ErrState
	}
	return s, nil
}

func (s *pairedScope) guard(check ControllerCheck) error {
	if !s.active.Load() || s.ctx.Err() != nil || s.failure != nil || s.journal.current(s.approval) != nil || s.journal.pairedIdentity() != s.binding.Worker {
		return ErrState
	}
	if check.Handoff != nil {
		copy := *check.Handoff
		check.Handoff = &copy
	}
	if s.checker(check) != nil {
		return ErrState
	}
	if !s.active.Load() || s.ctx.Err() != nil || s.journal.current(s.approval) != nil {
		return ErrState
	}
	return nil
}

func (s *pairedScope) store(p pairedEvent) (RecordRef, error) {
	ref, err := s.journal.appendRecord(Event{Kind: "paired", Paired: &p})
	if err != nil {
		s.failure = ErrState
	}
	return ref, err
}

func (s *pairedScope) preflight(check ControllerCheck, room int64) (ImageProfile, error) {
	if !s.journal.hasRoom(room) {
		return ImageProfile{}, ErrState
	}
	if err := s.guard(check); err != nil {
		return ImageProfile{}, err
	}
	if !s.journal.hasRoom(room) {
		return ImageProfile{}, ErrState
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	a := s.approval
	a.Phases = slices.Clone(a.Phases)
	image, err := s.runtime.Preflight(ctx, a)
	if err != nil {
		return ImageProfile{}, ErrApproval
	}
	image.Env, image.Labels = slices.Clone(image.Env), maps.Clone(image.Labels)
	return image, nil
}

func (w *PairedWorker) Bind(controllerIntent RecordRef) (PairReceipt, error) {
	s, err := w.enter("create")
	if err != nil {
		return PairReceipt{}, err
	}
	defer s.op.Unlock()
	state := s.journal.pairState()
	if !s.original || !validRef(controllerIntent) || controllerIntent.Sequence <= s.binding.Input.SetCreation.Sequence {
		return PairReceipt{}, ErrUncertain
	}
	check := ControllerCheck{Stage: CheckBind, PairSHA256: s.pairSHA, ControllerRecord: controllerIntent, WorkerRecord: state.pair.WorkerBound}
	if err := s.guard(check); err != nil {
		return PairReceipt{}, err
	}
	if state.binding != nil {
		if !s.boundHere || controllerIntent != state.pair.ControllerIntent {
			return PairReceipt{}, ErrUncertain
		}
		return state.pair, nil
	}
	if state.legacy || !s.journal.hasRoom(maxPairRecord) {
		return PairReceipt{}, ErrState
	}
	if _, err := s.store(pairedEvent{Bound: &pairBoundEvent{s.binding, controllerIntent}}); err != nil {
		return PairReceipt{}, err
	}
	s.boundHere = true
	return s.journal.pairState().pair, nil
}

func (w *PairedWorker) Create(h HandoffReceipt, jit string) (ContainerReceipt, error) {
	s, err := w.enter("create")
	if err != nil {
		return ContainerReceipt{}, err
	}
	defer s.op.Unlock()
	state := s.journal.pairState()
	if !s.original || !s.boundHere || state.binding == nil || !validHandoff(h, s.binding, state.pair) {
		return ContainerReceipt{}, ErrUncertain
	}
	check := ControllerCheck{Stage: CheckCreate, PairSHA256: s.pairSHA, ControllerRecord: h.HandoffIntent, WorkerRecord: state.pair.WorkerBound, Handoff: &h}
	if err := s.guard(check); err != nil {
		return ContainerReceipt{}, err
	}
	if state.created != nil {
		if s.image == nil || state.createInput == nil || *state.createInput.Handoff != h {
			return ContainerReceipt{}, ErrUncertain
		}
		_, proof, err := s.approval.creation(*s.image, jit)
		if err != nil || proof.EnvDigest != state.created.EnvDigest || proof.LabelsDigest != state.created.LabelsDigest {
			return ContainerReceipt{}, ErrUncertain
		}
		return *state.created, nil
	}
	if state.pending != "" || state.uncertain || state.createInput != nil {
		return ContainerReceipt{}, ErrUncertain
	}
	image, err := s.preflight(check, pairCallRoom)
	if err != nil {
		return ContainerReceipt{}, err
	}
	payload, proof, err := s.approval.creation(image, jit)
	if err != nil {
		return ContainerReceipt{}, err
	}
	if err := s.guard(check); err != nil {
		return ContainerReceipt{}, err
	}
	if !s.journal.hasRoom(pairCallRoom) {
		return ContainerReceipt{}, ErrState
	}
	intent, err := s.store(pairedEvent{Create: &pairCreateEvent{Kind: "intent", Handoff: &h, EnvDigest: proof.EnvDigest, LabelsDigest: proof.LabelsDigest}})
	if err != nil {
		return ContainerReceipt{}, err
	}
	if err := s.guard(check); err != nil {
		return ContainerReceipt{}, err
	}
	if !s.journal.hasRoom(maxPairRecord) {
		return ContainerReceipt{}, ErrState
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	containerID, warnings, callErr := s.runtime.Create(ctx, s.approval.name(), payload)
	cancel()
	result := pairCreateEvent{Kind: "result", Intent: intent, ContainerID: containerID}
	if callErr != nil || warnings || !id.MatchString(containerID) {
		result.Kind = "unknown"
		if !id.MatchString(containerID) {
			result.ContainerID = ""
		}
	}
	if _, err := s.store(pairedEvent{Create: &result}); err != nil {
		return ContainerReceipt{}, err
	}
	if result.Kind == "unknown" {
		return ContainerReceipt{}, ErrUncertain
	}
	s.image = &image
	return *s.journal.pairState().created, nil
}

func (w *PairedWorker) Start(c ContainerReceipt, handoffResult RecordRef) (RecordRef, error) {
	s, err := w.enter("start")
	if err != nil {
		return RecordRef{}, err
	}
	defer s.op.Unlock()
	state := s.journal.pairState()
	if !s.original || !s.boundHere || state.created == nil || c != *state.created || !validRef(handoffResult) || handoffResult.Sequence <= c.HandoffIntent.Sequence {
		return RecordRef{}, ErrUncertain
	}
	check := ControllerCheck{Stage: CheckStart, PairSHA256: s.pairSHA, ControllerRecord: handoffResult, WorkerRecord: c.CreateResult}
	if err := s.guard(check); err != nil {
		return RecordRef{}, err
	}
	if state.started != (RecordRef{}) {
		if state.startInput.ControllerResult != handoffResult {
			return RecordRef{}, ErrUncertain
		}
		return state.started, nil
	}
	if state.pending != "" || state.uncertain || state.startInput != nil || state.deleted != nil {
		return RecordRef{}, ErrUncertain
	}
	if _, err := s.preflight(check, 2*pairCallRoom); err != nil {
		return RecordRef{}, err
	}
	local, _, err := s.observe(check)
	if err != nil {
		return RecordRef{}, err
	}
	if !startFacts(local) {
		return RecordRef{}, ErrUncertain
	}
	if err := s.guard(check); err != nil {
		return RecordRef{}, err
	}
	if !s.journal.hasRoom(pairCallRoom) {
		return RecordRef{}, ErrState
	}
	intent, err := s.store(pairedEvent{Start: &pairStartEvent{Kind: "intent", CreateResult: c.CreateResult, ControllerResult: handoffResult}})
	if err != nil {
		return RecordRef{}, err
	}
	if err := s.guard(check); err != nil {
		return RecordRef{}, err
	}
	if !s.journal.hasRoom(maxPairRecord) {
		return RecordRef{}, ErrState
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	callErr := s.runtime.Start(ctx, c.ContainerID)
	cancel()
	result := pairStartEvent{Kind: "result", Intent: intent}
	if callErr != nil {
		result.Kind = "unknown"
	}
	ref, err := s.store(pairedEvent{Start: &result})
	if err != nil {
		return RecordRef{}, err
	}
	if callErr != nil {
		return RecordRef{}, ErrUncertain
	}
	return ref, nil
}
