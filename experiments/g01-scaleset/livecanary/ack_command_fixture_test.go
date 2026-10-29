//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
	"github.com/actions/scaleset"
)

// Every child owns an original signed synthetic completed-create prefix. The
// broker/preparer/receiver, protected FD3/stdin transport, SDK and journal are
// actual TEST commands; this does not establish actual create/workload lineage.
type ackCommandFixture struct {
	*acquireLossCommandFixture
	phase, variant, sid, admin string
	message                    int
	prefix                     []Event
	original                   []byte
	authority                  map[string][]byte
	mu                         sync.Mutex
	counts                     map[string]int
	steps                      []string
	brokerAttempts             []string
	entryMu                    sync.Mutex
	handlers                   sync.WaitGroup
	closing                    bool
	originalContext            *originalContextCommandCase
}

func newACKCommandFixture(t *testing.T, controller, broker, phase, variant string) *ackCommandFixture {
	f := &ackCommandFixture{acquireLossCommandFixture: newCommandQualificationFixture(t, controller, broker, variant, phase), phase: phase, variant: variant, sid: "00000000-0000-4000-8000-000000000001", message: 9, counts: map[string]int{}, authority: map[string][]byte{}}
	if variant == "job0" {
		f.message = 0
	}
	f.prefix = f.base.readEvents()
	f.original = f.read(filepath.Join(f.base.j.directory, "journal.jsonl"))
	for _, path := range []string{f.approvalPath, f.brokerApprovalPath, filepath.Join(f.base.j.directory, "paired-fixture.json"), filepath.Join(f.base.j.directory, "broker-experiment-fixture.json"), f.base.j.claim.file.Name()} {
		f.authority[path] = f.read(path)
	}
	// Override the loose baseline dispatcher completely. Attempts are counted
	// before body draining/validation/denial; valid acquire/close replies expose
	// intentional faults instead of masking them with a denied effect.
	f.additionalSecrets = append(f.additionalSecrets, []byte("synthetic-secret-acquire"), []byte("synthetic-secret-display"), []byte("failed to delete message"), []byte("failed to get next message"), []byte("failed to issue the request"), []byte("Get \""), []byte("Delete \""))
	f.base.beforeRequest = f.serve
	f.brokerBeforeRequest = f.brokerWire
	t.Cleanup(f.freezeHandlers)
	return f
}

func (f *ackCommandFixture) read(path string) []byte {
	f.t.Helper()
	data, e := os.ReadFile(path)
	if e != nil {
		f.t.Error("ACK private evidence unavailable")
	}
	return data
}

func (f *ackCommandFixture) freezeHandlers() {
	f.entryMu.Lock()
	f.closing = true
	f.entryMu.Unlock()
	f.handlers.Wait()
}

func (f *ackCommandFixture) serve(w http.ResponseWriter, r *http.Request) bool {
	f.entryMu.Lock()
	late := f.closing
	if !late {
		f.handlers.Add(1)
	}
	f.entryMu.Unlock()
	if !late {
		defer f.handlers.Done()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	t, a := f.t, f.base.a
	key := r.Method + " " + r.URL.Path
	f.counts[key]++
	if f.counts[key] != 1 {
		f.t.Error("ACK repeated physical attempt")
	}
	f.steps = append(f.steps, key)
	if late {
		t.Error("late receiver API attempt after settled command")
		w.WriteHeader(403)
		return true
	}
	endpoint, _ := url.Parse(f.base.api.baseURL)
	if r.Host != endpoint.Host || r.TLS == nil || r.ProtoMajor != 1 || r.URL.EscapedPath() != r.URL.Path {
		t.Error("ACK physical origin/TLS/HTTP1/path differs")
	}
	data, e := io.ReadAll(io.LimitReader(r.Body, 4097))
	r.Body.Close()
	if e != nil || len(data) > 4096 {
		t.Error("ACK physical bounded body differs")
	}
	// Store credentials only in memory for fixed-class scans. Never log them.
	for _, auth := range r.Header.Values("Authorization") {
		f.secretMu.Lock()
		f.additionalSecrets = append(f.additionalSecrets, []byte(auth), []byte(strings.TrimPrefix(auth, "Bearer ")))
		f.secretMu.Unlock()
	}
	query := func(want string) {
		if r.URL.RawQuery != want {
			t.Error("ACK physical exact query differs")
		}
	}
	auth := func(want string) {
		if !reflect.DeepEqual(r.Header.Values("Authorization"), []string{want}) {
			t.Error("ACK physical authorization differs")
		}
	}
	restHeaders := func() {
		if !reflect.DeepEqual(r.Header.Values("Accept"), []string{"application/vnd.github+json"}) || !reflect.DeepEqual(r.Header.Values("X-GitHub-Api-Version"), []string{"2022-11-28"}) {
			t.Error("ACK exact REST headers differ")
		}
	}
	content := func() {
		if !reflect.DeepEqual(r.Header.Values("Content-Type"), []string{"application/json"}) {
			t.Error("ACK physical JSON header differs")
		}
	}
	empty := func() {
		if len(data) != 0 {
			t.Error("ACK physical empty body differs")
		}
	}
	reply := func(status int, body any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			json.NewEncoder(w).Encode(body)
		}
	}
	runtime := "/tenant/_apis/runtime/runnerscalesets/7"
	set := scaleset.RunnerScaleSet{ID: 7, Name: a.setName(), RunnerGroupID: a.RunnerGroupID, Labels: []scaleset.Label{{Name: a.setName()}}, RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true}, Statistics: &scaleset.RunnerScaleSetStatistic{}}
	queueAuth := "Bearer synthetic-secret-queue"
	switch key {
	case "GET /installation/repositories", "GET /orgs/fixture-org/actions/runner-groups/3/repositories", "GET /repos/fixture-org/canary", "GET /orgs/fixture-org/actions/runner-groups/3":
		f.checkpoint("preflight", 8)
		restHeaders()
		auth("Bearer synthetic-process-installation-token")
		empty()
		q := ""
		if strings.HasSuffix(r.URL.Path, "/repositories") {
			q = "per_page=100"
		}
		query(q)
		acquireLossPreflight(w, r, a)
	case "POST /api/v3/orgs/fixture-org/actions/runners/registration-token":
		f.checkpoint("bootstrap-token", 10)
		if !reflect.DeepEqual(r.Header.Values("Content-Type"), []string{"application/vnd.github.v3+json"}) {
			t.Error("ACK registration-token content header differs")
		}
		query("")
		empty()
		auth("Bearer synthetic-process-installation-token")
		reply(201, map[string]string{"token": "synthetic-registration"})
	case "POST /api/v3/actions/runner-registration":
		f.checkpoint("bootstrap-admin", 10)
		query("")
		content()
		auth("RemoteAuth synthetic-registration")
		want, _ := json.Marshal(struct {
			URL   string `json:"url"`
			Event string `json:"runner_event"`
		}{f.base.api.baseURL + "/fixture-org", "register"})
		if !bytes.Equal(data, append(want, '\n')) {
			t.Error("ACK exact SDK runner-registration body differs")
		}
		claims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
		token := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + "."
		f.admin = "Bearer " + token
		f.secretMu.Lock()
		f.additionalSecrets = append(f.additionalSecrets, []byte(token))
		f.secretMu.Unlock()
		reply(200, map[string]string{"url": f.base.api.baseURL + "/tenant/", "token": token})
	case "GET " + runtime:
		f.checkpoint("owned", 10)
		query("api-version=6.0-preview")
		content()
		auth(f.admin)
		empty()
		reply(200, set)
	case "POST " + runtime + "/sessions":
		f.checkpoint("open", 12)
		query("api-version=6.0-preview")
		content()
		auth(f.admin)
		want, _ := json.Marshal(&scaleset.RunnerScaleSetSession{OwnerName: a.setName()})
		if !bytes.Equal(data, want) {
			t.Error("ACK exact SDK session-open body differs")
		}
		reply(200, scaleset.RunnerScaleSetSession{SessionID: mustACKSession(), OwnerName: a.setName(), MessageQueueURL: f.base.api.baseURL + "/queue", MessageQueueAccessToken: "synthetic-secret-queue", Statistics: &scaleset.RunnerScaleSetStatistic{}})
	case "GET /queue":
		f.checkpoint("poll", 14)
		query("")
		auth(queueAuth)
		empty()
		if !reflect.DeepEqual(r.Header.Values(scaleset.HeaderScaleSetMaxCapacity), []string{"1"}) || !reflect.DeepEqual(r.Header.Values("Accept"), []string{"application/json; api-version=6.0-preview"}) {
			t.Error("ACK physical poll capacity/accept differs")
		}
		items := []any{baselineFixtureItem(a, "JobAvailable")}
		stats := scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}
		if f.variant == "empty" {
			items = []any{}
			stats = scaleset.RunnerScaleSetStatistic{}
		}
		body, _ := json.Marshal(items)
		// A map ensures a present numeric zero before the SDK DTO can collapse it.
		reply(200, map[string]any{"messageId": f.message, "messageType": "RunnerScaleSetJobMessages", "body": string(body), "statistics": stats})
	case "GET " + workflowRunFixturePath(f.transport):
		f.checkpoint("source", 15)
		restHeaders()
		query("")
		empty()
		auth("Bearer " + f.base.api.credentials.VerificationToken)
		run := observationRun(a)
		if f.variant == "foreign-source" {
			run["head_sha"] = strings.Repeat("b", 40)
		}
		reply(200, run)
	case "DELETE /queue/" + strconv.Itoa(f.message):
		f.checkpoint("ACK", 17)
		query("")
		content()
		auth(queueAuth)
		empty()
		reply(204, nil)
	case "POST " + runtime + "/acquirejobs":
		// Keep this precise physical witness ahead of generic history/quarantine.
		if f.originalContext == nil || !f.originalContext.acquireAllowed() {
			t.Errorf("ACK physical acquire0 witness: actual=%d want=0", f.counts[key])
		}
		query("api-version=6.0-preview")
		content()
		auth(queueAuth)
		if !bytes.Equal(data, []byte("[42]")) {
			t.Error("ACK physical acquire request differs")
		}
		if f.originalContext != nil {
			f.checkpoint("acquire", 19)
		}
		events := f.base.readEvents()
		if len(events) != 19 || !reflect.DeepEqual(events[18], Event{Sequence: 19, Kind: "intent", Operation: "acquire", RequestIDs: []int64{42}}) {
			t.Error("ACK bypass acquire-intent19 witness differs")
		}
		reply(200, map[string]any{"count": 1, "value": []int64{42}})
	case "DELETE " + runtime + "/sessions/" + f.sid:
		end := 20
		if f.variant == "empty" {
			end = 16
		}
		if f.originalContext != nil && !f.originalContext.closeAllowed() {
			t.Errorf("original context physical close0 witness: actual=%d want=0", f.counts[key])
		}
		f.checkpoint("close", end)
		query("api-version=6.0-preview")
		content()
		auth(f.admin)
		empty()
		reply(204, nil)
	default:
		t.Error("ACK forbidden physical attempt")
		reply(403, nil)
	}
	return true
}

// Observe the actual broker server entry before the shared transport dispatches
// or drains a body. Keep provenance and final workflow verification distinct.
func (f *ackCommandFixture) brokerWire(r *http.Request) {
	t := f.t
	if r.TLS == nil || r.ProtoMajor != 1 || r.URL.EscapedPath() != r.URL.Path {
		t.Error("ACK broker physical TLS/HTTP1/path differs")
	}
	key := r.Method + " " + r.URL.Path
	f.brokerAttempts = append(f.brokerAttempts, key) // count before body drain or dispatch
	primary := []string{"GET " + workflowRunFixturePath(f.transport), "GET /app", "GET /orgs/fixture-org/installation", "GET " + workflowRunFixturePath(f.transport), "POST /app/installations/12/access_tokens", "GET /installation/repositories", "GET /repos/fixture-org/canary", "GET /orgs/fixture-org/actions/runner-groups/3", "GET /orgs/fixture-org/actions/runner-groups/3/repositories"}
	position := len(f.brokerAttempts) - 1 // shared lock held; already counted before drain
	fresh := f.transport.attempt == f.attempt+"-fresh"
	if fresh {
		if key != primary[0] || position != len(primary) {
			t.Error("ACK fresh broker exact observation-only entry differs")
		}
	} else if position >= len(primary) || primary[position] != key {
		t.Error("ACK broker physical exact call order differs")
	}
	data, e := io.ReadAll(io.LimitReader(r.Body, 4097))
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(data))
	if e != nil || len(data) > 4096 {
		t.Error("ACK broker bounded body differs")
	}
	query := ""
	if r.URL.Path == "/installation/repositories" || strings.HasSuffix(r.URL.Path, "/runner-groups/3/repositories") {
		query = "per_page=100"
	}
	if r.URL.RawQuery != query {
		t.Error("ACK broker exact query differs")
	}
	for header, want := range map[string]string{"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"} {
		if !reflect.DeepEqual(r.Header.Values(header), []string{want}) {
			t.Error("ACK broker exact REST headers differ")
		}
	}
	identityAPI := key == "GET /app" || key == "GET /orgs/fixture-org/installation"
	if identityAPI {
		if r.Header.Get("User-Agent") != "gh-runnerd-g02-evidence" || len(r.Header.Values("Content-Type")) != 0 {
			t.Error("ACK broker identity headers differ")
		}
	} else if r.Header.Get("User-Agent") != "gh-runnerd-g01-broker-evidence" || !reflect.DeepEqual(r.Header.Values("Content-Type"), []string{"application/json"}) || !reflect.DeepEqual(r.Header.Values("Accept-Encoding"), []string{"identity"}) {
		t.Error("ACK broker observation/scoped headers differ")
	}
	auth := r.Header.Get("Authorization")
	if len(r.Header.Values("Authorization")) != 1 {
		t.Error("ACK broker authorization multiplicity differs")
	}
	f.secretMu.Lock()
	f.additionalSecrets = append(f.additionalSecrets, []byte(auth), []byte(strings.TrimPrefix(auth, "Bearer ")))
	f.secretMu.Unlock()
	switch key {
	case "GET /app", "GET /orgs/fixture-org/installation", "POST /app/installations/12/access_tokens":
		f.verifyAppJWT(strings.TrimPrefix(auth, "Bearer "))
	default:
		token := "synthetic-process-installation-token"
		if r.URL.Path == workflowRunFixturePath(f.transport) {
			token = f.base.api.credentials.VerificationToken
			if position == 0 || fresh {
				token = "synthetic-observation-credential"
			}
		}
		if auth != "Bearer "+token {
			t.Error("ACK broker distinct observation/verification/installation authority differs")
		}
	}
	if key == "POST /app/installations/12/access_tokens" {
		if string(data) != `{"repository_ids":[42],"permissions":{"metadata":"read","organization_self_hosted_runners":"write"}}` {
			t.Error("ACK broker exact scoped mint body differs")
		}
	} else if len(data) != 0 {
		t.Error("ACK broker unexpected body")
	}
}

func (f *ackCommandFixture) verifyAppJWT(token string) {
	t := f.t
	parts := strings.Split(token, ".")
	var management struct {
		PEM string `json:"pem"`
	}
	if len(parts) != 3 || json.Unmarshal(f.management, &management) != nil {
		t.Error("ACK broker App JWT setup differs")
		return
	}
	block, _ := pem.Decode([]byte(management.PEM))
	if block == nil {
		t.Error("ACK broker synthetic App key unavailable")
		return
	}
	key, e := x509.ParsePKCS1PrivateKey(block.Bytes)
	if e != nil {
		t.Error("ACK broker synthetic App key invalid")
		return
	}
	header, he := base64.RawURLEncoding.DecodeString(parts[0])
	payload, pe := base64.RawURLEncoding.DecodeString(parts[1])
	signature, se := base64.RawURLEncoding.DecodeString(parts[2])
	var claims struct {
		Issuer  string `json:"iss"`
		Issued  int64  `json:"iat"`
		Expires int64  `json:"exp"`
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	now := time.Now().Unix()
	if he != nil || pe != nil || se != nil || string(header) != `{"alg":"RS256","typ":"JWT"}` || DecodeStrict(payload, &claims) != nil || claims.Issuer != strconv.FormatInt(f.base.a.AppID, 10) || claims.Issued > now || claims.Expires <= now || claims.Expires-claims.Issued != 600 || rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], signature) != nil {
		t.Error("ACK broker App JWT signature/identity/window differs")
	}
}

func mustACKSession() [16]byte { return [16]byte{6: 0x40, 8: 0x80, 15: 1} }

// Bind the full consumption to the actual broker's permanent provenance claim;
// the signed receipt itself remains in the protected transient transport.
func (f *ackCommandFixture) consumption() *controllerHandoffConsumption {
	raw := f.read(filepath.Join(f.admission, "broker-admission.jsonl"))
	var found *controllerHandoffConsumption
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		var event struct {
			Kind       string `json:"kind"`
			Slot       string `json:"slot"`
			Provenance *struct {
				Digest string `json:"digest"`
				Nonce  string `json:"receipt_nonce"`
				Source string `json:"source"`
			} `json:"provenance"`
		}
		if json.Unmarshal(line, &event) != nil {
			f.t.Error("ACK broker claim decode failed")
			continue
		}
		if event.Kind == "claim" && event.Slot == f.phase && event.Provenance != nil {
			p := event.Provenance
			if found != nil || p.Source != "github-rest-workflow-v1" || len(p.Digest) != 64 || len(p.Nonce) != 32 {
				f.t.Error("ACK original claim provenance differs")
			}
			found = &controllerHandoffConsumption{ReceiptNonce: p.Nonce, Phase: f.phase, ApprovalSHA256: handoff.Digest(f.raw), ReceiptSHA256: p.Digest, KeyID: "fixture-ed25519-v1"}
		}
	}
	if found == nil {
		f.t.Error("ACK actual broker phase claim missing")
	}
	return found
}

func (f *ackCommandFixture) expected() []Event {
	events := append([]Event(nil), f.prefix...)
	add := func(e Event) { e.Sequence = len(events) + 1; events = append(events, e) }
	add(Event{Kind: "controller-handoff-consumed", ControllerHandoff: f.consumption()})
	add(Event{Kind: "phase", Operation: f.phase})
	add(Event{Kind: "intent", Operation: "observe-owned"})
	add(Event{Kind: "result", Operation: "observe-owned"})
	add(Event{Kind: "intent", Operation: "session-open"})
	add(Event{Kind: "result", Operation: "session-open", SessionID: f.sid})
	add(Event{Kind: "intent", Operation: "observe-poll"})
	work := workDemand
	if f.variant == "empty" {
		work = ""
	}
	add(Event{Kind: "result", Operation: "observe-poll", Work: work})
	switch f.variant {
	case "foreign-source":
		add(Event{Kind: "unknown", Operation: "probe"})
	case "empty":
		add(Event{Kind: "intent", Operation: "session-close"})
		add(Event{Kind: "result", Operation: "session-close", SessionID: f.sid})
	default:
		add(Event{Kind: "observation", Operation: "poll", ID: f.message, SessionID: f.sid, RequestIDs: []int64{42}})
		add(Event{Kind: "intent", Operation: "ack"})
		add(Event{Kind: "result", Operation: "ack", ID: f.message, SessionID: f.sid})
		add(Event{Kind: "barrier", Operation: f.phase})
		add(Event{Kind: "intent", Operation: "session-close"})
		add(Event{Kind: "result", Operation: "session-close", SessionID: f.sid})
	}
	if f.originalContext != nil {
		return f.originalContext.expected(events[:18])
	}
	return events
}

func (f *ackCommandFixture) checkpoint(stage string, end int) {
	f.t.Helper()
	events := f.base.readEvents()
	want := f.expected()
	if len(events) != end || end > len(want) {
		f.t.Errorf("ACK %s entry durable length=%d want=%d", stage, len(events), end)
		return
	}
	for i, event := range events {
		if !reflect.DeepEqual(event, want[i]) {
			f.t.Errorf("ACK %s entry durable full-event witness sequence=%d kind=%s operation=%s wantKind=%s wantOperation=%s", stage, i+1, event.Kind, event.Operation, want[i].Kind, want[i].Operation)
		}
	}
	raw := f.read(filepath.Join(f.base.j.directory, "journal.jsonl"))
	if !bytes.HasPrefix(raw, f.original) {
		f.t.Error("ACK original header/create bytes changed")
	}
	for path, before := range f.authority {
		if !bytes.Equal(before, f.read(path)) {
			f.t.Error("ACK original authority/raw/claim changed")
		}
	}
	// Exactly the existing authority: no renewal or extra authority records.
	var header journalHeader
	if DecodeStrict(bytes.Split(raw, []byte{'\n'})[0], &header) != nil || header.Authority.Digest != approvalDigest(f.base.a) || !header.Authority.ExpiresAt.Equal(f.base.a.ExpiresAt) || !reflect.DeepEqual(header.Authority.Phases, f.base.a.Phases) {
		f.t.Error("ACK original header authority differs")
	}
}

func (f *ackCommandFixture) assertSettled() []Event {
	if f.originalContext != nil {
		return f.assertOriginalContextSettled()
	}
	f.t.Helper()
	t := f.t
	f.mu.Lock()
	defer f.mu.Unlock()
	runtime := "/tenant/_apis/runtime/runnerscalesets/7"
	ack, source, close := 1, 1, 1
	if f.variant == "foreign-source" {
		ack, close = 0, 0
	}
	if f.variant == "empty" {
		ack, source = 0, 0
	}
	counts := map[string]int{
		"GET /installation/repositories": 1, "GET /repos/fixture-org/canary": 1, "GET /orgs/fixture-org/actions/runner-groups/3": 1, "GET /orgs/fixture-org/actions/runner-groups/3/repositories": 1,
		"POST /api/v3/orgs/fixture-org/actions/runners/registration-token": 1, "POST /api/v3/actions/runner-registration": 1,
		"GET " + runtime: 1, "POST " + runtime + "/sessions": 1, "GET /queue": 1,
	}
	if source > 0 {
		counts["GET "+workflowRunFixturePath(f.transport)] = source
	}
	if ack > 0 {
		counts["DELETE /queue/"+strconv.Itoa(f.message)] = ack
	}
	if close > 0 {
		counts["DELETE "+runtime+"/sessions/"+f.sid] = close
	}
	// These physical witnesses precede every generic settled-history/guard check.
	if n := f.counts["POST "+runtime+"/acquirejobs"]; n != 0 {
		t.Errorf("ACK physical acquire0 witness: actual=%d want=0", n)
	}
	if n := f.counts["DELETE /queue/"+strconv.Itoa(f.message)]; n != ack {
		t.Errorf("ACK physical ACK1 witness: actual=%d want=%d", n, ack)
	}
	if !reflect.DeepEqual(f.counts, counts) {
		t.Errorf("ACK physical exact operation counts differ: actual=%v want=%v", f.counts, counts)
	}
	wantSteps := []string{"GET /installation/repositories", "GET /repos/fixture-org/canary", "GET /orgs/fixture-org/actions/runner-groups/3", "GET /orgs/fixture-org/actions/runner-groups/3/repositories", "POST /api/v3/orgs/fixture-org/actions/runners/registration-token", "POST /api/v3/actions/runner-registration", "GET " + runtime, "POST " + runtime + "/sessions", "GET /queue"}
	if source > 0 {
		wantSteps = append(wantSteps, "GET "+workflowRunFixturePath(f.transport))
	}
	if ack > 0 {
		wantSteps = append(wantSteps, "DELETE /queue/"+strconv.Itoa(f.message))
	}
	if close > 0 {
		wantSteps = append(wantSteps, "DELETE "+runtime+"/sessions/"+f.sid)
	}
	if !reflect.DeepEqual(f.steps, wantSteps) {
		t.Error("ACK exact physical ordering differs")
	}
	events := f.base.readEvents()
	want := f.expected()
	if !reflect.DeepEqual(events, want) {
		t.Errorf("ACK full settled events differ: count=%d want=%d", len(events), len(want))
	}
	j, e := OpenJournalForPairedFixtureAt(f.base.j.directory, f.base.a, f.admission)
	if e != nil {
		t.Fatal("ACK postexit concrete FileJournal reopen failed")
	}
	if !reflect.DeepEqual(j.Events(), events) {
		t.Error("ACK strict bytes and concrete reopen differ")
	}
	s := replayWithApproval(j.Events(), &f.base.a)
	if j.Close() != nil {
		t.Fatal("ACK concrete reopen close failed")
	}
	primary := f.variant == "job9" || f.variant == "job0"
	jobs := map[int64]bool{}
	if primary {
		jobs[42] = true
	}
	sid := ""
	if f.variant == "foreign-source" {
		sid = f.sid
	}
	if s.setID != 7 || s.deleted || !reflect.DeepEqual(s.phaseSeen, map[string]bool{"create": true, f.phase: true}) || !reflect.DeepEqual(s.observedJobs, jobs) || s.workObserved != (f.variant != "empty") || s.reserved != primary || s.uncertain != (f.variant == "foreign-source") || s.sessionID != sid {
		t.Error("ACK concrete replay work/reservation/uncertainty/session/authority differs")
	}
	f.checkpoint("settled", len(want))
	return events
}

func (f *ackCommandFixture) assertBroker() {
	f.brokerMu.Lock()
	defer f.brokerMu.Unlock()
	want := []string{"GET " + workflowRunFixturePath(f.transport), "GET /app", "GET /orgs/fixture-org/installation", "GET " + workflowRunFixturePath(f.transport), "POST /app/installations/12/access_tokens", "GET /installation/repositories", "GET /repos/fixture-org/canary", "GET /orgs/fixture-org/actions/runner-groups/3", "GET /orgs/fixture-org/actions/runner-groups/3/repositories"}
	if !reflect.DeepEqual(f.brokerAttempts, f.transport.calls) {
		f.t.Error("ACK broker server-entry counts differ from dispatched calls")
	}
	if f.transport.mints != 1 || !reflect.DeepEqual(f.transport.calls, want) {
		f.t.Errorf("ACK actual broker exact trace differs: calls=%v mints=%d", f.transport.calls, f.transport.mints)
	}
	raw := f.read(filepath.Join(f.attempt, "broker.jsonl"))
	completed := 0
	prepared := 0
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		var event struct {
			Phase    string `json:"phase"`
			Metadata struct {
				Receipt PreparationReceipt `json:"receipt"`
			} `json:"metadata"`
		}
		if json.Unmarshal(line, &event) != nil {
			f.t.Error("ACK broker durable record decode differs")
		}
		if event.Phase == "controller_completed" {
			completed++
		}
		if event.Phase == "controller_state_prepared" {
			prepared++
			f.validatePreparation(event.Metadata.Receipt, f.phase, f.original)
		}
	}
	wantComplete := 0
	if (f.variant == "job9" || f.variant == "job0") && (f.originalContext == nil || f.originalContext.closeAllowed()) {
		wantComplete = 1
	}
	if completed != wantComplete || prepared != 1 {
		f.t.Error("ACK durable broker preparation/completion differs")
	}
	// Completion preserves the permanent occupied claim and all its provenance.
	ledger := bytes.Split(bytes.TrimSpace(f.read(filepath.Join(f.admission, "broker-admission.jsonl"))), []byte{'\n'})
	if len(ledger) != 2+wantComplete {
		f.t.Error("ACK exact broker claim/completion count differs")
	} else if wantComplete == 1 {
		var claim, completion map[string]any
		if json.Unmarshal(ledger[1], &claim) != nil || json.Unmarshal(ledger[2], &completion) != nil || claim["kind"] != "claim" || completion["kind"] != "complete" {
			f.t.Error("ACK durable broker claim/completion unavailable")
		}
		completion["kind"] = "claim"
		if !reflect.DeepEqual(claim, completion) {
			f.t.Error("ACK completion changed occupied claim/provenance")
		}
	}
	snapshot := f.read(filepath.Join(f.attempt, "controller-approval.json"))
	if !bytes.Equal(snapshot, f.raw) {
		f.t.Error("ACK actual broker original raw approval snapshot differs")
	}
}

func (f *ackCommandFixture) validatePreparation(receipt PreparationReceipt, phase string, journal []byte) {
	identity := func(path string) PreparedIdentity {
		info, e := os.Stat(path)
		if e != nil {
			f.t.Error("ACK preparation identity unavailable")
			return PreparedIdentity{}
		}
		return preparedIdentity(info)
	}
	claimPath := f.base.j.claim.file.Name()
	want := PreparationReceipt{Version: 1, Status: "controller_journal_prepared", Phase: phase, ApprovalDigest: approvalDigest(f.base.a), State: identity(f.base.j.directory), Journal: identity(filepath.Join(f.base.j.directory, "journal.jsonl")), Claim: identity(claimPath), AdmissionDirectory: identity(f.admission), JournalDigest: handoff.Digest(journal), ClaimDigest: handoff.Digest(f.read(claimPath))}
	if !reflect.DeepEqual(receipt, want) {
		f.t.Error("ACK actual preparation full concrete receipt differs")
	}
}

func (f *ackCommandFixture) alternatePreparation() {
	// Both generic phases were already in the unchanged original approval.
	alternate := "after-ack"
	if f.phase == alternate {
		alternate = "before-acquire"
	}
	journal := filepath.Join(f.base.j.directory, "journal.jsonl")
	before := f.read(journal)
	claimPath := filepath.Join(f.admission, "broker-admission.jsonl")
	claim := f.read(claimPath)
	requests := f.base.requests.Load()
	f.brokerMu.Lock()
	calls, mints := len(f.transport.calls), f.transport.mints
	f.brokerMu.Unlock()
	args := []string{"--prepare-approved-journal", "--approval", f.approvalPath, "--state-dir", f.base.j.directory, "--phase", alternate}
	if f.variant == "empty" {
		var receipt PreparationReceipt
		f.runExpected(f.controller, args, commandJournalPrepared, &receipt)
		f.validatePreparation(receipt, alternate, before)
		f.run(f.controller, []string{"--prepare-approved-journal", "--approval", f.approvalPath, "--state-dir", f.base.j.directory, "--phase", f.phase})
	} else {
		f.run(f.controller, args)
	}
	f.brokerMu.Lock()
	defer f.brokerMu.Unlock()
	if !bytes.Equal(before, f.read(journal)) || !bytes.Equal(claim, f.read(claimPath)) || f.base.requests.Load() != requests || calls != len(f.transport.calls) || mints != f.transport.mints {
		f.t.Error("ACK alternate preparation changed history/claim or crossed network/mint boundary")
	}
}

func TestExperimentalACKBoundaryCommand(t *testing.T) {
	t.Run("original-context-protocol", originalContextProtocolNegatives)
	t.Run("original-context-setup", originalContextSetupNegatives)
	controller, broker := acquireLossHelpers(t)
	for _, phase := range []string{"after-ack", "before-acquire"} {
		for _, variant := range []string{"job9", "job0", "foreign-source", "empty"} {
			t.Run(phase+"/"+variant, func(t *testing.T) {
				f := newACKCommandFixture(t, controller, broker, phase, variant)
				f.runOutcome(f.broker, f.brokerArgs(f.attempt), variant == "job9" || variant == "job0", f.management, []byte("synthetic-observation-credential"), f.workflowSeed, f.issuerSeed)
				f.freezeHandlers()
				events := f.assertSettled()
				f.assertBroker()
				if t.Failed() {
					return
				} // Keep target-specific physical/durable fault witnesses.
				if variant == "job9" {
					f.guardControlsForPhase(events, phase)
				}
				if variant == "job9" || variant == "empty" {
					f.alternatePreparation()
				}
				f.assertSettled()
				f.scanCommandJournals()
			})
		}
	}
	for _, mode := range []string{"cancel", "continue"} {
		t.Run("after-ack/original-"+mode+"-after-ack-result", func(t *testing.T) { runOriginalContextCommandCase(t, controller, broker, "after-ack", mode) })
	}
}
