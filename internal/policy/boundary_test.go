package policy

import (
	"strings"
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
)

// These cover the edges that scripts/mutate found nothing failing for.
// Changing < to <=, > to >=, or == to != at each of them left the whole
// suite green, which means the rule at that line was written down and
// never checked.

// projecting builds a session whose five hour window has a baseline, so
// Project has something to measure.
func projecting(t *testing.T, now time.Time, span time.Duration, left time.Duration, from, to float64) state.Session {
	t.Helper()
	base := now.Add(-span)
	reset := now.Add(left).Unix()
	return state.Session{
		FiveHourPct:       &to,
		FiveHourBasePct:   &from,
		FiveHourBaseAt:    &base,
		FiveHourResetsAt:  &reset,
		FiveHourBaseReset: &reset,
	}
}

// A rate needs a span to be measured over. Exactly the minimum is long
// enough; anything shorter is not.
func TestTheRateNeedsAFullMinimumSpan(t *testing.T) {
	now := time.Now().UTC()

	if _, ok := Project(projecting(t, now, minRateSpan, time.Hour, 10, 40), now); !ok {
		t.Error("a span of exactly the minimum was refused")
	}
	if _, ok := Project(projecting(t, now, minRateSpan-time.Second, time.Hour, 10, 40), now); ok {
		t.Error("a span one second under the minimum was measured")
	}
}

// A window that has already reset, or one that reports more time left
// than a five hour window can have, is not one to project from.
func TestTheWindowHasToBeOneThatIsStillOpen(t *testing.T) {
	now := time.Now().UTC()

	if _, ok := Project(projecting(t, now, time.Hour, time.Second, 10, 40), now); !ok {
		t.Error("a window with a second left was refused")
	}
	if _, ok := Project(projecting(t, now, time.Hour, 0, 10, 40), now); ok {
		t.Error("a window that resets exactly now was projected from")
	}
	if _, ok := Project(projecting(t, now, time.Hour, 5*time.Hour, 10, 40), now); !ok {
		t.Error("a window with exactly five hours left was refused")
	}
	if _, ok := Project(projecting(t, now, time.Hour, 5*time.Hour+time.Second, 10, 40), now); ok {
		t.Error("a window reporting more than five hours left was projected from")
	}
}

// The gate asks when the window is projected to run out, and landing
// exactly on the limit is running out.
func TestAProjectionLandingExactlyOnTheLimitAsks(t *testing.T) {
	// The reset time is stored as whole seconds, so a now with a
	// fraction of a second in it makes the arithmetic land just under
	// 100 and the boundary cannot be hit at all.
	now := time.Now().UTC().Truncate(time.Second)
	c := config.Default()
	c.Guard.Mode = config.ModeConfirm
	g := NewGuard(c, false)

	// One hour of measurement, one hour left, and a rate that lands on
	// exactly 100 when the window resets.
	at100 := projecting(t, now, time.Hour, time.Hour, 0, 50)
	p, ok := Project(at100, now)
	if !ok {
		t.Fatal("the projection could not be measured")
	}
	if p.Percent != 100 {
		t.Fatalf("this session projects to %.1f%%, and the test needs exactly 100", p.Percent)
	}
	r := EvaluateAt(g, at100, now)
	if !hasTrigger(r, "projection") {
		t.Errorf("a projection landing exactly on the limit did not fire: %+v", r.Triggers)
	}

	// Just under it does not fire, or the check above would pass for a
	// rule that fires at any projection at all.
	under := projecting(t, now, time.Hour, time.Hour, 0, 49)
	if r := EvaluateAt(g, under, now); hasTrigger(r, "projection") {
		t.Errorf("a projection landing under the limit fired: %+v", r.Triggers)
	}
}

func hasTrigger(r Result, name string) bool {
	for _, tr := range r.Triggers {
		if tr.Name == name {
			return true
		}
	}
	return false
}

// Ask and deny say different things, and swapping them told somebody
// their file had been read when it had not, or the reverse.
func TestTheCredentialMessageMatchesTheDecision(t *testing.T) {
	ask, askReason := CredentialDecision(config.CredentialAsk, ".env", 3, []string{"aws access key id"})
	if ask != DecisionAsk {
		t.Fatalf("ask mode decided %q", ask)
	}
	if !strings.HasPrefix(askReason, "Reading this file would put a credential into the conversation.") {
		t.Errorf("the ask message does not say the reading has not happened yet: %q", askReason)
	}

	deny, denyReason := CredentialDecision(config.CredentialDeny, ".env", 3, []string{"aws access key id"})
	if deny != DecisionDeny {
		t.Fatalf("deny mode decided %q", deny)
	}
	if !strings.HasPrefix(denyReason, "ZeroTurn denied reading this file, because it holds a credential.") {
		t.Errorf("the deny message does not say the reading was stopped: %q", denyReason)
	}
	if askReason == denyReason {
		t.Error("ask and deny produce the same message")
	}
}

// One category is named on its own. Counting from the wrong place gave
// "and 0 more", which reads as a fault in the tool.
func TestCountingTheOtherCategories(t *testing.T) {
	cases := []struct {
		categories []string
		want       string
		notWant    string
	}{
		{[]string{"aws access key id"}, "looks like aws access key id.", "more"},
		{[]string{"aws access key id", "private key"}, "aws access key id and 1 more", ""},
		{[]string{"a", "b", "c"}, "a and 2 more", ""},
	}
	for _, c := range cases {
		_, reason := CredentialDecision(config.CredentialAsk, ".env", 1, c.categories)
		if !strings.Contains(reason, c.want) {
			t.Errorf("%v: reason %q does not contain %q", c.categories, reason, c.want)
		}
		if c.notWant != "" && strings.Contains(reason, c.notWant) {
			t.Errorf("%v: reason %q should not mention %q", c.categories, reason, c.notWant)
		}

		block, prompt := PromptCredentialDecision(config.PromptsBlock, c.categories)
		if !block {
			t.Errorf("%v: the prompt guard did not block", c.categories)
		}
		if !strings.Contains(prompt, c.want) && !strings.Contains(prompt, strings.TrimSuffix(c.want, ".")) {
			t.Errorf("%v: prompt reason %q does not contain %q", c.categories, prompt, c.want)
		}
	}
}

// The prompt guard is off unless it was switched on, and reading messages
// is a promise this project otherwise does not make.
func TestThePromptGuardOnlyActsWhenItIsOn(t *testing.T) {
	for _, setting := range []string{config.PromptsOff, "", "anything else"} {
		if block, reason := PromptCredentialDecision(setting, []string{"aws access key id"}); block || reason != "" {
			t.Errorf("setting %q blocked a message: %v %q", setting, block, reason)
		}
	}
	if block, _ := PromptCredentialDecision(config.PromptsBlock, []string{"aws access key id"}); !block {
		t.Error("the guard did not block with the setting on")
	}
	// Nothing found is nothing to block, whatever the setting.
	if block, _ := PromptCredentialDecision(config.PromptsBlock, nil); block {
		t.Error("the guard blocked a message with no findings")
	}
}

// The reason joins a sentence to a clause, so the first letter is lowered.
// Getting the range wrong left a capital in the middle of a sentence, or
// altered a character that was not a letter.
func TestLoweringTheFirstLetter(t *testing.T) {
	for in, want := range map[string]string{
		"":         "",
		"Alpha":    "alpha",
		"Zulu":     "zulu",
		"already":  "already",
		"zebra":    "zebra",
		"[bracket": "[bracket",
		"@at":      "@at",
		"9nine":    "9nine",
	} {
		if got := lowerFirst(in); got != want {
			t.Errorf("lowerFirst(%q) is %q, want %q", in, got, want)
		}
	}
}
