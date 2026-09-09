package livecanary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

// PairedTerminalFiles identify the two already-prepared private state domains.
// The paired terminal mode has one process and one controller input stream; the
// worker is never launched as a child process.
type PairedTerminalFiles struct {
	ControllerApprovalPath   string
	ControllerStateDirectory string
	WorkerApprovalPath       string
	WorkerStateDirectory     string
}

// PairedTerminalBinding is broker-captured identity evidence. It is not a
// pairing manifest or an authority source: controller approval plus the
// journal-derived PairInput remain authoritative for the terminal sequence.
// Every value is bounded and credential-free so it can cross the child argv
// boundary alongside the controller-only credential payload.
type PairedTerminalBinding struct {
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

func (b PairedTerminalBinding) equal(other PairedTerminalBinding) bool { return b == other }

func (b PairedTerminalBinding) valid() bool {
	return len(b.ControllerApprovalSHA256) == 64 && isLowerHex(b.ControllerApprovalSHA256) && b.ControllerApprovalDevice != 0 && b.ControllerApprovalInode != 0 && b.ControllerStateDevice != 0 && b.ControllerStateInode != 0 && len(b.WorkerApprovalSHA256) == 64 && isLowerHex(b.WorkerApprovalSHA256) && b.WorkerApprovalDevice != 0 && b.WorkerApprovalInode != 0 && b.WorkerStateDevice != 0 && b.WorkerStateInode != 0
}

func isLowerHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

type pairedApprovalSnapshot struct {
	raw  []byte
	info os.FileInfo
}

func readPairedApproval(path string, target any, decode func([]byte, any) error) (pairedApprovalSnapshot, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return pairedApprovalSnapshot{}, ErrApproval
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return pairedApprovalSnapshot{}, ErrApproval
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !privatePairedApproval(info) || info.Size() > 16384 {
		return pairedApprovalSnapshot{}, ErrApproval
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(data) > 16384 || decode(data, target) != nil {
		return pairedApprovalSnapshot{}, ErrApproval
	}
	return pairedApprovalSnapshot{raw: data, info: info}, nil
}

func privatePairedApproval(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && info.Mode().Perm() == 0600 && info.Mode().IsRegular() && stat.Nlink == 1
}

func pairedIdentity(info os.FileInfo) (device, inode uint64, ok bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(stat.Dev), stat.Ino, true
}

func pairedStateIdentity(path string) (device, inode uint64, err error) {
	if !privateStateDirectory(path) {
		return 0, 0, ErrJournal
	}
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0, ErrJournal
	}
	device, inode, ok := pairedIdentity(info)
	if !ok || device == 0 || inode == 0 {
		return 0, 0, ErrJournal
	}
	return device, inode, nil
}

func readPairedSnapshots(files PairedTerminalFiles) (Approval, liveworker.Approval, pairedApprovalSnapshot, pairedApprovalSnapshot, error) {
	var controller Approval
	controllerSnapshot, err := readPairedApproval(files.ControllerApprovalPath, &controller, DecodeStrict)
	if err != nil {
		return Approval{}, liveworker.Approval{}, pairedApprovalSnapshot{}, pairedApprovalSnapshot{}, ErrApproval
	}
	var worker liveworker.Approval
	workerSnapshot, err := readPairedApproval(files.WorkerApprovalPath, &worker, liveworker.DecodeStrict)
	if err != nil {
		return Approval{}, liveworker.Approval{}, pairedApprovalSnapshot{}, pairedApprovalSnapshot{}, ErrApproval
	}
	return controller, worker, controllerSnapshot, workerSnapshot, nil
}

func validatePairedBinding(files PairedTerminalFiles, binding PairedTerminalBinding, controllerSnapshot, workerSnapshot pairedApprovalSnapshot) error {
	if !binding.valid() {
		return ErrApproval
	}
	controllerDevice, controllerInode, ok := pairedIdentity(controllerSnapshot.info)
	if !ok || controllerDevice != binding.ControllerApprovalDevice || controllerInode != binding.ControllerApprovalInode || brokerDigestBytes(controllerSnapshot.raw) != binding.ControllerApprovalSHA256 {
		return ErrApproval
	}
	workerDevice, workerInode, ok := pairedIdentity(workerSnapshot.info)
	if !ok || workerDevice != binding.WorkerApprovalDevice || workerInode != binding.WorkerApprovalInode || brokerDigestBytes(workerSnapshot.raw) != binding.WorkerApprovalSHA256 {
		return ErrApproval
	}
	controllerStateDevice, controllerStateInode, err := pairedStateIdentity(files.ControllerStateDirectory)
	if err != nil || controllerStateDevice != binding.ControllerStateDevice || controllerStateInode != binding.ControllerStateInode {
		return ErrJournal
	}
	workerStateDevice, workerStateInode, err := pairedStateIdentity(files.WorkerStateDirectory)
	if err != nil || workerStateDevice != binding.WorkerStateDevice || workerStateInode != binding.WorkerStateInode {
		return ErrJournal
	}
	return nil
}

func brokerDigestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// ValidatePairedTerminalBinding is the credential-free pre-input fence used
// by g01-live. It compares broker evidence with the exact approval bytes and
// state roots currently named by the fixed argv.
func ValidatePairedTerminalBinding(files PairedTerminalFiles, binding PairedTerminalBinding) error {
	controller, worker, controllerSnapshot, workerSnapshot, err := readPairedSnapshots(files)
	if err != nil {
		return err
	}
	if controller.Validate(time.Now()) != nil || worker.Validate(time.Now()) != nil || ValidatePairedApprovals(controller, worker) != nil || ValidatePairedStatePaths(files.ControllerStateDirectory, files.WorkerStateDirectory) != nil {
		return ErrApproval
	}
	return validatePairedBinding(files, binding, controllerSnapshot, workerSnapshot)
}

// ValidatePairedApprovals is the credential-free front door shared by the CLI
// and the same-process runner. PairInput is still derived from the controller
// journal and approval; worker input only proves the intended trusted worker
// profile and shared immutable identity.
func ValidatePairedApprovals(controller Approval, worker liveworker.Approval) error {
	now := time.Now()
	if controller.Validate(now) != nil || worker.Validate(now) != nil {
		return ErrApproval
	}
	if controller.OwnerNonce != worker.OwnerNonce || controller.HarnessSHA != worker.HarnessSHA || controller.WorkflowSHA != worker.WorkflowSHA || controller.Controller != worker.Controller || !controller.ExpiresAt.Equal(worker.ExpiresAt) {
		return ErrApproval
	}
	if controller.WorkflowRunID <= 0 {
		return ErrApproval
	}
	verificationPhase := false
	for _, phase := range controller.Phases {
		if phase == "before-ack" || phase == "after-ack" || phase == "before-acquire" || phase == "acquire-loss" {
			verificationPhase = true
			break
		}
	}
	if !verificationPhase {
		return ErrApproval
	}
	for _, phase := range []string{"create", "inspect", "cleanup"} {
		found := false
		for _, candidate := range controller.Phases {
			if candidate == phase {
				found = true
				break
			}
		}
		if !found {
			return ErrApproval
		}
	}
	for _, phase := range []string{"create", "start", "inspect", "cleanup"} {
		found := false
		for _, candidate := range worker.Phases {
			if candidate == phase {
				found = true
				break
			}
		}
		if !found {
			return ErrApproval
		}
	}
	return nil
}

// ValidatePairedStatePaths checks the private roots before the controller
// credential stream is read. OpenJournal repeats the inode/claim checks while
// holding each domain lease; this early check only prevents an invalid or
// aliased root from reaching credential handling.
func ValidatePairedStatePaths(controller, worker string) error {
	if !privateStateDirectory(controller) || !privateStateDirectory(worker) {
		return ErrJournal
	}
	controllerReal, err := filepath.EvalSymlinks(controller)
	if err != nil {
		return ErrJournal
	}
	workerReal, err := filepath.EvalSymlinks(worker)
	if err != nil || workerReal == controllerReal {
		return ErrJournal
	}
	return nil
}

func privateStateDirectory(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}

type pairedTerminalAdapters struct {
	openController func(string, Approval) (*FileJournal, error)
	openWorker     func(string, liveworker.Approval) (*liveworker.FileJournal, error)
	newAPI         func(Approval, Credentials) (*SDKAPI, error)
	newDocker      func(liveworker.Approval) (*liveworker.Docker, error)
}

// This hook is nil in production. The g01_pair_fixture-only test support uses
// it solely to point real journal/SDK construction at generated private roots
// and a private TLS server; no caller-controlled runtime authority is exposed.
var pairedTerminalFixtureAdapters *pairedTerminalAdapters

// Pair fixture tests may replace only the cadence; production keeps the real
// terminal timing and all API/journal/lease behavior.
var pairedTerminalFixtureCadence func() pairedBaselineCadence

// RunPairedTerminal opens both existing journals, constructs the pinned SDK and
// direct Unix-socket runtime, and runs the reviewed terminal sequence in this
// process. It returns only fixed error categories; no private SDK/Docker error
// or path is part of the result boundary.
func RunPairedTerminal(ctx context.Context, files PairedTerminalFiles, credentials Credentials) error {
	if ctx == nil {
		return ErrApproval
	}
	if credentials.PairedBinding == nil {
		return ErrApproval
	}
	controller, worker, controllerSnapshot, workerSnapshot, err := readPairedSnapshots(files)
	if err != nil || ValidatePairedApprovals(controller, worker) != nil || ValidatePairedStatePaths(files.ControllerStateDirectory, files.WorkerStateDirectory) != nil || validatePairedBinding(files, *credentials.PairedBinding, controllerSnapshot, workerSnapshot) != nil {
		return ErrApproval
	}
	if credentials.validate(controller, time.Now()) != nil || len(credentials.VerificationToken) < 20 || len(credentials.VerificationToken) > 1024 || credentials.VerificationToken == credentials.InstallationToken || strings.ContainsAny(credentials.VerificationToken, "\r\n\x00") {
		return ErrApproval
	}
	adapters := pairedTerminalAdapters{openController: OpenJournal, openWorker: liveworker.OpenJournal, newAPI: NewSDKAPI, newDocker: liveworker.NewDocker}
	if pairedTerminalFixtureAdapters != nil {
		adapters = *pairedTerminalFixtureAdapters
	}
	controllerJournal, err := adapters.openController(files.ControllerStateDirectory, controller)
	if err != nil {
		return ErrJournal
	}
	defer controllerJournal.Close()
	workerJournal, err := adapters.openWorker(files.WorkerStateDirectory, worker)
	if err != nil {
		return ErrJournal
	}
	// LIFO closes the worker claim before the controller claim, matching the
	// paired cleanup order even when the terminal exits through an error path.
	defer workerJournal.Close()
	if err := ValidatePairedTerminalBinding(files, *credentials.PairedBinding); err != nil {
		return err
	}
	api, err := adapters.newAPI(controller, credentials)
	if err != nil {
		return ErrApproval
	}
	docker, err := adapters.newDocker(worker)
	if err != nil {
		return ErrJournal
	}
	bindingCheck := func() error {
		_, _, controllerSnapshot, workerSnapshot, e := readPairedSnapshots(files)
		if e != nil {
			return ErrApproval
		}
		return validatePairedBinding(files, *credentials.PairedBinding, controllerSnapshot, workerSnapshot)
	}
	cadence := realBaselineCadence()
	if pairedTerminalFixtureCadence != nil {
		cadence = pairedTerminalFixtureCadence()
	}
	result, err := runPairedTerminalWithBinding(ctx, &Driver{Approval: controller, Journal: controllerJournal, API: api}, &liveworker.Driver{Approval: worker, Journal: workerJournal, Runtime: docker}, cadence, bindingCheck)
	if err != nil || result.Terminal != terminalComplete {
		return ErrQuarantine
	}
	if bindingCheck() != nil {
		return ErrQuarantine
	}
	return nil
}
