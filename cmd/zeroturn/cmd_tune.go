package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/tune"
)

const tuneUsage = `zeroturn policy tune [--json] [--days <n>]

Suggest thresholds from what the gate asked and what happened next. A
threshold that always asks and is always approved is asking too early.
It never changes a setting itself.
`

func cmdTune(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("policy tune", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print machine readable output")
	days := fs.Int("days", 7, "how many days of observations to read")
	if err := parseFlags(fs, args, tuneUsage, "policy tune"); err != nil {
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
	sessions, serr := st.Since(time.Duration(*days) * 24 * time.Hour)
	if serr != nil {
		return output.Errorf(output.ExitInternal, "zeroturn could not read its records", serr.Error(),
			"check that your user data directory is readable")
	}

	report := tune.Analyse(c, sessions, state.RepoHash(repo.Root))
	if *asJSON {
		return output.JSON(os.Stdout, report)
	}
	printTuning(report, *days)
	return nil
}

func printTuning(r tune.Report, days int) {
	c := output.NewColor(os.Stdout, "")
	fmt.Println("ZEROTURN POLICY TUNE")
	fmt.Printf("Observations from the last %d days in this repository.\n\n", days)
	if r.Asks == 0 && r.Denied == 0 {
		fmt.Println("The gate has not asked anything yet, so there is nothing to learn from.")
		fmt.Println("Thresholds stay as they are. Run this again after a few sessions.")
		return
	}
	fmt.Print(output.Table([][2]string{
		{"sessions observed", fmt.Sprintf("%d", r.Sessions)},
		{"asked", fmt.Sprintf("%d", r.Asks)},
		{"approved after asking", fmt.Sprintf("%d", r.Approved)},
		{"denied", fmt.Sprintf("%d", r.Denied)},
	}))
	fmt.Println()

	for _, t := range r.Thresholds {
		fmt.Printf("%s  %s\n", c.Bold(t.Trigger), c.Dim(fmt.Sprintf("(%s, now %d)", t.Setting, t.Current)))
		fmt.Printf("  %s\n", t.Reason)
		if t.Suggested != nil {
			fmt.Printf("  %s\n", c.Yellow(fmt.Sprintf("zeroturn policy set %s %d", t.Setting, *t.Suggested)))
		}
		fmt.Println()
	}
	fmt.Println(tune.Note)
	fmt.Println("Nothing was changed. Run the command above if you agree with it.")
}
