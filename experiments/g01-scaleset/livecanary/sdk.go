package livecanary

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/actions/scaleset"
	"github.com/hashicorp/go-retryablehttp"
)

// Credentials come only from the trusted controller-side broker on stdin.
// The broker attests issuance identity, permission and expiry; GitHub does not
// expose an installation-token introspection endpoint for those fields.
// Preflight independently proves installation-token use, repository scope and
// current target policy. It cannot independently prove broker provenance.
type Credentials struct {
	InstallationToken string                 `json:"installation_token"`
	VerificationToken string                 `json:"verification_token"`
	AppID             int64                  `json:"app_id"`
	InstallationID    int64                  `json:"installation_id"`
	Organization      string                 `json:"organization"`
	ExpiresAt         time.Time              `json:"expires_at"`
	SelfHostedRunners string                 `json:"organization_self_hosted_runners"`
	Metadata          string                 `json:"metadata"`
	PairedBinding     *PairedTerminalBinding `json:"paired_binding,omitempty"`
}

func (c Credentials) validate(a Approval, now time.Time) error {
	if c.AppID != a.AppID || c.InstallationID != a.InstallationID || c.Organization != a.Organization || c.SelfHostedRunners != "write" || c.Metadata != "read" || !c.ExpiresAt.After(now.Add(time.Minute)) || c.ExpiresAt.After(now.Add(65*time.Minute)) || len(c.InstallationToken) < 20 || len(c.InstallationToken) > 1024 || strings.ContainsAny(c.InstallationToken, "\r\n\x00") {
		return ErrApproval
	}
	needsVerification := slices.ContainsFunc(a.Phases, func(p string) bool {
		return p == "before-ack" || p == "after-ack" || p == "before-acquire" || p == "acquire-loss" || p == "drain"
	})
	if needsVerification && (len(c.VerificationToken) < 20 || len(c.VerificationToken) > 1024 || c.VerificationToken == c.InstallationToken || strings.ContainsAny(c.VerificationToken, "\r\n\x00")) {
		return ErrApproval
	}
	return nil
}

type SDKAPI struct {
	client      *scaleset.Client
	rest        *http.Client
	baseURL     string
	approval    Approval
	credentials Credentials
	options     []scaleset.HTTPOption
	// drainClientFactory is nil in production. Tests use it only to bind the
	// pinned SDK client to an offline loopback transport while still exercising
	// OpenDrainSession and MessageSessionClient together.
	drainClientFactory func(*drainPollHook) (*SDKAPI, error)
}

// NewSDKAPI is network-lazy. It permits only api.github.com and exact approved
// Actions hosts over direct TLS. No proxy, redirect, retry logger, automatic HTTP
// retry, credential environment read or worker process is configured here.
func NewSDKAPI(a Approval, c Credentials) (*SDKAPI, error) {
	return newSDKAPIWithPollHook(a, c, nil)
}

func newSDKAPIWithPollHook(a Approval, c Credentials, hook *drainPollHook) (*SDKAPI, error) {
	if a.Validate(time.Now()) != nil || c.validate(a, time.Now()) != nil {
		return nil, ErrApproval
	}
	transport := newSDKTransport(a)
	retry := retryablehttp.NewClient()
	retry.RetryMax = 0
	retry.Logger = nil
	wrappers := []func(http.RoundTripper) http.RoundTripper(nil)
	if hook != nil {
		wrappers = append(wrappers, func(inner http.RoundTripper) http.RoundTripper {
			hook.inner = inner
			return hook
		})
	}
	retry.HTTPClient = &http.Client{Transport: withResponseBudget(transport, wrappers...), Timeout: operationTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	options := []scaleset.HTTPOption{scaleset.WithRetryableHTTPClint(retry), scaleset.WithLogger(slog.New(slog.DiscardHandler))}
	client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: "https://github.com/" + a.Organization, PersonalAccessToken: c.InstallationToken}, options...)
	if err != nil {
		return nil, ErrApproval
	}
	return &SDKAPI{client: client, rest: retry.HTTPClient, baseURL: "https://api.github.com", approval: a, credentials: c, options: options}, nil
}

// Keep the production transport construction separate so its TLS protocol and
// destination gates can be exercised against a private local TLS fixture.
func newSDKTransport(a Approval) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// HTTP/2 GODEBUG traces can print Authorization and response data. Pin the
	// inner credential transport, not just the response wrapper, to HTTP/1.
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	// Clone can inherit an already initialized HTTP/2 ALPN advertisement.
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = new(tls.Config)
	}
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		host := req.URL.Hostname()
		if req.URL.Scheme != "https" || req.URL.User != nil || (req.URL.Port() != "" && req.URL.Port() != "443") || (host != "api.github.com" && !slices.Contains(a.ActionsHosts, host)) {
			return nil, ErrApproval
		}
		return nil, nil
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" || (host != "api.github.com" && !slices.Contains(a.ActionsHosts, host)) {
			return nil, ErrApproval
		}
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
	}
	return transport
}

func (a *SDKAPI) get(ctx context.Context, path, token string, target any) error {
	if !a.credentials.ExpiresAt.After(time.Now()) {
		return ErrApproval
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return ErrRemote
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := a.rest.Do(req)
	if err != nil {
		return ErrRemote
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrRemote
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, target) != nil {
		return ErrRemote
	}
	return nil
}

type repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	Fork     bool   `json:"fork"`
}
type repositories struct {
	TotalCount   int          `json:"total_count"`
	Repositories []repository `json:"repositories"`
}

func (a *SDKAPI) Preflight(ctx context.Context, approval Approval) error {
	if a.credentials.validate(approval, time.Now()) != nil {
		return ErrApproval
	}
	var installed repositories
	// The broker must narrow this temporary token to the one private repository.
	if a.get(ctx, "/installation/repositories?per_page=100", a.credentials.InstallationToken, &installed) != nil {
		return ErrApproval
	}
	if installed.TotalCount != 1 || len(installed.Repositories) != 1 {
		return ErrApproval
	}
	r := installed.Repositories[0]
	if r.ID != approval.RepositoryID || r.FullName != approval.Organization+"/"+approval.Repository || !r.Private || r.Fork {
		return ErrApproval
	}
	var current repository
	if a.get(ctx, "/repos/"+approval.Organization+"/"+approval.Repository, a.credentials.InstallationToken, &current) != nil || current.ID != r.ID || current.FullName != r.FullName || !current.Private || current.Fork {
		return ErrApproval
	}
	var group struct {
		ID           int    `json:"id"`
		Visibility   string `json:"visibility"`
		Default      bool   `json:"default"`
		AllowsPublic bool   `json:"allows_public_repositories"`
		Inherited    bool   `json:"inherited"`
	}
	prefix := "/orgs/" + approval.Organization + "/actions/runner-groups/" + strconv.Itoa(approval.RunnerGroupID)
	if a.get(ctx, prefix, a.credentials.InstallationToken, &group) != nil || group.ID != approval.RunnerGroupID || group.Visibility != "selected" || group.Default || group.AllowsPublic || group.Inherited {
		return ErrApproval
	}
	var allowed repositories
	if a.get(ctx, prefix+"/repositories?per_page=100", a.credentials.InstallationToken, &allowed) != nil || allowed.TotalCount != 1 || len(allowed.Repositories) != 1 || allowed.Repositories[0].ID != r.ID {
		return ErrApproval
	}
	return nil
}

type workflowRun struct {
	ID             int64      `json:"id"`
	HeadSHA        string     `json:"head_sha"`
	Event          string     `json:"event"`
	Path           string     `json:"path"`
	RunAttempt     int        `json:"run_attempt"`
	Repository     repository `json:"repository"`
	HeadRepository repository `json:"head_repository"`
}

func matchesApprovedRun(approval Approval, id int64, run workflowRun) bool {
	return !(run.ID != id || run.HeadSHA != approval.WorkflowSHA || run.Event != "workflow_dispatch" || run.Path != approval.WorkflowPath || run.RunAttempt != 1 || run.Repository.ID != approval.RepositoryID || run.HeadRepository.ID != approval.RepositoryID || !run.Repository.Private || !run.HeadRepository.Private || run.Repository.Fork || run.HeadRepository.Fork)
}

func (a *SDKAPI) VerifyRun(ctx context.Context, approval Approval, id int64) error {
	if id <= 0 || id != approval.WorkflowRunID {
		return ErrApproval
	}
	out, err := a.observeApprovedRunFor(ctx, approval, id)
	if err != nil || out.Outcome == observationNotFound {
		return ErrApproval
	}
	return nil
}

func (a *SDKAPI) Inventory(ctx context.Context) (string, error) {
	digest, _, _, err := a.enumerateRoster(ctx, nil)
	return digest, err
}

// A nil guard preserves the legacy Inventory contract, including its error and
// cancellation semantics. Only observeRoster installs current-authority checks.
func (a *SDKAPI) enumerateRoster(ctx context.Context, guard func() error) (string, int, int, error) {
	var ids []int64
	seen := make(map[int64]bool)
	total, accepted := -1, 0
	check := func() bool { return guard == nil || guard() == nil }
	for page := 1; page <= 10; page++ {
		if !check() {
			return "", 0, accepted, ErrRemote
		}
		var list inventoryPage
		if a.get(ctx, "/orgs/"+a.approval.Organization+"/actions/runners?per_page=100&page="+strconv.Itoa(page), a.credentials.InstallationToken, &list) != nil {
			return "", 0, accepted, ErrRemote
		}
		if page == 1 {
			total = list.count
		}
		if list.count != total || len(ids)+len(list.ids) > total {
			return "", 0, accepted, ErrRemote
		}
		for _, id := range list.ids {
			if seen[id] {
				return "", 0, accepted, ErrRemote
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) != total && len(list.ids) == 0 || !check() {
			return "", 0, accepted, ErrRemote
		}
		accepted++
		if len(ids) == total {
			slices.Sort(ids)
			// Preserve sorted-ID encoding, including nil -> null for empty.
			data, _ := json.Marshal(ids)
			digest := sha256.Sum256(data)
			encoded := hex.EncodeToString(digest[:])
			if !check() {
				return "", 0, accepted, ErrRemote
			}
			return encoded, total, accepted, nil
		}
	}
	return "", 0, accepted, ErrRemote
}

func (a *SDKAPI) FindScaleSet(c context.Context, name string, group int) (*scaleset.RunnerScaleSet, error) {
	return a.client.GetRunnerScaleSet(c, group, name)
}
func (a *SDKAPI) GetScaleSet(c context.Context, id int) (*scaleset.RunnerScaleSet, error) {
	return a.client.GetRunnerScaleSetByID(c, id)
}

func (a *SDKAPI) drainEndpointHost() string {
	if a == nil {
		return ""
	}
	u, err := url.Parse(a.baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return ""
	}
	return u.Host
}

func (a *SDKAPI) drainGetScaleSet(c context.Context, id int, wire *baselineWireCapture) (*scaleset.RunnerScaleSet, error) {
	if wire == nil {
		return nil, ErrQuarantine
	}
	return a.GetScaleSet(wire.context(c), id)
}

func (a *SDKAPI) drainFindRunner(c context.Context, name string, wire *baselineWireCapture) (*scaleset.RunnerReference, error) {
	if a == nil || a.client == nil || wire == nil || name == "" {
		return nil, ErrQuarantine
	}
	wire.runnerName = name
	return a.client.GetRunnerByName(wire.context(c), name)
}

func validDrainQueueURL(value string, approvedHosts []string) bool {
	if value == "" || !baselineText(value, 4096) {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Fragment != "" || u.EscapedPath() != u.Path || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
		return false
	}
	queuePort := u.Port()
	if queuePort == "" {
		queuePort = "443"
	}
	if port, err := strconv.Atoi(queuePort); err != nil || port <= 0 || port > 65535 {
		return false
	}
	for _, approved := range approvedHosts {
		approvedHost, approvedPort, ok := drainApprovedHostPort(approved)
		if ok && strings.EqualFold(u.Hostname(), approvedHost) && queuePort == approvedPort {
			return true
		}
	}
	return false
}

func drainApprovedHostPort(value string) (string, string, bool) {
	if value == "" || strings.ContainsAny(value, "/?#@") {
		return "", "", false
	}
	host, port := value, "443"
	if strings.Contains(value, ":") {
		var err error
		host, port, err = net.SplitHostPort(value)
		if err != nil {
			return "", "", false
		}
		parsed, err := strconv.Atoi(port)
		if host == "" || err != nil || parsed <= 0 || parsed > 65535 {
			return "", "", false
		}
	}
	return host, port, true
}

func validDrainSessionWire(a Approval, id int, owner string, wire *baselineSessionFacts, session scaleset.RunnerScaleSetSession) bool {
	if wire == nil || session.SessionID == [16]byte{} || wire.SessionID != session.SessionID.String() || wire.Owner != owner || session.OwnerName != owner || !validDrainQueueURL(wire.queueURL, a.ActionsHosts) || wire.queueURL != session.MessageQueueURL || session.MessageQueueAccessToken == "" || !wire.Statistics.completeDrain() || !wire.NestedSet || wire.SetID != id || wire.SetName != owner || wire.GroupID != a.RunnerGroupID || !wire.NestedStatistics.completeDrain() || !wire.Statistics.matches(session.Statistics) {
		return false
	}
	set := session.RunnerScaleSet
	if set == nil || set.ID != id || set.Name != owner || set.RunnerGroupID != a.RunnerGroupID || !set.RunnerSetting.DisableUpdate || !slices.ContainsFunc(set.Labels, func(label scaleset.Label) bool { return label.Name == owner }) || set.Statistics == nil || !wire.NestedStatistics.matches(set.Statistics) {
		return false
	}
	return true
}

func (a *SDKAPI) CreateScaleSet(c context.Context, s *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error) {
	return a.client.CreateRunnerScaleSet(c, s)
}
func (a *SDKAPI) DeleteScaleSet(c context.Context, id int) error {
	return a.client.DeleteRunnerScaleSet(c, id)
}
func (a *SDKAPI) OpenSession(c context.Context, id int, owner string) (Session, error) {
	return a.client.MessageSessionClient(c, id, owner, a.options...)
}

func (a *SDKAPI) OpenDrainSession(c context.Context, id int, owner string, hook *drainPollHook) (Session, error) {
	if a == nil || hook == nil {
		return nil, ErrApproval
	}
	var configured *SDKAPI
	var err error
	if a.drainClientFactory != nil {
		configured, err = a.drainClientFactory(hook)
	} else {
		configured, err = newSDKAPIWithPollHook(a.approval, a.credentials, hook)
	}
	if err != nil {
		return nil, err
	}
	if configured == nil || configured.client == nil {
		return nil, ErrRemote
	}
	hook.mu.Lock()
	expectedOrigin := hook.origin
	expectedRuntimePathPrefix := hook.runtimePathPrefix
	expectedRuntimePathPrefixSet := hook.runtimePathPrefixSet
	hook.mu.Unlock()
	wire := &baselineWireCapture{stage: "session-open", setID: id, organization: configured.approval.Organization, owner: owner, origin: expectedOrigin, runtimePathPrefix: expectedRuntimePathPrefix, runtimePathPrefixSet: expectedRuntimePathPrefixSet, allowedHosts: baselineWireAllowedHosts(configured.approval, configured.drainEndpointHost())}
	session, err := configured.client.MessageSessionClient(wire.context(c), id, owner, configured.options...)
	if err != nil {
		return nil, ErrRemote
	}
	sessionFacts, _, _, status := wire.facts()
	sessionOrigin := wire.requestOrigin()
	sessionRuntimePathPrefix, prefixKnown := wire.requestRuntimePathPrefix()
	if session == nil || !wire.observed() || status != http.StatusOK || sessionOrigin == "" || !prefixKnown || !validDrainSessionWire(configured.approval, id, owner, sessionFacts, session.Session()) {
		return nil, ErrQuarantine
	}
	hook.mu.Lock()
	if (hook.origin != "" && hook.origin != sessionOrigin) || (hook.runtimePathPrefixSet && hook.runtimePathPrefix != sessionRuntimePathPrefix) {
		hook.invalid = true
		hook.mu.Unlock()
		return nil, ErrQuarantine
	}
	hook.target = sessionFacts.queueURL
	hook.origin = sessionOrigin
	hook.runtimePathPrefix = sessionRuntimePathPrefix
	hook.runtimePathPrefixSet = true
	hook.mu.Unlock()
	return session, nil
}
func (a *SDKAPI) FindRunner(c context.Context, name string) (*scaleset.RunnerReference, error) {
	return a.client.GetRunnerByName(c, name)
}
func (a *SDKAPI) GenerateJIT(c context.Context, id int, name string) (*scaleset.RunnerScaleSetJitRunnerConfig, error) {
	return a.client.GenerateJitRunnerConfig(c, &scaleset.RunnerScaleSetJitRunnerSetting{Name: name, WorkFolder: "_work"}, id)
}
