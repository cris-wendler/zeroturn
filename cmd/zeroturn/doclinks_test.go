package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A link to a document that was deleted is a 404 on a public repository,
// and nothing in this project noticed. Removing eleven documents left
// nine dead links behind, in files that had not been touched: the
// conformance suite's own README, the adapter template, and two of the
// documents that stayed.
func TestEveryLinkBetweenDocumentsResolves(t *testing.T) {
	root := filepath.Join("..", "..")
	link := regexp.MustCompile(`\]\(([^)]+)\)`)

	checked := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".claude", "node_modules", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		// The working notes are local and are not published.
		if info.Name() == "CLAUDE.md" {
			return nil
		}
		b, rerr := ioutil.ReadFile(path)
		if rerr != nil {
			return nil
		}
		for _, m := range link.FindAllStringSubmatch(string(b), -1) {
			dest := m[1]
			if strings.HasPrefix(dest, "http") || strings.HasPrefix(dest, "#") ||
				strings.HasPrefix(dest, "mailto:") {
				continue
			}
			// A link to a heading inside a file still names the file.
			if i := strings.IndexByte(dest, '#'); i >= 0 {
				dest = dest[:i]
			}
			if dest == "" {
				continue
			}
			checked++
			target := filepath.Join(filepath.Dir(path), dest)
			if _, serr := os.Stat(target); serr != nil {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s links to %s, which is not there", filepath.ToSlash(rel), m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 20 {
		t.Fatalf("only %d links were checked, so this test checks nothing", checked)
	}
}
