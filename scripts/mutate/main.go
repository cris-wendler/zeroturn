// Command mutate changes one operator in the source at a time and runs
// the tests for the package it changed. A change the tests still accept
// is a survivor: some behaviour is described by no test that can fail
// for it.
//
// This exists because an audit found twelve tests that passed whether or
// not the code worked, six of which survived the whole suite. They were
// found by hand, one at a time. This finds the next one.
//
// It edits files in place and restores them, so it refuses to run with
// uncommitted changes and restores on interrupt.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// swaps are the changes applied. Each is a different way for a condition
// to be wrong, and each is a change a person could plausibly make.
var swaps = map[token.Token]token.Token{
	token.EQL:  token.NEQ, // == becomes !=
	token.NEQ:  token.EQL, // != becomes ==
	token.LSS:  token.LEQ, // <  becomes <=
	token.GTR:  token.GEQ, // >  becomes >=
	token.LEQ:  token.LSS, // <= becomes <
	token.GEQ:  token.GTR, // >= becomes >
	token.LAND: token.LOR, // && becomes ||
	token.LOR:  token.LAND,
}

type mutant struct {
	pkg  string
	file string
	line int
	// fn is the function the change is inside. An accepted survivor is
	// recorded by function rather than by line, because a line number
	// moves whenever anything above it changes: deleting one helper
	// shifted every entry in the file below it and reported them all as
	// out of date at once.
	fn   string
	from string
	to   string
	// expr is the node to change, held so it can be put back.
	expr *ast.BinaryExpr
}

func main() {
	pkgs := flag.String("packages", "", "comma separated package directories to mutate")
	max := flag.Int("max", 0, "stop after this many mutants, 0 for all")
	timeout := flag.String("timeout", "120s", "test timeout for each mutant")
	acceptedPath := flag.String("accepted", "scripts/mutate/accepted", "file listing changes that alter nothing")
	flag.Parse()

	if *pkgs == "" {
		fmt.Fprintln(os.Stderr, "mutate: no packages given, for example -packages internal/policy,internal/state")
		os.Exit(2)
	}
	if dirty, err := workingTreeDirty(); err != nil {
		fmt.Fprintf(os.Stderr, "mutate: %v\n", err)
		os.Exit(2)
	} else if dirty {
		fmt.Fprintln(os.Stderr, "mutate: the working tree has uncommitted changes, and this edits files in place")
		os.Exit(2)
	}

	accepted, err := readAccepted(*acceptedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mutate: %v\n", err)
		os.Exit(2)
	}
	stillThere := map[string]bool{}
	// Staleness can only be judged for packages that were actually run,
	// or every entry looks out of date whenever a subset is mutated.
	ranPkg := map[string]bool{}

	var survivors []string
	total, killed, skipped := 0, 0, 0

	for _, pkg := range strings.Split(*pkgs, ",") {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}
		ranPkg[pkg] = true
		mutants, err := plan(pkg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mutate: %s: %v\n", pkg, err)
			os.Exit(2)
		}
		// A baseline run, so a package whose tests already fail is not
		// reported as a package whose tests notice nothing.
		if ok, out := test(pkg, *timeout); !ok {
			fmt.Fprintf(os.Stderr, "mutate: %s fails before any change was made\n%s\n", pkg, out)
			os.Exit(2)
		}

		for i, m := range mutants {
			if *max > 0 && total >= *max {
				skipped += len(mutants) - i
				break
			}
			total++
			survived, err := apply(m, *timeout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "mutate: %v\n", err)
				os.Exit(2)
			}
			id := fmt.Sprintf("%s %s %s becomes %s", m.file, m.fn, m.from, m.to)
			if survived {
				if _, ok := accepted[id]; ok {
					stillThere[id] = true
					continue
				}
				survivors = append(survivors, fmt.Sprintf("%s  (line %d)", id, m.line))
				fmt.Printf("SURVIVED  %s  (line %d)\n", id, m.line)
				continue
			}
			killed++
		}
	}

	fmt.Printf("\n%d changes made, %d noticed, %d not noticed", total, killed, len(survivors))
	if skipped > 0 {
		fmt.Printf(", %d not tried", skipped)
	}
	fmt.Println()

	// An accepted entry that is now noticed is out of date, and leaving
	// it would quietly excuse a future change at the same line.
	var stale []string
	for id := range accepted {
		if stillThere[id] {
			continue
		}
		if !ranPkg[filepath.Dir(strings.SplitN(id, " ", 2)[0])] {
			continue
		}
		stale = append(stale, id)
	}
	sort.Strings(stale)

	if len(survivors) > 0 {
		fmt.Println("\nEach line below is a change to the code that every test accepted.")
		fmt.Println("Read each one. If it alters behaviour, the behaviour has no test that")
		fmt.Println("can fail for it. If it alters nothing, add it to " + *acceptedPath + " with the reason.")
		sort.Strings(survivors)
		for _, s := range survivors {
			fmt.Println("  " + s)
		}
	}
	if len(stale) > 0 && *max == 0 {
		fmt.Println("\nThese are listed as altering nothing, and the tests now notice them.")
		fmt.Println("Remove them from " + *acceptedPath + ".")
		for _, s := range stale {
			fmt.Println("  " + s)
		}
	}
	if len(survivors) > 0 || (len(stale) > 0 && *max == 0) {
		os.Exit(1)
	}
}

// readAccepted loads the changes recorded as altering nothing. A line is
// an identifier, two spaces, and the reason it is there.
func readAccepted(path string) (map[string]string, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("%s: %q has no reason after it", path, line)
		}
		out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return out, nil
}

func workingTreeDirty() (bool, error) {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("git status: %v", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// plan lists every change that could be made in one package, in a stable
// order, so two runs of this command do the same thing.
func plan(pkg string) ([]mutant, error) {
	names, err := filepath.Glob(filepath.Join(pkg, "*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	var out []mutant
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		// A file this platform does not build cannot be noticed by any
		// test run here, so every change to it would be reported as a
		// survivor. proc_windows.go was reported that way.
		if built, err := build.Default.MatchFile(filepath.Dir(name), filepath.Base(name)); err != nil {
			return nil, err
		} else if !built {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		var found []mutant
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				be, ok := n.(*ast.BinaryExpr)
				if !ok {
					return true
				}
				to, ok := swaps[be.Op]
				if !ok {
					return true
				}
				found = append(found, mutant{
					pkg: pkg, file: name, line: fset.Position(be.OpPos).Line,
					fn: fn.Name.Name, from: be.Op.String(), to: to.String(),
				})
				return true
			})
		}
		sort.SliceStable(found, func(i, j int) bool { return found[i].line < found[j].line })
		out = append(out, found...)
	}
	return out, nil
}

// apply makes one change, runs the package's tests, and puts the file
// back. The original bytes are held in memory and restored on any exit,
// including an interrupt, so a run that is stopped leaves no edit behind.
func apply(m mutant, timeout string) (survived bool, err error) {
	original, err := ioutil.ReadFile(m.file)
	if err != nil {
		return false, err
	}
	restore := func() { ioutil.WriteFile(m.file, original, 0644) }

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-stop:
			restore()
			fmt.Fprintln(os.Stderr, "\nmutate: interrupted, the file was put back")
			os.Exit(130)
		case <-done:
		}
	}()
	defer func() {
		close(done)
		signal.Stop(stop)
		restore()
	}()

	mutated, err := rewrite(original, m)
	if err != nil {
		return false, err
	}
	if mutated == nil {
		// The change could not be placed, which is not a survivor.
		return false, nil
	}
	if err := ioutil.WriteFile(m.file, mutated, 0644); err != nil {
		return false, err
	}

	ok, _ := test(m.pkg, timeout)
	return ok, nil
}

// rewrite produces the source with one operator changed, found by line
// and operator rather than by holding a node across a reparse.
func rewrite(src []byte, m mutant) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, m.file, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var target *ast.BinaryExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if target != nil {
			return false
		}
		be, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if fset.Position(be.OpPos).Line == m.line && be.Op.String() == m.from {
			target = be
			return false
		}
		return true
	})
	if target == nil {
		return nil, nil
	}
	for from, to := range swaps {
		if from.String() == m.from && to.String() == m.to {
			target.Op = to
		}
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// test reports whether the package's tests pass. A build failure counts
// as noticed: a change that does not compile is not one that slipped by.
func test(pkg, timeout string) (bool, string) {
	cmd := exec.Command("go", "test", "-count=1", "-timeout", timeout, "./"+pkg+"/")
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}
