package security

import (
	"strings"
	"testing"
)

// These cover edges scripts/mutate found nothing failing for. Each is a
// rule written into a condition that no test could disagree with.

// The key on its own line starts at position zero. Treating "not found"
// and "found at the start" as the same thing left it in the output.
func TestAValueAtTheStartOfTheTextIsRedacted(t *testing.T) {
	key := "AKIA" + "QWERTYUIOPASDFGH"
	for _, in := range []string{
		key,
		key + " trailing words",
		"leading words " + key,
		"aws_access_key_id = " + key,
	} {
		got := Redact(in)
		if strings.Contains(got, key) {
			t.Errorf("Redact(%q) left the value: %q", in, got)
		}
		if got == "" {
			t.Errorf("Redact(%q) removed everything", in)
		}
	}
}

// Redaction has to leave the rest of the line alone, or a log becomes
// unreadable and the redaction looks like a fault.
func TestRedactionKeepsTheTextAround(t *testing.T) {
	key := "AKIA" + "QWERTYUIOPASDFGH"
	got := Redact("before " + key + " after")
	for _, want := range []string{"before ", " after"} {
		if !strings.Contains(got, want) {
			t.Errorf("Redact removed %q: %q", want, got)
		}
	}
	if strings.Contains(got, key) {
		t.Errorf("Redact left the value: %q", got)
	}
}

// Text with nothing in it comes back as it was. A rule that redacted a
// whole match rather than the value would rewrite ordinary output.
func TestTextWithNoCredentialIsUntouched(t *testing.T) {
	for _, in := range []string{
		"",
		"ordinary output\n",
		"aws_access_key_id = ${AWS_KEY}\n",
		"password = ${PASSWORD}\n",
	} {
		if got := Redact(in); got != in {
			t.Errorf("Redact(%q) changed it to %q", in, got)
		}
	}
}

// A line longer than the scanner's buffer is reported rather than
// silently dropped, because a scan that stopped early and said nothing
// would look like a file with no credentials in it.
func TestALineTooLongToScanIsReported(t *testing.T) {
	huge := strings.Repeat("x", 2*1024*1024)
	_, err := ScanBytes("huge", []byte(huge))
	if err == nil {
		t.Fatal("a line too long to scan was reported as a clean scan")
	}
	if !strings.Contains(err.Error(), "token too long") {
		t.Errorf("the error does not say what stopped it: %v", err)
	}
}
