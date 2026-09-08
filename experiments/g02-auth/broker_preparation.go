package enrollment

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type brokerPreparationReceipt struct {
	Version        int         `json:"version"`
	Status         string      `json:"status"`
	Phase          string      `json:"phase"`
	ApprovalDigest string      `json:"approval_digest"`
	State          brokerInode `json:"state"`
	Journal        brokerInode `json:"journal"`
	Claim          brokerInode `json:"claim"`
	JournalDigest  string      `json:"journal_digest"`
	ClaimDigest    string      `json:"claim_digest"`
}

func (r brokerPreparationReceipt) valid(p *brokerControllerPlan) bool {
	if p == nil {
		return false
	}
	phase := p.approval.Phase
	if p.approval.Mode == "paired-terminal" {
		// Paired preparation is its own local authority contract. It is not a
		// cleanup receipt lending cleanup authority to the full pair.
		phase = "paired-terminal"
	}
	return r.Version == 1 && r.Status == "controller_journal_prepared" && r.Phase == phase && r.ApprovalDigest == brokerDigest(p.controller) && r.State.Device != 0 && r.State.Inode != 0 && r.Journal.Device != 0 && r.Journal.Inode != 0 && r.Claim.Device != 0 && r.Claim.Inode != 0 && brokerSHA256.MatchString(r.JournalDigest) && brokerSHA256.MatchString(r.ClaimDigest)
}

type brokerPreparationOutput struct {
	mu       sync.Mutex
	data     []byte
	cancel   context.CancelFunc
	overflow bool
}

func (o *brokerPreparationOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.data)+len(p) > 4096 {
		o.overflow = true
		o.cancel()
		return 0, errBroker
	}
	o.data = append(o.data, p...)
	return len(p), nil
}
func invokeBrokerPreparation(parent context.Context, binary *verifiedBrokerBinary, workingDirectory, approvalPath, stateDirectory, phase string) (receipt brokerPreparationReceipt, err error) {
	if !brokerPhases[phase] || binary.check() != nil {
		return receipt, errBroker
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary.path, "--prepare-approved-journal", "--approval", approvalPath, "--state-dir", stateDirectory, "--phase", phase)
	command.Dir = workingDirectory
	command.Env = []string{"LANG=C", "LC_ALL=C"}
	command.Stdin = bytes.NewReader(nil)
	command.WaitDelay = time.Second
	output := &brokerPreparationOutput{cancel: cancel}
	errors := &brokerOutputBudget{cancel: cancel}
	command.Stdout = output
	command.Stderr = errors
	e := command.Run()
	output.mu.Lock()
	defer output.mu.Unlock()
	errors.mu.Lock()
	defer errors.mu.Unlock()
	if e != nil || ctx.Err() != nil || output.overflow || errors.overflow || decodeBrokerJSON(output.data, &receipt, true) != nil {
		return receipt, errBroker
	}
	return receipt, nil
}

// invokeBrokerPairedPreparation uses a distinct executable contract. The
// paired preparation receipt is not a cleanup receipt and carries no child
// credentials or worker authority.
func invokeBrokerPairedPreparation(parent context.Context, binary *verifiedBrokerBinary, workingDirectory, approvalPath, stateDirectory string) (receipt brokerPreparationReceipt, err error) {
	if binary == nil || binary.check() != nil || !filepath.IsAbs(approvalPath) || !filepath.IsAbs(stateDirectory) || filepath.Clean(approvalPath) != approvalPath || filepath.Clean(stateDirectory) != stateDirectory {
		return receipt, errBroker
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary.path, "--prepare-approved-paired-journal", "--approval", approvalPath, "--state-dir", stateDirectory)
	command.Dir = workingDirectory
	command.Env = []string{"LANG=C", "LC_ALL=C"}
	command.Stdin = bytes.NewReader(nil)
	command.WaitDelay = time.Second
	output := &brokerPreparationOutput{cancel: cancel}
	errors := &brokerOutputBudget{cancel: cancel}
	command.Stdout = output
	command.Stderr = errors
	e := command.Run()
	output.mu.Lock()
	defer output.mu.Unlock()
	errors.mu.Lock()
	defer errors.mu.Unlock()
	if e != nil || ctx.Err() != nil || output.overflow || errors.overflow || decodeBrokerJSON(output.data, &receipt, true) != nil {
		return receipt, errBroker
	}
	return receipt, nil
}

type brokerPreparedState struct {
	journal, claim             *os.File
	journalInfo, claimInfo     os.FileInfo
	journalDigest, claimDigest string
}

func (s *brokerPreparedState) close() {
	if s == nil {
		return
	}
	if s.journal != nil {
		s.journal.Close()
	}
	if s.claim != nil {
		s.claim.Close()
	}
}

// Each short check shares the canonical initialization lease, then validates
// both exact files under nonblocking exclusive leases. These are released before
// the child; same-UID mutation after a released check remains outside this proof.
func (p *brokerControllerPlan) preparedState(root *os.Root, capture bool) error {
	directory, e := root.Open(".")
	if e != nil {
		return errBroker
	}
	defer directory.Close()
	if syscall.Flock(int(directory.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errBroker
	}
	s := p.prepared
	if capture {
		if s != nil {
			return errBroker
		}
		s = &brokerPreparedState{}
		s.claim, e = root.OpenFile("admission.json", os.O_RDWR|syscall.O_NOFOLLOW, 0)
		if e != nil {
			s.close()
			return errBroker
		}
		s.journal, e = p.state.OpenFile("journal.jsonl", os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if e != nil {
			s.close()
			return errBroker
		}
	} else if s == nil {
		return errBroker
	}
	kept := false
	defer func() {
		if capture && !kept {
			s.close()
		}
	}()
	if !privateFile(s.claim) || !privateFile(s.journal) || syscall.Flock(int(s.claim.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errBroker
	}
	defer syscall.Flock(int(s.claim.Fd()), syscall.LOCK_UN)
	if syscall.Flock(int(s.journal.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errBroker
	}
	defer syscall.Flock(int(s.journal.Fd()), syscall.LOCK_UN)
	ji, je := s.journal.Stat()
	ci, ce := s.claim.Stat()
	jn, jne := p.state.Lstat("journal.jsonl")
	cn, cne := root.Lstat("admission.json")
	if je != nil || ce != nil || jne != nil || cne != nil || !os.SameFile(ji, jn) || !os.SameFile(ci, cn) || ji.Size() < 1 || ji.Size() > 1<<20 || ci.Size() < 1 || ci.Size() > 4096 {
		return errBroker
	}
	jb, e := io.ReadAll(io.NewSectionReader(s.journal, 0, (1<<20)+1))
	if e != nil {
		return errBroker
	}
	cb, e := io.ReadAll(io.NewSectionReader(s.claim, 0, 4097))
	if e != nil {
		return errBroker
	}
	jd, cd := brokerBytesDigest(jb), brokerBytesDigest(cb)
	expected := p.preparationReceipt
	if !expected.valid(p) || expected.State != brokerFileIdentity(p.stateInfo) || expected.Journal != brokerFileIdentity(ji) || expected.Claim != brokerFileIdentity(ci) || expected.JournalDigest != jd || expected.ClaimDigest != cd {
		return errBroker
	}

	if capture {
		s.journalInfo = ji
		s.claimInfo = ci
		s.journalDigest = jd
		s.claimDigest = cd
		p.prepared = s
		kept = true
		return nil
	}
	if !os.SameFile(ji, s.journalInfo) || !os.SameFile(ci, s.claimInfo) || jd != s.journalDigest || cd != s.claimDigest {
		return errBroker
	}
	return nil
}
