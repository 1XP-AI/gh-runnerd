package livecanary

import "github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"

func validResponse(r observationResponse, endpoint string) bool {
	if r.Endpoint != endpoint || r.Status != 0 && (r.Status < 100 || r.Status > 599) {
		return false
	}
	switch r.Outcome {
	case observationPresent, observationPending:
		return r.Status == 200
	case observationNotFound:
		return r.Status == 404
	case observationUnresolved:
		return true
	}
	return false
}
func (s *baselineHistory) acceptSample(f *baselineSample, a Approval) bool {
	if f.SDK == nil || f.Job == nil || f.Local == nil {
		return false
	}
	if !validResponse(f.SDK.Response, "sdk_runner") {
		return false
	}
	if f.SDK.Response.Outcome == observationPresent {
		if f.SDK.ID != sdkRunnerID(s.jit.Runner.ID) || f.SDK.Name != s.jit.Runner.Name || f.SDK.ScaleSetID != s.setID {
			return false
		}
		s.lastSDK = f.SDK
	} else if f.SDK.Response.Outcome != observationNotFound {
		return false
	}
	job := f.Job
	switch job.Response.Endpoint {
	case "rest_run", "rest_attempt_jobs", "rest_job":
	default:
		return false
	}
	if !validResponse(job.Response, job.Response.Endpoint) || job.Response.Outcome == observationUnresolved {
		return false
	}
	if job.ID > 0 {
		attempt := job.Attempt
		wire := observedJobWire{ID: job.ID, RunID: job.RunID, RunAttempt: &attempt, HeadSHA: job.HeadSHA, Status: job.Status, Conclusion: job.Conclusion, RunnerID: job.RunnerID, RunnerName: job.RunnerName, RunnerGroupID: job.RunnerGroupID}
		if !wire.valid(a) {
			return false
		}
		if job.RunnerName != nil && *job.RunnerName != "" && *job.RunnerName != s.jit.Runner.Name || job.RunnerGroupID != nil && *job.RunnerGroupID > 0 && *job.RunnerGroupID != int64(a.RunnerGroupID) {
			return false
		}
		if old := s.lastJob; old != nil {
			if old.ID != job.ID || !jobProgresses(observedJobWire{Status: old.Status, Conclusion: old.Conclusion}, wire) {
				return false
			}
			if old.RunnerID != nil && *old.RunnerID > 0 && job.RunnerID != nil && *job.RunnerID > 0 && *old.RunnerID != *job.RunnerID {
				return false
			}
		}
		merged := *job
		if old := s.lastJob; old != nil {
			if merged.RunnerID == nil || *merged.RunnerID == 0 {
				merged.RunnerID = old.RunnerID
			}
			if merged.RunnerName == nil || *merged.RunnerName == "" {
				merged.RunnerName = old.RunnerName
			}
			if merged.RunnerGroupID == nil || *merged.RunnerGroupID == 0 {
				merged.RunnerGroupID = old.RunnerGroupID
			}
		}
		s.lastJob = &merged
	}
	addressable := s.lastJob != nil && s.lastJob.RunnerID != nil && *s.lastJob.RunnerID > 0 && s.lastJob.RunnerName != nil && *s.lastJob.RunnerName == s.jit.Runner.Name && s.lastJob.RunnerGroupID != nil && *s.lastJob.RunnerGroupID == int64(a.RunnerGroupID)
	if f.RESTAddressable != addressable || addressable != (f.REST != nil) {
		return false
	}
	if f.REST != nil {
		r := f.REST
		if !validResponse(r.Response, "rest_runner") {
			return false
		}
		if r.Response.Outcome == observationPresent {
			if r.ID != *s.lastJob.RunnerID || r.Name != s.jit.Runner.Name || r.Busy == nil || (r.Status != "online" && r.Status != "offline") {
				return false
			}
			s.lastREST = r
		} else if r.Response.Outcome != observationNotFound {
			return false
		}
	}
	l := f.Local
	if l.PairSHA256 != s.pair.Receipt.PairSHA256 || l.ContainerID != s.handoff.Container.ContainerID || !refPresent(controllerRef(l.Intent)) || !refPresent(controllerRef(l.Result)) || l.Result.Sequence <= l.Intent.Sequence || l.Method != "GET" || l.Path != "/v1.45/containers/"+l.ContainerID+"/json" {
		return false
	}
	if l.Outcome == liveworker.LocalProfilePresent && l.HTTPStatus == 200 {
		if old := s.lastLocal; old != nil && old.State != nil && old.State.Status == liveworker.ContainerStatusExited && (l.State == nil || l.State.Status != liveworker.ContainerStatusExited) {
			return false
		}
		s.lastLocal = l
	} else if l.Outcome != liveworker.LocalNotFoundReported || l.HTTPStatus != 404 || l.State != nil {
		return false
	}
	return true
}
