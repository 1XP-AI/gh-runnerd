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

type ProcessHandoffEffectsForFixture struct{ BindingSignatures int }
type processFixtureSigner struct {
	key     ed25519.PrivateKey
	effects *ProcessHandoffEffectsForFixture
}

func (s processFixtureSigner) Sign(ctx context.Context, b []byte) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, errBroker
	}
	s.effects.BindingSignatures++
	return ed25519.Sign(s.key, b), nil
}

// RunProcessHandoffForFixture is excluded from ordinary builds. It provides
// synthetic keys/PEM and a memory-only HTTP transport to the actual broker entry,
// held claims, observer, mint/preflight, private assembly and fixed FD3 launcher.
// The receiver's public pins are compiled independently in its TEST process.
func RunProcessHandoffForFixture(ctx context.Context, files BrokerFiles, transport http.RoundTripper, grant WorkflowGrantForFixture) (BrokerResult, ProcessHandoffEffectsForFixture, error) {
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
		return newWorkflowProvenanceAdapter(reviewed, newWorkflowRunObserver(transport), func(context.Context) (string, error) { return "synthetic-observation-token", nil }, processFixtureSigner{workflowKey, &ProcessHandoffEffectsForFixture{}}, workflowRoot.KeyID(), time.Now, nil, check)
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
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	defer clear(private)
	data, e := json.Marshal(brokerInput{PEM: string(private)})
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
	assembly := &brokerHandoffAssembly{signer: processFixtureSigner{issuerKey, &effects}, issuer: issuerRoot, workflow: workflowRoot, source: grant.Request.Source, launch: func(context.Context, *issuedControllerHandoff, string) error { return errBroker }}
	result, e := runBrokerWithPrivateHandoff(ctx, files, input, api, assembly)
	return result, effects, e
}
