package liveworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func created(t *testing.T) (*Driver, *fakeRuntime, *memoryJournal) {
	t.Helper()
	j := &memoryJournal{}
	f := &fakeRuntime{image: ImageProfile{Env: []string{"PATH=/usr/bin"}}}
	d := &Driver{approval(), j, f}
	if err := d.Run(context.Background(), "create", syntheticJIT); err != nil {
		t.Fatal(err)
	}
	return d, f, j
}

func TestOneWorkerNeverRecreatedAndOnlyJITAddedToEnvironment(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "synthetic-controller-secret")
	d, f, j := created(t)
	if d.Run(context.Background(), "create", syntheticJIT) == nil || f.creates.Load() != 1 {
		t.Fatal("second worker admitted")
	}
	env := f.container.Config["Env"].([]any)
	if len(env) != 2 || env[0] != "PATH=/usr/bin" || env[1] != "ACTIONS_RUNNER_INPUT_JITCONFIG="+syntheticJIT {
		t.Fatal("worker inherited unintended controller environment")
	}
	data, _ := json.Marshal(j.Events())
	if strings.Contains(string(data), syntheticJIT) || strings.Contains(string(data), "synthetic-controller-secret") {
		t.Fatal("secret persisted")
	}
}

func TestUncertainStartNeverRetriesAndCannotCleanup(t *testing.T) {
	for _, failure := range []string{"response", "journal"} {
		t.Run(failure, func(t *testing.T) {
			d, f, j := created(t)
			if failure == "response" {
				f.startErr = errors.New("synthetic-secret-response")
			} else {
				f.afterStart = func() { j.fail = true }
			}
			err := d.Run(context.Background(), "start", "")
			if err == nil || f.starts.Load() != 1 || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("start ambiguity not sanitized")
			}
			j.fail = false
			restarted := Driver{d.Approval, j, f}
			if restarted.Run(context.Background(), "start", "") == nil || restarted.Run(context.Background(), "cleanup", "") == nil || f.starts.Load() != 1 || f.deletes.Load() != 0 {
				t.Fatal("uncertain start allowed retry or cleanup")
			}
		})
	}
}

func TestEveryRuntimeBoundaryRejectsProfileAndOwnershipMismatch(t *testing.T) {
	mutations := map[string]func(*Container){
		"owner": func(c *Container) { c.Name = "/manual-worker" }, "image": func(c *Container) { c.ImageID = "sha256:" + strings.Repeat("9", 64) },
		"entrypoint": func(c *Container) { c.Path = "/bin/sh" }, "job-count": func(c *Container) { c.Args = []string{"run"} },
		"user": func(c *Container) { c.Config["User"] = "0" }, "env": func(c *Container) { c.Config["Env"] = append(c.Config["Env"].([]any), "GITHUB_TOKEN=synthetic-secret") },
		"health": func(c *Container) { c.Config["Healthcheck"] = map[string]any{"Test": []any{"CMD-SHELL", "echo bad"}} },
		"mount":  func(c *Container) { c.Mounts = []any{map[string]any{"Source": "/controller"}} },
		"bind":   func(c *Container) { c.HostConfig["Binds"] = []any{"/docker.sock:/docker.sock"} },
		"pid":    func(c *Container) { c.HostConfig["PidMode"] = "host" }, "ipc": func(c *Container) { c.HostConfig["IpcMode"] = "host" },
		"network": func(c *Container) { c.HostConfig["NetworkMode"] = "host" }, "ports": func(c *Container) { c.HostConfig["PublishAllPorts"] = true },
		"privilege": func(c *Container) { c.HostConfig["Privileged"] = true }, "capability": func(c *Container) { c.HostConfig["CapAdd"] = []any{"SYS_ADMIN"} },
		"groups": func(c *Container) { c.HostConfig["GroupAdd"] = []any{"docker"} }, "security": func(c *Container) { c.HostConfig["SecurityOpt"] = []any{} },
		"memory": func(c *Container) { c.HostConfig["Memory"] = float64(memoryBytes * 2) }, "swap": func(c *Container) { c.HostConfig["MemorySwap"] = float64(memoryBytes * 2) },
		"cpu": func(c *Container) { c.HostConfig["NanoCpus"] = float64(nanoCPUs * 2) }, "pids": func(c *Container) { c.HostConfig["PidsLimit"] = float64(-1) },
		"logs": func(c *Container) { c.HostConfig["LogConfig"] = map[string]any{"Type": "json-file"} }, "restart": func(c *Container) { c.HostConfig["RestartPolicy"] = map[string]any{"Name": "always"} },
		"autoremove": func(c *Container) { c.HostConfig["AutoRemove"] = true }, "unknown": func(c *Container) { c.HostConfig["FutureMountOption"] = "/host" },
	}
	for name, mutation := range mutations {
		for _, phase := range []string{"start", "cleanup"} {
			t.Run(name+"/"+phase, func(t *testing.T) {
				d, f, _ := created(t)
				mutation(f.container)
				if d.Run(context.Background(), phase, "") == nil || f.starts.Load() != 0 || f.deletes.Load() != 0 {
					t.Fatal("unverified profile changed external state")
				}
			})
		}
	}
}

func TestCleanupRetainsActiveAndUnknownWorkers(t *testing.T) {
	for _, status := range []string{"running", "paused", "restarting", "dead", "removing", "unknown"} {
		t.Run(status, func(t *testing.T) {
			d, f, _ := created(t)
			f.container.State.Status = status
			if d.Run(context.Background(), "cleanup", "") == nil || f.deletes.Load() != 0 {
				t.Fatal("active/unknown worker removed")
			}
		})
	}
}

func TestOwnedTerminalCleanupAndRunningRemovalRace(t *testing.T) {
	for _, status := range []string{"created", "exited", "race"} {
		t.Run(status, func(t *testing.T) {
			d, f, j := created(t)
			f.container.State.Status = "exited"
			if status == "created" {
				f.container.State.Status = "created"
			}
			if status == "race" {
				f.deleteErr = errors.New("synthetic busy refusal")
			}
			err := d.Run(context.Background(), "cleanup", "")
			if f.deletes.Load() != 1 {
				t.Fatal("verified terminal removal not attempted once")
			}
			if status == "race" {
				if !errors.Is(err, ErrUncertain) || !replay(j.Events()).uncertain {
					t.Fatal("non-force busy refusal did not quarantine")
				}
			} else if err != nil || !replay(j.Events()).deleted {
				t.Fatal("terminal removal result not durable")
			}
			if d.Run(context.Background(), "create", syntheticJIT) == nil || f.creates.Load() != 1 {
				t.Fatal("cleanup released the one-worker experiment reservation")
			}
		})
	}
}

func TestUnverifiedRunnerUpdatePolicyRefusesBeforeRuntime(t *testing.T) {
	d, f, _ := created(t)
	d.Approval.RunnerUpdatesDisabled = false
	if d.Run(context.Background(), "start", "") == nil || f.starts.Load() != 0 {
		t.Fatal("image digest falsely used as proof of disabled runner updates")
	}
}
