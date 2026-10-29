package livecanary

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/google/uuid"
)

var errReplacementStopped = errors.New("replacement listener stopped")

type replacementApprovalGuardKey struct{}
type replacementRawSnapshotKey struct{}
type replacementProbeKey struct{}
type replacementWireProbeKey struct{}

type replacementScope struct {
	ctx                                      context.Context
	deadline                                 time.Time
	driver                                   *Driver
	approval                                 Approval
	raw                                      []byte
	journal                                  *FileJournal
	identity                                 controllerJournalIdentity
	proof                                    *controllerHandoffProof
	api                                      *SDKAPI
	network                                  *SDKAPI
	captured                                 SDKAPI
	credentials                              Credentials
	setID, records                           int
	hosts                                    []string
	origin, tenant, management, registration string // private, never persisted
	bootstrapStep                            int
	managementExpires                        time.Time
	objects                                  [3]*scaleset.MessageSessionClient
	snapshots                                [3]scaleset.RunnerScaleSetSession
	callbacks                                atomic.Int32
	retired                                  atomic.Bool
	invalid                                  atomic.Bool
	release                                  chan struct{}
	releaseOnce                              sync.Once
	held                                     chan error
	oldDone                                  chan error
}

func (d *Driver) runSessionReplacement(ctx context.Context, proof *controllerHandoffProof) error {
	if ctx == nil || proof == nil || d == nil {
		return ErrApproval
	}
	j, ok := d.Journal.(*FileJournal)
	api, aok := d.API.(*SDKAPI)
	raw, _ := ctx.Value(replacementRawSnapshotKey{}).([]byte)
	if !ok || !aok || j == nil || api == nil || len(raw) == 0 || handoff.Digest(raw) != proof.RawApprovalSHA256 {
		return ErrApproval
	}
	_, release, err := authorizePhaseWithProof(d.Approval, j, replacementPhase, ctx, proof)
	if err != nil {
		return err
	}
	defer release()
	deadline := time.Now().Add(10 * time.Minute)
	for _, limit := range []time.Time{d.Approval.ExpiresAt, proof.EffectiveDeadline, api.credentials.ExpiresAt} {
		if limit.Before(deadline) {
			deadline = limit
		}
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	stop := context.AfterFunc(proof.parent, cancel)
	defer stop()
	if err = j.Append(Event{Kind: "phase", Operation: replacementPhase}); err != nil {
		return err
	}
	if err = replacementAdmission(d.Approval, j, proof.RawApprovalSHA256, j.Events(), proof, bounded, true); err != nil {
		return err
	}
	id, err := j.controllerIdentity()
	if err != nil {
		return err
	}
	s := &replacementScope{ctx: bounded, deadline: deadline, driver: d, approval: d.Approval, raw: slices.Clone(raw), journal: j, identity: id, proof: proof, api: api, captured: *api, credentials: api.credentials, setID: j.Events()[6].ID, hosts: baselineWireAllowedHosts(d.Approval, api.drainEndpointHost()), release: make(chan struct{}), held: make(chan error, 1), oldDone: make(chan error, 1)}
	s.approval.Phases = slices.Clone(s.approval.Phases)
	s.approval.ActionsHosts = slices.Clone(s.approval.ActionsHosts)
	if api.baseURL == "https://api.github.com" {
		s.hosts = slices.Clone(s.approval.ActionsHosts)
	}
	s.network, err = api.replacementAPI(s)
	if err != nil {
		return err
	}
	defer s.network.physical.CloseIdleConnections()
	defer clear(s.raw)
	// Every exit only releases local delivery barriers and joins. There is no
	// deferred network close and no detached cleanup authority.
	oldStarted := false
	defer func() {
		s.retired.Store(true)
		s.releaseOnce.Do(func() { close(s.release) })
		if oldStarted {
			<-s.oldDone
		}
	}()
	if err = s.record(nil); err != nil {
		return err
	}
	if err = s.preflight(); err != nil {
		return err
	}
	if err = s.observe("set-before"); err != nil {
		return err
	}
	if err = s.observe("worker-before"); err != nil {
		return err
	}
	if err = s.open(1); err != nil {
		return err
	}
	oldStarted = true
	go func() { s.oldDone <- s.listen(1) }()
	select {
	case err = <-s.held:
		if err != nil {
			return err
		}
	case err = <-s.oldDone:
		oldStarted = false
		return err
	case <-bounded.Done():
		return ErrQuarantine
	}
	if err = s.record(nil); err != nil {
		return err
	} // durable retirement precedes local retirement and Close
	s.retired.Store(true)
	if err = s.close(1); err != nil {
		return err
	}
	if err = s.open(2); err != nil {
		return err
	}
	if err = s.listen(2); !errors.Is(err, errReplacementStopped) {
		return ErrQuarantine
	}
	s.releaseOnce.Do(func() { close(s.release) })
	err = <-s.oldDone
	oldStarted = false
	if !errors.Is(err, errReplacementStopped) || s.callbacks.Load() != 0 {
		return ErrQuarantine
	}
	if err = s.record(nil); err != nil {
		return err
	} // discard, after the return-generation fence and join
	if err = s.observe("set-after"); err != nil {
		return err
	}
	if err = s.observe("worker-after"); err != nil {
		return err
	}
	if err = s.close(2); err != nil {
		return err
	}
	return s.record(nil)
}

func (s *replacementScope) check() error {
	if s.invalid.Load() || s.ctx.Err() != nil || !s.deadline.After(time.Now()) || !s.credentials.ExpiresAt.After(time.Now()) || (s.management != "" && !s.managementExpires.After(time.Now())) || !s.proof.validAt(s.ctx, s.approval, s.journal, time.Now()) || s.driver.Journal != s.journal || s.driver.API != s.api || approvalDigest(s.driver.Approval) != approvalDigest(s.approval) || approvalDigest(s.api.approval) != approvalDigest(s.approval) || s.api.client != s.captured.client || s.api.rest != s.captured.rest || s.api.physical != s.captured.physical || s.api.baseURL != s.captured.baseURL || s.api.credentials != s.credentials || handoff.Digest(s.raw) != s.proof.RawApprovalSHA256 {
		return ErrQuarantine
	}
	if guard, _ := s.ctx.Value(replacementApprovalGuardKey{}).(func() error); guard != nil && guard() != nil {
		return ErrApproval
	}
	s.journal.mu.Lock()
	defer s.journal.mu.Unlock()
	id, err := s.journal.controllerIdentity()
	if err != nil || id != s.identity || !s.journal.authorityHeld(s.approval) || len(s.journal.events) != 9+s.records || !s.journal.replacementPhysicalHistoryLocked() || s.journal.replayReplacement(s.journal.events) != nil || s.journal.replacementCapacityLocked() != nil {
		return ErrJournal
	}
	return nil
}

func (s *replacementScope) record(fill func(*replacementRecord)) error {
	if s.check() != nil || s.records >= len(replacementSteps) {
		return ErrQuarantine
	}
	j := s.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.events) != 9+s.records || !j.authorityHeld(s.approval) || s.ctx.Err() != nil {
		return ErrJournal
	}
	step := replacementSteps[s.records]
	r := replacementRecord{Version: 1, Stage: step.stage, Outcome: step.outcome, Generation: step.generation, SetID: s.setID, Owner: s.approval.OwnerNonce, RawApproval: s.proof.RawApprovalSHA256, CanonicalApproval: approvalDigest(s.approval), CreatePrefixSHA256: j.replacementPrefixDigestLocked(), Identity: s.identity, Creation: j.replacementRefLocked(6), CreateConsumption: j.replacementRefLocked(0), Consumption: j.replacementRefLocked(7), Phase: j.replacementRefLocked(8), Previous: j.replacementRefLocked(len(j.events) - 1)}
	if step.generation > 0 {
		r.SessionID = s.snapshots[step.generation].SessionID.String()
		if s.snapshots[step.generation].SessionID == uuid.Nil {
			r.SessionID = ""
		}
	}
	if step.stage == "poll" && step.outcome == "intent" {
		zero := 0
		r.Cursor = &zero
	}
	if step.stage == "complete" {
		requests := map[int64]bool{}
		for _, e := range j.events {
			if e.Replacement != nil && e.Replacement.Work != nil {
				requests[e.Replacement.Work.RequestID] = true
			}
		}
		ids := make([]int64, 0, len(requests))
		for id := range requests {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, k int) bool { return ids[i] < ids[k] })
		callbacks := int(s.callbacks.Load())
		r.Summary = &replacementSummary{[2]string{s.snapshots[1].SessionID.String(), s.snapshots[2].SessionID.String()}, ids, true, &callbacks}
	}
	if fill != nil {
		fill(&r)
	}
	if _, err := j.appendStored(Event{Kind: "session-replacement", Replacement: &r}); err != nil {
		return err
	}
	s.records++
	if !s.proof.validAt(s.ctx, s.approval, j, time.Now()) {
		return ErrQuarantine
	}
	return nil
}

func (s *replacementScope) wire(stage string, g int) *replacementPermit {
	c := &baselineWireCapture{stage: stage, setID: s.setID, organization: s.approval.Organization, owner: s.approval.setName(), runnerName: s.approval.workerName(), origin: s.origin, runtimePathPrefix: s.tenant, runtimePathPrefixSet: s.tenant != "", allowedHosts: baselineWireAllowedHosts(s.approval, s.api.drainEndpointHost())}
	if g > 0 {
		v := s.snapshots[g]
		c.sessionID = v.SessionID.String()
		c.queue = v.MessageQueueURL
		c.authorization = v.MessageQueueAccessToken
		if stage == "terminal-session-close" {
			c.authorization = s.management
		}
	}
	p := &replacementPermit{scope: s, wire: c, atRecord: s.records, generation: g}
	if probe, _ := s.ctx.Value(replacementWireProbeKey{}).(func(*replacementPermit)); probe != nil {
		probe(p)
	}
	return p
}

func (s *replacementScope) preflight() error {
	if s.check() != nil {
		return ErrQuarantine
	}
	a := s.approval
	group := "/orgs/" + a.Organization + "/actions/runner-groups/" + strconv.Itoa(a.RunnerGroupID)
	paths := []string{group, group + "/repositories?per_page=100"}
	if a.ManagementScope == "" {
		paths = append(paths, "/installation/repositories?per_page=100", "/repos/"+a.Organization+"/"+a.Repository)
	}
	p := &replacementPermit{scope: s, exact: map[string]string{}, used: map[string]bool{}, atRecord: s.records, operation: "preflight"}
	for _, path := range paths {
		p.exact["GET "+s.api.baseURL+path] = "Bearer " + s.credentials.InstallationToken
	}
	ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
	defer cancel()
	if s.network.Preflight(p.context(ctx), a) != nil {
		return ErrApproval
	}
	if len(p.used) != len(p.exact) {
		return ErrQuarantine
	}
	return nil
}

func (s *replacementScope) setEligible(v *baselineSetFacts) bool {
	return v != nil && v.matchesOwner(s.approval, s.setID) && len(v.Labels) == 1 && v.Labels[0].Name == s.approval.setName() && v.Labels[0].Type == "System" && replacementIdle(v.Statistics)
}
func (s *replacementScope) observe(stage string) error {
	if err := s.record(nil); err != nil {
		return err
	}
	wireStage := "set-observe"
	if stage == "worker-before" || stage == "worker-after" {
		wireStage = "runner-observe"
	}
	p := s.wire(wireStage, 0)
	ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
	var stats *baselineStatistics
	known := false
	if wireStage == "set-observe" {
		v, err := s.network.GetScaleSet(p.context(ctx), s.setID)
		known = err == nil && p.wire.observed() && p.wire.status == 200 && s.setEligible(p.wire.set) && p.wire.set.matches(v)
		if known {
			stats = p.wire.set.Statistics
		}
	} else {
		v, err := s.network.FindRunner(p.context(ctx), s.approval.workerName())
		known = err == nil && p.wire.observed() && p.wire.status == 200 && p.wire.runner != nil && p.wire.runner.Count == 0 && p.wire.runner.matches(v)
	}
	cancel()
	if !known {
		return ErrQuarantine
	}
	return s.record(func(r *replacementRecord) { r.Status = 200; r.Statistics = stats })
}

func (s *replacementScope) open(g int) error {
	if err := s.record(nil); err != nil {
		return err
	}
	p := s.wire("session-open", g)
	ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
	obj, err := s.network.client.MessageSessionClient(p.context(ctx), s.setID, s.approval.setName(), s.network.options...)
	cancel()
	if err != nil || obj == nil || !p.wire.observed() || p.wire.status != 200 || p.nested == nil {
		return ErrQuarantine
	}
	v := obj.Session()
	sf := p.wire.session
	u, e := url.Parse(v.MessageQueueURL)
	if sf == nil || !sf.NestedSet || !sf.eligible(s.approval, s.setID) || !replacementIdle(sf.Statistics) || !replacementIdle(sf.NestedStatistics) || v.RunnerScaleSet == nil || !p.nested.matches(v.RunnerScaleSet) || sf.SessionID != v.SessionID.String() || sf.Owner != v.OwnerName || !sf.Statistics.matches(v.Statistics) || !sf.NestedStatistics.matches(v.RunnerScaleSet.Statistics) || v.SessionID == uuid.Nil || (g == 2 && v.SessionID == s.snapshots[1].SessionID) || e != nil || u.RawQuery != "" || u.User != nil || u.Fragment != "" || u.EscapedPath() != u.Path || !baselineOriginAllowed(u, s.hosts) || v.MessageQueueAccessToken == "" || sf.queueURL != v.MessageQueueURL || sf.authorization != v.MessageQueueAccessToken {
		return ErrQuarantine
	}
	origin, ok := baselineRequestOrigin(u)
	if !ok || origin != s.origin || !validDrainAuthorizationToken(v.MessageQueueAccessToken) {
		return ErrQuarantine
	}
	s.objects[g] = obj
	data, _ := json.Marshal(v)
	defer clear(data)
	if json.Unmarshal(data, &s.snapshots[g]) != nil {
		return ErrQuarantine
	}
	return s.record(func(r *replacementRecord) {
		r.SessionID = v.SessionID.String()
		r.Status = 200
		r.Statistics = sf.Statistics
	})
}

func (s *replacementScope) same(g int) bool {
	return s.objects[g] != nil && reflect.DeepEqual(s.objects[g].Session(), s.snapshots[g])
}
func (s *replacementScope) close(g int) error {
	if !s.same(g) || (g == 1 && !s.retired.Load()) || (g == 2 && s.callbacks.Load() != 0) {
		return ErrQuarantine
	}
	if err := s.record(nil); err != nil {
		return err
	}
	p := s.wire("terminal-session-close", g)
	ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
	err := s.objects[g].Close(p.context(ctx))
	cancel()
	if err != nil || !s.same(g) || !p.wire.observed() || p.wire.status != 204 {
		return ErrQuarantine
	}
	return s.record(func(r *replacementRecord) { r.Status = 204 })
}

type replacementListener struct {
	scope      *replacementScope
	generation int
	polled     bool
}

func (l *replacementListener) Session() scaleset.RunnerScaleSetSession {
	return l.scope.snapshots[l.generation]
}
func (l *replacementListener) Scale(ctx context.Context, m *scaleset.RunnerScaleSetMessage) error {
	if m != nil && m.MessageID == listener.InitialMessageID && !l.polled && l.scope.same(l.generation) && reflect.DeepEqual(m.Statistics, l.Session().Statistics) {
		return nil
	}
	l.scope.callbacks.Add(1)
	if probe, _ := ctx.Value(replacementProbeKey{}).(func(string, int)); probe != nil {
		probe("callback", l.generation)
	}
	return ErrQuarantine
}
func (l *replacementListener) DeleteMessage(context.Context, int) error { return ErrQuarantine }
func (l *replacementListener) AcquireJobs(context.Context, []int64) ([]int64, error) {
	return nil, ErrQuarantine
}
func (l *replacementListener) GetMessage(_ context.Context, last, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	s := l.scope
	g := l.generation
	if probe, _ := s.ctx.Value(replacementProbeKey{}).(func(string, int)); probe != nil && g == 2 {
		probe("cursor2", last)
	}
	fail := func(err error) (*scaleset.RunnerScaleSetMessage, error) {
		if g == 1 {
			s.held <- err
		}
		return nil, err
	}
	if l.polled || last != 0 || capacity != 1 || !s.same(g) {
		return fail(ErrQuarantine)
	}
	l.polled = true
	if err := s.record(nil); err != nil {
		return fail(err)
	}
	p := s.wire("poll", g)
	p.wire.cursor = last
	ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
	m, err := s.objects[g].GetMessage(p.context(ctx), last, capacity)
	cancel()
	if err != nil || !p.wire.observed() || !s.same(g) {
		return fail(ErrQuarantine)
	}
	status := p.wire.status
	b := p.wire.batch
	var work *replacementWork
	var source *baselineSource
	if status == 200 {
		if b == nil || !b.matches(m) || !replacementControlledBatch(b, s.approval) {
			return fail(ErrQuarantine)
		}
		vctx, vcancel, e := s.network.observationContext(s.ctx, true)
		if e != nil {
			return fail(ErrApproval)
		}
		auth := &replacementPermit{scope: s, exact: map[string]string{"GET " + s.api.baseURL + "/repos/" + s.approval.Organization + "/" + s.approval.Repository + "/actions/runs/" + strconv.FormatInt(s.approval.WorkflowRunID, 10): "Bearer " + s.credentials.VerificationToken}, used: map[string]bool{}, atRecord: s.records, operation: "source", generation: g}
		response, e := s.network.observeApprovedRun(auth.context(vctx))
		vcancel()
		if e != nil || response.Status != 200 || len(auth.used) != 1 {
			return fail(ErrApproval)
		}
		work = &replacementWork{b.Items[0].RequestID, b.Items[0].JobID}
		source = baselineApprovedSource(s.approval)
	} else if status != 202 || g != 2 || m != nil {
		return fail(ErrQuarantine)
	}
	if err = s.record(func(r *replacementRecord) {
		r.Status = status
		if status == 200 {
			id := b.MessageID
			r.MessageID = &id
			r.Work = work
			r.Statistics = b.Statistics
			r.VerifiedSource = source
		}
	}); err != nil {
		return fail(err)
	}
	if probe, _ := s.ctx.Value(replacementProbeKey{}).(func(string, int)); probe != nil {
		probe("poll", g)
	}
	if g == 2 {
		return nil, errReplacementStopped
	}
	s.held <- nil
	select {
	case <-s.release:
	case <-s.ctx.Done():
		return nil, errReplacementStopped
	}
	// Return-generation fence: this check must precede listener Scale, including
	// callbacks that would themselves refuse all downstream effects.
	if s.retired.Load() {
		return nil, errReplacementStopped
	}
	return m, nil
}

func replacementControlledBatch(b *baselineBatch, a Approval) bool {
	if b == nil || b.MessageID < 0 || len(b.Items) != 1 || !replacementIdle(b.Statistics) {
		return false
	}
	x := b.Items[0]
	return x.Kind == "JobAvailable" && x.Index == 0 && x.RequestID > 0 && x.JobID != "" && x.Owner != nil && *x.Owner == a.Organization && x.Repository != nil && *x.Repository == a.Repository && x.RunID != nil && *x.RunID == a.WorkflowRunID && x.Event != nil && *x.Event == "workflow_dispatch" && x.WorkflowRef != nil && *x.WorkflowRef == a.WorkflowRef && slices.Equal(x.Labels, []string{a.setName()}) && x.RunnerID == nil && x.RunnerName == nil && x.Result == nil
}
func (s *replacementScope) listen(g int) error {
	client := &replacementListener{scope: s, generation: g}
	l, err := listener.New(client, listener.Config{ScaleSetID: s.setID, MaxRunners: 1})
	if err != nil {
		return ErrQuarantine
	}
	err = l.Run(s.ctx, client)
	if errors.Is(err, errReplacementStopped) {
		return errReplacementStopped
	}
	return ErrQuarantine
}
