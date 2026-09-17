package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
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
  migrate Rewrite the file in the format this build writes
`

const policyMigrateUsage = `zeroturn policy migrate

Rewrite .zeroturn.json in the format this build writes, keeping every
value it already holds. Settings the file does not carry are written with
the value a new file would have, and a setting this build does not have is
named before it is dropped.

  --plan   Show what would change, changing nothing

A file is read whether or not it has been migrated. Running this only
makes the file say what ZeroTurn is already reading from it.
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
	case "migrate":
		return policyMigrate(ctx, args[1:])
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
		// The whole configuration, not the guard alone. Retention is a
		// setting outside the guard, so a document holding only the guard
		// would say it is the policy and be wrong. This is the shape
		// config.schema.json already describes.
		return output.JSON(os.Stdout, c)
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
	rows := make([][2]string, 0, len(config.Keys()))
	for _, k := range config.Keys() {
		rows = append(rows, [2]string{k.Label, k.Display(c)})
	}
	return output.Table(rows)
}

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
	} else {
		fmt.Println("thresholds crossed:")
		for _, t := range res.Triggers {
			fmt.Printf("  %-16s %s (limit %.0f)\n", t.Name, t.Text, t.Limit)
		}
	}
	// A threshold with no measurement behind it did not pass, it was
	// never checked, and the two are the same sentence without this.
	// doctor could already tell them apart and this command could not,
	// which is the wrong way round: this is the one a person runs to ask
	// what the gate would do.
	if found {
		printUnmeasured(policy.Unmeasured(sess), policy.AllUnmeasured(sess))
	}
	return nil
}

func printUnmeasured(absent []string, all bool) {
	if len(absent) == 0 {
		return
	}
	if all {
		fmt.Println("Nothing was measured. This session carried no values from the harness status line, " +
			"so the thresholds on " + output.List(absent) + " were not checked and cannot be crossed here. " +
			"Subagent counts are ZeroTurn's own and are still real. Run the harness in a terminal for the gate to have anything to read.")
		return
	}
	fmt.Println("Not measured in this session: " + output.List(absent) +
		". Those thresholds were not checked, rather than checked and found below the limit.")
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
	tested := false
	for _, t := range capabilities.ClaudeTestedVersions {
		if v == t {
			tested = true
			break
		}
	}
	if tested {
		fmt.Printf("harness         claude %s, a version on which a decision was tested\n", v)
	} else {
		fmt.Printf("harness         claude %s, decisions were tested on %s only\n",
			v, strings.Join(capabilities.ClaudeTestedVersions, " and "))
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

// applyPolicyKey writes one setting. The keys, what each one accepts,
// and the wording that refuses a bad value all come from the registry in
// internal/config, so this command cannot offer a key the rest of the
// program does not know about.
func applyPolicyKey(c *config.Config, key, value string) error {
	k, ok := config.Lookup(key)
	if !ok {
		return output.Errorf(output.ExitInvalidUsage,
			"zeroturn policy set changed nothing",
			"there is no policy key named "+key,
			"run zeroturn policy show to list the keys")
	}
	if err := k.Set(c, value); err != nil {
		ve, isValidation := err.(config.ValidationError)
		if !isValidation {
			return err
		}
		return output.Errorf(output.ExitInvalidUsage,
			"zeroturn policy set changed nothing", ve.Detail, ve.Fix)
	}
	return nil
}

// policyMigrate rewrites the file in the format this build writes. It is
// not needed to read a file: Load migrates in memory. It exists so that a
// file can be made to say what ZeroTurn is already reading from it, and so
// that a setting this build does not have is named rather than carried
// around unread.
func policyMigrate(ctx context.Context, args []string) error {
	plan := false
	for _, a := range args {
		switch a {
		case "--plan":
			plan = true
		case "--help", "-h":
			fmt.Print(policyMigrateUsage)
			return errHelp
		default:
			return output.Errorf(output.ExitInvalidUsage, "zeroturn policy migrate changed nothing",
				"flag "+a+" is not one it accepts", "run zeroturn policy migrate --help")
		}
	}
	repo, err := openRepo(ctx)
	if err != nil {
		return err
	}
	raw, err := ioutil.ReadFile(config.Path(repo.Root))
	if err != nil {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn policy migrate changed nothing",
			"there is no "+config.FileName+" in this repository",
			"run zeroturn init to write one")
	}

	c, report, err := config.Migrate(raw)
	if err != nil {
		var newer config.ErrNewer
		if errors.As(err, &newer) {
			return output.Errorf(output.ExitContractVersion, "zeroturn policy migrate changed nothing",
				newer.Error(), "upgrade ZeroTurn, which can read the newer file")
		}
		return output.Errorf(output.ExitInvalidUsage, "zeroturn policy migrate changed nothing",
			err.Error(), "correct "+config.FileName+", then run the command again")
	}
	if err := c.Validate(); err != nil {
		if ve, ok := err.(config.ValidationError); ok {
			return output.Errorf(output.ExitInvalidUsage, "zeroturn policy migrate changed nothing",
				ve.Field+" "+ve.Detail, ve.Fix)
		}
		return output.Errorf(output.ExitInvalidUsage, "zeroturn policy migrate changed nothing",
			err.Error(), "correct "+config.FileName+", then run the command again")
	}

	if !report.Changed() {
		fmt.Printf("%s is already in the format this build writes.\n", config.FileName)
		return nil
	}

	// The plan and the run print the same list, so the only thing that
	// differs is whether it has happened yet.
	write, dropped := "writes", "drops"
	if plan {
		write, dropped = "would write", "would drop"
	}
	fmt.Println("ZEROTURN POLICY MIGRATE")
	fmt.Printf("file            %s\n", config.Path(repo.Root))
	fmt.Printf("version         %d to %d\n", report.From, config.Version)
	for _, name := range report.Filled {
		k, ok := config.Lookup(name)
		if !ok {
			continue
		}
		fmt.Printf("%-15s %s %s, the value a new file would hold\n", write, name, k.Display(c))
	}
	for _, name := range report.Unknown {
		fmt.Printf("%-15s %s, which is not a ZeroTurn setting\n", dropped, name)
	}
	if plan {
		fmt.Println()
		fmt.Println("Nothing has changed. Run zeroturn policy migrate to write it.")
		return nil
	}

	if err := config.Save(repo.Root, c); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn could not write the configuration",
			err.Error(), "check that "+config.FileName+" is writable")
	}
	fmt.Printf("\nWrote %s\n", config.Path(repo.Root))
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
