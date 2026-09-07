package enrollment

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Private prepared inputs are constructed only after the front door verifies
// source, binary, authority and private paths. Test launchers are synthetic.
type brokerControllerPlan struct {
	approval     BrokerApproval
	controller   controllerApproval
	raw          []byte
	state        *os.Root
	statePath    string
	stateInfo    os.FileInfo
	snapshot     *os.File
	snapshotInfo os.FileInfo
	journal      *brokerJournal
	binaryCheck  func() error
	launch       func(context.Context, []byte, string) error
}

func (p *brokerControllerPlan) close() {
	if p != nil && p.snapshot != nil {
		p.snapshot.Close()
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
func (p *brokerControllerPlan) prepare(a BrokerApproval, j *brokerJournal, now time.Time) error {
	if p == nil || p.binaryCheck == nil || p.launch == nil || brokerDigest(p.approval) != brokerDigest(a) || p.controller.validate(a, now) != nil || brokerBytesDigest(p.raw) != a.ControllerApprovalSHA256 || p.binaryCheck() != nil {
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
	if p.binaryCheck() != nil || p.snapshot == nil || p.journal.check() != nil || !privateFile(p.snapshot) {
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
	// Full authority/replay validation remains the reviewed controller's job.
	return nil
}

func newBrokerControllerPlan(a BrokerApproval, c controllerApproval, raw []byte, root *os.Root, statePath string, check func() error, launch func(context.Context, []byte, string) error) (*brokerControllerPlan, error) {
	info, e := root.Stat(".")
	if e != nil {
		return nil, errBroker
	}
	return &brokerControllerPlan{approval: a, controller: c, raw: raw, state: root, statePath: filepath.Clean(statePath), stateInfo: info, binaryCheck: check, launch: launch}, nil
}
