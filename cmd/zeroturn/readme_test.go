package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/capabilities"
)

// A command is added in one place and listed in three others: the help
// text at the terminal, the table in the README, and the capabilities
// document an adapter reads. All three are written by hand beside the
// thing they describe, so all three drift, and one of them was already
// wrong when these tests were written. They read the dispatch itself and
// compare, which is the only version of the list that cannot be wrong.

// dispatched reports the command names main actually answers, read out of
// the switch in main.go. Aliases that start with a dash, and help, are
// left out: they are ways of asking rather than commands to document.
func dispatched(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		id, ok := sw.Tag.(*ast.Ident)
		if !ok || id.Name != "cmd" {
			return true
		}
		for _, stmt := range sw.Body.List {
			clause, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range clause.List {
				lit, ok := expr.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				name, err := strconv.Unquote(lit.Value)
				if err != nil || strings.HasPrefix(name, "-") || name == "help" {
					continue
				}
				found = append(found, name)
			}
		}
		return true
	})

	if len(found) == 0 {
		t.Fatal("no command switch was found in main.go, so these tests check nothing")
	}
	return found
}

// subcommands reports the names one command answers, read out of its own
// switch. policy has six of them and they are listed by hand in three
// places, the same shape of drift as the commands above.
func subcommands(t *testing.T, file, tag string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		// The subcommand is read from the first argument, which is what
		// distinguishes this switch from the ones inside the subcommands.
		idx, ok := sw.Tag.(*ast.IndexExpr)
		if !ok {
			return true
		}
		if id, ok := idx.X.(*ast.Ident); !ok || id.Name != tag {
			return true
		}
		for _, stmt := range sw.Body.List {
			clause, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range clause.List {
				lit, ok := expr.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				name, err := strconv.Unquote(lit.Value)
				if err != nil || strings.HasPrefix(name, "-") || name == "help" {
					continue
				}
				found = append(found, name)
			}
		}
		return true
	})

	if len(found) == 0 {
		t.Fatalf("no subcommand switch was found in %s, so this test checks nothing", file)
	}
	return found
}

// policy is the one command with subcommands, and they are written out in
// its own usage text and in the README as well as in its switch.
func TestEveryPolicySubcommandIsDescribed(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile("(?m)^\\| `zeroturn policy ([^`]+)`").FindStringSubmatch(string(b))
	if row == nil {
		t.Fatal("the README has no zeroturn policy row, so this test checks nothing")
	}

	for _, name := range subcommands(t, "cmd_policy.go", "args") {
		if !regexp.MustCompile(`(?m)^  ` + name + ` `).MatchString(policyUsage) {
			t.Errorf("zeroturn policy answers %q, and its help text does not list it", name)
		}
		if !strings.Contains(row[1], name) {
			t.Errorf("zeroturn policy answers %q, and the README row does not name it: %s", name, row[1])
		}
	}
}

// readmeCommands reports the commands the README table names, by the first
// word after zeroturn in each row.
func readmeCommands(t *testing.T) map[string]bool {
	t.Helper()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	rows := regexp.MustCompile("(?m)^\\| `zeroturn ([a-z]+)").FindAllStringSubmatch(string(b), -1)
	for _, m := range rows {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("the README has no command table, so these tests check nothing")
	}
	return out
}

func TestEveryCommandIsInTheHelpText(t *testing.T) {
	for _, name := range dispatched(t) {
		if !regexp.MustCompile(`(?m)^  ` + name + ` `).MatchString(usage) {
			t.Errorf("zeroturn answers %q, and the help text does not list it", name)
		}
	}
}

// The capabilities document is read by adapters rather than by people,
// which is why a command missing from it went unnoticed until this test
// was written. It is checked in both directions: a command that is not
// published cannot be discovered, and a published command that is gone
// sends an adapter at something that will fail.
func TestTheCapabilitiesDocumentPublishesEveryCommand(t *testing.T) {
	published := map[string]bool{}
	for _, name := range capabilities.Describe("test").Commands {
		published[name] = true
	}
	real := map[string]bool{}
	for _, name := range dispatched(t) {
		real[name] = true
		if !published[name] {
			t.Errorf("zeroturn answers %q, and capabilities does not publish it", name)
		}
	}
	for name := range published {
		if !real[name] {
			t.Errorf("capabilities publishes %q, which is not a command", name)
		}
	}
}

func TestEveryCommandIsInTheReadme(t *testing.T) {
	in := readmeCommands(t)
	for _, name := range dispatched(t) {
		if !in[name] {
			t.Errorf("zeroturn answers %q, and the README command table does not name it", name)
		}
	}
}

func TestTheReadmeNamesNoCommandThatIsGone(t *testing.T) {
	real := map[string]bool{}
	for _, name := range dispatched(t) {
		real[name] = true
	}
	for name := range readmeCommands(t) {
		if !real[name] {
			t.Errorf("the README names zeroturn %s, which is not a command", name)
		}
	}
}
