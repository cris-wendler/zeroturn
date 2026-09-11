// Package status renders the one line session summary.
package status

import (
	"fmt"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/policy"
	"github.com/cris-wendler/zeroturn/internal/state"
)

type Line struct {
	Session state.Session
	Result  policy.Result
	Branch  string
	Color   output.Color
	// ShowRepo adds branch information, which costs a Git call and is
	// therefore off for the status line, which repaints often.
	ShowRepo bool
}

// Render produces the status text. A measurement the harness did not
// supply is omitted rather than shown as zero.
func Render(l Line) string {
	c := l.Color
	parts := []string{c.Bold("ZT")}

	if l.Session.ContextPct != nil {
		parts = append(parts, l.paint("ctx", fmt.Sprintf("%.0f%%", *l.Session.ContextPct), levelOf(l.Result, "context")))
	}
	if l.Session.FiveHourPct != nil {
		parts = append(parts, l.paint("5h", fmt.Sprintf("%.0f%%", *l.Session.FiveHourPct), levelOf(l.Result, "fiveHour")))
	}
	if l.Session.SevenDayPct != nil {
		parts = append(parts, l.paint("7d", fmt.Sprintf("%.0f%%", *l.Session.SevenDayPct), levelOf(l.Result, "sevenDay")))
	}
	if l.Session.DurationMS != nil {
		mins := float64(*l.Session.DurationMS) / 60000.0
		parts = append(parts, l.paint("session", policy.HumanMinutes(mins), levelOf(l.Result, "duration")))
	}
	if l.Session.SubagentStarts > 0 || l.Session.ActiveSubagents > 0 {
		parts = append(parts, l.paint("agents", fmt.Sprintf("%d", l.Session.ActiveSubagents), levelOf(l.Result, "activeSubagents")))
	}
	if l.ShowRepo && l.Branch != "" {
		parts = append(parts, c.Dim("branch ")+l.Branch)
	}

	if len(parts) == 1 {
		parts = append(parts, c.Dim("session data unavailable"))
	}

	switch l.Result.Level {
	case policy.LevelWarn:
		parts = append(parts, c.Yellow("warn"))
	case policy.LevelConfirm:
		parts = append(parts, c.Yellow(l.Result.Mode))
	case policy.LevelCritical:
		parts = append(parts, c.Red("critical"))
	}

	return strings.Join(parts, "  ")
}

func (l Line) paint(label, value, level string) string {
	c := l.Color
	text := c.Dim(label+" ") + value
	switch level {
	case policy.LevelCritical:
		return c.Dim(label+" ") + c.Red(value)
	case policy.LevelConfirm:
		return c.Dim(label+" ") + c.Yellow(value)
	case policy.LevelWarn:
		return c.Dim(label+" ") + c.Yellow(value)
	}
	return text
}

func levelOf(r policy.Result, name string) string {
	for _, t := range r.Triggers {
		if t.Name == name {
			return t.Level
		}
	}
	return policy.LevelOK
}
