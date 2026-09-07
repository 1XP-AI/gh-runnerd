package livecanary

import (
	"context"
	"errors"
	"testing"
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
