package livecanary

import (
	"context"
	"errors"
	"testing"

	"github.com/actions/scaleset"
)

func TestAuditPR25ObservedJobsMustBlockCleanup(t *testing.T) {
	for _, phase := range []string{"before-ack", "after-ack", "before-acquire"} {
		t.Run(phase, func(t *testing.T) {
			d, f, j := created(t)
			if err := d.Run(context.Background(), phase); err != nil {
				t.Fatal("fixture barrier failed")
			}
			observed := false
			for _, e := range j.Events() {
				if e.Kind == "observation" && e.Operation == "poll" && len(e.RequestIDs) > 0 {
					observed = true
				}
			}
			if !observed {
				t.Fatal("fixture did not observe a job")
			}
			err := d.Run(context.Background(), "cleanup")
			if !errors.Is(err, ErrQuarantine) || f.deleteCalls != 0 {
				t.Fatalf("unresolved observed job allowed cleanup: error=%v delete_calls=%d", err, f.deleteCalls)
			}
		})
	}
}

func TestUnexpectedWorkMessageQuarantinesBeforeSafeClose(t *testing.T) {
	for _, shape := range []string{"started", "assigned", "completed", "empty", "nil"} {
		t.Run(shape, func(t *testing.T) {
			d, f, j := created(t)
			f.session.message.JobAvailableMessages = nil
			switch shape {
			case "empty":
				f.session.message.Statistics = &scaleset.RunnerScaleSetStatistic{}
			case "started":
				f.session.message.JobStartedMessages = []*scaleset.JobStarted{{}}
			case "assigned":
				f.session.message.JobAssignedMessages = []*scaleset.JobAssigned{{}}
			case "completed":
				f.session.message.JobCompletedMessages = []*scaleset.JobCompleted{{}}
			case "nil":
				f.session.message = nil
			}
			err := d.Run(context.Background(), "before-ack")
			if shape == "empty" || shape == "nil" {
				if !errors.Is(err, ErrNoMessage) || f.session.close != 1 {
					t.Fatal("empty queue did not retain no-message behavior")
				}
				return
			}
			if !errors.Is(err, ErrQuarantine) || f.session.close != 0 || !replay(j.Events()).uncertain {
				t.Errorf("unexpected work was treated as empty: error=%v close=%d", err, f.session.close)
			}
			if d.Run(context.Background(), "cleanup") == nil || f.deleteCalls != 0 {
				t.Error("unexpected work followed by stale zero permitted cleanup")
			}
		})
	}
}

func TestEmptyAvailableWithWorkStatisticsStaysQuarantined(t *testing.T) {
	for _, statistics := range []scaleset.RunnerScaleSetStatistic{
		{TotalRunningJobs: 1}, {TotalAssignedJobs: 1}, {TotalAcquiredJobs: 1},
		{TotalAvailableJobs: 1}, {TotalRegisteredRunners: 1}, {TotalBusyRunners: 1}, {TotalIdleRunners: 1},
	} {
		d, f, j := created(t)
		f.session.message.JobAvailableMessages = nil
		f.session.message.Statistics = &statistics
		if err := d.Run(context.Background(), "before-ack"); !errors.Is(err, ErrQuarantine) || f.session.close != 0 || !replay(j.Events()).uncertain {
			t.Errorf("nonzero work statistics took empty-poll safe close: error=%v close=%d", err, f.session.close)
		}
		if d.Run(context.Background(), "cleanup") == nil || f.deleteCalls != 0 {
			t.Error("positive work evidence was cleared by a stale-zero cleanup snapshot")
		}
	}
}

func TestAuditPR25MultiJobAcquisitionMustRefuseBeforeACK(t *testing.T) {
	for _, phase := range []string{"before-acquire", "acquire-loss"} {
		t.Run(phase, func(t *testing.T) {
			d, f, _ := created(t)
			second := *f.session.message.JobAvailableMessages[0]
			second.RunnerRequestID++
			f.session.message.JobAvailableMessages = append(f.session.message.JobAvailableMessages, &second)
			f.session.message.Statistics.TotalAssignedJobs = 2
			err := d.Run(context.Background(), phase)
			if err == nil || f.session.ack != 0 || f.session.acquire != 0 {
				t.Fatalf("multi-job acquisition was not refused before ACK: error=%v ack=%d acquire=%d", err, f.session.ack, f.session.acquire)
			}
		})
	}
}
