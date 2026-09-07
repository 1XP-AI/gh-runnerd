package liveworker

import (
	"context"
	"encoding/json"
	"net/http"
)

// ContainerStatus contains only normalized Docker v1.45 states. Unknown or
// absent status text is never copied into the serializable observation.
type ContainerStatus string

const (
	ContainerStatusUnknown    ContainerStatus = "unknown"
	ContainerStatusCreated    ContainerStatus = "created"
	ContainerStatusRunning    ContainerStatus = "running"
	ContainerStatusPaused     ContainerStatus = "paused"
	ContainerStatusRestarting ContainerStatus = "restarting"
	ContainerStatusRemoving   ContainerStatus = "removing"
	ContainerStatusExited     ContainerStatus = "exited"
	ContainerStatusDead       ContainerStatus = "dead"
)

// DockerStateFacts preserves present false/zero separately from missing/null.
// These facts do not verify the container profile or authorize an effect.
type DockerStateFacts struct {
	Status     ContainerStatus `json:"status"`
	Running    *bool           `json:"running"`
	Paused     *bool           `json:"paused"`
	Restarting *bool           `json:"restarting"`
	Dead       *bool           `json:"dead"`
	ExitCode   *int64          `json:"exit_code"`
}

type DockerInspectOutcome string

const (
	DockerInspectUnknown          DockerInspectOutcome = "unknown"
	DockerInspectPresent          DockerInspectOutcome = "present"
	DockerInspectNotFoundReported DockerInspectOutcome = "not-found-reported"
)

// DockerInspectObservation identifies one response from the approved Unix
// endpoint. NewDocker does not remember successful daemon preflight: even a
// supported 404 is only not-found-reported, not verified absence or ownership.
// Container is retained solely for the existing full profile verifier and must
// never enter serializable evidence with its raw configuration/environment.
type DockerInspectObservation struct {
	TargetID   string               `json:"target_id"`
	Method     string               `json:"method"`
	Path       string               `json:"path"`
	HTTPStatus int                  `json:"http_status"`
	Outcome    DockerInspectOutcome `json:"outcome"`
	State      *DockerStateFacts    `json:"state"`
	Container  *Container           `json:"-"`
}

type dockerStateWire struct {
	Status     *string
	Running    *bool
	Paused     *bool
	Restarting *bool
	Dead       *bool
	ExitCode   *int64
}

func normalizedContainerStatus(value *string) ContainerStatus {
	if value != nil {
		switch status := ContainerStatus(*value); status {
		case ContainerStatusCreated, ContainerStatusRunning, ContainerStatusPaused,
			ContainerStatusRestarting, ContainerStatusRemoving, ContainerStatusExited, ContainerStatusDead:
			return status
		}
	}
	return ContainerStatusUnknown
}

// InspectExact performs only the exact full-ID GET. It preserves bounded
// provenance on failed reads, but errors always retain the unknown outcome.
// Missing/null state is observable; callers choose their own presence policy.
func (d *Docker) InspectExact(ctx context.Context, containerID string) (DockerInspectObservation, error) {
	if !id.MatchString(containerID) {
		return DockerInspectObservation{}, ErrApproval
	}
	observation := DockerInspectObservation{
		TargetID: containerID, Method: http.MethodGet,
		Path:    "/v" + apiVersion + "/containers/" + containerID + "/json",
		Outcome: DockerInspectUnknown,
	}
	data, status, err := d.response(ctx, observation.Method, observation.Path, nil)
	observation.HTTPStatus = status
	if err != nil {
		return observation, err
	}
	defer clear(data)
	switch status {
	case http.StatusOK:
		if !validDockerJSON(data, dockerInspectObject) {
			return observation, ErrRemote
		}
		var container Container
		var wire struct{ State *dockerStateWire }
		if json.Unmarshal(data, &container) != nil || json.Unmarshal(data, &wire) != nil || container.ID != containerID {
			return observation, ErrRemote
		}
		if wire.State != nil {
			s := wire.State
			observation.State = &DockerStateFacts{normalizedContainerStatus(s.Status), s.Running, s.Paused, s.Restarting, s.Dead, s.ExitCode}
			container.State.Status = string(observation.State.Status)
		} else {
			container.State.Status = string(ContainerStatusUnknown)
		}
		observation.Container = &container
		observation.Outcome = DockerInspectPresent
	case http.StatusNotFound:
		// The pinned v1.45 ErrorResponse requires a non-null string message.
		// Validate its shape, then discard it; text never proves absence.
		var response struct {
			Message *string `json:"message"`
		}
		if !validDockerJSON(data, dockerErrorObject) || DecodeStrict(data, &response) != nil || response.Message == nil {
			return observation, ErrRemote
		}
		observation.Outcome = DockerInspectNotFoundReported
	default:
		return observation, ErrRemote
	}
	if ctx.Err() != nil {
		observation.Outcome, observation.State, observation.Container = DockerInspectUnknown, nil, nil
		return observation, ErrRemote
	}
	return observation, nil
}
