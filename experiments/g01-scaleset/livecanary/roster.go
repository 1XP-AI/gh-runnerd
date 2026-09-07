package livecanary

import "context"

type rosterCompleteness string

const (
	rosterUnresolved rosterCompleteness = "unresolved"
	rosterComplete   rosterCompleteness = "strict-stable-total-v1"
)

type rosterObservation struct {
	Organization string             `json:"organization"`
	Digest       string             `json:"digest"`
	Count        *int               `json:"count"`
	Pages        int                `json:"pages"`
	Completeness rosterCompleteness `json:"completeness"`
}

func (a *SDKAPI) observeRoster(context.Context) (rosterObservation, error) {
	return rosterObservation{Completeness: rosterUnresolved}, ErrRemote
}
