//go:build g01_live

// This command is absent from ordinary builds. Building it is not authorization
// to execute it; live calls additionally require a private exact-commit approval.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"syscall"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary"
	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

const (
	maxPublicEvidenceInputBytes = 1 << 20
	maxPublicEvidenceJSONDepth  = 16
)

type publicEvidenceInput struct {
	ElapsedMS         int64                         `json:"elapsed_ms"`
	Attempt           int                           `json:"attempt"`
	SessionGeneration int                           `json:"session_generation"`
	Phase             string                        `json:"phase"`
	Operation         string                        `json:"operation"`
	Outcome           string                        `json:"outcome"`
	Resources         []publicEvidenceResourceInput `json:"resources"`
	HTTPStatus        int                           `json:"http_status"`
	Succeeded         *bool                         `json:"succeeded"`
	Counts            *publicEvidenceCountsInput    `json:"counts"`
}

type publicEvidenceResourceInput struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type publicEvidenceCountsInput struct {
	Available  *int `json:"available"`
	Acquired   *int `json:"acquired"`
	Assigned   *int `json:"assigned"`
	Running    *int `json:"running"`
	Registered *int `json:"registered"`
	Busy       *int `json:"busy"`
	Idle       *int `json:"idle"`
}

func consumePublicEvidenceJSONValue(decoder *json.Decoder) error {
	return consumePublicEvidenceJSONValueAtDepth(decoder, 0)
}

func consumePublicEvidenceJSONValueAtDepth(decoder *json.Decoder, depth int) error {
	token, err := decoder.Token()
	if err != nil {
		return livecanary.ErrPublicEvidence
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		if depth >= maxPublicEvidenceJSONDepth {
			return livecanary.ErrPublicEvidence
		}
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok {
				return livecanary.ErrPublicEvidence
			}
			if _, exists := keys[key]; exists {
				return livecanary.ErrPublicEvidence
			}
			keys[key] = struct{}{}
			if consumePublicEvidenceJSONValueAtDepth(decoder, depth+1) != nil {
				return livecanary.ErrPublicEvidence
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return livecanary.ErrPublicEvidence
		}
	case '[':
		if depth >= maxPublicEvidenceJSONDepth {
			return livecanary.ErrPublicEvidence
		}
		for decoder.More() {
			if consumePublicEvidenceJSONValueAtDepth(decoder, depth+1) != nil {
				return livecanary.ErrPublicEvidence
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return livecanary.ErrPublicEvidence
		}
	default:
		return livecanary.ErrPublicEvidence
	}
	return nil
}

func publicEvidenceObjectFields(raw json.RawMessage, allowed, required []string) (map[string]json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(trimmed, &fields) != nil || fields == nil {
		return nil, false
	}
	allowedFields := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedFields[name] = struct{}{}
	}
	for name, value := range fields {
		if _, ok := allowedFields[name]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, false
		}
	}
	for _, name := range required {
		if _, ok := fields[name]; !ok {
			return nil, false
		}
	}
	return fields, true
}

func validatePublicEvidenceInputJSON(raw []byte) bool {
	duplicateCheck := json.NewDecoder(bytes.NewReader(raw))
	duplicateCheck.UseNumber()
	if consumePublicEvidenceJSONValue(duplicateCheck) != nil {
		return false
	}
	if _, err := duplicateCheck.Token(); err != io.EOF {
		return false
	}

	var events []json.RawMessage
	if json.Unmarshal(raw, &events) != nil || len(events) == 0 {
		return false
	}
	for _, event := range events {
		fields, ok := publicEvidenceObjectFields(event,
			[]string{"elapsed_ms", "attempt", "session_generation", "phase", "operation", "outcome", "resources", "http_status", "succeeded", "counts"},
			[]string{"elapsed_ms", "phase", "operation", "outcome"})
		if !ok {
			return false
		}
		if resources, exists := fields["resources"]; exists {
			trimmed := bytes.TrimSpace(resources)
			if len(trimmed) == 0 || trimmed[0] != '[' {
				return false
			}
			var items []json.RawMessage
			if json.Unmarshal(trimmed, &items) != nil {
				return false
			}
			for _, item := range items {
				if _, ok := publicEvidenceObjectFields(item, []string{"kind", "id"}, []string{"kind", "id"}); !ok {
					return false
				}
			}
		}
		if counts, exists := fields["counts"]; exists {
			if _, ok := publicEvidenceObjectFields(counts, []string{"available", "acquired", "assigned", "running", "registered", "busy", "idle"}, nil); !ok {
				return false
			}
		}
	}
	return true
}

func decodePublicEvidenceInput(input io.Reader) ([]livecanary.PublicEvidenceObservation, error) {
	if input == nil {
		return nil, livecanary.ErrPublicEvidence
	}
	raw, err := io.ReadAll(io.LimitReader(input, maxPublicEvidenceInputBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxPublicEvidenceInputBytes {
		return nil, livecanary.ErrPublicEvidence
	}
	if !validatePublicEvidenceInputJSON(raw) {
		return nil, livecanary.ErrPublicEvidence
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var encoded []publicEvidenceInput
	if decoder.Decode(&encoded) != nil {
		return nil, livecanary.ErrPublicEvidence
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || len(encoded) == 0 {
		return nil, livecanary.ErrPublicEvidence
	}

	observations := make([]livecanary.PublicEvidenceObservation, len(encoded))
	for index, event := range encoded {
		if event.ElapsedMS < 0 || event.ElapsedMS > int64((24*time.Hour)/time.Millisecond) {
			return nil, livecanary.ErrPublicEvidence
		}
		observation := livecanary.PublicEvidenceObservation{
			Elapsed:           time.Duration(event.ElapsedMS) * time.Millisecond,
			Attempt:           event.Attempt,
			SessionGeneration: event.SessionGeneration,
			Phase:             event.Phase,
			Operation:         event.Operation,
			Outcome:           event.Outcome,
			HTTPStatus:        event.HTTPStatus,
			Succeeded:         event.Succeeded,
		}
		if len(event.Resources) != 0 {
			observation.Resources = make([]livecanary.PublicEvidenceResource, len(event.Resources))
			for resourceIndex, resource := range event.Resources {
				observation.Resources[resourceIndex] = livecanary.PublicEvidenceResource{Kind: resource.Kind, ID: resource.ID}
			}
		}
		if event.Counts != nil {
			observation.Counts = &livecanary.PublicEvidenceCounts{
				Available:  event.Counts.Available,
				Acquired:   event.Counts.Acquired,
				Assigned:   event.Counts.Assigned,
				Running:    event.Counts.Running,
				Registered: event.Counts.Registered,
				Busy:       event.Counts.Busy,
				Idle:       event.Counts.Idle,
			}
		}
		observations[index] = observation
	}
	return observations, nil
}

func buildRevision() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	return buildRevisionFromInfo(info, ok)
}

func buildRevisionFromInfo(info *debug.BuildInfo, ok bool) (string, bool) {
	if !ok || info == nil || info.GoVersion != "go1.27.3" {
		return "", false
	}
	version, clean, sdk := "", false, false
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			version = setting.Value
		}
		if setting.Key == "vcs.modified" {
			clean = setting.Value == "false"
		}
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/actions/scaleset" {
			sdk = dep.Version == "v0.5.0" && dep.Replace == nil
		}
	}
	return version, clean && sdk && len(version) == 40
}

var pairedPrepareJournalForCommand = livecanary.PreparePairedJournal
var prepareWorkerJournalForCommand = liveworker.PrepareJournal
var prepareReplacementJournalForCommand = livecanary.PrepareSessionReplacementJournal
var prepareJITBoundaryJournalForCommand = livecanary.PrepareJITBoundaryJournal
var prepareReplacementJournalWithRootBindingForCommand = livecanary.PrepareSessionReplacementJournalWithRootBinding
var prepareJITBoundaryJournalWithRootBindingForCommand = livecanary.PrepareJITBoundaryJournalWithRootBinding
var prepareRootJournalForCommand = livecanary.PrepareJournalWithRootBinding
var verifyRootBindingForCommand = livecanary.VerifyControllerExperimentRootBinding

// These legacy direct-execution flags stay rejected. The signed controller
// handoff is the only executable path; a local approval or stdin stream cannot
// select credentials, caller, phase or paired execution.
const controllerExecutionQuarantineMessage = "canary quarantined; broker provenance is required"

func run(args []string, in io.Reader, out io.Writer) int {
	return runWithPreparation(args, in, out, buildRevision, livecanary.PrepareJournal)
}
func runWithPreparation(args []string, in io.Reader, out io.Writer, revisionForBuild func() (string, bool), prepareJournal func(string, livecanary.Approval, string) (livecanary.PreparationReceipt, error)) int {
	return runWithPreparationContext(context.Background(), args, in, out, revisionForBuild, prepareJournal)
}
func runWithPreparationContext(ctx context.Context, args []string, in io.Reader, out io.Writer, revisionForBuild func() (string, bool), prepareJournal func(string, livecanary.Approval, string) (livecanary.PreparationReceipt, error)) (code int) {
	// SDK errors and panic values can contain bearer credentials/response bodies.
	// This last-resort boundary prints no dynamic exception or caller input.
	defer func() {
		if recover() != nil {
			fmt.Fprintln(out, "canary stopped; retain private state for review")
			code = 1
		}
	}()
	flags := flag.NewFlagSet("g01-live", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	plan := flags.Bool("plan", false, "")
	sanitizePublicEvidence := flags.Bool("sanitize-public-evidence", false, "")
	execute := flags.Bool("execute-approved-canary", false, "")
	experimental := flags.Bool("execute-experimental-phase", false, "")
	experimentalMode := flags.String("mode", "", "")
	pairedExecute := flags.Bool("execute-approved-paired-terminal", false, "")
	prepare := flags.Bool("prepare-approved-journal", false, "")
	pairedPrepare := flags.Bool("prepare-approved-paired-journal", false, "")
	prepareWorker := flags.Bool("prepare-approved-paired-worker-journal", false, "")
	approvalPath := flags.String("approval", "", "")
	statePath := flags.String("state-dir", "", "")
	phase := flags.String("phase", "", "")
	workerApprovalPath := flags.String("worker-approval", "", "")
	workerStatePath := flags.String("worker-state-dir", "", "")
	pairedBinding := flags.String("paired-binding", "", "")
	rootBinding := flags.String("root-binding", "", "")
	reject := func() int {
		fmt.Fprintln(out, "canary refused; approval, authority or private state requires review")
		return 1
	}
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return reject()
	}
	if *sanitizePublicEvidence {
		if *plan || *execute || *experimental || *pairedExecute || *prepare || *pairedPrepare || *prepareWorker || *experimentalMode != "" || *approvalPath != "" || *statePath != "" || *phase != "" || *workerApprovalPath != "" || *workerStatePath != "" || *pairedBinding != "" || *rootBinding != "" {
			return reject()
		}
		observations, err := decodePublicEvidenceInput(in)
		if err != nil {
			fmt.Fprintln(out, "public evidence refused")
			return 1
		}
		encoded, err := livecanary.MarshalPublicEvidence(observations)
		if err != nil {
			fmt.Fprintln(out, "public evidence refused")
			return 1
		}
		encoded = append(encoded, '\n')
		if n, err := out.Write(encoded); err != nil || n != len(encoded) {
			return 1
		}
		return 0
	}
	if *execute || *pairedExecute {
		fmt.Fprintln(out, controllerExecutionQuarantineMessage)
		return 1
	}
	workerInputs := *workerApprovalPath != "" || *workerStatePath != ""
	if *experimental {
		if ctx == nil || ctx.Err() != nil {
			return reject()
		}
		if *plan || *execute || *pairedExecute || *prepare || *pairedPrepare || *prepareWorker || *pairedBinding != "" || *rootBinding != "" || *approvalPath == "" || *statePath == "" || *phase == "" {
			return reject()
		}
		a, e := livecanary.ReadApproval(*approvalPath)
		if e != nil {
			return reject()
		}
		revision, ok := revisionForBuild()
		if !ok || revision != a.HarnessSHA {
			return reject()
		}
		input, ok := in.(*os.File)
		if !ok {
			return reject()
		}
		files := livecanary.PairedTerminalFiles{ControllerApprovalPath: *approvalPath, ControllerStateDirectory: *statePath, WorkerApprovalPath: *workerApprovalPath, WorkerStateDirectory: *workerStatePath}
		if livecanary.RunControllerExperiment(ctx, files, *experimentalMode, *phase, input) != nil {
			return reject()
		}
		fmt.Fprintln(out, "experimental phase completed; inspect private evidence")
		return 0
	}
	if *experimentalMode != "" {
		return reject()
	}
	if *rootBinding != "" && !*prepare {
		return reject()
	}

	if *plan && !*experimental && !*prepare && !*pairedPrepare && !*prepareWorker {
		if *approvalPath != "" || *statePath != "" || *phase != "" || workerInputs || *pairedBinding != "" {
			return reject()
		}
		fmt.Fprintln(out, "Controller-only phases: create, before-ack, after-ack, before-acquire, acquire-loss, jit-loss, drain, inspect, cleanup. Paired terminal mode uses one same-process executable with explicit worker approval/state inputs and fixed terminal sequencing; no worker launch or workflow dispatch. Live execution requires an immutable reviewed build, exact private approval and controller-side broker input.")
		return 0
	}
	modeCount := 0
	if *prepare {
		modeCount++
	}
	if *pairedPrepare {
		modeCount++
	}
	if *prepareWorker {
		modeCount++
	}
	if modeCount != 1 || *plan || *approvalPath == "" || *statePath == "" {
		return reject()
	}
	if *prepareWorker {
		if *phase != "" || workerInputs || *pairedBinding != "" || *rootBinding != "" || prepareWorkerJournalForCommand == nil {
			return reject()
		}
	} else if *pairedPrepare {
		if *phase != "" || workerInputs || *pairedBinding != "" || *rootBinding != "" {
			return reject()
		}
	} else if *phase == "" || workerInputs || *pairedBinding != "" {
		return reject()
	}
	if *prepareWorker {
		worker, workerErr := liveworker.ReadApproval(*approvalPath)
		if workerErr != nil || worker.Validate(time.Now()) != nil {
			return reject()
		}
		revision, ok := revisionForBuild()
		if !ok || revision != worker.HarnessSHA {
			return reject()
		}
		receipt, preparationErr := prepareWorkerJournalForCommand(*statePath, worker)
		if preparationErr != nil || json.NewEncoder(out).Encode(receipt) != nil {
			return reject()
		}
		return 0
	}
	a, err := livecanary.ReadApproval(*approvalPath)
	if err != nil || a.Validate(time.Now()) != nil || (!*pairedPrepare && !slices.Contains(a.Phases, *phase)) {
		return reject()
	}
	revision, ok := revisionForBuild()
	if !ok || revision != a.HarnessSHA {
		return reject()
	}
	var expectedRootBinding *handoff.RootBinding
	if *rootBinding != "" {
		binding, e := handoff.DecodeStrictRootBinding([]byte(*rootBinding))
		if e != nil || verifyRootBindingForCommand(binding) != nil {
			return reject()
		}
		expectedRootBinding = &binding
	}
	if *pairedPrepare {
		receipt, e := pairedPrepareJournalForCommand(*statePath, a)
		if e != nil || json.NewEncoder(out).Encode(receipt) != nil {
			return reject()
		}
		return 0
	}
	if *prepare {
		if prepareJournal == nil {
			return reject()
		}
		var receipt livecanary.PreparationReceipt
		var e error
		if expectedRootBinding != nil {
			switch {
			case *phase == "before-jit" || *phase == "jit-loss":
				if prepareJITBoundaryJournalWithRootBindingForCommand == nil {
					return reject()
				}
				receipt, e = prepareJITBoundaryJournalWithRootBindingForCommand(*statePath, a, *approvalPath, *phase, *expectedRootBinding)
			case *phase == "session-replacement":
				if prepareReplacementJournalWithRootBindingForCommand == nil {
					return reject()
				}
				receipt, e = prepareReplacementJournalWithRootBindingForCommand(*statePath, a, *approvalPath, *expectedRootBinding)
			default:
				if prepareRootJournalForCommand == nil {
					return reject()
				}
				receipt, e = prepareRootJournalForCommand(*statePath, a, *phase, *expectedRootBinding)
			}
		} else if *phase == "before-jit" || *phase == "jit-loss" {
			if prepareJITBoundaryJournalForCommand == nil {
				return reject()
			}
			receipt, e = prepareJITBoundaryJournalForCommand(*statePath, a, *approvalPath, *phase)
		} else if *phase == "session-replacement" {
			if prepareReplacementJournalForCommand == nil {
				return reject()
			}
			receipt, e = prepareReplacementJournalForCommand(*statePath, a, *approvalPath)
		} else {
			receipt, e = prepareJournal(*statePath, a, *phase)
		}
		if e != nil {
			return reject()
		}
		if expectedRootBinding != nil && (receipt.RootBinding == nil || !receipt.RootBinding.Equal(*expectedRootBinding)) {
			return reject()
		}
		if json.NewEncoder(out).Encode(receipt) != nil {
			return reject()
		}
		return 0
	}
	return reject()
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(runWithPreparationContext(ctx, os.Args[1:], os.Stdin, os.Stdout, buildRevision, livecanary.PrepareJournal))
}
