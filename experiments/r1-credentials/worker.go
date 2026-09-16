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
// Env, argv, files and JIT are unexported so external callers cannot forge
// launch material; only PlanWorkerLaunch returns a provenance-marked plan.
type WorkerLaunchPlan struct {
	AppID          int64
	Organization   Organization
	InstallationID int64
	Repository     Repository
	env            []string
	argv           []string
	files          map[string][]byte
	hasJIT         bool
	proof          workerLaunchProof
}

type workerLaunchProof struct {
	appID          int64
	organization   Organization
	installationID int64
	repository     Repository
}

func newWorkerLaunchPlan(binding ValidatedBinding) WorkerLaunchPlan {
	return WorkerLaunchPlan{
		AppID:          binding.AppID,
		Organization:   binding.Organization,
		InstallationID: binding.InstallationID,
		Repository:     binding.Repository,
		proof: workerLaunchProof{
			appID:          binding.AppID,
			organization:   binding.Organization,
			installationID: binding.InstallationID,
			repository:     binding.Repository,
		},
	}
}

func (p WorkerLaunchPlan) hasValidatedConstruction() bool {
	if p.proof == (workerLaunchProof{}) {
		return false
	}
	return p.proof == workerLaunchProof{
		appID:          p.AppID,
		organization:   p.Organization,
		installationID: p.InstallationID,
		repository:     p.Repository,
	} && len(p.env) == 0 && len(p.argv) == 0 && len(p.files) == 0 && !p.hasJIT
}

func (WorkerLaunchPlan) String() string   { return "[redacted worker launch plan]" }
func (WorkerLaunchPlan) GoString() string { return "[redacted worker launch plan]" }
func (WorkerLaunchPlan) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted worker launch plan]"`), nil
}

// PlanWorkerLaunch copies identity metadata from a provenance-marked binding
// and refuses every worker surface that could carry management credentials or
// an unreviewed JIT envelope. Zero, forged and mutated bindings return
// ErrConfig. It never reads os.Environ. JIT minting stays a G01 live gap.
func PlanWorkerLaunch(req WorkerLaunchRequest) (WorkerLaunchPlan, error) {
	if !req.Binding.hasValidatedProvenance() {
		return WorkerLaunchPlan{}, ErrConfig
	}
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
	return newWorkerLaunchPlan(req.Binding), nil
}
