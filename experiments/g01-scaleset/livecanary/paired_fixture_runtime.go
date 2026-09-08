//go:build g01_pair_fixture

package livecanary

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/actions/scaleset"
	"github.com/hashicorp/go-retryablehttp"

	"github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"
)

// These constructors are only present in the explicitly tagged offline
// bridge fixture. The ordinary binary cannot select a caller-provided API
// endpoint or journal admission root.
func OpenJournalForPairedFixture(directory string, a Approval) (*FileJournal, error) {
	return OpenJournalForPairedFixtureAt(directory, a, directory)
}

func OpenJournalForPairedFixtureAt(directory string, a Approval, admissionDirectory string) (*FileJournal, error) {
	return openJournalAtAdmission(directory, a, admissionDirectory, func(file *os.File) error { return file.Sync() })
}

func PreparePairedJournalForFixture(directory string, a Approval) (PreparationReceipt, error) {
	return preparePairedJournal(directory, a, OpenJournalForPairedFixture)
}

func PreparePairedJournalForFixtureAt(directory string, a Approval, admissionDirectory string) (PreparationReceipt, error) {
	return preparePairedJournal(directory, a, func(path string, approval Approval) (*FileJournal, error) {
		return OpenJournalForPairedFixtureAt(path, approval, admissionDirectory)
	})
}

// PrepareWorkerJournalForPairedFixtureAt routes worker preparation through the
// canonical liveworker journal/admission parser while keeping the root inside
// the generated offline fixture.
func PrepareWorkerJournalForPairedFixtureAt(directory string, a liveworker.Approval, admissionDirectory string) (liveworker.PreparationReceipt, error) {
	return liveworker.PrepareJournalForPairedFixture(directory, a, admissionDirectory)
}

func fixtureEndpoint(baseURL string, caPEM []byte) (*url.URL, *x509.CertPool, *x509.Certificate, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, nil, nil, ErrApproval
	}
	host := u.Hostname()
	if net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || u.Port() == "" {
		return nil, nil, nil, ErrApproval
	}
	block, _ := pem.Decode(caPEM)
	if block == nil || block.Type != "CERTIFICATE" || len(block.Bytes) == 0 || len(caPEM) > 8192 {
		return nil, nil, nil, ErrApproval
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, nil, ErrApproval
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return u, pool, cert, nil
}

func NewSDKAPIForPairedFixture(a Approval, c Credentials, baseURL string, caPEM []byte) (*SDKAPI, error) {
	u, roots, certificate, err := fixtureEndpoint(baseURL, caPEM)
	if err != nil || a.Validate(time.Now()) != nil || c.validate(a, time.Now()) != nil {
		return nil, ErrApproval
	}
	serverName := u.Hostname()
	if err := certificate.VerifyHostname(serverName); err != nil {
		if len(certificate.DNSNames) == 0 {
			return nil, ErrApproval
		}
		serverName = certificate.DNSNames[0]
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: serverName, NextProtos: []string{"http/1.1"}}
	address := u.Host
	transport.DialContext = func(ctx context.Context, network, target string) (net.Conn, error) {
		if network != "tcp" || target != address {
			return nil, ErrApproval
		}
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, target)
	}
	retry := retryablehttp.NewClient()
	retry.RetryMax = 0
	retry.Logger = nil
	httpClient := &http.Client{Transport: withResponseBudget(transport), Timeout: operationTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	retry.HTTPClient = httpClient
	options := []scaleset.HTTPOption{scaleset.WithRetryableHTTPClint(retry), scaleset.WithLogger(slog.New(slog.DiscardHandler))}
	client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{GitHubConfigURL: baseURL + "/" + a.Organization, PersonalAccessToken: c.InstallationToken}, options...)
	if err != nil {
		return nil, ErrApproval
	}
	return &SDKAPI{client: client, rest: httpClient, baseURL: baseURL, approval: a, credentials: c, options: options}, nil
}

func fixtureFastPairCadence() pairedBaselineCadence {
	now := time.Now()
	return pairedBaselineCadence{
		now: func() time.Time { return now },
		wait: func(ctx context.Context, delay time.Duration) error {
			if ctx.Err() != nil {
				return ErrQuarantine
			}
			now = now.Add(delay)
			return nil
		},
	}
}

// RunPairedTerminalForFixture calls the exported production entrypoint with
// only generated private roots and a loopback TLS endpoint. The fast cadence
// is a test-only clock seam; it still retains every production HTTP, journal,
// binding, and effect ordering check.
func RunPairedTerminalForFixture(ctx context.Context, files PairedTerminalFiles, c Credentials, baseURL string, caPEM []byte, controllerAdmissionDirectory string) error {
	if ctx == nil || filepath.Clean(files.ControllerStateDirectory) != files.ControllerStateDirectory || filepath.Clean(files.WorkerStateDirectory) != files.WorkerStateDirectory {
		return ErrApproval
	}
	if _, _, _, err := fixtureEndpoint(baseURL, caPEM); err != nil {
		return err
	}
	if !filepath.IsAbs(controllerAdmissionDirectory) || filepath.Clean(controllerAdmissionDirectory) != controllerAdmissionDirectory {
		return ErrApproval
	}
	workerStateReal, err := filepath.EvalSymlinks(files.WorkerStateDirectory)
	if err != nil {
		return ErrJournal
	}
	workerAdmissionDirectory := filepath.Join(filepath.Dir(workerStateReal), "worker-admission")
	if err := os.Mkdir(workerAdmissionDirectory, 0700); err != nil && !os.IsExist(err) {
		return ErrJournal
	}
	oldAdapters, oldCadence := pairedTerminalFixtureAdapters, pairedTerminalFixtureCadence
	pairedTerminalFixtureAdapters = &pairedTerminalAdapters{
		openController: func(path string, a Approval) (*FileJournal, error) {
			return OpenJournalForPairedFixtureAt(path, a, controllerAdmissionDirectory)
		},
		openWorker: func(path string, a liveworker.Approval) (*liveworker.FileJournal, error) {
			return liveworker.OpenJournalForPairedFixture(path, a, workerAdmissionDirectory)
		},
		newAPI: func(a Approval, credentials Credentials) (*SDKAPI, error) {
			return NewSDKAPIForPairedFixture(a, credentials, baseURL, caPEM)
		},
		newDocker: func(a liveworker.Approval) (*liveworker.Docker, error) {
			return liveworker.NewDocker(a)
		},
	}
	pairedTerminalFixtureCadence = fixtureFastPairCadence
	defer func() {
		pairedTerminalFixtureAdapters, pairedTerminalFixtureCadence = oldAdapters, oldCadence
	}()
	return RunPairedTerminal(ctx, files, c)
}
