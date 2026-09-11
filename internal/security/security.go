// Package security detects credentials in content ZeroTurn is about to
// stage, and redacts them from anything ZeroTurn prints or logs.
//
// Findings report a file, a line number, and a category. The matched value
// is never returned, printed, stored, or logged, because a leak report that
// echoes the secret has moved the secret rather than contained it.
package security

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
)

type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Category string `json:"category"`
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d  %s", f.File, f.Line, f.Category)
}

type detector struct {
	category string
	re       *regexp.Regexp
	// requiresValue marks detectors whose match includes surrounding text,
	// so the placeholder filter must inspect the captured value.
	valueGroup int
}

var detectors = []detector{
	{"private key", regexp.MustCompile(`-----BEGIN (?:RSA |DSA |EC |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY( BLOCK)?-----`), 0},
	{"aws access key id", regexp.MustCompile(`\b(?:AKIA|ASIA|ABIA|ACCA|A3T[A-Z0-9])[A-Z0-9]{16}\b`), 0},
	{"github token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`), 0},
	{"slack token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), 0},
	{"stripe secret key", regexp.MustCompile(`\b(?:sk|rk)_live_[A-Za-z0-9]{16,}\b`), 0},
	{"google api key", regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`), 0},
	{"anthropic api key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_\-]{24,}`), 0},
	{"openai api key", regexp.MustCompile(`\bsk-proj-[A-Za-z0-9_\-]{20,}`), 0},
	{"npm token", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`), 0},
	{"pypi token", regexp.MustCompile(`\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_\-]{20,}`), 0},
	{"json web token", regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`), 0},
	{"credential in url", regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.\-]*://[^/\s:@"']+:[^/\s:@"']+@[^\s"']+`), 0},
	{"credential assignment", regexp.MustCompile(`(?i)\b(?:password|passwd|secret|api[_\-]?key|access[_\-]?token|auth[_\-]?token|private[_\-]?key|client[_\-]?secret)\b\s*[:=]\s*["']?([A-Za-z0-9/+=_\-]{16,})["']?`), 1},
}

// placeholders keep example configuration and documentation from being
// reported. A value that names itself as an example is not a credential.
var placeholders = regexp.MustCompile(`(?i)^(?:x{3,}|\.{3,}|changeme|placeholder|redacted|example|sample|dummy|test|fake|your[_\-]?|my[_\-]?|<.*>|\$\{.*\}|\$[A-Z_]+|null|none|true|false|todo|tbd|abc123|secret|password|token|0+|1234567890.*)$`)

func isPlaceholder(v string) bool {
	if placeholders.MatchString(v) {
		return true
	}
	// A value with no character variety is a template, not a credential.
	distinct := map[rune]bool{}
	for _, r := range v {
		distinct[r] = true
	}
	return len(distinct) <= 3
}

// ScanReader reports credential findings in one file's content.
func ScanReader(name string, r io.Reader) ([]Finding, error) {
	var out []Finding
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if len(text) > 8000 {
			text = text[:8000]
		}
		for _, d := range detectors {
			m := d.re.FindStringSubmatch(text)
			if m == nil {
				continue
			}
			if d.valueGroup > 0 && d.valueGroup < len(m) && isPlaceholder(m[d.valueGroup]) {
				continue
			}
			out = append(out, Finding{File: name, Line: line, Category: d.category})
		}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// LooksBinary reports whether content should be skipped by the scanner.
func LooksBinary(b []byte) bool {
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(b[:n], 0) >= 0
}

func ScanBytes(name string, b []byte) ([]Finding, error) {
	if LooksBinary(b) {
		return nil, nil
	}
	return ScanReader(name, bytes.NewReader(b))
}

// Redact replaces credential shaped text with a category label. It is
// applied to every command output ZeroTurn prints or writes to a log.
func Redact(s string) string {
	for _, d := range detectors {
		cat := d.category
		if d.valueGroup > 0 {
			s = d.re.ReplaceAllStringFunc(s, func(m string) string {
				sub := d.re.FindStringSubmatch(m)
				if len(sub) > d.valueGroup && isPlaceholder(sub[d.valueGroup]) {
					return m
				}
				idx := strings.LastIndex(m, sub[d.valueGroup])
				if idx < 0 {
					return m
				}
				return m[:idx] + "[redacted " + cat + "]"
			})
			continue
		}
		s = d.re.ReplaceAllString(s, "[redacted "+cat+"]")
	}
	return s
}

func Categories(f []Finding) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range f {
		if !seen[x.Category] {
			seen[x.Category] = true
			out = append(out, x.Category)
		}
	}
	return out
}
