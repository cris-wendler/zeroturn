package security

import (
	"strings"
	"testing"
)

// These cover conditions in the scanner that scripts/mutate could change
// without any test disagreeing. The scanner decides whether a credential
// reaches a model, so a rule here with nothing holding it is worse than
// one anywhere else in the project.

// A value with almost no character variety is a template rather than a
// secret: PASSWORD=xxxxxxxx, or a line of dashes. The rule is a count of
// distinct characters, and the count it turns on had nothing checking it.
func TestThePlaceholderBoundaryIsCounted(t *testing.T) {
	// Three distinct characters is still a template.
	three := "password=" + strings.Repeat("abc", 8)
	if !isPlaceholder(strings.Repeat("abc", 8)) {
		t.Error("a value of three distinct characters is not treated as a template")
	}
	if f, _ := ScanBytes("p", []byte(three+"\n")); len(f) != 0 {
		t.Errorf("a template value was reported as a credential: %v", f)
	}

	// Four is a value.
	four := "password=" + strings.Repeat("abcd", 6)
	if isPlaceholder(strings.Repeat("abcd", 6)) {
		t.Error("a value of four distinct characters is treated as a template")
	}
	if f, _ := ScanBytes("p", []byte(four+"\n")); len(f) != 1 {
		t.Errorf("a credential of four distinct characters was not reported: %v", f)
	}
}

// A key whose shape identifies it is a key whatever its characters are.
// AKIA followed by sixteen A's is a well formed AWS key id and has three
// distinct characters in it, so the template rule would throw it away if
// the template rule applied. It does not: a detector that matches a whole
// value never asks whether the value looks like a template, because the
// shape already answered that.
func TestAKeyOfFewDistinctCharactersIsStillAKey(t *testing.T) {
	key := "AKIA" + strings.Repeat("A", 16)
	if !isPlaceholder(key) {
		t.Fatal("this test needs a key the template rule would reject, and this one is not")
	}

	found, err := ScanBytes("p", []byte("aws_access_key_id = "+key+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Errorf("a well formed key was not reported: %v", found)
	}

	if got := Redact(key); strings.Contains(got, key) {
		t.Errorf("Redact left the key in place: %q", got)
	}
}
