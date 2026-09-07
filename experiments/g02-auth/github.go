package enrollment

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

const maxResponseBytes = 1 << 20

// GitHubAPI is a deliberately small github.com-only evidence adapter. Supplying
// a transport is only for synthetic tests; production must preserve TLS checks.
type GitHubAPI struct {
	client *http.Client
	now    func() time.Time
}

func NewGitHubAPI(now func() time.Time, transport http.RoundTripper) *GitHubAPI {
	if now == nil {
		now = time.Now
	}
	return &GitHubAPI{now: now, client: &http.Client{Timeout: 10 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (a *GitHubAPI) App(ctx context.Context, c Credential) (int64, error) {
	var result struct {
		ID int64 `json:"id"`
	}
	err := a.call(ctx, http.MethodGet, "/app", &c, http.StatusOK, &result)
	if err != nil || result.ID < 1 {
		return 0, errors.New("App lookup failed")
	}
	return result.ID, nil
}
func (a *GitHubAPI) OrganizationInstallation(ctx context.Context, c Credential, org string) (Installation, error) {
	if !organizationLogin.MatchString(org) {
		return Installation{}, errors.New("invalid organization")
	}
	var result struct {
		ID         int64  `json:"id"`
		AppID      int64  `json:"app_id"`
		TargetID   int64  `json:"target_id"`
		TargetType string `json:"target_type"`
		Account    struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
		Permissions map[string]string `json:"permissions"`
		SuspendedAt *string           `json:"suspended_at"`
	}
	if err := a.call(ctx, http.MethodGet, "/orgs/"+org+"/installation", &c, http.StatusOK, &result); err != nil {
		return Installation{}, errors.New("installation lookup failed")
	}
	return Installation{ID: result.ID, AppID: result.AppID, AccountID: result.Account.ID, Login: result.Account.Login, AccountType: result.Account.Type, TargetID: result.TargetID, TargetType: result.TargetType, Permissions: result.Permissions, Suspended: result.SuspendedAt != nil}, nil
}

// Convert exchanges a code once. It does not retry, create additional Apps, or
// persist the response. Unneeded OAuth/webhook secrets are intentionally omitted.
func (a *GitHubAPI) Convert(ctx context.Context, code string) (Candidate, error) {
	if !validCode(code) {
		return Candidate{}, errors.New("invalid conversion code")
	}
	var result struct {
		ID  int64  `json:"id"`
		PEM string `json:"pem"`
	}
	if err := a.call(ctx, http.MethodPost, "/app-manifests/"+code+"/conversions", nil, http.StatusCreated, &result); err != nil {
		return Candidate{}, errors.New("Manifest conversion failed")
	}
	c := Candidate{AppID: result.ID, PEM: []byte(result.PEM)}
	if _, err := parseCredential(c); err != nil {
		return Candidate{}, errors.New("invalid Manifest credential")
	}
	return c, nil
}

func (a *GitHubAPI) call(ctx context.Context, method, path string, c *Credential, status int, out any) error {
	fail := errors.New("GitHub request failed")
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, nil)
	if err != nil {
		return fail
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "gh-runnerd-g02-evidence")
	if c != nil {
		jwt, err := a.jwt(*c)
		if err != nil {
			return fail
		}
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	res, err := a.client.Do(req)
	if err != nil {
		return fail
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		return fail
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return fail
	}
	if json.Unmarshal(body, out) != nil {
		return fail
	}
	return nil
}

func (a *GitHubAPI) jwt(c Credential) (string, error) {
	if c.key == nil || c.AppID < 1 {
		return "", errors.New("invalid App credential")
	}
	now := a.now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}{strconv.FormatInt(c.AppID, 10), now.Add(-time.Minute).Unix(), now.Add(9 * time.Minute).Unix()})
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	hash := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, hash[:])
	if err != nil {
		return "", errors.New("App signing failed")
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (a *GitHubAPI) DescribeApp(ctx context.Context, c Credential) (AppIdentity, error) {
	var result struct {
		ID    int64  `json:"id"`
		Slug  string `json:"slug"`
		Owner struct {
			Login string `json:"login"`
			ID    int64  `json:"id"`
			Type  string `json:"type"`
		} `json:"owner"`
	}
	if a.call(ctx, http.MethodGet, "/app", &c, http.StatusOK, &result) != nil || result.ID < 1 || result.Owner.ID < 1 || result.Slug == "" || result.Owner.Login == "" {
		return AppIdentity{}, errors.New("App identity lookup failed")
	}
	return AppIdentity{ID: result.ID, Slug: result.Slug, OwnerID: result.Owner.ID, OwnerLogin: result.Owner.Login, OwnerType: result.Owner.Type}, nil
}
