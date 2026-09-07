package enrollment

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrokerFoldedDuplicateAuthorityFieldsRefuse(t *testing.T) {
	for _, data := range []string{
		`{"app_id":71,"APP_ID":99}`,
		`{"phase":"inspect","phaſe":"cleanup"}`,
		`{"phase":"inspect","PHASE":"cleanup"}`,
	} {
		var approval BrokerApproval
		if decodeBrokerJSON([]byte(data), &approval, true) == nil {
			t.Fatal("case-fold duplicate changed interpreted authority")
		}
	}
	var approval BrokerApproval
	if decodeBrokerJSON([]byte(`{"APP_ID":71,"phaſe":"inspect"}`), &approval, true) != nil || approval.AppID != 71 || approval.Phase != "inspect" {
		t.Fatal("single decoder-equivalent field alias refused")
	}
}

func TestBrokerCanonicalOrganizationRequiredBeforeMint(t *testing.T) {
	a, c, _, f, root := newBrokerFixture(t)
	api := newBrokerAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) {
		response, err := f.RoundTrip(r)
		if err != nil {
			return response, err
		}
		data, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		response.Body = io.NopCloser(strings.NewReader(strings.ReplaceAll(string(data), "org-a", "ORG-A")))
		return response, nil
	}))
	api.admissionDirectory = func() (string, error) { return f.admissionRoot, nil }
	_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil)
	if err == nil || f.tokenCalls != 0 {
		t.Fatalf("canonical mismatch consumed issuance: stopped=%t mints=%d", err != nil, f.tokenCalls)
	}
}

func TestBrokerRepositoryDotComponentsRefuseWithoutAPI(t *testing.T) {
	for _, repository := range []string{".", ".."} {
		a, c, api, f, root := newBrokerFixture(t)
		a.Repository = repository
		_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil)
		if err == nil || len(f.calls) != 0 || f.tokenCalls != 0 {
			t.Fatal("dot component reached API")
		}
	}
}

func TestBrokerDifferentAttemptCannotRepeatMint(t *testing.T) {
	a, c, api, f, root := newBrokerFixture(t)
	for i := 0; i < 2; i++ {
		if i == 1 {
			root = filepath.Join(filepath.Dir(root), "other-attempt")
			f.root = root
			a.ExpiresAt = a.ExpiresAt.Add(time.Minute)
		}
		_, err := brokerExecute(context.Background(), a, brokerInput{PEM: string(c.PEM)}, root, api, nil)
		if i == 0 && err != nil {
			t.Fatal("first discovery refused")
		}
		if i == 1 && err == nil {
			t.Error("same discovery repeated in another directory")
		}
	}
	if f.tokenCalls != 1 {
		t.Fatalf("mint requests=%d; want one across directories/expiry", f.tokenCalls)
	}
}
