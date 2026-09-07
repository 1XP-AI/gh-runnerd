//go:build darwin && cgo && g02runtime

// g02-keychain-probe is an explicitly invoked, synthetic-only current-login
// experiment. Building/testing the ordinary module never runs this command.
package main

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>

static OSStatus no_ui(void) { return SecKeychainSetUserInteractionAllowed(false); }
static OSStatus snapshot(CFArrayRef *list, SecKeychainRef *def) {
 OSStatus s=SecKeychainCopySearchList(list); if(s) return s;
 return SecKeychainCopyDefault(def);
}
static int unchanged(CFArrayRef list, SecKeychainRef def) {
 CFArrayRef after=NULL; SecKeychainRef next=NULL;
 OSStatus s=snapshot(&after,&next);
 int equal=s==errSecSuccess && CFEqual(list,after) && CFEqual(def,next);
 if(after)CFRelease(after); if(next)CFRelease(next); return equal;
}
static OSStatus create_canary(const char *path, const char *password, unsigned int length, const unsigned char *secret, SecKeychainRef *keychain) {
 OSStatus s=SecKeychainCreate(path,length,password,false,NULL,keychain); if(s)return s;
 s=SecKeychainUnlock(*keychain,length,password,true); if(s)return s;
 SecAccessRef access=NULL;
 s=SecAccessCreate(CFSTR("gh-runnerd G02 synthetic canary"),NULL,&access); if(s)return s;
 CFDataRef value=CFDataCreate(NULL,secret,32);
 const void *keys[]={kSecClass,kSecAttrService,kSecAttrAccount,kSecUseKeychain,kSecValueData,kSecAttrAccess};
 const void *values[]={kSecClassGenericPassword,CFSTR("gh-runnerd-g02-synthetic"),CFSTR("canary"),*keychain,value,access};
 CFDictionaryRef query=CFDictionaryCreate(NULL,keys,values,6,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 s=SecItemAdd(query,NULL);
 CFRelease(query);CFRelease(value);CFRelease(access);return s;
}
static OSStatus captured_path(SecKeychainRef keychain, char *path) {UInt32 length=4096;return SecKeychainGetPath(keychain,&length,path);}
static OSStatus read_canary(const char *path, unsigned char *bytes) {
 SecKeychainRef keychain=NULL; OSStatus s=SecKeychainOpen(path,&keychain);if(s)return s;
 const void *one[]={keychain};CFArrayRef list=CFArrayCreate(NULL,one,1,&kCFTypeArrayCallBacks);
 const void *keys[]={kSecClass,kSecAttrService,kSecAttrAccount,kSecMatchSearchList,kSecReturnData,kSecMatchLimit};
 const void *values[]={kSecClassGenericPassword,CFSTR("gh-runnerd-g02-synthetic"),CFSTR("canary"),list,kCFBooleanTrue,kSecMatchLimitOne};
 CFDictionaryRef query=CFDictionaryCreate(NULL,keys,values,6,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFTypeRef result=NULL;s=SecItemCopyMatching(query,&result);
 if(!s) {
  if(!result || CFGetTypeID(result)!=CFDataGetTypeID() || CFDataGetLength((CFDataRef)result)!=32)s=errSecDecode;
  else CFDataGetBytes((CFDataRef)result,CFRangeMake(0,32),bytes);
 }
 if(result)CFRelease(result);CFRelease(query);CFRelease(list);CFRelease(keychain);return s;
}
*/
import "C"

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type outcome struct {
	Attempted bool `json:"attempted"`
	Status    int  `json:"status"`
	Matches   bool `json:"canary_matches"`
	SameUID   bool `json:"same_uid"`
}
type report struct {
	Profile              string  `json:"profile"`
	Direct               outcome `json:"direct"`
	LaunchdUnlocked      outcome `json:"launchd_unlocked"`
	LaunchdLocked        outcome `json:"launchd_locked"`
	PreferencesUnchanged bool    `json:"keychain_preferences_unchanged"`
	Cleanup              bool    `json:"cleanup_complete"`
	RecoveryID           string  `json:"recovery_id,omitempty"`
}

func cstring(s string) (*C.char, func()) {
	p := C.CString(s)
	return p, func() { C.free(unsafe.Pointer(p)) }
}
func statusError(phase string, status C.OSStatus) error {
	return fmt.Errorf("%s failed (OSStatus %d)", phase, int(status))
}
func read(root, expected string) outcome {
	var bytes [32]byte
	name, valid := ownedKeychain(root)
	if !valid {
		return outcome{Status: -1}
	}
	path, free := cstring(filepath.Join(root, name))
	defer free()
	status := C.read_canary(path, (*C.uchar)(unsafe.Pointer(&bytes[0])))
	digest := sha256.Sum256(bytes[:])
	clear(bytes[:])
	info, err := os.Lstat(root)
	sameUID := false
	if err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		sameUID = ok && stat.Uid == uint32(os.Geteuid())
	}
	return outcome{Attempted: true, Status: int(status), Matches: status == 0 && hex.EncodeToString(digest[:]) == expected, SameUID: sameUID}
}

type ownerRecord struct {
	Version  string `json:"version"`
	Keychain string `json:"keychain"`
}

func ownedKeychain(root string) (string, bool) {
	if !filepath.IsAbs(root) || !strings.HasPrefix(filepath.Base(root), "gh-runnerd-g02-") {
		return "", false
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return "", false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return "", false
	}
	ownedFile := func(name string) bool {
		file, err := os.Lstat(filepath.Join(root, name))
		if err != nil || !file.Mode().IsRegular() || file.Mode().Perm()&0077 != 0 {
			return false
		}
		owner, ok := file.Sys().(*syscall.Stat_t)
		return ok && owner.Uid == uint32(os.Geteuid())
	}
	if !ownedFile("owned") {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(root, "owned"))
	var record ownerRecord
	if err != nil || json.Unmarshal(data, &record) != nil || record.Version != "gh-runnerd-g02-synthetic-v1" || record.Keychain == "" || record.Keychain == "." || filepath.Base(record.Keychain) != record.Keychain || !ownedFile(record.Keychain) {
		return "", false
	}
	return record.Keychain, true
}
func validRoot(root string) bool { _, valid := ownedKeychain(root); return valid }

func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func launch(parent context.Context, root, phase, expected string, servicesGone *bool) (result outcome, resultErr error) {
	executable, err := os.Executable()
	if err != nil {
		return outcome{}, errors.New("executable resolution failed")
	}
	label := "com.1xp.gh-runnerd.g02." + strings.TrimPrefix(filepath.Base(root), "gh-runnerd-g02-") + "." + phase
	domain := fmt.Sprintf("gui/%d", os.Geteuid())
	plist := filepath.Join(root, phase+".plist")
	output := filepath.Join(root, phase+".json")
	// Only this freshly generated service label is ever bootstrapped or booted out.
	body := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>Label</key><string>` + escape(label) + `</string><key>ProgramArguments</key><array><string>` + escape(executable) + `</string><string>--child-read</string><string>` + escape(root) + `</string><string>` + expected + `</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><false/><key>ProcessType</key><string>Background</string><key>StandardOutPath</key><string>` + escape(output) + `</string><key>StandardErrorPath</key><string>` + escape(filepath.Join(root, phase+".stderr")) + `</string></dict></plist>`
	if os.WriteFile(plist, []byte(body), 0600) != nil {
		return outcome{}, errors.New("probe plist write failed")
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	cleanup := func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "/bin/launchctl", "bootout", domain+"/"+label).Run()
		// A timeout/permission failure is unknown, not proof of absence. The exact
		// missing-service status is 113 on this platform; fail closed otherwise.
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer checkCancel()
		err := exec.CommandContext(checkCtx, "/bin/launchctl", "print", domain+"/"+label).Run()
		var exited *exec.ExitError
		return checkCtx.Err() == nil && errors.As(err, &exited) && exited.ExitCode() == 113
	}
	defer func() {
		if !cleanup() {
			*servicesGone = false
			resultErr = errors.New("probe service cleanup failed")
		}
	}()
	if exec.CommandContext(ctx, "/bin/launchctl", "bootstrap", domain, plist).Run() != nil {
		return outcome{}, errors.New("probe bootstrap failed")
	}
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if parent.Err() != nil {
			return outcome{}, errors.New("probe canceled")
		}
		data, err := os.ReadFile(output)
		var result outcome
		if err == nil && json.Unmarshal(data, &result) == nil {
			return result, nil
		}
	}
	return outcome{}, errors.New("probe result timed out")
}

func run(ctx context.Context) (result report, err error) {
	result.Profile = "synthetic-file-keychain-current-login"
	if os.Geteuid() == 0 {
		return result, errors.New("run this probe as the current non-root login user")
	}
	if status := C.no_ui(); status != 0 {
		return result, statusError("disable probe UI", status)
	}
	var list C.CFArrayRef
	var def C.SecKeychainRef
	if status := C.snapshot(&list, &def); status != 0 {
		return result, statusError("preference snapshot", status)
	}
	defer C.CFRelease(C.CFTypeRef(list))
	defer C.CFRelease(C.CFTypeRef(def))
	root, err := os.MkdirTemp("", "gh-runnerd-g02-")
	if err != nil {
		return result, errors.New("probe directory creation failed")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		_ = os.Remove(root)
		return result, errors.New("probe ownership capture failed")
	}
	var keychain C.SecKeychainRef
	servicesGone := true
	defer func() {
		result.Cleanup = finishOwned(servicesGone,
			func() {
				if keychain != 0 {
					_ = C.SecKeychainLock(keychain)
				}
			},
			func() bool { return keychain == 0 || C.SecKeychainDelete(keychain) == 0 },
			func() bool {
				current, e := os.Lstat(root)
				return e == nil && os.SameFile(rootInfo, current) && current.IsDir() && current.Mode().Perm() == 0700 && os.RemoveAll(root) == nil
			})
		if keychain != 0 {
			C.CFRelease(C.CFTypeRef(keychain))
		}
		result.PreferencesUnchanged = C.unchanged(list, def) != 0
		if !result.Cleanup {
			// Only a non-secret basename is reported; exact labels/plists remain in
			// this private owned directory for operator recovery. Never erase them
			// while service absence or Keychain deletion is uncertain.
			result.RecoveryID = filepath.Base(root)
			err = errors.New("probe cleanup incomplete; private recovery inventory retained")
		}
		if !result.PreferencesUnchanged {
			result.Cleanup = false
			err = errors.New("probe preference invariant failed")
		}
	}()

	passwordBytes := make([]byte, 32)
	if _, err = rand.Read(passwordBytes); err != nil {
		return result, errors.New("probe randomness failed")
	}
	password := hex.EncodeToString(passwordBytes)
	clear(passwordBytes)
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		return result, errors.New("probe randomness failed")
	}
	digest := sha256.Sum256(secret[:])
	expected := hex.EncodeToString(digest[:])
	path, freePath := cstring(filepath.Join(root, "synthetic.keychain"))
	defer freePath()
	pass, freePass := cstring(password)
	defer freePass()
	status := C.create_canary(path, pass, C.uint(len(password)), (*C.uchar)(unsafe.Pointer(&secret[0])), &keychain)
	clear(secret[:])
	if status != 0 {
		return result, statusError("synthetic keychain creation", status)
	}
	var captured [4096]C.char
	if status := C.captured_path(keychain, &captured[0]); status != 0 {
		return result, statusError("capture created Keychain path", status)
	}
	createdPath := C.GoString(&captured[0])
	createdParent, parentError := os.Stat(filepath.Dir(createdPath))
	if parentError != nil || !os.SameFile(rootInfo, createdParent) {
		return result, errors.New("created Keychain escaped owned directory")
	}
	marker, _ := json.Marshal(ownerRecord{Version: "gh-runnerd-g02-synthetic-v1", Keychain: filepath.Base(createdPath)})
	if os.WriteFile(filepath.Join(root, "owned"), marker, 0600) != nil || !validRoot(root) {
		return result, errors.New("probe ownership capture failed")
	}
	if C.unchanged(list, def) == 0 {
		return result, errors.New("private keychain changed preferences")
	}
	result.Direct = read(root, expected)
	result.LaunchdUnlocked, err = launch(ctx, root, "unlocked", expected, &servicesGone)
	if err != nil {
		return result, err
	}
	if status := C.SecKeychainLock(keychain); status != 0 {
		return result, statusError("synthetic keychain lock", status)
	}
	result.LaunchdLocked, err = launch(ctx, root, "locked", expected, &servicesGone)
	if err != nil {
		return result, err
	}
	if !result.Direct.Matches || !result.LaunchdUnlocked.Matches || !result.LaunchdUnlocked.SameUID || result.LaunchdLocked.Status != int(C.errSecInteractionNotAllowed) || result.LaunchdLocked.Matches || !result.LaunchdLocked.SameUID {
		return result, errors.New("probe access expectation failed")
	}
	return result, nil
}
func main() {
	if len(os.Args) == 4 && os.Args[1] == "--child-read" {
		if !validRoot(os.Args[2]) || len(os.Args[3]) != 64 || C.no_ui() != 0 {
			fmt.Fprintln(os.Stderr, "invalid owned probe request")
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(read(os.Args[2], os.Args[3]))
		return
	}
	if len(os.Args) != 2 || os.Args[1] != "--synthetic-current-login" {
		fmt.Fprintln(os.Stderr, "usage: g02-keychain-probe --synthetic-current-login")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := run(ctx)
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
