package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/state"
)

// enoughSamples is the point below which a suggestion would be noise
// rather than a pattern.
const enoughSamples = 5

type tuning struct {
	Trigger   string   `json:"trigger"`
	Setting   string   `json:"setting"`
	Current   int      `json:"current"`
	Asks      int      `json:"asks"`
	Approved  int      `json:"approved"`
	Suggested *int     `json:"suggested,omitempty"`
	Reason    string   `json:"reason"`
	Values    []string `json:"-"`
}

type tuneReport struct {
	Sessions  int       `json:"sessionsObserved"`
	Asks      int       `json:"asks"`
	Approved  int       `json:"approvedAfterAsking"`
	Denied    int       `json:"denied"`
	Tunings   []tuning  `json:"thresholds"`
	Generated time.Time `json:"generatedAt"`
	Note      string    `json:"note"`
}

const tuneNote = "Approval is inferred: a subagent starting after an ask means it was approved. Nothing about the work itself is recorded."

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

	report := analyse(c, sessions, state.RepoHash(repo.Root))
	if *asJSON {
		return output.JSON(os.Stdout, report)
	}
	printTuning(report, *days)
	return nil
}

// analyse looks at what the gate asked and what the developer did next.
func analyse(c config.Config, sessions []state.Session, repoHash string) tuneReport {
	// An empty list is written as a list, not as null, because the
	// schema says a list and an adapter reads it that way.
	r := tuneReport{Generated: time.Now().UTC(), Note: tuneNote, Tunings: []tuning{}}
	// approvedAt and declinedAt hold the measurement behind each ask, per
	// threshold, so a suggestion can name the value where behavior changed.
	approvedAt := map[string][]float64{}
	declinedAt := map[string][]float64{}
	counts := map[string][2]int{}

	for _, s := range sessions {
		if s.RepoHash != repoHash {
			continue
		}
		r.Sessions++
		for _, o := range s.GateOutcomes {
			if o.Decision == "deny" {
				r.Denied++
				continue
			}
			r.Asks++
			if o.Approved {
				r.Approved++
			}
			for _, name := range o.Triggers {
				value, ok := measurement(o, name)
				if !ok {
					continue
				}
				n := counts[name]
				n[0]++
				if o.Approved {
					n[1]++
					approvedAt[name] = append(approvedAt[name], value)
				} else {
					declinedAt[name] = append(declinedAt[name], value)
				}
				counts[name] = n
			}
		}
	}

	for name, n := range counts {
		t := tuning{Trigger: name, Setting: settingFor(name), Current: currentThreshold(c, name), Asks: n[0], Approved: n[1]}
		t.Reason, t.Suggested = suggest(t, approvedAt[name], declinedAt[name])
		r.Tunings = append(r.Tunings, t)
	}
	sort.Slice(r.Tunings, func(i, j int) bool { return r.Tunings[i].Asks > r.Tunings[j].Asks })
	return r
}

func measurement(o state.GateOutcome, trigger string) (float64, bool) {
	switch trigger {
	case "context":
		if o.ContextPct != nil {
			return *o.ContextPct, true
		}
	case "fiveHour":
		if o.FiveHourPct != nil {
			return *o.FiveHourPct, true
		}
	case "sevenDay":
		if o.SevenDayPct != nil {
			return *o.SevenDayPct, true
		}
	case "duration":
		if o.DurationMinutes != nil {
			return float64(*o.DurationMinutes), true
		}
	case "activeSubagents":
		return float64(o.ActiveSubagents), true
	case "subagentStarts":
		return float64(o.SubagentStarts), true
	}
	return 0, false
}

func settingFor(trigger string) string {
	switch trigger {
	case "context":
		return "guard.context.confirm"
	case "fiveHour":
		return "guard.limits.fiveHourWarn"
	case "sevenDay":
		return "guard.limits.sevenDayWarn"
	case "duration":
		return "guard.session.durationWarnMinutes"
	case "activeSubagents":
		return "guard.session.activeSubagentsWarn"
	case "subagentStarts":
		return "guard.session.subagentStartsWarn"
	}
	return ""
}

func currentThreshold(c config.Config, trigger string) int {
	switch trigger {
	case "context":
		return c.Guard.Context.Confirm
	case "fiveHour":
		return c.Guard.Limits.FiveHourWarn
	case "sevenDay":
		return c.Guard.Limits.SevenDayWarn
	case "duration":
		return c.Guard.Session.DurationWarnMinutes
	case "activeSubagents":
		return c.Guard.Session.ActiveSubagentsWarn
	case "subagentStarts":
		return c.Guard.Session.SubagentStartsWarn
	}
	return 0
}

// suggest reads one threshold's history. A threshold that always asks
// and is always approved is asking too early. One that is usually
// declined is asking too late, or about the wrong thing.
func suggest(t tuning, approved, declined []float64) (string, *int) {
	if t.Asks < enoughSamples {
		return fmt.Sprintf("only %d asks so far, too few to suggest a change", t.Asks), nil
	}
	declinedRate := float64(t.Asks-t.Approved) / float64(t.Asks)

	if len(declined) == 0 {
		// Everything was approved. Move the threshold above the highest
		// value seen, so the question is asked when it is new.
		highest := max(approved)
		next := int(highest) + 1
		if next <= t.Current {
			next = t.Current + 1
		}
		if next > 100 && isPercentage(t.Trigger) {
			return "every ask was approved, and the threshold is already near the top of its range", nil
		}
		return fmt.Sprintf("all %d asks were approved, the highest at %.0f", t.Asks, highest), &next
	}

	lowest := min(declined)
	if declinedRate >= 0.5 {
		return fmt.Sprintf("%d of %d asks were declined, the lowest at %.0f, so this threshold is doing its job",
			t.Asks-t.Approved, t.Asks, lowest), nil
	}
	// Mostly approved, with a decline at some point. The interesting
	// value is where the answer changed.
	next := int(lowest)
	if next <= t.Current {
		return fmt.Sprintf("%d of %d asks were approved, and the one you declined was at %.0f, below the current threshold",
			t.Approved, t.Asks, lowest), nil
	}
	return fmt.Sprintf("%d of %d asks were approved, and you declined at %.0f", t.Approved, t.Asks, lowest), &next
}

func isPercentage(trigger string) bool {
	switch trigger {
	case "context", "fiveHour", "sevenDay":
		return true
	}
	return false
}

func max(values []float64) float64 {
	out := values[0]
	for _, v := range values {
		if v > out {
			out = v
		}
	}
	return out
}

func min(values []float64) float64 {
	out := values[0]
	for _, v := range values {
		if v < out {
			out = v
		}
	}
	return out
}

func printTuning(r tuneReport, days int) {
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

	for _, t := range r.Tunings {
		fmt.Printf("%s  %s\n", c.Bold(t.Trigger), c.Dim(fmt.Sprintf("(%s, now %d)", t.Setting, t.Current)))
		fmt.Printf("  %s\n", t.Reason)
		if t.Suggested != nil {
			fmt.Printf("  %s\n", c.Yellow(fmt.Sprintf("zeroturn policy set %s %d", t.Setting, *t.Suggested)))
		}
		fmt.Println()
	}
	fmt.Println(tuneNote)
	fmt.Println("Nothing was changed. Run the command above if you agree with it.")
}
