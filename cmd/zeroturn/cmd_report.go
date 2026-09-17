package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/evidence"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/snapshot"
	"github.com/cris-wendler/zeroturn/internal/state"
)

// reportWindow is one span a report can cover. This list is the only
// description: the help text, the command that reads a window name, and
// the message that refuses one all read it, so a window cannot be offered
// in one place and missing from another.
type reportWindow struct {
	Name string
	Help string
	// Span is how far back the window reaches. Zero means the session
	// most recently observed, however long ago that was.
	Span time.Duration
}

var reportWindows = []reportWindow{
	{"current", "The session most recently observed", 0},
	{"day", "Sessions observed in the last 24 hours", 24 * time.Hour},
	{"week", "Sessions observed in the last 7 days", 7 * 24 * time.Hour},
	{"month", "Sessions observed in the last 30 days", 30 * 24 * time.Hour},
}

func lookupWindow(name string) (reportWindow, bool) {
	for _, w := range reportWindows {
		if w.Name == name {
			return w, true
		}
	}
	return reportWindow{}, false
}

func windowNames() []string {
	out := make([]string, 0, len(reportWindows)+1)
	for _, w := range reportWindows {
		out = append(out, w.Name)
	}
	return append(out, "purge")
}

func reportUsage() string {
	var b strings.Builder
	b.WriteString("zeroturn report <window>\n\n")
	for _, w := range reportWindows {
		fmt.Fprintf(&b, "  %-9s %s\n", w.Name, w.Help)
	}
	b.WriteString("  purge     Delete ZeroTurn session records, and with --all the\n")
	b.WriteString("            validation evidence as well\n")
	b.WriteString(`
  --since   Reach back a given span instead of naming a window, as a
            date (2026-09-01), a number of days (14d), or a duration (72h)
  --all-repositories
            Count sessions from every repository on this machine. Without
            it a report covers the repository you are in, and outside a
            repository it covers the machine and says so.

A report for the repository you are in also says whether the validation
evidence zeroturn verify recorded still covers the code that is there.

A report can only reach as far back as records are kept. Run
zeroturn policy show to see that setting.
`)
	return b.String()
}

const provenance = "Based only on events observed locally by ZeroTurn on this machine."

// What a report counted. Records are stored for the machine, and the
// repository is the scope every other command answers for.
const (
	scopeRepository = "repository"
	scopeMachine    = "machine"
)

// reportScope decides which records to count. Outside a repository there
// is nothing to narrow to, so the report covers the machine and says so
// rather than reporting nothing.
func reportScope(everywhere bool) (scope, root string) {
	if everywhere {
		return scopeMachine, ""
	}
	wd, err := os.Getwd()
	if err != nil {
		return scopeMachine, ""
	}
	r, ok := git.FindRoot(wd)
	if !ok {
		return scopeMachine, ""
	}
	return scopeRepository, r
}

// inScope keeps the records belonging to one repository. An empty root
// means the whole machine was asked for.
func inScope(sessions []state.Session, root string) []state.Session {
	if root == "" {
		return sessions
	}
	hash := state.RepoHash(root)
	out := make([]state.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.RepoHash == hash {
			out = append(out, s)
		}
	}
	return out
}

type reportJSON struct {
	Window string `json:"window"`
	// Scope says which records were counted. Without it a reader cannot
	// tell a quiet repository from a busy machine, and every other part
	// of this output speaks for one repository.
	Scope string `json:"scope"`
	// WindowStart says what the window actually covered. A name such as
	// week does not say when the week began, and a span given with
	// --since has no name at all.
	WindowStart        *time.Time `json:"windowStart,omitempty"`
	Sessions           int        `json:"sessionsObserved"`
	PeakContext        *float64   `json:"peakContextPercent,omitempty"`
	SubagentStarts     int        `json:"subagentsStarted"`
	HighestActive      int        `json:"highestActiveSubagents"`
	CredentialWarnings int        `json:"credentialWarnings"`
	ConfirmRequests    int        `json:"confirmationRequests"`
	DeniedStarts       int        `json:"deniedSubagentStarts"`
	DirectValidations  int        `json:"directValidations"`
	DirectGitOps       int        `json:"directGitOperations"`
	// Validation is the state of the recorded validation evidence for this
	// repository. It is absent from a report covering the machine, because
	// evidence belongs to one repository and there would be no way to say
	// which. It is not affected by the window: evidence is the latest
	// there is, not a count of what happened inside a span.
	Validation  *evidence.Assessment `json:"validation,omitempty"`
	GeneratedAt time.Time            `json:"generatedAt"`
	Provenance  string               `json:"provenance"`
}

// validationFor reads the recorded evidence for a repository and compares
// it with the repository as it is now.
//
// Anything that goes wrong here leaves the section out rather than
// failing the report. A report is about what was observed, and being
// unable to read one part of it is not a reason to withhold the rest.
func validationFor(ctx context.Context, st *state.Store, root string) *evidence.Assessment {
	if root == "" {
		return nil
	}
	c, err := config.Load(root)
	if err != nil {
		return nil
	}
	rec, found, err := evidence.Load(st, state.RepoHash(root))
	if err != nil {
		return nil
	}
	if !found {
		a := evidence.Assess(evidence.Record{}, false, snapshot.Snapshot{})
		return &a
	}
	repo, err := git.Open(ctx, root)
	if err != nil {
		return nil
	}
	now, err := snapshot.Compute(ctx, repo, c.Hash())
	if err != nil {
		return nil
	}
	a := evidence.Assess(rec, true, now)
	return &a
}

// resolveWindow turns what was typed into a span and the name the report
// will carry. A window name and --since are two ways of saying the same
// thing, so giving both is a question with two answers.
func resolveWindow(window, since string) (time.Duration, string, error) {
	if window != "" && since != "" {
		return 0, "", output.Errorf(output.ExitInvalidUsage, "zeroturn report printed nothing",
			"a window named "+window+" and --since both set the span",
			"give one of them, for example zeroturn report --since 14d")
	}
	if since != "" {
		span, err := parseSince(since)
		if err != nil {
			return 0, "", err
		}
		return span, "since", nil
	}
	w, ok := lookupWindow(window)
	if !ok {
		return 0, "", output.Errorf(output.ExitInvalidUsage, "zeroturn report printed nothing",
			"there is no report window named "+window,
			"use "+strings.Join(windowNames(), ", ")+", or --since")
	}
	return w.Span, w.Name, nil
}

// parseSince reads the three forms a person is likely to type. A date is
// the one that reads best in a report, a day count is the one people type,
// and a duration is what Go already understands.
func parseSince(value string) (time.Duration, error) {
	bad := func(detail string) error {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn report printed nothing", detail,
			"give a date such as 2026-09-01, a number of days such as 14d, or a duration such as 72h")
	}
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, bad("--since was given no value")
	}

	if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		span := time.Since(t)
		if span <= 0 {
			return 0, bad(v + " is not in the past")
		}
		return span, nil
	}
	if strings.HasSuffix(v, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		if err != nil || days < 1 {
			return 0, bad(v + " is not a whole number of days")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	span, err := time.ParseDuration(v)
	if err != nil || span <= 0 {
		return 0, bad(v + " is not a span ZeroTurn can read")
	}
	return span, nil
}

func cmdReport(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, reportUsage())
		return output.Errorf(output.ExitInvalidUsage, "zeroturn report printed nothing",
			"no window was given", "run zeroturn report current")
	}
	// The window is read before the flags, so a request for help never
	// reaches the flag parser and has to be answered here.
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Print(reportUsage())
		return errHelp
	}
	// A span given with --since stands in for a window name, so the first
	// argument is only a window when it is not a flag.
	window, rest := "", args
	if !strings.HasPrefix(args[0], "-") {
		window, rest = args[0], args[1:]
	}

	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print machine readable output")
	all := fs.Bool("all", false, "with purge, delete every ZeroTurn session record")
	retention := fs.Int("retention-days", config.DefaultRetentionDays, "with purge, keep records newer than this many days")
	since := fs.String("since", "", "reach back a date, a number of days, or a duration")
	everywhere := fs.Bool("all-repositories", false, "count sessions from every repository on this machine")
	if err := parseFlags(fs, rest, reportUsage(), "report"); err != nil {
		return err
	}

	st, err := openStore()
	if err != nil {
		return err
	}

	if window == "purge" {
		return reportPurge(st, *all, *retention)
	}

	span, name, serr := resolveWindow(window, *since)
	if serr != nil {
		fmt.Fprint(os.Stderr, reportUsage())
		return serr
	}

	// Records are kept for the whole machine, so a report has to say
	// which of them it counted. Every other command that reads records
	// answers for the repository it was run in; this one counted all of
	// them and said "this repository" underneath.
	scope, root := reportScope(*everywhere)

	var sessions []state.Session
	if span == 0 {
		list, lerr := st.List()
		if lerr != nil {
			return output.Errorf(output.ExitInternal, "zeroturn could not read its records", lerr.Error(),
				"check that your user data directory is readable")
		}
		list = inScope(list, root)
		if len(list) > 0 {
			sessions = list[:1]
		}
	} else {
		sessions, err = st.Since(span)
		sessions = inScope(sessions, root)
	}
	if err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn could not read its records", err.Error(),
			"check that your user data directory is readable")
	}

	r := summarise(name, sessions)
	r.Scope = scope
	r.Validation = validationFor(ctx, st, root)
	if span > 0 {
		start := time.Now().UTC().Add(-span)
		r.WindowStart = &start
	}
	if *asJSON {
		return output.JSON(os.Stdout, r)
	}

	if r.Scope == scopeMachine {
		fmt.Println("ZEROTURN REPORT (every repository on this machine)")
	} else {
		fmt.Println("ZEROTURN REPORT (this repository)")
	}
	rows := [][2]string{
		{"Sessions observed", fmt.Sprintf("%d", r.Sessions)},
	}
	if r.PeakContext != nil {
		rows = append(rows, [2]string{"Peak context", fmt.Sprintf("%.0f%%", *r.PeakContext)})
	} else {
		rows = append(rows, [2]string{"Peak context", "unavailable"})
	}
	rows = append(rows,
		[2]string{"Subagents started", fmt.Sprintf("%d", r.SubagentStarts)},
		[2]string{"Highest active subagents", fmt.Sprintf("%d", r.HighestActive)},
		[2]string{"Confirmation requests", fmt.Sprintf("%d", r.ConfirmRequests)},
		[2]string{"Credential warnings", fmt.Sprintf("%d", r.CredentialWarnings)},
		[2]string{"Denied subagent starts", fmt.Sprintf("%d", r.DeniedStarts)},
		[2]string{"Direct validations", fmt.Sprintf("%d", r.DirectValidations)},
		[2]string{"Direct Git operations", fmt.Sprintf("%d", r.DirectGitOps)},
	)
	fmt.Print(output.Table(rows))
	printValidation(r.Validation)
	// The retention setting belongs to a repository, so the note only
	// speaks when the report did too.
	// The retention setting belongs to a repository, so the note speaks
	// only when the report did too.
	if r.Scope == scopeRepository {
		if note := retentionNote(span); note != "" {
			fmt.Println(note)
		}
	}
	fmt.Println(provenance)
	return nil
}

// printValidation writes the validation state under the table. The reason
// is a whole sentence rather than a word, because "stale" on its own
// tells a reader something is wrong and not what to do about it.
func printValidation(a *evidence.Assessment) {
	if a == nil {
		return
	}
	fmt.Println()
	c := output.NewColor(os.Stdout, "")
	label := strings.ToUpper(a.State)
	switch a.State {
	case evidence.StatePassed:
		label = c.Green(label)
	case evidence.StateFailed:
		label = c.Red(label)
	case evidence.StateStale, evidence.StateCancelled, evidence.StateRunning:
		label = c.Yellow(label)
	default:
		label = c.Dim(label)
	}
	fmt.Printf("Validation  %s\n", label)
	fmt.Printf("  %s\n", a.Reason)
	if a.Evidence != nil {
		fmt.Printf("  Evidence %s recorded %s for repository state %s\n",
			a.Evidence.EvidenceID,
			a.Evidence.StartedAt.Local().Format("2006-01-02 15:04"),
			a.Evidence.Snapshot.Short())
	}
}

// retentionNote says when a window asks for more history than is kept.
// A heading of thirty days over seven days of records reads as a quiet
// month, which is the opposite of what happened.
//
// Retention is a repository setting and records are kept for the whole
// machine, so this speaks for the repository the command was run in and
// says so.
func retentionNote(span time.Duration) string {
	if span == 0 {
		return ""
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, ok := git.FindRoot(wd)
	if !ok {
		return ""
	}
	c, err := config.Load(root)
	if err != nil {
		return ""
	}
	if span <= time.Duration(c.Report.RetentionDays)*24*time.Hour {
		return ""
	}
	return fmt.Sprintf(
		"This repository keeps records for %s, so the window reaches further back than anything that survives. Change it with zeroturn policy set report.retentionDays.",
		output.Counted(c.Report.RetentionDays, "1 day", "%d days"))
}

func summarise(window string, sessions []state.Session) reportJSON {
	r := reportJSON{Window: window, Sessions: len(sessions), GeneratedAt: time.Now().UTC(), Provenance: provenance}
	for _, s := range sessions {
		if s.PeakContextPct != nil {
			if r.PeakContext == nil || *s.PeakContextPct > *r.PeakContext {
				v := *s.PeakContextPct
				r.PeakContext = &v
			}
		}
		r.SubagentStarts += s.SubagentStarts
		if s.PeakActive > r.HighestActive {
			r.HighestActive = s.PeakActive
		}
		r.CredentialWarnings += s.CredentialWarnings
		r.ConfirmRequests += s.ConfirmRequests
		r.DeniedStarts += s.DeniedStarts
		r.DirectValidations += s.DirectValidations
		r.DirectGitOps += s.DirectGitOps
	}
	return r
}

// reportPurge removes only ZeroTurn session records. Coding harness
// transcripts and settings are never touched.
func reportPurge(st *state.Store, all bool, retention int) error {
	if all {
		ok, err := confirm("Delete every ZeroTurn session record on this machine?")
		if err != nil {
			return err
		}
		if !ok {
			return output.Errorf(output.ExitDeclined, "zeroturn report purge deleted nothing",
				"the deletion was declined", "run the command again when you are ready")
		}
		n, perr := st.PurgeAll()
		if perr != nil {
			return output.Errorf(output.ExitInternal, "zeroturn could not purge its records", perr.Error(),
				"check that your user data directory is writable")
		}
		// Validation evidence is a ZeroTurn record too. Leaving it behind
		// would make "every ZeroTurn record" untrue in the one command a
		// person runs when they mean it.
		e, eerr := evidence.PurgeAll(st)
		if eerr != nil {
			return output.Errorf(output.ExitInternal, "zeroturn could not purge its records", eerr.Error(),
				"check that your user data directory is writable")
		}
		fmt.Printf("Deleted %s and %s. Harness transcripts and settings were not touched.\n",
			output.Counted(n, "1 ZeroTurn session record", "%d ZeroTurn session records"),
			output.Counted(e, "1 validation evidence record", "%d validation evidence records"))
		return nil
	}
	if retention < 0 {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn report purge deleted nothing",
			"the retention window cannot be negative", "pass a whole number of days")
	}
	n, err := st.Purge(retention)
	if err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn could not purge its records", err.Error(),
			"check that your user data directory is writable")
	}
	fmt.Printf("Deleted %s older than %s.\n",
		output.Counted(n, "1 ZeroTurn session record", "%d ZeroTurn session records"),
		output.Counted(retention, "1 day", "%d days"))
	return nil
}
