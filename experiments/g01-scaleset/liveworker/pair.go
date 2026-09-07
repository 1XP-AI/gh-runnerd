package liveworker

// RecordRef identifies an assigned durable event in its bound journal domain.
// It is a fact to compare with replay, not a bearer capability.
type RecordRef struct {
	Sequence    int    `json:"sequence"`
	EventSHA256 string `json:"event_sha256"`
}

type FileIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type JournalIdentity struct {
	OwnershipSHA256    string       `json:"ownership_sha256"`
	State              FileIdentity `json:"state"`
	Journal            FileIdentity `json:"journal"`
	AdmissionDirectory FileIdentity `json:"admission_directory"`
	Claim              FileIdentity `json:"claim"`
}

type PairSource struct {
	Organization  string `json:"organization"`
	Repository    string `json:"repository"`
	RepositoryID  int64  `json:"repository_id"`
	WorkflowRunID int64  `json:"workflow_run_id"`
	WorkflowPath  string `json:"workflow_path"`
	HeadSHA       string `json:"head_sha"`
	Attempt       int    `json:"attempt"`
}

type PairInput struct {
	Controller     JournalIdentity `json:"controller"`
	Source         PairSource      `json:"source"`
	RunnerGroupID  int             `json:"runner_group_id"`
	ScaleSetID     int             `json:"scale_set_id"`
	ScaleSetName   string          `json:"scale_set_name"`
	SetCreation    RecordRef       `json:"set_creation"`
	OwnerNonce     string          `json:"owner_nonce"`
	ControllerName string          `json:"controller_name"`
	HarnessSHA     string          `json:"harness_sha"`
}

type PairBinding struct {
	Version int             `json:"version"`
	Input   PairInput       `json:"input"`
	Worker  JournalIdentity `json:"worker"`
}

type PairReceipt struct {
	PairSHA256       string    `json:"pair_sha256"`
	ControllerIntent RecordRef `json:"controller_intent"`
	WorkerBound      RecordRef `json:"worker_bound"`
}

type SDKRunnerID int

type SDKRunnerIdentity struct {
	ID         SDKRunnerID `json:"id"`
	Name       string      `json:"name"`
	ScaleSetID int         `json:"scale_set_id"`
}

type HandoffReceipt struct {
	PairSHA256    string            `json:"pair_sha256"`
	PairResult    RecordRef         `json:"pair_result"`
	AcquireResult RecordRef         `json:"acquire_result"`
	JITResult     RecordRef         `json:"jit_result"`
	HandoffIntent RecordRef         `json:"handoff_intent"`
	RequestID     int64             `json:"request_id"`
	Runner        SDKRunnerIdentity `json:"runner"`
}

type ContainerReceipt struct {
	PairSHA256    string    `json:"pair_sha256"`
	HandoffIntent RecordRef `json:"handoff_intent"`
	CreateIntent  RecordRef `json:"create_intent"`
	CreateResult  RecordRef `json:"create_result"`
	ContainerID   string    `json:"container_id"`
	EnvDigest     string    `json:"env_digest"`
	LabelsDigest  string    `json:"labels_digest"`
}

type LocalOutcome string

const (
	LocalProfilePresent   LocalOutcome = "profile-present"
	LocalNotFoundReported LocalOutcome = "not-found-reported"
	LocalUnknown          LocalOutcome = "unknown"
)

type LocalReceipt struct {
	PairSHA256  string            `json:"pair_sha256"`
	Intent      RecordRef         `json:"intent"`
	Result      RecordRef         `json:"result"`
	ContainerID string            `json:"container_id"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	HTTPStatus  int               `json:"http_status"`
	Outcome     LocalOutcome      `json:"outcome"`
	State       *DockerStateFacts `json:"state"`
}

type TerminalDecisionRef struct {
	PairSHA256         string    `json:"pair_sha256"`
	ContainerID        string    `json:"container_id"`
	CreateResult       RecordRef `json:"create_result"`
	ControllerDecision RecordRef `json:"controller_decision"`
}

type DeletionReceipt struct {
	PairSHA256    string     `json:"pair_sha256"`
	ContainerID   string     `json:"container_id"`
	Intent        RecordRef  `json:"intent"`
	Result        RecordRef  `json:"result"`
	AbsenceResult *RecordRef `json:"absence_result"`
}

type CheckStage string

const (
	CheckScope  CheckStage = "scope"
	CheckBind   CheckStage = "bind"
	CheckCreate CheckStage = "create"
	CheckStart  CheckStage = "start"
	CheckDelete CheckStage = "delete"
)

type ControllerCheck struct {
	Stage            CheckStage      `json:"stage"`
	PairSHA256       string          `json:"pair_sha256"`
	ControllerRecord RecordRef       `json:"controller_record"`
	WorkerRecord     RecordRef       `json:"worker_record"`
	Handoff          *HandoffReceipt `json:"handoff"`
}
