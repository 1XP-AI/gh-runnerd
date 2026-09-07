package enrollment

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

func TestBrokerUnsupportedAccountLookupRefuses(t *testing.T) {
	if brokerNativeAccountLookup {
		t.Skip("unsupported profile test")
	}
	if directory, e := brokerAdmissionDirectory(); e == nil || directory != "" {
		t.Fatal("unsupported account lookup derived an admission root")
	}
}
func TestBrokerAccountRootIgnoresEnvironmentAndFailsClosed(t *testing.T) {
	home, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal("fixture")
	}
	os.Chmod(home, 0700)
	t.Setenv("HOME", "/synthetic-env-home")
	t.Setenv("USER", "synthetic-env-user")
	t.Setenv("XDG_STATE_HOME", "/synthetic-state-home")
	uid := strconv.Itoa(os.Geteuid())
	lookup := func(s string) (*user.User, error) {
		if s != uid {
			t.Fatal("wrong lookup UID")
		}
		return &user.User{Uid: uid, HomeDir: home}, nil
	}
	got, e := brokerDirectoryForAccount(lookup)
	if e != nil || got != filepath.Join(home, ".gh-runnerd-g01-experiment") {
		t.Fatal("account root changed with environment")
	}
	for _, kind := range []string{"missing", "relative", "wrong uid", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			_, e := brokerDirectoryForAccount(func(string) (*user.User, error) {
				u := &user.User{Uid: uid, HomeDir: home}
				switch kind {
				case "missing":
					return nil, errBroker
				case "relative":
					u.HomeDir = "relative"
				case "wrong uid":
					u.Uid = "-1"
				case "symlink":
					link := filepath.Join(t.TempDir(), "alias")
					os.Symlink(home, link)
					u.HomeDir = link
				}
				return u, nil
			})
			if e == nil {
				t.Fatal("unsupported lookup accepted")
			}
		})
	}
}
