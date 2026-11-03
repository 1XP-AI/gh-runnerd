package livecanary

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrPublicEvidence is returned without echoing rejected private values.
var ErrPublicEvidence = errors.New("public evidence rejected")

const (
	publicEvidenceSchema     = "gh-runnerd/g01-public-evidence/v1"
	maxPublicEvidenceEvents  = 10000
	maxPublicEvidenceObjects = 1000
	maxPublicEvidenceTotal   = 10000
	maxPublicIdentifierBytes = 256
	maxPublicCounter         = 1000000
	maxPublicElapsed         = 24 * time.Hour
)

// PublicEvidenceResource carries a private identifier only as input to the
// allowlist serializer. Neither field is directly JSON serializable.
type PublicEvidenceResource struct {
	Kind string `json:"-"`
	ID   string `json:"-"`
}

// PublicEvidenceCounts contains the bounded counters allowed in a public
// experiment timeline. These fields are emitted only by MarshalPublicEvidence.
type PublicEvidenceCounts struct {
	Available  *int `json:"-"`
	Acquired   *int `json:"-"`
	Assigned   *int `json:"-"`
	Running    *int `json:"-"`
	Registered *int `json:"-"`
	Busy       *int `json:"-"`
	Idle       *int `json:"-"`
}

// PublicEvidenceObservation is private input to the public evidence
// serializer. Its fields are excluded from direct JSON serialization.
type PublicEvidenceObservation struct {
	Elapsed           time.Duration            `json:"-"`
	Attempt           int                      `json:"-"`
	SessionGeneration int                      `json:"-"`
	Phase             string                   `json:"-"`
	Operation         string                   `json:"-"`
	Outcome           string                   `json:"-"`
	Resources         []PublicEvidenceResource `json:"-"`
	HTTPStatus        int                      `json:"-"`
	Succeeded         *bool                    `json:"-"`
	Counts            *PublicEvidenceCounts    `json:"-"`
}

type publicEvidenceDocument struct {
	Schema string                `json:"schema"`
	Events []publicEvidenceEvent `json:"events"`
}

type publicEvidenceEvent struct {
	ElapsedMS         int64                 `json:"elapsed_ms"`
	Attempt           int                   `json:"attempt,omitempty"`
	SessionGeneration int                   `json:"session_generation,omitempty"`
	Phase             string                `json:"phase"`
	Operation         string                `json:"operation"`
	Outcome           string                `json:"outcome"`
	Resources         []string              `json:"resources,omitempty"`
	HTTPStatusClass   string                `json:"http_status_class,omitempty"`
	Succeeded         *bool                 `json:"succeeded,omitempty"`
	Counts            *publicEvidenceCounts `json:"counts,omitempty"`
}

type publicEvidenceCounts struct {
	Available  *int `json:"available,omitempty"`
	Acquired   *int `json:"acquired,omitempty"`
	Assigned   *int `json:"assigned,omitempty"`
	Running    *int `json:"running,omitempty"`
	Registered *int `json:"registered,omitempty"`
	Busy       *int `json:"busy,omitempty"`
	Idle       *int `json:"idle,omitempty"`
}

type publicEvidenceIdentity struct {
	kind string
	id   string
}

// MarshalPublicEvidence emits only the documented public evidence allowlist.
// Resource IDs are replaced with stable, per-document aliases, HTTP statuses
// are reduced to their class, and caller-supplied text is never copied out.
func MarshalPublicEvidence(observations []PublicEvidenceObservation) ([]byte, error) {
	if len(observations) == 0 || len(observations) > maxPublicEvidenceEvents {
		return nil, ErrPublicEvidence
	}

	document := publicEvidenceDocument{
		Schema: publicEvidenceSchema,
		Events: make([]publicEvidenceEvent, 0, len(observations)),
	}
	aliases := make(map[publicEvidenceIdentity]string)
	aliasCounts := make(map[string]int)
	var previousElapsed time.Duration
	totalResources := 0

	for index, observation := range observations {
		if observation.Elapsed < 0 || observation.Elapsed > maxPublicElapsed || (index > 0 && observation.Elapsed < previousElapsed) {
			return nil, ErrPublicEvidence
		}
		previousElapsed = observation.Elapsed

		if observation.Attempt < 0 || observation.Attempt > maxPublicCounter ||
			observation.SessionGeneration < 0 || observation.SessionGeneration > maxPublicCounter ||
			!publicEvidencePhase(observation.Phase) ||
			!publicEvidenceOperation(observation.Operation) ||
			!publicEvidenceOutcome(observation.Outcome) ||
			len(observation.Resources) > maxPublicEvidenceObjects ||
			totalResources > maxPublicEvidenceTotal-len(observation.Resources) {
			return nil, ErrPublicEvidence
		}
		totalResources += len(observation.Resources)

		statusClass, ok := publicHTTPStatusClass(observation.HTTPStatus)
		if !ok {
			return nil, ErrPublicEvidence
		}

		event := publicEvidenceEvent{
			ElapsedMS:         observation.Elapsed.Milliseconds(),
			Attempt:           observation.Attempt,
			SessionGeneration: observation.SessionGeneration,
			Phase:             observation.Phase,
			Operation:         observation.Operation,
			Outcome:           observation.Outcome,
			HTTPStatusClass:   statusClass,
		}
		if observation.Succeeded != nil {
			succeeded := *observation.Succeeded
			event.Succeeded = &succeeded
		}
		if observation.Counts != nil {
			counts, valid := publicEvidenceCountsFor(observation.Counts)
			if !valid {
				return nil, ErrPublicEvidence
			}
			event.Counts = counts
		}

		for _, resource := range observation.Resources {
			if !publicEvidenceResourceKind(resource.Kind) || resource.ID == "" ||
				len(resource.ID) > maxPublicIdentifierBytes || strings.TrimSpace(resource.ID) == "" {
				return nil, ErrPublicEvidence
			}
			identity := publicEvidenceIdentity{kind: resource.Kind, id: resource.ID}
			alias, found := aliases[identity]
			if !found {
				aliasCounts[resource.Kind]++
				alias = fmt.Sprintf("%s-%03d", resource.Kind, aliasCounts[resource.Kind])
				aliases[identity] = alias
			}
			event.Resources = append(event.Resources, alias)
		}
		document.Events = append(document.Events, event)
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, ErrPublicEvidence
	}
	return encoded, nil
}

func publicEvidencePhase(value string) bool {
	switch value {
	case "baseline", "before-ack", "after-ack", "acquisition-ambiguity", "jit-ambiguity",
		"idle-assignment-during-drain", "busy-safe-deregistration", "session-replacement", "cleanup":
		return true
	default:
		return false
	}
}

func publicEvidenceOperation(value string) bool {
	switch value {
	case "ack", "acquire", "create", "delete", "drain", "drain-marker", "inspect", "inventory",
		"jit", "observe-discovery", "observe-owned", "observe-poll", "observe-runner", "poll",
		"probe", "session-close", "session-open":
		return true
	default:
		return false
	}
}

func publicEvidenceOutcome(value string) bool {
	switch value {
	case "success", "failure", "observed", "unknown", "intent", "unresolved", "absent", "present",
		"held", "completed", "timeout", "suppressed", "rejected", "interrupted":
		return true
	default:
		return false
	}
}

func publicEvidenceResourceKind(value string) bool {
	switch value {
	case "scale-set", "session", "message", "request", "runner", "job", "worker":
		return true
	default:
		return false
	}
}

func publicHTTPStatusClass(status int) (string, bool) {
	if status == 0 {
		return "", true
	}
	if status < 100 || status > 599 {
		return "", false
	}
	return fmt.Sprintf("%dxx", status/100), true
}

func publicEvidenceCountsFor(input *PublicEvidenceCounts) (*publicEvidenceCounts, bool) {
	counts := &publicEvidenceCounts{}
	var ok bool
	if counts.Available, ok = publicEvidenceCount(input.Available); !ok {
		return nil, false
	}
	if counts.Acquired, ok = publicEvidenceCount(input.Acquired); !ok {
		return nil, false
	}
	if counts.Assigned, ok = publicEvidenceCount(input.Assigned); !ok {
		return nil, false
	}
	if counts.Running, ok = publicEvidenceCount(input.Running); !ok {
		return nil, false
	}
	if counts.Registered, ok = publicEvidenceCount(input.Registered); !ok {
		return nil, false
	}
	if counts.Busy, ok = publicEvidenceCount(input.Busy); !ok {
		return nil, false
	}
	if counts.Idle, ok = publicEvidenceCount(input.Idle); !ok {
		return nil, false
	}
	return counts, true
}

func publicEvidenceCount(input *int) (*int, bool) {
	if input == nil {
		return nil, true
	}
	if *input < 0 || *input > maxPublicCounter {
		return nil, false
	}
	value := *input
	return &value, true
}
