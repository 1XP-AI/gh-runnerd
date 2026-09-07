package livecanary

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

func pairInput(id controllerJournalIdentity, a Approval, setID int, creation controllerRecordRef) liveworker.PairInput {
	return liveworker.PairInput{Controller: workerIdentity(id), Source: liveworker.PairSource{Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, WorkflowRunID: a.WorkflowRunID, WorkflowPath: a.WorkflowPath, HeadSHA: a.WorkflowSHA, Attempt: 1}, RunnerGroupID: a.RunnerGroupID, ScaleSetID: setID, ScaleSetName: a.setName(), SetCreation: workerRef(creation), OwnerNonce: a.OwnerNonce, ControllerName: a.Controller, HarnessSHA: a.HarnessSHA}
}
func (s *baselineHistory) executionRecord(r baselineRecord, ref controllerRecordRef, lookup func(controllerRecordRef) *Event, id controllerJournalIdentity, a Approval) error {
	if s.collectionRef.Sequence != 0 {
		return ErrJournal
	}
	if r.Stage == "collection" {
		f := r.Collection
		if s.pending != nil || s.child != nil || f.Pair != s.pairRef || f.Start != s.startRef || f.Completed != s.completedRef || f.LastSample != s.lastSample || f.Rounds != s.rounds || f.OutstandingSession != s.outstanding() || f.SessionIntent != s.sessionIntent || f.SessionResult != s.sessionResult {
			return ErrJournal
		}
		if f.Outcome != collectionCollected && f.Outcome != collectionIncomplete && f.Outcome != collectionUnresolved {
			return ErrJournal
		}
		if s.uncertain && f.Outcome != collectionUnresolved || f.Outcome == collectionCollected && !s.collected() {
			return ErrJournal
		}
		s.collectionRef = ref
		return nil
	}
	if s.uncertain {
		return ErrJournal
	}
	child := r.Stage == "jit" || r.Stage == "handoff" || r.Stage == "worker-start" || r.Stage == "identity-sample" && r.Sample.Parent.Sequence != 0
	parent := controllerRecordRef{}
	if child {
		if s.pending == nil || s.pending.Stage != "continuation" {
			return ErrJournal
		}
		parent = s.pendingRef
	}
	if r.Outcome == "intent" {
		if child && s.child != nil || !child && s.pending != nil {
			return ErrJournal
		}
		switch r.Stage {
		case "pair":
			p := r.Pair
			if s.pair != nil || s.sessionID != "" || p.Receipt != nil || p.Binding.Version != 1 || p.Binding.Input != pairInput(id, a, s.setID, s.creation) || !validWorkerJournalIdentity(p.Binding.Worker) || r.SessionID != "" {
				return ErrJournal
			}
		case "host-preflight":
			if s.pairRef.Sequence == 0 || s.hostRef.Sequence != 0 || r.Host.Pair != s.pairRef || !validSHA256(r.Host.ApprovalSHA256) || !validSHA256(strings.TrimPrefix(r.Host.ImageID, "sha256:")) || len(r.Host.DaemonID) < 1 || len(r.Host.DaemonID) > 256 || len(r.Host.Image) > 512 || r.Host.Image == "" || r.SessionID != "" {
				return ErrJournal
			}
		case "roster-anchor":
			f := r.Roster
			old := lookup(f.Inventory)
			if s.hostRef.Sequence == 0 || s.rosterRef.Sequence != 0 || f.Pair != s.pairRef || f.Host != s.hostRef || f.Observation != nil || old == nil || old.Kind != "inventory" || old.Sequence >= s.creation.Sequence || !validSHA256(old.Digest) || r.SessionID != "" {
				return ErrJournal
			}
		case "jit":
			f := r.JIT
			if s.pairRef.Sequence == 0 || s.jitRef.Sequence != 0 || s.anchor == nil || f.Parent != parent || f.Pair != s.pairRef || f.Acquire != s.acquireRef || f.RequestID != s.anchor.RequestID || f.ExpectedName != a.workerName() || f.Runner != nil || r.HTTPStatus != 0 {
				return ErrJournal
			}
		case "handoff":
			f := r.Handoff
			if s.jitRef.Sequence == 0 || s.handoffRef.Sequence != 0 || f.Parent != parent || f.Container != nil || f.Input != s.expectedHandoff(controllerRecordRef{}) {
				return ErrJournal
			}
		case "worker-start":
			f := r.Start
			if s.handoffRef.Sequence == 0 || s.startRef.Sequence != 0 || f.Parent != parent || f.Handoff != s.handoffRef || f.Container != *s.handoff.Container || f.WorkerResult != (liveworker.RecordRef{}) {
				return ErrJournal
			}
		case "identity-sample":
			f := r.Sample
			if s.startRef.Sequence == 0 || f.Round != s.rounds+1 || f.Round > 8 || f.Start != s.startRef || f.Previous != s.lastSample || f.SDK != nil || f.Job != nil || f.REST != nil || f.Local != nil || f.RESTAddressable {
				return ErrJournal
			}
			if f.Round <= 4 {
				if !child || f.Parent != parent || f.Completed.Sequence != 0 {
					return ErrJournal
				}
			} else if child || !s.complete || !s.continued || f.Parent.Sequence != 0 || f.Completed != s.completedRef {
				return ErrJournal
			}
		default:
			return ErrJournal
		}
		if child && r.SessionID != s.sessionID || r.Stage == "identity-sample" && r.SessionID != s.sessionID {
			return ErrJournal
		}
		if child {
			s.child = &r
			s.childRef = ref
		} else {
			s.pending = &r
			s.pendingRef = ref
		}
		return nil
	}
	pending, pendingRef := s.pending, s.pendingRef
	if child {
		pending, pendingRef = s.child, s.childRef
	}
	if pending == nil || r.Intent != pendingRef || r.Stage != pending.Stage || r.SessionID != pending.SessionID {
		return ErrJournal
	}
	// Compare the exact intent after removing only the permitted response slots.
	p := r
	p.Outcome = "intent"
	p.Intent = controllerRecordRef{}
	switch r.Stage {
	case "pair":
		v := *r.Pair
		v.Receipt = nil
		p.Pair = &v
	case "roster-anchor":
		v := *r.Roster
		v.Observation = nil
		p.Roster = &v
	case "jit":
		v := *r.JIT
		v.Runner = nil
		p.JIT = &v
		p.HTTPStatus = 0
	case "handoff":
		v := *r.Handoff
		v.Container = nil
		v.Input.HandoffIntent = liveworker.RecordRef{}
		p.Handoff = &v
	case "worker-start":
		v := *r.Start
		v.WorkerResult = liveworker.RecordRef{}
		p.Start = &v
	case "identity-sample":
		v := *r.Sample
		v.SDK = nil
		v.Job = nil
		v.REST = nil
		v.Local = nil
		v.RESTAddressable = false
		p.Sample = &v
	}
	if !reflect.DeepEqual(p, *pending) {
		return ErrJournal
	}
	if child {
		s.child = nil
		s.childRef = controllerRecordRef{}
	} else {
		s.pending = nil
	}
	if r.Stage == "roster-anchor" && (r.Roster.Observation == nil || !validRosterObservation(*r.Roster.Observation, a.Organization)) {
		return ErrJournal
	}
	if r.Outcome == "unknown" {
		s.uncertain = true
		return nil
	}
	switch r.Stage {
	case "pair":
		p := r.Pair
		if p.Receipt == nil || p.Receipt.ControllerIntent != workerRef(r.Intent) || p.Receipt.PairSHA256 != pairDigest(p.Binding) || !refPresent(controllerRef(p.Receipt.WorkerBound)) {
			return ErrJournal
		}
		s.pair = p
		s.pairIntent = r.Intent
		s.pairRef = ref
	case "host-preflight":
		s.hostRef = ref
	case "roster-anchor":
		obs := r.Roster.Observation
		old := lookup(r.Roster.Inventory)
		if obs.Completeness != rosterComplete || obs.Digest != old.Digest {
			return ErrJournal
		}
		s.rosterRef = ref
	case "jit":
		f := r.JIT
		if r.HTTPStatus != 200 || f.Runner == nil || f.Runner.ID <= 0 || f.Runner.Name != a.workerName() || f.Runner.ScaleSetID != s.setID {
			return ErrJournal
		}
		s.jit = f
		s.jitRef = ref
	case "handoff":
		c := r.Handoff.Container
		if r.Handoff.Input != s.expectedHandoff(r.Intent) {
			return ErrJournal
		}
		if c == nil || c.PairSHA256 != s.pair.Receipt.PairSHA256 || c.HandoffIntent != workerRef(r.Intent) || !refPresent(controllerRef(c.CreateIntent)) || !refPresent(controllerRef(c.CreateResult)) || c.CreateResult.Sequence <= c.CreateIntent.Sequence || !validSHA256(c.ContainerID) || !validSHA256(c.EnvDigest) || !validSHA256(c.LabelsDigest) {
			return ErrJournal
		}
		s.handoff = r.Handoff
		s.handoffRef = ref
	case "worker-start":
		if !refPresent(controllerRef(r.Start.WorkerResult)) || r.Start.WorkerResult.Sequence <= r.Start.Container.CreateResult.Sequence {
			return ErrJournal
		}
		s.start = r.Start
		s.startRef = ref
	case "identity-sample":
		if !s.acceptSample(r.Sample, a) {
			return ErrJournal
		}
		s.rounds = r.Sample.Round
		s.lastSample = ref
	default:
		return ErrJournal
	}
	return nil
}
func (s *baselineHistory) expectedHandoff(intent controllerRecordRef) liveworker.HandoffReceipt {
	return liveworker.HandoffReceipt{PairSHA256: s.pair.Receipt.PairSHA256, PairResult: workerRef(s.pairRef), AcquireResult: workerRef(s.acquireRef), JITResult: workerRef(s.jitRef), HandoffIntent: workerRef(intent), RequestID: s.anchor.RequestID, Runner: *s.jit.Runner}
}
func validWorkerJournalIdentity(i liveworker.JournalIdentity) bool {
	return validSHA256(i.OwnershipSHA256) && i.State.Inode != 0 && i.Journal.Inode != 0 && i.AdmissionDirectory.Inode != 0 && i.Claim.Inode != 0
}
func validRosterObservation(o rosterObservation, org string) bool {
	if o.Organization != org || o.Pages < 0 || o.Pages > 10 {
		return false
	}
	if o.Completeness == rosterUnresolved {
		return o.Count == nil && o.Digest == ""
	}
	if o.Completeness != rosterComplete || o.Count == nil || o.Pages == 0 || *o.Count < 0 || *o.Count > 1000 || !validSHA256(o.Digest) {
		return false
	}
	if *o.Count == 0 {
		sum := sha256.Sum256([]byte("null"))
		return o.Pages == 1 && o.Digest == hex.EncodeToString(sum[:])
	}
	return *o.Count >= o.Pages && *o.Count <= 100*o.Pages
}
func (s *baselineHistory) outstanding() sessionOutstanding {
	if s.sessionID != "" {
		return sessionKnownOpen
	}
	if s.sessionIntent.Sequence != 0 {
		return sessionOpenUnknown
	}
	return sessionNone
}
func (s *baselineHistory) collected() bool {
	return !s.uncertain && s.rounds == 8 && s.complete && s.continued && s.jit != nil && s.jit.Runner != nil && s.runnerID == sdkRunnerID(s.jit.Runner.ID) && s.runnerName == s.jit.Runner.Name && s.lastSDK != nil && s.lastJob != nil && s.lastREST != nil && s.lastLocal != nil
}
