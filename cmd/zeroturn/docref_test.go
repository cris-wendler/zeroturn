package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A message that sends someone to a file is worth nothing when the file
// is not there. This walks the source for paths under docs/, schemas/,
// and integrations/ and checks each one exists in the repository.
func TestEveryDocumentNamedInOutputExists(t *testing.T) {
	root := filepath.Join("..", "..")
	ref := regexp.MustCompile(`(docs|schemas|integrations)/[A-Za-z0-9._/-]+`)

	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "docs", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, rerr := ioutil.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, m := range ref.FindAllString(string(b), -1) {
			m = strings.TrimRight(m, ".,)")
			// A path ending in a separator is a directory being named,
			// and one with no dot is a package path, not a document.
			if strings.HasSuffix(m, "/") || !strings.Contains(filepath.Base(m), ".") {
				continue
			}
			if _, serr := os.Stat(filepath.Join(root, m)); serr != nil {
				t.Errorf("%s names %s, which does not exist", p, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
