package livecanary

import (
	"context"
	"os"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// Independently linked into the controller, with no production runtime setter.
var controllerExperimentPolicyJSON string

type controllerExperimentSelection struct {
	workflow handoff.TrustRoot
	issuer   handoff.IssuerRoot
	policy   handoff.PublicPolicy
	grant    handoff.PhaseGrant
	snapshot pairedApprovalSnapshot
	approval Approval
}

func selectControllerExperiment(files PairedTerminalFiles, mode, phase string, now time.Time) (controllerExperimentSelection, error) {
	p, w, i, e := handoff.DecodePublicPolicy([]byte(controllerExperimentPolicyJSON), "controller", now)
	if e != nil || phase == "cleanup" {
		return controllerExperimentSelection{}, ErrApproval
	}
	if mode == "controller" {
		if files.WorkerApprovalPath != "" || files.WorkerStateDirectory != "" || phase == pairedPreparationPhase {
			return controllerExperimentSelection{}, ErrApproval
		}
	} else if mode != "paired-terminal" || phase != pairedPreparationPhase {
		return controllerExperimentSelection{}, ErrApproval
	}
	var a Approval
	snap, e := readPairedApproval(files.ControllerApprovalPath, &a, DecodeStrict)
	if e != nil {
		return controllerExperimentSelection{}, ErrApproval
	}
	a, request, e := controllerHandoffRequest(snap.raw, phase, p.Source, now)
	if e != nil {
		return controllerExperimentSelection{}, ErrApproval
	}
	g, e := p.Select(mode, request, now)
	if e != nil || g.RepositoryID != a.RepositoryID || g.ExpiresAt.Before(a.ExpiresAt) {
		return controllerExperimentSelection{}, ErrApproval
	}
	if mode == "paired-terminal" {
		controller, worker, cs, _, e := readPairedSnapshots(files)
		if e != nil || ValidatePairedApprovals(controller, worker) != nil || ValidatePairedStatePaths(files.ControllerStateDirectory, files.WorkerStateDirectory) != nil || !os.SameFile(snap.info, cs.info) || handoff.Digest(cs.raw) != handoff.Digest(snap.raw) {
			return controllerExperimentSelection{}, ErrApproval
		}
	}
	return controllerExperimentSelection{w, i, p, g, snap, a}, nil
}

// RunControllerExperiment owns concrete stdin; FD3 is wrapped only after the
// compiled role/grant and exact raw approval are admitted. No legacy fallback.
func RunControllerExperiment(ctx context.Context, files PairedTerminalFiles, mode, phase string, input *os.File) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrApproval
	}
	selected, e := selectControllerExperiment(files, mode, phase, time.Now())
	if e != nil {
		return ErrApproval
	}
	return runControllerExperimentOwned(ctx, files, mode, phase, os.NewFile(3, "g01-control"), input, selected)
}

// RunControllerExperimentWithInputs accepts ownership, never trust roots or
// issuance claims. This is useful to embedders with already-owned handles.
func RunControllerExperimentWithInputs(ctx context.Context, files PairedTerminalFiles, mode, phase string, control, input *os.File) error {
	if control != nil {
		defer control.Close()
	}
	if input != nil {
		defer input.Close()
	}
	if ctx == nil || ctx.Err() != nil {
		return ErrApproval
	}
	selected, e := selectControllerExperiment(files, mode, phase, time.Now())
	if e != nil {
		return ErrApproval
	}
	return runControllerExperimentOwned(ctx, files, mode, phase, control, input, selected)
}
func runControllerExperimentOwned(parent context.Context, files PairedTerminalFiles, mode, phase string, control, input *os.File, selected controllerExperimentSelection) error {
	if parent == nil || parent.Err() != nil {
		return ErrApproval
	}
	owned, e := handoff.ProtectPipes(control, input)
	if e != nil {
		return ErrApproval
	}
	for _, f := range owned {
		defer f.Close()
	}
	deadline := selected.approval.ExpiresAt
	if selected.grant.ExpiresAt.Before(deadline) {
		deadline = selected.grant.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	open := OpenJournal
	if controllerExperimentFixtureOpen != nil {
		open = controllerExperimentFixtureOpen
	}
	j, e := open(files.ControllerStateDirectory, selected.approval)
	if e != nil {
		return ErrJournal
	}
	defer j.Close()
	// Re-read the named original under the independent lifecycle lease.
	release, e := j.authorize(selected.approval)
	if e != nil {
		return ErrJournal
	}
	var a Approval
	snap, e := readPairedApproval(files.ControllerApprovalPath, &a, DecodeStrict)
	release()
	if e != nil || !os.SameFile(selected.snapshot.info, snap.info) || handoff.Digest(selected.snapshot.raw) != handoff.Digest(snap.raw) || approvalDigest(a) != approvalDigest(selected.approval) {
		return ErrApproval
	}
	factory := func(ctx context.Context, a Approval, c Credentials) (API, error) {
		if ctx.Err() != nil || controllerHandoffParentCanceled(ctx) {
			return nil, ErrApproval
		}
		if controllerExperimentFixtureAPI != nil {
			return controllerExperimentFixtureAPI(a, c)
		}
		return NewSDKAPI(a, c)
	}
	if phase == "create" {
		return runControllerProcessHandoffCreate(ctx, j, snap.raw, owned[0], owned[1], selected.policy.Source, selected.workflow, selected.issuer, time.Now, factory)
	}
	var paired *PairedTerminalFiles
	if mode == "paired-terminal" {
		paired = &files
	}
	return runControllerProcessHandoffPhase(ctx, j, snap.raw, owned[0], owned[1], mode, phase, paired, selected.policy.Source, selected.workflow, selected.issuer, time.Now, factory)
}

// Only fixture-tag code may assign these private concrete constructors.
var controllerExperimentFixtureOpen func(string, Approval) (*FileJournal, error)
var controllerExperimentFixtureAPI func(Approval, Credentials) (API, error)
