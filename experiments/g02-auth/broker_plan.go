package enrollment

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const pairedWorkerImage = "ghcr.io/actions/actions-runner@sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d"

type pairedWorkerApproval struct {
	RunnerUpdatesDisabled bool      `json:"runner_updates_disabled"`
	HarnessSHA            string    `json:"harness_sha"`
	WorkflowSHA           string    `json:"workflow_sha"`
	OwnerNonce            string    `json:"owner_nonce"`
	Controller            string    `json:"controller"`
	Endpoint              string    `json:"endpoint"`
	DaemonID              string    `json:"daemon_id"`
	ImageID               string    `json:"image_id"`
	Image                 string    `json:"image"`
	ExpiresAt             time.Time `json:"expires_at"`
	Phases                []string  `json:"phases"`
}

func (a pairedWorkerApproval) validate(now time.Time) error {
	if !a.RunnerUpdatesDisabled || !brokerSHA40.MatchString(a.HarnessSHA) || !brokerSHA40.MatchString(a.WorkflowSHA) || !brokerNonce.MatchString(a.OwnerNonce) || !brokerComponent.MatchString(a.Controller) || !brokerWorkerComponent.MatchString(a.DaemonID) || !strings.HasPrefix(a.ImageID, "sha256:") || !brokerSHA256.MatchString(strings.TrimPrefix(a.ImageID, "sha256:")) || a.Image != pairedWorkerImage || !filepath.IsAbs(a.Endpoint) || filepath.Clean(a.Endpoint) != a.Endpoint || len(a.Endpoint) > 103 || strings.ContainsAny(a.Endpoint, "\x00\r\n") || !a.ExpiresAt.After(now) || a.ExpiresAt.After(now.Add(24*time.Hour)) || len(a.Phases) != 4 {
		return errBroker
	}
	seen := map[string]bool{}
	for _, phase := range a.Phases {
		if phase != "create" && phase != "start" && phase != "inspect" && phase != "cleanup" || seen[phase] {
			return errBroker
		}
		seen[phase] = true
	}
	return nil
}

type brokerWorkerPlan struct {
	approval     pairedWorkerApproval
	raw          []byte
	approvalPath string
	approvalFile *os.File
	approvalInfo os.FileInfo
	statePath    string
	state        *os.Root
	stateInfo    os.FileInfo
}

// brokerPairedBinding is immutable identity evidence carried to the child.
// It does not describe or authorize pairing; the controller approval and the
// journal-derived PairInput remain the canonical authority in G01.
type brokerPairedBinding struct {
	ControllerApprovalSHA256 string `json:"controller_approval_sha256"`
	ControllerApprovalDevice uint64 `json:"controller_approval_device"`
	ControllerApprovalInode  uint64 `json:"controller_approval_inode"`
	ControllerStateDevice    uint64 `json:"controller_state_device"`
	ControllerStateInode     uint64 `json:"controller_state_inode"`
	WorkerApprovalSHA256     string `json:"worker_approval_sha256"`
	WorkerApprovalDevice     uint64 `json:"worker_approval_device"`
	WorkerApprovalInode      uint64 `json:"worker_approval_inode"`
	WorkerStateDevice        uint64 `json:"worker_state_device"`
	WorkerStateInode         uint64 `json:"worker_state_inode"`
}

func (b brokerPairedBinding) valid() bool {
	return brokerSHA256.MatchString(b.ControllerApprovalSHA256) && b.ControllerApprovalDevice != 0 && b.ControllerApprovalInode != 0 && b.ControllerStateDevice != 0 && b.ControllerStateInode != 0 && brokerSHA256.MatchString(b.WorkerApprovalSHA256) && b.WorkerApprovalDevice != 0 && b.WorkerApprovalInode != 0 && b.WorkerStateDevice != 0 && b.WorkerStateInode != 0
}

func (p *brokerWorkerPlan) close() {
	if p == nil {
		return
	}
	if p.approvalFile != nil {
		_ = p.approvalFile.Close()
	}
	if p.state != nil {
		_ = p.state.Close()
	}
}

func (p *brokerWorkerPlan) check() error {
	if p == nil || p.approvalFile == nil || p.state == nil || !filepath.IsAbs(p.approvalPath) || filepath.Clean(p.approvalPath) != p.approvalPath || !filepath.IsAbs(p.statePath) || filepath.Clean(p.statePath) != p.statePath {
		return errBroker
	}
	approvalInfo, err := p.approvalFile.Stat()
	namedApproval, namedErr := os.Lstat(p.approvalPath)
	stateInfo, stateErr := p.state.Stat(".")
	namedState, namedStateErr := os.Lstat(p.statePath)
	if err != nil || namedErr != nil || stateErr != nil || namedStateErr != nil || !privateFile(p.approvalFile) || !brokerOwnedDirectory(namedState, true) || !os.SameFile(approvalInfo, p.approvalInfo) || !os.SameFile(namedApproval, p.approvalInfo) || !os.SameFile(stateInfo, p.stateInfo) || !os.SameFile(namedState, p.stateInfo) {
		return errBroker
	}
	data, err := io.ReadAll(io.NewSectionReader(p.approvalFile, 0, 16385))
	if err != nil || len(data) != len(p.raw) || brokerBytesDigest(data) != brokerBytesDigest(p.raw) {
		return errBroker
	}
	return nil
}

func (p *brokerWorkerPlan) binding() (brokerWorkerBinding, error) {
	if p.check() != nil {
		return brokerWorkerBinding{}, errBroker
	}
	return brokerWorkerBinding{Approval: brokerBytesDigest(p.raw), ApprovalFile: brokerFileIdentity(p.approvalInfo), State: brokerFileIdentity(p.stateInfo)}, nil
}

func openBrokerWorkerPlan(path, statePath, controllerStatePath string, a BrokerApproval, c controllerApproval) (*brokerWorkerPlan, error) {
	if a.Mode != "paired-terminal" || !filepath.IsAbs(path) || filepath.Clean(path) != path || !filepath.IsAbs(statePath) || filepath.Clean(statePath) != statePath || filepath.Clean(statePath) == filepath.Clean(controllerStatePath) {
		return nil, errBroker
	}
	controllerReal, err := filepath.EvalSymlinks(controllerStatePath)
	workerReal, workerErr := filepath.EvalSymlinks(statePath)
	if err != nil || workerErr != nil || controllerReal == workerReal {
		return nil, errBroker
	}
	file, err := openBrokerPrivateFile(path, 0600, 16384)
	if err != nil {
		return nil, errBroker
	}
	data, err := io.ReadAll(io.NewSectionReader(file, 0, 16385))
	if err != nil || len(data) > 16384 {
		file.Close()
		return nil, errBroker
	}
	var worker pairedWorkerApproval
	if decodeBrokerJSON(data, &worker, true) != nil || worker.validate(time.Now()) != nil || worker.OwnerNonce != c.OwnerNonce || worker.HarnessSHA != c.HarnessSHA || worker.WorkflowSHA != c.WorkflowSHA || worker.Controller != c.Controller || !worker.ExpiresAt.Equal(c.ExpiresAt) {
		file.Close()
		return nil, errBroker
	}
	root, err := openBrokerPrivateDirectory(statePath)
	if err != nil {
		file.Close()
		return nil, errBroker
	}
	stateInfo, err := root.Stat(".")
	if err != nil {
		file.Close()
		root.Close()
		return nil, errBroker
	}
	approvalInfo, err := file.Stat()
	if err != nil {
		file.Close()
		root.Close()
		return nil, errBroker
	}
	plan := &brokerWorkerPlan{approval: worker, raw: data, approvalPath: path, approvalFile: file, approvalInfo: approvalInfo, statePath: statePath, state: root, stateInfo: stateInfo}
	if plan.check() != nil {
		plan.close()
		return nil, errBroker
	}
	return plan, nil
}

// Private prepared inputs are constructed only after the front door verifies
// source, binary, authority and private paths. Test launchers are synthetic.
type brokerControllerPlan struct {
	approval           BrokerApproval
	controller         controllerApproval
	raw                []byte
	state              *os.Root
	statePath          string
	stateInfo          os.FileInfo
	snapshot           *os.File
	snapshotInfo       os.FileInfo
	journal            *brokerJournal
	binaryCheck        func() error
	localPrepare       func(context.Context, string) (brokerPreparationReceipt, error)
	preparationReceipt brokerPreparationReceipt
	prepared           *brokerPreparedState
	worker             *brokerWorkerPlan
	launch             func(context.Context, []byte, string) error
}

func (p *brokerControllerPlan) close() {
	if p != nil {
		p.prepared.close()
	}
	if p != nil && p.snapshot != nil {
		p.snapshot.Close()
	}
	if p != nil {
		p.worker.close()
	}
}
func (p *brokerControllerPlan) authority() brokerControllerAuthority {
	return brokerControllerAuthority{brokerDigest(p.controller), p.controller}
}
func (p *brokerControllerPlan) binding() (brokerControllerBinding, error) {
	if p == nil || p.state == nil {
		return brokerControllerBinding{}, errBroker
	}
	i, e := p.state.Stat(".")
	named, ne := os.Lstat(p.statePath)
	if e != nil || ne != nil || !brokerOwnedDirectory(named, true) || !os.SameFile(i, named) || !os.SameFile(i, p.stateInfo) {
		return brokerControllerBinding{}, errBroker
	}
	c := p.controller
	c.ExpiresAt = time.Time{}
	c.Phases = nil
	return brokerControllerBinding{brokerDigest(c), p.approval.ControllerBinarySHA256, p.approval.ControllerHarnessSHA, brokerFileIdentity(i)}, nil
}

func (p *brokerControllerPlan) pairedBinding() (brokerPairedBinding, error) {
	if p == nil || p.approval.Mode != "paired-terminal" || p.snapshotInfo == nil || p.worker == nil || p.check() != nil {
		return brokerPairedBinding{}, errBroker
	}
	worker, err := p.worker.binding()
	if err != nil {
		return brokerPairedBinding{}, errBroker
	}
	controllerApproval := brokerFileIdentity(p.snapshotInfo)
	controllerState := brokerFileIdentity(p.stateInfo)
	binding := brokerPairedBinding{ControllerApprovalSHA256: p.approval.ControllerApprovalSHA256, ControllerApprovalDevice: controllerApproval.Device, ControllerApprovalInode: controllerApproval.Inode, ControllerStateDevice: controllerState.Device, ControllerStateInode: controllerState.Inode, WorkerApprovalSHA256: worker.Approval, WorkerApprovalDevice: worker.ApprovalFile.Device, WorkerApprovalInode: worker.ApprovalFile.Inode, WorkerStateDevice: worker.State.Device, WorkerStateInode: worker.State.Inode}
	if !binding.valid() {
		return brokerPairedBinding{}, errBroker
	}
	return binding, nil
}
func (p *brokerControllerPlan) prepare(a BrokerApproval, j *brokerJournal, now time.Time) error {
	if p == nil || p.binaryCheck == nil || p.launch == nil || p.localPrepare == nil || brokerDigest(p.approval) != brokerDigest(a) || p.controller.validate(a, now) != nil || brokerBytesDigest(p.raw) != a.ControllerApprovalSHA256 || p.binaryCheck() != nil || (a.Mode == "paired-terminal" && p.worker == nil) {
		return errBroker
	}
	if p.worker != nil && p.worker.check() != nil {
		return errBroker
	}
	if _, e := p.binding(); e != nil {
		return errBroker
	}
	p.journal = j
	f, e := j.root.OpenFile("controller-approval.json", os.O_RDWR|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return errBroker
	}
	p.snapshot = f
	if n, e := f.Write(p.raw); e != nil || n != len(p.raw) || f.Sync() != nil || syncDirectory(j.root) != nil {
		return errBroker
	}
	p.snapshotInfo, e = f.Stat()
	if e != nil {
		return errBroker
	}
	return p.check()
}
func (p *brokerControllerPlan) check() error {
	if _, e := p.binding(); e != nil {
		return errBroker
	}
	if p.binaryCheck() != nil || p.snapshot == nil || p.journal.check() != nil || !privateFile(p.snapshot) || (p.worker != nil && p.worker.check() != nil) {
		return errBroker
	}
	named, e := p.journal.root.Lstat("controller-approval.json")
	i, ie := p.snapshot.Stat()
	if e != nil || ie != nil || !os.SameFile(named, p.snapshotInfo) || !os.SameFile(i, p.snapshotInfo) || i.Size() != int64(len(p.raw)) {
		return errBroker
	}
	data, e := io.ReadAll(io.NewSectionReader(p.snapshot, 0, 16385))
	if e != nil || brokerBytesDigest(data) != p.approval.ControllerApprovalSHA256 {
		return errBroker
	}
	return nil
}

// Read-only bridge to the controller's version-1 permanent admission contract.
// Its lock is released before the child starts; the broker has a distinct lock.
// This detects present conflicts, not hostile same-UID races after release.
func (p *brokerControllerPlan) compatibleControllerClaim(root *os.Root) error {
	// Share the controller/worker short initialization lease before even opening
	// their claim. Never contend for a newly created, not-yet-locked empty claim.
	directory, e := root.Open(".")
	if e != nil {
		return errBroker
	}
	defer directory.Close()
	if syscall.Flock(int(directory.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errBroker
	}

	binding, e := p.binding()
	if e != nil {
		return errBroker
	}
	f, e := root.OpenFile("admission.json", os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if os.IsNotExist(e) {
		_, je := p.state.Lstat("journal.jsonl")
		if os.IsNotExist(je) {
			return nil
		}
		return errBroker
	}
	if e != nil {
		return errBroker
	}
	defer f.Close()
	if !privateFile(f) || syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errBroker
	}
	info, e := f.Stat()
	named, ne := root.Lstat("admission.json")
	if e != nil || ne != nil || !os.SameFile(info, named) || info.Size() > 4096 {
		return errBroker
	}
	data, e := io.ReadAll(io.NewSectionReader(f, 0, 4097))
	var record struct {
		Version       int    `json:"version"`
		Ownership     string `json:"ownership"`
		StateDevice   uint64 `json:"state_device"`
		StateInode    uint64 `json:"state_inode"`
		JournalDevice uint64 `json:"journal_device"`
		JournalInode  uint64 `json:"journal_inode"`
	}
	if e != nil || decodeBrokerJSON(data, &record, true) != nil || record.Version != 1 || record.Ownership != binding.Ownership || record.StateDevice != binding.State.Device || record.StateInode != binding.State.Inode {
		return errBroker
	}
	journal, e := p.state.OpenFile("journal.jsonl", os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return errBroker
	}
	defer journal.Close()
	ji, e := journal.Stat()
	if e != nil || !privateFile(journal) || ji.Size() < 1 {
		return errBroker
	}
	id := brokerFileIdentity(ji)
	if id.Device != record.JournalDevice || id.Inode != record.JournalInode {
		return errBroker
	}
	// Credential-free canonical preparation validates full authority/replay
	// before authenticated work; this bridge only checks shared claim identity.
	return nil
}

func newBrokerControllerPlan(a BrokerApproval, c controllerApproval, raw []byte, root *os.Root, statePath string, check func() error, launch func(context.Context, []byte, string) error) (*brokerControllerPlan, error) {
	info, e := root.Stat(".")
	if e != nil {
		return nil, errBroker
	}
	return &brokerControllerPlan{approval: a, controller: c, raw: raw, state: root, statePath: filepath.Clean(statePath), stateInfo: info, binaryCheck: check, launch: launch}, nil
}
