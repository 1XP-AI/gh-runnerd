package enrollment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type workflowErrorBody struct {
	io.Reader
	readErr, closeErr bool
	closed            bool
}

func (b *workflowErrorBody) Read(p []byte) (int, error) {
	if b.readErr {
		return 0, errBroker
	}
	return b.Reader.Read(p)
}
func (b *workflowErrorBody) Close() error {
	b.closed = true
	if b.closeErr {
		return errBroker
	}
	return nil
}

func TestWorkflowTransportRejectsUpstreamCloseError(t *testing.T) {
	body := &workflowErrorBody{Reader: strings.NewReader(`{"id":7}`), closeErr: true}
	api := newBrokerAPI(time.Now, transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
	}))
	var out any
	if api.call(context.Background(), "GET", "/repos/fixture-org/canary/actions/runs/7", "Bearer synthetic-token", nil, 200, &out) == nil {
		t.Fatal("accepted upstream close failure")
	}
	if !body.closed {
		t.Fatal("upstream body not closed")
	}
}

func TestWorkflowObserverRejectsUntrustedResponses(t *testing.T) {
	grant := workflowGrantFixture()
	good := workflowRunJSON(t, grant)
	cases := []struct {
		name                                      string
		change                                    func(string) string
		status                                    int
		encoding                                  string
		readErr, closeErr, oversizeLength, cancel bool
	}{
		{name: "read failure", readErr: true}, {name: "close failure", closeErr: true},
		{name: "compressed", encoding: "gzip"}, {name: "large declared size", oversizeLength: true},
		{name: "large actual size", change: func(string) string { return strings.Repeat(" ", maxResponseBytes+1) }},
		{name: "redirect", status: 302}, {name: "forbidden", status: 403}, {name: "trailing JSON", change: func(s string) string { return s + `{}` }},
		{name: "duplicate identity", change: func(s string) string { return strings.Replace(s, `"id":7`, `"id":7,"id":7`, 1) }},
		{name: "folded identity", change: func(s string) string {
			return strings.Replace(s, `"head_sha":`, `"HEAD_SHA":"ambiguous","head_sha":`, 1)
		}},
		{name: "folded nested identity", change: func(s string) string { return strings.Replace(s, `"id":101`, `"ID":101,"id":101`, 1) }},
		{name: "cancel on response", cancel: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := &workflowErrorBody{Reader: strings.NewReader(good), readErr: tc.readErr, closeErr: tc.closeErr}
			if tc.change != nil {
				body.Reader = strings.NewReader(tc.change(good))
			}
			calls := 0
			observer := newWorkflowRunObserver(transportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.cancel {
					cancel()
				}
				status := tc.status
				if status == 0 {
					status = 200
				}
				r := &http.Response{StatusCode: status, Header: make(http.Header), Body: body}
				r.Header.Set("Content-Encoding", tc.encoding)
				if status == 302 {
					r.Header.Set("Location", "https://attacker.example/redirect")
				}
				if tc.oversizeLength {
					r.ContentLength = maxResponseBytes + 1
				}
				return r, nil
			}))
			if _, err := observer.observe(ctx, grant, "synthetic-observation-token"); err == nil {
				t.Fatal("accepted unsafe response")
			}
			if calls != 1 || !body.closed {
				t.Fatal("request retried or upstream body not closed")
			}
		})
	}
}

func TestWorkflowObserverRequiresExactRunAndPrivateNumericIdentities(t *testing.T) {
	grant := workflowGrantFixture()
	good := workflowRunJSON(t, grant)
	mutations := map[string]func(map[string]any){
		"run": func(r map[string]any) { r["id"] = 8 }, "commit": func(r map[string]any) { r["head_sha"] = strings.Repeat("d", 40) },
		"branch": func(r map[string]any) { r["head_branch"] = "other" }, "path": func(r map[string]any) { r["path"] = ".github/workflows/other.yml" },
		"event": func(r map[string]any) { r["event"] = "push" }, "attempt": func(r map[string]any) { r["run_attempt"] = 2 },
	}
	for _, repoField := range []string{"repository", "head_repository"} {
		for _, field := range []string{"id", "full_name", "private", "fork", "owner.id", "owner.login", "missing fork", "missing private"} {
			mutations[repoField+"/"+field] = func(r map[string]any) {
				repo := r[repoField].(map[string]any)
				switch field {
				case "id":
					repo[field] = 502
				case "full_name":
					repo[field] = "other/canary"
				case "private":
					repo[field] = false
				case "fork":
					repo[field] = true
				case "owner.id":
					repo["owner"].(map[string]any)["id"] = 102
				case "owner.login":
					repo["owner"].(map[string]any)["login"] = "other"
				case "missing fork":
					delete(repo, "fork")
				case "missing private":
					delete(repo, "private")
				}
			}
		}
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var run map[string]any
			if json.Unmarshal([]byte(good), &run) != nil {
				t.Fatal("fixture")
			}
			mutate(run)
			b, _ := json.Marshal(run)
			observer := newWorkflowRunObserver(transportFunc(func(*http.Request) (*http.Response, error) { return response(200, string(b)), nil }))
			if _, err := observer.observe(context.Background(), grant, "synthetic-observation-token"); err == nil {
				t.Fatal("accepted mismatched upstream identity")
			}
		})
	}
}

func TestWorkflowObserverRejectsAlreadyDecompressedResponse(t *testing.T) {
	grant := workflowGrantFixture()
	observer := newWorkflowRunObserver(transportFunc(func(*http.Request) (*http.Response, error) {
		res := response(200, workflowRunJSON(t, grant))
		res.Uncompressed = true
		return res, nil
	}))
	if _, err := observer.observe(context.Background(), grant, "synthetic-observation-token"); err == nil {
		t.Fatal("accepted a response marked as already decompressed")
	}
}
