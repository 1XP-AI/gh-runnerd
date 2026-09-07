package liveworker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
	"syscall"
)

const (
	maxPairBinding = 8 << 10
	maxPairRecord  = 16 << 10
	maxJournal     = 1 << 20
	pairCallRoom   = 2 * maxPairRecord
	pairDeleteRoom = 3 * pairCallRoom
)

var pairSourceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,99}$`)
var pairWorkflowPath = regexp.MustCompile(`^\.github/workflows/[a-zA-Z0-9_-]+\.ya?ml$`)

func validRef(r RecordRef) bool { return r.Sequence > 0 && id.MatchString(r.EventSHA256) }
func validIdentity(i JournalIdentity) bool {
	if !id.MatchString(i.OwnershipSHA256) {
		return false
	}
	seen := map[FileIdentity]bool{}
	for _, f := range []FileIdentity{i.State, i.Journal, i.AdmissionDirectory, i.Claim} {
		if f.Device == 0 || f.Inode == 0 || seen[f] {
			return false
		}
		seen[f] = true
	}
	return true
}

func validPairInput(in PairInput) bool {
	s := in.Source
	return validIdentity(in.Controller) && pairSourceName.MatchString(s.Organization) && pairSourceName.MatchString(s.Repository) &&
		s.Organization != "." && s.Organization != ".." && s.Repository != "." && s.Repository != ".." &&
		s.RepositoryID > 0 && s.WorkflowRunID > 0 && pairWorkflowPath.MatchString(s.WorkflowPath) && sha.MatchString(s.HeadSHA) && s.Attempt == 1 &&
		in.RunnerGroupID > 0 && in.ScaleSetID > 0 && nonce.MatchString(in.OwnerNonce) && in.ScaleSetName == "g01-"+in.OwnerNonce &&
		validRef(in.SetCreation) && component.MatchString(in.ControllerName) && sha.MatchString(in.HarnessSHA)
}

func validBinding(b PairBinding) bool {
	if b.Version != 1 || !validPairInput(b.Input) || !validIdentity(b.Worker) {
		return false
	}
	for _, w := range []FileIdentity{b.Worker.State, b.Worker.Journal, b.Worker.AdmissionDirectory, b.Worker.Claim} {
		for _, c := range []FileIdentity{b.Input.Controller.State, b.Input.Controller.Journal, b.Input.Controller.AdmissionDirectory, b.Input.Controller.Claim} {
			if w == c {
				return false
			}
		}
	}
	data, err := json.Marshal(b)
	return err == nil && len(data) <= maxPairBinding
}

func pairHash(b PairBinding) string {
	data, _ := json.Marshal(b)
	h := sha256.New()
	_, _ = h.Write([]byte("gh-runnerd/g01-pair/binding/v1\x00"))
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func workerEventRef(identity JournalIdentity, e Event) RecordRef {
	identityData, _ := json.Marshal(identity)
	eventData, _ := json.Marshal(e)
	h := sha256.New()
	_, _ = h.Write([]byte("gh-runnerd/g01-pair/worker-event/v1\x00"))
	_, _ = h.Write(identityData)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(eventData)
	return RecordRef{e.Sequence, hex.EncodeToString(h.Sum(nil))}
}

func identityOf(info os.FileInfo) FileIdentity {
	stat := info.Sys().(*syscall.Stat_t)
	return FileIdentity{uint64(stat.Dev), stat.Ino}
}

// Called only with the initialized concrete journal's execution lease held.
func (j *FileJournal) pairedIdentity() JournalIdentity {
	return JournalIdentity{j.ownership, identityOf(j.directoryInfo), identityOf(j.fileInfo), identityOf(j.claim.rootInfo), identityOf(j.claim.fileInfo)}
}

func validHandoff(h HandoffReceipt, b PairBinding, pair PairReceipt) bool {
	return h.PairSHA256 == pair.PairSHA256 && validRef(h.PairResult) && validRef(h.AcquireResult) && validRef(h.JITResult) && validRef(h.HandoffIntent) &&
		h.PairResult.Sequence > pair.ControllerIntent.Sequence && h.AcquireResult.Sequence > h.PairResult.Sequence &&
		h.JITResult.Sequence > h.AcquireResult.Sequence && h.HandoffIntent.Sequence > h.JITResult.Sequence &&
		h.RequestID > 0 && h.Runner.ID > 0 && h.Runner.Name == "g01-"+b.Input.OwnerNonce+"-worker-1" && h.Runner.ScaleSetID == b.Input.ScaleSetID
}

func cloneFacts(f *DockerStateFacts) *DockerStateFacts {
	if f == nil {
		return nil
	}
	copy := *f
	for _, pointer := range []**bool{&copy.Running, &copy.Paused, &copy.Restarting, &copy.Dead} {
		if *pointer != nil {
			value := **pointer
			*pointer = &value
		}
	}
	if copy.ExitCode != nil {
		value := *copy.ExitCode
		copy.ExitCode = &value
	}
	return &copy
}

func cloneLocal(r LocalReceipt) LocalReceipt { r.State = cloneFacts(r.State); return r }
func cloneDeletion(r DeletionReceipt) DeletionReceipt {
	if r.AbsenceResult != nil {
		copy := *r.AbsenceResult
		r.AbsenceResult = &copy
	}
	return r
}
