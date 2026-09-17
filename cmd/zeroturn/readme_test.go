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
	"github.com/cris-wendler/zeroturn/internal/snapshot"
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

// What a check does not see is the claim most worth holding to the code,
// because it is the one a reader relies on when deciding whether to
// trust the result. The list lived twice: once in internal/snapshot and
// once in the README, with nothing comparing them, which is the shape of
// six defects already recorded here.
//
// The package holds the sentences. This requires the README to carry
// each one word for word.
func TestTheReadmeStatesEveryLimitOfTheRepositoryDigest(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if len(snapshot.Limits) == 0 {
		t.Fatal("the package lists no limits, so this test checks nothing")
	}
	for _, limit := range snapshot.Limits {
		if !strings.Contains(text, limit) {
			t.Errorf("the README does not tell a reader that the digest does not cover:\n  %s", limit)
		}
	}
}

// A command that writes or deletes evidence has to say so in its own
// help. The README said all of it and the help text said none of it, and
// one of them was worse than silent: report's help described purge as
// deleting session records, which stopped being true when purge started
// removing evidence as well.
//
// Help is read by somebody standing in front of the command, which is
// later than the README and closer to the consequence. So this finds the
// commands that reach for the evidence package and requires each one to
// mention it, rather than trusting anybody to remember.
func TestEveryCommandThatTouchesEvidenceSaysSoInItsHelp(t *testing.T) {
	names, err := filepath.Glob("cmd_*.go")
	if err != nil || len(names) == 0 {
		t.Fatalf("no command files found: %v", err)
	}
	checked := 0
	for _, path := range names {
		b, rerr := ioutil.ReadFile(path)
		if rerr != nil {
			t.Fatal(rerr)
		}
		body := string(b)
		// The import is what says this command reaches the evidence store.
		if !strings.Contains(body, `zeroturn/internal/evidence"`) {
			continue
		}
		checked++
		// The help for a command lives in the same file, as a usage
		// constant or a function that builds one.
		help := helpTextIn(t, body)
		if help == "" {
			t.Errorf("%s touches evidence and has no help text to describe it", path)
			continue
		}
		if !strings.Contains(strings.ToLower(help), "evidence") {
			t.Errorf("%s writes or deletes evidence and its help never mentions it, "+
				"so somebody reading the command is not told", path)
		}
	}
	if checked == 0 {
		t.Fatal("no command reaches the evidence package, so this test checks nothing")
	}
}

// helpTextIn returns only the text a person is shown: the contents of
// the raw quoted blocks, and the quoted strings a usage function writes
// out. The code between them is excluded on purpose.
//
// The first version of this took every segment of the file that a split
// on the quote character produced, which is the blocks and the code in
// between. That made the check tautological, because a command file that
// imports the evidence package necessarily contains the word evidence in
// its code, so the test passed with the help text emptied out. It was
// caught by deleting the help and watching the test still pass.
func helpTextIn(t *testing.T, body string) string {
	t.Helper()
	var b strings.Builder
	// A split on the quote character alternates outside, inside, outside.
	// Only the odd positions are inside a block.
	parts := strings.Split(body, "`")
	for i := 1; i < len(parts); i += 2 {
		// A struct tag is a quoted block too, and one of them carries the
		// word this test looks for, which let the check pass with the help
		// text deleted. Help spans lines; a tag never does.
		if !strings.Contains(parts[i], "\n") {
			continue
		}
		b.WriteString(parts[i])
		b.WriteString("\n")
	}
	// A usage built up from ordinary quoted strings counts too, but only
	// the quoted part of the line, not the call around it.
	quoted := regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "WriteString(") && !strings.Contains(line, "Fprintf(&b,") {
			continue
		}
		for _, m := range quoted.FindAllStringSubmatch(line, -1) {
			b.WriteString(m[1])
			b.WriteString("\n")
		}
	}
	return b.String()
}
