//go:build g01_pair_fixture

package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

type ProcessHandoffEffectsForFixture struct {
	WorkflowSignatures int
	BindingSignatures  int
}

// ProcessHandoffFixtureOptions changes only the tagged offline fixture's
// handoff observation. It cannot configure an ordinary broker build.
type ProcessHandoffFixtureOptions struct {
	ObserveHandoff   func(receipt, envelope []byte)
	ObserveAppJWTKey func(*rsa.PublicKey)
	MutateReceipt    func([]byte) []byte
}
type processFixtureSigner struct {
	key     ed25519.PrivateKey
	effects *ProcessHandoffEffectsForFixture
	kind    string
}

func (s processFixtureSigner) Sign(ctx context.Context, b []byte) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, errBroker
	}
	if s.kind == "workflow" {
		s.effects.WorkflowSignatures++
	} else {
		s.effects.BindingSignatures++
	}
	return ed25519.Sign(s.key, b), nil
}

// RunProcessHandoffForFixture is excluded from ordinary builds. It provides
// synthetic keys/PEM and a memory-only HTTP transport to the actual broker entry,
// held claims, observer, mint/preflight, private assembly and fixed FD3 launcher.
// The receiver's public pins are compiled independently in its TEST process.
func RunProcessHandoffForFixture(ctx context.Context, files BrokerFiles, transport http.RoundTripper, grant WorkflowGrantForFixture) (BrokerResult, ProcessHandoffEffectsForFixture, error) {
	return RunProcessHandoffForFixtureWithOptions(ctx, files, transport, grant, ProcessHandoffFixtureOptions{})
}

// RunProcessHandoffForFixtureWithOptions reuses the actual broker entry and
// child FD launcher while permitting a bounded fixture-only receipt fault.
func RunProcessHandoffForFixtureWithOptions(ctx context.Context, files BrokerFiles, transport http.RoundTripper, grant WorkflowGrantForFixture, options ProcessHandoffFixtureOptions) (BrokerResult, ProcessHandoffEffectsForFixture, error) {
	var effects ProcessHandoffEffectsForFixture
	if transport == nil {
		return BrokerResult{}, effects, errBroker
	}
	workflowSeed := make([]byte, ed25519.SeedSize)
	for i := range workflowSeed {
		workflowSeed[i] = byte(i)
	}
	workflowKey := ed25519.NewKeyFromSeed(workflowSeed)
	defer clear(workflowKey)
	defer clear(workflowSeed)
	issuerKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{71}, ed25519.SeedSize))
	defer clear(issuerKey)
	workflowRoot, _ := handoff.NewTrustRoot("fixture-ed25519-v1", workflowKey.Public().(ed25519.PublicKey))
	issuerRoot, _ := handoff.NewIssuerRoot("fixture-issuer-v1", issuerKey.Public().(ed25519.PublicKey))
	api := newBrokerAPIWithProvenanceRoot(time.Now, transport, workflowRoot)
	api.admissionDirectory = func() (string, error) {
		return filepath.EvalSymlinks(filepath.Join(filepath.Dir(files.StateDirectory), "admission"))
	}
	api.provenanceFactory = func(check func() error) (BrokerProvenanceAdapter, error) {
		reviewed := reviewedWorkflowGrant{grant.Request, grant.OrganizationID, grant.RepositoryID, grant.ExpiresAt}
		return newWorkflowProvenanceAdapter(reviewed, newWorkflowRunObserver(transport), func(context.Context) (string, error) { return "synthetic-observation-token", nil }, processFixtureSigner{key: workflowKey, effects: &effects, kind: "workflow"}, workflowRoot.KeyID(), time.Now, nil, check)
	}
	// Narrow existing offline seam; production validBrokerBuild remains exact.
	old := brokerBinaryOpener
	brokerBinaryOpener = func(path string, a BrokerApproval) (*verifiedBrokerBinary, error) {
		f, e := openBrokerPrivateFile(path, 0500, 128<<20)
		if e != nil {
			return nil, errBroker
		}
		b := &verifiedBrokerBinary{path, f, a.ControllerBinarySHA256}
		if b.check() != nil {
			f.Close()
			return nil, errBroker
		}
		return b, nil
	}
	defer func() { brokerBinaryOpener = old }()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		return BrokerResult{}, effects, errBroker
	}
	if options.ObserveAppJWTKey != nil {
		options.ObserveAppJWTKey(&key.PublicKey)
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	defer clear(private)
	verificationToken := ""
	if grant.Request.Version == 2 && grant.Request.Source == handoff.ReusableCalleeSource {
		verificationToken = "synthetic-process-verification-token"
	}
	data, e := json.Marshal(brokerInput{PEM: string(private), VerificationToken: verificationToken})
	if e != nil {
		return BrokerResult{}, effects, errBroker
	}
	defer clear(data)
	input, e := os.CreateTemp(filepath.Dir(files.StateDirectory), "synthetic-management-")
	if e != nil {
		return BrokerResult{}, effects, errBroker
	}
	defer func() { input.Close(); os.Remove(input.Name()) }()
	if _, e = input.Write(data); e != nil {
		return BrokerResult{}, effects, errBroker
	}
	if _, e = input.Seek(0, 0); e != nil {
		return BrokerResult{}, effects, errBroker
	}
	experimental := grant.Request.Version == 2 && grant.Request.Source == handoff.ReusableCalleeSource
	assembly := &brokerHandoffAssembly{
		experimental: experimental,
		signer:       processFixtureSigner{key: issuerKey, effects: &effects, kind: "binding"},
		issuer:       issuerRoot,
		workflow:     workflowRoot,
		request:      grant.Request,
		grant:        reviewedWorkflowGrant{Request: grant.Request, OrganizationID: grant.OrganizationID, RepositoryID: grant.RepositoryID, ExpiresAt: grant.ExpiresAt},
		source:       grant.Request.Source,
		launch:       func(context.Context, *issuedControllerHandoff, string) error { return errBroker },
	}
	assembly.fixtureTransform = func(h *issuedControllerHandoff) error {
		if h == nil {
			return errBroker
		}
		if options.ObserveHandoff != nil {
			options.ObserveHandoff(bytes.Clone(h.receipt), bytes.Clone(h.envelope))
		}
		if options.MutateReceipt != nil {
			mutated := options.MutateReceipt(bytes.Clone(h.receipt))
			if len(mutated) == 0 {
				return errBroker
			}
			clear(h.receipt)
			h.receipt = mutated
		}
		return nil
	}
	result, e := runBrokerWithPrivateHandoff(ctx, files, input, api, assembly)
	return result, effects, e
}
