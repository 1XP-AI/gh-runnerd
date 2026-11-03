package livecanary

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func publicEvidenceInt(value int) *int { return &value }

func TestMarshalPublicEvidenceUsesAllowlistAndStableAliases(t *testing.T) {
	failed := false
	observations := []PublicEvidenceObservation{
		{
			Elapsed:           1250 * time.Millisecond,
			Attempt:           1,
			SessionGeneration: 2,
			Phase:             "after-ack",
			Operation:         "ack",
			Outcome:           "unknown",
			Resources: []PublicEvidenceResource{
				{Kind: "session", ID: "private-session-owner-needle"},
				{Kind: "message", ID: "private-message-id-needle"},
			},
			HTTPStatus: 503,
			Succeeded:  &failed,
			Counts:     &PublicEvidenceCounts{Available: publicEvidenceInt(0), Assigned: publicEvidenceInt(1)},
		},
		{
			Elapsed:           2400 * time.Millisecond,
			Attempt:           1,
			SessionGeneration: 2,
			Phase:             "after-ack",
			Operation:         "observe-poll",
			Outcome:           "observed",
			Resources: []PublicEvidenceResource{
				{Kind: "session", ID: "private-session-owner-needle"},
				{Kind: "request", ID: "private-request-id-needle"},
			},
			Counts: &PublicEvidenceCounts{Available: publicEvidenceInt(0), Assigned: publicEvidenceInt(1)},
		},
	}

	privateJSON, err := json.Marshal(observations[0])
	if err != nil || !bytes.Equal(privateJSON, []byte("{}")) {
		t.Fatalf("private input acquired an accidental JSON representation: %s, %v", privateJSON, err)
	}

	encoded, err := MarshalPublicEvidence(observations)
	if err != nil {
		t.Fatalf("MarshalPublicEvidence() error = %v", err)
	}
	for _, forbidden := range []string{
		"private-session-owner-needle",
		"private-message-id-needle",
		"private-request-id-needle",
		"1XP-AI",
		"workflow_ref",
		"authorization",
	} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("public evidence leaked %q: %s", forbidden, encoded)
		}
	}
	for _, expected := range []string{"\"elapsed_ms\":1250", "\"http_status_class\":\"5xx\"", "\"succeeded\":false", "\"available\":0", "\"assigned\":1", "\"session-001\"", "\"message-001\"", "\"request-001\""} {
		if !bytes.Contains(encoded, []byte(expected)) {
			t.Fatalf("public evidence lacks %s: %s", expected, encoded)
		}
	}

	var decoded struct {
		Schema string `json:"schema"`
		Events []struct {
			ElapsedMS         int64    `json:"elapsed_ms"`
			Attempt           int      `json:"attempt"`
			SessionGeneration int      `json:"session_generation"`
			Phase             string   `json:"phase"`
			Operation         string   `json:"operation"`
			Outcome           string   `json:"outcome"`
			Resources         []string `json:"resources"`
			HTTPStatusClass   string   `json:"http_status_class"`
			Succeeded         *bool    `json:"succeeded"`
		} `json:"events"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != "gh-runnerd/g01-public-evidence/v1" || len(decoded.Events) != 2 {
		t.Fatalf("public envelope = %+v", decoded)
	}
	first, second := decoded.Events[0], decoded.Events[1]
	if first.ElapsedMS != 1250 || first.Attempt != 1 || first.SessionGeneration != 2 || first.Phase != "after-ack" || first.Operation != "ack" || first.Outcome != "unknown" || first.HTTPStatusClass != "5xx" || first.Succeeded == nil || *first.Succeeded || len(first.Resources) != 2 || first.Resources[0] != "session-001" || first.Resources[1] != "message-001" {
		t.Fatalf("first public event = %+v", first)
	}
	if second.ElapsedMS != 2400 || len(second.Resources) != 2 || second.Resources[0] != "session-001" || second.Resources[1] != "request-001" {
		t.Fatalf("second public event = %+v", second)
	}
}

func TestMarshalPublicEvidenceRejectsUnallowlistedValues(t *testing.T) {
	base := PublicEvidenceObservation{
		Elapsed:   time.Second,
		Attempt:   1,
		Phase:     "baseline",
		Operation: "create",
		Outcome:   "success",
	}
	for _, tc := range []struct {
		name   string
		change func(*PublicEvidenceObservation)
	}{
		{name: "phase", change: func(o *PublicEvidenceObservation) { o.Phase = "private/repository-name" }},
		{name: "operation", change: func(o *PublicEvidenceObservation) { o.Operation = "https://api.github.com/private" }},
		{name: "outcome", change: func(o *PublicEvidenceObservation) { o.Outcome = "raw SDK error" }},
		{name: "resource kind", change: func(o *PublicEvidenceObservation) {
			o.Resources = []PublicEvidenceResource{{Kind: "owner", ID: "private"}}
		}},
		{name: "negative elapsed", change: func(o *PublicEvidenceObservation) { o.Elapsed = -time.Millisecond }},
		{name: "reversed elapsed", change: func(o *PublicEvidenceObservation) { o.Elapsed = 0 }},
		{name: "status", change: func(o *PublicEvidenceObservation) { o.HTTPStatus = 799 }},
		{name: "negative count", change: func(o *PublicEvidenceObservation) { o.Counts = &PublicEvidenceCounts{Running: publicEvidenceInt(-1)} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := base
			second := base
			second.Elapsed = 2 * time.Second
			tc.change(&second)
			_, err := MarshalPublicEvidence([]PublicEvidenceObservation{first, second})
			if err != ErrPublicEvidence {
				t.Fatalf("MarshalPublicEvidence() error = %v, want ErrPublicEvidence", err)
			}
		})
	}
}

func TestMarshalPublicEvidenceBoundsTotalResourceIdentifiers(t *testing.T) {
	resources := make([]PublicEvidenceResource, maxPublicEvidenceObjects)
	for index := range resources {
		resources[index] = PublicEvidenceResource{Kind: "session", ID: "private-session"}
	}
	observations := make([]PublicEvidenceObservation, maxPublicEvidenceTotal/maxPublicEvidenceObjects+1)
	for index := range observations {
		observations[index] = PublicEvidenceObservation{
			Phase:     "baseline",
			Operation: "observe-poll",
			Outcome:   "observed",
			Resources: resources,
		}
	}
	if _, err := MarshalPublicEvidence(observations); err != ErrPublicEvidence {
		t.Fatalf("MarshalPublicEvidence() error = %v, want ErrPublicEvidence", err)
	}
}
