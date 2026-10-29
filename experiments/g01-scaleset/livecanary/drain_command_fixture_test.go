//go:build g01_pair_fixture && !g01_live && !g01_worker

package livecanary

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
	"github.com/actions/scaleset"
)

// This seam belongs only to the tagged offline adapter. Its endpoint copy must
// never become a signed approval or a Driver/journal/admission authority.
func TestPairedDrainFixtureConstructor(t *testing.T) {
	var targetCalls, proxyCalls atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyCalls.Add(1) }))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("ALL_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, target.URL, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	a := approval()
	raw, _ := json.Marshal(a)
	c := credentials(a)
	api, err := NewSDKAPIForPairedFixture(a, c, server.URL, ca)
	if err != nil || api.drainClientFactory == nil {
		t.Fatal("fixture drain constructor unavailable")
	}
	defer api.physical.CloseIdleConnections()
	hook := newDrainPollHook("")
	configured, err := api.drainClientFactory(hook)
	if err != nil {
		t.Fatal("fixture hook construction failed")
	}
	defer configured.physical.CloseIdleConnections()
	u, _ := url.Parse(server.URL)
	if !reflect.DeepEqual(configured.approval.ActionsHosts, []string{u.Host}) || configured.approval.Validate(time.Now()) == nil || !reflect.DeepEqual(api.approval, a) || !reflect.DeepEqual(configured.approval.Phases, a.Phases) || configured.credentials != c || hook.inner == nil {
		t.Fatal("private drain adapter copy or hook composition differs")
	}
	configured.approval.ActionsHosts[0] = "altered"
	configured.approval.Phases[0] = "altered"
	after, _ := json.Marshal(a)
	if !bytes.Equal(raw, after) || !reflect.DeepEqual(api.approval, a) {
		t.Fatal("fixture approval slices alias original authority")
	}
	for _, bad := range []struct {
		a Approval
		c Credentials
	}{{func() Approval { b := a; b.ActionsHosts = []string{u.Host}; return b }(), c}, {a, Credentials{}}} {
		if got, e := NewSDKAPIForPairedFixture(bad.a, bad.c, server.URL, ca); e == nil || got != nil {
			t.Error("invalid ORIGINAL authority admitted before fixture substitution")
		}
	}
	request := func(address string) error {
		r, _ := http.NewRequest(http.MethodGet, address, nil)
		r.Header.Set("Authorization", "Bearer synthetic-constructor-credential")
		response, e := configured.rest.Do(r)
		if response != nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		return e
	}
	if request(server.URL+"/ok") != nil || request(server.URL+"/redirect") != nil {
		t.Error("valid endpoint/redirect refusal failed")
	}
	if request(target.URL) == nil || request(strings.Replace(server.URL, "127.0.0.1", "localhost", 1)) == nil {
		t.Error("different fixture host/port received credential transport")
	}
	// A syntactically valid, wrong trust root must fail TLS, not fall back to
	// ambient roots or InsecureSkipVerify. httptest's standard cert is shared;
	// generate an unrelated synthetic root.
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	defer clear(key)
	cert := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"fixture.invalid"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, e := x509.CreateCertificate(rand.Reader, cert, cert, pub, key)
	if e != nil {
		t.Fatal("synthetic wrong CA setup failed")
	}
	badCA := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	wrong, e := NewSDKAPIForPairedFixture(a, c, server.URL, badCA)
	if e != nil {
		t.Fatal("syntactically valid wrong CA constructor setup failed")
	} else {
		defer wrong.physical.CloseIdleConnections()
		r, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		r.Header.Set("Authorization", "Bearer synthetic-constructor-credential")
		response, err := wrong.rest.Do(r)
		if response != nil {
			response.Body.Close()
		}
		if err == nil {
			t.Error("bad fixture CA accepted")
		}
	}
	if targetCalls.Load() != 0 || proxyCalls.Load() != 0 || configured.physical.Proxy != nil || configured.physical.TLSClientConfig.InsecureSkipVerify {
		t.Error("fixture redirect/proxy/TLS credential escape")
	}
}

type drainCommandGate struct {
	stage   string
	release chan struct{}
}
type drainCommandTrace struct {
	mu                                                                             sync.Mutex
	steps, capacities                                                              []string
	open, polls, ack, acquire, close, forbidden, sets, runners, sources, bootstrap int
	sid, queueAuthorization, adminAuthorization                                    string
	snapshotAuthorization                                                          string
	disconnected                                                                   bool
}
type drainCommandFixture struct {
	*acquireLossCommandFixture
	x         drainCommandTrace
	gates     chan drainCommandGate
	gateDone  chan struct{}
	handlers  sync.WaitGroup
	entryMu   sync.Mutex
	closing   bool
	variant   string
	authority map[string][]byte
}

func drainCommandStats(available, acquired, assigned, busy int) map[string]int {
	return map[string]int{"totalAvailableJobs": available, "totalAcquiredJobs": acquired, "totalAssignedJobs": assigned, "totalRunningJobs": 0, "totalRegisteredRunners": 1, "totalBusyRunners": busy, "totalIdleRunners": 1 - busy}
}

func newDrainCommandFixture(t *testing.T, controller, broker, variant string) *drainCommandFixture {
	f := &drainCommandFixture{acquireLossCommandFixture: newCommandQualificationFixture(t, controller, broker, "", "drain"), gates: make(chan drainCommandGate), gateDone: make(chan struct{}), variant: variant}
	a := f.base.a
	var original Approval
	if DecodeStrict(f.raw, &original) != nil || !original.ExpiresAt.Equal(a.ExpiresAt) {
		t.Fatal("original raw fixture authority decode/deadline differs")
	}
	original.ExpiresAt = a.ExpiresAt // JSON has no monotonic clock component.
	if !reflect.DeepEqual(original, a) || !reflect.DeepEqual(f.base.api.approval, a) {
		t.Fatal("original typed/raw fixture authority differs")
	}
	f.x.sid = "00000000-0000-4000-8000-000000000021"
	f.x.queueAuthorization = "Bearer synthetic-secret-queue"
	f.authority = make(map[string][]byte)
	for _, p := range []string{f.approvalPath, f.brokerApprovalPath, filepath.Join(f.base.j.directory, "paired-fixture.json"), filepath.Join(f.base.j.directory, "broker-experiment-fixture.json")} {
		data, e := os.ReadFile(p)
		if e != nil {
			t.Fatal("original signed authority/configuration unavailable")
		}
		f.authority[p] = data
	}
	claimPath := f.base.j.claim.file.Name()
	claim, e := os.ReadFile(claimPath)
	if e != nil {
		t.Fatal("original ownership claim unavailable")
	}
	f.authority[claimPath] = claim
	go func() {
		defer close(f.gateDone)
		for gate := range f.gates {
			f.durableCheckpoint(gate.stage)
			close(gate.release)
		}
	}()
	t.Cleanup(func() { f.freezeHandlers(); close(f.gates); <-f.gateDone })
	f.base.beforeRequest = func(w http.ResponseWriter, r *http.Request) bool {
		f.entryMu.Lock()
		if f.closing {
			// Keep every late attempt visible, including a guard escaping
			// admission. It cannot acquire a gate or race Wait/close.
			f.x.mu.Lock()
			f.x.forbidden++
			f.x.mu.Unlock()
			f.entryMu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			return true
		}
		f.handlers.Add(1)
		f.entryMu.Unlock()
		defer f.handlers.Done()
		x := &f.x
		x.mu.Lock()
		defer x.mu.Unlock()
		endpoint, _ := url.Parse(f.base.api.baseURL)
		if auth := r.Header.Get("Authorization"); auth != "" {
			f.secretMu.Lock()
			f.additionalSecrets = append(f.additionalSecrets, []byte(auth), []byte(strings.TrimPrefix(auth, "Bearer ")))
			f.secretMu.Unlock()
		}
		if r.Host != endpoint.Host || r.TLS == nil || r.ProtoMajor != 1 {
			t.Error("physical host/origin/TLS/HTTP1 differs")
		}
		runtime := "/tenant/_apis/runtime/runnerscalesets/7"
		body := func(status int, v any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if v != nil {
				json.NewEncoder(w).Encode(v)
			}
		}
		gate := func(stage string) {
			g := drainCommandGate{stage: stage, release: make(chan struct{})}
			select {
			case f.gates <- g:
			case <-r.Context().Done():
				t.Error("command checkpoint gate canceled")
				return
			}
			select {
			case <-g.release:
			case <-r.Context().Done():
				t.Error("command checkpoint release canceled")
			}
		}
		query := func(want url.Values) {
			if got, e := url.ParseQuery(r.URL.RawQuery); e != nil || !reflect.DeepEqual(got, want) {
				t.Error("physical exact query differs")
			}
		}
		apiQuery := url.Values{"api-version": {"6.0-preview"}}
		set := func() map[string]any {
			stats := drainCommandStats(0, 0, 0, 0)
			if variant == "prerequisite-busy" {
				stats = drainCommandStats(0, 0, 0, 1)
			} else if x.sets > 1 {
				stats = drainCommandStats(0, 1, 0, 0)
			}
			return map[string]any{"id": 7, "name": a.setName(), "runnerGroupId": a.RunnerGroupID, "labels": []map[string]string{{"name": a.setName(), "type": "System"}}, "runnerSetting": map[string]bool{"disableUpdate": true}, "statistics": stats}
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == runtime:
			x.sets++
			if x.sets == 1 {
				x.snapshotAuthorization = r.Header.Get("Authorization")
			}
			if x.snapshotAuthorization == "" || !exactDrainAuthorization(r.Header, strings.TrimPrefix(x.snapshotAuthorization, "Bearer ")) {
				t.Error("snapshot set authorization differs")
			}
			query(apiQuery)
			stage := "before-set"
			if x.sets > 1 {
				stage = "after-set"
			}
			x.steps = append(x.steps, stage)
			if x.sets > 2 {
				x.forbidden++
			}
			body(200, set())
		case r.Method == http.MethodGet && r.URL.Path == "/tenant/_apis/distributedtask/pools/0/agents":
			x.runners++
			if !exactDrainAuthorization(r.Header, strings.TrimPrefix(x.snapshotAuthorization, "Bearer ")) {
				t.Error("snapshot runner authorization differs")
			}
			query(url.Values{"api-version": {"6.0-preview"}, "agentName": {a.workerName()}})
			stage := "before-runner"
			if x.runners > 1 {
				stage = "after-runner"
			}
			x.steps = append(x.steps, stage)
			if x.runners > 2 {
				x.forbidden++
			}
			body(200, map[string]any{"count": 1, "value": []any{map[string]any{"id": 19, "name": a.workerName(), "runnerScaleSetId": 7}}})
		case r.Method == http.MethodPost && r.URL.Path == runtime+"/sessions":
			x.open++
			x.steps = append(x.steps, "open")
			query(apiQuery)
			x.adminAuthorization = r.Header.Get("Authorization")
			data, e := io.ReadAll(io.LimitReader(r.Body, 4097))
			r.Body.Close()
			var opened scaleset.RunnerScaleSetSession
			if e != nil || len(data) > 4096 || DecodeStrict(data, &opened) != nil || opened.OwnerName != a.setName() || opened.SessionID != [16]byte{} || opened.RunnerScaleSet != nil || opened.MessageQueueURL != "" || opened.MessageQueueAccessToken != "" || opened.Statistics != nil {
				t.Error("physical session open body differs")
			}
			if x.open != 1 || x.adminAuthorization == "" {
				x.forbidden++
			}
			body(200, map[string]any{"sessionId": x.sid, "ownerName": a.setName(), "runnerScaleSet": set(), "messageQueueUrl": f.base.api.baseURL + "/queue", "messageQueueAccessToken": "synthetic-secret-queue", "statistics": drainCommandStats(0, 0, 0, 0)})
		case r.Method == http.MethodGet && r.URL.Path == "/queue":
			x.polls++
			capacity := r.Header.Values(scaleset.HeaderScaleSetMaxCapacity)
			if len(capacity) != 1 {
				t.Error("physical capacity header multiplicity differs")
			}
			x.capacities = append(x.capacities, r.Header.Get(scaleset.HeaderScaleSetMaxCapacity))
			if !exactDrainAuthorization(r.Header, strings.TrimPrefix(x.queueAuthorization, "Bearer ")) {
				t.Error("physical poll authorization differs")
			}
			if x.polls == 1 {
				x.steps = append(x.steps, "poll-old")
				query(url.Values{})
				gate("poll-old")
				items, _ := json.Marshal([]any{baselineFixtureItem(a, "JobAvailable")})
				x.steps = append(x.steps, "old-response-release")
				body(200, map[string]any{"messageId": 17, "messageType": "RunnerScaleSetJobMessages", "body": string(items), "statistics": drainCommandStats(1, 0, 1, 0)})
			} else {
				x.steps = append(x.steps, "poll-zero")
				query(url.Values{"lastMessageId": {"17"}})
				gate("poll-zero")
				if x.polls != 2 {
					x.forbidden++
				}
				x.steps = append(x.steps, "zero-response-release")
				if variant == "withdrawn-empty202" {
					body(202, nil)
				} else {
					body(202, map[string]any{"statistics": drainCommandStats(0, 1, 0, 0)})
				}
			}
		case r.Method == http.MethodDelete && r.URL.Path == "/queue/17":
			x.ack++
			x.steps = append(x.steps, "ack")
			query(url.Values{})
			if x.ack != 1 || x.sources != 1 || r.Header.Get("Authorization") != x.queueAuthorization {
				t.Error("physical ACK source/order/authorization differs")
			}
			if x.ack > 1 {
				x.forbidden++
			}
			gate("ack")
			x.steps = append(x.steps, "ack-response-release")
			if variant == "ACK-response-lost" {
				conn, _, e := w.(http.Hijacker).Hijack()
				if e != nil {
					t.Error("ACK hijack setup failed")
					return true
				}
				if conn.Close() != nil {
					t.Error("ACK disconnect setup failed")
					return true
				}
				x.disconnected = true
			} else {
				body(204, nil)
			}
		case r.Method == http.MethodPost && r.URL.Path == runtime+"/acquirejobs":
			x.acquire++
			x.steps = append(x.steps, "acquire")
			query(apiQuery)
			data, e := io.ReadAll(io.LimitReader(r.Body, 65))
			r.Body.Close()
			if e != nil || !bytes.Equal(data, []byte("[42]")) || r.Header.Get("Authorization") != x.queueAuthorization || x.acquire != 1 {
				t.Error("physical acquire body/order/authorization differs")
			}
			if x.acquire > 1 {
				x.forbidden++
			}
			gate("acquire")
			body(200, map[string]any{"count": 1, "value": []int64{42}})
		case r.Method == http.MethodDelete && r.URL.Path == runtime+"/sessions/"+x.sid:
			x.close++
			x.steps = append(x.steps, "close")
			query(apiQuery)
			if x.close != 1 || r.Header.Get("Authorization") != x.adminAuthorization {
				t.Error("physical session close authorization differs")
			}
			if x.close > 1 {
				x.forbidden++
			}
			gate("close")
			body(204, nil)
		case r.Method == http.MethodGet && r.URL.Path == workflowRunFixturePath(f.transport):
			x.sources++
			x.steps = append(x.steps, "source-verify")
			if r.Header.Get("Authorization") != "Bearer "+f.base.api.credentials.VerificationToken || r.URL.RawQuery != "" {
				t.Error("source verification authority/query differs")
			}
			body(200, observationRun(a))
		case r.Method == http.MethodPost && (r.URL.Path == "/api/v3/orgs/fixture-org/actions/runners/registration-token" || r.URL.Path == "/api/v3/actions/runner-registration" || r.URL.Path == "/actions/runner-registration"):
			x.bootstrap++
			if x.open > 0 {
				x.forbidden++
				t.Error("post-session bootstrap attempted")
			}
			return false // explicitly allowed registration bootstrap, baseline reply
		case r.Method == http.MethodGet && acquireLossPreflight(w, r, a):
			if x.open > 0 {
				x.forbidden++
				t.Error("post-session preflight attempted")
			}
			return true
		default:
			t.Errorf("forbidden synthetic fixture route method=%s path=%s", r.Method, r.URL.Path)
			x.forbidden++
			w.WriteHeader(http.StatusForbidden)
		}
		return true
	}
	return f
}

func (f *drainCommandFixture) freezeHandlers() {
	f.entryMu.Lock()
	f.closing = true
	f.entryMu.Unlock()
	f.handlers.Wait()
}

// Each gate is received by the parent, after the child fsync and before the
// parent server sends response bytes. Never open a second journal writer here.
func (f *drainCommandFixture) durableCheckpoint(stage string) {
	t := f.t
	events := f.base.readEvents()
	ends := map[string]int{"poll-old": 16, "ack": 18, "acquire": 20, "poll-zero": 22, "close": 24}
	end := ends[stage]
	if len(events) != end {
		t.Errorf("durable %s checkpoint length=%d want=%d", stage, len(events), end)
		return
	}
	sequence := []struct{ kind, op string }{{"controller-handoff", ""}, {"phase", "drain"}, {"intent", "observe-owned"}, {"result", "observe-owned"}, {"intent", "observe-runner"}, {"result", "observe-runner"}, {"intent", "session-open"}, {"result", "session-open"}, {"intent", "observe-poll"}, {"result", "observe-poll"}, {"intent", "ack"}, {"result", "ack"}, {"intent", "acquire"}, {"result", "acquire"}, {"intent", "observe-poll"}, {"result", "observe-poll"}, {"intent", "session-close"}}
	for i, e := range events[7:] {
		want := sequence[i]
		if i == 0 {
			if e.ControllerHandoff == nil || e.ControllerHandoff.Phase != "drain" || e.ControllerHandoff.ApprovalSHA256 != handoff.Digest(f.raw) {
				t.Error("durable original drain consumption missing")
			}
			continue
		}
		if e.Kind != want.kind || e.Operation != want.op {
			t.Errorf("durable checkpoint operation/order differs at sequence%d", i+8)
		}
		if e.Kind == "phase" && (e.ID != 7 || e.Sequence != 9) {
			t.Error("durable drain phase identity differs")
		}
		if e.Kind == "result" && (e.Operation == "session-open" || e.Operation == "observe-poll" || e.Operation == "ack" || e.Operation == "acquire") && e.SessionID != f.x.sid {
			t.Error("durable session correlation differs")
		}
		if e.Sequence == 17 && (e.ID != 17 || e.Work != workDemand || !reflect.DeepEqual(e.RequestIDs, []int64{42})) {
			t.Error("durable validated old source/message/request differs")
		}
		if e.Sequence == 19 && e.ID != 17 {
			t.Error("durable ACK result differs")
		}
		if (e.Sequence == 20 || e.Sequence == 21) && !reflect.DeepEqual(e.RequestIDs, []int64{42}) {
			t.Error("durable acquire request42 differs")
		}
	}
}

func (f *drainCommandFixture) assertDrain() []Event {
	t := f.t
	x := &f.x
	x.mu.Lock()
	defer x.mu.Unlock()
	want := []string{"before-set", "before-runner"}
	polls, ack, acquire, close := 0, 0, 0, 0
	if f.variant != "prerequisite-busy" {
		polls, ack = 1, 1
		want = append(want, "open", "poll-old", "old-response-release", "source-verify", "ack", "ack-response-release")
		if f.variant != "ACK-response-lost" {
			polls, acquire = 2, 1
			want = append(want, "acquire", "poll-zero", "zero-response-release")
			if f.variant == "observed" {
				close = 1
				want = append(want, "close")
			}
		}
		want = append(want, "after-set", "after-runner")
	}
	wantOpen := 1
	if f.variant == "prerequisite-busy" {
		wantOpen = 0
	}
	if x.acquire != acquire {
		t.Errorf("physical acquire1/prefix witness missing: acquire=%d want=%d", x.acquire, acquire)
	}
	wantReads, wantBootstrap := 2, 4
	if f.variant == "prerequisite-busy" {
		wantReads, wantBootstrap = 1, 2
	}
	if x.sets != wantReads || x.runners != wantReads || x.bootstrap != wantBootstrap || x.sources != ack {
		t.Errorf("exact before/after/source/bootstrap counts differ: set%d runner%d source%d bootstrap%d", x.sets, x.runners, x.sources, x.bootstrap)
	}
	if x.open != wantOpen || x.polls != polls || x.ack != ack || x.close != close || x.forbidden != 0 || !reflect.DeepEqual(x.steps, want) {
		t.Errorf("physical drain counts/order differ: open%d poll%d ACK%d acquire%d close%d forbidden%d", x.open, x.polls, x.ack, x.acquire, x.close, x.forbidden)
	}
	if polls > 0 {
		capacity := []string{"1"}
		if polls == 2 {
			capacity = append(capacity, "0")
		}
		if !reflect.DeepEqual(x.capacities, capacity) {
			t.Errorf("physical capacities must be [1,0]: reached=%v", x.capacities)
		}
	}
	if x.disconnected != (f.variant == "ACK-response-lost") {
		t.Error("ACK disconnect causal witness differs")
	}
	j, e := OpenJournalForPairedFixtureAt(f.base.j.directory, f.base.a, f.admission)
	if e != nil {
		t.Fatal("postexit strict FileJournal reopen failed")
	}
	events := j.Events()
	s := replayWithApproval(events, &f.base.a)
	if j.Close() != nil {
		t.Fatal("postexit journal close failed")
	}
	if !reflect.DeepEqual(events, f.base.readEvents()) {
		t.Error("strict active reader differs from postexit replay")
	}
	length := map[string]int{"observed": 30, "withdrawn-empty202": 29, "ACK-response-lost": 25, "prerequisite-busy": 14}[f.variant]
	if len(events) != length {
		t.Errorf("final drain event count=%d want=%d", len(events), length)
	}
	busy := f.variant == "prerequisite-busy"
	observed := f.variant == "observed"
	if s.setID != 7 || s.deleted || !s.phaseSeen["drain"] || s.drainPhasePending != busy || s.drainPhaseCompleted == busy || s.uncertain == observed || s.workObserved == busy || s.reserved == busy || s.observedJobs[42] == busy || s.phaseSeen["inspect"] || s.phaseSeen["cleanup"] {
		t.Error("reopened drain set/phase/uncertainty/work/reservation retention differs")
	}
	if busy && (s.drainPhaseSequence != 9 || s.drainPhaseSetID != 7 || len(s.observedJobs) != 0) {
		t.Error("busy prerequisite pending phase differs")
	}
	wantSession := x.sid
	if busy || observed {
		wantSession = ""
	}
	if s.sessionID != wantSession {
		t.Error("reopened retained session differs")
	}
	var final *drainObservation
	markers := 0
	for _, e := range events[7:] {
		if e.Drain != nil {
			final = e.Drain
		}
		if e.DrainMarker != "" {
			markers++
			wantMarker := drainMarkerQuarantine
			if busy {
				wantMarker = drainMarkerPrerequisiteFailed
			}
			if e.DrainMarker != wantMarker {
				t.Error("drain marker differs")
			}
		}
	}
	if busy {
		if final != nil || markers != 1 {
			t.Error("busy prerequisite produced final observation")
		}
	} else if final == nil {
		t.Error("final drain observation absent")
	} else {
		// Keep this exact raw equality before any no-remint/receiver guard retry.
		if final.Sequence != 9 {
			t.Errorf("raw final drain Sequence must equal phase9: got%d", final.Sequence)
		}
		outcome := drainOutcomeInconclusive
		if observed {
			outcome = drainOutcomeObserved
		}
		if final.Outcome != outcome || final.ServerReceipt != drainServerReceiptUnproven || final.Boundary != drainBoundaryRequestWritten || !final.ResponseHeld || final.InitialCapacity != 1 || final.WithdrawnCapacity != 0 || !validDrainIdlePrerequisite(final.Before) || !sameDrainRunner(final.Before.Runner, final.After.Runner) || final.Before.Set != final.After.Set {
			t.Error("bounded final drain boundary/identity/outcome differs")
		}
		if final.Poll.Message != drainMessagePresent || !final.Poll.StatsKnown || final.Poll.Statistics != (drainStatistics{Available: 1, Assigned: 1, Registered: 1, Idle: 1}) {
			t.Error("old poll complete statistics differ")
		}
		if f.variant == "ACK-response-lost" {
			if final.Poll.ACK != drainResponseUnknown || final.Poll.Acquisition != drainResponseNotAttempted || final.NextPoll.Message != drainMessageUnknown || final.NextPoll.StatsKnown {
				t.Error("lost ACK uncertainty differs")
			}
		} else {
			if final.Poll.ACK != drainResponseSucceeded || final.Poll.Acquisition != drainResponseSucceeded || final.NextPoll.Message != drainMessageAbsent || final.NextPoll.StatsKnown != observed || final.NextPoll.ACK != drainResponseNotAttempted || final.NextPoll.Acquisition != drainResponseNotAttempted {
				t.Error("withdrawn poll absence/unknown-statistics differs")
			}
			if observed && (final.NextPoll.Statistics != (drainStatistics{Acquired: 1, Registered: 1, Idle: 1}) || !reflect.DeepEqual(final.Ordering, []string{"poll-old", "ack", "acquire", "poll-zero"})) {
				t.Error("observed withdrawal statistics/ordering differs")
			}
		}
		if markers != map[bool]int{true: 0, false: 1}[observed] {
			t.Error("final quarantine marker count differs")
		}
	}
	for path, original := range f.authority {
		current, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(original, current) {
			t.Error("original authority/raw approval/claim changed")
		}
	}
	return events
}

func TestExperimentalDrainCommand(t *testing.T) {
	controller, broker := acquireLossHelpers(t)
	for _, variant := range []string{"observed", "withdrawn-empty202", "ACK-response-lost", "prerequisite-busy"} {
		t.Run(variant, func(t *testing.T) {
			f := newDrainCommandFixture(t, controller, broker, variant)
			f.runOutcome(f.broker, f.brokerArgs(f.attempt), variant == "observed", f.management, []byte("synthetic-observation-credential"), f.workflowSeed, f.issuerSeed)
			f.freezeHandlers()
			f.brokerMu.Lock()
			mints := f.transport.mints
			f.brokerMu.Unlock()
			if mints != 1 {
				t.Error("actual signed broker mint1 missing")
			}
			events := f.assertDrain()
			if t.Failed() {
				return
			} // Preserve the causal physical/raw mutation witness.
			f.guardControlsForPhase(events, "drain")
			f.assertDrain()
		})
	}
}
