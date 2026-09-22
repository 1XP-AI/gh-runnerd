package enrollment

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Optional metadata-only fixture. Build from a clean standalone approved source
// tree; this test always collides with a local snapshot and NEVER executes it.
func TestBrokerSnapshotCollisionBeforeMint(t *testing.T) {
	path := os.Getenv("G01_BROKER_METADATA_FIXTURE")
	if path == "" {
		t.Skip("requires clean metadata-only controller fixture")
	}
	brokerSnapshotFrontDoorFixture(t, path, true)
}
func TestBrokerUnsupportedMetadataFrontDoor(t *testing.T) {
	for _, name := range []string{"G01_BROKER_NO_CGO_FIXTURE", "G01_BROKER_OSUSERGO_FIXTURE"} {
		t.Run(name, func(t *testing.T) {
			path := os.Getenv(name)
			if path == "" {
				t.Skip("requires clean unsupported metadata-only controller fixture")
			}
			brokerSnapshotFrontDoorFixture(t, path, false)
		})
	}
}
func brokerSnapshotFrontDoorFixture(t *testing.T, path string, supported bool) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		t.Fatal("fixture metadata")
	}
	revision := ""
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			revision = s.Value
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("fixture bytes")
	}
	sum := sha256.Sum256(data)
	for _, kind := range []string{"regular", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			a, c, api, f, root := newBrokerFixture(t)
			a.Mode = "controller"
			a.Phase = "create"
			a.ControllerHarnessSHA = revision
			a.ControllerBinarySHA256 = hex.EncodeToString(sum[:])
			parent := filepath.Dir(root)
			binary := filepath.Join(parent, "controller")
			if os.WriteFile(binary, data, 0500) != nil {
				t.Fatal("fixture binary")
			}
			ctrl := controllerApproval{AppID: a.AppID, InstallationID: a.InstallationID, Organization: a.Organization, Repository: a.Repository, RepositoryID: a.RepositoryID, RunnerGroupID: a.RunnerGroupID, OwnerNonce: strings.Repeat("a", 32), HarnessSHA: revision, WorkflowSHA: strings.Repeat("b", 40), WorkflowPath: ".github/workflows/canary.yml", Controller: "trusted-controller", ExpiresAt: a.ExpiresAt, ActionsHosts: []string{"fixture.actions.githubusercontent.com"}, Phases: []string{"create"}}
			raw, _ := json.Marshal(ctrl)
			d := sha256.Sum256(raw)
			a.ControllerApprovalSHA256 = hex.EncodeToString(d[:])
			approval, _ := json.Marshal(a)
			ctrlPath := filepath.Join(parent, "controller.json")
			approvalPath := filepath.Join(parent, "approval.json")
			inputPath := filepath.Join(parent, "synthetic.json")
			inputData, _ := json.Marshal(brokerInput{PEM: string(c.PEM)})
			for name, bytes := range map[string][]byte{ctrlPath: raw, approvalPath: approval, inputPath: inputData} {
				if os.WriteFile(name, bytes, 0600) != nil {
					t.Fatal("fixture input")
				}
			}
			state := filepath.Join(parent, "controller-state")
			if os.Mkdir(state, 0700) != nil || os.Mkdir(root, 0700) != nil {
				t.Fatal("fixture state")
			}
			snapshot := filepath.Join(root, "controller-approval.json")
			if kind == "regular" {
				err = os.WriteFile(snapshot, []byte("existing"), 0600)
			} else {
				err = os.Symlink(ctrlPath, snapshot)
			}
			if err != nil {
				t.Fatal("fixture collision")
			}
			// Require the expected constructor result; only this metadata read is done.
			verified, e := openBrokerBinary(binary, a)
			if supported {
				if e != nil {
					t.Fatal("supported fixture was not accepted")
				}
				verified.file.Close()
			} else if e == nil {
				verified.file.Close()
				t.Error("unsupported metadata accepted")
			}

			input, _ := os.Open(inputPath)
			defer input.Close()
			_, e = runBrokerWithAPI(context.Background(), BrokerFiles{ApprovalPath: approvalPath, StateDirectory: root, ControllerBinary: binary, ControllerApproval: ctrlPath, ControllerStateDirectory: state}, input, api)
			if e == nil || f.tokenCalls != 0 || (!supported && len(f.calls) != 0) {
				t.Fatalf("snapshot collision accepted or minted: mint=%d", f.tokenCalls)
			}
		})
	}
}
