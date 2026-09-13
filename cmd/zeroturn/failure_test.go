package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// Whatever is wrong with the machine, a hook must not stop a developer's
// session. These check the fail open rule against conditions that are
// rare but real: a state directory that cannot be written, one that is
// not a directory, and configuration that cannot be parsed.
func TestGateFailsOpenOnABrokenStateDirectory(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeStrict })
	statusline(t, work, "s", contextAt(99))

	cases := map[string]func(t *testing.T) string{
		"the state directory is a file": func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "state-as-a-file")
			if err := os.WriteFile(p, []byte("not a directory"), 0644); err != nil {
				t.Fatal(err)
			}
			return p
		},
		"the state directory cannot be written": func(t *testing.T) string {
			if runtime.GOOS == "windows" {
				t.Skip("Unix permissions")
			}
			p := filepath.Join(t.TempDir(), "read-only")
			if err := os.MkdirAll(filepath.Join(p, "sessions"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(p, 0500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(p, 0700) })
			return p
		},
		"the state directory is somewhere that cannot be created": func(t *testing.T) string {
			if runtime.GOOS == "windows" {
				t.Skip("Unix permissions")
			}
			return "/proc/zeroturn-cannot-create-this"
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := setup(t)
			t.Setenv("ZEROTURN_STATE_DIR", dir)
			r := gate(t, work, "s")
			if r.code != 0 {
				t.Errorf("exit %d, a hook must always exit 0: %s", r.code, r.stderr)
			}
			if r.stdout != "" {
				t.Errorf("the gate answered while its state was broken: %q", r.stdout)
			}
			if line := statusline(t, work, "s", contextAt(99)); line.code != 0 {
				t.Errorf("the status line exited %d", line.code)
			}
		})
	}
}

func TestGateFailsOpenOnUnreadableConfiguration(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeStrict })
	statusline(t, work, "s", contextAt(99))
	testutil.Write(t, work, ".zeroturn.json", "{not json")

	// With no configuration to read, the defaults apply, and the default
	// is observe: silence.
	r := gate(t, work, "s")
	if r.code != 0 || r.stdout != "" {
		t.Fatalf("%+v", r)
	}
}

// A command a person runs must explain the problem rather than fail open.
func TestCommandsExplainABrokenConfiguration(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, ".zeroturn.json", `{"version": 1, "guard": {"mode": "sideways"}}`)
	for _, args := range [][]string{{"policy", "show"}, {"verify"}, {"ship", "--message", "x", "--files", "README.md"}} {
		r := run(t, work, "", args...)
		if r.code == 0 {
			t.Errorf("%v accepted a broken configuration", args)
		}
		if !strings.Contains(r.stderr, "next:") {
			t.Errorf("%v did not say what to do: %q", args, r.stderr)
		}
	}
}

// The credential guard reads a file that may be anything at all.
func TestCredentialGuardSurvivesOddFiles(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	odd := map[string]string{
		"empty.txt":       "",
		"binary.bin":      "\x00\x01\x02\x03",
		"huge-line.log":   strings.Repeat("x", 3<<20),
		"unicode.txt":     "会话 ключ κλειδί\n",
		"no-newline.conf": "key=value",
	}
	for name, content := range odd {
		testutil.Write(t, work, name, content)
		r := readGate(t, work, filepath.Join(work, name))
		if r.code != 0 {
			t.Errorf("%s: exit %d", name, r.code)
		}
	}
	if runtime.GOOS != "windows" {
		// A path that is a device, a socket, or a directory must not hang.
		for _, path := range []string{"/dev/zero", "/dev/null", work} {
			r := readGate(t, work, path)
			if r.code != 0 || r.stdout != "" {
				t.Errorf("%s: %+v", path, r)
			}
		}
	}
}
