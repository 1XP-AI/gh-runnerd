package liveworker

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type dockerFixture struct {
	driver   *Driver
	runtime  *fakeRuntime
	fault    string
	requests atomic.Int64
	approval Approval
	socket   string
}

type hookJournal struct {
	Journal
	operation   string
	afterIntent func()
}

func (j hookJournal) Append(e Event) error {
	if err := j.Journal.Append(e); err != nil {
		return err
	}
	if e.Kind == "intent" && e.Operation == j.operation {
		j.afterIntent()
	}
	return nil
}

type replacementSocket struct {
	calls       atomic.Int64
	receivedJIT atomic.Bool
}

func replaceSocket(t *testing.T, path string) *replacementSocket {
	t.Helper()
	if os.Rename(path, path+".original") != nil {
		t.Fatal("rename synthetic socket failed")
	}
	listener, err := net.Listen("unix", path)
	if err != nil || os.Chmod(path, 0600) != nil {
		t.Fatal("replacement synthetic socket failed")
	}
	replacement := &replacementSocket{}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replacement.calls.Add(1)
		data, _ := io.ReadAll(io.LimitReader(r.Body, 2*maxJIT))
		if strings.Contains(string(data), syntheticJIT) {
			replacement.receivedJIT.Store(true)
		}
		if strings.HasSuffix(r.URL.Path, "/create") {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": strings.Repeat("c", 64), "Warnings": []string{}})
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}), ErrorLog: log.New(io.Discard, "", 0)}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close(); listener.Close() })
	return replacement
}

func TestSocketReplacementAfterPreflightCannotReceiveAnyMutation(t *testing.T) {
	for _, phase := range []string{"create", "start", "cleanup"} {
		t.Run(phase, func(t *testing.T) {
			f := unixFixture(t)
			if phase != "create" && f.driver.Run(context.Background(), "create", syntheticJIT) != nil {
				t.Fatal("synthetic setup failed")
			}
			operation := phase
			if phase == "cleanup" {
				operation = "delete"
			}
			var replacement *replacementSocket
			f.driver.Journal = hookJournal{f.driver.Journal, operation, func() { replacement = replaceSocket(t, f.socket) }}
			jit := ""
			if phase == "create" {
				jit = syntheticJIT
			}
			err := f.driver.Run(context.Background(), phase, jit)
			if replacement == nil {
				t.Fatal("replacement boundary not reached")
			}
			if err == nil || replacement.calls.Load() != 0 || replacement.receivedJIT.Load() || !replay(f.driver.Journal.Events()).uncertain {
				t.Fatalf("socket replacement crossed authority boundary: error=%v calls=%d received_jit=%t", err, replacement.calls.Load(), replacement.receivedJIT.Load())
			}
			requests := f.requests.Load()
			_ = f.driver.Run(context.Background(), phase, jit)
			if f.requests.Load() != requests || replacement.calls.Load() != 0 {
				t.Fatal("replacement mutation retried")
			}
		})
	}
}

type socketMetadata struct {
	os.FileInfo
	uid uint32
}

func (s socketMetadata) Sys() any {
	copy := *s.FileInfo.Sys().(*syscall.Stat_t)
	copy.Uid = s.uid
	return &copy
}

func TestSocketModesAndControllerOwnership(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0755, 0757, 0775} {
		t.Run(mode.String(), func(t *testing.T) {
			f := unixFixture(t)
			if os.Chmod(f.socket, mode) != nil {
				t.Fatal("synthetic chmod failed")
			}
			err := f.driver.Run(context.Background(), "create", syntheticJIT)
			allowed := mode == 0600 || mode == 0755
			if (err == nil) != allowed || (!allowed && f.requests.Load() != 0) {
				t.Fatal("socket mode policy mismatch")
			}
			info, err := os.Lstat(f.socket)
			if err != nil {
				t.Fatal("synthetic stat failed")
			}
			if socketAllowed(socketMetadata{info, uint32(os.Geteuid() + 1)}) {
				t.Fatal("foreign socket owner accepted")
			}
		})
	}
}

// The actual production Unix HTTP client connects only to this private synthetic
// listener. These tests never contact a real Docker endpoint or pull/run an image.
func unixFixture(t *testing.T) *dockerFixture {
	t.Helper()
	dir, err := os.MkdirTemp("", "g01w-")
	if err != nil {
		t.Fatal("fixture directory failed")
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "api.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal("fixture socket failed")
	}
	if os.Chmod(socket, 0600) != nil {
		t.Fatal("fixture socket mode failed")
	}
	f := &dockerFixture{runtime: &fakeRuntime{image: ImageProfile{Env: []string{"PATH=/usr/bin"}}}, approval: approval(), socket: socket}
	f.approval.Endpoint = socket
	f.driver = &Driver{f.approval, &memoryJournal{}, nil}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		var body any
		status := http.StatusOK
		switch {
		case r.URL.Path == "/version":
			if f.fault == "redirect" {
				w.Header().Set("Location", "http://unapproved.example/secret")
				w.WriteHeader(http.StatusTemporaryRedirect)
				return
			}
			maximum := "1.51"
			if f.fault == "api" {
				maximum = "1.44"
			}
			body = map[string]any{"ApiVersion": maximum, "MinAPIVersion": "1.24"}
		case r.URL.Path == "/v1.45/info":
			daemon := f.approval.DaemonID
			if f.fault == "daemon" {
				daemon = "changed-daemon"
			}
			warnings := []string{}
			if f.fault == "warning" {
				warnings = []string{"synthetic-private-warning"}
			}
			body = map[string]any{"ID": daemon, "OSType": "linux", "Architecture": "aarch64", "NCPU": 4, "MemTotal": 4 * memoryBytes, "MemoryLimit": f.fault != "memory", "SwapLimit": f.fault != "swap", "CpuCfsQuota": f.fault != "cpu", "PidsLimit": f.fault != "pids", "Warnings": warnings}
		case strings.HasPrefix(r.URL.Path, "/v1.45/images/"):
			imageID := f.approval.ImageID
			arch := "arm64"
			if f.fault == "image" {
				imageID = "sha256:" + strings.Repeat("9", 64)
			}
			if f.fault == "architecture" {
				arch = "amd64"
			}
			volumes := map[string]any{}
			ports := map[string]any{}
			if f.fault == "image-volume" {
				volumes["/data"] = map[string]any{}
			}
			if f.fault == "image-port" {
				ports["8080/tcp"] = map[string]any{}
			}
			body = map[string]any{"Id": imageID, "Os": "linux", "Architecture": arch, "RepoDigests": []string{f.approval.Image}, "Config": map[string]any{"Env": f.runtime.image.Env, "Labels": f.runtime.image.Labels, "Volumes": volumes, "ExposedPorts": ports}}
			if f.fault == "absent-image" {
				status = http.StatusNotFound
			}
		case r.URL.Path == "/v1.45/containers/create" && r.Method == http.MethodPost:
			events := f.driver.Journal.Events()
			if len(events) == 0 || events[len(events)-1].Kind != "intent" || events[len(events)-1].Operation != "create" {
				t.Error("create reached runtime before durable intent")
			}
			var payload map[string]any
			if json.NewDecoder(io.LimitReader(r.Body, 2*maxJIT+1)).Decode(&payload) != nil {
				t.Error("invalid create request")
			}
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			containerID, _, _ := f.runtime.Create(ctx, r.URL.Query().Get("name"), payload)
			if f.fault == "lost-create" {
				connection, _, _ := w.(http.Hijacker).Hijack()
				connection.Close()
				return
			}
			status = http.StatusCreated
			body = map[string]any{"Id": containerID, "Warnings": []string{}}
			if f.fault == "oversize-create" {
				body = map[string]any{"Id": containerID, "extra": strings.Repeat("synthetic-private-response", responseLimit/8)}
			}
		case strings.HasSuffix(r.URL.Path, "/json") && r.Method == http.MethodGet:
			body = f.runtime.container
			if f.fault == "absent-container" {
				status = http.StatusNotFound
			}
		case strings.HasSuffix(r.URL.Path, "/start") && r.Method == http.MethodPost:
			events := f.driver.Journal.Events()
			if events[len(events)-1].Operation != "start" || events[len(events)-1].Kind != "intent" {
				t.Error("start reached runtime before durable intent")
			}
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			_ = f.runtime.Start(ctx, f.runtime.container.ID)
			if f.fault == "lost-start" {
				connection, _, _ := w.(http.Hijacker).Hijack()
				connection.Close()
				return
			}
			status = http.StatusNoContent
		case strings.HasPrefix(r.URL.Path, "/v1.45/containers/") && r.Method == http.MethodDelete:
			if r.URL.Query().Get("force") != "false" || r.URL.Query().Get("v") != "false" {
				t.Error("unsafe delete flags")
			}
			f.runtime.deletes.Add(1)
			if f.fault == "delete-race" {
				f.runtime.container.State.Running = true
				status = http.StatusConflict
				body = map[string]string{"message": "synthetic-private-busy-error"}
			} else {
				status = http.StatusNoContent
			}
		default:
			t.Error("unapproved runtime operation")
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		if f.fault == "oversize" {
			_, _ = io.CopyN(w, strings.NewReader(strings.Repeat("s", responseLimit+1)), responseLimit+1)
			return
		}
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}), ErrorLog: log.New(io.Discard, "", 0)}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close(); listener.Close() })
	client, err := NewDocker(f.approval)
	if err != nil {
		t.Fatal(err)
	}
	f.driver.Runtime = client
	return f
}

func TestUnixRuntimeRejectsWrongIdentityImagesAndUnsupportedLimits(t *testing.T) {
	for _, fault := range []string{"api", "daemon", "image", "architecture", "image-volume", "image-port", "absent-image", "memory", "swap", "cpu", "pids", "warning"} {
		t.Run(fault, func(t *testing.T) {
			f := unixFixture(t)
			f.fault = fault
			if f.driver.Run(context.Background(), "create", syntheticJIT) == nil || f.runtime.creates.Load() != 0 {
				t.Fatal("preflight failed to reject incompatible runtime")
			}
		})
	}
}

func TestUnixRuntimeOneShotCreateStartAndNonForceCleanup(t *testing.T) {
	f := unixFixture(t)
	if f.driver.Run(context.Background(), "create", syntheticJIT) != nil || f.driver.Run(context.Background(), "start", "") != nil {
		t.Fatal("synthetic baseline failed")
	}
	if f.runtime.creates.Load() != 1 || f.runtime.starts.Load() != 1 {
		t.Fatal("duplicate mutation")
	}
	if f.driver.Run(context.Background(), "cleanup", "") == nil || f.runtime.deletes.Load() != 0 {
		t.Fatal("running worker removed")
	}
	f.runtime.container.State.Running = false
	f.runtime.container.State.Status = "exited"
	if err := f.driver.Run(context.Background(), "cleanup", ""); err != nil || f.runtime.deletes.Load() != 1 {
		t.Fatal("owned exited worker not removed")
	}
}

func TestUnixRuntimeAmbiguousEffectsNeverRetry(t *testing.T) {
	for _, fault := range []string{"lost-create", "lost-start", "delete-race"} {
		t.Run(fault, func(t *testing.T) {
			f := unixFixture(t)
			phase := "create"
			jit := syntheticJIT
			if fault != "lost-create" {
				if f.driver.Run(context.Background(), "create", jit) != nil {
					t.Fatal("fixture create failed")
				}
				phase = "start"
				jit = ""
				if fault == "delete-race" {
					phase = "cleanup"
					f.runtime.container.State.Status = "exited"
				}
			}
			f.fault = fault
			err := f.driver.Run(context.Background(), phase, jit)
			if err == nil || strings.Contains(err.Error(), "synthetic-private") {
				t.Fatal("unsafe external error")
			}
			requests := f.requests.Load()
			_ = f.driver.Run(context.Background(), phase, jit)
			if f.requests.Load() != requests {
				t.Fatal("ambiguous effect retried")
			}
		})
	}
}

func TestChangedDaemonOrAbsentContainerNeverMeansCleanupComplete(t *testing.T) {
	for _, fault := range []string{"daemon", "absent-container"} {
		t.Run(fault, func(t *testing.T) {
			f := unixFixture(t)
			if f.driver.Run(context.Background(), "create", syntheticJIT) != nil {
				t.Fatal("fixture failed")
			}
			f.fault = fault
			if f.driver.Run(context.Background(), "cleanup", "") == nil || f.runtime.deletes.Load() != 0 || replay(f.driver.Journal.Events()).deleted {
				t.Fatal("identity change/absence released worker")
			}
		})
	}
}

func TestUnixTransportRejectsSymlinksAndInheritedTCPDestinations(t *testing.T) {
	f := unixFixture(t)
	client := f.driver.Runtime.(*Docker).client
	transport := client.Transport.(*http.Transport)
	if conn, err := transport.DialContext(context.Background(), "tcp", "evil.example:80"); err == nil {
		conn.Close()
		t.Fatal("TCP destination accepted")
	}
	if transport.Proxy != nil || !transport.DisableKeepAlives {
		t.Fatal("unexpected proxy or replayable pooled connection")
	}
	if os.Rename(f.socket, f.socket+".owned") != nil || os.Symlink(f.socket+".owned", f.socket) != nil {
		t.Fatal("socket fixture failed")
	}
	if f.driver.Run(context.Background(), "create", syntheticJIT) == nil || f.requests.Load() != 0 {
		t.Fatal("symlink endpoint accepted")
	}
}

func TestUnixResponsesAreBoundedAndRedirectsNeverFollowed(t *testing.T) {
	for _, fault := range []string{"oversize", "redirect", "oversize-create"} {
		t.Run(fault, func(t *testing.T) {
			f := unixFixture(t)
			f.fault = fault
			err := f.driver.Run(context.Background(), "create", syntheticJIT)
			if err == nil || strings.Contains(err.Error(), "synthetic-private") {
				t.Fatal("unsafe response accepted or exposed")
			}
			if fault == "oversize-create" {
				if f.runtime.creates.Load() != 1 || !replay(f.driver.Journal.Events()).uncertain {
					t.Fatal("oversized creation result was not retained as ambiguous")
				}
				_ = f.driver.Run(context.Background(), "create", syntheticJIT)
				if f.runtime.creates.Load() != 1 {
					t.Fatal("oversize effect repeated")
				}
			} else if f.runtime.creates.Load() != 0 || f.requests.Load() != 1 {
				t.Fatal("unexpected request after rejected response")
			}
		})
	}
}
