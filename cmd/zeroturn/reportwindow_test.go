package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// The window names live in four places: the registry, the help text, the
// README, and the enum in the published schema that an adapter reads.
// Three of those are descriptions written beside the thing they describe.
// These read the registry and compare.

func TestEveryWindowIsInTheHelpText(t *testing.T) {
	usage := reportUsage()
	for _, w := range reportWindows {
		if !strings.Contains(usage, "  "+w.Name+" ") {
			t.Errorf("report answers %q, and the help text does not list it", w.Name)
		}
	}
	if !strings.Contains(usage, "--since") {
		t.Error("the help text does not mention --since")
	}
}

func TestEveryWindowIsInTheSchema(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "schemas", "report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties struct {
			Window struct {
				Enum []string `json:"enum"`
			} `json:"window"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	published := map[string]bool{}
	for _, name := range doc.Properties.Window.Enum {
		published[name] = true
	}
	if len(published) == 0 {
		t.Fatal("the schema publishes no window names, so this test checks nothing")
	}

	// A span given with --since has no window name, so the report calls
	// it since, and that is the one value not in the registry.
	real := map[string]bool{"since": true}
	for _, w := range reportWindows {
		real[w.Name] = true
		if !published[w.Name] {
			t.Errorf("report answers %q, and the schema does not publish it", w.Name)
		}
	}
	for name := range published {
		if !real[name] {
			t.Errorf("the schema publishes %q, which is not a window report answers", name)
		}
	}
}

func TestTheReadmeNamesEveryWindow(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	var row string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "| `zeroturn report ") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatal("the README has no zeroturn report row, so this test checks nothing")
	}
	for _, w := range reportWindows {
		if !strings.Contains(row, w.Name) {
			t.Errorf("report answers %q, and the README row does not name it: %s", w.Name, row)
		}
	}
}

// A window name and --since are two ways of saying the same thing, so
// both together is a question with two answers.
func TestAWindowAndASpanTogetherAreRefused(t *testing.T) {
	_, _, err := resolveWindow("week", "14d")
	if err == nil {
		t.Fatal("a window and --since together were accepted")
	}
	if !strings.Contains(err.Error(), "both set the span") {
		t.Errorf("the error does not say what the conflict is: %v", err)
	}
}

func TestSinceReadsTheFormsAPersonTypes(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"14d", 14 * day},
		{"1d", day},
		{"72h", 72 * time.Hour},
		{"90m", 90 * time.Minute},
	}
	for _, c := range cases {
		got, err := parseSince(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s read as %s, want %s", c.in, got, c.want)
		}
	}

	// A date is measured from now, so the span only has to land on the
	// right day rather than on an exact number of hours.
	got, err := parseSince(time.Now().AddDate(0, 0, -3).Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if got < 2*day || got > 4*day {
		t.Errorf("a date three days ago read as %s", got)
	}
}

func TestSinceRefusesWhatItCannotRead(t *testing.T) {
	for _, in := range []string{"", "soon", "0d", "-5d", "2026-13-45", "last tuesday"} {
		if _, err := parseSince(in); err == nil {
			t.Errorf("%q was accepted as a span", in)
		}
	}
	// A date in the future would ask for a window that has not happened.
	future := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	if _, err := parseSince(future); err == nil {
		t.Errorf("%s was accepted, and it is not in the past", future)
	}
}

// Every refusal has to say what to do instead, which is the rule this
// project applies to every error it prints.
func TestARefusedSpanSaysWhatIsAccepted(t *testing.T) {
	_, err := parseSince("soon")
	oe, ok := err.(*output.Error)
	if !ok {
		t.Fatalf("got %v, want a ZeroTurn error", err)
	}
	for _, want := range []string{"2026-09-01", "14d", "72h"} {
		if !strings.Contains(oe.Next, want) {
			t.Errorf("the next action does not show the %s form: %q", want, oe.Next)
		}
	}
}

// A window longer than the records that survive has to say so. A heading
// of thirty days over seven days of records is an overstatement the
// reader cannot see.
func TestAWindowLongerThanTheRecordsKeptSaysSo(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	c := config.Default()
	c.Report.RetentionDays = 7
	if err := config.Save(work, c); err != nil {
		t.Fatal(err)
	}

	month, err := capture(t, work, true, func() error {
		return cmdReport(context.Background(), []string{"month"})
	})
	if err != nil {
		t.Fatalf("report month: %v\n%s", err, month)
	}
	if !strings.Contains(month, "keeps records for 7 days") {
		t.Errorf("a thirty day window over seven days of records said nothing:\n%s", month)
	}

	// A window inside the retention is not an overstatement, and saying
	// it every time would be noise.
	week, err := capture(t, work, true, func() error {
		return cmdReport(context.Background(), []string{"week"})
	})
	if err != nil {
		t.Fatalf("report week: %v\n%s", err, week)
	}
	if strings.Contains(week, "keeps records for") {
		t.Errorf("a window inside the retention was called an overstatement:\n%s", week)
	}

	// Raising the retention makes the same window honest.
	c.Report.RetentionDays = 60
	if err := config.Save(work, c); err != nil {
		t.Fatal(err)
	}
	longer, err := capture(t, work, true, func() error {
		return cmdReport(context.Background(), []string{"month"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(longer, "keeps records for") {
		t.Errorf("the window fits the retention now and it still complained:\n%s", longer)
	}
}
