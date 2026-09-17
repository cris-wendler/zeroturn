package policy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/state"
)

// The list of measurements is a description kept beside the thing it
// describes, which is the shape of defect this project has now found
// seven times. So it is compared with the gate rather than maintained
// against it: every value EvaluateAt reads through a nil check has to be
// in the list, and everything in the list has to be read by EvaluateAt.
// Both directions, so neither a missing entry nor a stale one survives.
func TestTheUnmeasuredListMatchesWhatTheGateReads(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "policy.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var body *ast.FuncDecl
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "EvaluateAt" {
			body = fn
		}
	}
	if body == nil {
		t.Fatal("EvaluateAt was not found, so this test checks nothing")
	}

	// Every "s.Field != nil" in the gate is a value that may be absent.
	read := map[string]bool{}
	ast.Inspect(body.Body, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok || be.Op != token.NEQ {
			return true
		}
		if id, isIdent := be.Y.(*ast.Ident); !isIdent || id.Name != "nil" {
			return true
		}
		sel, ok := be.X.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if x, isIdent := sel.X.(*ast.Ident); !isIdent || x.Name != "s" {
			return true
		}
		read[sel.Sel.Name] = true
		return true
	})
	if len(read) == 0 {
		t.Fatal("the gate reads no optional value, so this test checks nothing")
	}

	listed := map[string]bool{}
	for _, m := range measurements {
		listed[m.Field] = true
	}

	for field := range read {
		if !listed[field] {
			t.Errorf("EvaluateAt reads %s through a nil check and Unmeasured does not name it, so a reader is told no threshold was crossed when it could not be checked", field)
		}
	}
	for field := range listed {
		if !read[field] {
			t.Errorf("Unmeasured names %s and EvaluateAt does not read it, so the sentence reports something the gate never looks at", field)
		}
	}
}

// Every entry has to describe itself to a reader, and two sharing a
// label would make the sentence ambiguous about which was absent.
func TestEveryMeasurementHasItsOwnLabel(t *testing.T) {
	seen := map[string]string{}
	for _, m := range measurements {
		if m.Label == "" {
			t.Errorf("%s has no label, so it cannot be named in output", m.Field)
		}
		if other, dup := seen[m.Label]; dup {
			t.Errorf("%s and %s share the label %q", other, m.Field, m.Label)
		}
		seen[m.Label] = m.Field
	}
}

// The case this exists for: hooks arrive, the status line does not, and
// the gate has nothing to read. The subagent counts are still real,
// because ZeroTurn maintains those itself.
func TestASessionWithNoStatusLineIsEntirelyUnmeasured(t *testing.T) {
	var s state.Session
	s.SubagentStarts = 3
	s.ActiveSubagents = 1

	if !AllUnmeasured(s) {
		t.Fatalf("a session with no status line values reported measurements: %v", Unmeasured(s))
	}
	if len(Unmeasured(s)) != len(measurements) {
		t.Errorf("Unmeasured named %d of %d values", len(Unmeasured(s)), len(measurements))
	}
}

// One value in hand is not the same as none, and saying "nothing was
// measured" then would be its own overstatement.
func TestAMeasuredValueIsNotReportedAsAbsent(t *testing.T) {
	var s state.Session
	ctx := 42.0
	s.ContextPct = &ctx

	if AllUnmeasured(s) {
		t.Fatal("a session carrying a context value was called entirely unmeasured")
	}
	for _, name := range Unmeasured(s) {
		if name == "context" {
			t.Error("context was measured and is named as absent")
		}
	}
	if len(Unmeasured(s)) != len(measurements)-1 {
		t.Errorf("one value was measured and %d of %d are named absent",
			len(Unmeasured(s)), len(measurements))
	}
}
