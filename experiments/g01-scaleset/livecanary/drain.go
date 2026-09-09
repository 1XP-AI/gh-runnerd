package livecanary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
)

const (
	drainObservationVersion = 1
	drainInitialCapacity    = 1
	drainWithdrawnCapacity  = 0

	drainBoundaryRequestWritten      = "request-written"
	drainBoundaryResponseBeforeWrite = "response-before-request-written"
	drainBoundaryUnresolved          = "unresolved"
	drainServerReceiptUnproven       = "unproven"
	drainMessagePresent              = "present"
	drainMessageAbsent               = "absent"
	drainMessageUnknown              = "unknown"
	drainResponseNotAttempted        = "not-attempted"
	drainResponseSucceeded           = "succeeded"
	drainResponseUnknown             = "unknown"
	drainOutcomeObserved             = "observed"
	drainOutcomeInconclusive         = "inconclusive"
	drainMarkerPrerequisiteFailed    = "prerequisite-failed"
	drainMarkerCancelled             = "cancelled"
	drainMarkerDeadline              = "deadline"
	drainMarkerQuarantine            = "quarantine"
)

var errDrainCollected = errors.New("drain observation collected")

// drainStatistics is deliberately a closed, scalar schema. It contains no
// response text, queue URL, token or SDK object, and every value must be
// non-negative before it is persisted.
type drainStatistics struct {
	Available  int `json:"total_available_jobs"`
	Acquired   int `json:"total_acquired_jobs"`
	Assigned   int `json:"total_assigned_jobs"`
	Running    int `json:"total_running_jobs"`
	Registered int `json:"total_registered_runners"`
	Busy       int `json:"total_busy_runners"`
	Idle       int `json:"total_idle_runners"`
}

func newDrainStatistics(s *scaleset.RunnerScaleSetStatistic) (drainStatistics, error) {
	if s == nil || s.TotalAvailableJobs < 0 || s.TotalAcquiredJobs < 0 || s.TotalAssignedJobs < 0 || s.TotalRunningJobs < 0 || s.TotalRegisteredRunners < 0 || s.TotalBusyRunners < 0 || s.TotalIdleRunners < 0 {
		return drainStatistics{}, ErrQuarantine
	}
	result := drainStatistics{Available: s.TotalAvailableJobs, Acquired: s.TotalAcquiredJobs, Assigned: s.TotalAssignedJobs, Running: s.TotalRunningJobs, Registered: s.TotalRegisteredRunners, Busy: s.TotalBusyRunners, Idle: s.TotalIdleRunners}
	if !validKnownDrainStatistics(result) {
		return drainStatistics{}, ErrQuarantine
	}
	return result, nil
}

func validKnownDrainStatistics(s drainStatistics) bool {
	if s.Available < 0 || s.Acquired < 0 || s.Assigned < 0 || s.Running < 0 || s.Registered < 0 || s.Busy < 0 || s.Idle < 0 {
		return false
	}
	// The service does not expose a separate runner state. A known snapshot is
	// only useful when its bounded runner partition is internally consistent.
	// Avoid Busy+Idle integer overflow by comparing one operand to the
	// subtraction result.
	return s.Busy <= s.Registered && s.Idle <= s.Registered-s.Busy && s.Busy+s.Idle == s.Registered
}

type drainPollObservation struct {
	Capacity    int             `json:"capacity"`
	Message     string          `json:"message"`
	Statistics  drainStatistics `json:"statistics"`
	StatsKnown  bool            `json:"statistics_known"`
	ACK         string          `json:"ack"`
	Acquisition string          `json:"acquisition"`
}

var drainStatisticsFields = [...]string{
	"totalAvailableJobs",
	"totalAcquiredJobs",
	"totalAssignedJobs",
	"totalRunningJobs",
	"totalRegisteredRunners",
	"totalBusyRunners",
	"totalIdleRunners",
}

// drainStatisticsFromBody preserves only field presence and bounded scalar
// values at the SDK adapter boundary. The pinned SDK decodes an empty JSON
// object into a non-nil all-zero struct, so a pointer alone is not evidence
// that the service supplied a complete statistics sample.
func drainStatisticsFromBody(body []byte) (drainStatistics, bool) {
	if !uniqueKeys(json.NewDecoder(bytes.NewReader(body))) {
		return drainStatistics{}, false
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		return drainStatistics{}, false
	}
	raw, ok := envelope["statistics"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return drainStatistics{}, false
	}
	if !uniqueKeys(json.NewDecoder(bytes.NewReader(raw))) {
		return drainStatistics{}, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return drainStatistics{}, false
	}
	if len(fields) < len(drainStatisticsFields) {
		return drainStatistics{}, false
	}
	values := make([]int, len(drainStatisticsFields))
	for i, field := range drainStatisticsFields {
		rawValue, ok := fields[field]
		if !ok || bytes.Equal(bytes.TrimSpace(rawValue), []byte("null")) || json.Unmarshal(rawValue, &values[i]) != nil {
			return drainStatistics{}, false
		}
	}
	stats := drainStatistics{
		Available:  values[0],
		Acquired:   values[1],
		Assigned:   values[2],
		Running:    values[3],
		Registered: values[4],
		Busy:       values[5],
		Idle:       values[6],
	}
	if !validKnownDrainStatistics(stats) {
		return drainStatistics{}, false
	}
	return stats, true
}

type drainSetIdentity struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	RunnerGroupID int    `json:"runner_group_id"`
	Label         string `json:"label"`
}

// drainPhaseIdentity is the approved owner boundary derived from the current
// approval. Replay must compare durable snapshots to this identity instead of
// treating a candidate snapshot's own metadata as its expected authority.
type drainPhaseIdentity struct {
	Set        drainSetIdentity `json:"set"`
	RunnerName string           `json:"runner_name"`
}

type drainRunnerIdentity struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	ScaleSetID int    `json:"scale_set_id"`
}

type drainSnapshot struct {
	Set        drainSetIdentity     `json:"set"`
	Statistics drainStatistics      `json:"statistics"`
	StatsKnown bool                 `json:"statistics_known"`
	Runner     *drainRunnerIdentity `json:"runner,omitempty"`
}

// drainObservation is private journal evidence. Its JSON shape is bounded and
// contains only fixed categories, identities and non-negative counters.
type drainObservation struct {
	Version           int                  `json:"version"`
	Outcome           string               `json:"outcome"`
	InitialCapacity   int                  `json:"initial_capacity"`
	WithdrawnCapacity int                  `json:"withdrawn_capacity"`
	Boundary          string               `json:"boundary"`
	ServerReceipt     string               `json:"server_receipt"`
	ResponseHeld      bool                 `json:"response_held"`
	Poll              drainPollObservation `json:"poll"`
	NextPoll          drainPollObservation `json:"next_poll"`
	Before            drainSnapshot        `json:"before"`
	After             drainSnapshot        `json:"after"`
	Ordering          []string             `json:"ordering"`
	Sequence          int                  `json:"sequence"`
	ObservedAt        time.Time            `json:"observed_at"`
}

func approvedDrainPhaseIdentity(a Approval, setID int) drainPhaseIdentity {
	name := a.setName()
	return drainPhaseIdentity{Set: drainSetIdentity{ID: setID, Name: name, RunnerGroupID: a.RunnerGroupID, Label: name}, RunnerName: a.workerName()}
}

func validDrainPhaseIdentity(p drainPhaseIdentity) bool {
	return p.Set.ID > 0 && p.Set.RunnerGroupID > 0 && p.Set.Name != "" && p.Set.Label != "" && baselineText(p.Set.Name, 128) && baselineText(p.Set.Label, 128) && p.RunnerName != "" && baselineText(p.RunnerName, 256)
}

func validDrainResponse(value string) bool {
	return value == drainResponseNotAttempted || value == drainResponseSucceeded || value == drainResponseUnknown
}

func validDrainPoll(p drainPollObservation, wantCapacity int) bool {
	if p.Capacity != wantCapacity || p.Message != drainMessagePresent && p.Message != drainMessageAbsent && p.Message != drainMessageUnknown || !validDrainResponse(p.ACK) || !validDrainResponse(p.Acquisition) {
		return false
	}
	if !p.StatsKnown && p.Statistics != (drainStatistics{}) {
		return false
	}
	return !p.StatsKnown || validKnownDrainStatistics(p.Statistics)
}

func validDrainSnapshot(s drainSnapshot, expected drainSetIdentity) bool {
	if s.Set != expected || s.Set.ID <= 0 || s.Set.RunnerGroupID <= 0 || s.Set.Name == "" || s.Set.Label == "" || !baselineText(s.Set.Name, 128) || !baselineText(s.Set.Label, 128) {
		return false
	}
	if !s.StatsKnown || !validKnownDrainStatistics(s.Statistics) {
		return false
	}
	if s.Runner != nil && (s.Runner.ID <= 0 || s.Runner.ScaleSetID != s.Set.ID || s.Runner.Name == "" || !baselineText(s.Runner.Name, 256)) {
		return false
	}
	return true
}

func sameDrainRunner(before, after *drainRunnerIdentity) bool {
	return before != nil && after != nil && *before == *after
}

func sameDrainRunnerPartition(left, right drainStatistics) bool {
	return left.Registered == right.Registered && left.Busy == right.Busy && left.Idle == right.Idle
}

func validDrainObservation(o *drainObservation) bool {
	if o == nil || o.Version != drainObservationVersion || (o.Outcome != drainOutcomeObserved && o.Outcome != drainOutcomeInconclusive) || o.InitialCapacity != drainInitialCapacity || o.WithdrawnCapacity != drainWithdrawnCapacity || o.ServerReceipt != drainServerReceiptUnproven || o.Sequence <= 0 || o.ObservedAt.IsZero() || len(o.Ordering) > 8 {
		return false
	}
	if o.Boundary != drainBoundaryRequestWritten && o.Boundary != drainBoundaryResponseBeforeWrite && o.Boundary != drainBoundaryUnresolved || !validDrainPoll(o.Poll, drainInitialCapacity) || !validDrainPoll(o.NextPoll, drainWithdrawnCapacity) || !validDrainSnapshot(o.Before, o.Before.Set) || !validDrainSnapshot(o.After, o.Before.Set) {
		return false
	}
	if o.Outcome == drainOutcomeObserved && (o.Boundary != drainBoundaryRequestWritten || !o.ResponseHeld) {
		return false
	}
	if o.Outcome == drainOutcomeObserved && !sameDrainRunner(o.Before.Runner, o.After.Runner) {
		return false
	}
	if o.Outcome == drainOutcomeObserved && !validDrainIdlePrerequisite(o.Before) {
		return false
	}
	if o.Outcome == drainOutcomeObserved && !sameDrainRunnerPartition(o.Poll.Statistics, o.Before.Statistics) {
		return false
	}
	if o.Outcome == drainOutcomeObserved && (!sameDrainRunnerPartition(o.NextPoll.Statistics, o.Before.Statistics) || !sameDrainRunnerPartition(o.After.Statistics, o.Before.Statistics)) {
		return false
	}
	seen := map[string]bool{}
	for _, item := range o.Ordering {
		if item != "poll-old" && item != "ack" && item != "acquire" && item != "poll-zero" || seen[item] {
			return false
		}
		seen[item] = true
	}
	if o.Outcome == drainOutcomeObserved {
		if o.Poll.Message != drainMessagePresent || !o.Poll.StatsKnown || o.Poll.Statistics == (drainStatistics{}) || o.NextPoll.Message != drainMessageAbsent || !o.NextPoll.StatsKnown || o.NextPoll.ACK != drainResponseNotAttempted || o.NextPoll.Acquisition != drainResponseNotAttempted {
			return false
		}
		if o.Poll.ACK != drainResponseSucceeded || o.Poll.Acquisition != drainResponseSucceeded {
			return false
		}
		want := []string{"poll-old"}
		want = append(want, "ack", "acquire")
		want = append(want, "poll-zero")
		if !slices.Equal(o.Ordering, want) {
			return false
		}
	}
	return true
}

func (o *drainObservation) poll(index int) *drainPollObservation {
	if index == 1 {
		return &o.Poll
	}
	return &o.NextPoll
}

// drainPollHook is installed only in the experiment's SDK client. WroteRequest
// is a client-side transport fact; it does not prove GitHub accepted the
// request. The first response body is held before it reaches the SDK parser so
// the listener's capacity transition and ACK/acquisition order remain visible.
type drainPollHook struct {
	inner    http.RoundTripper
	target   string
	wrote    chan struct{}
	response chan struct{}
	release  chan struct{}

	mu                  sync.Mutex
	wroteRequest        bool
	responseBeforeWrite bool
	responseHeld        bool
	invalid             bool
	pollAttempts        int
	statistics          [3]drainStatistics
	statisticsKnown     [3]bool
	batches             [3]*baselineBatch
	batchesKnown        [3]bool
	wroteCallbacks      [3]int
	onRequestWritten    func()
	wroteOnce           sync.Once
	responseOnce        sync.Once
	releaseOnce         sync.Once
}

func newDrainPollHook(target string) *drainPollHook {
	return &drainPollHook{inner: http.DefaultTransport, target: target, wrote: make(chan struct{}), response: make(chan struct{}), release: make(chan struct{})}
}

func (h *drainPollHook) matches(req *http.Request) bool {
	if h == nil || req == nil || req.Method != http.MethodGet || req.URL == nil {
		return false
	}
	h.mu.Lock()
	target := h.target
	h.mu.Unlock()
	want, err := url.Parse(target)
	if err != nil || want.Scheme == "" || want.Host == "" {
		return false
	}
	// Queue access tokens may be embedded in the URL. Compare only the
	// transport destination/path; never copy the query into an observation.
	return req.URL.Scheme == want.Scheme && req.URL.Host == want.Host && req.URL.Path == want.Path
}

func (h *drainPollHook) RoundTrip(req *http.Request) (*http.Response, error) {
	if h == nil || h.inner == nil || !h.matches(req) {
		if h == nil || h.inner == nil {
			return nil, ErrQuarantine
		}
		return h.inner.RoundTrip(req)
	}
	h.mu.Lock()
	h.pollAttempts++
	attempt := h.pollAttempts
	first := h.pollAttempts == 1
	h.mu.Unlock()
	if !first {
		h.mu.Lock()
		second := h.pollAttempts == 2
		if !second {
			h.invalid = true
		}
		h.mu.Unlock()
		if !second {
			return nil, ErrQuarantine
		}
		response, err := h.inner.RoundTrip(h.tracedRequest(req, attempt))
		if err != nil {
			return nil, err
		}
		if response == nil {
			h.mu.Lock()
			h.invalid = true
			h.mu.Unlock()
			return nil, ErrQuarantine
		}
		if response.Body == nil {
			response.Body = http.NoBody
		}
		response.Body = &drainObservedBody{
			source: response.Body,
			status: response.StatusCode,
			onComplete: func(stats drainStatistics, known bool, batch *baselineBatch, batchKnown bool) {
				h.mu.Lock()
				h.statistics[attempt] = stats
				h.statisticsKnown[attempt] = known
				h.batches[attempt] = batch
				h.batchesKnown[attempt] = batchKnown
				h.mu.Unlock()
			},
		}
		return response, nil
	}

	response, err := h.inner.RoundTrip(h.tracedRequest(req, attempt))
	if err != nil {
		h.wroteOnce.Do(func() { close(h.wrote) })
		h.responseOnce.Do(func() { close(h.response) })
		return nil, err
	}
	if response == nil {
		h.mu.Lock()
		h.invalid = true
		h.mu.Unlock()
		h.wroteOnce.Do(func() { close(h.wrote) })
		h.responseOnce.Do(func() { close(h.response) })
		return nil, ErrQuarantine
	}
	h.mu.Lock()
	if !h.wroteRequest {
		h.responseBeforeWrite = true
	}
	h.responseHeld = true
	h.mu.Unlock()
	if response.Body == nil {
		response.Body = http.NoBody
	}
	observedBody := &drainObservedBody{
		source: response.Body,
		status: response.StatusCode,
		onComplete: func(stats drainStatistics, known bool, batch *baselineBatch, batchKnown bool) {
			h.mu.Lock()
			if attempt >= 1 && attempt < len(h.statistics) {
				h.statistics[attempt] = stats
				h.statisticsKnown[attempt] = known
				h.batches[attempt] = batch
				h.batchesKnown[attempt] = batchKnown
			}
			h.mu.Unlock()
		},
	}
	response.Body = &drainHeldBody{source: observedBody, release: h.release}
	h.responseOnce.Do(func() { close(h.response) })
	return response, nil
}

func (h *drainPollHook) tracedRequest(req *http.Request, attempt int) *http.Request {
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		notify := false
		h.mu.Lock()
		if attempt < 1 || attempt >= len(h.wroteCallbacks) {
			h.invalid = true
		} else {
			h.wroteCallbacks[attempt]++
			notify = attempt == 1 && h.wroteCallbacks[attempt] == 1
			if h.wroteCallbacks[attempt] != 1 {
				h.invalid = true
			}
		}
		if info.Err != nil {
			h.invalid = true
		} else if attempt == 1 {
			h.wroteRequest = true
		}
		h.mu.Unlock()
		if notify && info.Err == nil && h.onRequestWritten != nil {
			h.onRequestWritten()
		}
		h.wroteOnce.Do(func() { close(h.wrote) })
	}}
	return req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
}

func (h *drainPollHook) pollWritesValid() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.invalid && h.wroteCallbacks[1] == 1 && h.wroteCallbacks[2] == 1
}

func (h *drainPollHook) pollStatistics(index int) (drainStatistics, bool) {
	if h == nil || index < 1 || index >= len(h.statistics) {
		return drainStatistics{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.statistics[index], h.statisticsKnown[index]
}

func (h *drainPollHook) pollBatch(index int) (*baselineBatch, bool) {
	if h == nil || index < 1 || index >= len(h.batches) {
		return nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.batches[index], h.batchesKnown[index]
}

// drainObservedBody forwards response bytes unchanged while retaining a
// bounded, ephemeral copy solely to establish statistics field presence and
// strict embedded job facts. It never persists, logs or rewrites the response
// payload.
type drainObservedBody struct {
	source     io.ReadCloser
	status     int
	data       []byte
	over       bool
	mu         sync.Mutex
	finished   bool
	onComplete func(drainStatistics, bool, *baselineBatch, bool)
}

func (b *drainObservedBody) capture(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := int(responseBodyLimit) - len(b.data)
	if remaining <= 0 {
		b.over = true
		return
	}
	if len(data) > remaining {
		b.data = append(b.data, data[:remaining]...)
		b.over = true
		return
	}
	b.data = append(b.data, data...)
}

func (b *drainObservedBody) finish() {
	b.mu.Lock()
	if b.finished {
		b.mu.Unlock()
		return
	}
	b.finished = true
	data := append([]byte(nil), b.data...)
	over := b.over
	b.data = nil
	b.mu.Unlock()

	stats, known := drainStatisticsFromBody(data)
	batch, batchErr := decodeBaselineBatch(data)
	batchKnown := batchErr == nil
	if b.status != http.StatusOK && b.status != http.StatusAccepted {
		known = false
		stats = drainStatistics{}
		batch = nil
		batchKnown = false
	}
	if over {
		known = false
		stats = drainStatistics{}
		batch = nil
		batchKnown = false
	}
	if b.onComplete != nil {
		b.onComplete(stats, known, batch, batchKnown)
	}
}

func (b *drainObservedBody) Read(p []byte) (int, error) {
	n, err := b.source.Read(p)
	if n > 0 {
		b.capture(p[:n])
	}
	if err != nil {
		b.finish()
	}
	return n, err
}

func (b *drainObservedBody) Close() error {
	b.mu.Lock()
	finished := b.finished
	remaining := int(responseBodyLimit) - len(b.data)
	b.mu.Unlock()
	if !finished {
		if remaining < 0 {
			remaining = 0
		}
		data, err := io.ReadAll(io.LimitReader(b.source, int64(remaining)+1))
		if len(data) > remaining {
			b.mu.Lock()
			b.over = true
			b.mu.Unlock()
		}
		if len(data) > 0 {
			b.capture(data)
		}
		if err != nil {
			b.mu.Lock()
			b.over = true
			b.mu.Unlock()
		}
		b.finish()
	}
	return b.source.Close()
}

func (h *drainPollHook) releaseResponse() {
	if h == nil {
		return
	}
	h.releaseOnce.Do(func() { close(h.release) })
}

func (h *drainPollHook) boundary() (string, bool, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.wroteRequest && h.responseHeld && !h.responseBeforeWrite:
		return drainBoundaryRequestWritten, true, !h.invalid && h.wroteCallbacks[1] == 1
	case h.responseBeforeWrite:
		return drainBoundaryResponseBeforeWrite, h.responseHeld, false
	default:
		return drainBoundaryUnresolved, h.responseHeld, false
	}
}

type drainHeldBody struct {
	source  ioReadCloser
	release <-chan struct{}
}

type ioReadCloser interface {
	Read([]byte) (int, error)
	Close() error
}

func (b *drainHeldBody) Read(p []byte) (int, error) {
	<-b.release
	return b.source.Read(p)
}
func (b *drainHeldBody) Close() error {
	<-b.release
	return b.source.Close()
}

// drainClient records only high-level listener effects and the two bounded
// polls. It never changes ACK or acquisition order.
type drainClient struct {
	inner                 listener.Client
	validate              func(context.Context, *scaleset.RunnerScaleSetMessage) error
	obs                   *drainObservation
	hook                  *drainPollHook
	phaseCtx              context.Context
	reject                func(string) error
	ownedRunnerStats      drainStatistics
	ownedRunnerStatsKnown bool
	mu                    sync.Mutex
	polls                 int
	messageID             int
	requestID             int64
}

type drainContextBinder interface {
	bindDrainContext(context.Context)
}

func (c *drainClient) bindDrainContext(ctx context.Context) {
	c.mu.Lock()
	c.phaseCtx = ctx
	c.mu.Unlock()
	if binder, ok := c.inner.(drainContextBinder); ok {
		binder.bindDrainContext(ctx)
	}
}

func (c *drainClient) active() bool {
	c.mu.Lock()
	phaseCtx := c.phaseCtx
	c.mu.Unlock()
	return phaseCtx == nil || phaseCtx.Err() == nil
}

func (c *drainClient) rejectCall(operation string) error {
	if c.reject != nil {
		if err := c.reject(operation); err != nil {
			return err
		}
	}
	return ErrQuarantine
}

func (c *drainClient) Session() scaleset.RunnerScaleSetSession { return c.inner.Session() }

func (c *drainClient) GetMessage(ctx context.Context, last, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	if !c.active() {
		return nil, c.rejectCall("observe-poll")
	}
	c.mu.Lock()
	wantCapacity := drainInitialCapacity
	wantLast := 0
	if c.polls == 1 {
		wantCapacity = drainWithdrawnCapacity
		wantLast = c.messageID
	}
	if c.polls >= 2 || capacity != wantCapacity || last != wantLast || c.polls == 1 && (c.messageID <= 0 || c.obs.poll(c.polls).ACK != drainResponseSucceeded) {
		c.mu.Unlock()
		return nil, ErrQuarantine
	}
	c.polls++
	index := c.polls
	c.obs.poll(index).Capacity = capacity
	c.obs.poll(index).Statistics = drainStatistics{}
	c.obs.poll(index).StatsKnown = false
	c.obs.poll(index).ACK = drainResponseNotAttempted
	c.obs.poll(index).Acquisition = drainResponseNotAttempted
	c.obs.Ordering = append(c.obs.Ordering, map[int]string{1: "poll-old", 2: "poll-zero"}[index])
	c.mu.Unlock()

	message, err := c.inner.GetMessage(ctx, last, capacity)
	stats, statsKnown := drainStatistics{}, false
	if c.hook == nil {
		if message != nil {
			var statsErr error
			stats, statsErr = newDrainStatistics(message.Statistics)
			statsKnown = statsErr == nil
		}
	} else {
		stats, statsKnown = c.hook.pollStatistics(index)
	}
	if err != nil {
		c.mu.Lock()
		c.obs.poll(index).Message = drainMessageUnknown
		c.obs.poll(index).Statistics = drainStatistics{}
		c.obs.poll(index).StatsKnown = false
		c.mu.Unlock()
		return nil, err
	}
	if message == nil {
		c.mu.Lock()
		if statsKnown && c.ownedRunnerStatsKnown && !sameDrainRunnerPartition(stats, c.ownedRunnerStats) {
			c.obs.poll(index).Message = drainMessageUnknown
			c.obs.poll(index).Statistics = drainStatistics{}
			c.obs.poll(index).StatsKnown = false
			c.mu.Unlock()
			return nil, ErrQuarantine
		}
		c.obs.poll(index).Message = drainMessageAbsent
		c.obs.poll(index).Statistics = stats
		c.obs.poll(index).StatsKnown = statsKnown
		c.mu.Unlock()
		return nil, nil
	}
	if !statsKnown || message.Statistics == nil {
		c.mu.Lock()
		c.obs.poll(index).Message = drainMessageUnknown
		c.obs.poll(index).Statistics = drainStatistics{}
		c.obs.poll(index).StatsKnown = false
		c.mu.Unlock()
		return nil, ErrQuarantine
	}
	if c.hook != nil {
		batch, batchKnown := c.hook.pollBatch(index)
		if !batchKnown || !batch.matches(message) {
			c.mu.Lock()
			c.obs.poll(index).Message = drainMessageUnknown
			c.mu.Unlock()
			return nil, ErrQuarantine
		}
	}
	messageStats, statsErr := newDrainStatistics(message.Statistics)
	if statsErr != nil || (c.hook != nil && stats != messageStats) || (index <= 2 && c.ownedRunnerStatsKnown && !sameDrainRunnerPartition(stats, c.ownedRunnerStats)) || message.MessageID <= 0 || len(message.JobAssignedMessages) != 0 || len(message.JobStartedMessages) != 0 || len(message.JobCompletedMessages) != 0 || len(message.JobAvailableMessages) != 1 || message.JobAvailableMessages[0] == nil || message.JobAvailableMessages[0].RunnerRequestID <= 0 {
		c.mu.Lock()
		c.obs.poll(index).Message = drainMessageUnknown
		c.mu.Unlock()
		return nil, ErrQuarantine
	}
	if index == 2 {
		// A message after withdrawal is retained as an unresolved race; do not
		// ACK or acquire a second message in this bounded phase.
		c.mu.Lock()
		c.obs.poll(index).Message = drainMessageUnknown
		c.mu.Unlock()
		return nil, ErrQuarantine
	}
	if c.validate != nil {
		if err := c.validate(ctx, message); err != nil {
			c.mu.Lock()
			c.obs.poll(index).Message = drainMessageUnknown
			c.mu.Unlock()
			return nil, err
		}
	}
	c.mu.Lock()
	c.obs.poll(index).Message = drainMessagePresent
	c.obs.poll(index).Statistics = stats
	c.obs.poll(index).StatsKnown = true
	c.messageID = message.MessageID
	if len(message.JobAvailableMessages) == 1 {
		c.requestID = message.JobAvailableMessages[0].RunnerRequestID
	}
	c.mu.Unlock()
	return message, nil
}

func (c *drainClient) DeleteMessage(ctx context.Context, id int) error {
	if !c.active() {
		return c.rejectCall("ack")
	}
	c.mu.Lock()
	index := c.polls
	valid := index == 1 && c.messageID > 0 && id == c.messageID && c.obs.poll(index).ACK == drainResponseNotAttempted
	c.mu.Unlock()
	if !valid {
		return c.rejectCall("ack")
	}
	err := c.inner.DeleteMessage(ctx, id)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.obs.poll(index).ACK = drainResponseSucceeded
	c.obs.Ordering = append(c.obs.Ordering, "ack")
	if err != nil {
		c.obs.poll(index).ACK = drainResponseUnknown
	}
	return err
}

func (c *drainClient) AcquireJobs(ctx context.Context, ids []int64) ([]int64, error) {
	if !c.active() {
		return nil, c.rejectCall("acquire")
	}
	c.mu.Lock()
	index := c.polls
	valid := index == 1 && len(ids) == 1 && c.requestID > 0 && ids[0] == c.requestID && c.obs.poll(index).ACK == drainResponseSucceeded && c.obs.poll(index).Acquisition == drainResponseNotAttempted
	c.mu.Unlock()
	if !valid {
		return nil, c.rejectCall("acquire")
	}
	got, err := c.inner.AcquireJobs(ctx, slices.Clone(ids))
	c.mu.Lock()
	defer c.mu.Unlock()
	c.obs.poll(index).Acquisition = drainResponseSucceeded
	c.obs.Ordering = append(c.obs.Ordering, "acquire")
	if err != nil {
		c.obs.poll(index).Acquisition = drainResponseUnknown
		return nil, err
	}
	if !slices.Equal(got, ids) {
		c.obs.poll(index).Acquisition = drainResponseUnknown
		return nil, ErrQuarantine
	}
	return got, nil
}

type drainScaler struct{ client *drainClient }

func (s drainScaler) HandleDesiredRunnerCount(_ context.Context, count int) (int, error) {
	if count < 0 {
		return 0, ErrQuarantine
	}
	s.client.mu.Lock()
	polls := s.client.polls
	messageID := s.client.messageID
	s.client.mu.Unlock()
	if !s.client.active() {
		return 0, s.client.rejectCall("observe-poll")
	}
	if polls >= 2 || polls >= 1 && messageID == 0 {
		return 0, errDrainCollected
	}
	return min(count, drainInitialCapacity), nil
}
func (drainScaler) HandleJobStarted(context.Context, *scaleset.JobStarted) error {
	return ErrQuarantine
}
func (drainScaler) HandleJobCompleted(context.Context, *scaleset.JobCompleted) error {
	return ErrQuarantine
}

func runDrainListener(ctx context.Context, client listener.Client, setID int, hook *drainPollHook) (drainObservation, error) {
	obs := drainObservation{Version: drainObservationVersion, Outcome: drainOutcomeInconclusive, InitialCapacity: drainInitialCapacity, WithdrawnCapacity: drainWithdrawnCapacity, Boundary: drainBoundaryUnresolved, ServerReceipt: drainServerReceiptUnproven, Sequence: 1, ObservedAt: time.Now().UTC()}
	obs.Poll = drainPollObservation{Capacity: drainInitialCapacity, Message: drainMessageUnknown, ACK: drainResponseNotAttempted, Acquisition: drainResponseNotAttempted}
	obs.NextPoll = drainPollObservation{Capacity: drainWithdrawnCapacity, Message: drainMessageUnknown, ACK: drainResponseNotAttempted, Acquisition: drainResponseNotAttempted}
	if ctx == nil || client == nil || setID <= 0 || hook == nil {
		return obs, ErrApproval
	}
	hook.mu.Lock()
	target := hook.target
	hook.mu.Unlock()
	if target == "" {
		return obs, ErrApproval
	}
	initial := client.Session()
	if initial.SessionID == [16]byte{} || initial.Statistics == nil {
		return obs, ErrQuarantine
	}
	initialStats, err := newDrainStatistics(initial.Statistics)
	if err != nil {
		return obs, err
	}
	obs.Poll.Statistics = initialStats
	obs.Poll.StatsKnown = true
	c := &drainClient{inner: client, obs: &obs, hook: hook, ownedRunnerStats: initialStats, ownedRunnerStatsKnown: true}
	if rejecter, ok := client.(interface{ reject(string) error }); ok {
		c.reject = rejecter.reject
	}
	l, err := listener.New(c, listener.Config{ScaleSetID: setID, MaxRunners: drainInitialCapacity})
	if err != nil {
		return obs, ErrQuarantine
	}
	hook.onRequestWritten = func() { l.SetMaxRunners(drainWithdrawnCapacity) }
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	c.bindDrainContext(runCtx)
	runResult := make(chan error, 1)
	go func() { runResult <- l.Run(runCtx, drainScaler{client: c}) }()
	cancelAndJoin := func() {
		hook.releaseResponse()
		stop()
		<-runResult
	}

	release := false
	defer func() {
		if !release {
			hook.releaseResponse()
		}
	}()
	responseCaptured := false
	select {
	case <-hook.wrote:
		// The callback has already called the public listener method. This is
		// deliberately a client transport boundary, not server acceptance.
	case <-hook.response:
		// A response before the request-written marker is a timing miss. Release
		// the held body only so the listener can finish classifying the result;
		// never promote this path to an observed in-flight boundary. If both
		// channels are ready, retain a valid request-written boundary instead of
		// letting select's choice turn a valid run into a flaky skip.
		boundary, held, proven := hook.boundary()
		obs.Boundary, obs.ResponseHeld = boundary, held
		hook.releaseResponse()
		release = true
		if !proven {
			select {
			case err := <-runResult:
				if errors.Is(err, errDrainCollected) {
					obs.Outcome = drainOutcomeInconclusive
					return obs, ErrNoMessage
				}
				return obs, drainBoundaryError(&obs, hook, err)
			case <-ctx.Done():
				cancelAndJoin()
				return obs, ErrQuarantine
			}
		}
		responseCaptured = true
	case err := <-runResult:
		return obs, drainBoundaryError(&obs, hook, err)
	case <-ctx.Done():
		cancelAndJoin()
		return obs, ErrQuarantine
	}
	if !responseCaptured {
		select {
		case <-hook.response:
			boundary, held, proven := hook.boundary()
			obs.Boundary, obs.ResponseHeld = boundary, held
			if !proven {
				obs.Outcome = drainOutcomeInconclusive
			}
			hook.releaseResponse()
			release = true
		case err := <-runResult:
			return obs, drainBoundaryError(&obs, hook, err)
		case <-ctx.Done():
			cancelAndJoin()
			return obs, ErrQuarantine
		}
	}
	select {
	case err = <-runResult:
	case <-ctx.Done():
		cancelAndJoin()
		return obs, ErrQuarantine
	}
	if errors.Is(err, errDrainCollected) {
		if obs.Boundary == "" {
			obs.Boundary, obs.ResponseHeld, _ = hook.boundary()
		}
		if obs.Boundary == drainBoundaryRequestWritten && obs.ResponseHeld && hook.pollWritesValid() && obs.Poll.Message == drainMessagePresent && obs.Poll.StatsKnown && obs.Poll.Statistics != (drainStatistics{}) && c.ownedRunnerStatsKnown && sameDrainRunnerPartition(obs.Poll.Statistics, c.ownedRunnerStats) && obs.Poll.ACK == drainResponseSucceeded && obs.Poll.Acquisition == drainResponseSucceeded && obs.NextPoll.Message == drainMessageAbsent && obs.NextPoll.StatsKnown && sameDrainRunnerPartition(obs.NextPoll.Statistics, c.ownedRunnerStats) {
			obs.Outcome = drainOutcomeObserved
			return obs, nil
		}
		obs.Outcome = drainOutcomeInconclusive
		return obs, ErrNoMessage
	}
	return obs, drainBoundaryError(&obs, hook, err)
}

func drainBoundaryError(obs *drainObservation, hook *drainPollHook, err error) error {
	if obs != nil && hook != nil {
		obs.Boundary, obs.ResponseHeld, _ = hook.boundary()
	}
	if err == nil {
		return ErrNoMessage
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrQuarantine
	}
	if obs != nil && (obs.Boundary == drainBoundaryUnresolved || obs.Boundary == drainBoundaryResponseBeforeWrite) {
		return ErrNoMessage
	}
	return ErrQuarantine
}
