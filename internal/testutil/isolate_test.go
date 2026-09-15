package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Isolate exists so that no test can read or change the settings of the
// person running it. It has to do that on every platform, not only the
// one the tests are usually run on: os.UserHomeDir reads USERPROFILE on
// Windows and HOME everywhere else, and setting one of them hid the home
// directory on two platforms out of three.
//
// The hole was found by a test writing into a real home directory on
// Windows continuous integration, which is a slow and lucky way to find
// it. This pins it on whatever platform the tests are run on.
func TestIsolateHidesTheRealHomeDirectory(t *testing.T) {
	before, err := os.UserHomeDir()
	if err != nil {
		t.Skip("this machine reports no home directory")
	}

	Isolate(t)

	after, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("after Isolate there is no home directory: %v", err)
	}
	if after == before {
		t.Fatalf("Isolate left the home directory at %s, so a test can write into it", before)
	}
	if !strings.HasPrefix(after, filepath.Clean(os.TempDir())) && !strings.Contains(after, "Test") {
		t.Errorf("the home directory is %s, which does not look like a throwaway one", after)
	}

	// Both names have to agree. A test that reads one and a program that
	// reads the other would otherwise disagree about where home is.
	for _, name := range []string{"HOME", "USERPROFILE"} {
		if got := os.Getenv(name); got != after {
			t.Errorf("%s is %q, and the home directory is %q", name, got, after)
		}
	}
}

// The state directory is the other place a test could reach the real
// machine, and it is read from the environment by every command.
func TestIsolateMovesTheStateDirectory(t *testing.T) {
	Isolate(t)
	dir := os.Getenv("ZEROTURN_STATE_DIR")
	if dir == "" {
		t.Fatal("ZEROTURN_STATE_DIR is not set, so commands would use the real one")
	}
	home := os.Getenv("HOME")
	if filepath.Dir(dir) != filepath.Dir(home) {
		t.Errorf("the state directory %s is not beside the throwaway home %s", dir, home)
	}
}
