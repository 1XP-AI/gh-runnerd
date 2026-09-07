package enrollment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"syscall"
	"time"
)

// BrokerFiles are private paths only. Credentials are accepted exclusively from
// the supplied owned stdin descriptor, never flags, environment or these files.
type controllerApproval struct {
	AppID          int64     `json:"app_id"`
	InstallationID int64     `json:"installation_id"`
	Organization   string    `json:"organization"`
	Repository     string    `json:"repository"`
	RepositoryID   int64     `json:"repository_id"`
	RunnerGroupID  int64     `json:"runner_group_id"`
	OwnerNonce     string    `json:"owner_nonce"`
	HarnessSHA     string    `json:"harness_sha"`
	WorkflowSHA    string    `json:"workflow_sha"`
	WorkflowPath   string    `json:"workflow_path"`
	WorkflowRunID  int64     `json:"workflow_run_id"`
	Controller     string    `json:"controller"`
	ExpiresAt      time.Time `json:"expires_at"`
	ActionsHosts   []string  `json:"actions_hosts"`
	Phases         []string  `json:"phases"`
}

var brokerNonce = regexp.MustCompile(`^[a-f0-9]{32}$`)
var brokerWorkflow = regexp.MustCompile(`^\.github/workflows/[a-zA-Z0-9_-]+\.ya?ml$`)

func (c controllerApproval) needsVerification() bool {
	return slices.ContainsFunc(c.Phases, func(phase string) bool {
		return phase == "before-ack" || phase == "after-ack" || phase == "before-acquire" || phase == "acquire-loss"
	})
}
func (c controllerApproval) validate(a BrokerApproval, now time.Time) error {
	if c.AppID != a.AppID || c.InstallationID != a.InstallationID || c.Organization != a.Organization || c.Repository != a.Repository || c.RepositoryID != a.RepositoryID || c.RunnerGroupID != a.RunnerGroupID || c.HarnessSHA != a.ControllerHarnessSHA || !brokerSHA40.MatchString(c.HarnessSHA) || !brokerSHA40.MatchString(c.WorkflowSHA) || !brokerNonce.MatchString(c.OwnerNonce) || !brokerWorkflow.MatchString(c.WorkflowPath) || !brokerComponent.MatchString(c.Controller) || !c.ExpiresAt.After(now.Add(time.Minute)) || c.ExpiresAt.After(now.Add(24*time.Hour)) || c.ExpiresAt.Before(a.ExpiresAt) || len(c.ActionsHosts) < 1 || len(c.ActionsHosts) > 8 || len(c.Phases) < 1 || len(c.Phases) > 8 || !slices.Contains(c.Phases, a.Phase) || c.needsVerification() != a.AllowVerificationAuthority || (c.needsVerification() && c.WorkflowRunID < 1) {
		return errBroker
	}
	seen := map[string]bool{}
	for _, host := range c.ActionsHosts {
		validated, err := brokerActionsHost("https://" + host)
		if err != nil || validated != host || seen[host] {
			return errBroker
		}
		seen[host] = true
	}
	seen = map[string]bool{}
	for _, phase := range c.Phases {
		if !brokerPhases[phase] || seen[phase] {
			return errBroker
		}
		seen[phase] = true
	}
	return nil
}
func readBrokerPrivateJSON(path string, target any) ([]byte, error) {
	file, err := openBrokerPrivateFile(path, 0600, 16384)
	if err != nil {
		return nil, errBroker
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(data) > 16384 || decodeBrokerJSON(data, target, true) != nil {
		return nil, errBroker
	}
	return data, nil
}
func readBrokerInput(parent context.Context, input *os.File) (brokerInput, error) {
	if input == nil {
		return brokerInput{}, errBroker
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return brokerInput{}, errBroker
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Getuid()) || info.Mode().Perm()&0077 != 0 || !((info.Mode().IsRegular() && s.Nlink == 1) || info.Mode()&os.ModeNamedPipe != 0) {
		return brokerInput{}, errBroker
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	data, err := readPrivateInput(ctx, input, 65536)
	defer clear(data)
	var result brokerInput
	if err != nil || len(data) > 65536 || ctx.Err() != nil || decodeBrokerJSON(data, &result, true) != nil || len(result.PEM) > 32768 || len(result.VerificationToken) > 1024 {
		return brokerInput{}, errBroker
	}
	return result, nil
}

// RunBroker is network-lazy until all private approval/input checks succeed.
// Calling this function with live credentials requires separate exact approval.
func RunBroker(ctx context.Context, files BrokerFiles, input *os.File) (BrokerResult, error) {
	return runBrokerWithAPI(ctx, files, input, newBrokerAPI(time.Now, nil))
}
func runBrokerWithAPI(ctx context.Context, files BrokerFiles, input *os.File, api *brokerAPI) (BrokerResult, error) {
	if ctx == nil || api == nil || !filepath.IsAbs(files.StateDirectory) {
		return BrokerResult{}, errBroker
	}
	var approval BrokerApproval
	if _, err := readBrokerPrivateJSON(files.ApprovalPath, &approval); err != nil || approval.validate(api.now()) != nil {
		return BrokerResult{}, errBroker
	}
	var binary *verifiedBrokerBinary
	var controller controllerApproval
	var controllerData []byte
	var controllerRoot *os.Root
	if approval.Mode == "controller" {
		if !brokerSHA256.MatchString(approval.ControllerBinarySHA256) || !brokerSHA256.MatchString(approval.ControllerApprovalSHA256) || !brokerSHA40.MatchString(approval.ControllerHarnessSHA) {
			return BrokerResult{}, errBroker
		}
		var err error
		controllerData, err = readBrokerPrivateJSON(files.ControllerApproval, &controller)
		digest := sha256.Sum256(controllerData)
		if err != nil || hex.EncodeToString(digest[:]) != approval.ControllerApprovalSHA256 || controller.validate(approval, api.now()) != nil {
			return BrokerResult{}, errBroker
		}
		binary, err = openBrokerBinary(files.ControllerBinary, approval)
		if err != nil {
			return BrokerResult{}, errBroker
		}
		defer binary.file.Close()
		controllerRoot, err = openBrokerPrivateDirectory(files.ControllerStateDirectory)
		if err != nil {
			return BrokerResult{}, errBroker
		}
		defer controllerRoot.Close()
	} else if files.ControllerBinary != "" || files.ControllerApproval != "" || files.ControllerStateDirectory != "" || approval.ControllerBinarySHA256 != "" || approval.ControllerApprovalSHA256 != "" || approval.ControllerHarnessSHA != "" {
		return BrokerResult{}, errBroker
	}
	credentialInput, err := readBrokerInput(ctx, input)
	if err != nil || (approval.AllowVerificationAuthority && credentialInput.VerificationToken == "") {
		return BrokerResult{}, errBroker
	}
	var launch func(context.Context, []byte) error
	if approval.Mode == "controller" {
		launch = func(ctx context.Context, data []byte) error {
			if controller.needsVerification() && api.verifyWorkflow(ctx, approval, controller, credentialInput.VerificationToken) != nil {
				return errBroker
			}
			actual, e := controllerRoot.Stat(".")
			named, err := os.Lstat(files.ControllerStateDirectory)
			if e != nil || err != nil || !os.SameFile(actual, named) {
				return errBroker
			}
			root, err := openBrokerPrivateDirectory(files.StateDirectory)
			if err != nil {
				return errBroker
			}
			defer root.Close()
			// Only the non-secret, digest-verified approval snapshot is written. The
			// child never reopens the operator's original mutable approval path.
			snapshot, err := root.OpenFile("controller-approval.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
			if err != nil {
				return errBroker
			}
			_, writeErr := snapshot.Write(controllerData)
			syncErr := snapshot.Sync()
			closeErr := snapshot.Close()
			if writeErr != nil || syncErr != nil || closeErr != nil || syncDirectory(root) != nil {
				return errBroker
			}
			return invokeBrokerController(ctx, binary, files.StateDirectory, filepath.Join(files.StateDirectory, "controller-approval.json"), files.ControllerStateDirectory, approval.Phase, data)
		}
	}
	return brokerExecute(ctx, approval, credentialInput, files.StateDirectory, api, launch)
}
func (a *brokerAPI) verifyWorkflow(ctx context.Context, approval BrokerApproval, controller controllerApproval, token string) error {
	var run struct {
		ID             int64            `json:"id"`
		HeadSHA        string           `json:"head_sha"`
		Event          string           `json:"event"`
		Path           string           `json:"path"`
		RunAttempt     int              `json:"run_attempt"`
		Repository     brokerRepository `json:"repository"`
		HeadRepository brokerRepository `json:"head_repository"`
	}
	if a.call(ctx, "GET", "/repos/"+approval.Organization+"/"+approval.Repository+"/actions/runs/"+strconv.FormatInt(controller.WorkflowRunID, 10), "Bearer "+token, nil, 200, &run) != nil || run.ID != controller.WorkflowRunID || run.HeadSHA != controller.WorkflowSHA || run.Path != controller.WorkflowPath || run.Event != "workflow_dispatch" || run.RunAttempt != 1 || !run.Repository.matches(approval) || !run.HeadRepository.matches(approval) {
		return errBroker
	}
	return nil
}
