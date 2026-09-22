package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The environment a mutant's tests run in has to hold two things at once.
// Records must land in the sandbox, because this tool changes the code
// that decides where records are kept, and a mutant of that code once
// wrote a developer's own state directory full of test fixtures. The Go
// caches must not, because Go works them out from the home directory and
// following it there rebuilt every package from cold for every mutant.
func TestTheSandboxRedirectsRecordsAndNotTheBuildCache(t *testing.T) {
	sandbox := t.TempDir()
	env := map[string]string{}
	for _, kv := range sandboxEnv(sandbox) {
		if i := strings.IndexByte(kv, '='); i > 0 {
			env[kv[:i]] = kv[i+1:]
		}
	}

	for _, name := range []string{"HOME", "USERPROFILE", "ZEROTURN_STATE_DIR"} {
		got := env[name]
		if got == "" {
			t.Errorf("%s is not set, so the run is not isolated", name)
			continue
		}
		if !strings.HasPrefix(got, sandbox) {
			t.Errorf("%s is %q, which is outside the sandbox", name, got)
		}
	}

	for _, name := range []string{"GOCACHE", "GOMODCACHE"} {
		got := env[name]
		if got == "" {
			t.Skipf("%s could not be read from go env here", name)
		}
		if strings.HasPrefix(got, sandbox) {
			t.Errorf("%s is %q, inside a directory deleted after every mutant", name, got)
		}
	}

	// And the state directory is under the sandbox rather than beside it,
	// so removing the sandbox removes the records with it.
	if want := filepath.Join(sandbox, "state"); env["ZEROTURN_STATE_DIR"] != want {
		t.Errorf("the state directory is %q, want %q", env["ZEROTURN_STATE_DIR"], want)
	}
	if _, err := os.Stat(sandbox); err != nil {
		t.Fatalf("the sandbox is not there: %v", err)
	}
}
