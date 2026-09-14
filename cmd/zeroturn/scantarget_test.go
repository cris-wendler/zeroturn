package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The guard decides what to scan from the file it opened, not from the
// path it was given, and it never reads more than the limit whatever the
// size says. A path can name a different file by the time it is opened,
// and what would be scanned then is not what was checked.
func TestScanTargetReadsAnOrdinaryFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.env")
	if err := ioutil.WriteFile(p, []byte("TOKEN=value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	content, ok := scanTarget(p)
	if !ok {
		t.Fatal("an ordinary file was not scanned")
	}
	if string(content) != "TOKEN=value\n" {
		t.Fatalf("content %q", content)
	}
}

func TestScanTargetRefusesWhatCannotBeScanned(t *testing.T) {
	dir := t.TempDir()

	big := filepath.Join(dir, "big")
	if err := ioutil.WriteFile(big, make([]byte, maxScan+1), 0600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"a directory":              dir,
		"a file that is not there": filepath.Join(dir, "absent"),
		"a file over the limit":    big,
	}
	for what, p := range cases {
		if _, ok := scanTarget(p); ok {
			t.Errorf("%s was scanned", what)
		}
	}
}

// A device reports a size of zero and then never reaches the end, so
// opening one and reading it would hang the read a developer is waiting
// for. It has to be refused before it is opened.
func TestScanTargetRefusesADevice(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero on this system")
	}
	if _, ok := scanTarget("/dev/zero"); ok {
		t.Fatal("a device was scanned, which would not have ended")
	}
}

// A file of exactly the limit is scanned, and all of it is read. The
// limit is a maximum, not a threshold to stay under.
func TestScanTargetReadsAFileAtTheLimit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "at-the-limit")
	if err := ioutil.WriteFile(p, []byte(strings.Repeat("a", maxScan)), 0600); err != nil {
		t.Fatal(err)
	}
	content, ok := scanTarget(p)
	if !ok {
		t.Fatal("a file of exactly the limit was refused")
	}
	if len(content) != maxScan {
		t.Fatalf("read %d bytes of a %d byte file", len(content), maxScan)
	}
}
