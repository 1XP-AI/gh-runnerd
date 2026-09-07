package enrollment

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AppIdentity struct {
	ID               int64
	Slug, OwnerLogin string
	OwnerID          int64
	OwnerType        string
}
type Proposal struct {
	Owner, AppName string
	Organizations  []Binding
}
type DriverAPI interface {
	API
	Convert(context.Context, string) (Candidate, error)
	DescribeApp(context.Context, Credential) (AppIdentity, error)
}
type DriverSummary struct {
	CredentialsNotPersisted bool  `json:"credentials_not_persisted"`
	VerifiedOrganizations   int   `json:"verified_organizations"`
	AppID                   int64 `json:"app_id"`
}

var errDriver = errors.New("verification incomplete; inspect existing App and use manual import")
var appSlug = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

func validateProposal(p Proposal, manual bool) error {
	if !organizationLogin.MatchString(p.Owner) || !appSlug.MatchString(p.AppName) || len(p.Organizations) != 2 {
		return errDriver
	}
	ownerFound := false
	logins := map[string]bool{}
	ids := map[int64]bool{}
	inst := map[int64]bool{}
	for _, b := range p.Organizations {
		login := strings.ToLower(b.Login)
		if !organizationLogin.MatchString(b.Login) || b.OrganizationID < 1 || logins[login] || ids[b.OrganizationID] || (manual && (b.InstallationID < 1 || inst[b.InstallationID])) {
			return errDriver
		}
		if strings.EqualFold(b.Login, p.Owner) {
			ownerFound = true
		}
		logins[login] = true
		ids[b.OrganizationID] = true
		inst[b.InstallationID] = true
	}
	if !ownerFound {
		return errDriver
	}
	return nil
}
func verifyIdentity(ctx context.Context, p Proposal, c Candidate, api DriverAPI) error {
	cred, err := parseCredential(c)
	if err != nil {
		return errDriver
	}
	identity, err := api.DescribeApp(ctx, cred)
	if err != nil {
		return errDriver
	}
	var ownerID int64
	for _, b := range p.Organizations {
		if strings.EqualFold(p.Owner, b.Login) {
			ownerID = b.OrganizationID
		}
	}
	if identity.ID != c.AppID || identity.Slug != p.AppName || !strings.EqualFold(identity.OwnerLogin, p.Owner) || identity.OwnerID != ownerID || identity.OwnerType != "Organization" {
		return errDriver
	}
	return nil
}

// Driver is a ten-minute, current-process evidence harness. It deliberately has
// no persistence sink for credentials and no runner/token-broker integration.
type Driver struct {
	mu        sync.Mutex
	proposal  Proposal
	api       DriverAPI
	journal   *journal
	attempt   *Attempt
	candidate Candidate
	sink      memorySink
	baseURL   string
	server    *http.Server
	cancel    context.CancelFunc
	done      chan struct{}
	success   chan struct{}
	summary   DriverSummary
	err       error
}
type memorySink struct {
	credential Credential
	bindings   []Binding
}

func (s *memorySink) commit(ctx context.Context, c Credential, b []Binding) error {
	if ctx.Err() != nil {
		return errDriver
	}
	*s = memorySink{credential: c, bindings: append([]Binding(nil), b...)}
	return nil
}
func StartManifest(parent context.Context, p Proposal, path string, api DriverAPI, lifetime time.Duration) (*Driver, error) {
	if api == nil || parent == nil || lifetime <= 0 || lifetime > 10*time.Minute || validateProposal(p, false) != nil {
		return nil, errDriver
	}
	p.Organizations = append([]Binding(nil), p.Organizations...)
	j, err := openJournal(path, p, false, 0)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		j.close()
		return nil, errDriver
	}
	ctx, cancel := context.WithTimeout(parent, lifetime)
	d := &Driver{proposal: p, api: api, journal: j, baseURL: "http://" + listener.Addr().String(), cancel: cancel, done: make(chan struct{}), success: make(chan struct{})}
	d.attempt, err = NewAttempt(listener.Addr().String(), time.Now, d.convert)
	if err != nil {
		cancel()
		listener.Close()
		j.close()
		return nil, errDriver
	}
	d.attempt.onSuccess = func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusSeeOther) }
	d.attempt.onReject = cleanCallbackFailure
	d.server = &http.Server{Handler: d, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 2 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0), BaseContext: func(net.Listener) context.Context { return ctx }}
	served := make(chan error, 1)
	go func() { served <- d.server.Serve(listener) }()
	go func() {
		select {
		case <-d.success:
		case <-ctx.Done():
		case <-served:
		}
		stopCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		_ = d.server.Shutdown(stopCtx)
		stop()
		cancel()
		_ = d.server.Close()
		// Shutdown cancels requests before acquiring the lock held by API operations.
		d.mu.Lock()
		clear(d.candidate.PEM)
		d.candidate = Candidate{}
		d.sink = memorySink{}
		if !d.summary.CredentialsNotPersisted {
			d.err = errDriver
		}
		d.journal.close()
		d.mu.Unlock()
		close(d.done)
	}()
	return d, nil
}
func (d *Driver) URL() string                  { return d.baseURL }
func (d *Driver) Close()                       { d.cancel(); <-d.done }
func (d *Driver) Wait() (DriverSummary, error) { <-d.done; return d.summary, d.err }
func (d *Driver) convert(ctx context.Context, code string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.journal.record.Phase != "registration_started" || d.journal.phase("conversion_started", 0, nil) != nil {
		return errDriver
	}
	candidate, err := d.api.Convert(ctx, code)
	if err != nil {
		_ = d.journal.phase("conversion_failed", 0, nil)
		return errDriver
	}
	// Own our copy: test adapters and callers may retain their original bytes.
	candidate.PEM = append([]byte(nil), candidate.PEM...)
	if verifyIdentity(ctx, d.proposal, candidate, d.api) != nil {
		clear(candidate.PEM)
		_ = d.journal.phase("conversion_failed", candidate.AppID, nil)
		return errDriver
	}
	if d.journal.phase("app_received", candidate.AppID, nil) != nil {
		clear(candidate.PEM)
		return errDriver
	}
	d.candidate = candidate
	return nil
}
func browserHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self' https://github.com")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func (d *Driver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	browserHeaders(w)
	if r.Host != strings.TrimPrefix(d.baseURL, "http://") || r.URL.EscapedPath() != r.URL.Path || r.URL.Fragment != "" {
		http.Error(w, "invalid request", 400)
		return
	}
	if r.URL.Path == callbackPath {
		d.mu.Lock()
		prepared := d.journal.record.Phase == "prepared"
		d.mu.Unlock()
		if prepared {
			cleanCallbackFailure(w, r)
			return
		}
		d.attempt.ServeHTTP(w, r)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "invalid request", 400)
		return
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "/callback-result") {
		origins := r.Header.Values("Origin")
		if len(origins) > 1 || (len(origins) == 1 && origins[0] != d.baseURL) {
			http.Error(w, "invalid request", 403)
			return
		}
		if r.URL.Path == "/callback-result" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("Callback not accepted. Return to the local enrollment page. If conversion failed or was interrupted, inspect the recorded App and use manual import.\n"))
			return
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		d.renderHome(w)
		return
	}
	if r.Method != http.MethodPost || (r.URL.Path != "/register" && r.URL.Path != "/verify") {
		http.Error(w, "invalid request", 400)
		return
	}
	values, err := d.localForm(w, r)
	if err != nil {
		http.Error(w, "invalid local form", 403)
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if r.URL.Path == "/register" {
		if len(values) != 1 {
			http.Error(w, "invalid local form", 400)
			return
		}
		if d.journal.record.Phase != "prepared" {
			http.Error(w, "registration already started; inspect existing App", 409)
			return
		}
		if d.journal.phase("registration_started", 0, nil) != nil {
			http.Error(w, "journal unavailable", 500)
			return
		}
		d.renderManifest(w)
		return
	}
	if len(values) != 3 || len(values["installation_0"]) != 1 || len(values["installation_1"]) != 1 {
		http.Error(w, "invalid bindings", 400)
		return
	}
	if d.journal.record.Phase != "app_received" && d.journal.record.Phase != "verification_failed" {
		http.Error(w, "App unavailable", 409)
		return
	}
	candidate := d.candidate
	candidate.Organizations = append([]Binding(nil), d.proposal.Organizations...)
	for i := range candidate.Organizations {
		value := values.Get("installation_" + strconv.Itoa(i))
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id < 1 || strconv.FormatInt(id, 10) != value {
			http.Error(w, "invalid bindings", 400)
			return
		}
		candidate.Organizations[i].InstallationID = id
	}
	if d.journal.phase("verifying", 0, nil) != nil {
		http.Error(w, "journal unavailable", 500)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := ManualImport(ctx, candidate, d.api, d.sink.commit)
	if err != nil {
		d.sink = memorySink{}
		_ = d.journal.phase("verification_failed", 0, nil)
		http.Error(w, "organization verification failed", 400)
		return
	}
	if d.journal.phase("verified", result.AppID, result.Organizations) != nil {
		d.sink = memorySink{}
		http.Error(w, "journal unavailable", 500)
		return
	}
	d.summary = DriverSummary{CredentialsNotPersisted: true, VerifiedOrganizations: len(result.Organizations), AppID: result.AppID}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d.summary)
	close(d.success)
}

func cleanCallbackFailure(w http.ResponseWriter, r *http.Request) {
	// Fixed local path: never copy any query parameter into Location or the page.
	http.Redirect(w, r, "/callback-result", http.StatusSeeOther)
}
func (d *Driver) localForm(w http.ResponseWriter, r *http.Request) (url.Values, error) {
	origins := r.Header.Values("Origin")
	if len(origins) != 1 || origins[0] != d.baseURL {
		return nil, errDriver
	}
	types := r.Header.Values("Content-Type")
	if len(types) != 1 {
		return nil, errDriver
	}
	media, params, err := mime.ParseMediaType(types[0])
	if err != nil || media != "application/x-www-form-urlencoded" || len(params) != 0 {
		return nil, errDriver
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errDriver
	}
	values, err := url.ParseQuery(string(data))
	if err != nil || len(values["csrf"]) != 1 || subtle.ConstantTimeCompare([]byte(values.Get("csrf")), []byte(d.attempt.State())) != 1 {
		return nil, errDriver
	}
	for _, v := range values {
		if len(v) != 1 {
			return nil, errDriver
		}
	}
	return values, nil
}

var homeTemplate = template.Must(template.New("home").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><title>Verify GitHub App enrollment</title><h1>Verify GitHub App enrollment</h1><p>Owner: {{.Owner}}. App: {{.AppName}}. Credentials stay in this process and are not persisted.</p>{{if .Ready}}<p>Install this App in both listed organizations using GitHub, then independently confirm each installation ID.</p><form method="post" action="/verify"><input type="hidden" name="csrf" value="{{.CSRF}}">{{range $i,$b:=.Organizations}}<p><label>{{$b.Login}} (organization {{$b.OrganizationID}}): <input required name="installation_{{$i}}" inputmode="numeric" autocomplete="off"></label></p>{{end}}<button>Verify both organizations</button></form>{{else if .Prepared}}<form method="post" action="/register"><input type="hidden" name="csrf" value="{{.CSRF}}"><button>Prepare the approved GitHub form</button></form>{{else}}<p>Registration started. Continue the existing GitHub flow. If interrupted, inspect this App and use manual import; do not create another App.</p>{{end}}</html>`))

func (d *Driver) renderHome(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		Proposal
		CSRF            string
		Ready, Prepared bool
	}{d.proposal, d.attempt.State(), d.candidate.AppID > 0, d.journal.record.Phase == "prepared"}
	_ = homeTemplate.Execute(w, data)
}

var manifestTemplate = template.Must(template.New("manifest").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><title>Register approved disposable App</title><h1>Register {{.Name}}</h1><p>This creates one public GitHub App with organization self-hosted runners write and metadata read permissions. Webhook delivery is disabled. Keep the approved name unchanged. Submit once; if interrupted, inspect the existing App and use manual import.</p><form method="post" action="{{.Action}}"><input type="hidden" name="manifest" value="{{.Manifest}}"><button>Continue to GitHub</button></form></html>`))

func (d *Driver) renderManifest(w http.ResponseWriter) {
	manifest, _ := json.Marshal(map[string]any{"name": d.proposal.AppName, "url": "https://github.com/1XP-AI/gh-runnerd", "redirect_url": d.baseURL + callbackPath, "public": true, "hook_attributes": map[string]any{"active": false, "url": "https://example.invalid/gh-runnerd-g02-unused"}, "default_permissions": map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"}, "default_events": []string{}, "request_oauth_on_install": false})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = manifestTemplate.Execute(w, struct{ Name, Action, Manifest string }{d.proposal.AppName, "https://github.com/organizations/" + d.proposal.Owner + "/settings/apps/new?state=" + url.QueryEscape(d.attempt.State()), string(manifest)})
}

// VerifyManual consumes a bounded private input without writing it anywhere.
// It can reconcile an existing matching journal but never creates an App.
func VerifyManual(parent context.Context, p Proposal, path string, appID int64, input io.ReadCloser, api DriverAPI) (DriverSummary, error) {
	if input == nil {
		return DriverSummary{}, errDriver
	}
	defer input.Close()
	if parent == nil || api == nil || appID < 1 || validateProposal(p, true) != nil {
		return DriverSummary{}, errDriver
	}
	j, err := openJournal(path, p, true, appID)
	if err != nil {
		return DriverSummary{}, err
	}
	defer j.close()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	data, err := readPrivateInput(ctx, input, 32*1024)
	defer clear(data)
	if err != nil || len(data) > 32*1024 || ctx.Err() != nil {
		return DriverSummary{}, errDriver
	}
	candidate := Candidate{AppID: appID, PEM: data, Organizations: append([]Binding(nil), p.Organizations...)}
	if verifyIdentity(ctx, p, candidate, api) != nil {
		return DriverSummary{}, errDriver
	}
	if j.phase("verifying", appID, nil) != nil {
		return DriverSummary{}, errJournal
	}
	var sink memorySink
	defer func() { sink = memorySink{} }()
	result, err := ManualImport(ctx, candidate, api, sink.commit)
	if err != nil {
		_ = j.phase("verification_failed", 0, nil)
		return DriverSummary{}, errDriver
	}
	if j.phase("verified", result.AppID, result.Organizations) != nil {
		return DriverSummary{}, errJournal
	}
	return DriverSummary{CredentialsNotPersisted: true, VerifiedOrganizations: len(result.Organizations), AppID: result.AppID}, nil
}
