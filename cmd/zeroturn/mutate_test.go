package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The mutation guard names its packages in the workflow file, split into
// groups so the job's wall time is the slowest group rather than the sum.
// That is a list of package directories kept by hand beside the packages,
// which is the shape of drift this project keeps finding: a package that
// is renamed or moved leaves a group naming something that is not there,
// and the guard would report it rather than run.
//
// Which packages are covered is a judgement and is recorded in
// docs/decisions.md. That a named one exists, and is named once, is not.
func TestEveryPackageTheMutationGuardNamesIsThere(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	lists := regexp.MustCompile(`(?m)^\s+packages: (.+)$`).FindAllStringSubmatch(string(b), -1)
	if len(lists) == 0 {
		t.Fatal("the workflow names no packages to mutate, so this test checks nothing")
	}

	seen := map[string]bool{}
	for _, m := range lists {
		for _, pkg := range strings.Split(strings.TrimSpace(m[1]), ",") {
			pkg = strings.TrimSpace(pkg)
			if seen[pkg] {
				t.Errorf("%s is in two groups, so it is mutated twice", pkg)
			}
			seen[pkg] = true
			if _, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(pkg))); err != nil {
				t.Errorf("the mutation guard names %s, which is not there: %v", pkg, err)
			}
		}
	}
}
