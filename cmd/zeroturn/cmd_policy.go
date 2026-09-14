package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/capabilities"
	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/policy"
	"github.com/cris-wendler/zeroturn/internal/trust"
)

const policyUsage = `zeroturn policy <subcommand>

  show    Print the guard thresholds in force
  check   Evaluate the current session against them, changing nothing
  set     Change one value, for example: zeroturn policy set guard.mode confirm
  reset   Restore the default thresholds
  tune    Suggest thresholds from what the gate asked and what you did
`

const policyResetUsage = `zeroturn policy reset

Restore the default guard thresholds in .zeroturn.json. The validation
steps and the Git settings are left as they are.
`

const policySetUsage = `zeroturn policy set <key> <value>

Change one guard threshold in .zeroturn.json, after showing what it will
write. Run zeroturn policy show to list the keys and their values.

Turning on strict mode asks for confirmation, states what can be
blocked, and records the approval on this machine only.
`

func cmdPolicy(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, policyUsage)
		return output.Errorf(output.ExitInvalidUsage, "zeroturn policy needs a subcommand",
			"no subcommand was given", "run zeroturn policy show")
	}
	switch args[0] {
	case "--help", "-h", "help":
		// The subcommand is read before any flag parser runs, so a
		// request for help is answered here.
		fmt.Print(policyUsage)
		return errHelp
	case "show":
		return policyShow(ctx, args[1:])
	case "check":
		return policyCheck(ctx, args[1:])
	case "set":
		return policySet(ctx, args[1:])
	case "reset":
		return policyReset(ctx, args[1:])
	case "tune":
		return cmdTune(ctx, args[1:])
	default:
		fmt.Fprint(os.Stderr, policyUsage)
		return output.Errorf(output.ExitInvalidUsage,
			"zeroturn policy has no subcommand named "+args[0],
			"the subcommand was not recognised", "run zeroturn policy show")
	}
}

func policyShow(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("policy show", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print machine readable output")
	if err := parseFlags(fs, args, policyUsage, "policy"); err != nil {
		return err
	}
	repo, err := openRepo(ctx)
	if err != nil {
		return err
	}
	c, err := loadConfig(repo.Root)
	if err != nil {
		return err
	}
	if *asJSON {
		return output.JSON(os.Stdout, c.Guard)
	}
	fmt.Println("ZEROTURN POLICY")
	fmt.Print(policyTable(c))
	fmt.Println(modeDescription(c.Guard.Mode))
	if c.Guard.Mode == config.ModeStrict {
		if st, serr := openStore(); serr == nil && !trust.StrictApproved(st, repo.Root) {
			fmt.Println("Strict mode is requested by " + config.FileName + " but has not been approved on this machine, so the gate asks instead of denying. Run zeroturn policy set guard.mode strict to approve it.")
		}
	}
	return nil
}

func policyTable(c config.Config) string {
	return output.Table([][2]string{
		{"mode", c.Guard.Mode},
		{"context warn", pctText(c.Guard.Context.Warn)},
		{"context confirm", pctText(c.Guard.Context.Confirm)},
		{"context critical", pctText(c.Guard.Context.Critical)},
		{"five hour warn", pctText(c.Guard.Limits.FiveHourWarn)},
		{"seven day warn", pctText(c.Guard.Limits.SevenDayWarn)},
		{"rate projection", c.Guard.Limits.Projection},
		{"duration warn", fmt.Sprintf("%d minutes", c.Guard.Session.DurationWarnMinutes)},
		{"active subagents warn", fmt.Sprintf("%d", c.Guard.Session.ActiveSubagentsWarn)},
		{"subagent starts warn", fmt.Sprintf("%d", c.Guard.Session.SubagentStartsWarn)},
		{"credential guard", c.Guard.Credentials.Mode},
		{"prompt guard", c.Guard.Credentials.Prompts},
	})
}

func pctText(v int) string { return strconv.Itoa(v) + "%" }

func modeDescription(mode string) string {
	switch mode {
	case config.ModeObserve:
		return "Observe mode displays session condition and never blocks a tool."
	case config.ModeConfirm:
		return "Confirm mode asks for approval before a new subagent when a threshold has been crossed."
	case config.ModeStrict:
		return "Strict mode denies a new subagent when the critical threshold has been crossed."
	}
	return ""
}

func policyCheck(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("policy check", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print machine readable output")
	if err := parseFlags(fs, args, policyUsage, "policy"); err != nil {
		return err
	}
	repo, err := openRepo(ctx)
	if err != nil {
		return err
	}
	c, err := loadConfig(repo.Root)
	if err != nil {
		return err
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	sess, found := recentSession(st, repo.Root)
	res := policy.Evaluate(guardFor(st, repo.Root, c), sess)

	if *asJSON {
		return output.JSON(os.Stdout, struct {
			Available bool          `json:"sessionDataAvailable"`
			Policy    policy.Result `json:"policy"`
		}{found, res})
	}

	fmt.Println("ZEROTURN POLICY CHECK")
	if !found {
		fmt.Println("No recent session record for this repository. Thresholds below were evaluated against empty session data.")
	}
	fmt.Printf("mode       %s\n", res.Mode)
	fmt.Printf("level      %s\n", res.Level)
	fmt.Printf("decision   %s\n", res.Decision)
	if len(res.Triggers) == 0 {
		fmt.Println("No threshold has been crossed.")
		return nil
	}
	fmt.Println("thresholds crossed:")
	for _, t := range res.Triggers {
		fmt.Printf("  %-16s %s (limit %.0f)\n", t.Name, t.Text, t.Limit)
	}
	return nil
}

func policySet(ctx context.Context, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Print(policySetUsage)
		return errHelp
	}
	if len(args) != 2 {
		return output.Errorf(output.ExitInvalidUsage,
			"zeroturn policy set changed nothing",
			"it needs exactly one key and one value",
			"for example: zeroturn policy set guard.context.confirm 85")
	}
	repo, err := openRepo(ctx)
	if err != nil {
		return err
	}
	c, err := loadConfig(repo.Root)
	if err != nil {
		return err
	}
	key, value := args[0], args[1]
	if err := applyPolicyKey(&c, key, value); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		if ve, ok := err.(config.ValidationError); ok {
			return output.Errorf(output.ExitInvalidUsage,
				"zeroturn policy set changed nothing",
				ve.Field+" "+ve.Detail, ve.Fix)
		}
		return err
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	enablingStrict := key == "guard.mode" && c.Guard.Mode == config.ModeStrict
	if enablingStrict {
		if err := confirmStrict(c); err != nil {
			return err
		}
	}
	if key == "guard.credentials.prompts" && c.Guard.Credentials.Prompts == config.PromptsBlock {
		if err := confirmPromptGuard(); err != nil {
			return err
		}
	}
	if err := config.Save(repo.Root, c); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn could not write the configuration",
			err.Error(), "check that "+config.FileName+" is writable")
	}
	switch {
	case enablingStrict:
		if err := trust.ApproveStrict(st, repo.Root); err != nil {
			return output.Errorf(output.ExitInternal, "zeroturn wrote the mode but could not record the approval",
				err.Error(), "check that your user data directory is writable, until then the gate asks instead of denying")
		}
	case key == "guard.mode":
		if err := trust.RevokeStrict(st, repo.Root); err != nil {
			return output.Errorf(output.ExitInternal, "zeroturn wrote the mode but could not withdraw the Strict approval",
				err.Error(), "check that your user data directory is writable")
		}
	}
	fmt.Printf("%s is now %s\n", key, value)
	if key == "guard.credentials.prompts" {
		fmt.Println("Run zeroturn integrate claude --plan to add or remove the prompt hook in the harness settings.")
	}
	return nil
}

// confirmPromptGuard states both costs before the prompt guard is
// switched on: ZeroTurn starts reading messages, and a stopped message
// is erased by the harness rather than returned to the developer.
func confirmPromptGuard() error {
	fmt.Println("ZEROTURN PROMPT GUARD")
	fmt.Println()
	fmt.Println("With this on, every message you send in this repository passes through ZeroTurn first.")
	fmt.Println("It is read in memory, checked for credentials, and never stored, logged, or sent anywhere.")
	fmt.Println("Everywhere else, ZeroTurn still never reads what you write.")
	fmt.Println()
	fmt.Println("A message that carries a credential is stopped. The harness erases it, so a long message")
	fmt.Println("is lost rather than handed back. The value is never shown in the explanation.")
	fmt.Println()
	fmt.Println("It looks for high confidence patterns, so it catches a common mistake, not every one.")
	fmt.Println("To turn it off later: zeroturn policy set guard.credentials.prompts off")
	fmt.Println()
	ok, err := confirm("Let ZeroTurn read your messages in this repository to look for credentials?")
	if err != nil {
		return err
	}
	if !ok {
		return output.Errorf(output.ExitDeclined, "zeroturn policy set changed nothing",
			"the prompt guard was not confirmed", "run the command again when you are ready")
	}
	return nil
}

// harnessVersion reports the installed harness release. It is a variable
// so tests can run without the harness installed.
var harnessVersion = func() (string, error) {
	out, err := exec.Command("claude", "--version").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", fmt.Errorf("claude --version printed nothing")
	}
	return fields[0], nil
}

// confirmStrict carries out the steps the contract requires before Strict
// can deny anything: show the policy, state what can be blocked and how to
// undo it, check the harness, run the fixture compatibility test, and ask.
func confirmStrict(c config.Config) error {
	fmt.Println("ZEROTURN STRICT MODE")
	fmt.Print(policyTable(c))
	fmt.Println()
	fmt.Printf("Strict mode denies a new subagent when context use reaches %d%%.\n", c.Guard.Context.Critical)
	fmt.Println("Every other threshold asks for approval instead of denying. No other tool is gated.")
	fmt.Println("To turn it off, run zeroturn policy set guard.mode confirm, or observe.")
	fmt.Println("The approval applies to this repository on this machine only.")
	fmt.Println()

	v, err := harnessVersion()
	if err != nil {
		return output.Errorf(output.ExitNoIntegration, "zeroturn policy set changed nothing",
			"the claude executable was not found, so there is no harness for Strict mode to act on",
			"install the harness and run zeroturn integrate claude --plan, then try again")
	}
	if v == capabilities.ClaudeTestedVersion {
		fmt.Printf("harness         claude %s, the version on which denial was tested\n", v)
	} else {
		fmt.Printf("harness         claude %s, denial was tested on %s only\n", v, capabilities.ClaudeTestedVersion)
	}
	for _, ck := range compatFixtures() {
		if ck.Status == checkFail {
			return output.Errorf(output.ExitPolicyFailure, "zeroturn policy set changed nothing",
				"the compatibility test failed: "+ck.Name+", "+ck.Detail,
				"keep guard.mode on confirm and report the failure")
		}
	}
	fmt.Println("compatibility   observe allows, confirm asks, and strict denies on fixture events")
	fmt.Println("                run zeroturn doctor --compat --live to see a denial in a real session")
	fmt.Println()

	ok, err := confirm("Enable Strict mode for this repository?")
	if err != nil {
		return err
	}
	if !ok {
		return output.Errorf(output.ExitDeclined, "zeroturn policy set changed nothing",
			"Strict mode was not confirmed", "run the command again when you are ready")
	}
	return nil
}

func applyPolicyKey(c *config.Config, key, value string) error {
	intVal := func() (int, error) {
		n, err := strconv.Atoi(value)
		if err != nil {
			return 0, output.Errorf(output.ExitInvalidUsage,
				"zeroturn policy set changed nothing",
				"value "+strconv.Quote(value)+" for "+key+" is not a whole number",
				"supply a whole number")
		}
		return n, nil
	}
	switch key {
	case "guard.mode":
		v := strings.ToLower(value)
		if v != config.ModeObserve && v != config.ModeConfirm && v != config.ModeStrict {
			return output.Errorf(output.ExitInvalidUsage,
				"zeroturn policy set changed nothing",
				"value "+strconv.Quote(value)+" is not a guard mode",
				"use observe, confirm, or strict")
		}
		c.Guard.Mode = v
	case "guard.context.warn":
		n, err := intVal()
		if err != nil {
			return err
		}
		c.Guard.Context.Warn = n
	case "guard.context.confirm":
		n, err := intVal()
		if err != nil {
			return err
		}
		c.Guard.Context.Confirm = n
	case "guard.context.critical":
		n, err := intVal()
		if err != nil {
			return err
		}
		c.Guard.Context.Critical = n
	case "guard.limits.fiveHourWarn":
		n, err := intVal()
		if err != nil {
			return err
		}
		c.Guard.Limits.FiveHourWarn = n
	case "guard.limits.sevenDayWarn":
		n, err := intVal()
		if err != nil {
			return err
		}
		c.Guard.Limits.SevenDayWarn = n
	case "guard.limits.projection":
		v := strings.ToLower(value)
		if v != config.ProjectionOn && v != config.ProjectionOff {
			return output.Errorf(output.ExitInvalidUsage,
				"zeroturn policy set changed nothing",
				"value "+strconv.Quote(value)+" is not a projection setting",
				"use on, or off")
		}
		c.Guard.Limits.Projection = v
	case "guard.session.durationWarnMinutes":
		n, err := intVal()
		if err != nil {
			return err
		}
		c.Guard.Session.DurationWarnMinutes = n
	case "guard.session.activeSubagentsWarn":
		n, err := intVal()
		if err != nil {
			return err
		}
		c.Guard.Session.ActiveSubagentsWarn = n
	case "guard.credentials.mode":
		v := strings.ToLower(value)
		if v != config.CredentialOff && v != config.CredentialAsk && v != config.CredentialDeny {
			return output.Errorf(output.ExitInvalidUsage,
				"zeroturn policy set changed nothing",
				"value "+strconv.Quote(value)+" is not a credential guard mode",
				"use off, ask, or deny")
		}
		c.Guard.Credentials.Mode = v
	case "guard.credentials.prompts":
		v := strings.ToLower(value)
		if v != config.PromptsOff && v != config.PromptsBlock {
			return output.Errorf(output.ExitInvalidUsage,
				"zeroturn policy set changed nothing",
				"value "+strconv.Quote(value)+" is not a prompt guard setting",
				"use off, or block")
		}
		c.Guard.Credentials.Prompts = v
	case "guard.session.subagentStartsWarn":
		n, err := intVal()
		if err != nil {
			return err
		}
		c.Guard.Session.SubagentStartsWarn = n
	default:
		return output.Errorf(output.ExitInvalidUsage,
			"zeroturn policy set changed nothing",
			"there is no policy key named "+key,
			"run zeroturn policy show to list the keys")
	}
	return nil
}

func policyReset(ctx context.Context, args []string) error {
	// This command writes, and it took no notice of its arguments, so
	// asking it for help restored the defaults instead of explaining
	// itself. Anything that is not understood stops it now.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Print(policyResetUsage)
			return errHelp
		}
		return output.Errorf(output.ExitInvalidUsage, "zeroturn policy reset changed nothing",
			"it accepts no arguments and got "+a, "run zeroturn policy reset")
	}
	repo, err := openRepo(ctx)
	if err != nil {
		return err
	}
	c, err := loadConfig(repo.Root)
	if err != nil {
		return err
	}
	d := config.Default()
	c.Guard = d.Guard
	if err := config.Save(repo.Root, c); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn could not write the configuration",
			err.Error(), "check that "+config.FileName+" is writable")
	}
	if st, serr := openStore(); serr == nil {
		trust.RevokeStrict(st, repo.Root)
	}
	fmt.Println("Guard thresholds restored to defaults. Validation steps and Git settings were left unchanged.")
	return nil
}
