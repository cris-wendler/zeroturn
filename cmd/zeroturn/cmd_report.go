package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/state"
)

const reportUsage = `zeroturn report <window>

  current   The session most recently observed
  day       Sessions observed in the last 24 hours
  week      Sessions observed in the last 7 days
  purge     Delete ZeroTurn session records
`

const provenance = "Based only on events observed locally by ZeroTurn on this machine."

type reportJSON struct {
	Window             string    `json:"window"`
	Sessions           int       `json:"sessionsObserved"`
	PeakContext        *float64  `json:"peakContextPercent,omitempty"`
	SubagentStarts     int       `json:"subagentsStarted"`
	HighestActive      int       `json:"highestActiveSubagents"`
	CredentialWarnings int       `json:"credentialWarnings"`
	ConfirmRequests    int       `json:"confirmationRequests"`
	DeniedStarts       int       `json:"deniedSubagentStarts"`
	DirectValidations  int       `json:"directValidations"`
	DirectGitOps       int       `json:"directGitOperations"`
	GeneratedAt        time.Time `json:"generatedAt"`
	Provenance         string    `json:"provenance"`
}

func cmdReport(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, reportUsage)
		return output.Errorf(output.ExitInvalidUsage, "zeroturn report printed nothing",
			"no window was given", "run zeroturn report current")
	}
	window := args[0]
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print machine readable output")
	all := fs.Bool("all", false, "with purge, delete every ZeroTurn session record")
	retention := fs.Int("retention-days", state.DefaultRetentionDays, "with purge, keep records newer than this many days")
	if err := fs.Parse(args[1:]); err != nil {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn could not read the flags", err.Error(), "run zeroturn report --help")
	}

	st, err := openStore()
	if err != nil {
		return err
	}

	if window == "purge" {
		return reportPurge(st, *all, *retention)
	}

	var sessions []state.Session
	switch window {
	case "current":
		list, lerr := st.List()
		if lerr != nil {
			return output.Errorf(output.ExitInternal, "zeroturn could not read its records", lerr.Error(),
				"check that your user data directory is readable")
		}
		if len(list) > 0 {
			sessions = list[:1]
		}
	case "day":
		sessions, err = st.Since(24 * time.Hour)
	case "week":
		sessions, err = st.Since(7 * 24 * time.Hour)
	default:
		fmt.Fprint(os.Stderr, reportUsage)
		return output.Errorf(output.ExitInvalidUsage, "zeroturn report printed nothing",
			"there is no report window named "+window, "use current, day, week, or purge")
	}
	if err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn could not read its records", err.Error(),
			"check that your user data directory is readable")
	}

	r := summarise(window, sessions)
	if *asJSON {
		return output.JSON(os.Stdout, r)
	}

	fmt.Println("ZEROTURN REPORT")
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
	fmt.Println(provenance)
	return nil
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
		fmt.Printf("Deleted %d ZeroTurn session records. Harness transcripts and settings were not touched.\n", n)
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
	fmt.Printf("Deleted %d ZeroTurn session records older than %d days.\n", n, retention)
	return nil
}
