package liveworker

import (
	"testing"
	"time"
)

func TestApprovalPinsByteVerifiedRunner3380ARM64(t *testing.T) {
	for _, test := range []struct {
		name, image string
		accept      bool
	}{
		{"byte-verified platform manifest", "ghcr.io/actions/actions-runner@sha256:9644537b29b4799cb752a2f23a45a1e603cf74c3c5998241a1533f0e8c3dc649", true},
		{"superseded 2.337.0 manifest", "ghcr.io/actions/actions-runner@sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d", false},
		{"mutable release tag", "ghcr.io/actions/actions-runner:2.338.0", false},
		{"multi-platform index", "ghcr.io/actions/actions-runner@sha256:4ffadc0002b2581327e06101fc8c06cd189232baf79fe561fac9caeb76f5e807", false},
		{"config instead of manifest", "ghcr.io/actions/actions-runner@sha256:5149aa9f00e2f5f0bbb0d5127cf31aea260e45e559d3d2f3d829623439660eae", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := approval()
			a.Image = test.image
			if err := a.Validate(time.Now()); (err == nil) != test.accept {
				t.Fatalf("image admission accepted=%v, want %v", err == nil, test.accept)
			}
		})
	}
}
