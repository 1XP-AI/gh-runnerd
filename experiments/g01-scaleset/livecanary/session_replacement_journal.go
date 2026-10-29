package livecanary

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"slices"
	"sort"

	"github.com/google/uuid"
)

func (j *FileJournal) replacementPhysicalHistoryLocked() bool {
	if len(j.headerBytes) == 0 || len(j.events) != len(j.eventBytes) {
		return false
	}
	expected := append([]byte(nil), j.headerBytes...)
	for _, line := range j.eventBytes {
		expected = append(expected, line...)
	}
	defer clear(expected)
	info, err := j.file.Stat()
	if err != nil || info.Size() != int64(len(expected)) || info.Size() > baselineJournalLimit {
		return false
	}
	got := make([]byte, len(expected))
	defer clear(got)
	_, err = io.ReadFull(io.NewSectionReader(j.file, 0, int64(len(got))), got)
	return err == nil && bytes.Equal(got, expected)
}

const replacementRecordLimit = 16 << 10
const replacementRecordCount = 32

type replacementWork struct {
	RequestID int64  `json:"request_id"`
	JobID     string `json:"job_id"`
}
type replacementSummary struct {
	ClosedSessions     [2]string `json:"closed_sessions"`
	RetainedRequestIDs []int64   `json:"retained_request_ids"`
	SetRetained        bool      `json:"set_retained"`
	CallbackEntries    *int      `json:"callback_entries"`
}
type replacementRecord struct {
	Version            int                       `json:"version"`
	Stage              string                    `json:"stage"`
	Outcome            string                    `json:"outcome"`
	Generation         int                       `json:"generation"`
	SetID              int                       `json:"set_id"`
	Owner              string                    `json:"owner"`
	RawApproval        string                    `json:"raw_approval_sha256"`
	CanonicalApproval  string                    `json:"canonical_approval_sha256"`
	CreatePrefixSHA256 string                    `json:"create_prefix_sha256"`
	Identity           controllerJournalIdentity `json:"identity"`
	Creation           controllerRecordRef       `json:"creation"`
	CreateConsumption  controllerRecordRef       `json:"create_consumption"`
	Consumption        controllerRecordRef       `json:"consumption"`
	Phase              controllerRecordRef       `json:"phase_ref"`
	Previous           controllerRecordRef       `json:"previous"`
	SessionID          string                    `json:"session_id,omitempty"`
	Cursor             *int                      `json:"cursor,omitempty"`
	MessageID          *int                      `json:"message_id,omitempty"`
	Status             int                       `json:"status,omitempty"`
	Statistics         *baselineStatistics       `json:"statistics,omitempty"`
	Work               *replacementWork          `json:"work,omitempty"`
	VerifiedSource     *baselineSource           `json:"verified_source,omitempty"`
	Summary            *replacementSummary       `json:"summary,omitempty"`
}

// Require exactly the fields emitted by this version, including explicit
// generation zero and cursor/message zero. Nulls cannot manufacture presence.
func (r *replacementRecord) UnmarshalJSON(data []byte) error {
	type plain replacementRecord
	var p plain
	if DecodeStrict(data, &p) != nil {
		return ErrJournal
	}
	canonical, _ := json.Marshal(p)
	var got, want map[string]json.RawMessage
	if json.Unmarshal(data, &got) != nil || json.Unmarshal(canonical, &want) != nil || len(got) != len(want) {
		return ErrJournal
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			return ErrJournal
		}
	}
	*r = replacementRecord(p)
	return nil
}

type replacementStep struct {
	stage, outcome string
	generation     int
}

var replacementSteps = []replacementStep{
	{"start", "observed", 0},
	{"set-before", "intent", 0}, {"set-before", "result", 0},
	{"worker-before", "intent", 0}, {"worker-before", "result", 0},
	{"open", "intent", 1}, {"open", "result", 1},
	{"poll", "intent", 1}, {"poll", "result", 1},
	{"retire", "observed", 1},
	{"close", "intent", 1}, {"close", "result", 1},
	{"open", "intent", 2}, {"open", "result", 2},
	{"poll", "intent", 2}, {"poll", "result", 2},
	{"discard", "observed", 1},
	{"set-after", "intent", 0}, {"set-after", "result", 0},
	{"worker-after", "intent", 0}, {"worker-after", "result", 0},
	{"close", "intent", 2}, {"close", "result", 2},
	{"complete", "observed", 0},
}

func validReplacementShape(e Event) bool {
	if e.Replacement == nil || e.Kind != "session-replacement" {
		return false
	}
	r := e.Replacement
	x := e
	x.Replacement = nil
	if !reflect.DeepEqual(x, Event{Sequence: e.Sequence, Kind: e.Kind}) || r.Version != 1 || r.SetID <= 0 || !nonce.MatchString(r.Owner) || !validHandoffSHA256(r.RawApproval) {
		return false
	}
	data, err := json.Marshal(e)
	return err == nil && len(data)+1 <= replacementRecordLimit
}

func (j *FileJournal) replacementRefLocked(index int) controllerRecordRef {
	if index < 0 || index >= len(j.events) || index >= len(j.eventBytes) {
		return controllerRecordRef{}
	}
	id, err := j.controllerIdentity()
	if err != nil {
		return controllerRecordRef{}
	}
	encoded, _ := json.Marshal(id)
	h := sha256.New()
	h.Write([]byte("gh-runnerd/g01-session-replacement/physical-event/v1\x00"))
	h.Write(encoded)
	h.Write([]byte{0})
	h.Write(j.eventBytes[index])
	return controllerRecordRef{Sequence: index + 1, EventSHA256: hex.EncodeToString(h.Sum(nil))}
}

func (j *FileJournal) replacementPrefixDigestLocked() string {
	if len(j.eventBytes) < 7 {
		return ""
	}
	h := sha256.New()
	h.Write([]byte("gh-runnerd/g01-session-replacement/create-prefix/v1\x00"))
	h.Write(j.headerBytes)
	for _, line := range j.eventBytes[:7] {
		h.Write(line)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Called by append and OpenJournal under exclusive ownership. Every accepted
// prefix is historical evidence only; there is no constructor from this state.
func (j *FileJournal) replayReplacement(events []Event) error {
	anchor := slices.IndexFunc(events, func(e Event) bool { return e.ControllerHandoff != nil && e.ControllerHandoff.Phase == replacementPhase })
	if anchor < 0 {
		if slices.ContainsFunc(events, func(e Event) bool { return e.Replacement != nil || e.Kind == "session-replacement" }) {
			return ErrJournal
		}
		return nil
	}
	if anchor != 7 || len(j.eventBytes) < 7 {
		return ErrJournal
	}
	id, err := j.controllerIdentity()
	if err != nil {
		return err
	}
	raw := events[0].ControllerHandoff
	if raw == nil {
		return ErrJournal
	}
	if _, err = replacementCreatePrefix(j.approval, j, raw.ApprovalSHA256, events[:7]); err != nil {
		return err
	}
	if !validControllerHandoffEvent(events[7]) || events[7].ControllerHandoff.ApprovalSHA256 != raw.ApprovalSHA256 {
		return ErrJournal
	}
	if len(events) == 8 {
		return nil
	}
	if !reflect.DeepEqual(events[8], Event{Sequence: 9, Kind: "phase", Operation: replacementPhase}) {
		return replacementInspectSuffix(events[8:])
	}
	first := 9
	end := first
	for end < len(events) && events[end].Replacement != nil {
		end++
	}
	if end-first > len(replacementSteps) || (end > first && len(j.eventBytes) < first) {
		return ErrJournal
	}
	sessions := [3]string{}
	requests := map[int64]bool{}
	for n, e := range events[first:end] {
		if !validReplacementShape(e) || e.Sequence != first+n+1 {
			return ErrJournal
		}
		r := e.Replacement
		step := replacementSteps[n]
		expected := replacementRecord{Version: 1, Stage: step.stage, Outcome: step.outcome, Generation: step.generation, SetID: events[6].ID, Owner: j.approval.OwnerNonce, RawApproval: raw.ApprovalSHA256, CanonicalApproval: approvalDigest(j.approval), CreatePrefixSHA256: j.replacementPrefixDigestLocked(), Identity: id, Creation: j.replacementRefLocked(6), CreateConsumption: j.replacementRefLocked(0), Consumption: j.replacementRefLocked(7), Phase: j.replacementRefLocked(8), Previous: j.replacementRefLocked(e.Sequence - 2)}
		if step.generation > 0 {
			if step.stage == "open" && step.outcome == "result" {
				u, err := uuid.Parse(r.SessionID)
				if err != nil || u == uuid.Nil || u.String() != r.SessionID || r.SessionID == sessions[1] {
					return ErrJournal
				}
				sessions[step.generation] = r.SessionID
			}
			expected.SessionID = sessions[step.generation]
		}
		switch {
		case step.stage == "poll" && step.outcome == "intent":
			zero := 0
			expected.Cursor = &zero
		case step.outcome == "result":
			switch step.stage {
			case "set-before", "set-after", "open":
				if !replacementIdle(r.Statistics) {
					return ErrJournal
				}
				expected.Status = 200
				expected.Statistics = r.Statistics
			case "worker-before", "worker-after":
				expected.Status = 200
			case "close":
				expected.Status = 204
			case "poll":
				expected.Status = r.Status
				if r.Status == 202 && step.generation == 2 {
					break
				}
				if r.Status != 200 || r.MessageID == nil || *r.MessageID < 0 || r.Work == nil || r.Work.RequestID <= 0 || !baselineText(r.Work.JobID, 256) || r.Work.JobID == "" || !replacementIdle(r.Statistics) || !reflect.DeepEqual(r.VerifiedSource, baselineApprovedSource(j.approval)) {
					return ErrJournal
				}
				expected.MessageID = r.MessageID
				expected.Work = r.Work
				expected.Statistics = r.Statistics
				expected.VerifiedSource = r.VerifiedSource
				requests[r.Work.RequestID] = true
			}
		}
		if step.stage == "complete" {
			zero := 0
			ids := make([]int64, 0, len(requests))
			for id := range requests {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, k int) bool { return ids[i] < ids[k] })
			expected.Summary = &replacementSummary{[2]string{sessions[1], sessions[2]}, ids, true, &zero}
		}
		if !reflect.DeepEqual(*r, expected) {
			return ErrJournal
		}
	}
	return replacementInspectSuffix(events[end:])
}

// The one pre-existing inspect slot remains available after any stopped
// prefix. It admits reads only and never clears the replacement quarantine.
func replacementInspectSuffix(events []Event) error {
	if len(events) == 0 {
		return nil
	}
	if events[0].ControllerHandoff != nil {
		if !validControllerHandoffEvent(events[0]) || events[0].ControllerHandoff.Phase != "inspect" {
			return ErrJournal
		}
		events = events[1:]
	}
	if len(events) == 0 {
		return nil
	}
	if !reflect.DeepEqual(events[0], Event{Sequence: events[0].Sequence, Kind: "phase", Operation: "inspect"}) {
		return ErrJournal
	}
	stages := []struct{ kind, op string }{{"intent", "observe-owned"}, {"result", "observe-owned"}, {"intent", "observe-runner"}, {"result", "observe-runner"}, {"observation", "inspect"}}
	if len(events)-1 > len(stages) {
		return ErrJournal
	}
	for n, e := range events[1:] {
		if e.Kind != stages[n].kind || e.Operation != stages[n].op || !validEvent(e) || e.Replacement != nil || e.Baseline != nil || e.Authority != nil || e.ControllerHandoff != nil {
			return ErrJournal
		}
	}
	return nil
}

func replacementIdle(s *baselineStatistics) bool {
	return s != nil && s.eligible(false) && s.Assigned != nil && *s.Assigned == 0
}

func (j *FileJournal) replacementCapacityLocked() error {
	st, err := j.file.Stat()
	count := 0
	for _, e := range j.events {
		if e.Replacement != nil {
			count++
		}
	}
	if err != nil || j.writeFailed || count >= replacementRecordCount || st.Size()+int64((replacementRecordCount-count)*replacementRecordLimit) > baselineJournalLimit {
		return ErrJournal
	}
	return nil
}

func cloneReplacement(r *replacementRecord) *replacementRecord {
	if r == nil {
		return nil
	}
	data, _ := json.Marshal(r)
	defer clear(data)
	var x replacementRecord
	if DecodeStrict(data, &x) != nil {
		return nil
	}
	return &x
}
