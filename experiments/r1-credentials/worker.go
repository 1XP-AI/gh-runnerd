package credentials

// WorkerLaunchRequest is the controller-side input for a one-job worker plan.
// Extra environment, argv, files and JIT must not carry management credentials.
type WorkerLaunchRequest struct {
	Binding   ValidatedBinding
	ExtraEnv  []string
	ExtraArgv []string
	Files     map[string][]byte
	JIT       []byte
}

// WorkerLaunchPlan is the metadata-only worker launch description. It has no
// App private key, JWT, installation token or inherited process environment.
type WorkerLaunchPlan struct {
	AppID          int64
	Organization   Organization
	InstallationID int64
	Repository     Repository
	Env            []string
	Argv           []string
	Files          map[string][]byte
	HasJIT         bool
}

func (WorkerLaunchPlan) String() string   { return "[redacted worker launch plan]" }
func (WorkerLaunchPlan) GoString() string { return "[redacted worker launch plan]" }
func (WorkerLaunchPlan) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted worker launch plan]"`), nil
}

// PlanWorkerLaunch copies identity metadata and refuses every worker surface
// that could carry management credentials or an unreviewed JIT envelope.
// It never reads os.Environ. JIT minting stays a G01 live gap.
func PlanWorkerLaunch(req WorkerLaunchRequest) (WorkerLaunchPlan, error) {
	if _, err := normalizeConfig(Config{
		AppID:          req.Binding.AppID,
		Organization:   req.Binding.Organization,
		InstallationID: req.Binding.InstallationID,
		Repository:     req.Binding.Repository,
	}); err != nil {
		return WorkerLaunchPlan{}, ErrConfig
	}
	if len(req.JIT) > 0 {
		return WorkerLaunchPlan{}, ErrJIT
	}
	if len(req.ExtraEnv) > 0 || len(req.ExtraArgv) > 0 || len(req.Files) > 0 {
		return WorkerLaunchPlan{}, ErrWorkerCredential
	}
	return WorkerLaunchPlan{
		AppID:          req.Binding.AppID,
		Organization:   req.Binding.Organization,
		InstallationID: req.Binding.InstallationID,
		Repository:     req.Binding.Repository,
	}, nil
}
