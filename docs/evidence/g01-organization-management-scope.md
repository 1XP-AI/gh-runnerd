# G01 organization-only management compatibility

Writer checkpoint: 2026-10-25, based on merged main
`edd5d8fb651476c03d906434712130e1163a998c`. This is the single change-impact
and finding ledger for the experimental broker/controller scope correction.
G01 remains OPEN / Project In progress. Implementation review and GitHub review
for this new candidate are pending; earlier PR approvals do not approve it.

## Problem and explicit authority

The existing runner-management App has only organization self-hosted-runners
write permission and no repository access. An authenticated read of the existing
organization installation reconfirmed that permission and active state. The
[earlier preparation correction](https://github.com/1XP-AI/gh-runnerd/issues/1#issuecomment-6341510264)
already established that its role does not require adding repository access.
Private identifiers are kept out of this ledger. No installation token was minted.

The broker nevertheless always requested repository IDs plus metadata read,
required one installation-accessible repository, and used its management token
for a private-repository GET. G01 preflight repeated that assumption. These
requirements made the experimental path incompatible with the intended App.

The new optional `management_scope` field accepts exactly two authorities:

| Approved value | Request and preflight |
| --- | --- |
| Omitted/empty | Existing selected-repository contract: exact repository ID, runner write plus metadata read, one returned/accessibly installed private repository and the dedicated group policy. |
| `organization-only` | Only runner-write permission, with both repository selectors omitted; exact one-entry permission response and no returned repositories. Group and group-repository GETs establish the selected private target; management credentials never call installation-repository enumeration or direct repository GET. |

Unknown values refuse. Broker approval, controller approval and handed-off
credentials must agree; no response/error automatically chooses a scope. The
scope participates in raw controller approval bytes, typed approval/ownership
digests, broker resource identity, preparation receipts and their existing
paired/provenance bindings. Changing it cannot renew inspection/cleanup authority,
reuse consumed state or grant another attempt. New fields use `omitempty`, so
empty-scope legacy canonical bytes and digests remain unchanged.

For an organization-only response, absent/empty/null repositories are accepted;
a nonempty list refuses. Absent/empty, `all` and `selected` response selection
labels are compatibility data, not repository authority; an unknown label
refuses. Missing or additional permission, including metadata read, refuses.
Existing token/body/expiry bounds, identity/suspension checks and no-retry
behavior remain required. No enrollment permission policy changes.

Both group readers require the exact group, selected visibility, known false
default/inherited/public-access flags, count/list length one, and exact private,
explicitly nonfork repository identity and owner. G02 also checks the approved
numeric organization owner ID; G01 checks the approved owner login, since its
approval has no organization-ID field. G01 reuses `observationGET` to reject
ambiguous JSON, body/close failures and pagination links while accepting unrelated
GitHub metadata. Workflow checks still use their distinct verification token.

Organization runner-write remains organization-wide management authority.
Repository eligibility is enforced by the selected group and reviewed trusted
controller; this change does not create GitHub-enforced single-group permission.
Group-policy reads are separate snapshots, not an atomic policy lock. Exact
workflow-ref policy and live readiness remain separate requirements.

## Primary source and design review

The official [installation-token instructions](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app)
were read through `gh` using reusable blob
`27474b160fdc4f7661027a0813288f1a2bc3c98c`. Repository selectors are optional;
omission retains whatever repository access the installation already has.
Permissions may be narrowed, and repositories are returned when applicable.
The [official REST schema](https://github.com/github/rest-api-description/blob/755ee33b4a81d51f6f07a902cce26cbec3b822f2/descriptions/api.github.com/api.github.com.json)
requires only token/expiry in the token response, enumerates the known selection
labels, and enables the group-repository GET for Apps. The experiment deliberately
requires stronger explicit permission/policy evidence than schema minimums.
Missing live facts must refuse, not cause a broader grant or fallback.

An independent Codex GPT-6.1 Sol/medium design consultation approved this bounded
offline direction with explicit scope matching, durable bindings, legacy-byte
compatibility, exact mint permissions and complete group-policy checks. That
consultation does not substitute for review of the implemented candidate.

## Actual TDD and focused checks

After adding optional schema/payload scaffolding but before implementing scope
behavior, the G02 selector `^TestBroker(OrganizationOnly|ManagementScope)`
failed (exit 1, 0.600s): the mint still requested repository selectors/metadata,
valid organization-only issuance was rejected, and unknown scope was accepted.
G01 selector `^Test(OrganizationOnlyPreflight|ManagementScope)` failed
(exit 1, 0.343s): its valid organization-only preflight refused before any read,
and credential/scope mismatch cases were incorrectly admitted.

After the main correction, three additional ambiguous-policy cases reproduced
acceptance of duplicate/case-folded policy/owner keys through the old `get`
reader (exit 1, 0.333s). Reusing the existing stricter observer reader corrected
this new branch without changing the legacy preflight reader.

From G02, this final focused race selection passed (exit 0, 2.578s):

```text
go test -race -count=1 -timeout=45s -run '^Test(Broker(OrganizationOnly.*|ManagementScope.*|DiscoveryUsesOneRestrictedTokenThenScopeBeforeAuth|FailedGroupNeverObtainsAuthOrHandsOff|WorkflowVerificationUsesHeadBranchForAttestedBranchRef|WorkflowReceiptRejectsMissingPinnedRootBeforeCredentialInput|WorkflowReceiptRejectsAdapterSelectedRootBeforeCredentialInput|ControllerPayloadUsesActualIssuanceAndPrivateHandoff|MissingPolicyFactsAndForgedIssuanceRefuse|AmbiguousMintSurvivesRestartWithoutRetry)|ManagementScopeLegacyBrokerAndControllerGolden)$' .
```

From G01, this final focused race selection passed (exit 0, 1.578s):

```text
go test -race -count=1 -timeout=45s -run '^Test(OrganizationOnlyPreflight|ManagementScope|AuthoritySplitAndPolicyRejection|CredentialAttestationMismatchAndExpiredTokenRejected|PreflightRejectsCompleteResponseWhenBodyCloseFailsBeforeDrain|ReadApprovalAcceptsTrustedBrokerWorkflowRef|StrictJSONRejectsDecoderEquivalentDuplicateFields)' ./livecanary
```

Coverage includes exact positive request/response/handoff and group reads;
additional/missing permissions and bad token/expiry; complete policy facts;
separate workflow authority; refusal before credential/provenance/API access;
both scope-change directions; unchanged legacy golden bytes and resource,
approval and ownership digests; state-preserving recovery refusal; and no retry
after ambiguous issuance. All authentication and handoff work is synthetic.

The existing tagged front-door refusal/quarantine controls also passed:

```text
go test -race -count=1 -timeout=45s -tags=g01_live,g01_worker -run '^(TestPlanAndRefusalsNeverReadCredentialsOrEchoInputs|TestPairedTerminalModeQuarantinesBeforeControllerInput|TestTaggedControllerExecutionRequiresBrokerProvenanceBeforeInput|TestPairedTerminalModeRequiresWorkflowVerificationAuthorityBeforeInput)$' ./cmd/g01-live ./cmd/g01-worker
```

The controller package passed in 1.668s; the worker package compiled and reported
no matching tests (1.647s), not worker runtime coverage. Both changed packages'
`go vet` and `git diff --check` passed. The allowlisted
environment uses stock Go 1.27.3 Darwin/ARM64, empty `GOEXPERIMENT`, `GOENV=off`,
`GOWORK=off`, `GOAUTH=off`, `GOTELEMETRY=off`, `GOMAXPROCS=2`, `GOFLAGS=-p=2`,
cgo and explicit trusted compiler/cache/toolchain locations. No ambient
credentials, private proxy or native account admission root was used by tests.
The unchanged SDK protocol/image evidence is reused by its prior immutable
source; the full main matrix belongs to the eventual merge, not each edit.

## Remaining gates and rollback

No actual issuance response or runtime operation has qualified this mode. No
private key was read, token minted, auth scope changed, runner/group/App setting
changed, Docker daemon contacted, workflow dispatched or controller phase run.
Tagged execution remains quarantined. Authenticated provenance/trust-root
provisioning, conditional cleanup, exact approved workflow/group/runtime inputs
and concrete live authorization remain unresolved. The pinned v0.5.0
[SDK delete](https://github.com/actions/scaleset/blob/af394d86c0d5024225238b0209043c70193bcfe6/client.go#L642)
is still unconditional; this compatibility correction does not supply that
capability or complete G01/full G02.

Rollback is reverting the bounded scope/test/docs change. Existing legacy
experiments are not migrated; new organization-only approvals/state must not be
reinterpreted by an older binary. Preserve private inventory for explicit review.
Record candidate review and CI receipts in the PR without changing source merely
to copy their results into this ledger.
