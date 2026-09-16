package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestPlanWorkerLaunchKeepsManagementCredentialsOutOfWorkerSurfaces(t *testing.T) {
	canary := "fixture-private-key-env-canary"
	t.Setenv("GITHUB_APP_PRIVATE_KEY", canary)
	t.Setenv("GH_RUNNERD_CANARY", canary)

	session, err := PrepareForeground(context.Background(), fixtureDocument(), fixtureSource(t), fixtureAPIValue(), nil)
	if err != nil {
		t.Fatalf("foreground preparation rejected: %v", err)
	}
	plan, err := PlanWorkerLaunch(WorkerLaunchRequest{Binding: session.Binding})
	if err != nil {
		t.Fatalf("worker launch plan rejected: %v", err)
	}
	if plan.AppID != session.Binding.AppID || plan.Organization != session.Binding.Organization || plan.InstallationID != session.Binding.InstallationID || plan.Repository != session.Binding.Repository {
		t.Fatalf("worker plan dropped binding metadata: plan=%+v binding=%+v", plan, session.Binding)
	}
	if !plan.hasValidatedConstruction() || plan.hasJIT || len(plan.env) != 0 || len(plan.argv) != 0 || len(plan.files) != 0 {
		t.Fatalf("worker plan carried launch material: env=%d argv=%d files=%d jit=%t", len(plan.env), len(plan.argv), len(plan.files), plan.hasJIT)
	}
	joined := strings.Join(append(append([]string{}, plan.env...), plan.argv...), "\x00")
	if strings.Contains(joined, canary) || strings.Contains(joined, "PRIVATE KEY") {
		t.Fatal("worker plan inherited process environment or credential material")
	}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, plan), "PRIVATE KEY") || strings.Contains(fmt.Sprintf(format, plan), canary) {
			t.Fatalf("worker plan formatting exposed credential material for %s", format)
		}
	}
	data, err := json.Marshal(plan)
	if err != nil || strings.Contains(string(data), "PRIVATE KEY") || strings.Contains(string(data), canary) {
		t.Fatal("worker plan JSON exposed credential material")
	}
}

func TestPlanWorkerLaunchRejectsManagementCredentialAndJITMaterial(t *testing.T) {
	session, err := PrepareForeground(context.Background(), fixtureDocument(), fixtureSource(t), fixtureAPIValue(), nil)
	if err != nil {
		t.Fatalf("foreground preparation rejected: %v", err)
	}
	pemBytes := fixturePEM(t)
	for _, tc := range []struct {
		name string
		req  WorkerLaunchRequest
		want error
	}{
		{name: "pem environment", req: WorkerLaunchRequest{Binding: session.Binding, ExtraEnv: []string{"GITHUB_APP_PRIVATE_KEY=" + string(pemBytes)}}, want: ErrWorkerCredential},
		{name: "pem argv", req: WorkerLaunchRequest{Binding: session.Binding, ExtraArgv: []string{"--app-private-key", string(pemBytes)}}, want: ErrWorkerCredential},
		{name: "pem file", req: WorkerLaunchRequest{Binding: session.Binding, Files: map[string][]byte{"app.pem": pemBytes}}, want: ErrWorkerCredential},
		{name: "named credential file", req: WorkerLaunchRequest{Binding: session.Binding, Files: map[string][]byte{"installation.token": []byte("fixture-token")}}, want: ErrWorkerCredential},
		{name: "jit envelope", req: WorkerLaunchRequest{Binding: session.Binding, JIT: []byte("synthetic-jit-envelope")}, want: ErrJIT},
		{name: "management key as jit", req: WorkerLaunchRequest{Binding: session.Binding, JIT: pemBytes}, want: ErrJIT},
		{name: "zero binding", req: WorkerLaunchRequest{}, want: ErrConfig},
		{name: "forged binding", req: WorkerLaunchRequest{Binding: forgedValidatedBinding()}, want: ErrConfig},
		{name: "mutated binding", req: WorkerLaunchRequest{Binding: mutatedValidatedBinding(session.Binding)}, want: ErrConfig},
		{name: "forged binding with jit", req: WorkerLaunchRequest{Binding: forgedValidatedBinding(), JIT: pemBytes}, want: ErrConfig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanWorkerLaunch(tc.req)
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(plan, WorkerLaunchPlan{}) {
				t.Fatalf("worker credential boundary accepted unsafe launch material: plan=%+v err=%v", plan, err)
			}
			if err != nil && (strings.Contains(err.Error(), "PRIVATE KEY") || strings.Contains(err.Error(), "synthetic-jit-envelope")) {
				t.Fatal("worker launch error leaked credential material")
			}
		})
	}
}

func TestPlanWorkerLaunchRejectsDirectlyConstructedLaunchMaterial(t *testing.T) {
	session, err := PrepareForeground(context.Background(), fixtureDocument(), fixtureSource(t), fixtureAPIValue(), nil)
	if err != nil {
		t.Fatalf("foreground preparation rejected: %v", err)
	}
	plan, err := PlanWorkerLaunch(WorkerLaunchRequest{Binding: session.Binding})
	if err != nil {
		t.Fatalf("validated worker launch plan rejected: %v", err)
	}
	if !plan.hasValidatedConstruction() {
		t.Fatalf("plan missing validated construction: %+v", plan)
	}

	forged := WorkerLaunchPlan{
		AppID:          session.Binding.AppID,
		Organization:   session.Binding.Organization,
		InstallationID: session.Binding.InstallationID,
		Repository:     session.Binding.Repository,
		env:            []string{"GITHUB_APP_PRIVATE_KEY=fixture-private-key"},
		argv:           []string{"--app-private-key", "fixture-private-key"},
		files:          map[string][]byte{"app.pem": []byte("fixture-private-key")},
		hasJIT:         true,
	}
	if forged.hasValidatedConstruction() {
		t.Fatal("directly constructed launch plan was treated as validated")
	}
	mutated := plan
	mutated.AppID = 999
	if mutated.hasValidatedConstruction() {
		t.Fatal("mutated worker launch plan retained validated construction")
	}
}

func forgedValidatedBinding() ValidatedBinding {
	config := fixtureConfig()
	return ValidatedBinding{
		AppID:          config.AppID,
		Organization:   config.Organization,
		InstallationID: config.InstallationID,
		Repository:     config.Repository,
		Permissions:    map[string]string{"organization_self_hosted_runners": "write", "metadata": "read"},
	}
}

func mutatedValidatedBinding(binding ValidatedBinding) ValidatedBinding {
	binding.AppID = 999
	return binding
}
