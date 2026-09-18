package tune

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"testing"
)

// The published schema refuses any trigger name it does not list, so the
// list is part of the contract an adapter reads. It was written by hand
// beside the code that produces the names, and it drifted: it advertised
// backgroundTasks, which this package cannot produce, so an adapter
// author would write handling for a value that can never arrive.
//
// measurement is the gate. A trigger becomes a threshold only when
// measurement answers for it, so the cases in that switch are the names
// this package can emit, and they are read from the source rather than
// repeated here.
func triggersTheCodeCanEmit(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "tune.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if d, ok := d.(*ast.FuncDecl); ok && d.Name.Name == "measurement" {
			fn = d
		}
	}
	if fn == nil {
		t.Fatal("measurement was not found, so this test checks nothing")
	}

	names := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		c, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, e := range c.List {
			lit, ok := e.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			names[lit.Value[1:len(lit.Value)-1]] = true
		}
		return true
	})
	if len(names) == 0 {
		t.Fatal("measurement names no trigger, so this test checks nothing")
	}
	return names
}

func publishedTriggers(t *testing.T) map[string]bool {
	t.Helper()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "schemas", "policy-tune.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Defs struct {
			Threshold struct {
				Properties struct {
					Trigger struct {
						Enum []string `json:"enum"`
					} `json:"trigger"`
				} `json:"properties"`
			} `json:"threshold"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, n := range doc.Defs.Threshold.Properties.Trigger.Enum {
		out[n] = true
	}
	if len(out) == 0 {
		t.Fatal("the schema publishes no trigger names, so this test checks nothing")
	}
	return out
}

// Both directions. A name the code emits and the schema refuses makes
// valid output fail validation for every adapter. A name the schema
// advertises and the code cannot produce is a promise nothing keeps.
func TestThePublishedTriggersAreTheOnesTheCodeCanEmit(t *testing.T) {
	code := triggersTheCodeCanEmit(t)
	published := publishedTriggers(t)

	for name := range code {
		if !published[name] {
			t.Errorf("tune can report %q and the schema refuses it, so valid output would fail validation", name)
		}
	}
	for name := range published {
		if !code[name] {
			t.Errorf("the schema advertises %q and tune cannot produce it, so an adapter would handle a value that never arrives", name)
		}
	}
}
