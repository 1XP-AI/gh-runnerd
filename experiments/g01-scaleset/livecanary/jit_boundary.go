package livecanary

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
	"github.com/actions/scaleset"
)

var errJITNotAttempted = errors.New("planned JIT no-forward boundary")
var errJITSuppressed = errors.New("planned JIT response suppression")

type jitBoundaryProbeKey struct{}
type jitBoundaryWireProbeKey struct{}
type jitBoundaryMutationKey struct{}
type jitBoundaryScope struct {
	ctx                                      context.Context
	deadline                                 time.Time
	driver                                   *Driver
	approval                                 Approval
	phase                                    string
	raw                                      []byte
	journal                                  *FileJournal
	identity                                 controllerJournalIdentity
	proof                                    *controllerHandoffProof
	api, network                             *SDKAPI
	captured, networkCaptured                SDKAPI
	outer                                    *http.Transport
	outerTransport                           jitBoundaryTransportIdentity
	originalTransport, dedicatedTransport    jitBoundaryTransportIdentity
	originalHTTPTransport                    http.RoundTripper
	originalRedirect, dedicatedRedirect      uintptr
	credentials                              Credentials
	setID, records                           int
	hosts                                    []string
	origin, tenant, management, registration string
	bootstrapStep                            int
	managementExpires                        time.Time
	invalid                                  atomic.Bool
}

func (d *Driver) runJITBoundary(ctx context.Context, phase string, proof *controllerHandoffProof) error {
	if ctx == nil || ctx.Err() != nil || proof == nil || d == nil || !jitBoundaryPhase(phase) {
		return ErrApproval
	}
	j, ok := d.Journal.(*FileJournal)
	api, aok := d.API.(*SDKAPI)
	raw, _ := ctx.Value(jitBoundaryRawKey{}).([]byte)
	if !ok || !aok || j == nil || api == nil || len(raw) == 0 || handoff.Digest(raw) != proof.RawApprovalSHA256 {
		return ErrApproval
	}
	_, release, err := authorizePhaseWithProof(d.Approval, j, phase, ctx, proof)
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
	if err = j.Append(Event{Kind: "phase", Operation: phase}); err != nil {
		return err
	}
	if err = jitBoundaryAdmission(d.Approval, j, proof.RawApprovalSHA256, phase, j.Events(), proof, bounded, true); err != nil {
		return err
	}
	id, err := j.controllerIdentity()
	if err != nil {
		return err
	}
	s := &jitBoundaryScope{ctx: bounded, deadline: deadline, driver: d, approval: d.Approval, phase: phase, raw: slices.Clone(raw), journal: j, identity: id, proof: proof, api: api, captured: *api, credentials: api.credentials, setID: j.Events()[6].ID, hosts: baselineWireAllowedHosts(d.Approval, api.drainEndpointHost())}
	s.approval.Phases = slices.Clone(s.approval.Phases)
	s.approval.ActionsHosts = slices.Clone(s.approval.ActionsHosts)
	if api.baseURL == "https://api.github.com" {
		s.hosts = slices.Clone(s.approval.ActionsHosts)
	}
	s.network, err = api.jitBoundaryAPI(s)
	if err != nil {
		return err
	}
	s.networkCaptured = *s.network
	s.originalTransport = jitBoundaryTransportSnapshot(s.api.physical)
	s.dedicatedTransport = jitBoundaryTransportSnapshot(s.network.physical)
	s.outerTransport = jitBoundaryTransportSnapshot(s.outer)
	s.originalHTTPTransport = s.api.rest.Transport
	s.originalRedirect = jitBoundaryFunction(s.api.rest.CheckRedirect)
	s.dedicatedRedirect = jitBoundaryFunction(s.network.rest.CheckRedirect)
	defer s.network.physical.CloseIdleConnections()
	defer clear(s.raw)
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
	if err = s.record(nil); err != nil {
		return err
	} // stable worker reservation, before SDK call
	p := s.wire("jit")
	if s.check() != nil {
		return ErrQuarantine
	}
	call, ccancel := context.WithTimeout(s.ctx, operationTimeout)
	dto, callErr := s.network.GenerateJIT(p.context(call), s.setID, s.approval.workerName())
	// TEST-only observation must run at the SDK return boundary, before any scrub.
	if probe, _ := s.ctx.Value(jitBoundaryProbeKey{}).(func(*scaleset.RunnerScaleSetJitRunnerConfig, error)); probe != nil {
		probe(dto, callErr)
	}
	ccancel()
	want := errJITNotAttempted
	if phase == "jit-loss" {
		want = errJITSuppressed
	}
	if dto != nil || !errors.Is(callErr, want) || !p.expected || s.check() != nil {
		return ErrQuarantine
	}
	if err = s.observe("worker-after"); err != nil {
		return err
	}
	return s.record(func(r *jitBoundaryRecord) {
		obs := "none"
		if phase == "jit-loss" {
			obs = "observed-one"
		}
		r.Summary = &jitBoundarySummary{true, true, false, obs}
	})
}
func (s *jitBoundaryScope) checkAuthority() error {
	now := time.Now()
	if s == nil || s.invalid.Load() || s.ctx.Err() != nil || !s.deadline.After(now) || !s.credentials.ExpiresAt.After(now) || (s.management != "" && !s.managementExpires.After(now.Add(time.Minute))) || !s.proof.validAt(s.ctx, s.approval, s.journal, now) || s.driver.Journal != s.journal || s.driver.API != s.api || approvalDigest(s.driver.Approval) != approvalDigest(s.approval) || approvalDigest(s.api.approval) != approvalDigest(s.approval) || s.api.client != s.captured.client || s.api.rest != s.captured.rest || s.api.physical != s.captured.physical || s.api.baseURL != s.captured.baseURL || s.api.credentials != s.credentials || handoff.Digest(s.raw) != s.proof.RawApprovalSHA256 {
		return ErrQuarantine
	}
	if s.network == nil || s.network.client != s.networkCaptured.client || s.network.rest != s.networkCaptured.rest || s.network.physical != s.networkCaptured.physical || s.network.baseURL != s.networkCaptured.baseURL || s.network.credentials != s.credentials || approvalDigest(s.network.approval) != approvalDigest(s.approval) || s.network.rest.Transport != s.outer || s.network.rest.Timeout != operationTimeout || s.network.rest.CheckRedirect == nil || !s.network.physical.DisableKeepAlives || s.network.physical.MaxConnsPerHost != 1 || s.network.physical.Proxy != nil || s.network.physical.Protocols == nil || !s.network.physical.Protocols.HTTP1() || s.network.physical.Protocols.HTTP2() {
		return ErrQuarantine
	}
	if !s.originalTransport.matches(s.api.physical) || !s.dedicatedTransport.matches(s.network.physical) || !s.outerTransport.matches(s.outer) || s.api.rest.Transport != s.originalHTTPTransport || jitBoundaryFunction(s.api.rest.CheckRedirect) != s.originalRedirect || jitBoundaryFunction(s.network.rest.CheckRedirect) != s.dedicatedRedirect || s.network.rest.Jar != nil {
		return ErrQuarantine
	}
	if guard, _ := s.ctx.Value(jitBoundaryGuardKey{}).(func() error); guard == nil || guard() != nil {
		return ErrApproval
	}
	now = time.Now()
	if s.ctx.Err() != nil || !s.deadline.After(now) || !s.proof.validAt(s.ctx, s.approval, s.journal, now) || !s.credentials.ExpiresAt.After(now) || s.management != "" && !s.managementExpires.After(now.Add(time.Minute)) {
		return ErrQuarantine
	}
	return nil
}
func (s *jitBoundaryScope) checkLocked() error {
	if err := s.checkAuthority(); err != nil {
		return err
	}
	j := s.journal
	id, err := j.controllerIdentity()
	if err != nil || id != s.identity || !j.authorityHeld(s.approval) || len(j.events) != 9+s.records || !j.jitBoundaryPhysicalLocked() || j.replayJITBoundary(j.events) != nil || j.jitBoundaryCapacityLocked() != nil {
		return ErrJournal
	}
	if _, err = jitBoundaryCreatePrefix(s.approval, j, s.proof.RawApprovalSHA256, s.phase, j.events[:7]); err != nil {
		return err
	}
	if !s.proof.matchesEvent(j.events[7]) || j.events[8].Kind != "phase" || j.events[8].Operation != s.phase {
		return ErrJournal
	}
	return nil
}
func (s *jitBoundaryScope) check() error {
	s.journal.mu.Lock()
	defer s.journal.mu.Unlock()
	return s.checkLocked()
}
func (s *jitBoundaryScope) recordLocked(fill func(*jitBoundaryRecord)) error {
	if s.checkLocked() != nil || s.records >= len(jitBoundarySteps(s.phase)) {
		return ErrQuarantine
	}
	r := s.journal.jitBoundaryBaseLocked(s.phase, s.records)
	if r.Stage == "start" || r.Stage == "jit" && r.Outcome == "intent" {
		v := true
		r.Retained = &v
	}
	if fill != nil {
		fill(&r)
	}
	s.journal.jitBoundaryWriting = true
	_, appendErr := s.journal.appendStored(Event{Kind: "jit-boundary", JITBoundary: &r})
	s.journal.jitBoundaryWriting = false
	if err := appendErr; err != nil {
		return err
	}
	s.records++
	return s.checkLocked() // includes post-fsync lifetime and physical bytes
}
func (s *jitBoundaryScope) record(fill func(*jitBoundaryRecord)) error {
	s.journal.mu.Lock()
	defer s.journal.mu.Unlock()
	return s.recordLocked(fill)
}
func (s *jitBoundaryScope) wire(stage string) *jitBoundaryPermit {
	c := &baselineWireCapture{stage: stage, setID: s.setID, organization: s.approval.Organization, owner: s.approval.setName(), runnerName: s.approval.workerName(), origin: s.origin, runtimePathPrefix: s.tenant, runtimePathPrefixSet: s.tenant != "", allowedHosts: s.hosts}
	p := &jitBoundaryPermit{scope: s, wire: c, atRecord: s.records}
	if probe, _ := s.ctx.Value(jitBoundaryWireProbeKey{}).(func(*jitBoundaryPermit)); probe != nil {
		probe(p)
	}
	return p
}
func (s *jitBoundaryScope) preflight() error {
	if err := s.record(nil); err != nil {
		return err
	}
	a := s.approval
	group := "/orgs/" + a.Organization + "/actions/runner-groups/" + strconv.Itoa(a.RunnerGroupID)
	paths := []string{group, group + "/repositories?per_page=100"}
	if a.ManagementScope == "" {
		paths = append(paths, "/installation/repositories?per_page=100", "/repos/"+a.Organization+"/"+a.Repository)
	}
	p := &jitBoundaryPermit{scope: s, exact: map[string]string{}, used: map[string]bool{}, atRecord: s.records}
	for _, path := range paths {
		p.exact["GET "+s.api.baseURL+path] = "Bearer " + s.credentials.InstallationToken
	}
	ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
	defer cancel()
	if s.network.Preflight(p.context(ctx), a) != nil || len(p.used) != len(p.exact) {
		return ErrQuarantine
	}
	return s.record(func(r *jitBoundaryRecord) { n := len(paths); r.PreflightReads = &n })
}
func (s *jitBoundaryScope) observe(stage string) error {
	if err := s.record(nil); err != nil {
		return err
	}
	wireStage := "runner-observe"
	if stage == "set-before" {
		wireStage = "set-observe"
	}
	p := s.wire(wireStage)
	if s.check() != nil {
		return ErrQuarantine
	}
	ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
	defer cancel()
	var set *baselineSetFacts
	var lookup *jitBoundaryLookup
	if stage == "set-before" {
		v, err := s.network.GetScaleSet(p.context(ctx), s.setID)
		if err != nil || !p.wire.observed() || p.wire.status != 200 || !jitBoundaryZeroSet(s.approval, s.setID, p.wire.set) || !p.wire.set.matches(v) {
			return ErrQuarantine
		}
		set = p.wire.set
	} else {
		v, err := s.network.FindRunner(p.context(ctx), s.approval.workerName())
		f := p.wire.runner
		if err != nil || !p.wire.observed() || p.wire.status != 200 || f == nil || !f.matches(v) || f.Count == 1 && (f.Name != s.approval.workerName() || f.ScaleSetID != s.setID) || (stage == "worker-before" || s.phase == beforeJITPhase) && f.Count != 0 {
			return ErrQuarantine
		}
		lookup = &jitBoundaryLookup{Count: f.Count}
		if f.Count == 1 {
			id := f.ID
			lookup.RunnerID = &id
		}
	}
	return s.record(func(r *jitBoundaryRecord) { status := 200; r.Status = &status; r.Set = set; r.Lookup = lookup })
}
