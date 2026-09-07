package liveworker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const apiVersion = "1.45"
const responseLimit = 2 << 20 // Image/container inspect may include the <=1 MiB JIT environment.
type Docker struct {
	approval Approval
	client   *http.Client
}

func socketAllowed(info os.FileInfo) bool {
	if info == nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm()&0022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}

type socketDialer struct {
	endpoint string
	mu       sync.Mutex
	pinned   os.FileInfo
	connect  func(context.Context, string, string) (net.Conn, error)
}

func newSocketDialer(endpoint string) *socketDialer {
	return &socketDialer{endpoint: endpoint, connect: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}
}

// Pin the socket inode used by preflight for this client lifetime. Check before
// dialing and again after connect, before net/http can transmit any request.
// A later command may establish a new pin only by repeating daemon preflight.
func (d *socketDialer) dial(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" || address != "docker.invalid:80" {
		return nil, ErrApproval
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	info, err := os.Lstat(d.endpoint)
	if err != nil || !socketAllowed(info) || (d.pinned != nil && !os.SameFile(d.pinned, info)) || ctx.Err() != nil {
		return nil, ErrApproval
	}
	if d.pinned == nil {
		d.pinned = info
	}
	connection, err := d.connect(ctx, "unix", d.endpoint)
	if err != nil {
		return nil, ErrApproval
	}
	observed, err := os.Lstat(d.endpoint)
	if err != nil || !socketAllowed(observed) || !os.SameFile(d.pinned, observed) || ctx.Err() != nil {
		connection.Close()
		return nil, ErrApproval
	}
	return connection, nil
}

// NewDocker does not connect or read environment-based Docker configuration.
// Each request goes directly to the one approved local Unix socket; Docker CLI,
// contexts, credential helpers, image pulls and host/TCP endpoints are absent.
func NewDocker(a Approval) (*Docker, error) {
	if a.Validate(time.Now()) != nil {
		return nil, ErrApproval
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxResponseHeaderBytes: 64 << 10}
	transport.DialContext = newSocketDialer(a.Endpoint).dial
	return &Docker{a, &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (d *Docker) request(ctx context.Context, method, path string, payload any, want int, result any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil || len(body) > 2*maxJIT {
			return ErrApproval
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker.invalid"+path, bytes.NewReader(body))
	if err != nil {
		return ErrRemote
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(req)
	if err != nil {
		return ErrRemote
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil || len(data) > responseLimit || response.StatusCode != want {
		return ErrRemote
	}
	if result != nil && json.Unmarshal(data, result) != nil {
		return ErrRemote
	}
	return nil
}

func versionNumber(version string) (int, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return 0, ErrApproval
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major != 1 {
		return 0, ErrApproval
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 || minor > 999 {
		return 0, ErrApproval
	}
	return minor, nil
}

func (d *Docker) Preflight(ctx context.Context, a Approval) (ImageProfile, error) {
	if digest(a) != digest(d.approval) {
		return ImageProfile{}, ErrApproval
	}
	var version struct {
		APIVersion    string `json:"ApiVersion"`
		MinAPIVersion string `json:"MinAPIVersion"`
	}
	if d.request(ctx, http.MethodGet, "/version", nil, http.StatusOK, &version) != nil {
		return ImageProfile{}, ErrApproval
	}
	maximum, err := versionNumber(version.APIVersion)
	if err != nil {
		return ImageProfile{}, ErrApproval
	}
	minimum, err := versionNumber(version.MinAPIVersion)
	if err != nil || minimum > 45 || maximum < 45 {
		return ImageProfile{}, ErrApproval
	}
	var info struct {
		ID                                             string
		OSType, Architecture                           string
		NCPU                                           int
		MemTotal                                       int64
		MemoryLimit, SwapLimit, CpuCfsQuota, PidsLimit bool
		Warnings                                       []string
	}
	if d.request(ctx, http.MethodGet, "/v"+apiVersion+"/info", nil, http.StatusOK, &info) != nil {
		return ImageProfile{}, ErrApproval
	}
	if info.ID != a.DaemonID || info.OSType != "linux" || !slices.Contains([]string{"arm64", "aarch64"}, info.Architecture) || info.NCPU < 2 || info.MemTotal < 2*memoryBytes || !info.MemoryLimit || !info.SwapLimit || !info.CpuCfsQuota || !info.PidsLimit || len(info.Warnings) != 0 {
		return ImageProfile{}, ErrApproval
	}
	var image struct {
		ID           string `json:"Id"`
		OS           string `json:"Os"`
		Architecture string
		RepoDigests  []string
		Config       struct {
			Env                   []string
			Labels                map[string]string
			Volumes, ExposedPorts map[string]any
		}
	}
	if d.request(ctx, http.MethodGet, "/v"+apiVersion+"/images/"+url.PathEscape(a.Image)+"/json", nil, http.StatusOK, &image) != nil {
		return ImageProfile{}, ErrApproval
	}
	if image.ID != a.ImageID || image.OS != "linux" || image.Architecture != "arm64" || !slices.Contains(image.RepoDigests, a.Image) || len(image.Config.Volumes) != 0 || len(image.Config.ExposedPorts) != 0 {
		return ImageProfile{}, ErrApproval
	}
	if _, err := environmentDigest(image.Config.Env); err != nil {
		return ImageProfile{}, err
	}
	envBytes, labelBytes := 0, 0
	for _, entry := range image.Config.Env {
		envBytes += len(entry)
	}
	for key, value := range image.Config.Labels {
		labelBytes += len(key) + len(value)
	}
	if envBytes > 64<<10 || labelBytes > 64<<10 || len(image.Config.Labels) > 64 {
		return ImageProfile{}, ErrApproval
	}
	return ImageProfile{Env: image.Config.Env, Labels: image.Config.Labels}, nil
}

func (d *Docker) Create(ctx context.Context, name string, payload map[string]any) (string, bool, error) {
	if name != d.approval.name() {
		return "", false, ErrApproval
	}
	var response struct {
		ID       string `json:"Id"`
		Warnings []string
	}
	err := d.request(ctx, http.MethodPost, "/v"+apiVersion+"/containers/create?name="+url.QueryEscape(name), payload, http.StatusCreated, &response)
	return response.ID, len(response.Warnings) > 0, err
}
func (d *Docker) Inspect(ctx context.Context, containerID string) (*Container, error) {
	if !id.MatchString(containerID) {
		return nil, ErrApproval
	}
	var result Container
	err := d.request(ctx, http.MethodGet, "/v"+apiVersion+"/containers/"+containerID+"/json", nil, http.StatusOK, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
func (d *Docker) Start(ctx context.Context, containerID string) error {
	if !id.MatchString(containerID) {
		return ErrApproval
	}
	return d.request(ctx, http.MethodPost, "/v"+apiVersion+"/containers/"+containerID+"/start", nil, http.StatusNoContent, nil)
}
func (d *Docker) Delete(ctx context.Context, containerID string) error {
	if !id.MatchString(containerID) {
		return ErrApproval
	}
	return d.request(ctx, http.MethodDelete, "/v"+apiVersion+"/containers/"+containerID+"?force=false&v=false", nil, http.StatusNoContent, nil)
}
