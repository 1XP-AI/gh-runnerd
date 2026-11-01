package enrollment

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/1XP-AI/gh-runnerd/experiments/g02-auth/handoff"
)

func TestFinalReusableWorkflowVerification(t *testing.T) {
	grant := workflowGrantFixture()
	grant.Request.Version = 2
	grant.Request.Source = handoff.ReusableCalleeSource
	grant.Request.Callee = handoff.CalleeIdentity{Repository: grant.Request.Repository, WorkflowPath: ".github/workflows/reusable.yml", SHA: strings.Repeat("d", 40)}
	a := brokerApprovalFixture()
	c := controllerApproval{WorkflowRunID: grant.Request.WorkflowRunID, WorkflowSHA: grant.Request.WorkflowSHA, WorkflowRef: grant.Request.WorkflowRef, WorkflowPath: grant.Request.WorkflowPath}
	entry := `{"path":"` + grant.Request.Repository + `/` + grant.Request.Callee.WorkflowPath + `@` + grant.Request.Callee.SHA + `","sha":"` + grant.Request.Callee.SHA + `"}`
	ref := `"referenced_workflows":[` + entry + `]`
	good := strings.TrimSuffix(workflowRunJSON(t, grant), "}") + `,` + ref + `}`
	cases := map[string]string{
		"correct":           good,
		"changed callee":    strings.Replace(good, "reusable.yml", "changed.yml", 1),
		"missing":           strings.Replace(good, ","+ref, "", 1),
		"null":              strings.Replace(good, ref, `"referenced_workflows":null`, 1),
		"extra":             strings.Replace(good, ref, `"referenced_workflows":[`+entry+`,`+entry+`]`, 1),
		"casefold":          strings.Replace(good, `"referenced_workflows"`, `"REFERENCED_WORKFLOWS"`, 1),
		"duplicate":         strings.Replace(good, ref, ref+`,`+ref, 1),
		"wrong source":      strings.Replace(good, grant.Request.Repository+`/`+grant.Request.Callee.WorkflowPath, "other/repo/"+grant.Request.Callee.WorkflowPath, 1),
		"wrong sha":         strings.Replace(good, grant.Request.Callee.SHA, strings.Repeat("e", 40), 1),
		"missing authority": strings.Replace(good, `"private":true`, `"private":null`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			api := newBrokerAPI(time.Now, transportFunc(func(r *http.Request) (*http.Response, error) { calls++; return response(200, body), nil }))
			err := api.verifyFinalReusableWorkflow(context.Background(), a, c, "synthetic-observation-token", grant)
			if (err == nil) != (name == "correct") || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
	t.Run("invalid grant no read", func(t *testing.T) {
		calls := 0
		api := newBrokerAPI(time.Now, transportFunc(func(*http.Request) (*http.Response, error) { calls++; return response(200, good), nil }))
		bad := grant
		bad.Request.Source = "wrong-source"
		if api.verifyFinalReusableWorkflow(context.Background(), a, c, "synthetic-observation-token", bad) == nil || calls != 0 {
			t.Fatal("invalid grant reached transport")
		}
	})
}

func TestFinalReusableWorkflowChangedAfterObservation(t *testing.T) {
	grant := workflowGrantFixture()
	grant.Request.Version = 2
	grant.Request.Source = handoff.ReusableCalleeSource
	grant.Request.Callee = handoff.CalleeIdentity{Repository: grant.Request.Repository, WorkflowPath: ".github/workflows/reusable.yml", SHA: strings.Repeat("d", 40)}
	good := strings.TrimSuffix(workflowRunJSON(t, grant), "}") + `,"referenced_workflows":[{"path":"` + grant.Request.Repository + `/` + grant.Request.Callee.WorkflowPath + `@` + grant.Request.Callee.SHA + `","sha":"` + grant.Request.Callee.SHA + `"}]}`
	calls := 0
	transport := transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(200, good), nil
		}
		return response(200, strings.Replace(good, "reusable.yml", "changed.yml", 1)), nil
	})
	api := newBrokerAPI(time.Now, transport)
	observer := workflowRunObserver{client: api.client}
	if _, err := observer.observe(context.Background(), grant, "synthetic-observation-token"); err != nil {
		t.Fatal(err)
	}
	c := controllerApproval{WorkflowRunID: grant.Request.WorkflowRunID, WorkflowSHA: grant.Request.WorkflowSHA, WorkflowRef: grant.Request.WorkflowRef, WorkflowPath: grant.Request.WorkflowPath}
	if api.verifyFinalReusableWorkflow(context.Background(), brokerApprovalFixture(), c, "synthetic-observation-token", grant) == nil || calls != 2 {
		t.Fatalf("changed callee accepted or wrong request count: %d", calls)
	}
}

func TestFinalReusableWorkflowFailureAndNoRead(t *testing.T) {
	grant := workflowGrantFixture()
	grant.Request.Version = 2
	grant.Request.Source = handoff.ReusableCalleeSource
	grant.Request.Callee = handoff.CalleeIdentity{Repository: grant.Request.Repository, WorkflowPath: ".github/workflows/reusable.yml", SHA: strings.Repeat("d", 40)}
	c := controllerApproval{WorkflowRunID: grant.Request.WorkflowRunID, WorkflowSHA: grant.Request.WorkflowSHA, WorkflowRef: grant.Request.WorkflowRef, WorkflowPath: grant.Request.WorkflowPath}
	for _, tc := range []struct {
		name    string
		status  int
		invalid bool
	}{{"http failure", 403, false}, {"mismatched controller", 200, true}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			api := newBrokerAPI(time.Now, transportFunc(func(*http.Request) (*http.Response, error) { calls++; return response(tc.status, `{}`), nil }))
			controller := c
			if tc.invalid {
				controller.WorkflowSHA = strings.Repeat("e", 40)
			}
			if api.verifyFinalReusableWorkflow(context.Background(), brokerApprovalFixture(), controller, "synthetic-observation-token", grant) == nil {
				t.Fatal("accepted")
			}
			want := 1
			if tc.invalid {
				want = 0
			}
			if calls != want {
				t.Fatalf("calls=%d want=%d", calls, want)
			}
		})
	}
}
