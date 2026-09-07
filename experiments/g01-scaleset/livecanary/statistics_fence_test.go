package livecanary

import (
	"context"
	"errors"
	"testing"

	"github.com/actions/scaleset"
)

type statisticsAPI struct {
	API
	create, owned bool
	statistics    *scaleset.RunnerScaleSetStatistic
}

func (a *statisticsAPI) CreateScaleSet(ctx context.Context, set *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error) {
	result, err := a.API.CreateScaleSet(ctx, set)
	if err == nil && result != nil && a.create {
		result.Statistics = a.statistics
	}
	return result, err
}

func (a *statisticsAPI) GetScaleSet(ctx context.Context, id int) (*scaleset.RunnerScaleSet, error) {
	result, err := a.API.GetScaleSet(ctx, id)
	if err == nil && result != nil && a.owned {
		copy := *result
		copy.Statistics = a.statistics
		return &copy, nil
	}
	return result, err
}

func statisticsSource(d *Driver, f *fakeAPI, j *memoryJournal, source string, statistics *scaleset.RunnerScaleSetStatistic) string {
	phase := "before-ack"
	f.session.message = nil
	switch source {
	case "create":
		j.events = nil
		f.set = nil
		f.createCalls = 0
		d.API = &statisticsAPI{API: f, create: true, statistics: statistics}
		phase = "create"
	case "owned", "inspect":
		d.API = &statisticsAPI{API: f, owned: true, statistics: statistics}
		if source == "inspect" {
			phase = "inspect"
		}
	case "session":
		f.session.session.Statistics = statistics
	case "session-set":
		copy := *f.set
		copy.Statistics = statistics
		f.session.session.RunnerScaleSet = &copy
	case "poll":
		f.session.message = &scaleset.RunnerScaleSetMessage{MessageID: 9, Statistics: statistics}
	}
	return phase
}

func TestStatisticsAtEverySourceSurviveLaterZeroAndFreshDriver(t *testing.T) {
	for _, source := range []string{"create", "owned", "inspect", "session", "session-set", "poll"} {
		for _, observation := range []struct {
			name       string
			statistics *scaleset.RunnerScaleSetStatistic
		}{
			{"available demand", &scaleset.RunnerScaleSetStatistic{TotalAvailableJobs: 1}},
			{"assigned demand", &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}},
			{"acquired", &scaleset.RunnerScaleSetStatistic{TotalAcquiredJobs: 1}},
			{"running", &scaleset.RunnerScaleSetStatistic{TotalRunningJobs: 1}},
			{"registered", &scaleset.RunnerScaleSetStatistic{TotalRegisteredRunners: 1}},
			{"busy", &scaleset.RunnerScaleSetStatistic{TotalBusyRunners: 1}},
			{"idle", &scaleset.RunnerScaleSetStatistic{TotalIdleRunners: 1}},
			{"negative", &scaleset.RunnerScaleSetStatistic{TotalAvailableJobs: -1}},
			{"missing", nil},
		} {
			t.Run(source+"/"+observation.name, func(t *testing.T) {
				d, f, j := created(t)
				phase := statisticsSource(d, f, j, source, observation.statistics)
				_ = d.Run(context.Background(), phase)
				if source == "create" && replay(j.Events()).setID != 7 {
					t.Error("valid create receipt was lost with observed statistics")
				}
				unsafe := observation.name != "available demand" && observation.name != "assigned demand"
				if unsafe && (source == "session" || source == "session-set") && (f.session.close != 0 || f.session.ack != 0 || f.session.acquire != 0 || replay(j.Events()).sessionID == "") {
					t.Error("unsafe initial statistics did not retain the exact open session")
				}
				f.set.Statistics = &scaleset.RunnerScaleSetStatistic{}
				f.session.session.Statistics = &scaleset.RunnerScaleSetStatistic{}
				f.session.session.RunnerScaleSet = nil
				restarted := Driver{d.Approval, j, f}
				_ = restarted.Run(context.Background(), "inspect")
				if err := restarted.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) || f.deleteCalls != 0 {
					t.Fatalf("observed statistics forgotten after zero inspection: cleanup=%v deletes=%d", err, f.deleteCalls)
				}
			})
		}
	}
}

func TestDemandStatisticsAllowControlledProbeButNeverCleanup(t *testing.T) {
	for _, source := range []string{"owned", "session", "session-set", "poll"} {
		t.Run(source, func(t *testing.T) {
			d, f, j := created(t)
			message := f.session.message
			statisticsSource(d, f, j, source, &scaleset.RunnerScaleSetStatistic{TotalAvailableJobs: 1, TotalAssignedJobs: 1})
			f.session.message = message
			if err := d.Run(context.Background(), "after-ack"); err != nil || f.session.ack != 1 || f.session.acquire != 0 || f.session.close != 1 {
				t.Fatalf("normal demand prevented the controlled barrier: %v", err)
			}
			if d.Run(context.Background(), "cleanup") == nil || f.deleteCalls != 0 {
				t.Fatal("controlled probe cleared demand or observed request evidence")
			}
		})
	}
}

func TestStatisticsFenceSurvivesFileJournalReopen(t *testing.T) {
	for _, source := range []string{"create", "owned", "inspect", "session", "session-set", "poll"} {
		t.Run(source, func(t *testing.T) {
			d, f, memory := created(t)
			phase := statisticsSource(d, f, memory, source, &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1})
			dir := privateDir(t)
			j, err := openTestJournal(t, dir, d.Approval)
			if err != nil {
				t.Fatal("private journal fixture")
			}
			for _, e := range memory.Events() {
				if e.Kind == "inventory" {
					e.Digest = fixtureInventory
				}
				if j.Append(e) != nil {
					t.Fatal("private receipt fixture")
				}
			}
			d.Journal = j
			d.API = statisticsInventoryAPI{d.API}
			_ = d.Run(context.Background(), phase)
			if j.Close() != nil {
				t.Fatal("close private fixture")
			}
			j, err = openTestJournal(t, dir, d.Approval)
			if err != nil {
				t.Fatal("reopen same owned journal")
			}
			defer j.Close()
			f.set.Statistics = &scaleset.RunnerScaleSetStatistic{}
			restarted := Driver{d.Approval, j, statisticsInventoryAPI{f}}
			_ = restarted.Run(context.Background(), "inspect")
			if err := restarted.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) || f.deleteCalls != 0 {
				t.Fatalf("statistics fence lost across actual journal reopen: cleanup=%v deletes=%d", err, f.deleteCalls)
			}
		})
	}
}

const fixtureInventory = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type statisticsInventoryAPI struct{ API }

func (statisticsInventoryAPI) Inventory(context.Context) (string, error) {
	return fixtureInventory, nil
}

// Fail the result after the server's synthetic read, then restore journal writes
// as a restarted process would. A pre-read durable intent must still stop delete.
type statisticsResultFailure struct {
	*memoryJournal
	refuse bool
}

func (j *statisticsResultFailure) Append(e Event) error {
	if j.refuse && e.Kind == "result" && e.Operation == "observe-owned" {
		return ErrJournal
	}
	return j.memoryJournal.Append(e)
}

func TestStatisticsResultWriteFailureRetainsFenceAcrossRestart(t *testing.T) {
	d, f, memory := created(t)
	d.API = &statisticsAPI{API: f, owned: true, statistics: &scaleset.RunnerScaleSetStatistic{TotalRunningJobs: 1}}
	j := &statisticsResultFailure{memoryJournal: memory, refuse: true}
	d.Journal = j
	if err := d.Run(context.Background(), "inspect"); !errors.Is(err, ErrJournal) {
		t.Errorf("positive-read result failure was not surfaced: %v", err)
	}
	j.refuse = false
	restarted := Driver{d.Approval, j, f}
	if err := restarted.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) || f.deleteCalls != 0 {
		t.Fatalf("failed observation persistence lost the pre-read fence: cleanup=%v deletes=%d", err, f.deleteCalls)
	}
}

func TestUnsafeStatisticsStopNewEffectsBeforeControlledMessage(t *testing.T) {
	for _, source := range []string{"owned", "session", "session-set", "poll"} {
		for _, phase := range []string{"after-ack", "acquire-loss", "jit-loss"} {
			if phase == "jit-loss" && source != "owned" {
				continue
			}
			t.Run(source+"/"+phase, func(t *testing.T) {
				d, f, j := created(t)
				message := f.session.message
				statisticsSource(d, f, j, source, &scaleset.RunnerScaleSetStatistic{TotalRunningJobs: 1})
				if source == "poll" {
					message.Statistics.TotalRunningJobs = 1
				}
				f.session.message = message
				if err := d.Run(context.Background(), phase); !errors.Is(err, ErrQuarantine) || f.session.ack != 0 || f.session.acquire != 0 || f.session.close != 0 || f.jitCalls != 0 {
					t.Fatalf("unsafe statistics admitted effects: phase=%v ack=%d acquire=%d close=%d jit=%d", err, f.session.ack, f.session.acquire, f.session.close, f.jitCalls)
				}
			})
		}
	}
}

func TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup(t *testing.T) {
	for _, source := range []string{"create", "owned", "inspect", "session", "session-set", "poll"} {
		t.Run(source, func(t *testing.T) {
			d, f, j := created(t)
			phase := statisticsSource(d, f, j, source, &scaleset.RunnerScaleSetStatistic{})
			err := d.Run(context.Background(), phase)
			if err != nil && !errors.Is(err, ErrNoMessage) {
				t.Fatalf("zero/optional absence fixture refused: %v", err)
			}
			if err := d.Run(context.Background(), "cleanup"); err != nil || f.deleteCalls != 1 {
				t.Fatalf("genuine empty cleanup refused: %v", err)
			}
		})
	}
}

func TestOlderPendingIntentSurvivesSuccessfulZeroInspection(t *testing.T) {
	d, f, j := created(t)
	if j.Append(Event{Kind: "intent", Operation: "session-open"}) != nil {
		t.Fatal("synthetic pending intent")
	}
	if err := d.Run(context.Background(), "inspect"); err != nil {
		t.Fatalf("read-only zero inspection refused: %v", err)
	}
	if !replay(j.Events()).uncertain || d.Run(context.Background(), "cleanup") == nil || f.deleteCalls != 0 {
		t.Fatal("a later successful observation released an older pending intent")
	}
}

func TestObservedRunnerSurvivesLaterAbsenceAndFirstCleanup(t *testing.T) {
	for _, phase := range []string{"inspect", "jit-loss"} {
		for _, reference := range []string{"owned", "wrong name", "wrong set", "invalid id"} {
			t.Run(phase+"/"+reference, func(t *testing.T) {
				d, f, j := created(t)
				f.findRunner = &scaleset.RunnerReference{ID: 8, Name: d.Approval.workerName(), RunnerScaleSetID: 7}
				switch reference {
				case "wrong name":
					f.findRunner.Name = "unexpected"
				case "wrong set":
					f.findRunner.RunnerScaleSetID = 99
				case "invalid id":
					f.findRunner.ID = 0
				}
				_ = d.Run(context.Background(), phase)
				if f.jitCalls != 0 {
					t.Fatal("known runner presence allowed another JIT")
				}
				if reference == "owned" {
					found := false
					for _, e := range j.Events() {
						found = found || e.ID == 8
					}
					if !found {
						t.Error("validated runner identity was not retained")
					}
				}
				f.findRunner = nil
				restarted := Driver{d.Approval, j, f}
				_ = restarted.Run(context.Background(), "inspect")
				if err := restarted.Run(context.Background(), "cleanup"); !errors.Is(err, ErrQuarantine) || f.deleteCalls != 0 {
					t.Fatalf("runner evidence lost before first cleanup: %v deletes=%d", err, f.deleteCalls)
				}
			})
		}
	}
}
