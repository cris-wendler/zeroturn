package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// "Result: 1 checks passed" reads as a defect in the tool that printed
// it. This class has now been fixed three times: once for the gate
// prompt, once for doctor, and once across eighteen other places. Each
// time the fix was a helper that the next counted noun did not use.
//
// A number followed by a plural noun, in a string a person reads, has to
// go through output.Counted. This finds the ones that do not.
var pluralAfterCount = regexp.MustCompile(`%d [a-z]+s\b`)

// Words that end in s and are not nouns being counted. A short list, and
// a wrong entry here only ever hides a real one, so it stays small.
var notAPluralNoun = map[string]bool{
	"is": true, "was": true, "has": true, "as": true, "its": true,
	"this": true, "thus": true, "less": true, "plus": true, "versus": true,
	// Verbs that follow a number without counting it.
	"looks": true, "reads": true, "needs": true, "holds": true,
}

func TestEveryCountedNounGoesThroughTheHelper(t *testing.T) {
	roots := []string{".", filepath.Join("..", "..", "internal")}
	exempt := exemptLiterals(t, roots)

	checked := 0
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") ||
				strings.HasSuffix(info.Name(), "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return nil
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					return true
				}
				for _, m := range pluralAfterCount.FindAllString(value, -1) {
					word := strings.TrimSpace(strings.TrimPrefix(m, "%d"))
					if notAPluralNoun[word] {
						continue
					}
					checked++
					if exempt[value] {
						continue
					}
					t.Errorf("%s: %q counts %q without output.Counted, so one of them reads as %q",
						filepath.ToSlash(path), value, word, strings.Replace(m, "%d", "1", 1))
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked == 0 {
		t.Fatal("no counted nouns were found at all, so this test checks nothing")
	}
}

// exemptLiterals collects the plural forms handed to output.Counted and
// output.CountedPair, which are the strings that already have a singular
// beside them.
func exemptLiterals(t *testing.T, roots []string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, root := range roots {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
				return nil
			}
			fset := token.NewFileSet()
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return nil
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name != "Counted" && sel.Sel.Name != "CountedPair" {
					return true
				}
				for _, arg := range call.Args {
					if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if v, uerr := strconv.Unquote(lit.Value); uerr == nil {
							out[v] = true
						}
					}
				}
				return true
			})
			return nil
		})
	}
	if len(out) == 0 {
		t.Fatal("no calls to output.Counted were found, so nothing would be exempt")
	}
	return out
}
