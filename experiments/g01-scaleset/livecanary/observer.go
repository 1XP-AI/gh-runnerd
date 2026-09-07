package livecanary

import "context"

// Private facts for a future authorized caller, never ownership or permission.
type sdkRunnerID int
type restRunnerID int64
type restJobID int64
type observationOutcome string

const (
	observationPresent    observationOutcome = "present"
	observationPending    observationOutcome = "pending"
	observationNotFound   observationOutcome = "not_found_reported"
	observationUnresolved observationOutcome = "unresolved"
)

type observationResponse struct {
	Endpoint string
	Status   int
	Outcome  observationOutcome
}
type sdkRunnerObservation struct {
	Response   observationResponse
	ID         sdkRunnerID
	Name       string
	ScaleSetID int
}
type restRunnerObservation struct {
	Response observationResponse
	ID       restRunnerID
	Name     string
	Status   string
	Busy     *bool
}
type restJobObservation struct {
	Response      observationResponse
	ID            restJobID
	RunID         int64
	Attempt       int
	HeadSHA       string
	Status        string
	Conclusion    *string
	RunnerID      *restRunnerID
	RunnerName    *string
	RunnerGroupID *int64
}

func (a *SDKAPI) observeSDKRunner(context.Context, sdkRunnerID, string, int) (sdkRunnerObservation, error) {
	return sdkRunnerObservation{}, ErrRemote
}
func (a *SDKAPI) observeRESTRunner(context.Context, restRunnerID, string) (restRunnerObservation, error) {
	return restRunnerObservation{}, ErrRemote
}
func (a *SDKAPI) observeRESTJob(context.Context, int, restJobID) (restJobObservation, error) {
	return restJobObservation{}, ErrRemote
}
