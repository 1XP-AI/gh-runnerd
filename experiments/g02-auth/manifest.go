package enrollment

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
)

const (
	manifestHomepage = "https://github.com/1XP-AI/gh-runnerd"
	manifestHookURL  = "https://example.invalid/gh-runnerd-g02-unused"
)

// EncodeManifest builds one GitHub App Manifest for a current-process loopback
// attempt. It requires an already-bound 127.0.0.1 redirect, disables webhook
// delivery, and omits OAuth callback/setup fields. A documented redirect_url is
// not live proof that GitHub accepts this HTTP loopback shape.
func EncodeManifest(name, redirectURL string) ([]byte, error) {
	invalid := errors.New("invalid Manifest")
	if !appSlug.MatchString(name) {
		return nil, invalid
	}
	u, err := url.Parse(redirectURL)
	if err != nil || u.Scheme != "http" || u.Path != callbackPath || (u.RawPath != "" && u.RawPath != callbackPath) || u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.Opaque != "" {
		return nil, invalid
	}
	ip, port, err := net.SplitHostPort(u.Host)
	n, convErr := strconv.Atoi(port)
	if err != nil || convErr != nil || ip != "127.0.0.1" || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return nil, invalid
	}
	payload, err := json.Marshal(map[string]any{
		"name":         name,
		"url":          manifestHomepage,
		"redirect_url": redirectURL,
		"public":       true,
		"hook_attributes": map[string]any{
			"active": false,
			"url":    manifestHookURL,
		},
		"default_permissions": map[string]string{
			"organization_self_hosted_runners": "write",
			"metadata":                         "read",
		},
		"default_events":           []string{},
		"request_oauth_on_install": false,
	})
	if err != nil {
		return nil, invalid
	}
	return payload, nil
}
