// g02-synthetic demonstrates manual import with generated keys and an in-memory
// fake API/storage sink. It has no live mode and never opens a network connection.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"

	enrollment "github.com/1XP-AI/gh-runnerd/experiments/g02-auth"
)

type syntheticAPI struct{}

func (syntheticAPI) App(context.Context, enrollment.Credential) (int64, error) { return 71, nil }
func (syntheticAPI) OrganizationInstallation(_ context.Context, _ enrollment.Credential, org string) (enrollment.Installation, error) {
	id, installation := int64(101), int64(201)
	if org == "g02-synthetic-b" {
		id, installation = 102, 202
	}
	return enrollment.Installation{ID: installation, AppID: 71, AccountID: id, TargetID: id, Login: org, AccountType: "Organization", TargetType: "Organization", Permissions: map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"}}, nil
}
func run() error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("synthetic key generation failed")
	}
	candidate := enrollment.Candidate{AppID: 71, PEM: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), Organizations: []enrollment.Binding{
		{Login: "g02-synthetic-a", OrganizationID: 101, InstallationID: 201},
		{Login: "g02-synthetic-b", OrganizationID: 102, InstallationID: 202},
	}}
	// Keep the single commit in memory. The process exits without persisting keys.
	stored := false
	_, err = enrollment.ManualImport(context.Background(), candidate, syntheticAPI{}, func(context.Context, enrollment.Credential, []enrollment.Binding) error { stored = true; return nil })
	if err != nil {
		return fmt.Errorf("synthetic import failed")
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Profile       string `json:"profile"`
		Organizations int    `json:"verified_organizations"`
		Committed     bool   `json:"in_memory_commit"`
		Live          bool   `json:"live_github"`
	}{"synthetic", 2, stored, false})
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
