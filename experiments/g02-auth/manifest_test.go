package enrollment

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeManifestRejectsNonLoopbackAndOmitsOAuthFields(t *testing.T) {
	for _, redirect := range []string{
		"http://localhost:43111/manifest/callback",
		"http://127.0.0.1/manifest/callback",
		"https://127.0.0.1:43111/manifest/callback",
		"http://127.0.0.1:43111/manifest/callback?x=1",
		"http://127.0.0.1:43111/other",
		"http://0.0.0.0:43111/manifest/callback",
		"http://127.0.0.2:43111/manifest/callback",
		"http://127.0.0.1:0/manifest/callback",
		"http://127.0.0.1:43111/manifest/%63allback",
	} {
		if _, err := EncodeManifest("synthetic-app", redirect); err == nil {
			t.Errorf("accepted untrusted Manifest redirect %q", redirect)
		}
	}
	raw, err := EncodeManifest("synthetic-app", "http://127.0.0.1:43111/manifest/callback")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(raw, &payload) != nil {
		t.Fatal("Manifest is not JSON")
	}
	for _, forbidden := range []string{"callback_urls", "setup_url", "setup_on_update"} {
		if _, ok := payload[forbidden]; ok {
			t.Errorf("OAuth/setup field %s present", forbidden)
		}
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("OAuth/setup field %s leaked into Manifest bytes", forbidden)
		}
	}
	var decoded struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		RedirectURL string `json:"redirect_url"`
		Public      bool   `json:"public"`
		Hook        struct {
			Active bool   `json:"active"`
			URL    string `json:"url"`
		} `json:"hook_attributes"`
		Permissions map[string]string `json:"default_permissions"`
		Events      []string          `json:"default_events"`
		OAuth       bool              `json:"request_oauth_on_install"`
	}
	if json.Unmarshal(raw, &decoded) != nil {
		t.Fatal("Manifest fields lost")
	}
	if decoded.Name != "synthetic-app" || decoded.URL != "https://github.com/1XP-AI/gh-runnerd" || decoded.RedirectURL != "http://127.0.0.1:43111/manifest/callback" || !decoded.Public || decoded.Hook.Active || decoded.Hook.URL != "https://example.invalid/gh-runnerd-g02-unused" || decoded.Permissions["organization_self_hosted_runners"] != "write" || decoded.Permissions["metadata"] != "read" || len(decoded.Permissions) != 2 || decoded.Events == nil || len(decoded.Events) != 0 || decoded.OAuth {
		t.Fatalf("incorrect disabled-webhook loopback Manifest: %+v", decoded)
	}
}

func TestEncodeManifestRejectsInvalidName(t *testing.T) {
	redirect := "http://127.0.0.1:43111/manifest/callback"
	for _, name := range []string{"", "Synthetic App", "../app", "UPPER"} {
		if _, err := EncodeManifest(name, redirect); err == nil {
			t.Errorf("accepted invalid Manifest name %q", name)
		}
	}
}
