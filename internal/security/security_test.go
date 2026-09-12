package security

import (
	"strings"
	"testing"
)

// Test values are assembled at run time so that this file does not itself
// contain anything a credential scanner would report.
func j(parts ...string) string { return strings.Join(parts, "") }

var samples = map[string]string{
	"private key":           j("-----BEGIN ", "RSA PRIVATE KEY-----"),
	"aws access key id":     j("AKIA", "QWERTYUIOPASDFGH"),
	"github token":          j("ghp_", strings.Repeat("aB3d", 9)),
	"slack token":           j("xoxb-", "1234567890-abcdefghij"),
	"stripe secret key":     j("sk_", "live_", "4eC39HqLyjWDarjtT1zdp7dc"),
	"google api key":        j("AIza", "SyA1b2C3d4E5f6G7h8I9j0KlMnOpQrStUvW"),
	"anthropic api key":     j("sk-", "ant-", "api03-Zx9Yw8Vu7Ts6Rq5Po4Nm3Lk2"),
	"openai api key":        j("sk-", "proj-", "Zx9Yw8Vu7Ts6Rq5Po4Nm3Lk2"),
	"npm token":             j("npm_", strings.Repeat("aB3dE", 7), "f"),
	"json web token":        j("eyJ", "hbGciOiJIUzI1NiJ9", ".eyJ", "zdWIiOiIxMjM0NTY3ODkwIn0", ".dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"),
	"credential in url":     j("https://deploy:", "hunter2hunter2", "@git.example.com/repo.git"),
	"credential assignment": j("api_key", " = \"", "Q7vN2kLp9XwR4tYz8BmC", "\""),
}

func TestEachCategoryDetected(t *testing.T) {
	for cat, value := range samples {
		f, err := ScanBytes("f.txt", []byte("line one\n"+value+"\n"))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, x := range f {
			if x.Category == cat && x.Line == 2 && x.File == "f.txt" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s not detected, got %+v", cat, f)
		}
	}
}

func TestFindingNeverCarriesTheValue(t *testing.T) {
	for cat, value := range samples {
		f, _ := ScanBytes("f.txt", []byte(value))
		for _, x := range f {
			if strings.Contains(x.String(), value) || strings.Contains(x.Category, value) {
				t.Errorf("%s: finding exposes the value", cat)
			}
		}
	}
}

func TestPlaceholdersAreNotReported(t *testing.T) {
	for _, line := range []string{
		`password = "changeme"`,
		`api_key: "xxxxxxxxxxxxxxxxxxxx"`,
		`secret = "${SECRET_FROM_ENV}"`,
		`client_secret = "<your-client-secret>"`,
		`access_token = "0000000000000000000"`,
		`password = "aaaaaaaaaaaaaaaaaaaa"`,
		`api_key: "your-api-key-here"`,
		`client_secret = "example_secret_value_here"`,
		`access_token = "replace-with-your-token"`,
		`secret = "{{ vault_secret }}"`,
		`password = "%DEPLOY_PASSWORD%"`,
	} {
		f, _ := ScanBytes("example.md", []byte(line))
		if len(f) != 0 {
			t.Errorf("placeholder reported: %q -> %+v", line, f)
		}
	}
}

func TestOrdinaryCodeIsNotReported(t *testing.T) {
	src := `package main

func passwordPolicy(minLength int) bool { return minLength >= 12 }
var tokenCount = 42
// See https://github.com/org/repo for details.
`
	if f, _ := ScanBytes("main.go", []byte(src)); len(f) != 0 {
		t.Fatalf("false positives: %+v", f)
	}
}

func TestBinaryContentSkipped(t *testing.T) {
	b := append([]byte{0, 1, 2}, []byte(samples["aws access key id"])...)
	if f, _ := ScanBytes("img.png", b); f != nil {
		t.Fatalf("binary scanned: %+v", f)
	}
}

func TestRedactRemovesEveryValue(t *testing.T) {
	for cat, value := range samples {
		out := Redact("before " + value + " after")
		if strings.Contains(out, value) {
			t.Errorf("%s: value survived redaction: %q", cat, out)
		}
		if !strings.Contains(out, "[redacted ") {
			t.Errorf("%s: no redaction marker in %q", cat, out)
		}
	}
}

func TestRedactKeepsAssignmentKey(t *testing.T) {
	out := Redact(samples["credential assignment"])
	if !strings.HasPrefix(out, "api_key = \"") || strings.Contains(out, "Q7vN2kLp9XwR4tYz8BmC") {
		t.Fatalf("got %q", out)
	}
}

func TestRedactLeavesPlainTextAlone(t *testing.T) {
	s := "PASS  test  4.8s\nok  github.com/x/y  0.2s"
	if Redact(s) != s {
		t.Fatal("plain output changed")
	}
}

func TestLongLinesDoNotFail(t *testing.T) {
	long := strings.Repeat("a", 1<<20) + samples["aws access key id"]
	if _, err := ScanBytes("min.js", []byte(long)); err != nil {
		t.Fatalf("long line: %v", err)
	}
}

func TestCategoriesDeduplicated(t *testing.T) {
	c := Categories([]Finding{{Category: "a"}, {Category: "b"}, {Category: "a"}})
	if len(c) != 2 {
		t.Fatalf("got %v", c)
	}
}
