package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A document written for someone judging this work counts things: how
// many decisions were recorded, how many schemas the contract publishes.
// Those numbers were written once and then went stale, one of them by a
// hundred tests and one by ten decisions, because nothing compared them
// with what is in the repository.
//
// Understating is not safe either. A reader who counts finds a different
// answer, and then everything else in the document is worth less.
func TestTheCountsInTheProseAreTheCountsInTheRepository(t *testing.T) {
	root := filepath.Join("..", "..")
	prose, err := ioutil.ReadFile(filepath.Join(root, "docs", "how-this-was-built.md"))
	if err != nil {
		t.Fatal(err)
	}

	decisions, err := ioutil.ReadFile(filepath.Join(root, "docs", "decisions.md"))
	if err != nil {
		t.Fatal(err)
	}
	entries := len(regexp.MustCompile(`(?m)^## [0-9]+\.`).FindAll(decisions, -1))
	claimed(t, string(prose), `([0-9]+) entries in \[decisions\.md\]`, entries, "decisions")

	schemas, err := filepath.Glob(filepath.Join(root, "schemas", "*.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	claimed(t, string(prose), `against ([0-9]+) schemas`, len(schemas), "schemas")

	// The count this comment blames for going stale by a hundred was the
	// only one still unchecked, and it was wrong again by the time anyone
	// looked.
	tests, files := countTests(t, root)
	claimed(t, string(prose), `([0-9]+) tests across`, tests, "tests")
	claimed(t, string(prose), `tests across ([0-9]+) files`, files, "test files")
}

// countTests counts the test functions in the module and the files that
// hold them. Benchmarks are left out: the sentence says tests.
func countTests(t *testing.T, root string) (tests, files int) {
	t.Helper()
	fn := regexp.MustCompile(`(?m)^func Test`)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}
		b, rerr := ioutil.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		files++
		tests += len(fn.FindAll(b, -1))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tests, files
}

// claimed reads one number out of the prose and compares it with what
// was counted, naming the correction when they differ.
func claimed(t *testing.T, prose, pattern string, actual int, what string) {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(prose)
	if m == nil {
		t.Errorf("%s: the document no longer states a count in the form %q, so it cannot be checked", what, pattern)
		return
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Errorf("%s: %q is not a number", what, m[1])
		return
	}
	if n != actual {
		t.Errorf("%s: the document says %d, the repository has %d. Correct the sentence: %q",
			what, n, actual, strings.TrimSpace(m[0]))
	}
}
