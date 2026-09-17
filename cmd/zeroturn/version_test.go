package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// A build made with go install carries no version from the release
// script, and reporting a development version to somebody who installed
// a published one is a lie about what they are running.
func TestVersionPrefersWhatTheBuildSet(t *testing.T) {
	saved := Version
	defer func() { Version = saved }()

	Version = "1.2.3"
	if got := version(); got != "1.2.3" {
		t.Fatalf("version() = %q, want the value the build set", got)
	}
}

// From a checkout there is no module version to fall back to, so the
// default stands rather than something misleading.
func TestVersionFallsBackToTheDefault(t *testing.T) {
	// Not empty and not the placeholder left every other answer passing,
	// including one invented on the spot. In a test binary there is no
	// module version, so the default is the answer.
	if got := version(); got != Version {
		t.Fatalf("version() is %q, want the default %q", got, Version)
	}
	if strings.Contains(version(), "(devel)") {
		t.Fatalf("version() reports the Go placeholder: %q", version())
	}
}

// The two tests above prove version() answers correctly. Nothing proved
// that anything calls it. Version is the raw variable, empty of meaning
// in a build the release script did not stamp, and reading it directly
// skips the fallback that makes a go install build report the version it
// actually is. Two commands did exactly that, and both publish the answer:
// capabilities is the document an adapter author is told to read, and
// verify writes the version into every evidence record.
//
// This reads the package rather than the two commands, because the next
// place to publish a version has not been written yet.
func TestTheVersionIsAlwaysReadThroughTheFunction(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["main"]
	if !ok {
		t.Fatal("package main was not parsed, so this test checks nothing")
	}

	// Inside version() the raw variable is the subject, not a mistake,
	// and the declaration names it once.
	declared := map[token.Pos]bool{}
	inFunc := map[token.Pos]bool{}
	for _, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.ValueSpec:
				for _, name := range v.Names {
					declared[name.Pos()] = true
				}
			case *ast.FuncDecl:
				if v.Name.Name == "version" && v.Recv == nil {
					ast.Inspect(v.Body, func(in ast.Node) bool {
						if id, isIdent := in.(*ast.Ident); isIdent {
							inFunc[id.Pos()] = true
						}
						return true
					})
				}
			}
			return true
		})
	}

	var direct []string
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.SelectorExpr:
				// config.Version and info.Main.Version name something
				// else that happens to share the word.
				ast.Inspect(v.X, func(in ast.Node) bool { return true })
				return false
			case *ast.KeyValueExpr:
				// A struct field called Version is a field, not the
				// variable, so only the value side is walked.
				ast.Inspect(v.Value, func(in ast.Node) bool {
					if id, isIdent := in.(*ast.Ident); isIdent && id.Name == "Version" &&
						!declared[id.Pos()] && !inFunc[id.Pos()] {
						direct = append(direct, fset.Position(id.Pos()).String())
					}
					return true
				})
				return false
			case *ast.Ident:
				if v.Name == "Version" && !declared[v.Pos()] && !inFunc[v.Pos()] {
					direct = append(direct, fset.Position(v.Pos()).String())
				}
			}
			return true
		})
		_ = name
	}

	for _, at := range direct {
		t.Errorf("%s reads the Version variable directly, so a build the release script did not stamp reports a development version here. Call version() instead", at)
	}
}
