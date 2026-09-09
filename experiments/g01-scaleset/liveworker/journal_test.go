package liveworker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private fixture failed")
	}
	return dir
}

func TestPrivateJournalLocksAndRetainsReservationAcrossRestart(t *testing.T) {
	dir := privateDir(t)
	a := approval()
	j, err := openTestJournal(t, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := openTestJournal(t, dir, a); err == nil {
		second.Close()
		t.Fatal("second controller took private journal")
	}
	_, intent, err := a.creation(ImageProfile{}, syntheticJIT)
	if err != nil {
		t.Fatal(err)
	}
	intent.Kind = "intent"
	intent.Operation = "create"
	if j.Append(intent) != nil {
		t.Fatal("intent could not persist")
	}
	j.Close()
	reopened, err := openTestJournal(t, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	f := &fakeRuntime{}
	d := Driver{a, reopened, f}
	if d.Run(context.Background(), "create", syntheticJIT) == nil || f.creates.Load() != 0 || !replay(reopened.Events()).uncertain {
		t.Fatal("incomplete creation intent lost its reservation")
	}
	data, err := os.ReadFile(filepath.Join(dir, "journal.jsonl"))
	if err != nil || strings.Contains(string(data), syntheticJIT) {
		t.Fatal("private journal contains raw JIT")
	}
}

func TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect(t *testing.T) {
	dir := privateDir(t)
	a := approval()
	open := func(path string, approval Approval) (*FileJournal, error) {
		return openTestJournal(t, path, approval)
	}
	receipt, err := prepareJournal(dir, a, open)
	if err != nil {
		t.Fatal("fresh worker preparation refused", err)
	}
	if receipt.Version != 1 || receipt.Status != "worker_journal_prepared" || receipt.Phase != "paired-worker" || receipt.ApprovalDigest != approvalDigest(a) || receipt.State.Inode == 0 || receipt.Journal.Inode == 0 || receipt.Claim.Inode == 0 || receipt.AdmissionDirectory.Inode == 0 || !id.MatchString(receipt.JournalDigest) || !id.MatchString(receipt.ClaimDigest) {
		t.Fatalf("incomplete worker preparation receipt: %+v", receipt)
	}
	j, err := openTestJournal(t, dir, a)
	if err != nil {
		t.Fatal("reopen prepared worker journal", err)
	}
	if err := j.Append(Event{Kind: "intent", Operation: "create"}); err != nil {
		t.Fatal("persist prior worker intent", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal("close prior worker journal", err)
	}
	if _, err := prepareJournal(dir, a, open); err == nil {
		t.Fatal("worker preparation adopted prior effect")
	}
}

func TestJournalRejectsChangedApprovalTornTailAndUnsafeFiles(t *testing.T) {
	for _, fault := range []string{"endpoint", "daemon", "image", "workflow", "tail", "symlink", "hardlink", "mode"} {
		t.Run(fault, func(t *testing.T) {
			dir := privateDir(t)
			a := approval()
			j, err := openTestJournal(t, dir, a)
			if err != nil {
				t.Fatal(err)
			}
			j.Close()
			path := filepath.Join(dir, "journal.jsonl")
			switch fault {
			case "endpoint":
				a.Endpoint = "/other/docker.sock"
			case "daemon":
				a.DaemonID = "other-daemon"
			case "image":
				a.ImageID = "sha256:" + strings.Repeat("a", 64)
			case "workflow":
				a.WorkflowSHA = strings.Repeat("b", 40)
			case "tail":
				f, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
				f.WriteString(`{"sequence":1`)
				f.Close()
			case "symlink":
				if os.Rename(path, path+".owned") != nil || os.Symlink(path+".owned", path) != nil {
					t.Fatal("fixture failed")
				}
			case "hardlink":
				if os.Link(path, path+".alias") != nil {
					t.Fatal("fixture failed")
				}
			case "mode":
				os.Chmod(path, 0644)
			}
			if opened, err := openTestJournal(t, dir, a); err == nil {
				opened.Close()
				t.Fatal("unsafe journal accepted")
			}
		})
	}
}

func TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart(t *testing.T) {
	j := &memoryJournal{}
	f := &fakeRuntime{warnings: true}
	d := Driver{approval(), j, f}
	if d.Run(context.Background(), "create", syntheticJIT) == nil {
		t.Fatal("warning ignored")
	}
	s := replay(j.Events())
	if s.id != f.container.ID || !s.uncertain {
		t.Fatal("known warning response identity lost")
	}
	if d.Run(context.Background(), "start", "") == nil || f.starts.Load() != 0 {
		t.Fatal("warned worker started")
	}
	if d.Run(context.Background(), "inspect", "") != nil || !replay(j.Events()).uncertain {
		t.Fatal("inspection cleared uncertainty")
	}
}

func TestObservationDoesNotJournalRawRuntimeStatus(t *testing.T) {
	d, f, j := created(t)
	f.container.State.Status = "synthetic-private-error"
	if d.Run(context.Background(), "inspect", "") != nil {
		t.Fatal("read observation failed")
	}
	events := j.Events()
	if events[len(events)-1].Status != "unknown" {
		t.Fatal("raw status persisted")
	}
}

func TestStrictInputRejectsAmbiguousOrExtraAuthorityFields(t *testing.T) {
	var a Approval
	for _, data := range []string{`{"daemon_id":"one","daemon_id":"two"}`, `{"unknown_authority":true}`, `{} {}`} {
		if DecodeStrict([]byte(data), &a) == nil {
			t.Fatal("ambiguous authority input accepted")
		}
	}
}

func testAdmissionDirectory(t *testing.T, directory string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(directory), "admission")
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		t.Fatal("fixture admission directory")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal("fixture admission identity")
	}
	return canonical
}
func openTestJournal(t *testing.T, directory string, a Approval) (*FileJournal, error) {
	t.Helper()
	return openJournalAtAdmission(directory, a, testAdmissionDirectory(t, directory), func(f *os.File) error { return f.Sync() })
}
