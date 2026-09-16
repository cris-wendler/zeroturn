// Package tune reads what the gate asked and what the developer did
// next, and suggests thresholds from it.
//
// A threshold that always asks and is always approved is asking too
// early. One that is usually declined is asking too late, or about the
// wrong thing. Nothing here changes a setting: it prints the command
// that would, and the person decides.
//
// Approval is inferred from a subagent starting after an ask, which is
// the only outcome a harness reports. Nothing about the work itself is
// read or recorded.
package tune

import (
	"fmt"
	"sort"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"

	"github.com/cris-wendler/zeroturn/internal/output"
)

// EnoughSamples is the point below which a suggestion would be noise
// rather than a pattern.
const EnoughSamples = 5

// Note is printed with every report, because a reader has to know that
// the approval was inferred rather than recorded.
const Note = "Approval is inferred: a subagent starting after an ask means it was approved. Nothing about the work itself is recorded."

// Threshold is one guard threshold and what the history says about it.
// The JSON names are part of what zeroturn policy tune --json prints.
type Threshold struct {
	Trigger   string `json:"trigger"`
	Setting   string `json:"setting"`
	Current   int    `json:"current"`
	Asks      int    `json:"asks"`
	Approved  int    `json:"approved"`
	Suggested *int   `json:"suggested,omitempty"`
	Reason    string `json:"reason"`
}

type Report struct {
	Sessions   int         `json:"sessionsObserved"`
	Asks       int         `json:"asks"`
	Approved   int         `json:"approvedAfterAsking"`
	Denied     int         `json:"denied"`
	Thresholds []Threshold `json:"thresholds"`
	Generated  time.Time   `json:"generatedAt"`
	Note       string      `json:"note"`
}

// Analyse reads the sessions belonging to one repository. Records from
// another repository are ignored: thresholds are per repository, and a
// history from elsewhere would suggest a change for work it never saw.
func Analyse(c config.Config, sessions []state.Session, repoHash string) Report {
	// An empty list is written as a list, not as null, because the
	// schema says a list and an adapter reads it that way.
	r := Report{Generated: time.Now().UTC(), Note: Note, Thresholds: []Threshold{}}
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
			// A denial cannot be approved, so it is counted and then left
			// out of every suggestion.
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
		t := Threshold{Trigger: name, Setting: SettingFor(name), Current: currentThreshold(c, name), Asks: n[0], Approved: n[1]}
		t.Reason, t.Suggested = suggest(t, approvedAt[name], declinedAt[name])
		r.Thresholds = append(r.Thresholds, t)
	}
	sortThresholds(r.Thresholds)
	return r
}

// sortThresholds puts the most asked first, and orders a tie by name.
// Without the second test the order of a tie came out of the sort rather
// than out of the data, so the same observations could print in a
// different order twice running.
func sortThresholds(t []Threshold) {
	sort.Slice(t, func(i, j int) bool { return lessThreshold(t[i], t[j]) })
}

// lessThreshold orders two rows of the report. It is named rather than
// written inline so that a test can ask it the question sort relies on
// and cannot ask through a sorted list: a row is never less than itself.
// A comparator that answers yes to that is invalid, and a sorted list
// does not reveal it, because an invalid comparator still lands on the
// right order for any particular input.
func lessThreshold(a, b Threshold) bool {
	if a.Asks != b.Asks {
		return a.Asks > b.Asks
	}
	return a.Trigger < b.Trigger
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

// SettingFor reports the configuration key a trigger is governed by, so
// the report can print the command that would change it.
func SettingFor(trigger string) string {
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

// suggest reads one threshold's history.
func suggest(t Threshold, approved, declined []float64) (string, *int) {
	if t.Asks < EnoughSamples {
		return output.Counted(t.Asks, "only 1 ask so far, too few to suggest a change",
			"only %d asks so far, too few to suggest a change"), nil
	}
	declinedRate := float64(t.Asks-t.Approved) / float64(t.Asks)

	if len(declined) == 0 {
		// Everything was approved. Move the threshold above the highest
		// value seen, so the question is asked when it is new.
		highest := highest(approved)
		next := int(highest) + 1
		if next <= t.Current {
			next = t.Current + 1
		}
		if next > 100 && isPercentage(t.Trigger) {
			return "every ask was approved, and the threshold is already near the top of its range", nil
		}
		return fmt.Sprintf("%s, the highest at %.0f",
			output.Counted(t.Asks, "the 1 ask was approved", "all %d asks were approved"), highest), &next
	}

	low := lowest(declined)
	if declinedRate >= 0.5 {
		return fmt.Sprintf("%s declined, the lowest at %.0f, so this threshold is doing its job",
			ofAsks(t.Asks-t.Approved, t.Asks), low), nil
	}
	// Mostly approved, with a decline at some point. The interesting
	// value is where the answer changed.
	next := int(low)
	if next <= t.Current {
		return fmt.Sprintf("%s approved, and the one you declined was at %.0f, below the current threshold",
			ofAsks(t.Approved, t.Asks), low), nil
	}
	return fmt.Sprintf("%s approved, and you declined at %.0f", ofAsks(t.Approved, t.Asks), low), &next
}

// ofAsks writes "1 of 7 asks was" or "3 of 7 asks were". The noun stays
// plural because it counts the whole set; the verb agrees with the first
// number, which is the one that can be one.
func ofAsks(n, total int) string {
	verb := "were"
	if n == 1 {
		verb = "was"
	}
	return output.CountedPair(n, total, "%d of %d %s "+verb, "ask", "asks")
}

func isPercentage(trigger string) bool {
	switch trigger {
	case "context", "fiveHour", "sevenDay":
		return true
	}
	return false
}

func highest(values []float64) float64 {
	out := values[0]
	for _, v := range values {
		if v > out {
			out = v
		}
	}
	return out
}

func lowest(values []float64) float64 {
	out := values[0]
	for _, v := range values {
		if v < out {
			out = v
		}
	}
	return out
}
