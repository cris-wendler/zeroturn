// Package verify runs the configured validation steps directly.
//
// Each step is an argument array executed without a shell. Full output is
// written to a log under the repository's .git directory, and only a short
// redacted excerpt is printed when a step fails.
package verify

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/security"
)

type StepResult struct {
	Name     string  `json:"name"`
	Command  string  `json:"command"`
	Status   string  `json:"status"`
	ExitCode int     `json:"exitCode"`
	Seconds  float64 `json:"seconds"`
	LogFile  string  `json:"logFile,omitempty"`
	Excerpt  string  `json:"excerpt,omitempty"`
}

type Result struct {
	Steps     []StepResult `json:"steps"`
	Passed    int          `json:"passed"`
	Failed    int          `json:"failed"`
	Skipped   int          `json:"skipped"`
	Seconds   float64      `json:"seconds"`
	Cancelled bool         `json:"cancelled"`
}

const (
	StatusPass      = "pass"
	StatusFail      = "fail"
	StatusSkipped   = "skipped"
	StatusMissing   = "missing"
	StatusCancelled = "cancelled"
)

type Options struct {
	RepoRoot string
	// OnStep, when set, receives each step result as soon as it is known,
	// so progress streams one line per step instead of arriving at the end.
	OnStep func(StepResult)
}

// LogDir is where verify keeps a log for each step. It sits inside the
// repository's Git directory, which is not always a directory called
// .git: in a linked worktree and in a submodule, .git is a file holding
// the path of the real one. Treating it as a directory made verify and
// ship fail outright in both, with an error that said the directory was
// not writable when the trouble was that it was not a directory.
func LogDir(repoRoot string) string {
	return filepath.Join(GitDir(repoRoot), "zeroturn", "logs")
}

// GitDir resolves the Git directory for a working tree. The one line
// gitdir: form is what Git itself writes for a worktree and a submodule,
// and reading it costs nothing, where asking Git would mean starting a
// process on a path that runs for every verify step.
func GitDir(repoRoot string) string {
	p := filepath.Join(repoRoot, ".git")
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return p
	}
	line := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return p
	}
	dir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if dir == "" {
		return p
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoRoot, dir)
	}
	return filepath.Clean(dir)
}

// MissingExecutables reports configured steps whose executable is absent,
// so a run can fail with a precise message before anything is started.
func MissingExecutables(c config.Config) []string {
	var out []string
	for _, s := range c.Verify.Steps {
		if len(s.Command) == 0 {
			continue
		}
		if _, err := exec.LookPath(s.Command[0]); err != nil {
			out = append(out, s.Command[0])
		}
	}
	return out
}

func Run(ctx context.Context, c config.Config, o Options) (Result, error) {
	start := time.Now()
	res := Result{Steps: []StepResult{}}
	if err := os.MkdirAll(LogDir(o.RepoRoot), 0700); err != nil {
		return res, err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	add := func(sr StepResult) {
		res.Steps = append(res.Steps, sr)
		if o.OnStep != nil {
			o.OnStep(sr)
		}
	}
	skipRest := func(rest []config.Step, status string) {
		for _, r := range rest {
			add(StepResult{Name: r.Name, Command: display(r.Command), Status: status})
			res.Skipped++
		}
	}

	for i, step := range c.Verify.Steps {
		if ctx.Err() != nil {
			skipRest(c.Verify.Steps[i:], StatusCancelled)
			res.Cancelled = true
			break
		}
		sr := runStep(ctx, step, o, stamp)
		add(sr)
		switch sr.Status {
		case StatusPass:
			res.Passed++
		case StatusCancelled:
			res.Cancelled = true
			res.Skipped++
		case StatusSkipped:
			res.Skipped++
		default:
			res.Failed++
		}
		if sr.Status == StatusFail || sr.Status == StatusMissing || sr.Status == StatusCancelled {
			skipRest(c.Verify.Steps[i+1:], StatusSkipped)
			break
		}
	}
	res.Seconds = time.Since(start).Seconds()
	return res, nil
}

func runStep(ctx context.Context, step config.Step, o Options, stamp string) StepResult {
	sr := StepResult{Name: step.Name, Command: display(step.Command)}
	if _, err := exec.LookPath(step.Command[0]); err != nil {
		sr.Status = StatusMissing
		sr.ExitCode = -1
		sr.Excerpt = step.Command[0] + " was not found on PATH"
		return sr
	}
	logPath := filepath.Join(LogDir(o.RepoRoot), stamp+"-"+safeName(step.Name)+".log")
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		sr.Status = StatusFail
		sr.Excerpt = "the step log could not be opened"
		return sr
	}
	defer lf.Close()
	sr.LogFile = logPath

	var tail tailBuffer
	cmd := exec.CommandContext(ctx, step.Command[0], step.Command[1:]...)
	cmd.Dir = o.RepoRoot
	cmd.Stdin = nil
	w := io.MultiWriter(&redactingWriter{w: lf}, &tail)
	cmd.Stdout = w
	cmd.Stderr = w
	setProcessGroup(cmd)

	start := time.Now()
	runErr := cmd.Start()
	if runErr == nil {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case runErr = <-done:
		case <-ctx.Done():
			killGroup(cmd)
			<-done
			sr.Status = StatusCancelled
			sr.Seconds = time.Since(start).Seconds()
			sr.Excerpt = "the step was cancelled"
			return sr
		}
	}
	sr.Seconds = time.Since(start).Seconds()

	if runErr == nil {
		sr.Status = StatusPass
		return sr
	}
	sr.Status = StatusFail
	sr.ExitCode = exitCode(runErr)
	sr.Excerpt = security.Redact(tail.String())
	return sr
}

func exitCode(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}

func display(cmd []string) string { return strings.Join(cmd, " ") }

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// redactingWriter keeps credentials out of the preserved log.
type redactingWriter struct {
	w   io.Writer
	buf bytes.Buffer
}

func (r *redactingWriter) Write(p []byte) (int, error) {
	r.buf.Write(p)
	for {
		line, err := r.buf.ReadString('\n')
		if err != nil {
			r.buf.WriteString(line)
			break
		}
		if _, werr := io.WriteString(r.w, security.Redact(line)); werr != nil {
			return len(p), werr
		}
	}
	return len(p), nil
}

const tailLines = 20

type tailBuffer struct{ lines []string }

func (t *tailBuffer) Write(p []byte) (int, error) {
	for _, line := range strings.Split(string(p), "\n") {
		t.lines = append(t.lines, line)
	}
	if len(t.lines) > tailLines*4 {
		t.lines = t.lines[len(t.lines)-tailLines*4:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	out := t.lines
	var kept []string
	for _, l := range out {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, l)
		}
	}
	if len(kept) > tailLines {
		kept = kept[len(kept)-tailLines:]
	}
	return strings.Join(kept, "\n")
}
