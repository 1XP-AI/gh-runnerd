package livecanary

import (
	"context"
	"errors"
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
	return drainStatistics{Available: s.TotalAvailableJobs, Acquired: s.TotalAcquiredJobs, Assigned: s.TotalAssignedJobs, Running: s.TotalRunningJobs, Registered: s.TotalRegisteredRunners, Busy: s.TotalBusyRunners, Idle: s.TotalIdleRunners}, nil
}

type drainPollObservation struct {
	Capacity    int             `json:"capacity"`
	Message     string          `json:"message"`
	Statistics  drainStatistics `json:"statistics"`
	StatsKnown  bool            `json:"statistics_known"`
	ACK         string          `json:"ack"`
	Acquisition string          `json:"acquisition"`
}

type drainSetIdentity struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	RunnerGroupID int    `json:"runner_group_id"`
	Label         string `json:"label"`
}

type drainRunnerIdentity struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	ScaleSetID int    `json:"scale_set_id"`
}

type drainSnapshot struct {
	Set        drainSetIdentity     `json:"set"`
	Statistics drainStatistics      `json:"statistics"`
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

func validDrainResponse(value string) bool {
	return value == drainResponseNotAttempted || value == drainResponseSucceeded || value == drainResponseUnknown
}

func validDrainPoll(p drainPollObservation, wantCapacity int) bool {
	if p.Capacity != wantCapacity || p.Message != drainMessagePresent && p.Message != drainMessageAbsent && p.Message != drainMessageUnknown || !validDrainResponse(p.ACK) || !validDrainResponse(p.Acquisition) {
		return false
	}
	if p.Statistics.Available < 0 || p.Statistics.Acquired < 0 || p.Statistics.Assigned < 0 || p.Statistics.Running < 0 || p.Statistics.Registered < 0 || p.Statistics.Busy < 0 || p.Statistics.Idle < 0 {
		return false
	}
	return true
}

func validDrainSnapshot(s drainSnapshot, expected drainSetIdentity) bool {
	if s.Set != expected || s.Set.ID <= 0 || s.Set.RunnerGroupID <= 0 || s.Set.Name == "" || s.Set.Label == "" || !baselineText(s.Set.Name, 128) || !baselineText(s.Set.Label, 128) {
		return false
	}
	if s.Statistics.Available < 0 || s.Statistics.Acquired < 0 || s.Statistics.Assigned < 0 || s.Statistics.Running < 0 || s.Statistics.Registered < 0 || s.Statistics.Busy < 0 || s.Statistics.Idle < 0 {
		return false
	}
	if s.Runner != nil && (s.Runner.ID <= 0 || s.Runner.ScaleSetID != s.Set.ID || s.Runner.Name == "" || !baselineText(s.Runner.Name, 256)) {
		return false
	}
	return true
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
	seen := map[string]bool{}
	for _, item := range o.Ordering {
		if item != "poll-old" && item != "ack" && item != "acquire" && item != "poll-zero" || seen[item] {
			return false
		}
		seen[item] = true
	}
	if o.Outcome == drainOutcomeObserved {
		if o.NextPoll.Message != drainMessageAbsent || o.NextPoll.ACK != drainResponseNotAttempted || o.NextPoll.Acquisition != drainResponseNotAttempted {
			return false
		}
		if o.Poll.Message == drainMessagePresent {
			if o.Poll.ACK != drainResponseSucceeded || (o.Poll.Acquisition != drainResponseNotAttempted && o.Poll.Acquisition != drainResponseSucceeded) {
				return false
			}
		} else if o.Poll.Message == drainMessageAbsent && (o.Poll.ACK != drainResponseNotAttempted || o.Poll.Acquisition != drainResponseNotAttempted) {
			return false
		} else if o.Poll.Message != drainMessageAbsent {
			return false
		}
		want := []string{"poll-old"}
		if o.Poll.Message == drainMessagePresent {
			want = append(want, "ack")
			if o.Poll.Acquisition == drainResponseSucceeded {
				want = append(want, "acquire")
			}
		}
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
	first := h.pollAttempts == 1
	h.mu.Unlock()
	if !first {
		return h.inner.RoundTrip(req)
	}

	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		h.mu.Lock()
		if info.Err != nil {
			h.invalid = true
		} else {
			h.wroteRequest = true
		}
		h.mu.Unlock()
		if info.Err == nil {
			h.wroteOnce.Do(func() {
				if h.onRequestWritten != nil {
					h.onRequestWritten()
				}
				close(h.wrote)
			})
		} else {
			h.wroteOnce.Do(func() { close(h.wrote) })
		}
	}}
	response, err := h.inner.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
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
	response.Body = &drainHeldBody{source: response.Body, release: h.release}
	h.responseOnce.Do(func() { close(h.response) })
	return response, nil
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
		return drainBoundaryRequestWritten, true, !h.invalid
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
func (b *drainHeldBody) Close() error { return b.source.Close() }

// drainClient records only high-level listener effects and the two bounded
// polls. It never changes ACK or acquisition order.
type drainClient struct {
	inner     listener.Client
	validate  func(context.Context, *scaleset.RunnerScaleSetMessage) error
	obs       *drainObservation
	mu        sync.Mutex
	polls     int
	messageID int
	requestID int64
}

func (c *drainClient) Session() scaleset.RunnerScaleSetSession { return c.inner.Session() }

func (c *drainClient) GetMessage(ctx context.Context, last, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	c.mu.Lock()
	if capacity != drainInitialCapacity && capacity != drainWithdrawnCapacity || c.polls >= 2 {
		c.mu.Unlock()
		return nil, ErrQuarantine
	}
	c.polls++
	index := c.polls
	c.obs.poll(index).Capacity = capacity
	c.obs.poll(index).ACK = drainResponseNotAttempted
	c.obs.poll(index).Acquisition = drainResponseNotAttempted
	c.obs.Ordering = append(c.obs.Ordering, map[int]string{1: "poll-old", 2: "poll-zero"}[index])
	c.mu.Unlock()

	message, err := c.inner.GetMessage(ctx, last, capacity)
	if err != nil {
		c.mu.Lock()
		c.obs.poll(index).Message = drainMessageUnknown
		c.mu.Unlock()
		return nil, err
	}
	if message == nil {
		c.mu.Lock()
		c.obs.poll(index).Message = drainMessageAbsent
		c.mu.Unlock()
		return nil, nil
	}
	stats, statsErr := newDrainStatistics(message.Statistics)
	if statsErr != nil || message.MessageID <= 0 || len(message.JobAssignedMessages) != 0 || len(message.JobStartedMessages) != 0 || len(message.JobCompletedMessages) != 0 || len(message.JobAvailableMessages) > 1 || len(message.JobAvailableMessages) == 1 && (message.JobAvailableMessages[0] == nil || message.JobAvailableMessages[0].RunnerRequestID <= 0) {
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
	err := c.inner.DeleteMessage(ctx, id)
	c.mu.Lock()
	defer c.mu.Unlock()
	index := c.polls
	if index < 1 || index > 2 || c.messageID <= 0 || id != c.messageID {
		return ErrQuarantine
	}
	c.obs.poll(index).ACK = drainResponseSucceeded
	c.obs.Ordering = append(c.obs.Ordering, "ack")
	if err != nil {
		c.obs.poll(index).ACK = drainResponseUnknown
	}
	return err
}

func (c *drainClient) AcquireJobs(ctx context.Context, ids []int64) ([]int64, error) {
	if len(ids) != 1 {
		return nil, ErrQuarantine
	}
	got, err := c.inner.AcquireJobs(ctx, slices.Clone(ids))
	c.mu.Lock()
	defer c.mu.Unlock()
	index := c.polls
	if index < 1 || index > 2 || c.requestID <= 0 || ids[0] != c.requestID {
		return nil, ErrQuarantine
	}
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
	s.client.mu.Unlock()
	if polls >= 2 {
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
	if stats, err := newDrainStatistics(initial.Statistics); err != nil {
		return obs, err
	} else {
		obs.Poll.Statistics = stats
		obs.Poll.StatsKnown = true
	}
	c := &drainClient{inner: client, obs: &obs}
	l, err := listener.New(c, listener.Config{ScaleSetID: setID, MaxRunners: drainInitialCapacity})
	if err != nil {
		return obs, ErrQuarantine
	}
	hook.onRequestWritten = func() { l.SetMaxRunners(drainWithdrawnCapacity) }
	runResult := make(chan error, 1)
	go func() { runResult <- l.Run(ctx, drainScaler{client: c}) }()

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
				return obs, ErrQuarantine
			}
		}
		responseCaptured = true
	case err := <-runResult:
		return obs, drainBoundaryError(&obs, hook, err)
	case <-ctx.Done():
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
			return obs, ErrQuarantine
		}
	}
	err = <-runResult
	if errors.Is(err, errDrainCollected) {
		if obs.Boundary == "" {
			obs.Boundary, obs.ResponseHeld, _ = hook.boundary()
		}
		if obs.Boundary == drainBoundaryRequestWritten && obs.ResponseHeld {
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
	if obs != nil && obs.Boundary == drainBoundaryUnresolved {
		return ErrNoMessage
	}
	return ErrQuarantine
}
