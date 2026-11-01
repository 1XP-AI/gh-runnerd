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
	return request.Source != handoff.ReusableCalleeSource && request.Valid()
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

// selectedBrokerRequestMatchesApproval keeps the caller identity anchored to
// the broker/controller approvals while allowing an independently selected
// reusable-callee request to carry its additional callee identity. The callee
// is never reconstructed from a receipt or provider response.
func selectedBrokerRequestMatchesApproval(approval BrokerApproval, controller controllerApproval, request BrokerProvenanceRequest, source string) bool {
	baseSource := source
	if source == handoff.ReusableCalleeSource {
		baseSource = workflowProvenanceSource
	}
	base, err := brokerProvenanceRequest(approval, controller, baseSource)
	if err != nil || !request.Valid() || request.Source != source || request.ControllerApprovalSHA256 != base.ControllerApprovalSHA256 || request.Repository != base.Repository || request.WorkflowRunID != base.WorkflowRunID || request.WorkflowRef != base.WorkflowRef || request.WorkflowSHA != base.WorkflowSHA || request.WorkflowPath != base.WorkflowPath || request.Phase != base.Phase || request.OwnerNonce != base.OwnerNonce {
		return false
	}
	if request.Version == 0 {
		return request == base
	}
	return request.Version == 2 && request.Source == handoff.ReusableCalleeSource
}

func brokerHandoffRequest(approval BrokerApproval, plan *brokerControllerPlan, source string) (BrokerProvenanceRequest, error) {
	if plan == nil {
		return BrokerProvenanceRequest{}, errBroker
	}
	if plan.handoff != nil && plan.handoff.experimental {
		request := plan.handoff.request
		if !selectedBrokerRequestMatchesApproval(approval, plan.controller, request, source) {
			return BrokerProvenanceRequest{}, errBroker
		}
		return request, nil
	}
	return brokerProvenanceRequest(approval, plan.controller, source)
}
