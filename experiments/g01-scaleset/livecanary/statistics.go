package livecanary

import (
	"context"

	"github.com/actions/scaleset"
)

const (
	workDemand     = "demand"
	workUnresolved = "unresolved"
)

// Demand is expected before a controlled probe but never proves empty cleanup.
// This no-worker fault harness cannot reconcile acquired/running work or any
// existing runner count. Missing required or negative statistics are unknown.
func statisticsWork(s *scaleset.RunnerScaleSetStatistic) string {
	if s == nil || s.TotalAvailableJobs < 0 || s.TotalAssignedJobs < 0 || s.TotalAcquiredJobs != 0 || s.TotalRunningJobs != 0 || s.TotalRegisteredRunners != 0 || s.TotalBusyRunners != 0 || s.TotalIdleRunners != 0 {
		return workUnresolved
	}
	if s.TotalAvailableJobs > 0 || s.TotalAssignedJobs > 0 {
		return workDemand
	}
	return ""
}

func combinedWork(first, second string) string {
	if first == workUnresolved || second == workUnresolved {
		return workUnresolved
	}
	if first == workDemand || second == workDemand {
		return workDemand
	}
	return ""
}

// Discovery never adopts the response. A nil object means absence of a result,
// not permission to erase earlier work evidence. If present, stats are required.
func (d *Driver) discover(ctx context.Context) (*scaleset.RunnerScaleSet, error) {
	var set *scaleset.RunnerScaleSet
	err := d.effect(ctx, "observe-discovery", nil, func(c context.Context) (Event, error) {
		var err error
		set, err = d.API.FindScaleSet(c, d.Approval.setName(), d.Approval.RunnerGroupID)
		if err != nil {
			return Event{}, err
		}
		if set == nil {
			return Event{}, nil
		}
		return Event{Work: statisticsWork(set.Statistics)}, nil
	})
	return set, err
}

// A present runner consumes unknown capacity in this no-worker harness. Retain
// its ID only after exact binding checks; an unexpected reference also fences.
func (d *Driver) runner(ctx context.Context, setID int) (*scaleset.RunnerReference, error) {
	var ref *scaleset.RunnerReference
	err := d.effect(ctx, "observe-runner", nil, func(c context.Context) (Event, error) {
		var err error
		ref, err = d.API.FindRunner(c, d.Approval.workerName())
		if err != nil {
			return Event{}, err
		}
		if ref == nil {
			return Event{}, nil
		}
		e := Event{Work: workUnresolved}
		if ref.ID > 0 && ref.Name == d.Approval.workerName() && ref.RunnerScaleSetID == setID {
			e.ID = ref.ID
		}
		return e, nil
	})
	return ref, err
}
