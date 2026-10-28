package livecanary

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func TestPollableHandoffPipeForkExclusion(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	type result struct {
		file *os.File
		err  error
	}
	started, done := make(chan struct{}), make(chan result, 1)
	// Model the descriptor snapshot interval used by Go's fork/exec. The
	// receiver must not allocate an inheritable duplicate during this interval.
	syscall.ForkLock.Lock()
	go func() {
		close(started)
		file, err := pollableHandoffPipe(reader)
		done <- result{file, err}
	}()
	<-started
	var got result
	completed := false
	select {
	case got = <-done:
		completed = true
	case <-time.After(100 * time.Millisecond):
	}
	syscall.ForkLock.Unlock()
	if !completed {
		got = <-done
	}
	if got.file != nil {
		defer got.file.Close()
	}
	if completed {
		t.Fatal("pipe normalization completed during the fork descriptor snapshot")
	}
	if got.err != nil || got.file == nil {
		t.Fatalf("normalize after fork: %v", got.err)
	}
	if _, err := reader.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("original descriptor remains open: %v", err)
	}
	raw, err := got.file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	if err = raw.Control(func(fd uintptr) {
		flags, _, e := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFD, 0)
		if e != 0 || flags&syscall.FD_CLOEXEC == 0 {
			t.Errorf("duplicate lacks close-on-exec: flags=%d err=%v", flags, e)
		}
		flags, _, e = syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
		if e != 0 || flags&syscall.O_NONBLOCK == 0 {
			t.Errorf("duplicate lacks nonblocking IO: flags=%d err=%v", flags, e)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPollableHandoffPipeExecInheritance(t *testing.T) {
	type probe struct {
		FD            int
		Dev, Ino      uint64
		WantInherited bool
	}
	const probeEnv = "G01_TEST_PIPE_INHERITANCE"
	if value := os.Getenv(probeEnv); value != "" {
		var p probe
		if err := json.Unmarshal([]byte(value), &p); err != nil {
			t.Fatal(err)
		}
		var stat syscall.Stat_t
		err := syscall.Fstat(p.FD, &stat)
		inherited := err == nil && uint64(stat.Dev) == p.Dev && uint64(stat.Ino) == p.Ino
		if err != nil && err != syscall.EBADF {
			t.Fatal(err)
		}
		if inherited != p.WantInherited {
			t.Fatalf("synthetic pipe inherited=%t, want %t", inherited, p.WantInherited)
		}
		return
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	owned, err := pollableHandoffPipe(reader)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	// Fd is used only in this inheritance probe, never for cancellation reads.
	fd := int(owned.Fd())
	var stat syscall.Stat_t
	if err = syscall.Fstat(fd, &stat); err != nil {
		t.Fatal(err)
	}
	for _, explicit := range []bool{true, false} {
		p := probe{fd, uint64(stat.Dev), uint64(stat.Ino), explicit}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPollableHandoffPipeExecInheritance$")
		if explicit {
			// Positive control: the child must recognize this exact pipe when
			// intentionally passed, even if its descriptor number is reused.
			cmd.ExtraFiles = []*os.File{owned}
			p.FD = 3
		}
		value, _ := json.Marshal(p)
		cmd.Env = []string{"LANG=C", "LC_ALL=C", probeEnv + "=" + string(value)}
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("owned test child (explicit=%t): %v: %s", explicit, err, output)
		}
	}
}

func processBinding(t *testing.T, f controllerHandoffFixture, payload []byte) (handoff.CredentialBinding, handoff.IssuerRoot, ed25519.PrivateKey) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{71}, ed25519.SeedSize))
	root, e := handoff.NewIssuerRoot("fixture-issuer-v1", key.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	b := handoff.CredentialBinding{BindingClaims: handoff.BindingClaims{Request: f.request, Version: 1, Kind: handoff.BindingKind, Algorithm: "ed25519", Audience: handoff.BindingAudience, KeyID: root.KeyID(), IssuerSource: handoff.IssuerSource, WorkflowKeyID: f.receipt.KeyID, ReceiptSHA256: handoff.Digest(f.receiptBytes), PayloadSHA256: handoff.Digest(payload), PayloadLength: len(payload), ReceiptNonce: f.receipt.ReceiptNonce, IssuedAt: f.now, ExpiresAt: f.now.Add(20 * time.Second)}}
	b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes()))
	return b, root, key
}
func processPipes(t *testing.T, receipt []byte, b handoff.CredentialBinding, payload []byte) (*os.File, *os.File) {
	t.Helper()
	raw, _ := json.Marshal(b)
	frame, e := handoff.EncodeControl(receipt, raw)
	if e != nil {
		t.Fatal(e)
	}
	cr, cw, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	ir, iw, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { cr.Close(); cw.Close(); ir.Close(); iw.Close() })
	done := make(chan struct{})
	go func() { defer close(done); _, _ = cw.Write(frame); cw.Close(); _, _ = iw.Write(payload); iw.Close() }()
	t.Cleanup(func() { cr.Close(); ir.Close(); <-done })
	return cr, ir
}
func TestControllerProcessHandoffPayloadSubstitution(t *testing.T) {
	f, j := openCreateControllerHandoff(t)
	original := handoffCredentialBytes(t, f.approval)
	c := credentials(f.approval)
	c.InstallationToken = strings.Repeat("B", len(c.InstallationToken))
	substitute, _ := json.Marshal(c)
	b, root, _ := processBinding(t, f, original)
	cr, ir := processPipes(t, f.receiptBytes, b, substitute)
	fake := &fakeAPI{}
	factories := 0
	err := runControllerProcessHandoffCreate(context.Background(), j, f.snapshot, cr, ir, f.request.Source, f.root, root, time.Now, func(context.Context, Approval, Credentials) (API, error) {
		factories++
		return handoffValidInventoryAPI{fake}, nil
	})
	if err == nil || factories != 0 || fake.createCalls != 0 || len(j.Events()) != 1 || j.Events()[0].ControllerHandoff == nil {
		t.Fatalf("substitution reached factory/create: err=%v factories=%d creates=%d events=%d", err, factories, fake.createCalls, len(j.Events()))
	}
}

func processChildRoots() (handoff.TrustRoot, handoff.IssuerRoot) {
	// Public-only synthetic TEST pins; deliberately independent of broker assembly.
	workflowBytes, _ := hex.DecodeString("03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8")
	issuerBytes, _ := hex.DecodeString("e28a8970753332bd72fef413e6b0b2ef1b4aadda7aa2c141f233712a6876b351")
	w, _ := handoff.NewTrustRoot("fixture-ed25519-v1", workflowBytes)
	i, _ := handoff.NewIssuerRoot("fixture-issuer-v1", issuerBytes)
	return w, i
}
func runProcessHandoffTestChild() int {
	if len(os.Args) != 8 || os.Args[2] != "--approval" || os.Args[4] != "--state-dir" || os.Args[6] != "--phase" || os.Args[7] != "create" {
		return 2
	}
	for _, entry := range os.Environ() {
		if entry != "LANG=C" && entry != "LC_ALL=C" {
			return 3
		}
	}
	raw, e := os.ReadFile(os.Args[3])
	if e != nil {
		return 4
	}
	var a Approval
	if DecodeStrict(raw, &a) != nil {
		return 5
	}
	directory, e := filepath.EvalSymlinks(filepath.Join(filepath.Dir(os.Args[5]), "admission"))
	if e != nil {
		return 6
	}
	j, e := openJournalAtAdmission(os.Args[5], a, directory, func(f *os.File) error { return f.Sync() })
	if e != nil {
		return 7
	}
	defer j.Close()
	w, i := processChildRoots()
	syncs := 0
	j.syncFile = func(file *os.File) error {
		if e := file.Sync(); e != nil {
			return e
		}
		syncs++
		return nil
	}
	fake := &fakeAPI{}
	factories := 0
	e = runControllerProcessHandoffCreate(context.Background(), j, raw, os.NewFile(3, "g01-control"), os.Stdin, "github-rest-workflow-v1", w, i, time.Now, func(ctx context.Context, a Approval, c Credentials) (API, error) {
		factories++
		if syncs != 1 || len(j.Events()) != 1 || j.Events()[0].ControllerHandoff == nil {
			return nil, ErrApproval
		}
		stored, e := j.root.ReadFile("journal.jsonl")
		if e != nil || !bytes.Contains(stored, []byte("controller-handoff-consumed")) {
			return nil, ErrApproval
		}
		return handoffValidInventoryAPI{fake}, nil
	})
	if e != nil || factories != 1 || fake.createCalls != 1 {
		return 8
	}
	return 0
}

// Keep a test-only duplicate of stdin to inspect unread bytes after the receiver
// closes its owned endpoint. No production caller retains a duplicate.
func processUnreadCopy(t *testing.T, input *os.File) *os.File {
	t.Helper()
	fd, e := syscall.Dup(int(input.Fd()))
	if e != nil {
		t.Fatal(e)
	}
	copy := os.NewFile(uintptr(fd), "test-unread-copy")
	t.Cleanup(func() { copy.Close() })
	return copy
}
func TestControllerProcessHandoffRejectsBeforeInput(t *testing.T) {
	for _, fault := range []string{"workflow root", "issuer root", "same role key", "rootless", "source", "audience", "issuer source", "workflow key", "phase", "owner", "receipt nonce", "raw approval", "raw receipt", "signature", "expiry", "approval expiry", "parent deadline", "nil factory", "approval empty", "approval oversize"} {
		t.Run(fault, func(t *testing.T) {
			f, j := openCreateControllerHandoff(t)
			data := handoffCredentialBytes(t, f.approval)
			b, issuer, key := processBinding(t, f, data)
			workflow := f.root
			source := f.request.Source
			snapshot := f.snapshot
			receipt := f.receiptBytes
			ctx := context.Background()
			factory := func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("rejected envelope reached factory")
				return nil, nil
			}
			switch fault {
			case "workflow root":
				other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{72}, 32))
				workflow, _ = handoff.NewTrustRoot(workflow.KeyID(), other.Public().(ed25519.PublicKey))
			case "issuer root":
				other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{72}, 32))
				issuer, _ = handoff.NewIssuerRoot(issuer.KeyID(), other.Public().(ed25519.PublicKey))
			case "same role key":
				issuer, _ = handoff.NewIssuerRoot("other-role", f.privateKey.Public().(ed25519.PublicKey))
			case "rootless":
				issuer = handoff.IssuerRoot{}
			case "source":
				source = "other-source"
			case "audience":
				b.Audience = "other"
			case "issuer source":
				b.IssuerSource = "other"
			case "workflow key":
				b.WorkflowKeyID = "other"
			case "phase":
				b.Phase = "cleanup"
			case "owner":
				b.OwnerNonce = strings.Repeat("e", 32)
			case "receipt nonce":
				b.ReceiptNonce = strings.Repeat("e", 32)
			case "raw approval":
				snapshot = append(bytes.Clone(snapshot), ' ')
			case "raw receipt":
				receipt = append(bytes.Clone(receipt), ' ')
			case "expiry":
				b.ExpiresAt = time.Now().Add(-time.Second)
			case "approval expiry":
				b.ExpiresAt = f.approval.ExpiresAt.Add(time.Second)
			case "parent deadline":
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
			case "nil factory":
				factory = nil
			case "approval empty":
				snapshot = nil
			case "approval oversize":
				snapshot = bytes.Repeat([]byte(" "), maxControllerApprovalBytes+1)
			}
			b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes()))
			if fault == "signature" {
				b.Signature = strings.Repeat("A", 86)
			}
			cr, ir := processPipes(t, receipt, b, data)
			unread := processUnreadCopy(t, ir)
			if runControllerProcessHandoffCreate(ctx, j, snapshot, cr, ir, source, workflow, issuer, time.Now, factory) == nil || len(j.Events()) != 0 {
				t.Fatal("invalid authority consumed create")
			}
			remaining, e := io.ReadAll(unread)
			if e != nil || !bytes.Equal(remaining, data) {
				t.Fatal("rejected authority read stdin")
			}
		})
	}
}
func TestControllerProcessHandoffByteAndSemanticFailuresBurnCreate(t *testing.T) {
	for _, fault := range []string{"whitespace", "length", "oversize", "empty", "semantic", "token margin", "input tail"} {
		t.Run(fault, func(t *testing.T) {
			f, j := openCreateControllerHandoff(t)
			signed := handoffCredentialBytes(t, f.approval)
			data := bytes.Clone(signed)
			switch fault {
			case "whitespace":
				data = append(data, ' ')
			case "length":
				data = data[:len(data)-1]
			case "oversize":
				data = bytes.Repeat([]byte(" "), handoff.MaxCredentialBytes+1)
			case "empty":
				data = nil
			case "semantic":
				c := credentials(f.approval)
				c.InstallationID++
				data, _ = json.Marshal(c)
				signed = bytes.Clone(data)
			case "token margin":
				c := credentials(f.approval)
				c.ExpiresAt = f.now.Add(time.Minute + 10*time.Second)
				data, _ = json.Marshal(c)
				signed = bytes.Clone(data)
			case "input tail":
				data = append(data, []byte(" {}")...)
			}
			b, issuer, _ := processBinding(t, f, signed)
			cr, ir := processPipes(t, f.receiptBytes, b, data)
			copy := processUnreadCopy(t, ir)
			if runControllerProcessHandoffCreate(context.Background(), j, f.snapshot, cr, ir, f.request.Source, f.root, issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("bad bytes built API")
				return nil, nil
			}) == nil {
				t.Fatal("invalid credentials accepted")
			}
			if len(j.Events()) != 1 || j.Events()[0].ControllerHandoff == nil {
				t.Fatal("failed credential read reset consumed slot")
			}
			if remaining, _ := io.ReadAll(copy); len(remaining) != 0 {
				t.Fatal("payload not read exactly once")
			}
			directory := j.directory
			if j.Close() != nil {
				t.Fatal("close consumed journal")
			}
			reopened, e := openTestJournal(t, directory, f.approval)
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			r := f.receipt
			r.ReceiptNonce = strings.Repeat("e", 32)
			f.receipt = r
			f.receiptBytes = f.encodeReceipt(t, r, true)
			b, issuer, _ = processBinding(t, f, signed)
			cr, ir = processPipes(t, f.receiptBytes, b, signed)
			unread := processUnreadCopy(t, ir)
			if runControllerProcessHandoffCreate(context.Background(), reopened, f.snapshot, cr, ir, f.request.Source, f.root, issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("fresh nonce retried API")
				return nil, nil
			}) == nil {
				t.Fatal("fresh nonce replayed")
			}
			remaining, e := io.ReadAll(unread)
			if e != nil || !bytes.Equal(remaining, signed) {
				t.Fatal("reopened create reread stdin")
			}
		})
	}
}
func TestControllerProcessHandoffConcurrentOnce(t *testing.T) {
	f, j := openCreateControllerHandoff(t)
	data := handoffCredentialBytes(t, f.approval)
	b, issuer, _ := processBinding(t, f, data)
	start := make(chan struct{})
	done := make(chan error, 2)
	var factories atomic.Int32
	for range 2 {
		cr, ir := processPipes(t, f.receiptBytes, b, data)
		go func() {
			<-start
			done <- runControllerProcessHandoffCreate(context.Background(), j, f.snapshot, cr, ir, f.request.Source, f.root, issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) { factories.Add(1); return nil, ErrApproval })
		}()
	}
	close(start)
	for range 2 {
		if <-done == nil {
			t.Fatal("nil API accepted")
		}
	}
	if factories.Load() != 1 || len(j.Events()) != 1 {
		t.Fatalf("concurrent consumption: factories=%d events=%d", factories.Load(), len(j.Events()))
	}
}
func TestControllerProcessHandoffDescriptorAndEOFBounds(t *testing.T) {
	for _, fault := range []string{"missing control", "missing input", "same file", "aliased pipe", "regular control", "regular input", "malformed", "truncated", "tail", "control withheld EOF", "input withheld EOF", "blocked control", "blocked input", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			f, j := openCreateControllerHandoff(t)
			data := handoffCredentialBytes(t, f.approval)
			b, issuer, key := processBinding(t, f, data)
			b.ExpiresAt = time.Now().Add(200 * time.Millisecond)
			b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes()))
			raw, _ := json.Marshal(b)
			frame, _ := handoff.EncodeControl(f.receiptBytes, raw)
			cr, cw, _ := os.Pipe()
			ir, iw, _ := os.Pipe()
			t.Cleanup(func() { cr.Close(); cw.Close(); ir.Close(); iw.Close() })
			control, input := cr, ir
			switch fault {
			case "missing control":
				control = nil
			case "missing input":
				input = nil
			case "same file":
				input = cr
			case "aliased pipe":
				input = processUnreadCopy(t, cr)
			case "regular control":
				control, e := os.CreateTemp(t.TempDir(), "regular")
				if e != nil {
					t.Fatal(e)
				}
				defer control.Close()
				cr.Close()
				cr = control
			case "regular input":
				file, e := os.CreateTemp(t.TempDir(), "regular")
				if e != nil {
					t.Fatal(e)
				}
				defer file.Close()
				input = file
			case "malformed":
				frame[0] = 'x'
			case "truncated":
				frame = frame[:len(frame)-1]
			case "tail":
				frame = append(frame, 'x')
			}
			if fault == "regular control" {
				control = cr
			}
			stop, writerDone := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(writerDone)
				if fault == "blocked control" {
					<-stop
					cw.Close()
					iw.Close()
					return
				}
				_, _ = cw.Write(frame)
				if fault == "control withheld EOF" {
					<-stop
				}
				cw.Close()
				if fault == "blocked input" {
					<-stop
					iw.Close()
					return
				}
				_, _ = iw.Write(data)
				if fault == "input withheld EOF" {
					<-stop
				}
				iw.Close()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			if fault == "canceled" {
				cancel()
			}
			started := time.Now()
			e := runControllerProcessHandoffCreate(ctx, j, f.snapshot, control, input, f.request.Source, f.root, issuer, time.Now, func(context.Context, Approval, Credentials) (API, error) {
				t.Fatal("invalid descriptors/input built API")
				return nil, nil
			})
			cr.Close()
			ir.Close()
			close(stop)
			<-writerDone
			wantEvents := 0
			if fault == "input withheld EOF" || fault == "blocked input" {
				wantEvents = 1
			}
			if e == nil || time.Since(started) > time.Second || len(j.Events()) != wantEvents {
				t.Fatalf("unbounded/unsafe IO: err=%v elapsed=%v events=%d", e, time.Since(started), len(j.Events()))
			}
		})
	}
}
func TestControllerProcessHandoffCumulativeDeadline(t *testing.T) {
	f, j := openCreateControllerHandoff(t)
	data := handoffCredentialBytes(t, f.approval)
	b, issuer, key := processBinding(t, f, data)
	started := time.Now()
	b.ExpiresAt = started.Add(200 * time.Millisecond)
	b.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b.SigningBytes()))
	cr, ir := processPipes(t, f.receiptBytes, b, data)
	calls := 0
	e := runControllerProcessHandoffCreate(context.Background(), j, f.snapshot, cr, ir, f.request.Source, f.root, issuer, time.Now, func(ctx context.Context, _ Approval, _ Credentials) (API, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || deadline.After(b.ExpiresAt) || deadline.After(started.Add(handoff.MaxProcessDuration)) {
			t.Fatal("factory extended cumulative input budget")
		}
		<-ctx.Done()
		return nil, ErrApproval
	})
	if e == nil || calls != 1 || time.Since(started) > time.Second || len(j.Events()) != 1 {
		t.Fatal("effect was not bounded by original deadline")
	}
}
