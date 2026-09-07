package livecanary

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAdmissionDirectoryUsesOSAccountWithoutEnvironmentFallback(t *testing.T) {
	home, err := filepath.EvalSymlinks(privateDir(t))
	if err != nil {
		t.Fatal("private account fixture")
	}
	other := privateDir(t)
	t.Setenv("HOME", other)
	t.Setenv("XDG_STATE_HOME", other)
	uid := strconv.Itoa(os.Geteuid())
	lookup := func(requested string) (*user.User, error) {
		if requested != uid {
			t.Error("lookup did not use effective controller UID")
		}
		return &user.User{Uid: uid, HomeDir: home}, nil
	}
	got, err := admissionDirectoryForAccount(lookup)
	if err != nil || got != filepath.Join(home, ".gh-runnerd-g01-experiment") {
		t.Fatal("environment selected the permanent admission root")
	}
	for _, fault := range []string{"lookup", "missing home", "foreign uid", "relative home", "symlink home", "shared home"} {
		t.Run(fault, func(t *testing.T) {
			account := &user.User{Uid: uid, HomeDir: home}
			var failure error
			switch fault {
			case "lookup":
				failure = errors.New("synthetic account lookup failure")
			case "missing home":
				account.HomeDir = ""
			case "foreign uid":
				account.Uid = strconv.Itoa(os.Geteuid() + 1)
			case "relative home":
				account.HomeDir = "relative"
			case "symlink home":
				alias := filepath.Join(privateDir(t), "alias")
				if os.Symlink(home, alias) != nil {
					t.Fatal("fixture alias")
				}
				account.HomeDir = alias
			case "shared home":
				account.HomeDir = privateDir(t)
				if os.Chmod(account.HomeDir, 0777) != nil {
					t.Fatal("fixture shared account")
				}
			}
			if _, err := admissionDirectoryForAccount(func(string) (*user.User, error) { return account, failure }); err == nil {
				t.Fatal("unsafe account lookup used a fallback")
			}
		})
	}
	if _, err := admissionDirectoryForAccount(nil); err == nil {
		t.Fatal("missing account lookup accepted")
	}
}

func TestAdmissionAuthorityChecksTheCurrentClaim(t *testing.T) {
	for _, fault := range []string{"content", "inode", "directory"} {
		t.Run(fault, func(t *testing.T) {
			parent := privateDir(t)
			state := admissionState(t, parent, "state")
			capRoot := testAdmissionDirectory(t, state)
			a := approval()
			j, err := openJournalAtAdmission(state, a, capRoot, func(f *os.File) error { return f.Sync() })
			if err != nil {
				t.Fatal("private admission fixture")
			}
			defer j.Close()
			path := filepath.Join(capRoot, "admission.json")
			switch fault {
			case "content":
				if os.WriteFile(path, []byte("{}"), 0600) != nil {
					t.Fatal("fixture content")
				}
			case "inode":
				if os.Rename(path, path+".original") != nil || os.WriteFile(path, []byte("{}"), 0600) != nil {
					t.Fatal("fixture replacement")
				}
			case "directory":
				if os.Rename(capRoot, capRoot+".original") != nil || os.Mkdir(capRoot, 0700) != nil {
					t.Fatal("fixture root replacement")
				}
			}
			if release, err := j.authorize(a); err == nil {
				release()
				t.Fatal("changed permanent claim authorized another phase")
			}
		})
	}
}
