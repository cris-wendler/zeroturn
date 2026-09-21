package policy

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"testing"
)

// The two published schemas refuse any trigger name they do not list, so
// those lists are part of the contract an adapter reads. They were
// written by hand beside the code that produces the names, which is the
// shape of drift this project keeps finding: the same list in
// policy-tune.schema.json had already gained a name that nothing could
// emit.
//
// The names come out of the source instead. Every trigger this package
// publishes is built as a Trigger composite literal whose first element
// is the name, and every unmeasured entry takes its name from the
// measurements table.
func triggerNamesInTheCode(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "policy.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "Trigger" {
			return true
		}
		if len(lit.Elts) == 0 {
			return true
		}
		s, ok := lit.Elts[0].(*ast.BasicLit)
		if !ok || s.Kind != token.STRING {
			return true
		}
		names[s.Value[1:len(s.Value)-1]] = true
		return true
	})
	for _, m := range measurements {
		names[m.Trigger] = true
	}
	if len(names) == 0 {
		t.Fatal("no trigger name was found in the source, so this test checks nothing")
	}
	return names
}

// publishedNames reads one enum out of one schema, by the definition and
// property that hold it.
func publishedNames(t *testing.T, schema, def string) map[string]bool {
	t.Helper()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "schemas", schema))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Defs map[string]struct {
			Properties struct {
				Name struct {
					Enum []string `json:"enum"`
				} `json:"name"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, n := range doc.Defs[def].Properties.Name.Enum {
		out[n] = true
	}
	if len(out) == 0 {
		t.Fatalf("%s publishes no names under %s, so this test checks nothing", schema, def)
	}
	return out
}

// Both directions. A name the code emits and a schema refuses makes
// valid output fail validation for every adapter reading the contract. A
// name a schema advertises and the code cannot produce is a promise
// nothing keeps.
func TestThePublishedTriggerNamesAreTheOnesTheCodeCanEmit(t *testing.T) {
	code := triggerNamesInTheCode(t)
	for _, schema := range []string{"status.schema.json", "policy-check.schema.json"} {
		published := publishedNames(t, schema, "trigger")
		for name := range code {
			if !published[name] {
				t.Errorf("the gate can report %q and %s refuses it, so valid output would fail validation", name, schema)
			}
		}
		for name := range published {
			if !code[name] {
				t.Errorf("%s advertises %q and the gate cannot produce it, so an adapter would handle a value that never arrives", schema, name)
			}
		}
	}
}

// The unmeasured list is narrower than the trigger list on purpose: only
// a measurement that arrives through the status line can be absent, and
// subagent counts are maintained here so zero means zero.
func TestThePublishedUnmeasuredNamesAreTheMeasurementsThatCanBeAbsent(t *testing.T) {
	code := map[string]bool{}
	for _, m := range measurements {
		code[m.Trigger] = true
	}
	for _, schema := range []string{"status.schema.json", "policy-check.schema.json"} {
		published := publishedNames(t, schema, "unmeasured")
		for name := range code {
			if !published[name] {
				t.Errorf("%q can be reported as unmeasured and %s refuses it", name, schema)
			}
		}
		for name := range published {
			if !code[name] {
				t.Errorf("%s advertises %q as unmeasured and no measurement carries that name", schema, name)
			}
		}
	}
}
