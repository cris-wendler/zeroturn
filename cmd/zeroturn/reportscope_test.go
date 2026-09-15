package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// plantSession writes a session record belonging to some other repository,
// which is the situation nothing tested: records are kept for the machine,
// and every command except report narrows them to one repository.
func plantSession(t *testing.T, id, repoHash string, starts int) {
	t.Helper()
	dir := filepath.Join(os.Getenv("ZEROTURN_STATE_DIR"), "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	b, err := json.Marshal(state.Session{
		SchemaVersion:  state.SchemaVersion,
		SessionID:      id,
		RepoHash:       repoHash,
		StartedAt:      now,
		UpdatedAt:      now,
		SubagentStarts: starts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, id+".json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func reportJSONFor(t *testing.T, work string, args ...string) reportJSON {
	t.Helper()
	out, err := capture(t, work, true, func() error {
		return cmdReport(context.Background(), append(args, "--json"))
	})
	if err != nil {
		t.Fatalf("report %v: %v\n%s", args, err, out)
	}
	var r reportJSON
	if jerr := json.Unmarshal([]byte(out), &r); jerr != nil {
		t.Fatalf("report --json is not JSON: %v\n%s", jerr, out)
	}
	return r
}

// A report headed as this repository counted every repository on the
// machine. Somebody looking at a quiet project saw another project's
// numbers, and the retention line underneath said "This repository".
func TestAReportCountsOnlyTheRepositoryItWasRunIn(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	plantSession(t, "somewhere-else", strings.Repeat("f", 64), 9)

	mine := reportJSONFor(t, work, "day")
	if mine.Scope != scopeRepository {
		t.Errorf("scope is %q, want %q", mine.Scope, scopeRepository)
	}
	if mine.Sessions != 0 || mine.SubagentStarts != 0 {
		t.Errorf("a repository with no sessions of its own counted %d sessions and %d subagent starts",
			mine.Sessions, mine.SubagentStarts)
	}

	// The machine wide numbers are still reachable, by asking for them.
	all := reportJSONFor(t, work, "day", "--all-repositories")
	if all.Scope != scopeMachine {
		t.Errorf("scope is %q, want %q", all.Scope, scopeMachine)
	}
	if all.Sessions != 1 || all.SubagentStarts != 9 {
		t.Errorf("--all-repositories counted %d sessions and %d subagent starts, want 1 and 9",
			all.Sessions, all.SubagentStarts)
	}
}

// The current window reads one record, so a record from elsewhere must not
// be the one it picks.
func TestTheCurrentWindowPicksASessionFromThisRepository(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	plantSession(t, "somewhere-else", strings.Repeat("f", 64), 9)

	r := reportJSONFor(t, work, "current")
	if r.Sessions != 0 {
		t.Errorf("current picked %d sessions from another repository", r.Sessions)
	}
}

// Outside a repository there is nothing to narrow to. Reporting nothing
// would be useless, so it reports the machine and has to say so.
func TestOutsideARepositoryTheReportSaysItCoveredTheMachine(t *testing.T) {
	testutil.Isolate(t)
	outside := t.TempDir()
	plantSession(t, "somewhere-else", strings.Repeat("f", 64), 9)

	out, err := capture(t, outside, true, func() error {
		return cmdReport(context.Background(), []string{"day"})
	})
	if err != nil {
		t.Fatalf("report outside a repository: %v\n%s", err, out)
	}
	if !strings.Contains(out, "every repository on this machine") {
		t.Errorf("the heading does not say what it counted:\n%s", out)
	}
}

// The retention setting belongs to a repository. Printed under a machine
// wide report it is the same mixing of scopes that this fixed.
func TestTheRetentionNoteOnlySpeaksForARepository(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	c := config.Default()
	c.Report.RetentionDays = 7
	if err := config.Save(work, c); err != nil {
		t.Fatal(err)
	}

	mine, err := capture(t, work, true, func() error {
		return cmdReport(context.Background(), []string{"month"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mine, "keeps records for 7 days") {
		t.Errorf("a repository report said nothing about retention:\n%s", mine)
	}

	all, err := capture(t, work, true, func() error {
		return cmdReport(context.Background(), []string{"month", "--all-repositories"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(all, "This repository keeps records") {
		t.Errorf("a machine wide report spoke for one repository:\n%s", all)
	}
}

// The scope is published, so an adapter can tell the two apart. It has to
// be in the schema an adapter validates against.
func TestTheSchemaPublishesEveryScope(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "schemas", "report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Required   []string `json:"required"`
		Properties struct {
			Scope struct {
				Enum []string `json:"enum"`
			} `json:"scope"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	published := map[string]bool{}
	for _, name := range doc.Properties.Scope.Enum {
		published[name] = true
	}
	for _, want := range []string{scopeRepository, scopeMachine} {
		if !published[want] {
			t.Errorf("the schema does not publish the scope %q", want)
		}
	}
	if len(published) != 2 {
		t.Errorf("the schema publishes %v, want exactly the two scopes", doc.Properties.Scope.Enum)
	}
	found := false
	for _, r := range doc.Required {
		if r == "scope" {
			found = true
		}
	}
	if !found {
		t.Error("scope is not required, so a reader cannot rely on it being there")
	}
}
