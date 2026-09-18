package main

import (
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/capabilities"
)

// The exit codes are a published contract and live in three places.
// capabilities is already compared with the constants, so comparing the
// README with capabilities joins the chain rather than adding a fourth.
func TestTheReadmeExitCodeTableIsTheOneTheProgramPublishes(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}

	row := regexp.MustCompile(`(?m)^\|\s*(\d+)\s*\|\s*([^|]+?)\s*\|\s*$`)
	documented := map[string]string{}
	for _, m := range row.FindAllStringSubmatch(string(b), -1) {
		documented[m[1]] = m[2]
	}
	if len(documented) == 0 {
		t.Fatal("the README documents no exit codes, so this test checks nothing")
	}

	published := capabilities.Describe("test").ExitCodes
	if len(published) == 0 {
		t.Fatal("the program publishes no exit codes, so this test checks nothing")
	}

	for code, meaning := range published {
		got, ok := documented[code]
		if !ok {
			t.Errorf("exit code %s is published as %q and the README does not list it", code, meaning)
			continue
		}
		if !strings.EqualFold(got, meaning) {
			t.Errorf("exit code %s: the README says %q and the program publishes %q", code, got, meaning)
		}
	}
	for code, meaning := range documented {
		if _, ok := published[code]; !ok {
			t.Errorf("the README lists exit code %s as %q, and the program does not publish it", code, meaning)
		}
	}
}
