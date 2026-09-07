package enrollment

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestBrokerAdmissionProcessChild(t *testing.T) {
	parent := os.Getenv("G01_SYNTHETIC_BROKER_PARENT")
	if parent == "" {
		return
	}
	a := brokerApprovalFixture()
	root := filepath.Join(parent, "child-attempt")
	f := &brokerHTTPFixture{t: t, root: root, token: "synthetic-private-installation-token"}
	api := newBrokerAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/app/installations/201/access_tokens" {
			if os.WriteFile(filepath.Join(parent, "issuance-entered"), []byte("intent held\n"), 0600) != nil {
				t.Fatal("signal")
			}
			select {}
		}
		return f.RoundTrip(r)
	}))
	api.admissionDirectory = func() (string, error) { return filepath.Join(parent, "admission"), nil }
	c := syntheticCandidate(t)
	brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil)
	t.Fatal("child unexpectedly returned")
}
func TestBrokerAdmissionSeparateProcessCrashKeepsSlot(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	parent, e := filepath.EvalSymlinks(filepath.Dir(root))
	if e != nil {
		t.Fatal("fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBrokerAdmissionProcessChild$")
	cmd.Env = []string{"G01_SYNTHETIC_BROKER_PARENT=" + parent, "GORACE=atexit_sleep_ms=0"}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if cmd.Start() != nil {
		t.Fatal("synthetic child")
	}
	waited := false
	defer func() {
		if !waited {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	for {
		if _, e := os.Stat(filepath.Join(parent, "issuance-entered")); e == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("child did not reach synthetic issuance")
		}
		time.Sleep(5 * time.Millisecond)
	}
	ledger := filepath.Join(f.admissionRoot, "broker-admission.jsonl")
	before, e := os.ReadFile(ledger)
	if e != nil {
		t.Fatal("durable claim")
	}
	// A second process cannot claim while the lifetime lock is held.
	if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil); e == nil || len(f.calls) != 0 {
		t.Fatal("concurrent attempt reached API")
	}
	cmd.Process.Kill()
	cmd.Wait()
	waited = true
	root = filepath.Join(parent, "after-crash")
	f.root = root
	if _, e := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil); e == nil || len(f.calls) != 0 {
		t.Fatal("crashed attempt retried issuance")
	}
	after, e := os.ReadFile(ledger)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("crash/close changed permanent claim")
	}
}

func TestBrokerConcurrentFirstClaimAdmitsOne(t *testing.T) {
	for iteration := 0; iteration < 50; iteration++ {
		parent, e := filepath.EvalSymlinks(t.TempDir())
		if e != nil {
			t.Fatal("fixture")
		}
		os.Chmod(parent, 0700)
		directory := filepath.Join(parent, "admission")
		if os.Mkdir(directory, 0700) != nil {
			t.Fatal("fixture root")
		}
		a := brokerApprovalFixture()
		journals := make([]*brokerJournal, 2)
		claims := make([]*brokerAdmission, 2)
		for i := range journals {
			journals[i], e = openBrokerJournal(filepath.Join(parent, fmt.Sprintf("attempt-%d", i)), a)
			if e != nil {
				t.Fatal("fixture journal")
			}
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range journals {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				claims[i], _ = openBrokerAdmission(directory, a, journals[i], nil, syncDirectory)
			}(i)
		}
		close(start)
		wg.Wait()
		admitted := 0
		for i := range journals {
			if claims[i] != nil {
				admitted++
				claims[i].close()
			}
			journals[i].close()
		}
		if admitted != 1 {
			t.Fatalf("first-claim contenders admitted %d; want exactly one (iteration %d)", admitted, iteration)
		}
	}
}
