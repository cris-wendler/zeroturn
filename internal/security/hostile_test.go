package security

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

// The scanner reads files chosen by someone else. Whatever it is handed,
// it must finish, never panic, and never put a value in a finding.
// A key the scanner really detects, split so this file does not read as
// one itself.
const hostileKey = "AKIA" + "ZXCVBNMASDFGHJKL"

func TestHostileContent(t *testing.T) {
	cases := map[string][]byte{
		"empty":                 {},
		"only newlines":         []byte(strings.Repeat("\n", 10000)),
		"no newline at all":     []byte(strings.Repeat("abcdefghij", 100000)),
		"one enormous line":     []byte(strings.Repeat("x", 3<<20)),
		"invalid utf8":          {0xff, 0xfe, 'a', 'b', '\n', 0xc3, '\n'},
		"control characters":    []byte("\x01\x02\x03\x04\x05\n\x7f\n"),
		"a line of separators":  []byte(strings.Repeat("://@", 20000)),
		"repeated anchor words": []byte(strings.Repeat("password=\n", 20000)),
		"nul bytes in text":     []byte("password=hunter2hunter2\x00more text\n"),
		// Every case above is refused or finds nothing, so the check that
		// a finding never carries the value never ran on a finding. This
		// one is hostile and scannable at the same time.
		"a real secret among hostile input": []byte(
			strings.Repeat("x", 4000) + "\naws_access_key_id = " + hostileKey + "\n" +
				strings.Repeat("://@", 4000) + "\n"),
	}
	found := 0
	for name, content := range cases {
		start := time.Now()
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s panicked: %v", name, r)
				}
			}()
			findings, err := ScanBytes(name, content)
			if err != nil && !strings.Contains(err.Error(), "token too long") {
				t.Errorf("%s: %v", name, err)
			}
			found += len(findings)
			for _, f := range findings {
				if strings.Contains(f.String(), "hunter2") || strings.Contains(f.String(), hostileKey) {
					t.Errorf("%s: a finding carried the value: %s", name, f.String())
				}
			}
		}()
		if took := time.Since(start); took > 20*time.Second {
			t.Errorf("%s took %s", name, took)
		}
	}
	// Without a finding anywhere, the check inside the loop asserted
	// nothing at all.
	if found == 0 {
		t.Fatal("no hostile input produced a finding, so the value check never ran")
	}
}

func TestRandomContentNeverPanics(t *testing.T) {
	r := rand.New(rand.NewSource(20260912))
	alphabet := []byte("abcdefgh0123456789_-=:/@.\"' \n\tAKIAghp_xoxeyJBEGIN")
	for i := 0; i < 500; i++ {
		b := make([]byte, r.Intn(4000))
		for j := range b {
			b[j] = alphabet[r.Intn(len(alphabet))]
		}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("random content panicked: %v", rec)
				}
			}()
			ScanBytes("random", b)
			Redact(string(b))
		}()
	}
}

// Redaction runs on output that ZeroTurn prints. It must never leave a
// value behind, whatever surrounds it.
func TestRedactionUnderAwkwardSurroundings(t *testing.T) {
	key := "AKIA" + "QWERTYUIOPASDFGH"
	surroundings := []string{
		"%s", "prefix%s", "%ssuffix", "[%s]", "\"%s\"", "line one\n%s\nline three",
		"%s %s", "{\"key\":\"%s\"}", "\t%s\t", "url=https://x/%s",
	}
	for _, pattern := range surroundings {
		text := strings.ReplaceAll(pattern, "%s", key)
		if out := Redact(text); strings.Contains(out, key) {
			t.Errorf("the value survived in %q: %q", pattern, out)
		}
	}
}
