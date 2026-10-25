package enrollment

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPairedWorkerImageAdmissionMatchesCanonicalSelection(t *testing.T) {
	// G02 stays dependency-free. Read the adjacent canonical source instead of
	// importing the G01 module (which already depends on G02's handoff package).
	source, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "g01-scaleset", "liveworker", "worker.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var canonicalImage string
	for _, decl := range source.Decls {
		group, ok := decl.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		for _, spec := range group.Specs {
			value := spec.(*ast.ValueSpec)
			if len(value.Names) != 1 || value.Names[0].Name != "ImageReference" || len(value.Values) != 1 {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatal("canonical worker image must remain an explicit pinned string")
			}
			canonicalImage, err = strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if canonicalImage == "" {
		t.Fatal("canonical worker image pin missing")
	}
	for _, test := range []struct {
		name, image string
		accept      bool
	}{
		{"canonical worker manifest", canonicalImage, true},
		{"superseded 2.337.0 manifest", "ghcr.io/actions/actions-runner@sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d", false},
		{"mutable release tag", "ghcr.io/actions/actions-runner:2.338.0", false},
		{"multi-platform index", "ghcr.io/actions/actions-runner@sha256:4ffadc0002b2581327e06101fc8c06cd189232baf79fe561fac9caeb76f5e807", false},
		{"config instead of manifest", "ghcr.io/actions/actions-runner@sha256:5149aa9f00e2f5f0bbb0d5127cf31aea260e45e559d3d2f3d829623439660eae", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, c, controllerState, workerState, workerPath := pairedPlanInputs(t)
			data, err := os.ReadFile(workerPath)
			if err != nil {
				t.Fatal(err)
			}
			var worker pairedWorkerApproval
			if err := decodeBrokerJSON(data, &worker, true); err != nil {
				t.Fatal(err)
			}
			worker.Image = test.image
			data, err = json.Marshal(worker)
			if err != nil || os.WriteFile(workerPath, data, 0600) != nil {
				t.Fatal("worker approval rewrite")
			}
			plan, err := openBrokerWorkerPlan(workerPath, workerState, controllerState, a, c)
			if plan != nil {
				defer plan.close()
			}
			if (err == nil) != test.accept {
				t.Fatalf("broker image admission accepted=%v, want %v", err == nil, test.accept)
			}
		})
	}
}
