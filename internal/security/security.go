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
	// anchors are literal strings, one of which must appear in a line
	// before the expression is run at all. A regular expression call
	// costs a few microseconds even when it cannot match, and almost
	// every line of an ordinary file cannot match any of these.
	anchors []string
	// fold marks a detector whose anchors are compared against the line
	// in lower case.
	fold bool
}

var detectors = []detector{
	{"private key", regexp.MustCompile(`-----BEGIN (?:RSA |DSA |EC |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY( BLOCK)?-----`), 0, []string{"-----BEGIN"}, false},
	{"aws access key id", regexp.MustCompile(`\b(?:AKIA|ASIA|ABIA|ACCA|A3T[A-Z0-9])[A-Z0-9]{16}\b`), 0, []string{"AKIA", "ASIA", "ABIA", "ACCA", "A3T"}, false},
	{"github token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`), 0, []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}, false},
	{"slack token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), 0, []string{"xox"}, false},
	{"stripe secret key", regexp.MustCompile(`\b(?:sk|rk)_live_[A-Za-z0-9]{16,}\b`), 0, []string{"sk_live_", "rk_live_"}, false},
	{"google api key", regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`), 0, []string{"AIza"}, false},
	{"anthropic api key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_\-]{24,}`), 0, []string{"sk-ant-"}, false},
	{"openai api key", regexp.MustCompile(`\bsk-proj-[A-Za-z0-9_\-]{20,}`), 0, []string{"sk-proj-"}, false},
	{"npm token", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`), 0, []string{"npm_"}, false},
	{"pypi token", regexp.MustCompile(`\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_\-]{20,}`), 0, []string{"pypi-"}, false},
	{"json web token", regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`), 0, []string{"eyJ"}, false},
	{"github fine grained token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), 0, []string{"github_pat_"}, false},
	{"gitlab token", regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{20,}`), 0, []string{"glpat-"}, false},
	{"google oauth client secret", regexp.MustCompile(`\bGOCSPX-[A-Za-z0-9_\-]{20,}`), 0, []string{"GOCSPX-"}, false},
	{"hugging face token", regexp.MustCompile(`\bhf_[A-Za-z0-9]{30,}`), 0, []string{"hf_"}, false},
	{"sendgrid api key", regexp.MustCompile(`\bSG\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}`), 0, []string{"SG."}, false},
	{"twilio api key", regexp.MustCompile(`\bSK[0-9a-fA-F]{32}\b`), 0, []string{"SK"}, false},
	{"shopify access token", regexp.MustCompile(`\bshp(at|ss|ca|pa)_[0-9a-fA-F]{32}\b`), 0, []string{"shpat_", "shpss_", "shpca_", "shppa_"}, false},
	{"square access token", regexp.MustCompile(`\bsq0(atp|csp)-[A-Za-z0-9_\-]{20,}`), 0, []string{"sq0atp-", "sq0csp-"}, false},
	{"mailgun api key", regexp.MustCompile(`\bkey-[0-9a-f]{32}\b`), 0, []string{"key-"}, false},
	{"new relic key", regexp.MustCompile(`\bNRAK-[A-Z0-9]{20,}`), 0, []string{"NRAK-"}, false},
	{"telegram bot token", regexp.MustCompile(`\b[0-9]{8,10}:AA[A-Za-z0-9_\-]{32,}`), 0, []string{":AA"}, false},
	{"slack webhook", regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Za-z0-9]+/B[A-Za-z0-9]+/[A-Za-z0-9]{16,}`), 0, []string{"hooks.slack.com"}, false},
	{"azure storage key", regexp.MustCompile(`AccountKey=[A-Za-z0-9+/=]{60,}`), 0, []string{"AccountKey="}, false},
	{"putty private key", regexp.MustCompile(`PuTTY-User-Key-File-[0-9]+:`), 0, []string{"PuTTY-User-Key-File"}, false},
	{"google service account key", regexp.MustCompile(`"type"\s*:\s*"service_account"`), 0, []string{"service_account"}, false},
	{"basic authorization header", regexp.MustCompile(`(?i)authorization["'\s:]+basic\s+[A-Za-z0-9+/]{16,}={0,2}`), 0, []string{"authorization"}, true},
	{"bearer token header", regexp.MustCompile(`(?i)authorization["'\s:]+bearer\s+[A-Za-z0-9._\-]{24,}`), 0, []string{"authorization"}, true},
	{"credential in url", regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.\-]*://[^/\s:@"']+:[^/\s:@"']+@[^\s"']+`), 0, []string{"://"}, false},
	{"credential assignment", regexp.MustCompile(`(?i)\b(?:password|passwd|secret|api[_\-]?key|access[_\-]?token|auth[_\-]?token|private[_\-]?key|client[_\-]?secret)\b\s*[:=]\s*["']?([A-Za-z0-9/+=_\-]{16,})["']?`), 1, []string{"password", "passwd", "secret", "api_key", "apikey", "api-key", "access_token", "accesstoken", "access-token", "auth_token", "authtoken", "auth-token", "private_key", "privatekey", "private-key", "client_secret", "clientsecret", "client-secret"}, true},
}

// placeholders keep example configuration and documentation from being
// reported. A value that names itself as an example is not a credential.
// A value that names itself as an example is not a credential. These
// match from the start of the value, because a placeholder is usually a
// phrase rather than one word: "your-api-key-here", "example_token".
var placeholders = regexp.MustCompile(`(?i)^(?:x{3,}|\.{3,}|changeme|placeholder|redacted|example|sample|dummy|test|fake|insert|replace|enter|add)[-_ ]?.*$|` +
	`^(?:your|my|our|the)[-_ ].*$|` +
	`^(?:<.*>|\$\{.*\}|\$[A-Z_]+|%[A-Za-z_]+%|\{\{.*\}\})$|` +
	`^(?:null|none|nil|true|false|todo|tbd|abc123|secret|password|passwd|token|apikey|api_key|key|value)$|` +
	`^(?:0+|1234567890.*)$`)

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
		lowered := ""
		for _, d := range detectors {
			candidate := text
			if d.fold {
				if lowered == "" {
					lowered = strings.ToLower(text)
				}
				candidate = lowered
			}
			if !anchored(candidate, d.anchors) {
				continue
			}
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

// anchored reports whether any of the literal anchors appears in the
// line. Every detector has at least one, so a line without any of them
// cannot match and is never handed to a regular expression.
func anchored(line string, anchors []string) bool {
	for _, a := range anchors {
		if strings.Contains(line, a) {
			return true
		}
	}
	return false
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
