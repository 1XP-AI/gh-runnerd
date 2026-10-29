package livecanary

import (
	"context"
	"testing"
	"time"
)

func TestSessionReplacementCredentialVerificationRequired(t *testing.T) {
	a := approval()
	a.Phases = []string{"create", replacementPhase}
	if a.Validate(time.Now()) != nil {
		t.Fatal("replacement approval refused")
	}
	c := credentials(a)
	if c.validate(a, time.Now()) != nil {
		t.Fatal("separate verification refused")
	}
	for _, token := range []string{"", c.InstallationToken} {
		bad := c
		bad.VerificationToken = token
		if bad.validate(a, time.Now()) == nil {
			t.Fatal("replacement borrowed management authority for workflow verification")
		}
	}
	a.WorkflowRunID = 0
	if a.Validate(time.Now()) == nil {
		t.Fatal("replacement omitted controlled run")
	}
}

func TestSessionReplacementLegacyRunHasNoAuthority(t *testing.T) {
	var d *Driver
	if d.Run(context.Background(), replacementPhase) == nil {
		t.Fatal("legacy Run minted replacement authority")
	}
}
