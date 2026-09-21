package evidence

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

// The words an assessment can carry are published in two schemas and
// written once here. Each list was kept by hand beside the constants
// with nothing comparing them, which is how the trigger names in three
// other schemas drifted.
func wordsWithPrefix(t *testing.T, prefix string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "evidence.go", nil, 0)
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
			if !strings.HasPrefix(v.Names[0].Name, prefix) {
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
		t.Fatalf("no constant named %s was found, so this test checks nothing", prefix)
	}
	return out
}

// enumAt reads one enum out of a schema by the path of keys that reaches
// it, so the three lists are read the same way rather than through three
// hand written structures.
func enumAt(t *testing.T, schema string, path ...string) map[string]bool {
	t.Helper()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "schemas", schema))
	if err != nil {
		t.Fatal(err)
	}
	var doc interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	at := doc
	for _, key := range path {
		m, ok := at.(map[string]interface{})
		if !ok {
			t.Fatalf("%s has nothing at %v, so this test checks nothing", schema, path)
		}
		at = m[key]
	}
	m, ok := at.(map[string]interface{})
	if !ok {
		t.Fatalf("%s has nothing at %v, so this test checks nothing", schema, path)
	}
	list, ok := m["enum"].([]interface{})
	if !ok || len(list) == 0 {
		t.Fatalf("%s publishes no enum at %v, so this test checks nothing", schema, path)
	}
	out := map[string]bool{}
	for _, v := range list {
		out[v.(string)] = true
	}
	return out
}

func compare(t *testing.T, what string, code, published map[string]bool) {
	t.Helper()
	for name := range code {
		if !published[name] {
			t.Errorf("%s can be %q and it is not published, so valid output would fail validation", what, name)
		}
	}
	for name := range published {
		if !code[name] {
			t.Errorf("%s is published as able to be %q, which the code cannot produce", what, name)
		}
	}
}

// A report says what the recorded run concluded and gives one word for a
// person. Both lists come from the constants.
func TestThePublishedAssessmentWordsAreTheOnesTheCodeCanReport(t *testing.T) {
	compare(t, "an assessment state", wordsWithPrefix(t, "State"),
		enumAt(t, "report.schema.json", "$defs", "assessment", "properties", "state"))
	compare(t, "an assessment result", wordsWithPrefix(t, "Result"),
		enumAt(t, "report.schema.json", "$defs", "assessment", "properties", "result"))
}

// The stored record is narrower than the assessment by exactly one word.
// never-run is not a result a record can hold, because it is what an
// assessment says when there is no record at all.
func TestThePublishedRecordResultsAreTheOnesThatCanBeStored(t *testing.T) {
	stored := wordsWithPrefix(t, "Result")
	if !stored[ResultNeverRun] {
		t.Fatal("never-run is no longer a result, so this test checks the wrong exception")
	}
	delete(stored, ResultNeverRun)
	compare(t, "a stored result", stored,
		enumAt(t, "evidence.schema.json", "properties", "result"))
}
