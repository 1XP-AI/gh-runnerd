//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

type terminalFixture struct {
	controllerResponse func(string, any) any
	*pairedIntegrationFixture
	sessionDeletes, workerDeletes, setDeletes, setAbsences, workerAbsences, rosters atomic.Int32
	deletedSet, deletedWorker                                                       atomic.Bool
}

func newTerminalFixture(t *testing.T, cleanup bool) *terminalFixture {
	phases := []string{"create", "start", "inspect"}
	if cleanup {
		phases = append(phases, "cleanup")
	}
	return newTerminalFixtureWithPhases(t, phases)
}
func newTerminalFixtureWithPhases(t *testing.T, phases []string) *terminalFixture {
	return newTerminalFixtureWithControllerPhases(t, phases, nil)
}
func newTerminalFixtureWithControllerPhases(t *testing.T, phases, controllerPhases []string) *terminalFixture {
	t.Helper()
	f := &terminalFixture{}
	config := &pairedFixtureConfiguration{controllerResponse: func(stage string, value any) any {
		if f.controllerResponse != nil {
			value = f.controllerResponse(stage, value)
		}
		if stage == "set" && f.deletedSet.Load() {
			if _, ok := value.(baselineReply); ok {
				return value
			}
			f.setAbsences.Add(1)
			return baselineReply{status: 404}
		}
		return value
	}}
	config.workerPhases = phases
	config.controllerPhases = controllerPhases
	f.pairedIntegrationFixture = newPairedIntegrationFixtureConfigured(t, config)
	f.githubBefore = func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == "DELETE" && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7/sessions/00000000-0000-4000-8000-000000000001"):
			f.sessionDeletes.Add(1)
			w.WriteHeader(204)
			return true
		case r.Method == "DELETE" && strings.HasSuffix(r.URL.Path, "/runnerscalesets/7"):
			f.setDeletes.Add(1)
			f.deletedSet.Store(true)
			w.WriteHeader(204)
			return true
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/actions/runners"):
			f.rosters.Add(1)
		case r.Method == "GET" && f.c.polls.Load() >= 2 && strings.HasSuffix(r.URL.Path, "/agents/81"):
			f.sdkReads.Add(1)
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"typeName":"AgentNotFoundException","message":"synthetic missing runner"}`))
			return true
		case r.Method == "GET" && f.c.polls.Load() >= 2 && strings.HasSuffix(r.URL.Path, "/actions/runners/9001"):
			f.restReads.Add(1)
			w.WriteHeader(404)
			return true
		}
		return false
	}
	f.dockerBefore = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == "DELETE" && r.URL.Path == "/v1.45/containers/"+strings.Repeat("c", 64) {
			if r.URL.Query().Get("force") != "false" || r.URL.Query().Get("v") != "false" {
				t.Error("force or volume removal requested")
				w.WriteHeader(403)
				return true
			}
			f.workerDeletes.Add(1)
			f.deletedWorker.Store(true)
			w.WriteHeader(204)
			return true
		}
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/json") {
			if f.deletedWorker.Load() {
				f.workerAbsences.Add(1)
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"message":"synthetic no such container"}`))
				return true
			}
			if f.c.polls.Load() >= 2 {
				f.container["State"] = map[string]any{"Status": "exited", "Running": false, "Paused": false, "Restarting": false, "Dead": false, "ExitCode": 0}
			}
		}
		return false
	}
	return f
}
func (f *terminalFixture) run() (pairedBaselineTerminalResult, error) {
	return runPairedTerminalWithCadence(context.Background(), &Driver{Approval: f.c.a, Journal: f.c.j, API: f.c.api}, f.w, fastPairCadence())
}

type exportedPairedFixture struct {
	files       PairedTerminalFiles
	credentials Credentials
}

func pairedFixtureIdentity(t *testing.T, path string) (uint64, uint64) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("fixture identity unavailable")
	}
	return uint64(stat.Dev), stat.Ino
}

func pairedFixtureDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func newExportedPairedFixture(t *testing.T, f *terminalFixture) exportedPairedFixture {
	t.Helper()
	controllerState := f.c.j.directory
	controllerAdmission := f.c.j.claim.directory
	workerState := f.wf.StateDirectory()
	workerAdmission := f.wf.AdmissionDirectory()
	if f.c.j.Close() != nil || f.wf.Journal.Close() != nil {
		t.Fatal("close initial fixture journals")
	}
	root := filepath.Dir(controllerState)
	controllerPath := filepath.Join(root, "controller-approval.json")
	workerPath := filepath.Join(root, "worker-approval.json")
	controllerData, err := json.Marshal(f.c.a)
	if err != nil || os.WriteFile(controllerPath, controllerData, 0600) != nil {
		t.Fatal("controller approval fixture")
	}
	workerData, err := json.Marshal(f.w.Approval)
	if err != nil || os.WriteFile(workerPath, workerData, 0600) != nil {
		t.Fatal("worker approval fixture")
	}
	controllerDevice, controllerInode := pairedFixtureIdentity(t, controllerPath)
	controllerStateDevice, controllerStateInode := pairedFixtureIdentity(t, controllerState)
	workerDevice, workerInode := pairedFixtureIdentity(t, workerPath)
	workerStateDevice, workerStateInode := pairedFixtureIdentity(t, workerState)
	binding := &PairedTerminalBinding{ControllerApprovalSHA256: pairedFixtureDigest(t, controllerPath), ControllerApprovalDevice: controllerDevice, ControllerApprovalInode: controllerInode, ControllerStateDevice: controllerStateDevice, ControllerStateInode: controllerStateInode, WorkerApprovalSHA256: pairedFixtureDigest(t, workerPath), WorkerApprovalDevice: workerDevice, WorkerApprovalInode: workerInode, WorkerStateDevice: workerStateDevice, WorkerStateInode: workerStateInode}
	files := PairedTerminalFiles{ControllerApprovalPath: controllerPath, ControllerStateDirectory: controllerState, WorkerApprovalPath: workerPath, WorkerStateDirectory: workerState}
	oldAdapters, oldCadence := pairedTerminalFixtureAdapters, pairedTerminalFixtureCadence
	pairedTerminalFixtureCadence = fastPairCadence
	pairedTerminalFixtureAdapters = &pairedTerminalAdapters{
		openController: func(path string, a Approval) (*FileJournal, error) {
			j, err := openJournalAtAdmission(path, a, controllerAdmission, func(file *os.File) error { return file.Sync() })
			if err == nil {
				f.c.j = j
			}
			return j, err
		},
		openWorker: func(path string, a liveworker.Approval) (*liveworker.FileJournal, error) {
			j, err := liveworker.OpenJournalForPairedFixture(path, a, workerAdmission)
			if err == nil {
				f.wf.Journal = j
			}
			return j, err
		},
		newAPI: func(a Approval, c Credentials) (*SDKAPI, error) {
			f.c.api.approval = a
			f.c.api.credentials = c
			return f.c.api, nil
		},
		newDocker: liveworker.NewDocker,
	}
	t.Cleanup(func() {
		pairedTerminalFixtureAdapters, pairedTerminalFixtureCadence = oldAdapters, oldCadence
	})
	return exportedPairedFixture{files: files, credentials: Credentials{InstallationToken: f.c.api.credentials.InstallationToken, VerificationToken: f.c.api.credentials.VerificationToken, AppID: f.c.a.AppID, InstallationID: f.c.a.InstallationID, Organization: f.c.a.Organization, ExpiresAt: f.c.api.credentials.ExpiresAt, SelfHostedRunners: "write", Metadata: "read", PairedBinding: binding}}
}

func TestPairedTerminalExportedAdapterActualJournalsFinalize(t *testing.T) {
	f := newTerminalFixture(t, true)
	fixture := newExportedPairedFixture(t, f)
	if err := RunPairedTerminal(context.Background(), fixture.files, fixture.credentials); err != nil {
		t.Fatalf("exported paired adapter refused private TLS/Unix fixture: %v", err)
	}
	if f.c.acks.Load() != 2 || f.c.acquires.Load() != 1 || f.jit.Load() != 1 || f.creates.Load() != 1 || f.starts.Load() != 1 || f.sessionDeletes.Load() != 1 || f.workerDeletes.Load() != 1 || f.setDeletes.Load() != 1 || f.workerAbsences.Load() != 1 || f.setAbsences.Load() != 1 || f.rosters.Load() != 4 || f.cleanup.Load() != 0 || f.c.forbidden.Load() != 0 {
		t.Fatalf("exported terminal sequence: ack=%d acquire=%d JIT=%d create=%d start=%d session=%d worker=%d set=%d worker404=%d set404=%d roster=%d forbidden=%d", f.c.acks.Load(), f.c.acquires.Load(), f.jit.Load(), f.creates.Load(), f.starts.Load(), f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load(), f.workerAbsences.Load(), f.setAbsences.Load(), f.rosters.Load(), f.c.forbidden.Load())
	}
	raw, _ := json.Marshal(f.c.j.Events())
	workerRaw, _ := json.Marshal(f.wf.Journal.Events())
	for _, secret := range []string{fixture.credentials.InstallationToken, fixture.credentials.VerificationToken, pairFixtureJIT} {
		if strings.Contains(string(raw), secret) || strings.Contains(string(workerRaw), secret) {
			t.Fatal("exported fixture journal leaked credential or JIT")
		}
	}
}

func TestPairedTerminalExportedAdapterCancellationAndReopenNoReplay(t *testing.T) {
	f := newTerminalFixture(t, true)
	fixture := newExportedPairedFixture(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.c.afterResponse = func(r *http.Request, response *http.Response) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/sessions/") && response.StatusCode == http.StatusNoContent {
			cancel()
		}
	}
	if err := RunPairedTerminal(ctx, fixture.files, fixture.credentials); err == nil || f.sessionDeletes.Load() != 1 || f.workerDeletes.Load() != 0 || f.setDeletes.Load() != 0 {
		t.Fatalf("canceled exported terminal crossed cleanup boundary: err=%v session=%d worker=%d set=%d", err, f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load())
	}
	before := [3]int32{f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load()}
	reopened := newExportedPairedFixture(t, f)
	if err := RunPairedTerminal(context.Background(), reopened.files, reopened.credentials); err == nil {
		t.Fatal("reopened canceled exported terminal was replayed")
	}
	after := [3]int32{f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load()}
	if before != after {
		t.Fatalf("reopened canceled terminal issued another effect: before=%v after=%v", before, after)
	}
}

func TestPairedTerminalExportedAdapterLostResponseStopsWithoutReplay(t *testing.T) {
	f := newTerminalFixture(t, true)
	original := f.githubBefore
	f.githubBefore = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/sessions/") {
			terminalLose(w)
			return true
		}
		return original(w, r)
	}
	fixture := newExportedPairedFixture(t, f)
	if err := RunPairedTerminal(context.Background(), fixture.files, fixture.credentials); err == nil || f.sessionDeletes.Load() != 0 || f.workerDeletes.Load() != 0 || f.setDeletes.Load() != 0 {
		t.Fatalf("lost exported session response crossed effect boundary: err=%v session=%d worker=%d set=%d", err, f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load())
	}
	before := [3]int32{f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load()}
	reopened := newExportedPairedFixture(t, f)
	if err := RunPairedTerminal(context.Background(), reopened.files, reopened.credentials); err == nil {
		t.Fatal("reopened lost-response terminal was replayed")
	}
	after := [3]int32{f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load()}
	if before != after {
		t.Fatalf("reopened lost-response terminal issued another effect: before=%v after=%v", before, after)
	}
}

func TestPairedTerminalBindingRechecksAfterJournalOpenBeforeEffects(t *testing.T) {
	f := newTerminalFixture(t, true)
	fixture := newExportedPairedFixture(t, f)
	adapters := *pairedTerminalFixtureAdapters
	originalOpen := adapters.openController
	adapters.openController = func(path string, a Approval) (*FileJournal, error) {
		j, err := originalOpen(path, a)
		if err == nil {
			data, readErr := os.ReadFile(fixture.files.WorkerApprovalPath)
			if readErr != nil || os.WriteFile(fixture.files.WorkerApprovalPath, append(data, '\n'), 0600) != nil {
				t.Fatal("worker approval replacement")
			}
		}
		return j, err
	}
	pairedTerminalFixtureAdapters = &adapters
	if err := RunPairedTerminal(context.Background(), fixture.files, fixture.credentials); err == nil || f.c.requests.Load() != 0 || f.c.acquires.Load() != 0 || f.jit.Load() != 0 || f.creates.Load() != 0 || f.starts.Load() != 0 || f.sessionDeletes.Load() != 0 || f.workerDeletes.Load() != 0 || f.setDeletes.Load() != 0 {
		t.Fatalf("approval replacement after initial binding crossed pre-effect fence: err=%v requests=%d acquire=%d JIT=%d create=%d start=%d session=%d worker=%d set=%d", err, f.c.requests.Load(), f.c.acquires.Load(), f.jit.Load(), f.creates.Load(), f.starts.Load(), f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load())
	}
}

func TestPairedTerminalBindingRejectsChangedBytesAndStateBeforeEffects(t *testing.T) {
	for _, kind := range []string{"controller-bytes", "controller-inode", "controller-hash", "controller-state", "controller-state-symlink", "worker-bytes", "worker-inode", "worker-hash", "worker-state", "worker-state-symlink"} {
		t.Run(kind, func(t *testing.T) {
			f := newTerminalFixture(t, true)
			fixture := newExportedPairedFixture(t, f)
			mutateFile := func(path string, data []byte) {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "controller-bytes":
				data, _ := os.ReadFile(fixture.files.ControllerApprovalPath)
				mutateFile(fixture.files.ControllerApprovalPath, append(data, '\n'))
			case "controller-inode":
				data, _ := os.ReadFile(fixture.files.ControllerApprovalPath)
				if err := os.Rename(fixture.files.ControllerApprovalPath, fixture.files.ControllerApprovalPath+".retained"); err != nil {
					t.Fatal(err)
				}
				mutateFile(fixture.files.ControllerApprovalPath, data)
			case "controller-hash":
				binding := *fixture.credentials.PairedBinding
				binding.ControllerApprovalSHA256 = strings.Repeat("0", 64)
				fixture.credentials.PairedBinding = &binding
			case "controller-state":
				if err := os.Rename(fixture.files.ControllerStateDirectory, fixture.files.ControllerStateDirectory+".retained"); err != nil || os.Mkdir(fixture.files.ControllerStateDirectory, 0700) != nil {
					t.Fatal("controller state replacement")
				}
			case "controller-state-symlink":
				if err := os.Rename(fixture.files.ControllerStateDirectory, fixture.files.ControllerStateDirectory+".retained"); err != nil || os.Symlink(fixture.files.ControllerStateDirectory+".retained", fixture.files.ControllerStateDirectory) != nil {
					t.Fatal("controller state symlink")
				}
			case "worker-bytes":
				data, _ := os.ReadFile(fixture.files.WorkerApprovalPath)
				mutateFile(fixture.files.WorkerApprovalPath, append(data, '\n'))
			case "worker-inode":
				data, _ := os.ReadFile(fixture.files.WorkerApprovalPath)
				if err := os.Rename(fixture.files.WorkerApprovalPath, fixture.files.WorkerApprovalPath+".retained"); err != nil {
					t.Fatal(err)
				}
				mutateFile(fixture.files.WorkerApprovalPath, data)
			case "worker-hash":
				binding := *fixture.credentials.PairedBinding
				binding.WorkerApprovalSHA256 = strings.Repeat("0", 64)
				fixture.credentials.PairedBinding = &binding
			case "worker-state":
				if err := os.Rename(fixture.files.WorkerStateDirectory, fixture.files.WorkerStateDirectory+".retained"); err != nil || os.Mkdir(fixture.files.WorkerStateDirectory, 0700) != nil {
					t.Fatal("worker state replacement")
				}
			case "worker-state-symlink":
				if err := os.Rename(fixture.files.WorkerStateDirectory, fixture.files.WorkerStateDirectory+".retained"); err != nil || os.Symlink(fixture.files.WorkerStateDirectory+".retained", fixture.files.WorkerStateDirectory) != nil {
					t.Fatal("worker state symlink")
				}
			}
			if err := RunPairedTerminal(context.Background(), fixture.files, fixture.credentials); err == nil || f.c.requests.Load() != 0 || f.c.acquires.Load() != 0 || f.jit.Load() != 0 || f.creates.Load() != 0 || f.starts.Load() != 0 || f.sessionDeletes.Load() != 0 || f.workerDeletes.Load() != 0 || f.setDeletes.Load() != 0 {
				t.Fatalf("changed paired identity crossed pre-effect gate: err=%v requests=%d acquire=%d jit=%d create=%d start=%d session=%d worker=%d set=%d", err, f.c.requests.Load(), f.c.acquires.Load(), f.jit.Load(), f.creates.Load(), f.starts.Load(), f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load())
			}
		})
	}
}

func TestPairedTerminalActualJournalsFinalize(t *testing.T) {
	f := newTerminalFixture(t, true)
	if len(f.c.j.Events()) != 3 || f.wf.Journal == nil {
		t.Fatal("actual journal fixture not prepared")
	}
	out, err := f.run()
	if err != nil || out.Terminal != terminalComplete || out.Collection.Outcome != collectionCollected || out.Collection.Rounds != 8 || out.Collection.Result.Sequence == 0 {
		t.Fatalf("terminal feature unavailable: outcome=%s measurement=%s rounds=%d error=%v", out.Terminal, out.Collection.Outcome, out.Collection.Rounds, err)
	}
	if f.c.acquires.Load() != 1 || f.jit.Load() != 1 || f.creates.Load() != 1 || f.starts.Load() != 1 || f.sessionDeletes.Load() != 1 || f.workerDeletes.Load() != 1 || f.setDeletes.Load() != 1 || f.workerAbsences.Load() != 1 || f.setAbsences.Load() != 1 || f.rosters.Load() != 4 || f.cleanup.Load() != 0 || f.c.forbidden.Load() != 0 {
		t.Fatalf("terminal sequence: acquire=%d JIT=%d create=%d start=%d session=%d worker=%d set=%d worker404=%d set404=%d roster=%d forbidden=%d", f.c.acquires.Load(), f.jit.Load(), f.creates.Load(), f.starts.Load(), f.sessionDeletes.Load(), f.workerDeletes.Load(), f.setDeletes.Load(), f.workerAbsences.Load(), f.setAbsences.Load(), f.rosters.Load(), f.c.forbidden.Load())
	}
}
func TestPairedTerminalMissingCleanupRefusesBeforePrefix(t *testing.T) {
	f := newTerminalFixture(t, false)
	_, err := f.run()
	if err == nil || f.c.requests.Load() != 0 || f.dockerReads.Load() != 0 || len(f.c.j.Events()) != 3 {
		t.Fatal("missing cleanup authority reached the prefix")
	}
}
