package livecanary

import (
	"context"
	"errors"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
)

// Compiled red checkpoint: the SDK's typed listener boundary alone does not
// retain whole-message evidence or validate the raw acquisition count.
type baselineAcquisition struct{ RequestID int64 }
type baselineListener struct {
	ctx      context.Context
	approval Approval
	journal  *FileJournal
	api      *SDKAPI
	setID    int
	session  Session
	after    func(context.Context, baselineAcquisition) error
}

var errBaselineCollected = errors.New("baseline callback collection complete")

func newBaselineListenerHeld(ctx context.Context, a Approval, j *FileJournal, api *SDKAPI, setID int) (*baselineListener, error) {
	return &baselineListener{ctx: ctx, approval: a, journal: j, api: api, setID: setID}, nil
}
func (b *baselineListener) run(after func(context.Context, baselineAcquisition) error) error {
	b.after = after
	d := Driver{b.approval, b.journal, b.api}
	if err := d.effect(b.ctx, "session-open", nil, func(ctx context.Context) (Event, error) {
		var err error
		b.session, err = b.api.OpenSession(ctx, b.setID, b.approval.setName())
		if err != nil {
			return Event{}, err
		}
		return Event{SessionID: b.session.Session().SessionID.String()}, nil
	}); err != nil {
		return err
	}
	l, err := listener.New(b, listener.Config{ScaleSetID: b.setID, MaxRunners: 1})
	if err != nil {
		return ErrQuarantine
	}
	if err = l.Run(b.ctx, b); errors.Is(err, errBaselineCollected) {
		return nil
	}
	return ErrQuarantine
}
func (b *baselineListener) Session() scaleset.RunnerScaleSetSession { return b.session.Session() }
func (b *baselineListener) GetMessage(_ context.Context, last, capacity int) (*scaleset.RunnerScaleSetMessage, error) {
	m, err := b.session.GetMessage(b.ctx, last, capacity)
	if err == nil && m != nil {
		err = b.journal.Append(Event{Kind: "observation", Operation: "poll", ID: m.MessageID})
	}
	return m, err
}
func (b *baselineListener) DeleteMessage(_ context.Context, id int) error {
	return b.session.DeleteMessage(b.ctx, id)
}
func (b *baselineListener) AcquireJobs(_ context.Context, ids []int64) ([]int64, error) {
	got, err := b.session.AcquireJobs(b.ctx, ids)
	if err == nil && len(got) == 1 {
		err = b.after(b.ctx, baselineAcquisition{RequestID: got[0]})
	}
	return got, err
}
func (b *baselineListener) HandleJobStarted(context.Context, *scaleset.JobStarted) error { return nil }
func (b *baselineListener) HandleJobCompleted(context.Context, *scaleset.JobCompleted) error {
	return errBaselineCollected
}
func (b *baselineListener) HandleDesiredRunnerCount(_ context.Context, n int) (int, error) {
	return min(n, 1), nil
}
