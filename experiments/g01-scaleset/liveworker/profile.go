package liveworker

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
)

const memoryBytes int64 = 1 << 30
const nanoCPUs int64 = 1_000_000_000
const pidLimit int64 = 256
const maxJIT = 1 << 20

var sha = regexp.MustCompile(`^[a-f0-9]{40}$`)
var nonce = regexp.MustCompile(`^[a-f0-9]{32}$`)
var id = regexp.MustCompile(`^[a-f0-9]{64}$`)
var component = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (a Approval) name() string { return "g01-" + a.OwnerNonce + "-worker-1" }

func (a Approval) Validate(now time.Time) error {
	if !a.RunnerUpdatesDisabled {
		return ErrApproval
	}
	if !sha.MatchString(a.HarnessSHA) || !sha.MatchString(a.WorkflowSHA) || !nonce.MatchString(a.OwnerNonce) || !component.MatchString(a.Controller) || !component.MatchString(a.DaemonID) || !strings.HasPrefix(a.ImageID, "sha256:") || !id.MatchString(strings.TrimPrefix(a.ImageID, "sha256:")) || a.Image != ImageReference || !filepath.IsAbs(a.Endpoint) || filepath.Clean(a.Endpoint) != a.Endpoint || len(a.Endpoint) > 103 || strings.ContainsAny(a.Endpoint, "\x00\r\n") || !a.ExpiresAt.After(now) || a.ExpiresAt.After(now.Add(24*time.Hour)) || len(a.Phases) == 0 {
		return ErrApproval
	}
	seen := map[string]bool{}
	for _, phase := range a.Phases {
		if !slices.Contains([]string{"create", "start", "inspect", "cleanup"}, phase) || seen[phase] {
			return ErrApproval
		}
		seen[phase] = true
	}
	return nil
}

func hostProfile() map[string]any {
	return map[string]any{
		"Memory": memoryBytes, "MemorySwap": memoryBytes, "NanoCpus": nanoCPUs, "PidsLimit": pidLimit,
		"CapDrop": []string{"ALL"}, "CapAdd": []string{}, "SecurityOpt": []string{"no-new-privileges:true"},
		"Privileged": false, "ReadonlyRootfs": false, "PublishAllPorts": false, "AutoRemove": false,
		"RestartPolicy": map[string]any{"Name": "no", "MaximumRetryCount": 0}, "LogConfig": map[string]any{"Type": "none", "Config": map[string]string{}},
		"NetworkMode": "bridge", "IpcMode": "private", "PidMode": "", "UTSMode": "", "CgroupnsMode": "private", "UsernsMode": "",
		"Runtime": "runc", "Isolation": "default", "ShmSize": int64(64 << 20), "OomKillDisable": false, "Init": false,
		"Binds": []string{}, "Mounts": []any{}, "VolumesFrom": []string{}, "Devices": []any{}, "DeviceRequests": []any{}, "GroupAdd": []string{}, "PortBindings": map[string]any{},
		"MaskedPaths":   []string{"/proc/asound", "/proc/acpi", "/proc/interrupts", "/proc/kcore", "/proc/keys", "/proc/latency_stats", "/proc/timer_list", "/proc/timer_stats", "/proc/sched_debug", "/proc/scsi", "/sys/firmware", "/sys/devices/virtual/powercap"},
		"ReadonlyPaths": []string{"/proc/bus", "/proc/fs", "/proc/irq", "/proc/sys", "/proc/sysrq-trigger"},
	}
}

func (a Approval) configProfile() map[string]any {
	return map[string]any{
		"Hostname": a.name(), "Domainname": "", "User": "1001:1001", "WorkingDir": "/home/runner", "Image": a.Image,
		"Entrypoint": []string{"/home/runner/bin/Runner.Listener"}, "Cmd": []string{"run", "--once"},
		"Healthcheck": map[string]any{"Test": []string{"NONE"}}, "StopSignal": "SIGTERM",
		"AttachStdin": false, "AttachStdout": false, "AttachStderr": false, "Tty": false, "OpenStdin": false, "StdinOnce": false, "NetworkDisabled": false,
		"Volumes": map[string]any{}, "ExposedPorts": map[string]any{},
	}
}

func digest(v any) string {
	data, _ := json.Marshal(v)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func environmentDigest(env []string) (string, error) {
	if len(env) > 64 {
		return "", ErrApproval
	}
	seen := map[string]bool{}
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !envName.MatchString(name) || seen[name] || strings.ContainsRune(value, 0) {
			return "", ErrApproval
		}
		seen[name] = true
	}
	copy := slices.Clone(env)
	slices.Sort(copy)
	return digest(copy), nil
}

func (a Approval) creation(image ImageProfile, jit string) (map[string]any, Event, error) {
	if len(jit) < 16 || len(jit) > maxJIT {
		return nil, Event{}, ErrApproval
	}
	decoded, err := base64.StdEncoding.DecodeString(jit)
	if err != nil {
		return nil, Event{}, ErrApproval
	}
	clear(decoded)
	for _, entry := range image.Env {
		if strings.HasPrefix(entry, "ACTIONS_RUNNER_INPUT_JITCONFIG=") {
			return nil, Event{}, ErrApproval
		}
	}
	workerEnv := "ACTIONS_RUNNER_INPUT_JITCONFIG=" + jit
	envDigest, err := environmentDigest(append(slices.Clone(image.Env), workerEnv))
	if err != nil {
		return nil, Event{}, err
	}
	labels := map[string]string{}
	for key, value := range image.Labels {
		if strings.HasPrefix(key, "io.gh-runnerd.g01.") {
			return nil, Event{}, ErrApproval
		}
		labels[key] = value
	}
	ownedLabels := map[string]string{"io.gh-runnerd.g01.owner": a.OwnerNonce, "io.gh-runnerd.g01.harness": a.HarnessSHA, "io.gh-runnerd.g01.workflow": a.WorkflowSHA}
	for key, value := range ownedLabels {
		labels[key] = value
	}
	payload := a.configProfile()
	payload["Env"] = []string{workerEnv}
	payload["Labels"] = ownedLabels
	payload["HostConfig"] = hostProfile()
	return payload, Event{EnvDigest: envDigest, LabelsDigest: digest(labels)}, nil
}

// Docker fills many omitted fields with zero/null defaults. Nonzero unknown
// fields fail closed, preventing an unreviewed mount/namespace/runtime extension
// from silently passing the security profile. Zero-valued defaults are inert.
func zero(v any) bool {
	switch value := v.(type) {
	case nil:
		return true
	case bool:
		return !value
	case string:
		return value == ""
	case float64:
		return value == 0
	case []any:
		for _, item := range value {
			if !zero(item) {
				return false
			}
		}
		return true
	case map[string]any:
		for _, item := range value {
			if !zero(item) {
				return false
			}
		}
		return true
	}
	return false
}
func normalized(v any) any {
	data, _ := json.Marshal(v)
	var decoded any
	_ = json.Unmarshal(data, &decoded)
	return decoded
}
func matches(actual, expected map[string]any) bool {
	expected = normalized(expected).(map[string]any)
	for key, want := range expected {
		got := actual[key]
		if nested, ok := want.(map[string]any); ok {
			observed, ok := got.(map[string]any)
			if !ok {
				if zero(got) && zero(want) {
					continue
				}
				return false
			}
			if !matches(observed, nested) {
				return false
			}
			continue
		}
		if !reflect.DeepEqual(got, want) && !(zero(got) && zero(want)) {
			return false
		}
	}
	for key, got := range actual {
		if _, ok := expected[key]; !ok && !zero(got) {
			return false
		}
	}
	return true
}

func (a Approval) verify(c *Container, s state) error {
	if c == nil || !id.MatchString(c.ID) || c.ID != s.id || c.Name != "/"+a.name() || c.ImageID != a.ImageID || len(c.Mounts) != 0 || c.Path != "/home/runner/bin/Runner.Listener" || !slices.Equal(c.Args, []string{"run", "--once"}) {
		return ErrUncertain
	}
	config := map[string]any{}
	for key, value := range c.Config {
		config[key] = value
	}
	var env []string
	data, _ := json.Marshal(config["Env"])
	if json.Unmarshal(data, &env) != nil {
		return ErrUncertain
	}
	gotEnv, err := environmentDigest(env)
	if err != nil || gotEnv != s.envDigest {
		return ErrUncertain
	}
	if digest(config["Labels"]) != s.labelsDigest {
		return ErrUncertain
	}
	delete(config, "Env")
	delete(config, "Labels")
	if !matches(config, a.configProfile()) || !matches(c.HostConfig, hostProfile()) {
		return ErrUncertain
	}
	if len(c.NetworkSettings.Networks) > 1 {
		return ErrUncertain
	}
	for name := range c.NetworkSettings.Networks {
		if name != "bridge" {
			return ErrUncertain
		}
	}
	return nil
}
