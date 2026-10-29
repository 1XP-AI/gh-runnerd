package livecanary

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
)

const jitBoundaryRecordLimit = 16 << 10
const jitBoundaryRecordCount = 16

type jitBoundaryLookup struct {
	Count    int  `json:"count"`
	RunnerID *int `json:"runner_id,omitempty"`
}
type jitBoundarySummary struct {
	SetRetained           bool   `json:"set_retained"`
	WorkerReservationHeld bool   `json:"worker_reservation_held"`
	JITRetryAllowed       bool   `json:"jit_retry_allowed"`
	ForwardObservation    string `json:"forward_observation"`
}
type jitBoundaryRecord struct {
	Version              int                       `json:"version"`
	Phase                string                    `json:"phase"`
	Stage                string                    `json:"stage"`
	Outcome              string                    `json:"outcome"`
	SetID                int                       `json:"set_id"`
	Owner                string                    `json:"owner"`
	WorkerName           string                    `json:"worker_name"`
	RawApproval          string                    `json:"raw_approval_sha256"`
	CanonicalApproval    string                    `json:"canonical_approval_sha256"`
	CreatePrefixSHA256   string                    `json:"create_prefix_sha256"`
	Identity             controllerJournalIdentity `json:"identity"`
	Creation             controllerRecordRef       `json:"creation"`
	CreateConsumption    controllerRecordRef       `json:"create_consumption"`
	Consumption          controllerRecordRef       `json:"consumption"`
	PhaseRef             controllerRecordRef       `json:"phase_ref"`
	Previous             controllerRecordRef       `json:"previous"`
	Status               *int                      `json:"http_status,omitempty"`
	PreflightReads       *int                      `json:"preflight_reads,omitempty"`
	Set                  *baselineSetFacts         `json:"set,omitempty"`
	Lookup               *jitBoundaryLookup        `json:"lookup,omitempty"`
	RequestValidated     *bool                     `json:"request_validated,omitempty"`
	Forwarded            string                    `json:"forwarded,omitempty"`
	ObservedForwardCount *int                      `json:"observed_forward_count,omitempty"`
	Suppressed           *bool                     `json:"suppressed,omitempty"`
	Retained             *bool                     `json:"retained,omitempty"`
	Summary              *jitBoundarySummary       `json:"summary,omitempty"`
}

// Compare member presence recursively with this version's serialized schema.
// This rejects null, folded aliases and zero-valued foreign envelope members.
func jitBoundaryMemberShape(data, canonical []byte) bool {
	var got, want any
	if json.Unmarshal(data, &got) != nil || json.Unmarshal(canonical, &want) != nil {
		return false
	}
	var same func(any, any) bool
	same = func(a, b any) bool {
		if a == nil || b == nil {
			return false
		}
		switch x := b.(type) {
		case map[string]any:
			y, ok := a.(map[string]any)
			if !ok || len(y) != len(x) {
				return false
			}
			for k, v := range x {
				z, ok := y[k]
				if !ok || !same(z, v) {
					return false
				}
			}
			return true
		case []any:
			y, ok := a.([]any)
			if !ok || len(y) != len(x) {
				return false
			}
			for i := range x {
				if !same(y[i], x[i]) {
					return false
				}
			}
			return true
		default:
			return reflect.TypeOf(a) == reflect.TypeOf(b)
		}
	}
	return same(got, want)
}
func (r *jitBoundaryRecord) UnmarshalJSON(data []byte) error {
	type plain jitBoundaryRecord
	var p plain
	if DecodeStrict(data, &p) != nil {
		return ErrJournal
	}
	canonical, _ := json.Marshal(p)
	if !jitBoundaryMemberShape(data, canonical) {
		return ErrJournal
	}
	*r = jitBoundaryRecord(p)
	return nil
}
func (e *Event) UnmarshalJSON(data []byte) error {
	type plain Event
	var p plain
	if DecodeStrict(data, &p) != nil {
		return ErrJournal
	}
	if p.JITBoundary != nil || p.Kind == "jit-boundary" {
		canonical, _ := json.Marshal(p)
		if p.JITBoundary == nil || !jitBoundaryMemberShape(data, canonical) {
			return ErrJournal
		}
	}
	*e = Event(p)
	return nil
}

type jitBoundaryStep struct{ stage, outcome string }

func jitBoundarySteps(phase string) []jitBoundaryStep {
	x := []jitBoundaryStep{{"start", "observed"}, {"preflight", "intent"}, {"preflight", "result"}, {"set-before", "intent"}, {"set-before", "result"}, {"worker-before", "intent"}, {"worker-before", "result"}, {"jit", "intent"}, {"jit", "request-validated"}}
	if phase == beforeJITPhase {
		x = append(x, jitBoundaryStep{"jit", "not-attempted"})
	} else {
		x = append(x, jitBoundaryStep{"jit", "send-intent"}, jitBoundaryStep{"jit", "response-suppressed"})
	}
	return append(x, jitBoundaryStep{"worker-after", "intent"}, jitBoundaryStep{"worker-after", "result"}, jitBoundaryStep{"complete", "observed"})
}
func validJITBoundaryShape(e Event) bool {
	if e.JITBoundary == nil || e.Kind != "jit-boundary" {
		return false
	}
	x := e
	x.JITBoundary = nil
	r := e.JITBoundary
	if !reflect.DeepEqual(x, Event{Sequence: e.Sequence, Kind: e.Kind}) || r.Version != 1 || !jitBoundaryPhase(r.Phase) || r.SetID <= 0 || !nonce.MatchString(r.Owner) || !validHandoffSHA256(r.RawApproval) {
		return false
	}
	data, err := json.Marshal(e)
	return err == nil && len(data)+1 <= jitBoundaryRecordLimit
}
func (j *FileJournal) jitBoundaryRefLocked(index int) controllerRecordRef {
	if index < 0 || index >= len(j.events) || index >= len(j.eventBytes) {
		return controllerRecordRef{}
	}
	id, err := j.controllerIdentity()
	if err != nil {
		return controllerRecordRef{}
	}
	encoded, _ := json.Marshal(id)
	h := sha256.New()
	h.Write([]byte("gh-runnerd/g01-jit-boundary/physical-event/v1\x00"))
	h.Write(encoded)
	h.Write([]byte{0})
	h.Write(j.eventBytes[index])
	return controllerRecordRef{index + 1, hex.EncodeToString(h.Sum(nil))}
}
func (j *FileJournal) jitBoundaryPrefixDigestLocked() string {
	if len(j.eventBytes) < 7 {
		return ""
	}
	h := sha256.New()
	h.Write([]byte("gh-runnerd/g01-jit-boundary/create-prefix/v1\x00"))
	h.Write(j.headerBytes)
	for _, line := range j.eventBytes[:7] {
		h.Write(line)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (j *FileJournal) jitBoundaryBaseLocked(phase string, n int) jitBoundaryRecord {
	step := jitBoundarySteps(phase)[n]
	id, _ := j.controllerIdentity()
	return jitBoundaryRecord{Version: 1, Phase: phase, Stage: step.stage, Outcome: step.outcome, SetID: j.events[6].ID, Owner: j.approval.OwnerNonce, WorkerName: j.approval.workerName(), RawApproval: j.events[0].ControllerHandoff.ApprovalSHA256, CanonicalApproval: approvalDigest(j.approval), CreatePrefixSHA256: j.jitBoundaryPrefixDigestLocked(), Identity: id, Creation: j.jitBoundaryRefLocked(6), CreateConsumption: j.jitBoundaryRefLocked(0), Consumption: j.jitBoundaryRefLocked(7), PhaseRef: j.jitBoundaryRefLocked(8), Previous: j.jitBoundaryRefLocked(8 + n)}
}
func jitBoundaryZeroSet(a Approval, id int, v *baselineSetFacts) bool {
	return v != nil && v.matchesOwner(a, id) && len(v.Labels) == 1 && v.Labels[0].Name == a.setName() && v.Labels[0].Type == "System" && replacementIdle(v.Statistics) && v.Statistics.Available != nil && *v.Statistics.Available == 0
}
func (j *FileJournal) replayJITBoundary(events []Event) error {
	typed := slices.IndexFunc(events, func(e Event) bool { return e.JITBoundary != nil || e.Kind == "jit-boundary" })
	anchor := slices.IndexFunc(events, func(e Event) bool { return e.ControllerHandoff != nil && jitBoundaryPhase(e.ControllerHandoff.Phase) })
	if typed < 0 {
		// Ordinary jit-loss history stays ordinary. Consumption/phase alone fences
		// future effects, but only the new typed domain activates its suffix parser.
		if anchor >= 0 && events[anchor].ControllerHandoff.Phase == beforeJITPhase {
			if anchor != 7 {
				return ErrJournal
			}
			if _, err := jitBoundaryCreatePrefix(j.approval, j, events[7].ControllerHandoff.ApprovalSHA256, beforeJITPhase, events[:7]); err != nil {
				return err
			}
			if len(events) > 8 {
				if reflect.DeepEqual(events[8], Event{Sequence: 9, Kind: "phase", Operation: beforeJITPhase}) {
					return replacementInspectSuffix(events[9:])
				}
				return replacementInspectSuffix(events[8:])
			}
		}
		return nil
	}
	if typed != 9 || anchor != 7 || len(j.eventBytes) < 9 {
		return ErrJournal
	}
	phase := events[7].ControllerHandoff.Phase
	if _, err := jitBoundaryCreatePrefix(j.approval, j, events[7].ControllerHandoff.ApprovalSHA256, phase, events[:7]); err != nil {
		return err
	}
	if !validControllerHandoffEvent(events[7]) || !reflect.DeepEqual(events[8], Event{Sequence: 9, Kind: "phase", Operation: phase}) {
		return ErrJournal
	}
	end := 9
	for end < len(events) && events[end].JITBoundary != nil {
		end++
	}
	steps := jitBoundarySteps(phase)
	if end-9 > len(steps) {
		return ErrJournal
	}
	for n, e := range events[9:end] {
		if !validJITBoundaryShape(e) || e.Sequence != n+10 {
			return ErrJournal
		}
		r := e.JITBoundary
		expected := j.jitBoundaryBaseLocked(phase, n)
		yes, no, zero, one, status := true, false, 0, 1, 200
		switch {
		case r.Stage == "start" || r.Stage == "jit" && r.Outcome == "intent":
			expected.Retained = &yes
		case r.Stage == "preflight" && r.Outcome == "result":
			count := 2
			if j.approval.ManagementScope == "" {
				count = 4
			}
			expected.PreflightReads = &count
		case r.Stage == "set-before" && r.Outcome == "result":
			if !jitBoundaryZeroSet(j.approval, expected.SetID, r.Set) {
				return ErrJournal
			}
			expected.Status = &status
			expected.Set = r.Set
		case (r.Stage == "worker-before" || r.Stage == "worker-after") && r.Outcome == "result":
			if r.Lookup == nil || r.Lookup.Count < 0 || r.Lookup.Count > 1 || r.Lookup.Count == 0 && r.Lookup.RunnerID != nil || r.Lookup.Count == 1 && (r.Lookup.RunnerID == nil || *r.Lookup.RunnerID <= 0) || (r.Stage == "worker-before" || phase == beforeJITPhase) && r.Lookup.Count != 0 {
				return ErrJournal
			}
			expected.Status = &status
			expected.Lookup = r.Lookup
		case r.Stage == "jit" && r.Outcome == "request-validated":
			expected.RequestValidated = &yes
		case r.Stage == "jit" && r.Outcome == "not-attempted":
			expected.RequestValidated = &yes
			expected.Forwarded = "none"
			expected.ObservedForwardCount = &zero
		case r.Stage == "jit" && r.Outcome == "send-intent":
			expected.Forwarded = "possible"
		case r.Stage == "jit" && r.Outcome == "response-suppressed":
			expected.Status = &status
			expected.Forwarded = "observed-one"
			expected.ObservedForwardCount = &one
			expected.Suppressed = &yes
		case r.Stage == "complete":
			obs := "none"
			if phase == "jit-loss" {
				obs = "observed-one"
			}
			expected.Summary = &jitBoundarySummary{yes, yes, no, obs}
		}
		if !reflect.DeepEqual(*r, expected) {
			return ErrJournal
		}
	}
	suffix := events[end:]
	if slices.ContainsFunc(suffix, func(e Event) bool { return e.JITBoundary != nil }) {
		return ErrJournal
	}
	return replacementInspectSuffix(suffix)
}
func (j *FileJournal) jitBoundaryCapacityLocked() error {
	st, err := j.file.Stat()
	count := 0
	for _, e := range j.events {
		if e.JITBoundary != nil {
			count++
		}
	}
	if err != nil || j.writeFailed || count > jitBoundaryRecordCount || st.Size()+int64((jitBoundaryRecordCount-count)*jitBoundaryRecordLimit) > baselineJournalLimit {
		return ErrJournal
	}
	return nil
}
func cloneJITBoundary(r *jitBoundaryRecord) *jitBoundaryRecord {
	if r == nil {
		return nil
	}
	data, _ := json.Marshal(r)
	defer clear(data)
	var out jitBoundaryRecord
	if DecodeStrict(data, &out) != nil {
		return nil
	}
	return &out
}
func (j *FileJournal) jitBoundaryPhysicalLocked() bool {
	if !j.replacementPhysicalHistoryLocked() {
		return false
	}
	// Also bind in-memory decoded facts to each actual stored line.
	for i, line := range j.eventBytes {
		var e Event
		if DecodeStrict(bytes.TrimSuffix(line, []byte{'\n'}), &e) != nil || !reflect.DeepEqual(e, j.events[i]) {
			return false
		}
	}
	return true
}
