package livecanary

import (
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/actions/scaleset"
)

// Pointer facts retain missing/null separately from explicit zero/false. No
// queue/acquisition URL, token, arbitrary display text or raw response is kept.
type baselineStatistics struct {
	Available  *int `json:"total_available_jobs"`
	Acquired   *int `json:"total_acquired_jobs"`
	Assigned   *int `json:"total_assigned_jobs"`
	Running    *int `json:"total_running_jobs"`
	Registered *int `json:"total_registered_runners"`
	Busy       *int `json:"total_busy_runners"`
	Idle       *int `json:"total_idle_runners"`
}
type baselineItem struct {
	Index       int          `json:"index"`
	Kind        string       `json:"kind"`
	RequestID   int64        `json:"request_id"`
	JobID       string       `json:"job_id"`
	Owner       *string      `json:"owner"`
	Repository  *string      `json:"repository"`
	RunID       *int64       `json:"run_id"`
	Event       *string      `json:"event"`
	WorkflowRef *string      `json:"workflow_ref"`
	RunnerID    *sdkRunnerID `json:"sdk_runner_id"`
	RunnerName  *string      `json:"runner_name"`
	Result      *string      `json:"result"`
	Labels      []string     `json:"labels"`
	QueueTime   *time.Time   `json:"queue_time"`
	AssignTime  *time.Time   `json:"assign_time"`
	RunnerTime  *time.Time   `json:"runner_time"`
	FinishTime  *time.Time   `json:"finish_time"`
}
type baselineBatch struct {
	MessageID  int                 `json:"message_id"`
	Statistics *baselineStatistics `json:"statistics"`
	Items      []baselineItem      `json:"items"`
}
type baselineSessionFacts struct {
	SessionID        string              `json:"session_id"`
	Owner            string              `json:"owner"`
	Statistics       *baselineStatistics `json:"statistics"`
	NestedStatistics *baselineStatistics `json:"nested_statistics"`
	NestedSet        bool                `json:"nested_set"`
	SetID            int                 `json:"set_id"`
	SetName          string              `json:"set_name"`
	GroupID          int                 `json:"group_id"`
}
type baselineAccepted struct {
	Count *int    `json:"count"`
	IDs   []int64 `json:"ids"`
}

func (a *baselineAccepted) matches(ids []int64) bool {
	return a != nil && a.Count != nil && *a.Count == len(ids) && slices.Equal(a.IDs, ids)
}

type baselineSetFacts struct {
	ID            int                 `json:"id"`
	Name          string              `json:"name"`
	GroupID       int                 `json:"group_id"`
	Labels        []scaleset.Label    `json:"labels"`
	DisableUpdate *bool               `json:"disable_update"`
	Statistics    *baselineStatistics `json:"statistics"`
}

func decodeBaselineSet(data []byte) (*baselineSetFacts, error) {
	var w struct {
		ID      int              `json:"id"`
		Name    string           `json:"name"`
		Group   int              `json:"runnerGroupId"`
		Labels  []scaleset.Label `json:"labels"`
		Setting struct {
			Disabled *bool `json:"disableUpdate"`
		} `json:"RunnerSetting"`
		Stats json.RawMessage `json:"statistics"`
	}
	if !uniqueKeys(json.NewDecoder(strings.NewReader(string(data)))) || json.Unmarshal(data, &w) != nil || !baselineText(w.Name, 128) || len(w.Labels) > 8 {
		return nil, ErrRemote
	}
	for _, l := range w.Labels {
		if !baselineText(l.Name, 128) || !baselineText(l.Type, 32) {
			return nil, ErrRemote
		}
	}
	s, e := baselineStats(w.Stats)
	if e != nil {
		return nil, e
	}
	return &baselineSetFacts{w.ID, w.Name, w.Group, w.Labels, w.Setting.Disabled, s}, nil
}
func (s *baselineSetFacts) eligible(a Approval, id int) bool {
	return s.eligibleWithStats(a, id, false)
}

func (s *baselineSetFacts) eligibleForDrain(a Approval, id int) bool {
	if !s.matchesOwner(a, id) || s.Statistics == nil {
		return false
	}
	for _, p := range []*int{s.Statistics.Available, s.Statistics.Acquired, s.Statistics.Assigned, s.Statistics.Running, s.Statistics.Registered, s.Statistics.Busy, s.Statistics.Idle} {
		if p == nil || *p < 0 {
			return false
		}
	}
	return true
}

func (s *baselineSetFacts) eligibleWithStats(a Approval, id int, acquired bool) bool {
	return s.matchesOwner(a, id) && s.Statistics.eligible(acquired)
}

func (s *baselineSetFacts) matchesOwner(a Approval, id int) bool {
	if s == nil || s.ID != id || s.Name != a.setName() || s.GroupID != a.RunnerGroupID || s.DisableUpdate == nil || !*s.DisableUpdate {
		return false
	}
	for _, l := range s.Labels {
		if l.Name == a.setName() {
			return true
		}
	}
	return false
}

// matches compares the bounded facts captured by the strict wire reader with
// the SDK value returned from the same request. A caller may use eligible to
// establish approved identity, but must also prove that the lossy SDK object
// did not disagree with those wire facts.
func (s *baselineSetFacts) matches(v *scaleset.RunnerScaleSet) bool {
	if s == nil || v == nil || s.ID != v.ID || s.Name != v.Name || s.GroupID != v.RunnerGroupID || s.DisableUpdate == nil || *s.DisableUpdate != v.RunnerSetting.DisableUpdate || !slices.Equal(s.Labels, v.Labels) {
		return false
	}
	return s.Statistics.matches(v.Statistics)
}
func baselineText(s string, limit int) bool {
	if len(s) > limit || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func baselineStats(data json.RawMessage) (*baselineStatistics, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var w struct {
		Available  *int `json:"totalAvailableJobs"`
		Acquired   *int `json:"totalAcquiredJobs"`
		Assigned   *int `json:"totalAssignedJobs"`
		Running    *int `json:"totalRunningJobs"`
		Registered *int `json:"totalRegisteredRunners"`
		Busy       *int `json:"totalBusyRunners"`
		Idle       *int `json:"totalIdleRunners"`
	}
	if DecodeStrict(data, &w) != nil {
		return nil, ErrRemote
	}
	return &baselineStatistics{w.Available, w.Acquired, w.Assigned, w.Running, w.Registered, w.Busy, w.Idle}, nil
}
func (s *baselineStatistics) eligible(acquired bool) bool {
	if s == nil {
		return false
	}
	for _, p := range []*int{s.Available, s.Acquired, s.Assigned, s.Running, s.Registered, s.Busy, s.Idle} {
		if p == nil || *p < 0 || *p > 1 {
			return false
		}
	}
	return acquired || (*s.Acquired == 0 && *s.Running == 0 && *s.Registered == 0 && *s.Busy == 0 && *s.Idle == 0)
}
func (s *baselineStatistics) matches(sdk *scaleset.RunnerScaleSetStatistic) bool {
	if s == nil || sdk == nil {
		return s == nil && sdk == nil
	}
	got := []int{sdk.TotalAvailableJobs, sdk.TotalAcquiredJobs, sdk.TotalAssignedJobs, sdk.TotalRunningJobs, sdk.TotalRegisteredRunners, sdk.TotalBusyRunners, sdk.TotalIdleRunners}
	for i, p := range []*int{s.Available, s.Acquired, s.Assigned, s.Running, s.Registered, s.Busy, s.Idle} {
		if p != nil && *p != got[i] {
			return false
		}
	}
	return true
}
func decodeBaselineItem(data []byte, index int) (baselineItem, error) {
	var w struct {
		Kind        string          `json:"messageType"`
		RequestID   int64           `json:"runnerRequestId"`
		JobID       string          `json:"jobId"`
		Owner       *string         `json:"ownerName"`
		Repository  *string         `json:"repositoryName"`
		RunID       *int64          `json:"workflowRunId"`
		Event       *string         `json:"eventName"`
		WorkflowRef *string         `json:"jobWorkflowRef"`
		RunnerID    *sdkRunnerID    `json:"runnerId"`
		RunnerName  *string         `json:"runnerName"`
		Result      *string         `json:"result"`
		Labels      []string        `json:"requestLabels"`
		QueueTime   *time.Time      `json:"queueTime"`
		AssignTime  *time.Time      `json:"scaleSetAssignTime"`
		RunnerTime  *time.Time      `json:"runnerAssignTime"`
		FinishTime  *time.Time      `json:"finishTime"`
		AcquireURL  json.RawMessage `json:"acquireJobUrl"`
		Display     json.RawMessage `json:"jobDisplayName"`
	}
	if string(data) == "null" || DecodeStrict(data, &w) != nil {
		return baselineItem{Index: index, Kind: "invalid"}, ErrRemote
	}
	x := baselineItem{index, w.Kind, w.RequestID, w.JobID, w.Owner, w.Repository, w.RunID, w.Event, w.WorkflowRef, w.RunnerID, w.RunnerName, w.Result, w.Labels, w.QueueTime, w.AssignTime, w.RunnerTime, w.FinishTime}
	if !x.bounded() {
		return baselineItem{Index: index, Kind: "invalid"}, ErrRemote
	}
	if w.Kind != "JobAvailable" && w.Kind != "JobAssigned" && w.Kind != "JobStarted" && w.Kind != "JobCompleted" {
		x.Kind = "unknown"
		return x, ErrRemote
	}
	// These ignored fields are still required to have their pinned SDK wire type.
	for _, raw := range []json.RawMessage{w.AcquireURL, w.Display} {
		if len(raw) > 0 && string(raw) != "null" {
			var ignored string
			if json.Unmarshal(raw, &ignored) != nil {
				return x, ErrRemote
			}
		}
	}
	return x, nil
}
func (x baselineItem) bounded() bool {
	if !baselineText(x.JobID, 256) || len(x.Labels) > 8 {
		return false
	}
	for _, p := range []*string{x.Owner, x.Repository, x.Event, x.WorkflowRef, x.RunnerName, x.Result} {
		if p != nil && !baselineText(*p, 512) {
			return false
		}
	}
	for _, s := range x.Labels {
		if !baselineText(s, 128) {
			return false
		}
	}
	return true
}
func decodeBaselineBatch(data []byte) (*baselineBatch, error) {
	var w struct {
		ID    int             `json:"messageId"`
		Type  string          `json:"messageType"`
		Body  *string         `json:"body"`
		Stats json.RawMessage `json:"statistics"`
	}
	if DecodeStrict(data, &w) != nil || w.Type != "RunnerScaleSetJobMessages" || w.Body == nil {
		return nil, ErrRemote
	}
	s, err := baselineStats(w.Stats)
	if err != nil {
		return nil, err
	}
	b := &baselineBatch{MessageID: w.ID, Statistics: s}
	var items []json.RawMessage
	if strings.TrimSpace(*w.Body) == "null" || json.Unmarshal([]byte(*w.Body), &items) != nil || len(items) > 4 {
		return b, ErrRemote
	}
	var invalid bool
	for i, raw := range items {
		x, e := decodeBaselineItem(raw, i)
		if e != nil {
			invalid = true
		}
		b.Items = append(b.Items, x)
	}
	if invalid {
		return b, ErrRemote
	}
	return b, nil
}
func decodeBaselineSession(data []byte) (*baselineSessionFacts, error) {
	var w struct {
		ID    string          `json:"sessionId"`
		Owner string          `json:"ownerName"`
		Stats json.RawMessage `json:"statistics"`
		Set   json.RawMessage `json:"runnerScaleSet"`
		URL   string          `json:"messageQueueUrl"`
		Token string          `json:"messageQueueAccessToken"`
	}
	if DecodeStrict(data, &w) != nil || !baselineText(w.ID, 36) || !baselineText(w.Owner, 128) {
		return nil, ErrRemote
	}
	s, err := baselineStats(w.Stats)
	if err != nil {
		return nil, err
	}
	x := &baselineSessionFacts{SessionID: w.ID, Owner: w.Owner, Statistics: s}
	if len(w.Set) > 0 && string(w.Set) != "null" {
		// Set contains SDK-defined fields not used here; ambiguity is rejected,
		// but unrelated forward-compatible set metadata is not copied to history.
		var set struct {
			ID    int             `json:"id"`
			Name  string          `json:"name"`
			Group int             `json:"runnerGroupId"`
			Stats json.RawMessage `json:"statistics"`
		}
		if !uniqueKeys(json.NewDecoder(strings.NewReader(string(w.Set)))) || json.Unmarshal(w.Set, &set) != nil || !baselineText(set.Name, 128) {
			return x, ErrRemote
		}
		x.NestedSet = true
		x.SetID = set.ID
		x.SetName = set.Name
		x.GroupID = set.Group
		x.NestedStatistics, err = baselineStats(set.Stats)
		if err != nil {
			return x, err
		}
	}
	return x, nil
}

// Compare SDK values to captured wire facts without inventing presence for its
// zero-valued fields. The original type partition preserves order within a type.
func (b *baselineBatch) matches(m *scaleset.RunnerScaleSetMessage) bool {
	if b == nil || m == nil || b.MessageID != m.MessageID || !b.Statistics.matches(m.Statistics) {
		return false
	}
	byKind := map[string][]any{}
	for _, x := range m.JobAvailableMessages {
		byKind["JobAvailable"] = append(byKind["JobAvailable"], x)
	}
	for _, x := range m.JobAssignedMessages {
		byKind["JobAssigned"] = append(byKind["JobAssigned"], x)
	}
	for _, x := range m.JobStartedMessages {
		byKind["JobStarted"] = append(byKind["JobStarted"], x)
	}
	for _, x := range m.JobCompletedMessages {
		byKind["JobCompleted"] = append(byKind["JobCompleted"], x)
	}
	for _, x := range b.Items {
		values := byKind[x.Kind]
		if len(values) == 0 {
			return false
		}
		byKind[x.Kind] = values[1:]
		data, err := json.Marshal(values[0])
		if err != nil {
			return false
		}
		got, err := decodeBaselineItem(data, x.Index)
		if err != nil {
			return false
		}
		if !baselineSDKItemMatches(x, got) {
			return false
		}

	}
	for _, v := range byKind {
		if len(v) != 0 {
			return false
		}
	}
	return true
}

// SDK struct fields lose wire presence. Its zero-filled value must nevertheless
// agree with every captured fact; a newly populated callback field is a change.
func baselineValue[T comparable](p *T) T {
	if p != nil {
		return *p
	}
	var zero T
	return zero
}
func baselineSDKItemMatches(x, got baselineItem) bool {
	if x.Index != got.Index || x.Kind != got.Kind || x.RequestID != got.RequestID || x.JobID != got.JobID || baselineValue(x.RunID) != baselineValue(got.RunID) || baselineValue(x.RunnerID) != baselineValue(got.RunnerID) || !slices.Equal(x.Labels, got.Labels) {
		return false
	}
	for _, p := range [][2]*string{{x.Owner, got.Owner}, {x.Repository, got.Repository}, {x.Event, got.Event}, {x.WorkflowRef, got.WorkflowRef}, {x.RunnerName, got.RunnerName}, {x.Result, got.Result}} {
		if baselineValue(p[0]) != baselineValue(p[1]) {
			return false
		}
	}
	for _, p := range [][2]*time.Time{{x.QueueTime, got.QueueTime}, {x.AssignTime, got.AssignTime}, {x.RunnerTime, got.RunnerTime}, {x.FinishTime, got.FinishTime}} {
		if !baselineValue(p[0]).Equal(baselineValue(p[1])) {
			return false
		}
	}
	return true
}
