package livecanary

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func TestControllerExperimentSelectsReusableCalleeIndependently(t *testing.T) {
	now := time.Now()
	a := controllerHandoffApprovalAt(now)
	a.Phases = []string{"create"}
	a.GroupPolicy = controllerGroupPolicy()
	a.GroupPolicy.Workflow = a.Organization + "/" + a.Repository + "/.github/workflows/runner.yml@" + strings.Repeat("d", 40)
	raw, _ := json.Marshal(a)
	_, req, err := controllerHandoffRequest(raw, "create", "github-rest-workflow-v1", now)
	if err != nil {
		t.Fatal(err)
	}
	req.Version, req.Source = 2, handoff.ReusableCalleeSource
	req.Callee = handoff.CalleeIdentity{Repository: req.Repository, WorkflowPath: ".github/workflows/runner.yml", SHA: strings.Repeat("d", 40)}
	g := handoff.PhaseGrant{Request: req, Mode: "controller", OrganizationID: a.GroupPolicy.OrganizationID, RepositoryID: a.RepositoryID, ExpiresAt: a.ExpiresAt}
	g.GroupPolicySHA256 = a.GroupPolicy.GrantDigest(req, a.RepositoryID, int64(a.RunnerGroupID), a.ManagementScope)
	p := handoff.PublicPolicy{Version: 3, Role: "controller", Source: handoff.ReusableCalleeSource, WorkflowKeyID: "workflow-v1", WorkflowPublicKey: "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8", IssuerKeyID: "issuer-v1", IssuerPublicKey: "e28a8970753332bd72fef413e6b0b2ef1b4aadda7aa2c141f233712a6876b351", Grants: []handoff.PhaseGrant{g}}
	files := PairedTerminalFiles{ControllerApprovalPath: filepath.Join(privateDir(t), "approval.json")}
	if err := os.WriteFile(files.ControllerApprovalPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	old := controllerExperimentPolicyJSON
	defer func() { controllerExperimentPolicyJSON = old }()
	setPolicy := func(q handoff.PublicPolicy) { b, _ := json.Marshal(q); controllerExperimentPolicyJSON = string(b) }
	setPolicy(p)
	selected, err := selectControllerExperiment(files, "controller", "create", now)
	if err != nil || selected.grant.Request != req {
		t.Fatalf("independent selection failed: %v", err)
	}
	for _, fault := range []string{"callee", "group", "run", "approval-bytes"} {
		t.Run(fault, func(t *testing.T) {
			q := p
			q.Grants = append([]handoff.PhaseGrant(nil), p.Grants...)
			switch fault {
			case "callee":
				q.Grants[0].Request.Callee.SHA = strings.Repeat("f", 40)
			case "group":
				q.Grants[0].GroupPolicySHA256 = strings.Repeat("f", 64)
			case "run":
				q.Grants[0].Request.WorkflowRunID++
			case "approval-bytes":
				q.Grants[0].Request.ControllerApprovalSHA256 = strings.Repeat("f", 64)
			}
			setPolicy(q)
			if _, err := selectControllerExperiment(files, "controller", "create", now); err == nil {
				t.Fatal("mismatched policy selected")
			}
		})
	}
	setPolicy(p)
	var inputs, copies [2]*os.File
	for i := range inputs {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		fd, err := syscall.Dup(int(r.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		inputs[i], copies[i] = r, os.NewFile(uintptr(fd), "inspection")
		defer copies[i].Close()
		if _, err = w.Write([]byte("unconsumed")); err != nil {
			t.Fatal(err)
		}
		w.Close()
	}
	if err := RunControllerExperimentWithInputs(context.Background(), files, "controller", "create", inputs[0], inputs[1]); err == nil {
		t.Fatal("unintegrated v3 reached experiment")
	}
	for _, f := range copies {
		data, err := io.ReadAll(f)
		if err != nil || string(data) != "unconsumed" {
			t.Fatal("v3 rejection consumed input")
		}
	}
}
