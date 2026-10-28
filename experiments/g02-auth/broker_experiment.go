package enrollment

import (
	"context"
	"crypto/ed25519"
	"os"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// Set only by the reviewed linker input of this broker build. No environment,
// argument, adapter, payload or exported setter can configure production policy.
var brokerExperimentPolicyJSON string

type brokerExperimentSelection struct {
	policy   handoff.PublicPolicy
	workflow handoff.TrustRoot
	issuer   handoff.IssuerRoot
	grant    handoff.PhaseGrant
	limit    time.Time
}

func selectBrokerExperiment(files BrokerFiles, now time.Time) (brokerExperimentSelection, error) {
	p, w, i, e := handoff.DecodePublicPolicy([]byte(brokerExperimentPolicyJSON), "broker", now)
	if e != nil {
		return brokerExperimentSelection{}, errBroker
	}
	var a BrokerApproval
	if _, e = readBrokerPrivateJSON(files.ApprovalPath, &a); e != nil || a.validate(now) != nil || a.Mode != "controller" && a.Mode != "paired-terminal" || a.Phase == "cleanup" {
		return brokerExperimentSelection{}, errBroker
	}
	var c controllerApproval
	raw, e := readBrokerPrivateJSON(files.ControllerApproval, &c)
	if e != nil || handoff.Digest(raw) != a.ControllerApprovalSHA256 || c.validate(a, now) != nil {
		return brokerExperimentSelection{}, errBroker
	}
	request, e := brokerProvenanceRequest(a, c, p.Source)
	if e != nil {
		return brokerExperimentSelection{}, errBroker
	}
	g, e := p.Select(a.Mode, request, now)
	if e != nil || g.OrganizationID != a.OrganizationID || g.RepositoryID != a.RepositoryID || g.ExpiresAt.Before(a.ExpiresAt) {
		return brokerExperimentSelection{}, errBroker
	}
	limit := minTime(a.ExpiresAt, c.ExpiresAt)
	if a.Mode == "paired-terminal" {
		var worker pairedWorkerApproval
		if _, e := readBrokerPrivateJSON(files.WorkerApproval, &worker); e != nil || worker.validate(now) != nil {
			return brokerExperimentSelection{}, errBroker
		}
		limit = minTime(limit, worker.ExpiresAt)
	}
	return brokerExperimentSelection{p, w, i, g, limit}, nil
}

// RunBrokerExperiment uses fixed inherited handles only after sealed policy and
// exact grant validation. Ownership transfers even on rejection; default builds
// refuse before wrapping arbitrary descriptors or creating durable intent.
func RunBrokerExperiment(ctx context.Context, files BrokerFiles, input *os.File) (BrokerResult, error) {
	if ctx == nil || ctx.Err() != nil {
		return BrokerResult{}, errBroker
	}
	selected, e := selectBrokerExperiment(files, time.Now())
	if e != nil {
		return BrokerResult{}, errBroker
	}
	return runBrokerExperimentOwned(ctx, files, input, os.NewFile(3, "g01-observation"), os.NewFile(4, "g01-workflow-signer"), os.NewFile(5, "g01-issuer-signer"), selected)
}

// RunBrokerExperimentWithInputs accepts owned concrete input pipes, never roots
// or claims. Its authority remains the independently compiled policy above.
func RunBrokerExperimentWithInputs(ctx context.Context, files BrokerFiles, input, observation, workflowSeed, issuerSeed *os.File) (BrokerResult, error) {
	for _, f := range []*os.File{input, observation, workflowSeed, issuerSeed} {
		if f != nil {
			defer f.Close()
		}
	}
	if ctx == nil || ctx.Err() != nil {
		return BrokerResult{}, errBroker
	}
	selected, e := selectBrokerExperiment(files, time.Now())
	if e != nil {
		return BrokerResult{}, errBroker
	}
	return runBrokerExperimentOwned(ctx, files, input, observation, workflowSeed, issuerSeed, selected)
}

type experimentalSigner struct {
	key   ed25519.PrivateKey
	check func() error
}

func (s *experimentalSigner) Sign(ctx context.Context, data []byte) ([]byte, error) {
	if s == nil || len(s.key) != ed25519.PrivateKeySize || ctx == nil || ctx.Err() != nil || s.check == nil || s.check() != nil {
		return nil, errBroker
	}
	signed := ed25519.Sign(s.key, data)
	if ctx.Err() != nil || s.check() != nil {
		clear(signed)
		return nil, errBroker
	}
	return signed, nil
}
func readExperimentalSeed(ctx context.Context, file *os.File, matches func(ed25519.PublicKey) bool) (ed25519.PrivateKey, error) {
	data, e := handoff.ReadProtected(ctx, file, ed25519.SeedSize)
	defer clear(data)
	if e != nil || len(data) != ed25519.SeedSize {
		return nil, errBroker
	}
	key := ed25519.NewKeyFromSeed(data)
	if !matches(key.Public().(ed25519.PublicKey)) {
		clear(key)
		return nil, errBroker
	}
	return key, nil
}
func runBrokerExperimentOwned(parent context.Context, files BrokerFiles, input, observation, workflowSeed, issuerSeed *os.File, selected brokerExperimentSelection) (BrokerResult, error) {
	if parent == nil || parent.Err() != nil {
		return BrokerResult{}, errBroker
	}
	owned, e := handoff.ProtectPipes(input, observation, workflowSeed, issuerSeed)
	if e != nil {
		return BrokerResult{}, errBroker
	}
	for _, f := range owned {
		defer f.Close()
	}
	// Chosen once before held claims/observation/input/mint; signing and launch
	// only narrow this original budget, never restart it.
	deadline := minTime(time.Now().Add(handoff.MaxPhaseDuration), minTime(selected.grant.ExpiresAt, selected.limit))
	if selected.grant.Request.Phase == "create" {
		deadline = minTime(deadline, time.Now().Add(handoff.MaxProcessDuration))
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	api := newBrokerAPIWithProvenanceRoot(time.Now, nil, selected.workflow)
	if brokerExperimentFixtureAPI != nil {
		api = brokerExperimentFixtureAPI(selected.workflow)
	}
	workflowSigner, issuerSigner := &experimentalSigner{}, &experimentalSigner{}
	defer func() { clear(workflowSigner.key); clear(issuerSigner.key) }()
	assembly := &brokerHandoffAssembly{experimental: true, signer: issuerSigner, issuer: selected.issuer, workflow: selected.workflow, source: selected.policy.Source, launch: func(context.Context, *issuedControllerHandoff, string) error { return errBroker }}
	api.provenanceFactory = func(check func() error) (BrokerProvenanceAdapter, error) {
		if ctx.Err() != nil {
			return nil, errBroker
		}
		if check == nil || check() != nil {
			return nil, errBroker
		}
		inputContext, stop := context.WithTimeout(ctx, 30*time.Second)
		defer stop()
		workflowSigner.check = check
		issuerSigner.check = check
		var e error
		workflowSigner.key, e = readExperimentalSeed(inputContext, owned[2], selected.workflow.MatchesPublicKey)
		if e != nil || check() != nil || ctx.Err() != nil {
			return nil, errBroker
		}
		issuerSigner.key, e = readExperimentalSeed(inputContext, owned[3], selected.issuer.MatchesPublicKey)
		if e != nil || check() != nil || ctx.Err() != nil {
			return nil, errBroker
		}
		grant := reviewedWorkflowGrant{selected.grant.Request, selected.grant.OrganizationID, selected.grant.RepositoryID, selected.grant.ExpiresAt}
		return newWorkflowProvenanceAdapter(grant, newWorkflowRunObserver(api.client.Transport), func(c context.Context) (string, error) {
			if c.Err() != nil || check() != nil {
				return "", errBroker
			}
			data, e := handoff.ReadProtected(c, owned[1], 1024)
			defer clear(data)
			if e != nil || !validBrokerToken(string(data)) || check() != nil {
				return "", errBroker
			}
			return string(data), nil
		}, workflowSigner, selected.workflow.KeyID(), time.Now, nil, check)
	}
	return runBrokerWithPrivateHandoff(ctx, files, owned[0], api, assembly)
}

// Nil in production; the fixture-tag file alone can assign the concrete fake
// HTTP/admission constructors. It cannot change the sealed grant selection.
var brokerExperimentFixtureAPI func(handoff.TrustRoot) *brokerAPI

func readBrokerExperimentalInput(parent context.Context, input *os.File) (brokerInput, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	data, e := handoff.ReadProtected(ctx, input, 65536)
	defer clear(data)
	var result brokerInput
	if e != nil || decodeBrokerJSON(data, &result, true) != nil || len(result.PEM) > 32768 || len(result.VerificationToken) > 1024 {
		return brokerInput{}, errBroker
	}
	return result, nil
}
