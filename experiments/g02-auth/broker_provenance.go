package enrollment

import (
	"context"
	"crypto/ed25519"
	"regexp"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

type BrokerProvenanceRequest = handoff.Request
type BrokerProvenanceReceipt = handoff.Receipt
type BrokerProvenanceTrustRoot = handoff.TrustRoot

type BrokerProvenanceAdapter interface {
	Source() string
	Attest(context.Context, BrokerProvenanceRequest) (BrokerProvenanceReceipt, error)
	Verify(context.Context, BrokerProvenanceRequest, BrokerProvenanceReceipt) error
}

func NewBrokerProvenanceTrustRoot(keyID string, publicKey ed25519.PublicKey) (BrokerProvenanceTrustRoot, error) {
	root, err := handoff.NewTrustRoot(keyID, publicKey)
	if err != nil {
		return BrokerProvenanceTrustRoot{}, errBroker
	}
	return root, nil
}

var (
	brokerProvenanceSource = regexp.MustCompile(`^[a-z][a-z0-9._/-]{0,63}$`)
	brokerWorkflowRef      = regexp.MustCompile(`^refs/(?:heads|tags)/[A-Za-z0-9._/-]+$|^refs/pull/[1-9][0-9]*/(?:head|merge)$`)
)

func brokerProvenanceRequestValid(request BrokerProvenanceRequest) bool {
	return request.Valid()
}

func brokerProvenanceRequest(approval BrokerApproval, controller controllerApproval, source string) (BrokerProvenanceRequest, error) {
	phase := approval.Phase
	if approval.Mode == "paired-terminal" {
		phase = "paired-terminal"
	}
	request := BrokerProvenanceRequest{ControllerApprovalSHA256: approval.ControllerApprovalSHA256, Repository: approval.Organization + "/" + approval.Repository, WorkflowRunID: controller.WorkflowRunID, WorkflowRef: controller.WorkflowRef, WorkflowSHA: controller.WorkflowSHA, WorkflowPath: controller.WorkflowPath, Phase: phase, OwnerNonce: approval.OwnerNonce, Source: source}
	if approval.Mode != "controller" && approval.Mode != "paired-terminal" || !brokerProvenanceRequestValid(request) {
		return BrokerProvenanceRequest{}, errBroker
	}
	return request, nil
}
