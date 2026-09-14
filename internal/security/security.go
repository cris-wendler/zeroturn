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
	"sync"
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
	// pattern is compiled when the detector is first needed rather than
	// at package initialisation. This executable starts fresh for every
	// hook call, including each status line repaint, and most calls never
	// scan anything. The anchors below mean that even a scan usually
	// compiles none of these.
	pattern string
	once    sync.Once
	re      *regexp.Regexp
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

var detectors = []*detector{
	{category: "private key", pattern: `-----BEGIN (?:RSA |DSA |EC |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY( BLOCK)?-----`, valueGroup: 0, anchors: []string{"-----BEGIN"}, fold: false},
	{category: "aws access key id", pattern: `\b(?:AKIA|ASIA|ABIA|ACCA|A3T[A-Z0-9])[A-Z0-9]{16}\b`, valueGroup: 0, anchors: []string{"AKIA", "ASIA", "ABIA", "ACCA", "A3T"}, fold: false},
	{category: "github token", pattern: `\bgh[pousr]_[A-Za-z0-9]{36,}\b`, valueGroup: 0, anchors: []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}, fold: false},
	{category: "slack token", pattern: `\bxox[baprs]-[A-Za-z0-9-]{10,}`, valueGroup: 0, anchors: []string{"xox"}, fold: false},
	{category: "stripe secret key", pattern: `\b(?:sk|rk)_live_[A-Za-z0-9]{16,}\b`, valueGroup: 0, anchors: []string{"sk_live_", "rk_live_"}, fold: false},
	{category: "google api key", pattern: `\bAIza[0-9A-Za-z_\-]{35}\b`, valueGroup: 0, anchors: []string{"AIza"}, fold: false},
	{category: "anthropic api key", pattern: `\bsk-ant-[A-Za-z0-9_\-]{24,}`, valueGroup: 0, anchors: []string{"sk-ant-"}, fold: false},
	{category: "openai api key", pattern: `\bsk-proj-[A-Za-z0-9_\-]{20,}`, valueGroup: 0, anchors: []string{"sk-proj-"}, fold: false},
	{category: "npm token", pattern: `\bnpm_[A-Za-z0-9]{36}\b`, valueGroup: 0, anchors: []string{"npm_"}, fold: false},
	{category: "pypi token", pattern: `\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_\-]{20,}`, valueGroup: 0, anchors: []string{"pypi-"}, fold: false},
	{category: "json web token", pattern: `\beyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`, valueGroup: 0, anchors: []string{"eyJ"}, fold: false},
	{category: "github fine grained token", pattern: `\bgithub_pat_[A-Za-z0-9_]{20,}`, valueGroup: 0, anchors: []string{"github_pat_"}, fold: false},
	{category: "gitlab token", pattern: `\bglpat-[A-Za-z0-9_\-]{20,}`, valueGroup: 0, anchors: []string{"glpat-"}, fold: false},
	{category: "google oauth client secret", pattern: `\bGOCSPX-[A-Za-z0-9_\-]{20,}`, valueGroup: 0, anchors: []string{"GOCSPX-"}, fold: false},
	{category: "hugging face token", pattern: `\bhf_[A-Za-z0-9]{30,}`, valueGroup: 0, anchors: []string{"hf_"}, fold: false},
	{category: "sendgrid api key", pattern: `\bSG\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}`, valueGroup: 0, anchors: []string{"SG."}, fold: false},
	{category: "twilio api key", pattern: `\bSK[0-9a-fA-F]{32}\b`, valueGroup: 0, anchors: []string{"SK"}, fold: false},
	{category: "shopify access token", pattern: `\bshp(at|ss|ca|pa)_[0-9a-fA-F]{32}\b`, valueGroup: 0, anchors: []string{"shpat_", "shpss_", "shpca_", "shppa_"}, fold: false},
	{category: "square access token", pattern: `\bsq0(atp|csp)-[A-Za-z0-9_\-]{20,}`, valueGroup: 0, anchors: []string{"sq0atp-", "sq0csp-"}, fold: false},
	{category: "mailgun api key", pattern: `\bkey-[0-9a-f]{32}\b`, valueGroup: 0, anchors: []string{"key-"}, fold: false},
	{category: "new relic key", pattern: `\bNRAK-[A-Z0-9]{20,}`, valueGroup: 0, anchors: []string{"NRAK-"}, fold: false},
	{category: "telegram bot token", pattern: `\b[0-9]{8,10}:AA[A-Za-z0-9_\-]{32,}`, valueGroup: 0, anchors: []string{":AA"}, fold: false},
	{category: "slack webhook", pattern: `https://hooks\.slack\.com/services/T[A-Za-z0-9]+/B[A-Za-z0-9]+/[A-Za-z0-9]{16,}`, valueGroup: 0, anchors: []string{"hooks.slack.com"}, fold: false},
	{category: "azure storage key", pattern: `AccountKey=[A-Za-z0-9+/=]{60,}`, valueGroup: 0, anchors: []string{"AccountKey="}, fold: false},
	{category: "putty private key", pattern: `PuTTY-User-Key-File-[0-9]+:`, valueGroup: 0, anchors: []string{"PuTTY-User-Key-File"}, fold: false},
	{category: "google service account key", pattern: `"type"\s*:\s*"service_account"`, valueGroup: 0, anchors: []string{"service_account"}, fold: false},
	{category: "basic authorization header", pattern: `(?i)authorization["'\s:]+basic\s+[A-Za-z0-9+/]{16,}={0,2}`, valueGroup: 0, anchors: []string{"authorization"}, fold: true},
	{category: "bearer token header", pattern: `(?i)authorization["'\s:]+bearer\s+[A-Za-z0-9._\-]{24,}`, valueGroup: 0, anchors: []string{"authorization"}, fold: true},
	{category: "credential in url", pattern: `\b[a-zA-Z][a-zA-Z0-9+.\-]*://[^/\s:@"']+:[^/\s:@"']+@[^\s"']+`, valueGroup: 0, anchors: []string{"://"}, fold: false},
	{category: "credential assignment", pattern: `(?i)\b(?:password|passwd|secret|api[_\-]?key|access[_\-]?token|auth[_\-]?token|private[_\-]?key|client[_\-]?secret)\b\s*[:=]\s*["']?([A-Za-z0-9/+=_\-]{16,})["']?`, valueGroup: 1, anchors: []string{"password", "passwd", "secret", "api_key", "apikey", "api-key", "access_token", "accesstoken", "access-token", "auth_token", "authtoken", "auth-token", "private_key", "privatekey", "private-key", "client_secret", "clientsecret", "client-secret"}, fold: true},
}

// expr compiles the pattern the first time this detector is actually
// reached, which is after one of its anchors appeared in a line.
func (d *detector) expr() *regexp.Regexp {
	d.once.Do(func() { d.re = regexp.MustCompile(d.pattern) })
	return d.re
}

// placeholders keep example configuration and documentation from being
// reported. A value that names itself as an example is not a credential.
// A value that names itself as an example is not a credential. These
// match from the start of the value, because a placeholder is usually a
// phrase rather than one word: "your-api-key-here", "example_token".
const placeholderPattern = `(?i)^(?:x{3,}|\.{3,}|changeme|placeholder|redacted|example|sample|dummy|test|fake|insert|replace|enter|add)[-_ ]?.*$|` +
	`^(?:your|my|our|the)[-_ ].*$|` +
	`^(?:<.*>|\$\{.*\}|\$[A-Z_]+|%[A-Za-z_]+%|\{\{.*\}\})$|` +
	`^(?:null|none|nil|true|false|todo|tbd|abc123|secret|password|passwd|token|apikey|api_key|key|value)$|` +
	`^(?:0+|1234567890.*)$`

var (
	placeholdersOnce sync.Once
	placeholders     *regexp.Regexp
)

func placeholderExpr() *regexp.Regexp {
	placeholdersOnce.Do(func() { placeholders = regexp.MustCompile(placeholderPattern) })
	return placeholders
}

func isPlaceholder(v string) bool {
	if placeholderExpr().MatchString(v) {
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
			m := d.expr().FindStringSubmatch(text)
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

// redactors are the same patterns without word boundaries. Detection
// keeps the boundaries, so a finding is precise. Redaction drops them,
// because a value glued to other text, which happens in logs, still has
// to be removed: leaving one behind is worse than removing too much.
var (
	redactorsOnce  sync.Once
	builtRedactors []*detector
)

// redactorList builds the loose patterns on first use. Redaction runs on
// command output, so a session that only reports its own condition never
// pays for it.
func redactorList() []*detector {
	redactorsOnce.Do(func() { builtRedactors = buildRedactors() })
	return builtRedactors
}

func buildRedactors() []*detector {
	out := make([]*detector, 0, len(detectors))
	for _, d := range detectors {
		out = append(out, &detector{
			category:   d.category,
			pattern:    strings.ReplaceAll(d.pattern, `\b`, ""),
			valueGroup: d.valueGroup,
			anchors:    d.anchors,
			fold:       d.fold,
		})
	}
	return out
}

// Redact replaces credential shaped text with a category label. It is
// applied to every command output ZeroTurn prints or writes to a log.
func Redact(s string) string {
	for _, d := range redactorList() {
		cat := d.category
		if d.valueGroup > 0 {
			re := d.expr()
			s = re.ReplaceAllStringFunc(s, func(m string) string {
				sub := re.FindStringSubmatch(m)
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
		s = d.expr().ReplaceAllString(s, "[redacted "+cat+"]")
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
