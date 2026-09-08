package livecanary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

type baselinePair struct {
	Binding liveworker.PairBinding  `json:"binding"`
	Receipt *liveworker.PairReceipt `json:"receipt,omitempty"`
}
type baselineHost struct {
	Pair           controllerRecordRef `json:"pair"`
	ApprovalSHA256 string              `json:"approval_sha256"`
	DaemonID       string              `json:"daemon_id"`
	ImageID        string              `json:"image_id"`
	Image          string              `json:"image"`
}
type baselineRoster struct {
	Pair        controllerRecordRef `json:"pair"`
	Host        controllerRecordRef `json:"host"`
	Inventory   controllerRecordRef `json:"inventory"`
	Observation *rosterObservation  `json:"observation,omitempty"`
}
type baselineJIT struct {
	Parent       controllerRecordRef           `json:"parent"`
	Pair         controllerRecordRef           `json:"pair"`
	Acquire      controllerRecordRef           `json:"acquire"`
	RequestID    int64                         `json:"request_id"`
	ExpectedName string                        `json:"expected_name"`
	Runner       *liveworker.SDKRunnerIdentity `json:"runner,omitempty"`
}
type baselineHandoff struct {
	Parent    controllerRecordRef          `json:"parent"`
	Input     liveworker.HandoffReceipt    `json:"input"`
	Container *liveworker.ContainerReceipt `json:"container,omitempty"`
}
type baselineStart struct {
	Parent       controllerRecordRef         `json:"parent"`
	Handoff      controllerRecordRef         `json:"handoff"`
	Container    liveworker.ContainerReceipt `json:"container"`
	WorkerResult liveworker.RecordRef        `json:"worker_result"`
}
type baselineSample struct {
	Parent          controllerRecordRef      `json:"parent"`
	Completed       controllerRecordRef      `json:"completed"`
	Start           controllerRecordRef      `json:"start"`
	Previous        controllerRecordRef      `json:"previous"`
	Round           int                      `json:"round"`
	SDK             *sdkRunnerObservation    `json:"sdk,omitempty"`
	Job             *restJobObservation      `json:"job,omitempty"`
	REST            *restRunnerObservation   `json:"rest,omitempty"`
	RESTAddressable bool                     `json:"rest_addressable"`
	Local           *liveworker.LocalReceipt `json:"local,omitempty"`
}
type collectionOutcome string

const (
	collectionCollected  collectionOutcome = "collected"
	collectionIncomplete collectionOutcome = "incomplete"
	collectionUnresolved collectionOutcome = "unresolved"
)

type sessionOutstanding string

const (
	sessionNone        sessionOutstanding = "none"
	sessionKnownOpen   sessionOutstanding = "known-open"
	sessionOpenUnknown sessionOutstanding = "open-unknown"
)

type baselineCollectionFacts struct {
	Terminal           *terminalCollectionFacts `json:"terminal,omitempty"`
	Outcome            collectionOutcome        `json:"outcome"`
	Pair               controllerRecordRef      `json:"pair"`
	Start              controllerRecordRef      `json:"start"`
	Completed          controllerRecordRef      `json:"completed"`
	LastSample         controllerRecordRef      `json:"last_sample"`
	Rounds             int                      `json:"rounds"`
	OutstandingSession sessionOutstanding       `json:"outstanding_session"`
	SessionIntent      controllerRecordRef      `json:"session_intent"`
	SessionResult      controllerRecordRef      `json:"session_result"`
}

func workerRef(r controllerRecordRef) liveworker.RecordRef {
	return liveworker.RecordRef{Sequence: r.Sequence, EventSHA256: r.EventSHA256}
}
func controllerRef(r liveworker.RecordRef) controllerRecordRef {
	return controllerRecordRef{Sequence: r.Sequence, EventSHA256: r.EventSHA256}
}
func workerIdentity(i controllerJournalIdentity) liveworker.JournalIdentity {
	f := func(i controllerFileIdentity) liveworker.FileIdentity {
		return liveworker.FileIdentity{Device: i.Device, Inode: i.Inode}
	}
	return liveworker.JournalIdentity{OwnershipSHA256: i.OwnershipSHA256, State: f(i.State), Journal: f(i.Journal), AdmissionDirectory: f(i.AdmissionDirectory), Claim: f(i.Claim)}
}
func pairDigest(b liveworker.PairBinding) string {
	data, _ := json.Marshal(b)
	h := sha256.New()
	_, _ = h.Write([]byte("gh-runnerd/g01-pair/binding/v1\x00"))
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
func refPresent(r controllerRecordRef) bool { return r.Sequence > 0 && validSHA256(r.EventSHA256) }

// Exactly one closed payload is admitted for a stage. Nested intent/result
// differences are checked by replay against the actual stored predecessor.
func executionStage(stage string) bool {
	switch stage {
	case "pair", "host-preflight", "roster-anchor", "jit", "handoff", "worker-start", "identity-sample", "collection":
		return true
	}
	return false
}
func executionShape(r baselineRecord) bool {
	shape := baselineRecord{Version: r.Version, Stage: r.Stage, Outcome: r.Outcome, SetID: r.SetID, Creation: r.Creation, SessionID: r.SessionID}
	if r.Outcome == "result" || r.Outcome == "unknown" {
		shape.Intent = r.Intent
	}
	switch r.Stage {
	case "pair":
		shape.Pair = r.Pair
		if r.Pair == nil {
			return false
		}
	case "host-preflight":
		shape.Host = r.Host
		if r.Host == nil {
			return false
		}
	case "roster-anchor":
		shape.Roster = r.Roster
		if r.Roster == nil {
			return false
		}
	case "jit":
		shape.JIT = r.JIT
		shape.HTTPStatus = r.HTTPStatus
		if r.JIT == nil {
			return false
		}
	case "handoff":
		shape.Handoff = r.Handoff
		if r.Handoff == nil {
			return false
		}
	case "worker-start":
		shape.Start = r.Start
		if r.Start == nil {
			return false
		}
	case "identity-sample":
		shape.Sample = r.Sample
		if r.Sample == nil {
			return false
		}
	case "collection":
		shape.Collection = r.Collection
		return r.Collection != nil && r.Outcome == "observed" && reflect.DeepEqual(r, shape)
	default:
		return false
	}
	return r.Outcome != "observed" && reflect.DeepEqual(r, shape)
}

func validSHA256(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
