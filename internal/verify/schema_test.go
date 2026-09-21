package verify

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"testing"
)

// Two published schemas describe a validation step, and both list the
// statuses one may carry. Each list was written by hand beside the
// constants, with nothing comparing them, which is the drift this
// project keeps finding: a status added here and not there makes valid
// output fail validation for every adapter reading the contract, and one
// listed there and not here is a promise nothing keeps.
//
// evidence.schema.json describes the same step, because evidence copies
// the status straight out of a step result.
func statusConstants(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "verify.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	out := map[string]bool{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, s := range g.Specs {
			v, ok := s.(*ast.ValueSpec)
			if !ok || len(v.Names) != 1 || len(v.Values) != 1 {
				continue
			}
			if !isPrefixed(v.Names[0].Name, "Status") {
				continue
			}
			lit, ok := v.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			out[lit.Value[1:len(lit.Value)-1]] = true
		}
	}
	if len(out) == 0 {
		t.Fatal("no status constant was found, so this test checks nothing")
	}
	return out
}

func isPrefixed(name, prefix string) bool {
	return len(name) > len(prefix) && name[:len(prefix)] == prefix
}

func publishedStepStatuses(t *testing.T, schema string) map[string]bool {
	t.Helper()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "schemas", schema))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Defs struct {
			Step struct {
				Properties struct {
					Status struct {
						Enum []string `json:"enum"`
					} `json:"status"`
				} `json:"properties"`
			} `json:"step"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, n := range doc.Defs.Step.Properties.Status.Enum {
		out[n] = true
	}
	if len(out) == 0 {
		t.Fatalf("%s publishes no step status, so this test checks nothing", schema)
	}
	return out
}

func TestThePublishedStepStatusesAreTheOnesAStepCanCarry(t *testing.T) {
	code := statusConstants(t)
	for _, schema := range []string{"verify.schema.json", "evidence.schema.json"} {
		published := publishedStepStatuses(t, schema)
		for name := range code {
			if !published[name] {
				t.Errorf("a step can be %q and %s refuses it, so valid output would fail validation", name, schema)
			}
		}
		for name := range published {
			if !code[name] {
				t.Errorf("%s advertises the status %q, which no step can carry", schema, name)
			}
		}
	}
}
