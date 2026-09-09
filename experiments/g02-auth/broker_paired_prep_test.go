package enrollment

import (
	"crypto/sha256"
	"debug/buildinfo"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

const (
	pairedFixtureBridgeTags     = "g01_live,g01_pair_fixture"
	pairedRealCadenceBridgeTags = "g01_live,g01_pair_fixture,g01_pair_real_cadence"
	pairedFixtureProgramPath    = "github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/cmd/g01-live"
	pairedFixtureSDKVersion     = "v0.4.0"
	pairedFixtureGoVersion      = "go1.26.8"
)

type preparedBridgeBinary struct {
	path, harness, digest string
}

type preparedBridgeReceipt struct {
	head, digest, tags, goVersion, path, sdk, goos, goarch, cgo, modified string
}

var pairedBridgeCompileProbe atomic.Uint32

func notePairedBridgeCompile() {
	pairedBridgeCompileProbe.Add(1)
	if path := os.Getenv("G01_PAIR_COMPILE_PROBE"); path != "" {
		_ = os.WriteFile(path, []byte("compiled\n"), 0600)
	}
}

func validPairedFixtureTags(tags string) bool {
	return tags == pairedFixtureBridgeTags || tags == pairedRealCadenceBridgeTags
}

func validPairedCompileTags(tags string) bool {
	return tags == "g01_live" || validPairedFixtureTags(tags)
}

func pairedFixtureTagSets() []string {
	return []string{pairedFixtureBridgeTags, pairedRealCadenceBridgeTags}
}

func buildPairedG01BinaryWithTags(t *testing.T, tags string) (string, string, string) {
	t.Helper()
	if !validPairedCompileTags(tags) {
		t.Fatal("unreviewed paired fixture tags")
	}
	if validPairedFixtureTags(tags) {
		if dir, ok := os.LookupEnv("G01_PAIR_BRIDGE_PREP_DIR"); ok {
			binary, err := loadPreparedPairedBridgeBinary(dir, tags)
			if err != nil {
				t.Fatal("prepared g01 bridge binary")
			}
			return binary.path, binary.harness, binary.digest
		}
	}
	binary := compilePairedG01Binaries(t, []string{tags})[tags]
	return binary.path, binary.harness, binary.digest
}

func compilePairedG01Binaries(t *testing.T, tagSets []string) map[string]preparedBridgeBinary {
	t.Helper()
	if len(tagSets) == 0 {
		t.Fatal("reviewed g01 bridge tags")
	}
	for _, tags := range tagSets {
		if !validPairedCompileTags(tags) {
			t.Fatal("unreviewed paired fixture tags")
		}
	}
	notePairedBridgeCompile()
	repo := bridgeRepoRoot(t)
	harness := reviewedBridgeHead(t)
	cloneRoot := filepath.Join(t.TempDir(), "repo")
	command := exec.Command("git", "clone", "--no-hardlinks", "--quiet", repo, cloneRoot)
	if output, err := command.CombinedOutput(); err != nil {
		_ = output
		t.Fatal("clone reviewed g01 bridge source")
	}
	binaries := make(map[string]preparedBridgeBinary, len(tagSets))
	for _, tags := range tagSets {
		outRoot := t.TempDir()
		if err := os.Chmod(outRoot, 0700); err != nil {
			t.Fatal("pin reviewed bridge binary parent mode")
		}
		out := filepath.Clean(filepath.Join(outRoot, "g01-live"))
		if !filepath.IsAbs(out) {
			t.Fatal("canonical reviewed bridge binary path")
		}
		command = exec.Command("go", "build", "-buildvcs=true", "-tags", tags, "-o", out, "./cmd/g01-live")
		command.Dir = filepath.Join(cloneRoot, "experiments", "g01-scaleset")
		command.Env = append(os.Environ(), "GOTOOLCHAIN=go1.26.8")
		if output, err := command.CombinedOutput(); err != nil {
			_ = output
			t.Fatal("build reviewed g01 bridge binary")
		}
		if err := os.Chmod(out, 0500); err != nil {
			t.Fatal("pin reviewed bridge binary mode")
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal("read reviewed bridge binary")
		}
		digest := sha256.Sum256(data)
		binaries[tags] = preparedBridgeBinary{path: out, harness: harness, digest: hexDigest(digest[:])}
	}
	return binaries
}

func reviewedBridgeHead(t *testing.T) string {
	t.Helper()
	head, err := currentReviewedBridgeHead()
	if err != nil {
		t.Fatal("read reviewed bridge head")
	}
	return head
}

func currentReviewedBridgeHead() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errBroker
	}
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	harnessBytes, err := command.Output()
	if err != nil {
		return "", errBroker
	}
	harness := strings.TrimSpace(string(harnessBytes))
	if !brokerSHA40.MatchString(harness) {
		return "", errBroker
	}
	return harness, nil
}

func validPairedFixtureBuild(info *debug.BuildInfo, expectedHead, expectedTags string) bool {
	if info == nil || info.GoVersion != pairedFixtureGoVersion || info.Path != pairedFixtureProgramPath || !brokerSHA40.MatchString(expectedHead) || !validPairedFixtureTags(expectedTags) {
		return false
	}
	revision, clean, sdk := "", false, false
	goos, goarch, cgo, tags := "", "", "", ""
	for _, setting := range info.Settings {
		switch setting.Key {
		case "GOOS":
			goos = setting.Value
		case "GOARCH":
			goarch = setting.Value
		case "CGO_ENABLED":
			cgo = setting.Value
		case "-tags":
			tags = setting.Value
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			clean = setting.Value == "false"
		}
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/actions/scaleset" {
			sdk = dep.Version == pairedFixtureSDKVersion && dep.Replace == nil
		}
	}
	return goos == runtime.GOOS && goarch == runtime.GOARCH && cgo == "1" && revision == expectedHead && clean && sdk && tags == expectedTags
}

func formatPreparedBridgeReceipt(head, digest, tags string) []byte {
	return []byte(strings.Join([]string{
		"head " + head,
		"digest " + digest,
		"tags " + tags,
		"go " + pairedFixtureGoVersion,
		"path " + pairedFixtureProgramPath,
		"sdk " + pairedFixtureSDKVersion,
		"goos " + runtime.GOOS,
		"goarch " + runtime.GOARCH,
		"cgo 1",
		"modified false",
	}, "\n") + "\n")
}

func parsePreparedBridgeReceipt(data []byte) (preparedBridgeReceipt, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 10 {
		return preparedBridgeReceipt{}, errBroker
	}
	var receipt preparedBridgeReceipt
	seen := map[string]bool{}
	for _, line := range lines {
		key, value, ok := strings.Cut(line, " ")
		if !ok || value == "" || strings.Contains(value, "\n") || seen[key] {
			return preparedBridgeReceipt{}, errBroker
		}
		seen[key] = true
		switch key {
		case "head":
			receipt.head = value
		case "digest":
			receipt.digest = value
		case "tags":
			receipt.tags = value
		case "go":
			receipt.goVersion = value
		case "path":
			receipt.path = value
		case "sdk":
			receipt.sdk = value
		case "goos":
			receipt.goos = value
		case "goarch":
			receipt.goarch = value
		case "cgo":
			receipt.cgo = value
		case "modified":
			receipt.modified = value
		default:
			return preparedBridgeReceipt{}, errBroker
		}
	}
	if !brokerSHA40.MatchString(receipt.head) || !brokerSHA256.MatchString(receipt.digest) || !validPairedFixtureTags(receipt.tags) {
		return preparedBridgeReceipt{}, errBroker
	}
	if receipt.goVersion != pairedFixtureGoVersion || receipt.path != pairedFixtureProgramPath || receipt.sdk != pairedFixtureSDKVersion {
		return preparedBridgeReceipt{}, errBroker
	}
	if receipt.goos != runtime.GOOS || receipt.goarch != runtime.GOARCH || receipt.cgo != "1" || receipt.modified != "false" {
		return preparedBridgeReceipt{}, errBroker
	}
	return receipt, nil
}

func writeAtomicPrivateFile(root *os.Root, name string, data []byte, mode os.FileMode) error {
	if root == nil || name == "" || name != filepath.Base(name) || len(data) == 0 {
		return errBroker
	}
	tmp := name + ".tmp"
	_ = root.Remove(tmp)
	file, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return errBroker
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = root.Remove(tmp)
		return errBroker
	}
	if err := file.Sync(); err != nil {
		file.Close()
		_ = root.Remove(tmp)
		return errBroker
	}
	if err := file.Close(); err != nil {
		_ = root.Remove(tmp)
		return errBroker
	}
	if err := root.Chmod(tmp, mode); err != nil {
		_ = root.Remove(tmp)
		return errBroker
	}
	if err := root.Rename(tmp, name); err != nil {
		_ = root.Remove(tmp)
		return errBroker
	}
	return nil
}

func loadPreparedPairedBridgeBinary(directory, tags string) (preparedBridgeBinary, error) {
	directory = filepath.Clean(directory)
	if directory == "" || directory == "." || !filepath.IsAbs(directory) || !validPairedFixtureTags(tags) {
		return preparedBridgeBinary{}, errBroker
	}
	head, err := currentReviewedBridgeHead()
	if err != nil {
		return preparedBridgeBinary{}, errBroker
	}
	parent, err := openBrokerPrivateDirectory(directory)
	if err != nil {
		return preparedBridgeBinary{}, errBroker
	}
	parent.Close()
	variantPath := filepath.Join(directory, tags)
	variant, err := openBrokerPrivateDirectory(variantPath)
	if err != nil {
		return preparedBridgeBinary{}, errBroker
	}
	variant.Close()
	metaFile, err := openBrokerPrivateFile(filepath.Join(variantPath, "g01-live.meta"), 0600, 4096)
	if err != nil {
		return preparedBridgeBinary{}, errBroker
	}
	meta, err := io.ReadAll(metaFile)
	metaFile.Close()
	if err != nil {
		return preparedBridgeBinary{}, errBroker
	}
	receipt, err := parsePreparedBridgeReceipt(meta)
	if err != nil || receipt.tags != tags || receipt.head != head {
		return preparedBridgeBinary{}, errBroker
	}
	path := filepath.Join(variantPath, "g01-live")
	file, err := openBrokerPrivateFile(path, 0500, 128<<20)
	if err != nil {
		return preparedBridgeBinary{}, errBroker
	}
	binary := &verifiedBrokerBinary{path: path, file: file, digest: receipt.digest}
	info, err := buildinfo.Read(file)
	if err != nil || !validPairedFixtureBuild(info, head, tags) || binary.check() != nil {
		file.Close()
		return preparedBridgeBinary{}, errBroker
	}
	if file.Close() != nil {
		return preparedBridgeBinary{}, errBroker
	}
	return preparedBridgeBinary{path: path, harness: head, digest: receipt.digest}, nil
}

func storePreparedBridgeBinary(t *testing.T, directory, tags, path, harness, digest string) {
	t.Helper()
	directory = filepath.Clean(directory)
	if directory == "" || directory == "." || !filepath.IsAbs(directory) || !validPairedFixtureTags(tags) {
		t.Fatal("bounded fixture preparation directory")
	}
	if !brokerSHA40.MatchString(harness) || !brokerSHA256.MatchString(digest) {
		t.Fatal("reviewed bridge receipt")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		t.Fatal("bounded fixture preparation directory")
	}
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal("pin fixture preparation directory mode")
	}
	parent, err := openBrokerPrivateDirectory(directory)
	if err != nil {
		t.Fatal("owned fixture preparation directory")
	}
	if err := parent.Mkdir(tags, 0700); err != nil {
		existing, statErr := parent.Lstat(tags)
		if statErr != nil || !existing.IsDir() {
			parent.Close()
			t.Fatal("owned fixture variant directory")
		}
	}
	if err := parent.Chmod(tags, 0700); err != nil {
		parent.Close()
		t.Fatal("pin fixture variant directory mode")
	}
	parent.Close()
	variant, err := openBrokerPrivateDirectory(filepath.Join(directory, tags))
	if err != nil {
		t.Fatal("owned fixture variant directory")
	}
	defer variant.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read reviewed bridge binary")
	}
	if hexDigest(sha256Sum(data)) != digest {
		t.Fatal("store reviewed bridge digest")
	}
	if writeAtomicPrivateFile(variant, "g01-live", data, 0500) != nil {
		t.Fatal("store reviewed bridge binary")
	}
	if writeAtomicPrivateFile(variant, "g01-live.meta", formatPreparedBridgeReceipt(harness, digest, tags), 0600) != nil {
		t.Fatal("store reviewed bridge metadata")
	}
}

func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func ownedPrepDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal("pin fixture preparation directory mode")
	}
	return dir
}

func materializePreparedVariants(t *testing.T) string {
	t.Helper()
	dest := ownedPrepDirectory(t)
	var binaries map[string]preparedBridgeBinary
	if dir, ok := os.LookupEnv("G01_PAIR_BRIDGE_PREP_DIR"); ok {
		binaries = make(map[string]preparedBridgeBinary, 2)
		for _, tags := range pairedFixtureTagSets() {
			binary, err := loadPreparedPairedBridgeBinary(dir, tags)
			if err != nil {
				t.Fatal("prepared fixture source")
			}
			binaries[tags] = binary
		}
	} else {
		binaries = compilePairedG01Binaries(t, pairedFixtureTagSets())
	}
	for _, tags := range pairedFixtureTagSets() {
		binary := binaries[tags]
		storePreparedBridgeBinary(t, dest, tags, binary.path, binary.harness, binary.digest)
	}
	return dest
}

func clonePreparedVariants(t *testing.T, src string) string {
	t.Helper()
	dest := ownedPrepDirectory(t)
	for _, tags := range pairedFixtureTagSets() {
		binary, err := loadPreparedPairedBridgeBinary(src, tags)
		if err != nil {
			t.Fatal("clone prepared fixture")
		}
		storePreparedBridgeBinary(t, dest, tags, binary.path, binary.harness, binary.digest)
	}
	return dest
}

func overwritePreparedMeta(t *testing.T, dir, tags string, data []byte) {
	t.Helper()
	variant, err := openBrokerPrivateDirectory(filepath.Join(dir, tags))
	if err != nil {
		t.Fatal("owned fixture variant directory")
	}
	defer variant.Close()
	_ = variant.Remove("g01-live.meta")
	if writeAtomicPrivateFile(variant, "g01-live.meta", data, 0600) != nil {
		t.Fatal("rewrite prepared fixture metadata")
	}
}

func buildinfoTags(t *testing.T, path string) string {
	t.Helper()
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		t.Fatal("read prepared fixture buildinfo")
	}
	for _, setting := range info.Settings {
		if setting.Key == "-tags" {
			return setting.Value
		}
	}
	t.Fatal("prepared fixture tags")
	return ""
}

func unsetPairedBridgePrepDir(t *testing.T) {
	t.Helper()
	old, ok := os.LookupEnv("G01_PAIR_BRIDGE_PREP_DIR")
	if err := os.Unsetenv("G01_PAIR_BRIDGE_PREP_DIR"); err != nil {
		t.Fatal("clear prepared fixture directory")
	}
	t.Cleanup(func() {
		if !ok {
			_ = os.Unsetenv("G01_PAIR_BRIDGE_PREP_DIR")
			return
		}
		_ = os.Setenv("G01_PAIR_BRIDGE_PREP_DIR", old)
	})
}

func filteredPairedPrepEnv(extra ...string) []string {
	drop := map[string]bool{
		"G01_PAIR_BRIDGE_PREP_DIR":    true,
		"G01_PAIR_COMPILE_PROBE":      true,
		"G01_PAIR_MISSING_PREP_CHILD": true,
	}
	env := make([]string, 0, len(os.Environ())+len(extra))
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if drop[key] {
			continue
		}
		env = append(env, item)
	}
	return append(env, extra...)
}

func TestPairedBrokerPrepareReviewedG01LiveBinary(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private Unix fixture requires a Unix host")
	}
	binaries := compilePairedG01Binaries(t, pairedFixtureTagSets())
	dir, ok := os.LookupEnv("G01_PAIR_BRIDGE_PREP_DIR")
	if !ok {
		return
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		t.Fatal("bounded fixture preparation directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal("pin fixture preparation directory mode")
	}
	for _, tags := range pairedFixtureTagSets() {
		binary := binaries[tags]
		storePreparedBridgeBinary(t, dir, tags, binary.path, binary.harness, binary.digest)
		if _, err := loadPreparedPairedBridgeBinary(dir, tags); err != nil {
			t.Fatal("stored prepared g01 bridge binary")
		}
	}
}

func TestPairedBrokerPreparedGateRefusesMissingPrepWithoutCompile(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private Unix fixture requires a Unix host")
	}
	if os.Getenv("G01_PAIR_MISSING_PREP_CHILD") == "1" {
		buildPairedG01BinaryWithTags(t, pairedRealCadenceBridgeTags)
		return
	}
	probe := filepath.Join(t.TempDir(), "compile-probe")
	dir := ownedPrepDirectory(t)
	command := exec.Command(os.Args[0], "-test.run=^TestPairedBrokerPreparedGateRefusesMissingPrepWithoutCompile$", "-test.count=1")
	command.Env = filteredPairedPrepEnv(
		"G01_PAIR_MISSING_PREP_CHILD=1",
		"G01_PAIR_BRIDGE_PREP_DIR="+dir,
		"G01_PAIR_COMPILE_PROBE="+probe,
	)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "prepared g01 bridge binary") {
		t.Fatalf("missing prep compiled or passed: %s", output)
	}
	if _, statErr := os.Lstat(probe); statErr == nil {
		t.Fatal("missing prep invoked git clone or go build")
	}
}

func TestPairedBrokerPreparedNormalAndCadenceLoadSameHeadWithoutCompile(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private Unix fixture requires a Unix host")
	}
	dir := materializePreparedVariants(t)
	t.Setenv("G01_PAIR_BRIDGE_PREP_DIR", dir)
	before := pairedBridgeCompileProbe.Load()
	cadencePath, cadenceHead, _ := buildPairedG01BinaryWithTags(t, pairedRealCadenceBridgeTags)
	normalPath, normalHead, _ := buildPairedG01BinaryWithTags(t, pairedFixtureBridgeTags)
	if pairedBridgeCompileProbe.Load() != before {
		t.Fatal("prepared load rebuilt a fixture variant")
	}
	head := reviewedBridgeHead(t)
	if cadenceHead != head || normalHead != head {
		t.Fatal("prepared variants are not the current reviewed head")
	}
	if cadencePath == normalPath {
		t.Fatal("cadence and normal variants share a path")
	}
	if got := buildinfoTags(t, cadencePath); got != pairedRealCadenceBridgeTags {
		t.Fatalf("cadence tags=%q", got)
	}
	if got := buildinfoTags(t, normalPath); got != pairedFixtureBridgeTags {
		t.Fatalf("normal tags=%q", got)
	}
}

func TestPairedBrokerPreparedReceiptRefusesStaleCorruptVariantAndSymlink(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private Unix fixture requires a Unix host")
	}
	src := materializePreparedVariants(t)
	before := pairedBridgeCompileProbe.Load()
	head := reviewedBridgeHead(t)
	for _, tc := range []struct {
		name string
		tags string
		mut  func(*testing.T, string)
	}{
		{
			name: "missing variant",
			tags: pairedRealCadenceBridgeTags,
			mut:  func(*testing.T, string) {},
		},
		{
			name: "stale head",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				binary, err := loadPreparedPairedBridgeBinary(dir, pairedRealCadenceBridgeTags)
				if err != nil {
					t.Fatal("prepared cadence source")
				}
				overwritePreparedMeta(t, dir, pairedRealCadenceBridgeTags, formatPreparedBridgeReceipt(strings.Repeat("a", 40), binary.digest, pairedRealCadenceBridgeTags))
			},
		},
		{
			name: "digest mismatch",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				binary, err := loadPreparedPairedBridgeBinary(dir, pairedRealCadenceBridgeTags)
				if err != nil {
					t.Fatal("prepared cadence source")
				}
				overwritePreparedMeta(t, dir, pairedRealCadenceBridgeTags, formatPreparedBridgeReceipt(binary.harness, strings.Repeat("c", 64), pairedRealCadenceBridgeTags))
			},
		},
		{
			name: "old two-line meta",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				overwritePreparedMeta(t, dir, pairedRealCadenceBridgeTags, []byte(head+"\n"+strings.Repeat("d", 64)+"\n"))
			},
		},
		{
			name: "wrong variant",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				normal, err := loadPreparedPairedBridgeBinary(dir, pairedFixtureBridgeTags)
				if err != nil {
					t.Fatal("prepared normal source")
				}
				storePreparedBridgeBinary(t, dir, pairedRealCadenceBridgeTags, normal.path, normal.harness, normal.digest)
			},
		},
		{
			name: "mismatched bytes",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				path := filepath.Join(dir, pairedRealCadenceBridgeTags, "g01-live")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal("read prepared cadence binary")
				}
				if os.Chmod(path, 0600) != nil || os.WriteFile(path, append(append([]byte{}, data...), 0), 0500) != nil || os.Chmod(path, 0500) != nil {
					t.Fatal("mutate prepared cadence bytes")
				}
			},
		},
		{
			name: "root mode",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				if os.Chmod(dir, 0777) != nil {
					t.Fatal("relax fixture root mode")
				}
			},
		},
		{
			name: "meta mode",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				if os.Chmod(filepath.Join(dir, pairedRealCadenceBridgeTags, "g01-live.meta"), 0644) != nil {
					t.Fatal("relax fixture metadata mode")
				}
			},
		},
		{
			name: "parent symlink",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				link := filepath.Join(t.TempDir(), "prep-link")
				if os.Symlink(dir, link) != nil {
					t.Fatal("fixture parent symlink")
				}
				if _, err := loadPreparedPairedBridgeBinary(link, pairedRealCadenceBridgeTags); err == nil {
					t.Fatal("parent symlink accepted")
				}
			},
		},
		{
			name: "binary symlink",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				path := filepath.Join(dir, pairedRealCadenceBridgeTags, "g01-live")
				backup := path + ".real"
				if os.Rename(path, backup) != nil || os.Symlink(backup, path) != nil {
					t.Fatal("fixture binary symlink")
				}
			},
		},
		{
			name: "meta symlink",
			tags: pairedRealCadenceBridgeTags,
			mut: func(t *testing.T, dir string) {
				path := filepath.Join(dir, pairedRealCadenceBridgeTags, "g01-live.meta")
				backup := path + ".real"
				if os.Rename(path, backup) != nil || os.Symlink(backup, path) != nil {
					t.Fatal("fixture metadata symlink")
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := ownedPrepDirectory(t)
			if tc.name != "missing variant" {
				dir = clonePreparedVariants(t, src)
			}
			tc.mut(t, dir)
			if tc.name == "parent symlink" {
				return
			}
			if _, err := loadPreparedPairedBridgeBinary(dir, tc.tags); err == nil {
				t.Fatal("invalid prepared fixture accepted")
			}
		})
	}
	if pairedBridgeCompileProbe.Load() != before {
		t.Fatal("negative prepared receipt checks compiled")
	}
}

func TestPairedBrokerPreparedDirChangeDoesNotReusePriorArtifact(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private Unix fixture requires a Unix host")
	}
	first := materializePreparedVariants(t)
	second := clonePreparedVariants(t, first)
	before := pairedBridgeCompileProbe.Load()
	t.Setenv("G01_PAIR_BRIDGE_PREP_DIR", first)
	path1, _, _ := buildPairedG01BinaryWithTags(t, pairedRealCadenceBridgeTags)
	t.Setenv("G01_PAIR_BRIDGE_PREP_DIR", second)
	path2, _, _ := buildPairedG01BinaryWithTags(t, pairedRealCadenceBridgeTags)
	if path1 == path2 {
		t.Fatal("changed prep dir reused the prior artifact")
	}
	if !strings.HasPrefix(path1, first+string(os.PathSeparator)) || !strings.HasPrefix(path2, second+string(os.PathSeparator)) {
		t.Fatal("prepared load did not consume the current prep dir")
	}
	if pairedBridgeCompileProbe.Load() != before {
		t.Fatal("prep dir change compiled")
	}
}

func TestPreparedBridgeStandaloneUnsetPrepCompilesDistinctVariant(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private Unix fixture requires a Unix host")
	}
	unsetPairedBridgePrepDir(t)
	before := pairedBridgeCompileProbe.Load()
	path, _, _ := buildPairedG01BinaryWithTags(t, pairedFixtureBridgeTags)
	if pairedBridgeCompileProbe.Load() == before {
		t.Fatal("unset prep dir did not compile")
	}
	if got := buildinfoTags(t, path); got != pairedFixtureBridgeTags {
		t.Fatalf("standalone tags=%q", got)
	}
}
