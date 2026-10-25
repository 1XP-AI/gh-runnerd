package livecanary

import (
	"context"
	"testing"
)

func TestBaselineV05HandlerValidatesDurableBatchBeforeACK(t *testing.T) {
	for _, fault := range []string{"valid", "id", "statistics", "request-id", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			f := newBaselineFixture(t, nil)
			b := baselineStepped(t, f)
			msg, err := b.GetMessage(context.Background(), 0, 1)
			if err != nil || msg == nil {
				t.Fatal("controlled poll failed")
			}
			switch fault {
			case "id":
				msg.MessageID++
			case "statistics":
				msg.Statistics.TotalAssignedJobs++
			case "request-id":
				msg.JobAvailableMessages[0].RunnerRequestID++
			case "canceled":
				b.cancel()
			}
			before := f.requests.Load()
			err = b.Scale(context.Background(), msg)
			if (err == nil) != (fault == "valid") || f.requests.Load() != before || f.acks.Load() != 0 || f.acquires.Load() != 0 || f.continuations.Load() != 0 {
				t.Fatal("pre-ACK handler bypassed durable validation or performed external work")
			}
		})
	}
}
