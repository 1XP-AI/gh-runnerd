package livecanary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
	"syscall"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
	"github.com/google/uuid"
)

const baselineRecordLimit = 16 << 10
const baselineJournalLimit = 1 << 20

// Field order/tags and domain match the approved worker pair contract. These
// private controller types require no dependency on unfinished worker code.
type controllerRecordRef struct {
	Sequence    int    `json:"sequence"`
	EventSHA256 string `json:"event_sha256"`
}
type controllerFileIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}
type controllerJournalIdentity struct {
	OwnershipSHA256    string                 `json:"ownership_sha256"`
	State              controllerFileIdentity `json:"state"`
	Journal            controllerFileIdentity `json:"journal"`
	AdmissionDirectory controllerFileIdentity `json:"admission_directory"`
	Claim              controllerFileIdentity `json:"claim"`
}
type baselineSource struct {
	Organization string `json:"organization"`
	Repository   string `json:"repository"`
	RepositoryID int64  `json:"repository_id"`
	RunID        int64  `json:"run_id"`
	Path         string `json:"path"`
	HeadSHA      string `json:"head_sha"`
	Event        string `json:"event"`
	Attempt      int    `json:"attempt"`
}
type baselineRecord struct {
	Terminal   *baselineTerminalPayload `json:"terminal,omitempty"`
	Pair       *baselinePair            `json:"pair,omitempty"`
	Host       *baselineHost            `json:"host,omitempty"`
	Roster     *baselineRoster          `json:"roster,omitempty"`
	JIT        *baselineJIT             `json:"jit,omitempty"`
	Handoff    *baselineHandoff         `json:"handoff,omitempty"`
	Start      *baselineStart           `json:"start,omitempty"`
	Sample     *baselineSample          `json:"sample,omitempty"`
	Collection *baselineCollectionFacts `json:"collection,omitempty"`

	Version    int                   `json:"version"`
	Stage      string                `json:"stage"`
	Outcome    string                `json:"outcome"`
	SetID      int                   `json:"set_id"`
	SessionID  string                `json:"session_id,omitempty"`
	Cursor     int                   `json:"cursor,omitempty"`
	MessageID  int                   `json:"message_id,omitempty"`
	Intent     controllerRecordRef   `json:"intent"`
	Creation   controllerRecordRef   `json:"creation"`
	BatchRef   controllerRecordRef   `json:"batch_ref"`
	SourceRef  controllerRecordRef   `json:"source_ref"`
	ACKRef     controllerRecordRef   `json:"ack_ref"`
	AcquireRef controllerRecordRef   `json:"acquire_ref"`
	ItemIndex  *int                  `json:"item_index,omitempty"`
	HTTPStatus int                   `json:"http_status,omitempty"`
	NoMessage  bool                  `json:"no_message,omitempty"`
	Desired    *int                  `json:"desired,omitempty"`
	Set        *baselineSetFacts     `json:"set,omitempty"`
	Session    *baselineSessionFacts `json:"session,omitempty"`
	Batch      *baselineBatch        `json:"batch,omitempty"`
	Source     *baselineSource       `json:"source,omitempty"`
	Accepted   *baselineAccepted     `json:"accepted,omitempty"`
}
type baselineAcquisition struct {
	SessionID      string              `json:"session_id"`
	MessageID      int                 `json:"message_id"`
	RequestID      int64               `json:"request_id"`
	JobID          string              `json:"job_id"`
	AvailableIndex int                 `json:"available_index"`
	WholeBatch     controllerRecordRef `json:"whole_batch"`
	SourceCheck    controllerRecordRef `json:"source_check"`
	ACK            controllerRecordRef `json:"ack"`
	Intent         controllerRecordRef `json:"intent"`
	Result         controllerRecordRef `json:"result"`
}

func baselineApprovedSource(a Approval) *baselineSource {
	return &baselineSource{a.Organization, a.Repository, a.RepositoryID, a.WorkflowRunID, a.WorkflowPath, a.WorkflowSHA, "workflow_dispatch", 1}
}
func (j *FileJournal) controllerIdentity() (controllerJournalIdentity, error) {
	if j.claim == nil || !j.claim.matches(j) || !j.ownsCurrentJournal() {
		return controllerJournalIdentity{}, ErrJournal
	}
	identity := func(s *syscall.Stat_t) controllerFileIdentity { return controllerFileIdentity{uint64(s.Dev), s.Ino} }
	return controllerJournalIdentity{j.ownership, identity(j.directoryInfo.Sys().(*syscall.Stat_t)), identity(j.fileInfo.Sys().(*syscall.Stat_t)), identity(j.claim.rootInfo.Sys().(*syscall.Stat_t)), identity(j.claim.fileInfo.Sys().(*syscall.Stat_t))}, nil
}
func controllerEventRef(identity controllerJournalIdentity, e Event) controllerRecordRef {
	i, _ := json.Marshal(identity)
	data, _ := json.Marshal(e)
	h := sha256.New()
	_, _ = h.Write([]byte("gh-runnerd/g01-pair/controller-event/v1\x00"))
	_, _ = h.Write(i)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(data)
	return controllerRecordRef{e.Sequence, hex.EncodeToString(h.Sum(nil))}
}
func cloneEvent(e Event) Event {
	if e.Authority != nil {
		x := *e.Authority
		x.Phases = slices.Clone(x.Phases)
		e.Authority = &x
	}
	e.RequestIDs = slices.Clone(e.RequestIDs)
	if e.Baseline != nil {
		data, _ := json.Marshal(e.Baseline)
		var x baselineRecord
		_ = json.Unmarshal(data, &x)
		e.Baseline = &x
	}
	return e
}
func (j *FileJournal) appendRecord(e Event) (controllerRecordRef, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	id, err := j.controllerIdentity()
	if err != nil {
		return controllerRecordRef{}, err
	}
	stored, err := j.appendStored(e)
	if err != nil {
		return controllerRecordRef{}, err
	}
	return controllerEventRef(id, stored), nil
}

// appendStored is called under mu. It also supports authority renewal before
// the admission claim is opened; that path returns no domain-bound reference.
func (j *FileJournal) appendStored(e Event) (Event, error) {
	if j.writeFailed || !validEvent(e) {
		return Event{}, ErrJournal
	}
	if e.Baseline == nil && e.Kind != "authority" && slices.ContainsFunc(j.events, func(old Event) bool { return old.Baseline != nil }) {
		return Event{}, ErrJournal
	}
	e = cloneEvent(e)
	e.Sequence = len(j.events) + 1
	if e.Baseline != nil {
		id, err := j.controllerIdentity()
		if err != nil {
			return Event{}, err
		}
		if _, err = replayBaseline(append(slices.Clone(j.events), e), id, j.approval); err != nil {
			return Event{}, err
		}
	}
	if err := j.write(e); err != nil {
		j.writeFailed = true
		return Event{}, err
	}
	j.events = append(j.events, e)
	return cloneEvent(e), nil
}
func (j *FileJournal) baselineCapacity() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	s, err := j.file.Stat()
	if err != nil || j.writeFailed || s.Size()+2*baselineRecordLimit > baselineJournalLimit {
		return ErrJournal
	}
	return nil
}
func validBaselineShape(e Event) bool {
	r := e.Baseline
	if r == nil || r.Version != 1 || r.SetID <= 0 {
		return false
	}
	x := e
	x.Baseline = nil
	if !reflect.DeepEqual(x, Event{Sequence: e.Sequence, Kind: "baseline"}) {
		return false
	}
	data, err := json.Marshal(e)
	if err != nil || len(data)+1 > baselineRecordLimit {
		return false
	}
	if r.Outcome != "intent" && r.Outcome != "result" && r.Outcome != "unknown" && r.Outcome != "observed" {
		return false
	}
	if terminalStage(r.Stage) {
		return terminalShape(*r)
	}
	if executionStage(r.Stage) {
		return executionShape(*r)
	}
	switch r.Stage {
	case "set-observe", "session-open", "poll", "source", "ack", "acquire", "continuation", "started", "completed", "desired":
	default:
		return false
	}
	shape := baselineRecord{Version: r.Version, Stage: r.Stage, Outcome: r.Outcome, SetID: r.SetID, SessionID: r.SessionID, Creation: r.Creation}
	if r.Outcome != "intent" && r.Outcome != "observed" {
		shape.Intent = r.Intent
	}
	result := r.Outcome == "result" || r.Outcome == "unknown"
	switch r.Stage {
	case "set-observe":
		if result {
			shape.Set = r.Set
			shape.HTTPStatus = r.HTTPStatus
		}
	case "session-open":
		if result {
			shape.Session = r.Session
			shape.HTTPStatus = r.HTTPStatus
		}
	case "poll":
		shape.Cursor = r.Cursor
		if result {
			shape.Batch = r.Batch
			shape.HTTPStatus = r.HTTPStatus
			shape.NoMessage = r.NoMessage
		}
	case "source":
		shape.BatchRef = r.BatchRef
		if result {
			shape.Source = r.Source
			shape.HTTPStatus = r.HTTPStatus
		}
	case "ack":
		shape.BatchRef = r.BatchRef
		shape.SourceRef = r.SourceRef
		shape.MessageID = r.MessageID
	case "acquire":
		shape.BatchRef = r.BatchRef
		shape.SourceRef = r.SourceRef
		shape.ACKRef = r.ACKRef
		if result {
			shape.Accepted = r.Accepted
			shape.HTTPStatus = r.HTTPStatus
		}
	case "continuation":
		shape.AcquireRef = r.AcquireRef
	case "started", "completed":
		shape.BatchRef = r.BatchRef
		shape.ACKRef = r.ACKRef
		shape.ItemIndex = r.ItemIndex
	case "desired":
		shape.BatchRef = r.BatchRef
		shape.Desired = r.Desired
	}
	if !reflect.DeepEqual(*r, shape) {
		return false
	}
	if r.Stage == "started" || r.Stage == "completed" || r.Stage == "desired" {
		return r.Outcome == "observed"
	}
	return r.Outcome != "observed"
}

type baselineHistory struct {
	measurementEvidence                                                                                         *terminalEvidence
	terminalIntent, terminalResult, terminalLast, terminalDecision                                              controllerRecordRef
	terminalEvidence                                                                                            *terminalEvidence
	terminalStep                                                                                                int
	sessionCloseIntent, sessionCloseResult, workerDeleteRef, setDeleteIntent, setDeleteResult, setAbsenceResult controllerRecordRef
	workerDeletion                                                                                              *liveworker.DeletionReceipt
	startedRef                                                                                                  controllerRecordRef

	pair                                       *baselinePair
	pairIntent, pairRef, hostRef, rosterRef    controllerRecordRef
	jit                                        *baselineJIT
	jitRef, handoffRef, startRef, completedRef controllerRecordRef
	handoff                                    *baselineHandoff
	start                                      *baselineStart
	child                                      *baselineRecord
	childRef                                   controllerRecordRef
	sessionIntent, sessionResult               controllerRecordRef
	rounds                                     int
	lastSample                                 controllerRecordRef
	lastSDK                                    *sdkRunnerObservation
	lastJob                                    *restJobObservation
	lastREST                                   *restRunnerObservation
	lastLocal                                  *liveworker.LocalReceipt
	collectionRef                              controllerRecordRef

	seen                                    bool
	uncertain                               bool
	setID                                   int
	creation                                controllerRecordRef
	setObserved                             bool
	sessionID                               string
	pending                                 *baselineRecord
	pendingRef                              controllerRecordRef
	polls                                   int
	cursor                                  int
	messageIDs                              map[int]bool
	batch                                   *baselineBatch
	batchRef, sourceRef, ackRef, acquireRef controllerRecordRef
	latest                                  *baselineStatistics
	anchor                                  *baselineItem
	acquired, continued, complete           bool
	runnerID                                sdkRunnerID
	runnerName, result                      string
	callbacks                               map[int]bool
	messageDone                             bool
}

func replayBaseline(events []Event, identity controllerJournalIdentity, a Approval) (baselineHistory, error) {
	s := baselineHistory{messageIDs: map[int]bool{}, callbacks: map[int]bool{}, messageDone: true}
	lookup := func(ref controllerRecordRef) *Event {
		if ref.Sequence <= 0 || ref.Sequence > len(events) {
			return nil
		}
		e := events[ref.Sequence-1]
		if controllerEventRef(identity, e) != ref {
			return nil
		}
		return &e
	}
	for _, e := range events {
		if e.Baseline == nil {
			if s.seen && e.Kind != "authority" {
				return s, ErrJournal
			}
			continue
		}
		if !validBaselineShape(e) {
			return s, ErrJournal
		}
		r := e.Baseline
		ref := controllerEventRef(identity, e)
		if !s.seen {
			created := lookup(r.Creation)
			if (r.Stage != "set-observe" && r.Stage != "pair") || r.Outcome != "intent" || created == nil || created.Sequence >= e.Sequence || created.Kind != "result" || created.Operation != "create" || created.ID != r.SetID {
				return s, ErrJournal
			}
			s.setID = r.SetID
			s.creation = r.Creation
			s.seen = true
		}
		if r.SetID != s.setID || r.Creation != s.creation {
			return s, ErrJournal
		}
		if terminalStage(r.Stage) {
			if err := s.terminalRecord(*r, ref, lookup, a); err != nil {
				return s, err
			}
			continue
		}
		if executionStage(r.Stage) {
			if err := s.executionRecord(*r, ref, lookup, identity, a); err != nil {
				return s, err
			}
			continue
		}
		if s.terminalIntent.Sequence != 0 || s.collectionRef.Sequence != 0 || (s.uncertain && !(r.Stage == "continuation" && r.Outcome == "unknown" && s.child == nil && s.pending != nil && s.pending.Stage == "continuation")) {
			return s, ErrJournal
		}
		if r.Outcome == "intent" {
			if s.pending != nil || r.Intent != (controllerRecordRef{}) || r.Set != nil || r.Session != nil || r.Batch != nil || r.Source != nil || r.Accepted != nil || r.HTTPStatus != 0 || r.NoMessage || r.Desired != nil || r.ItemIndex != nil {
				return s, ErrJournal
			}
			switch r.Stage {
			case "set-observe":
				if s.setObserved || s.sessionID != "" || r.SessionID != "" || (s.pair != nil && s.rosterRef.Sequence == 0) {
					return s, ErrJournal
				}
			case "session-open":
				if !s.setObserved || s.sessionID != "" || r.SessionID != "" {
					return s, ErrJournal
				}
			case "poll":
				if s.sessionID == "" || !s.messageDone || s.polls >= 16 || r.Cursor != s.cursor {
					return s, ErrJournal
				}
				s.polls++
			case "source":
				if s.batch == nil || s.messageDone || s.sourceRef != (controllerRecordRef{}) || r.BatchRef != s.batchRef {
					return s, ErrJournal
				}
			case "ack":
				if s.batch == nil || s.messageDone || s.sourceRef == (controllerRecordRef{}) || s.ackRef != (controllerRecordRef{}) || r.BatchRef != s.batchRef || r.SourceRef != s.sourceRef || r.MessageID != s.batch.MessageID {
					return s, ErrJournal
				}
			case "acquire":
				if s.acquired || s.anchor == nil || s.batch == nil || !slices.ContainsFunc(s.batch.Items, func(x baselineItem) bool {
					return x.Kind == "JobAvailable" && x.Index == s.anchor.Index && x.RequestID == s.anchor.RequestID
				}) || s.ackRef == (controllerRecordRef{}) || r.BatchRef != s.batchRef || r.SourceRef != s.sourceRef || r.ACKRef != s.ackRef {
					return s, ErrJournal
				}
			case "continuation":
				if !s.acquired || s.continued || r.AcquireRef != s.acquireRef {
					return s, ErrJournal
				}
			default:
				return s, ErrJournal
			}
			if r.Stage != "set-observe" && r.Stage != "session-open" && r.SessionID != s.sessionID {
				return s, ErrJournal
			}
			s.pending = r
			s.pendingRef = ref
			if r.Stage == "session-open" {
				s.sessionIntent = ref
			}
			continue
		}
		if r.Outcome == "observed" {
			if s.pending != nil || s.sessionID == "" || r.SessionID != s.sessionID || r.BatchRef != s.batchRef || r.Intent != (controllerRecordRef{}) || r.Set != nil || r.Session != nil || r.Batch != nil || r.Source != nil || r.Accepted != nil {
				return s, ErrJournal
			}
			if r.Stage == "desired" {
				if r.Desired == nil || s.latest == nil || s.latest.Assigned == nil || *r.Desired != *s.latest.Assigned {
					return s, ErrJournal
				}
				if s.batch != nil {
					if s.ackRef == (controllerRecordRef{}) {
						return s, ErrJournal
					}
					for i, x := range s.batch.Items {
						if x.Kind == "JobAvailable" && !s.continued || (x.Kind == "JobStarted" || x.Kind == "JobCompleted") && !s.callbacks[i] {
							return s, ErrJournal
						}
					}
				}
				s.messageDone = true
			} else {
				if s.batch == nil || s.ackRef == (controllerRecordRef{}) || r.ACKRef != s.ackRef || r.ItemIndex == nil || *r.ItemIndex < 0 || *r.ItemIndex >= len(s.batch.Items) || s.callbacks[*r.ItemIndex] || !s.continued {
					return s, ErrJournal
				}
				x := s.batch.Items[*r.ItemIndex]
				kind := "JobStarted"
				if r.Stage == "completed" {
					kind = "JobCompleted"
				}
				if x.Kind != kind {
					return s, ErrJournal
				}
				s.callbacks[*r.ItemIndex] = true
				if r.Stage == "started" {
					s.startedRef = ref
				}
				if r.Stage == "completed" {
					s.complete = true
					s.completedRef = ref
				}
			}
			continue
		}
		if s.pending == nil || r.Intent != s.pendingRef || r.Stage != s.pending.Stage || r.SessionID != s.pending.SessionID || r.Cursor != s.pending.Cursor || r.MessageID != s.pending.MessageID || r.BatchRef != s.pending.BatchRef || r.SourceRef != s.pending.SourceRef || r.ACKRef != s.pending.ACKRef || r.AcquireRef != s.pending.AcquireRef {
			return s, ErrJournal
		}
		if r.Stage == "continuation" && (s.child != nil || (r.Outcome == "result" && s.pair != nil && (s.startRef.Sequence == 0 || s.rounds != 4))) {
			return s, ErrJournal
		}
		if r.Stage == "session-open" {
			s.sessionIntent = r.Intent
			s.sessionResult = ref
		}
		s.pending = nil
		if r.Outcome == "unknown" {
			s.uncertain = true
			continue
		}
		switch r.Stage {
		case "set-observe":
			if r.HTTPStatus != 200 || r.Set == nil || !r.Set.eligible(a, s.setID) {
				return s, ErrJournal
			}
			s.setObserved = true
		case "session-open":
			x := r.Session
			if r.HTTPStatus != 200 || x == nil || !x.eligible(a, s.setID) {
				return s, ErrJournal
			}
			s.sessionID = x.SessionID
			s.latest = x.Statistics
		case "poll":
			if r.NoMessage {
				if r.HTTPStatus != 202 || r.Batch != nil {
					return s, ErrJournal
				}
				break
			}
			if r.HTTPStatus != 200 || r.Batch == nil || r.Batch.MessageID <= 0 || s.messageIDs[r.Batch.MessageID] || !r.Batch.Statistics.eligible(s.acquired) {
				return s, ErrJournal
			}
			if err := s.admitBatch(r.Batch, a); err != nil {
				return s, err
			}
			s.batch = r.Batch
			s.batchRef = ref
			s.sourceRef = controllerRecordRef{}
			s.ackRef = controllerRecordRef{}
			s.callbacks = map[int]bool{}
			s.messageDone = false
			s.messageIDs[r.Batch.MessageID] = true
			s.latest = r.Batch.Statistics
		case "source":
			if r.HTTPStatus != 200 || !reflect.DeepEqual(r.Source, baselineApprovedSource(a)) {
				return s, ErrJournal
			}
			s.sourceRef = ref
		case "ack":
			s.ackRef = ref
			s.cursor = s.batch.MessageID
		case "acquire":
			if r.HTTPStatus != 200 || r.Accepted == nil || r.Accepted.Count == nil || *r.Accepted.Count != 1 || len(r.Accepted.IDs) != 1 || r.Accepted.IDs[0] != s.anchor.RequestID {
				return s, ErrJournal
			}
			s.acquired = true
			s.acquireRef = ref
		case "continuation":
			s.continued = true
		default:
			return s, ErrJournal
		}
	}
	return s, nil
}
func (s *baselineHistory) admitBatch(b *baselineBatch, a Approval) error {
	if len(b.Items) > 4 {
		return ErrJournal
	}
	var available *baselineItem
	for i := range b.Items {
		x := &b.Items[i]
		if !x.bounded() || x.Index != i || x.RequestID <= 0 || x.JobID == "" {
			return ErrJournal
		}
		if x.Kind == "JobAvailable" {
			if available != nil || s.anchor != nil || s.acquired {
				return ErrJournal
			}
			available = x
		}
		if !s.acquired && (x.Kind == "JobStarted" || x.Kind == "JobCompleted") {
			return ErrJournal
		}
	}
	if available != nil {
		x := available
		if x.Owner == nil || *x.Owner != a.Organization || x.Repository == nil || *x.Repository != a.Repository || x.RunID == nil || *x.RunID != a.WorkflowRunID || x.Event == nil || *x.Event != "workflow_dispatch" || x.WorkflowRef == nil || *x.WorkflowRef == "" {
			return ErrJournal
		}
		s.anchor = x
	}
	for _, x := range b.Items {
		if s.anchor == nil || x.RequestID != s.anchor.RequestID || x.JobID != s.anchor.JobID {
			return ErrJournal
		}
		if x.Kind != "JobAvailable" && x.Kind != "JobAssigned" && x.Kind != "JobStarted" && x.Kind != "JobCompleted" {
			return ErrJournal
		}
		for _, p := range [][2]*string{{x.Owner, s.anchor.Owner}, {x.Repository, s.anchor.Repository}, {x.Event, s.anchor.Event}, {x.WorkflowRef, s.anchor.WorkflowRef}} {
			if p[0] != nil && *p[0] != "" && (p[1] == nil || *p[0] != *p[1]) {
				return ErrJournal
			}
		}
		if x.RunID != nil && (*x.RunID < 0 || (*x.RunID > 0 && *x.RunID != a.WorkflowRunID)) {
			return ErrJournal
		}
		if (x.Kind == "JobAvailable" || x.Kind == "JobAssigned") && (x.RunnerID != nil || x.RunnerName != nil || x.Result != nil) {
			return ErrJournal
		}
		if s.jit != nil && s.jit.Runner != nil {
			if x.RunnerID != nil && *x.RunnerID > 0 && int(*x.RunnerID) != int(s.jit.Runner.ID) {
				return ErrJournal
			}
			if x.RunnerName != nil && *x.RunnerName != "" && *x.RunnerName != s.jit.Runner.Name {
				return ErrJournal
			}
		}
		if x.RunnerID != nil {
			if *x.RunnerID < 0 || (*x.RunnerID > 0 && s.runnerID > 0 && *x.RunnerID != s.runnerID) {
				return ErrJournal
			}
			if *x.RunnerID > 0 {
				s.runnerID = *x.RunnerID
			}
		}
		if x.RunnerName != nil && *x.RunnerName != "" {
			if s.runnerName != "" && s.runnerName != *x.RunnerName {
				return ErrJournal
			}
			s.runnerName = *x.RunnerName
		}
		if x.Result != nil && *x.Result != "" {
			if x.Kind != "JobCompleted" || (s.result != "" && s.result != *x.Result) {
				return ErrJournal
			}
			s.result = *x.Result
		}
	}
	return nil
}
func (s *baselineSessionFacts) eligible(a Approval, id int) bool {
	if s == nil || s.Owner != a.setName() || !s.Statistics.eligible(false) {
		return false
	}
	u, e := uuid.Parse(s.SessionID)
	if e != nil || u == uuid.Nil || u.String() != s.SessionID {
		return false
	}
	return !s.NestedSet || (s.SetID == id && s.SetName == a.setName() && s.GroupID == a.RunnerGroupID && s.NestedStatistics.eligible(false))
}
