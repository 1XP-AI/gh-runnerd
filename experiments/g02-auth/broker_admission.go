package enrollment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

// One finite experiment per native controller account. Never selected by HOME,
// approval, attempt path or a CLI flag; the operator must prepare this root.
func brokerAdmissionDirectory() (string, error) {
	if !brokerNativeAccountLookup {
		return "", errBroker
	}
	return brokerDirectoryForAccount(user.LookupId)
}

func workerAdmissionDirectory() (string, error) {
	if !brokerNativeAccountLookup {
		return "", errBroker
	}
	return workerDirectoryForAccount(user.LookupId)
}

// resolveWorkerClaimDirectory is the trusted worker admission root. Production
// uses the native-account pin. Offline tests may replace it; production never
// searches fixture siblings or other candidate paths for a matching inode.
var resolveWorkerClaimDirectory = workerAdmissionDirectory

func brokerHomeDir(lookup func(string) (*user.User, error)) (string, error) {
	if lookup == nil {
		return "", errBroker
	}
	uid := strconv.Itoa(os.Geteuid())
	u, e := lookup(uid)
	if e != nil || u == nil || u.Uid != uid || !filepath.IsAbs(u.HomeDir) || filepath.Clean(u.HomeDir) != u.HomeDir {
		return "", errBroker
	}
	real, e := filepath.EvalSymlinks(u.HomeDir)
	info, ie := os.Lstat(u.HomeDir)
	if e != nil || ie != nil || real != u.HomeDir || !brokerOwnedDirectory(info, false) {
		return "", errBroker
	}
	return u.HomeDir, nil
}

func brokerDirectoryForAccount(lookup func(string) (*user.User, error)) (string, error) {
	home, err := brokerHomeDir(lookup)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gh-runnerd-g01-experiment"), nil
}

func workerDirectoryForAccount(lookup func(string) (*user.User, error)) (string, error) {
	home, err := brokerHomeDir(lookup)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gh-runnerd-g01-worker-experiment"), nil
}
func brokerOwnedDirectory(i os.FileInfo, private bool) bool {
	if i == nil || !i.IsDir() || i.Mode().Perm()&0022 != 0 || (private && i.Mode().Perm() != 0700) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && int(s.Uid) == os.Geteuid()
}

// The finite slot schema is the union of the named phases and special slots.
// Each stored controller authority is bounded by the 16 KiB input contract;
// allow one claim/completion, one workflow-provenance GET start/confirmation
// pair and one read-token revocation start/confirmation pair for every schema
// slot.
const maxBrokerLedgerBytes = 512 << 10

type brokerInode struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

func brokerFileIdentity(i os.FileInfo) brokerInode {
	s := i.Sys().(*syscall.Stat_t)
	return brokerInode{uint64(s.Dev), s.Ino}
}
func brokerDigest(value any) string     { b, _ := json.Marshal(value); return brokerBytesDigest(b) }
func brokerBytesDigest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func brokerResource(a BrokerApproval) string {
	// Only mode/phase/expiry and controller-specific authority vary within this
	// finite experiment; all resource identities and the nonce remain pinned.
	a.Mode = ""
	a.Phase = ""
	a.ExpiresAt = time.Time{}
	a.AllowVerificationAuthority = false
	a.ControllerBinarySHA256 = ""
	a.ControllerApprovalSHA256 = ""
	a.ControllerHarnessSHA = ""
	return brokerDigest(a)
}

type brokerControllerBinding struct {
	Ownership string      `json:"ownership"`
	Binary    string      `json:"binary"`
	Harness   string      `json:"harness"`
	State     brokerInode `json:"state"`
}
type brokerWorkerBinding struct {
	Approval     string      `json:"approval"`
	ApprovalFile brokerInode `json:"approval_file"`
	State        brokerInode `json:"state"`
}
type brokerControllerAuthority struct {
	Digest   string             `json:"digest"`
	Approval controllerApproval `json:"approval"`
}

// The durable ledger records only the receipt identity, not its signature or
// any credential. A distinct ReceiptNonce makes each signed phase handoff
// one-shot while the validated receipt remains available to the broker
// handoff plan.
type brokerProvenanceBinding struct {
	Digest       string `json:"digest"`
	ReceiptNonce string `json:"receipt_nonce"`
	Source       string `json:"source"`
}

func (b brokerProvenanceBinding) valid() bool {
	return brokerSHA256.MatchString(b.Digest) && brokerNonce.MatchString(b.ReceiptNonce) && brokerProvenanceSource.MatchString(b.Source)
}

type brokerClaimEvent struct {
	Kind           string                     `json:"kind"`
	Slot           string                     `json:"slot"`
	Attempt        brokerInode                `json:"attempt"`
	Journal        brokerInode                `json:"journal"`
	Controller     *brokerControllerBinding   `json:"controller,omitempty"`
	Worker         *brokerWorkerBinding       `json:"worker,omitempty"`
	Authority      *brokerControllerAuthority `json:"authority,omitempty"`
	Provenance     *brokerProvenanceBinding   `json:"provenance,omitempty"`
	Snapshot       brokerInode                `json:"snapshot"`
	SnapshotDigest string                     `json:"snapshot_digest,omitempty"`
}
type brokerTokenLifecycleEvent struct {
	Kind    string      `json:"kind"`
	Slot    string      `json:"slot"`
	Attempt brokerInode `json:"attempt"`
	Journal brokerInode `json:"journal"`
}

// brokerWorkflowProvenanceEvent binds the authenticated workflow-run GET to
// the exact attempt, journal, approval and observed run identity. The event
// carries no credential or response body. A started event is durable before
// dispatch; only its exact confirmation or an explicit no-dispatch outcome
// may release replay.
type brokerWorkflowProvenanceEvent struct {
	Kind                     string      `json:"kind"`
	Slot                     string      `json:"slot"`
	Attempt                  brokerInode `json:"attempt"`
	Journal                  brokerInode `json:"journal"`
	ControllerApprovalSHA256 string      `json:"controller_approval_sha256"`
	Source                   string      `json:"source"`
	WorkflowRunID            int64       `json:"workflow_run_id"`
	WorkflowRef              string      `json:"workflow_ref"`
	WorkflowSHA              string      `json:"workflow_sha"`
	WorkflowPath             string      `json:"workflow_path"`
}
type brokerLedgerHeader struct {
	Version  int         `json:"version"`
	Resource string      `json:"resource"`
	Root     brokerInode `json:"root"`
	Lock     brokerInode `json:"lock"`
}
type brokerAdmission struct {
	file                           *os.File
	lock                           *os.File
	lockInfo                       os.FileInfo
	root, parent                   *os.Root
	path                           string
	rootInfo, parentInfo, fileInfo os.FileInfo
	data                           []byte
	event                          brokerClaimEvent
	tokenLifecyclePending          bool
	tokenLifecycleKind             string
	workflowProvenancePending      *brokerWorkflowProvenanceEvent
	workflowProvenanceConfirmed    map[string]brokerWorkflowProvenanceEvent
	workflowProvenanceNoDispatch   map[string]bool
	ledgerOnly                     bool
	ledgerSlot                     string
	ledgerAttempt, ledgerJournal   brokerInode
	sync                           func(*os.Root) error
}

func (c *brokerAdmission) close() {
	if c == nil {
		return
	}
	if c.file != nil {
		c.file.Close()
	}
	if c.lock != nil {
		c.lock.Close()
	}
	if c.root != nil {
		c.root.Close()
	}
	if c.parent != nil {
		c.parent.Close()
	}
}
func (c *brokerAdmission) check() error {
	if c.lock == nil || !privateFile(c.lock) {
		return errBroker
	}
	lock, le := c.root.Lstat("broker-admission.lock")
	openedLock, loe := c.lock.Stat()
	if le != nil || loe != nil || !os.SameFile(lock, c.lockInfo) || !os.SameFile(openedLock, c.lockInfo) || openedLock.Size() != 0 || c.lock.Sync() != nil {
		return errBroker
	}
	real, e := filepath.EvalSymlinks(c.path)
	info, ie := os.Lstat(c.path)
	pi, pe := os.Lstat(filepath.Dir(c.path))
	named, ne := c.root.Lstat("broker-admission.jsonl")
	opened, oe := c.file.Stat()
	if e != nil || real != c.path || ie != nil || pe != nil || ne != nil || oe != nil || !brokerOwnedDirectory(info, true) || !brokerOwnedDirectory(pi, false) || !os.SameFile(info, c.rootInfo) || !os.SameFile(pi, c.parentInfo) || !os.SameFile(named, c.fileInfo) || !os.SameFile(opened, c.fileInfo) || !privateFile(c.file) || opened.Size() > maxBrokerLedgerBytes {
		return errBroker
	}
	b, e := io.ReadAll(io.NewSectionReader(c.file, 0, maxBrokerLedgerBytes+1))
	if e != nil || !bytes.Equal(b, c.data) || c.file.Sync() != nil || c.sync(c.parent) != nil || c.sync(c.root) != nil {
		return errBroker
	}
	return nil
}
func (c *brokerAdmission) append(value any) error {
	b, e := json.Marshal(value)
	if e != nil {
		return errBroker
	}
	b = append(b, '\n')
	if n, e := c.file.Write(b); e != nil || n != len(b) || c.file.Sync() != nil {
		return errBroker
	}
	c.data = append(c.data, b...)
	return nil
}
func (c *brokerAdmission) complete() error {
	if c == nil || c.tokenLifecyclePending || c.check() != nil {
		return errBroker
	}
	e := c.event
	e.Kind = "complete"
	if c.append(e) != nil {
		return errBroker
	}
	return c.check()
}
func (c *brokerAdmission) beginReadTokenLifecycle() error {
	return c.beginTokenLifecycle("read_token_lifecycle_started", "read")
}
func (c *brokerAdmission) beginManagementTokenLifecycle() error {
	return c.beginTokenLifecycle("management_token_lifecycle_started", "management")
}
func (c *brokerAdmission) beginTokenLifecycle(eventKind, lifecycleKind string) error {
	if c == nil || c.tokenLifecyclePending || c.check() != nil {
		return errBroker
	}
	event := brokerTokenLifecycleEvent{Kind: eventKind, Slot: c.event.Slot, Attempt: c.event.Attempt, Journal: c.event.Journal}
	if c.append(event) != nil || c.check() != nil {
		return errBroker
	}
	c.tokenLifecyclePending = true
	c.tokenLifecycleKind = lifecycleKind
	return nil
}
func (c *brokerAdmission) confirmReadTokenLifecycle() error {
	return c.confirmTokenLifecycle("read_token_lifecycle_confirmed", "read")
}
func (c *brokerAdmission) confirmManagementTokenLifecycle() error {
	return c.confirmTokenLifecycle("management_token_lifecycle_confirmed", "management")
}
func (c *brokerAdmission) confirmTokenLifecycle(eventKind, lifecycleKind string) error {
	if c == nil || !c.tokenLifecyclePending || c.tokenLifecycleKind != lifecycleKind || c.check() != nil {
		return errBroker
	}
	event := brokerTokenLifecycleEvent{Kind: eventKind, Slot: c.event.Slot, Attempt: c.event.Attempt, Journal: c.event.Journal}
	if c.append(event) != nil || c.check() != nil {
		return errBroker
	}
	c.tokenLifecyclePending = false
	c.tokenLifecycleKind = ""
	return nil
}
func (c *brokerAdmission) recordReadTokenRequestNotDispatched() error {
	if c == nil || c.tokenLifecyclePending || c.event.Provenance == nil || c.event.Provenance.Source != handoff.ReusableCalleeSource || c.check() != nil {
		return errBroker
	}
	event := brokerTokenLifecycleEvent{Kind: "read_token_request_not_dispatched", Slot: c.event.Slot, Attempt: c.event.Attempt, Journal: c.event.Journal}
	if c.append(event) != nil || c.check() != nil {
		return errBroker
	}
	return nil
}
func (c *brokerAdmission) recordManagementTokenRequestNotDispatched() error {
	if c == nil || c.tokenLifecyclePending || c.check() != nil {
		return errBroker
	}
	event := brokerTokenLifecycleEvent{Kind: "management_token_request_not_dispatched", Slot: c.event.Slot, Attempt: c.event.Attempt, Journal: c.event.Journal}
	if c.append(event) != nil || c.check() != nil {
		return errBroker
	}
	return nil
}

func (c *brokerAdmission) beginWorkflowProvenance(event brokerWorkflowProvenanceEvent) error {
	if c == nil || !c.ledgerOnly || c.workflowProvenancePending != nil || !validBrokerWorkflowProvenanceEvent(event) || event.Kind != "workflow_provenance_get_started" {
		return errBroker
	}
	_, confirmed := c.workflowProvenanceConfirmed[event.Slot]
	if event.Slot != c.ledgerSlot || event.Attempt != c.ledgerAttempt || event.Journal != c.ledgerJournal || confirmed || c.workflowProvenanceNoDispatch[event.Slot] || c.check() != nil {
		return errBroker
	}
	if c.append(event) != nil || c.check() != nil {
		return errBroker
	}
	c.workflowProvenancePending = &event
	return nil
}

func (c *brokerAdmission) confirmWorkflowProvenance(event brokerWorkflowProvenanceEvent) error {
	if c == nil || !c.ledgerOnly || c.workflowProvenancePending == nil || event.Kind != "workflow_provenance_get_confirmed" || !sameBrokerWorkflowProvenance(*c.workflowProvenancePending, event) || c.check() != nil {
		return errBroker
	}
	if c.append(event) != nil || c.check() != nil {
		return errBroker
	}
	c.workflowProvenanceConfirmed[event.Slot] = event
	c.workflowProvenancePending = nil
	return nil
}

func (c *brokerAdmission) recordWorkflowProvenanceNotDispatched(event brokerWorkflowProvenanceEvent) error {
	if c == nil || !c.ledgerOnly || event.Kind != "workflow_provenance_get_not_dispatched" || !validBrokerWorkflowProvenanceEvent(event) || event.Slot != c.ledgerSlot || event.Attempt != c.ledgerAttempt || event.Journal != c.ledgerJournal || c.workflowProvenanceNoDispatch[event.Slot] || c.workflowProvenanceConfirmed[event.Slot].Kind != "" || c.check() != nil {
		return errBroker
	}
	if c.workflowProvenancePending != nil && !sameBrokerWorkflowProvenance(*c.workflowProvenancePending, event) {
		return errBroker
	}
	if c.append(event) != nil || c.check() != nil {
		return errBroker
	}
	c.workflowProvenanceNoDispatch[event.Slot] = true
	c.workflowProvenancePending = nil
	return nil
}

func openBrokerAdmission(directory string, a BrokerApproval, j *brokerJournal, p *brokerControllerPlan, syncRoot func(*os.Root) error) (*brokerAdmission, error) {
	return openBrokerAdmissionMode(directory, a, j, p, syncRoot, false)
}

// openBrokerAdmissionLedger acquires the same fixed account lock and replays
// the same ledger as claim admission, but deliberately creates no phase claim.
// The caller holds the returned lock across the authenticated provenance GET.
func openBrokerAdmissionLedger(directory string, a BrokerApproval, j *brokerJournal, syncRoot func(*os.Root) error) (*brokerAdmission, error) {
	return openBrokerAdmissionMode(directory, a, j, nil, syncRoot, true)
}

func openBrokerAdmissionMode(directory string, a BrokerApproval, j *brokerJournal, p *brokerControllerPlan, syncRoot func(*os.Root) error, ledgerOnly bool) (claim *brokerAdmission, err error) {
	if syncRoot == nil || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return nil, errBroker
	}
	real, e := filepath.EvalSymlinks(directory)
	info, ie := os.Lstat(directory)
	pi, pe := os.Lstat(filepath.Dir(directory))
	if e != nil || real != directory || ie != nil || pe != nil || !brokerOwnedDirectory(info, true) || !brokerOwnedDirectory(pi, false) {
		return nil, errBroker
	}
	parent, e := os.OpenRoot(filepath.Dir(directory))
	if e != nil {
		return nil, errBroker
	}
	c := &brokerAdmission{parent: parent, path: directory, rootInfo: info, parentInfo: pi, sync: syncRoot}
	defer func() {
		if err != nil {
			c.close()
		}
	}()
	captured, e := parent.Stat(".")
	if e != nil || !os.SameFile(pi, captured) || syncRoot(parent) != nil {
		return nil, errBroker
	}
	c.root, e = parent.OpenRoot(filepath.Base(directory))
	if e != nil {
		return nil, errBroker
	}
	captured, e = c.root.Stat(".")
	if e != nil || !os.SameFile(info, captured) {
		return nil, errBroker
	}

	// The empty lock is only serialization, never recovery authority. Acquire it
	// before creating/reading content so a contender cannot win an empty ledger.
	c.lock, e = c.root.OpenFile("broker-admission.lock", os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if e != nil || !privateFile(c.lock) || syscall.Flock(int(c.lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return nil, errBroker
	}
	c.lockInfo, e = c.lock.Stat()
	namedLock, ne := c.root.Lstat("broker-admission.lock")
	if e != nil || ne != nil || c.lockInfo.Size() != 0 || !os.SameFile(c.lockInfo, namedLock) || c.lock.Sync() != nil || syncRoot(c.root) != nil {
		return nil, errBroker
	}
	wantHeader := brokerLedgerHeader{1, brokerResource(a), brokerFileIdentity(info), brokerFileIdentity(c.lockInfo)}
	created := true
	c.file, e = c.root.OpenFile("broker-admission.jsonl", os.O_RDWR|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if os.IsExist(e) {
		created = false
		c.file, e = c.root.OpenFile("broker-admission.jsonl", os.O_RDWR|syscall.O_NOFOLLOW, 0)
	}
	if e != nil || !privateFile(c.file) {
		return nil, errBroker
	}
	c.fileInfo, e = c.file.Stat()
	if e != nil || c.fileInfo.Size() > maxBrokerLedgerBytes {
		return nil, errBroker
	}
	if created {
		if c.append(wantHeader) != nil {
			return nil, errBroker
		}
	} else {
		c.data, e = io.ReadAll(io.LimitReader(c.file, maxBrokerLedgerBytes+1))
		if e != nil {
			return nil, errBroker
		}
	}
	if c.check() != nil {
		return nil, errBroker
	}
	lines := bytes.Split(c.data, []byte{'\n'})
	if len(lines) < 2 || len(lines) > brokerLedgerMaxLines() || len(lines[len(lines)-1]) != 0 {
		return nil, errBroker
	}
	var header brokerLedgerHeader
	if decodeBrokerJSON(lines[0], &header, true) != nil || header != wantHeader {
		return nil, errBroker
	}
	slots := map[string]brokerClaimEvent{}
	done := map[string]bool{}
	var binding *brokerControllerBinding
	var workerBinding *brokerWorkerBinding
	var authority *brokerControllerAuthority
	provenanceNonces := map[string]bool{}
	readTokenLifecycleSlots := map[string]bool{}
	managementTokenLifecycleSlots := map[string]bool{}
	readTokenRequestNotDispatchedSlots := map[string]bool{}
	managementTokenRequestNotDispatchedSlots := map[string]bool{}
	workflowProvenanceConfirmed := map[string]brokerWorkflowProvenanceEvent{}
	workflowProvenanceNoDispatch := map[string]bool{}
	latestSlot := ""
	var pendingTokenLifecycle *brokerTokenLifecycleEvent
	var pendingWorkflowProvenance *brokerWorkflowProvenanceEvent
	for _, line := range lines[1 : len(lines)-1] {
		var kind struct {
			Kind string `json:"kind"`
		}
		if decodeBrokerJSON(line, &kind, false) != nil {
			return nil, errBroker
		}
		switch kind.Kind {
		case "workflow_provenance_get_started", "workflow_provenance_get_confirmed", "workflow_provenance_get_not_dispatched":
			var event brokerWorkflowProvenanceEvent
			if decodeBrokerJSON(line, &event, true) != nil || !validBrokerWorkflowProvenanceEvent(event) {
				return nil, errBroker
			}
			switch event.Kind {
			case "workflow_provenance_get_started":
				if pendingWorkflowProvenance != nil || event.Slot == "discover-actions-host" || workflowProvenanceConfirmed[event.Slot].Kind != "" || workflowProvenanceNoDispatch[event.Slot] {
					return nil, errBroker
				}
				if _, exists := slots[event.Slot]; exists {
					return nil, errBroker
				}
				for _, receipt := range slots {
					if receipt.Attempt == event.Attempt || receipt.Journal == event.Journal {
						return nil, errBroker
					}
				}
				pendingWorkflowProvenance = &event
			case "workflow_provenance_get_confirmed":
				if pendingWorkflowProvenance == nil || !sameBrokerWorkflowProvenance(*pendingWorkflowProvenance, event) {
					return nil, errBroker
				}
				workflowProvenanceConfirmed[event.Slot] = event
				pendingWorkflowProvenance = nil
			case "workflow_provenance_get_not_dispatched":
				if workflowProvenanceNoDispatch[event.Slot] || workflowProvenanceConfirmed[event.Slot].Kind != "" {
					return nil, errBroker
				}
				if _, exists := slots[event.Slot]; exists {
					return nil, errBroker
				}
				if pendingWorkflowProvenance != nil && !sameBrokerWorkflowProvenance(*pendingWorkflowProvenance, event) {
					return nil, errBroker
				}
				workflowProvenanceNoDispatch[event.Slot] = true
				pendingWorkflowProvenance = nil
			}
		case "read_token_lifecycle_started", "read_token_lifecycle_confirmed", "read_token_revocation_started", "read_token_revocation_confirmed", "management_token_lifecycle_started", "management_token_lifecycle_confirmed", "read_token_request_not_dispatched", "management_token_request_not_dispatched":
			var event brokerTokenLifecycleEvent
			if decodeBrokerJSON(line, &event, true) != nil || !validBrokerTokenLifecycleEvent(event) {
				return nil, errBroker
			}
			switch event.Kind {
			case "read_token_lifecycle_started", "read_token_revocation_started", "management_token_lifecycle_started":
				old, ok := slots[event.Slot]
				purpose := brokerTokenLifecyclePurpose(event.Kind)
				recorded := readTokenLifecycleSlots[event.Slot] || managementTokenRequestNotDispatchedSlots[event.Slot]
				if purpose == "management" {
					recorded = managementTokenLifecycleSlots[event.Slot] || managementTokenRequestNotDispatchedSlots[event.Slot]
				}
				if pendingTokenLifecycle != nil || recorded || latestSlot != event.Slot || !ok || done[event.Slot] || old.Attempt != event.Attempt || old.Journal != event.Journal {
					return nil, errBroker
				}
				if purpose == "management" {
					managementTokenLifecycleSlots[event.Slot] = true
				} else {
					readTokenLifecycleSlots[event.Slot] = true
				}
				pendingTokenLifecycle = &event
			case "read_token_lifecycle_confirmed", "read_token_revocation_confirmed", "management_token_lifecycle_confirmed":
				if pendingTokenLifecycle == nil || !sameBrokerTokenLifecycle(*pendingTokenLifecycle, event) {
					return nil, errBroker
				}
				pendingTokenLifecycle = nil
			case "read_token_request_not_dispatched":
				old, ok := slots[event.Slot]
				if latestSlot != event.Slot || !ok || done[event.Slot] || readTokenRequestNotDispatchedSlots[event.Slot] || managementTokenRequestNotDispatchedSlots[event.Slot] || old.Attempt != event.Attempt || old.Journal != event.Journal || old.Provenance == nil || old.Provenance.Source != handoff.ReusableCalleeSource {
					return nil, errBroker
				}
				if pendingTokenLifecycle != nil {
					if pendingTokenLifecycle.Kind != "read_token_lifecycle_started" || !sameBrokerTokenIdentity(*pendingTokenLifecycle, event) {
						return nil, errBroker
					}
					pendingTokenLifecycle = nil
				} else if readTokenLifecycleSlots[event.Slot] {
					return nil, errBroker
				}
				readTokenLifecycleSlots[event.Slot] = true
				readTokenRequestNotDispatchedSlots[event.Slot] = true
			case "management_token_request_not_dispatched":
				old, ok := slots[event.Slot]
				if latestSlot != event.Slot || !ok || done[event.Slot] || managementTokenRequestNotDispatchedSlots[event.Slot] || old.Attempt != event.Attempt || old.Journal != event.Journal {
					return nil, errBroker
				}
				if pendingTokenLifecycle != nil {
					if pendingTokenLifecycle.Kind != "management_token_lifecycle_started" || !sameBrokerTokenIdentity(*pendingTokenLifecycle, event) {
						return nil, errBroker
					}
					pendingTokenLifecycle = nil
				} else if managementTokenLifecycleSlots[event.Slot] {
					return nil, errBroker
				}
				managementTokenRequestNotDispatchedSlots[event.Slot] = true
			}
		case "claim", "complete":
			if pendingTokenLifecycle != nil || pendingWorkflowProvenance != nil {
				return nil, errBroker
			}
			var event brokerClaimEvent
			if decodeBrokerJSON(line, &event, true) != nil || !validBrokerClaimEvent(a, event) {
				return nil, errBroker
			}
			switch event.Kind {
			case "claim":
				if event.Provenance != nil {
					nonce := event.Provenance.ReceiptNonce
					if provenanceNonces[nonce] {
						return nil, errBroker
					}
					provenanceNonces[nonce] = true
				}
				for previous, receipt := range slots {
					if (!done[previous] && event.Slot != "inspect" && event.Slot != "cleanup") || receipt.Attempt == event.Attempt || receipt.Journal == event.Journal || (event.Controller != nil && receipt.Controller != nil && receipt.Snapshot == event.Snapshot) || (event.Worker != nil && receipt.Worker != nil && receipt.Worker.State == event.Worker.State) {
						return nil, errBroker
					}
				}
				latestSlot = event.Slot
				if _, ok := slots[event.Slot]; ok {
					return nil, errBroker
				}
				if event.Controller != nil {
					if binding != nil && *binding != *event.Controller {
						return nil, errBroker
					}
					if event.Authority == nil {
						return nil, errBroker
					}
					if !brokerAuthorityTransition(authority, event.Authority) {
						return nil, errBroker
					}
					binding = event.Controller
					if event.Worker != nil {
						if workerBinding != nil && *workerBinding != *event.Worker {
							return nil, errBroker
						}
						workerBinding = event.Worker
					}
					authority = event.Authority
				}
				slots[event.Slot] = event
			case "complete":
				old, ok := slots[event.Slot]
				event.Kind = "claim"
				if !ok || latestSlot != event.Slot || done[event.Slot] || readTokenRequestNotDispatchedSlots[event.Slot] || managementTokenRequestNotDispatchedSlots[event.Slot] || brokerDigest(old) != brokerDigest(event) {
					return nil, errBroker
				}
				done[event.Slot] = true
			}
		default:
			return nil, errBroker
		}
	}
	if pendingTokenLifecycle != nil || pendingWorkflowProvenance != nil {
		return nil, errBroker
	}
	for slot, event := range slots {
		if !done[slot] && !managementTokenLifecycleSlots[slot] && !managementTokenRequestNotDispatchedSlots[slot] {
			// Older incomplete claims do not prove whether a management-token
			// request was accepted. Keep them fail-stopped across upgrades.
			return nil, errBroker
		}
		if !done[slot] && event.Provenance != nil && event.Provenance.Source == handoff.ReusableCalleeSource && !readTokenLifecycleSlots[slot] {
			// Older reusable-callee claims may predate account-wide token
			// lifecycle intents. Their incomplete state cannot prove whether a
			// read-token POST was accepted, so recovery must fail closed.
			return nil, errBroker
		}
		if event.Provenance != nil && event.Provenance.Source == handoff.ReusableCalleeSource {
			confirmed, ok := workflowProvenanceConfirmed[slot]
			if !ok || !brokerClaimMatchesWorkflowProvenance(event, confirmed) {
				// A reusable-callee claim is valid only after the exact
				// authenticated workflow-run GET for this claim was confirmed
				// in account history.
				return nil, errBroker
			}
		}
	}
	slot := a.Phase
	if a.Mode == "discover-actions-host" {
		slot = a.Mode
	} else if a.Mode == "paired-terminal" {
		slot = "paired-terminal"
	}
	if _, ok := slots[slot]; ok || workflowProvenanceNoDispatch[slot] {
		return nil, errBroker
	}
	for previous := range slots {
		if !done[previous] && slot != "inspect" && slot != "cleanup" {
			return nil, errBroker
		}
	}
	if j == nil || j.file == nil || j.root == nil {
		return nil, errBroker
	}
	ji, e := j.file.Stat()
	ri, re := j.root.Stat(".")
	if e != nil || re != nil || j.check() != nil {
		return nil, errBroker
	}
	if ledgerOnly {
		c.ledgerOnly = true
		c.ledgerSlot = slot
		c.ledgerAttempt = brokerFileIdentity(ri)
		c.ledgerJournal = brokerFileIdentity(ji)
		c.workflowProvenanceConfirmed = workflowProvenanceConfirmed
		c.workflowProvenanceNoDispatch = workflowProvenanceNoDispatch
		return c, nil
	}
	c.event = brokerClaimEvent{Kind: "claim", Slot: slot, Attempt: brokerFileIdentity(ri), Journal: brokerFileIdentity(ji)}
	c.workflowProvenanceConfirmed = workflowProvenanceConfirmed
	c.workflowProvenanceNoDispatch = workflowProvenanceNoDispatch
	if p != nil {
		b, e := p.binding()
		if e != nil || (binding != nil && *binding != b) {
			return nil, errBroker
		}
		next := p.authority()
		if !brokerAuthorityTransition(authority, &next) {
			return nil, errBroker
		}
		c.event.Controller = &b
		c.event.Authority = &next
		if p.provenance != nil {
			binding := brokerProvenanceBinding{Digest: brokerDigest(*p.provenance), ReceiptNonce: p.provenance.ReceiptNonce, Source: p.provenance.Source}
			if !binding.valid() || provenanceNonces[binding.ReceiptNonce] {
				return nil, errBroker
			}
			c.event.Provenance = &binding
			if p.provenance.Source == handoff.ReusableCalleeSource {
				expected, e := brokerWorkflowProvenanceEventFor(a, p.controller, j, p.provenance.Source)
				confirmed, ok := workflowProvenanceConfirmed[slot]
				if e != nil || !ok || !sameBrokerWorkflowProvenance(expected, confirmed) {
					return nil, errBroker
				}
			}
		}
		c.event.Snapshot = brokerFileIdentity(p.snapshotInfo)
		c.event.SnapshotDigest = a.ControllerApprovalSHA256
		if p.worker != nil {
			worker, e := p.worker.binding()
			if e != nil || (workerBinding != nil && *workerBinding != worker) {
				return nil, errBroker
			}
			c.event.Worker = &worker
		} else if a.Mode == "paired-terminal" {
			return nil, errBroker
		}
	} else if a.Mode == "controller" {
		return nil, errBroker
	}
	if c.append(c.event) != nil || c.check() != nil {
		return nil, errBroker
	}
	return c, nil
}

func brokerAuthorityTransition(old, next *brokerControllerAuthority) bool {
	if next == nil {
		return false
	}
	if old == nil || old.Digest == next.Digest {
		return true
	}
	if !next.Approval.ExpiresAt.After(old.Approval.ExpiresAt) {
		return false
	}
	for _, phase := range next.Approval.Phases {
		if phase != "inspect" && phase != "cleanup" {
			return false
		}
	}
	return len(next.Approval.Phases) > 0
}
func validBrokerClaimEvent(a BrokerApproval, e brokerClaimEvent) bool {
	if e.Kind != "claim" && e.Kind != "complete" {
		return false
	}
	if e.Attempt.Inode == 0 || e.Journal.Inode == 0 {
		return false
	}
	if e.Slot == "discover-actions-host" {
		return e.Controller == nil && e.Worker == nil && e.Authority == nil && e.Provenance == nil && e.Snapshot == (brokerInode{}) && e.SnapshotDigest == ""
	}
	paired := e.Slot == "paired-terminal"
	if !brokerSlotAllowed(e.Slot) || e.Slot == "discover-actions-host" || (paired && e.Slot != "paired-terminal") || e.Controller == nil || e.Authority == nil || e.Controller.State.Inode == 0 || e.Snapshot.Inode == 0 || !brokerSHA256.MatchString(e.SnapshotDigest) || !brokerSHA256.MatchString(e.Controller.Ownership) || !brokerSHA256.MatchString(e.Controller.Binary) || !brokerSHA40.MatchString(e.Controller.Harness) {
		return false
	}
	if paired {
		if e.Worker == nil || !brokerSHA256.MatchString(e.Worker.Approval) || e.Worker.ApprovalFile.Device == 0 || e.Worker.ApprovalFile.Inode == 0 || e.Worker.State.Device == 0 || e.Worker.State.Inode == 0 {
			return false
		}
	} else if e.Worker != nil {
		return false
	}
	if e.Provenance != nil && !e.Provenance.valid() {
		return false
	}
	c := e.Authority.Approval
	if c.ExpiresAt.IsZero() || e.Authority.Digest != brokerDigest(c) {
		return false
	}
	// Validate a historical event against the mode and slot it records. The
	// current request may be reciprocal (for example, a paired retry after a
	// controller create, or an inspect/cleanup after a failed paired claim), so
	// using its mode here would incorrectly turn current authority into a
	// prerequisite for replaying old ledger records.
	eventApproval := a
	if paired {
		eventApproval.Mode = "paired-terminal"
	} else {
		eventApproval.Mode = "controller"
	}
	eventApproval.Phase = e.Slot
	eventApproval.ExpiresAt = c.ExpiresAt
	eventApproval.ControllerHarnessSHA = e.Controller.Harness
	eventApproval.AllowVerificationAuthority = c.needsVerification()
	validationNow := c.ExpiresAt.Add(-2 * time.Minute)
	if paired {
		// Paired approvals reserve a two-minute completion budget. Validate the
		// historical authority at a point before that budget, rather than at the
		// exact expiry boundary where the current-mode minimum would fail.
		validationNow = c.ExpiresAt.Add(-pairedTerminalMinimumAuthority - time.Second)
	}
	if c.validate(eventApproval, validationNow) != nil {
		return false
	}
	c.ExpiresAt = time.Time{}
	c.Phases = nil
	return e.Controller.Ownership == brokerDigest(c)
}

func validBrokerTokenLifecycleEvent(e brokerTokenLifecycleEvent) bool {
	readTokenEvent := e.Kind == "read_token_lifecycle_started" || e.Kind == "read_token_lifecycle_confirmed" || e.Kind == "read_token_revocation_started" || e.Kind == "read_token_revocation_confirmed" || e.Kind == "read_token_request_not_dispatched"
	managementTokenEvent := e.Kind == "management_token_lifecycle_started" || e.Kind == "management_token_lifecycle_confirmed" || e.Kind == "management_token_request_not_dispatched"
	return (readTokenEvent || managementTokenEvent) && brokerSlotAllowed(e.Slot) &&
		(!readTokenEvent || e.Slot != "discover-actions-host") &&
		e.Attempt.Device != 0 && e.Attempt.Inode != 0 && e.Journal.Device != 0 && e.Journal.Inode != 0
}

func brokerTokenLifecyclePurpose(kind string) string {
	if kind == "management_token_lifecycle_started" || kind == "management_token_lifecycle_confirmed" || kind == "management_token_request_not_dispatched" {
		return "management"
	}
	return "read"
}

func sameBrokerTokenIdentity(started, event brokerTokenLifecycleEvent) bool {
	return started.Slot == event.Slot && started.Attempt == event.Attempt && started.Journal == event.Journal
}

func sameBrokerTokenLifecycle(started, confirmed brokerTokenLifecycleEvent) bool {
	legacyPair := started.Kind == "read_token_revocation_started" && confirmed.Kind == "read_token_revocation_confirmed"
	lifecyclePair := started.Kind == "read_token_lifecycle_started" && confirmed.Kind == "read_token_lifecycle_confirmed"
	managementPair := started.Kind == "management_token_lifecycle_started" && confirmed.Kind == "management_token_lifecycle_confirmed"
	return (legacyPair || lifecyclePair || managementPair) && sameBrokerTokenIdentity(started, confirmed)
}

func brokerWorkflowProvenanceEventFor(a BrokerApproval, c controllerApproval, j *brokerJournal, source string) (brokerWorkflowProvenanceEvent, error) {
	if j == nil || j.file == nil || j.root == nil || (a.Mode != "controller" && a.Mode != "paired-terminal") || !brokerProvenanceSource.MatchString(source) {
		return brokerWorkflowProvenanceEvent{}, errBroker
	}
	ji, e := j.file.Stat()
	ri, re := j.root.Stat(".")
	if e != nil || re != nil || j.check() != nil {
		return brokerWorkflowProvenanceEvent{}, errBroker
	}
	slot := a.Phase
	if a.Mode == "paired-terminal" {
		slot = "paired-terminal"
	}
	event := brokerWorkflowProvenanceEvent{
		Kind:                     "workflow_provenance_get_started",
		Slot:                     slot,
		Attempt:                  brokerFileIdentity(ri),
		Journal:                  brokerFileIdentity(ji),
		ControllerApprovalSHA256: a.ControllerApprovalSHA256,
		Source:                   source,
		WorkflowRunID:            c.WorkflowRunID,
		WorkflowRef:              c.WorkflowRef,
		WorkflowSHA:              c.WorkflowSHA,
		WorkflowPath:             c.WorkflowPath,
	}
	if !validBrokerWorkflowProvenanceEvent(event) {
		return brokerWorkflowProvenanceEvent{}, errBroker
	}
	return event, nil
}

func validBrokerWorkflowProvenanceEvent(e brokerWorkflowProvenanceEvent) bool {
	return (e.Kind == "workflow_provenance_get_started" || e.Kind == "workflow_provenance_get_confirmed" || e.Kind == "workflow_provenance_get_not_dispatched") &&
		brokerSlotAllowed(e.Slot) && e.Slot != "discover-actions-host" &&
		e.Attempt.Device != 0 && e.Attempt.Inode != 0 && e.Journal.Device != 0 && e.Journal.Inode != 0 &&
		brokerSHA256.MatchString(e.ControllerApprovalSHA256) && brokerProvenanceSource.MatchString(e.Source) && e.WorkflowRunID > 0 &&
		brokerWorkflowRef.MatchString(e.WorkflowRef) && brokerSHA40.MatchString(e.WorkflowSHA) && brokerWorkflow.MatchString(e.WorkflowPath)
}

func sameBrokerWorkflowProvenance(started, event brokerWorkflowProvenanceEvent) bool {
	return started.Slot == event.Slot && started.Attempt == event.Attempt && started.Journal == event.Journal &&
		started.ControllerApprovalSHA256 == event.ControllerApprovalSHA256 && started.Source == event.Source &&
		started.WorkflowRunID == event.WorkflowRunID && started.WorkflowRef == event.WorkflowRef &&
		started.WorkflowSHA == event.WorkflowSHA && started.WorkflowPath == event.WorkflowPath &&
		event.Kind != started.Kind
}

func brokerClaimMatchesWorkflowProvenance(claim brokerClaimEvent, confirmed brokerWorkflowProvenanceEvent) bool {
	if claim.Provenance == nil || claim.Authority == nil || confirmed.Kind != "workflow_provenance_get_confirmed" {
		return false
	}
	approval := claim.Authority.Approval
	return confirmed.Slot == claim.Slot && confirmed.Attempt == claim.Attempt && confirmed.Journal == claim.Journal &&
		confirmed.ControllerApprovalSHA256 == claim.SnapshotDigest && confirmed.Source == claim.Provenance.Source &&
		confirmed.WorkflowRunID == approval.WorkflowRunID && confirmed.WorkflowRef == approval.WorkflowRef &&
		confirmed.WorkflowSHA == approval.WorkflowSHA && confirmed.WorkflowPath == approval.WorkflowPath
}
