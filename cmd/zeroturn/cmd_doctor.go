package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/policy"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/trust"
)

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

const (
	checkOK   = "ok"
	checkWarn = "warn"
	checkFail = "fail"
)

const doctorUsage = `zeroturn doctor [--json] [--compat [--live]]

Check the local installation: the executable, the harnesses found, the
state directory, and the repository.

--compat tests that guard decisions are produced correctly, from
fixtures. Adding --live starts one real coding session in a throwaway
repository and reports what the harness actually did with the answer.
`

func cmdDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print machine readable output")
	compat := fs.Bool("compat", false, "test that guard decisions are produced correctly")
	live := fs.Bool("live", false, "with --compat, start one real coding session to test the harness response")
	if err := parseFlags(fs, args, doctorUsage, "doctor"); err != nil {
		return err
	}

	var checks []check
	checks = append(checks, checkExecutable("git", "git", "--version"))
	checks = append(checks, checkExecutable("claude", "claude", "--version"))
	checks = append(checks, checkExecutable("copilot", "copilot", "--version"))
	checks = append(checks, checkState())
	checks = append(checks, checkRepo(ctx)...)
	checks = append(checks, check{"short alias", checkWarn,
		"ZeroTurn does not install a zt alias, because zt is the naming convention of an unrelated project. Add your own shell alias if you want one."})

	if *compat {
		checks = append(checks, compatFixtures()...)
		if *live {
			checks = append(checks, compatLive(ctx)...)
		} else {
			checks = append(checks, check{"live harness test", checkWarn,
				"not run. Add --live to start one short coding session and confirm the harness honours a denial."})
		}
	}

	if *asJSON {
		return output.JSON(os.Stdout, checks)
	}

	c := output.NewColor(os.Stdout, "")
	fmt.Println("ZEROTURN DOCTOR")
	failed := 0
	for _, ck := range checks {
		label := c.Green("OK  ")
		switch ck.Status {
		case checkWarn:
			label = c.Yellow("WARN")
		case checkFail:
			label = c.Red("FAIL")
			failed++
		}
		fmt.Printf("%s  %-22s %s\n", label, ck.Name, ck.Detail)
	}
	if failed > 0 {
		return output.Errorf(output.ExitPolicyFailure, "zeroturn doctor found problems",
			fmt.Sprintf("%d checks failed", failed), "fix the failures listed above")
	}
	return nil
}

func checkExecutable(name, bin string, args ...string) check {
	p, err := exec.LookPath(bin)
	if err != nil {
		status := checkWarn
		detail := bin + " was not found on PATH"
		if bin == "git" {
			status = checkFail
			detail = "git was not found on PATH, the Direct Lane cannot run without it"
		}
		return check{name, status, detail}
	}
	out, err := exec.Command(p, args...).Output()
	v := strings.TrimSpace(string(bytes.SplitN(out, []byte("\n"), 2)[0]))
	if err != nil || v == "" {
		return check{name, checkOK, "found at " + p}
	}
	return check{name, checkOK, v}
}

func checkState() check {
	st, err := state.Open()
	if err != nil {
		return check{"local state", checkFail, "the state directory could not be opened: " + err.Error()}
	}
	probe := filepath.Join(st.Dir(), ".write-probe")
	if werr := state.AtomicWrite(probe, []byte("ok\n"), 0600); werr != nil {
		return check{"local state", checkFail, st.Dir() + " is not writable"}
	}
	os.Remove(probe)
	return check{"local state", checkOK, st.Dir()}
}

func checkRepo(ctx context.Context) []check {
	repo, err := openRepo(ctx)
	if err != nil {
		return []check{{"repository", checkWarn, "not inside a Git repository, repository checks were skipped"}}
	}
	out := []check{{"repository", checkOK, repo.Root}}

	if !config.Exists(repo.Root) {
		out = append(out, check{"configuration", checkWarn, "no " + config.FileName + ", run zeroturn init"})
		return out
	}
	c, lerr := config.Load(repo.Root)
	if lerr != nil {
		out = append(out, check{"configuration", checkFail, lerr.Error()})
		return out
	}
	out = append(out, check{"configuration", checkOK,
		fmt.Sprintf("mode %s, %d validation steps", c.Guard.Mode, len(c.Verify.Steps))})

	if st, serr := state.Open(); serr == nil {
		ts := trust.Check(st, repo.Root, c)
		if ts.Trusted {
			out = append(out, check{"command approval", checkOK, "the configured commands are approved on this machine"})
		} else {
			out = append(out, check{"command approval", checkWarn, ts.Reason + ", run zeroturn verify --approve"})
		}
	}

	installed, paths := installedIntegration(repo.Root)
	if installed == "" {
		out = append(out, check{"claude integration", checkWarn,
			"not installed for this repository, run zeroturn integrate claude --plan"})
		return out
	}
	out = append(out, check{"claude integration", checkOK, "installed in " + installed})

	// A hook pointing at an executable that has moved or been removed
	// fails in silence, which for a guard is the worst way to fail.
	self := selfPath()
	for _, p := range paths {
		if samePath(p, self) {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			out = append(out, check{"integration path", checkFail,
				"the settings point at " + p + ", which is not there, so the hooks do nothing. Run zeroturn integrate claude --apply to repoint them"})
		} else {
			out = append(out, check{"integration path", checkWarn,
				"the settings point at " + p + ", this is " + self + ". Run zeroturn integrate claude --apply to repoint them"})
		}
		return out
	}
	out = append(out, check{"integration path", checkOK, "the settings point at this executable"})
	return out
}

// compatFixtures proves that each mode produces the decision it claims,
// using fixture events rather than a live session.
// installedIntegration reports which settings file holds ZeroTurn's
// entries, and every executable path those entries name.
func installedIntegration(root string) (string, []string) {
	for _, name := range []string{"settings.local.json", "settings.json"} {
		p := filepath.Join(root, ".claude", name)
		b, err := ioutil.ReadFile(p)
		if err != nil {
			continue
		}
		var doc struct {
			StatusLine struct {
				Command string `json:"command"`
			} `json:"statusLine"`
			Hooks map[string][]struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		if json.Unmarshal(b, &doc) != nil {
			continue
		}
		seen := map[string]bool{}
		var paths []string
		add := func(command string) {
			if !owned(command) {
				return
			}
			if path := installedPath(command); path != "" && !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
		}
		add(doc.StatusLine.Command)
		for _, entries := range doc.Hooks {
			for _, entry := range entries {
				for _, h := range entry.Hooks {
					add(h.Command)
				}
			}
		}
		if len(paths) > 0 {
			return p, paths
		}
	}
	return "", nil
}

func compatFixtures() []check {
	var out []check
	sess := state.Session{
		SessionID:       "fixture",
		ContextPct:      f64(95),
		ActiveSubagents: 3,
		SubagentStarts:  6,
	}
	cases := []struct {
		mode string
		want string
	}{
		{config.ModeObserve, policy.DecisionAllow},
		{config.ModeConfirm, policy.DecisionAsk},
		{config.ModeStrict, policy.DecisionDeny},
	}
	for _, tc := range cases {
		c := config.Default()
		c.Guard.Mode = tc.mode
		// Strict is approved here because this check asks what the policy
		// table does, not what this machine has approved. Without it the
		// Strict case would be downgraded and the check would read as a
		// failure on a machine that simply has not approved Strict.
		got := policy.Evaluate(policy.NewGuard(c, true), sess)
		if got.Decision != tc.want {
			out = append(out, check{"policy " + tc.mode, checkFail,
				"expected " + tc.want + " at a critical threshold, got " + got.Decision})
			continue
		}
		detail := "produces " + got.Decision + " at a critical threshold"
		if got.Reason != "" {
			detail += ", reason: " + got.Reason
		}
		out = append(out, check{"policy " + tc.mode, checkOK, detail})
	}

	payload := `{"session_id":"fixture","hook_event_name":"PreToolUse","tool_name":"Agent",` +
		`"tool_input":{"prompt":"secret instructions","subagent_type":"Explore"},` +
		`"transcript_path":"/should/not/be/read.jsonl"}`
	e, err := events.ParseClaude(strings.NewReader(payload), "PreToolUse")
	if err != nil {
		out = append(out, check{"event parsing", checkFail, err.Error()})
		return out
	}
	blob, _ := json.Marshal(e)
	if bytes.Contains(blob, []byte("secret instructions")) || bytes.Contains(blob, []byte("should/not/be/read")) {
		out = append(out, check{"event redaction", checkFail,
			"the parsed event retained content it must discard"})
	} else {
		out = append(out, check{"event redaction", checkOK,
			"the subagent prompt and transcript path are discarded on receipt"})
	}
	return out
}

// compatLive starts one short coding session with a temporary settings
// file, a temporary repository, and a temporary ZeroTurn state directory.
// It writes nothing to the user's own configuration, state, or repository.
//
// A non interactive session sends no status line, so the gate would have
// no context value to act on. The test therefore fixes the session
// identifier and seeds a critical context reading for it.
func compatLive(ctx context.Context) []check {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return []check{{"live harness test", checkWarn, "claude was not found on PATH"}}
	}
	base, err := ioutil.TempDir("", "zeroturn-compat-")
	if err != nil {
		return []check{{"live harness test", checkFail, "a temporary directory could not be created"}}
	}
	defer os.RemoveAll(base)
	dir := filepath.Join(base, "repo")
	stateDir := filepath.Join(base, "state")
	if out, gerr := exec.CommandContext(ctx, "git", "init", "--quiet", dir).CombinedOutput(); gerr != nil {
		return []check{{"live harness test", checkFail, "a temporary repository could not be created: " + strings.TrimSpace(string(out))}}
	}
	if resolved, rerr := filepath.EvalSymlinks(dir); rerr == nil {
		dir = resolved
	}
	sessionID, err := newUUID()
	if err != nil {
		return []check{{"live harness test", checkFail, "a session identifier could not be generated"}}
	}
	if serr := seedCompatState(stateDir, dir, sessionID); serr != nil {
		return []check{{"live harness test", checkFail, "the temporary state could not be written: " + serr.Error()}}
	}

	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Agent",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": `"` + selfPath() + `" event --harness claude --event PreToolUse`,
							"timeout": 10,
						},
					},
				},
			},
		},
	}
	b, _ := json.MarshalIndent(settings, "", "  ")
	sp := filepath.Join(base, "settings.json")
	if werr := ioutil.WriteFile(sp, b, 0600); werr != nil {
		return []check{{"live harness test", checkFail, "the temporary settings file could not be written"}}
	}
	cfg := config.Default()
	cfg.Guard.Mode = config.ModeStrict
	if cerr := config.Save(dir, cfg); cerr != nil {
		return []check{{"live harness test", checkFail, "the temporary configuration could not be written"}}
	}

	cmd := exec.CommandContext(ctx, bin, "-p",
		"Use the Agent tool to launch the Explore subagent to list files here.",
		"--model", "haiku", "--settings", sp, "--session-id", sessionID,
		"--output-format", "json", "--max-turns", "3")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "ZEROTURN_STATE_DIR="+stateDir)
	out, runErr := cmd.Output()
	if runErr != nil && len(out) == 0 {
		return []check{{"live harness test", checkWarn,
			"the session could not be started, so denial could not be confirmed"}}
	}
	var result struct {
		PermissionDenials []struct {
			ToolName string `json:"tool_name"`
		} `json:"permission_denials"`
		SubagentStats struct {
			Spawned int `json:"spawned"`
		} `json:"subagent_stats"`
	}
	if json.Unmarshal(out, &result) != nil {
		return []check{{"live harness test", checkWarn, "the session result could not be read"}}
	}
	if len(result.PermissionDenials) > 0 && result.SubagentStats.Spawned == 0 {
		return []check{{"live harness test", checkOK,
			"the harness honoured a denial and no subagent started"}}
	}
	return []check{{"live harness test", checkFail,
		"the harness did not honour the denial, keep guard.mode on observe"}}
}

// seedCompatState prepares the temporary state for compatLive: Strict is
// approved for the temporary repository and the session starts at 95%
// context, above the default critical threshold.
func seedCompatState(stateDir, repoRoot, sessionID string) error {
	prev, had := os.LookupEnv("ZEROTURN_STATE_DIR")
	os.Setenv("ZEROTURN_STATE_DIR", stateDir)
	defer func() {
		if had {
			os.Setenv("ZEROTURN_STATE_DIR", prev)
		} else {
			os.Unsetenv("ZEROTURN_STATE_DIR")
		}
	}()
	st, err := state.Open()
	if err != nil {
		return err
	}
	if err := trust.ApproveStrict(st, repoRoot); err != nil {
		return err
	}
	_, err = st.Update(sessionID, state.RepoHash(repoRoot), func(s *state.Session) {
		s.ContextPct = f64(95)
	})
	return err
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func f64(v float64) *float64 { return &v }
