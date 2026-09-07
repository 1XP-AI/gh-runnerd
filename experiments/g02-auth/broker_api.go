package enrollment

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type brokerAPI struct {
	github *GitHubAPI
	client *http.Client
	now    func() time.Time
}
type brokerTransport struct{ inner http.RoundTripper }

func (t brokerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" || r.URL.User != nil || r.URL.Fragment != "" || r.Host != "api.github.com" {
		return nil, errBroker
	}
	res, err := t.inner.RoundTrip(r)
	if err != nil {
		return nil, errBroker
	}
	if res == nil || res.Body == nil {
		return nil, errBroker
	}
	defer res.Body.Close()
	if res.ContentLength > maxResponseBytes || (res.Header.Get("Content-Encoding") != "" && res.Header.Get("Content-Encoding") != "identity") {
		return nil, errBroker
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return nil, errBroker
	}
	// Apply duplicate-key/depth checks to identity responses reused from G02 too.
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		var value any
		if decodeBrokerJSON(data, &value, false) != nil {
			return nil, errBroker
		}
	}
	res.Body = io.NopCloser(bytes.NewReader(data))
	return res, nil
}
func newBrokerAPI(now func() time.Time, fixture http.RoundTripper) *brokerAPI {
	if now == nil {
		now = time.Now
	}
	if fixture == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Protocols = new(http.Protocols)
		transport.Protocols.SetHTTP1(true)
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = new(tls.Config)
		}
		transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
		transport.Proxy = nil
		transport.DisableCompression = true
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "api.github.com:443" {
				return nil, errBroker
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
		}
		fixture = transport
	}
	transport := brokerTransport{fixture}
	return &brokerAPI{github: NewGitHubAPI(now, transport), client: &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, now: now}
}
func (a *brokerAPI) call(ctx context.Context, method, path, authorization string, body any, status int, out any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return errBroker
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, bytes.NewReader(data))
	if err != nil {
		return errBroker
	}
	r.Header.Set("Authorization", authorization)
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept-Encoding", "identity")
	r.Header.Set("User-Agent", "gh-runnerd-g01-broker-evidence")
	res, err := a.client.Do(r)
	if err != nil {
		return errBroker
	}
	defer res.Body.Close()
	if res.ContentLength > maxResponseBytes || (res.Header.Get("Content-Encoding") != "" && res.Header.Get("Content-Encoding") != "identity") {
		return errBroker
	}
	data, err = io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes || res.StatusCode != status || decodeBrokerJSON(data, out, false) != nil {
		return errBroker
	}
	return nil
}

type brokerRepository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	Fork     *bool  `json:"fork"`
	Owner    struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"owner"`
}

func (r brokerRepository) matches(a BrokerApproval) bool {
	return r.ID == a.RepositoryID && r.FullName == a.Organization+"/"+a.Repository && r.Private && r.Fork != nil && !*r.Fork && r.Owner.ID == a.OrganizationID && r.Owner.Login == a.Organization
}

type brokerRepositories struct {
	TotalCount   int                `json:"total_count"`
	Repositories []brokerRepository `json:"repositories"`
}

func (r brokerRepositories) matches(a BrokerApproval) bool {
	return r.TotalCount == 1 && len(r.Repositories) == 1 && r.Repositories[0].matches(a)
}

type brokerIssued struct {
	Token               string             `json:"token"`
	ExpiresAt           time.Time          `json:"expires_at"`
	Permissions         map[string]string  `json:"permissions"`
	RepositorySelection string             `json:"repository_selection"`
	Repositories        []brokerRepository `json:"repositories"`
}

func (brokerIssued) String() string   { return "[redacted issuance]" }
func (brokerIssued) GoString() string { return "[redacted issuance]" }
func (a *brokerAPI) mint(ctx context.Context, approval BrokerApproval, cred Credential) (brokerIssued, error) {
	jwt, err := a.github.jwt(cred)
	if err != nil {
		return brokerIssued{}, errBroker
	}
	body := struct {
		RepositoryIDs []int64           `json:"repository_ids"`
		Permissions   map[string]string `json:"permissions"`
	}{[]int64{approval.RepositoryID}, map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"}}
	var result brokerIssued
	if a.call(ctx, "POST", "/app/installations/"+strconv.FormatInt(approval.InstallationID, 10)+"/access_tokens", "Bearer "+jwt, body, 201, &result) != nil || !validBrokerToken(result.Token) || len(result.Token) > 1024 || !result.ExpiresAt.After(a.now().Add(time.Minute)) || result.ExpiresAt.After(a.now().Add(65*time.Minute)) || len(result.Permissions) != 2 || result.Permissions["metadata"] != "read" || !minimalPermissions(result.Permissions) || result.RepositorySelection != "selected" || len(result.Repositories) != 1 || !result.Repositories[0].matches(approval) {
		return brokerIssued{}, errBroker
	}
	return result, nil
}
func (a *brokerAPI) preflight(ctx context.Context, approval BrokerApproval, token string) error {
	auth := "Bearer " + token
	var installed brokerRepositories
	if a.call(ctx, "GET", "/installation/repositories?per_page=100", auth, nil, 200, &installed) != nil || !installed.matches(approval) {
		return errBroker
	}
	var current brokerRepository
	if a.call(ctx, "GET", "/repos/"+approval.Organization+"/"+approval.Repository, auth, nil, 200, &current) != nil || !current.matches(approval) {
		return errBroker
	}
	var group struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		Visibility   string `json:"visibility"`
		Default      *bool  `json:"default"`
		Inherited    *bool  `json:"inherited"`
		AllowsPublic *bool  `json:"allows_public_repositories"`
	}
	prefix := "/orgs/" + approval.Organization + "/actions/runner-groups/" + strconv.FormatInt(approval.RunnerGroupID, 10)
	if a.call(ctx, "GET", prefix, auth, nil, 200, &group) != nil || group.ID != approval.RunnerGroupID || group.Name != approval.RunnerGroupName || group.Visibility != "selected" || group.Default == nil || *group.Default || group.Inherited == nil || *group.Inherited || group.AllowsPublic == nil || *group.AllowsPublic {
		return errBroker
	}
	var selected brokerRepositories
	if a.call(ctx, "GET", prefix+"/repositories?per_page=100", auth, nil, 200, &selected) != nil || !selected.matches(approval) {
		return errBroker
	}
	return nil
}

var brokerDNSLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func brokerActionsHost(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Port() != "" && u.Port() != "443") {
		return "", errBroker
	}
	host := u.Hostname()
	if len(host) > 253 || !strings.HasSuffix(host, ".actions.githubusercontent.com") {
		return "", errBroker
	}
	for _, label := range strings.Split(host, ".") {
		if !brokerDNSLabel.MatchString(label) {
			return "", errBroker
		}
	}
	return host, nil
}
func (a *brokerAPI) discover(ctx context.Context, approval BrokerApproval, token string, j *brokerJournal) (string, error) {
	if j.append("registration_auth_started", nil) != nil {
		return "", errBroker
	}
	var registration struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if a.call(ctx, "POST", "/orgs/"+approval.Organization+"/actions/runners/registration-token", "Bearer "+token, nil, 201, &registration) != nil || !validBrokerToken(registration.Token) || len(registration.Token) > 1024 || !registration.ExpiresAt.After(a.now().Add(time.Minute)) || registration.ExpiresAt.After(a.now().Add(65*time.Minute)) {
		return "", errBroker
	}
	if j.append("tenant_auth_started", nil) != nil {
		return "", errBroker
	}
	var connection struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	body := map[string]string{"url": "https://github.com/" + approval.Organization, "runner_event": "register"}
	if a.call(ctx, "POST", "/actions/runner-registration", "RemoteAuth "+registration.Token, body, 200, &connection) != nil || !validBrokerToken(connection.Token) {
		return "", errBroker
	}
	// The tenant is never contacted in discovery; only this validated hostname
	// crosses the result boundary. Registration/admin credentials are discarded.
	return brokerActionsHost(connection.URL)
}
