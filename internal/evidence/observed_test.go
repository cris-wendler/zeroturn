package evidence

import (
	"context"
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/snapshot"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

func plan() []config.Step {
	return []config.Step{
		{Name: "vet", Command: []string{"go", "vet", "./..."}},
		{Name: "test", Command: []string{"go", "test", "./..."}},
	}
}

// What counts as the step decides whether a record claims more than was
// seen. Only the step run argument for argument records anything: a
// narrower run did less work, and a record saying otherwise is the
// overstatement this whole feature has to avoid.
func TestWhatCountsAsTheStep(t *testing.T) {
	for _, c := range []struct {
		name    string
		command string
		want    MatchKind
		step    string
	}{
		{"the step itself", "go test ./...", Exact, "test"},
		{"the other step", "go vet ./...", Exact, "vet"},
		{"extra spacing is still the step", "go  test   ./...", Exact, "test"},
		{"one package rather than all", "go test ./internal/policy", Near, "test"},
		{"the step with a filter", "go test -run TestX ./...", Near, "test"},
		{"a different program", "make test", NoMatch, ""},
		{"nothing to do with the plan", "ls -la", NoMatch, ""},
		{"empty", "", NoMatch, ""},
	} {
		got, step := Match(c.command, plan())
		if got != c.want {
			t.Errorf("%s: %q gave %v, want %v", c.name, c.command, got, c.want)
		}
		if c.step != "" && step.Name != c.step {
			t.Errorf("%s: %q matched step %q, want %q", c.name, c.command, step.Name, c.step)
		}
	}
}

// go vet and go test share a program and must not read as near misses of
// each other, or running one would report the other as nearly matched.
func TestTheStepsAreNotNearMissesOfEachOther(t *testing.T) {
	got, step := Match("go vet ./...", plan())
	if got != Exact || step.Name != "vet" {
		t.Fatalf("go vet matched %v %q", got, step.Name)
	}
	got, step = Match("go vet ./internal/policy", plan())
	if got != Near || step.Name != "vet" {
		t.Errorf("a narrower vet matched %v %q, want a near miss of vet", got, step.Name)
	}
}

// A command a shell would read differently from a plain split is not
// matched at all. What ran is not knowable from the string, and a guess
// here would record evidence for work that did not happen.
func TestAShellCommandIsNeverMatched(t *testing.T) {
	for _, command := range []string{
		"go test ./... && echo done",
		"go test ./... | tee out.txt",
		"go test ./...; go vet ./...",
		"cd sub && go test ./...",
		"go test ./... > /dev/null",
		"FOO=1 go test ./...",
	} {
		if got, _ := Match(command, plan()); got == Exact {
			t.Errorf("%q was taken for the step", command)
		}
	}
}

// A plan with no steps matches nothing rather than everything.
func TestAnEmptyPlanMatchesNothing(t *testing.T) {
	if got, _ := Match("go test ./...", nil); got != NoMatch {
		t.Errorf("an empty plan gave %v", got)
	}
	if got, _ := Match("go test ./...", []config.Step{{Name: "broken"}}); got != NoMatch {
		t.Errorf("a step with no command gave %v", got)
	}
}

// The rule the whole feature rests on: a record assembled from
// observations says passed only when every step of the plan has been
// seen to pass at the same state. One step is not a plan.
func TestOneStepIsNotAPassingPlan(t *testing.T) {
	st, repo, work := fixture(t)
	snap := snapshotOf(t, repo)

	r, err := Observe(st, state.RepoHash(repo.Root), "test", snap, plan()[1], plan(), 1.5, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != ResultPartial {
		t.Fatalf("one step of two gave %q, want %q", r.Result, ResultPartial)
	}
	if got := Missing(r, plan()); len(got) != 1 || got[0] != "vet" {
		t.Errorf("missing steps are %v, want vet", got)
	}

	// The other step, at the same state, completes it.
	r, err = Observe(st, state.RepoHash(repo.Root), "test", snap, plan()[0], plan(), 0.2, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != ResultPassed {
		t.Fatalf("both steps gave %q, want %q", r.Result, ResultPassed)
	}
	if got := Missing(r, plan()); len(got) != 0 {
		t.Errorf("nothing should be missing, got %v", got)
	}
	_ = work
}

// Steps seen against different code do not add up. The earlier one ran
// against something that is no longer here, so it is dropped rather than
// counted towards a plan passing at the state in front of us.
func TestStepsSeenAtDifferentStatesDoNotCombine(t *testing.T) {
	st, repo, work := fixture(t)

	first := snapshotOf(t, repo)
	if _, err := Observe(st, state.RepoHash(repo.Root), "test", first, plan()[0], plan(), 0.2, time.Now()); err != nil {
		t.Fatal(err)
	}

	testutil.Write(t, work, "moved.txt", "the code changed between the two steps\n")
	second := snapshotOf(t, repo)
	if first.Digest == second.Digest {
		t.Fatal("the digest did not move, so this test proves nothing")
	}

	r, err := Observe(st, state.RepoHash(repo.Root), "test", second, plan()[1], plan(), 1.5, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Result == ResultPassed {
		t.Fatal("two steps at two different states were counted as a passing plan")
	}
	if len(r.Steps) != 1 || r.Steps[0].Name != "test" {
		t.Errorf("the record holds %+v, want only the step seen at this state", r.Steps)
	}
}

// A record written by verify is the stronger statement, because verify
// ran every step itself. An observation never edits one: it starts its
// own, so a partial cannot quietly downgrade a real run.
func TestAnObservationDoesNotEditAVerifyRecord(t *testing.T) {
	st, repo, _ := fixture(t)
	snap := snapshotOf(t, repo)

	full := record(t, st, repo, "plan", passing())
	if full.Result != ResultPassed {
		t.Fatalf("the fixture run is %q", full.Result)
	}

	r, err := Observe(st, state.RepoHash(repo.Root), "test", snap, plan()[0], plan(), 0.2, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.EvidenceID == full.EvidenceID {
		t.Error("the observation was written into the verify record")
	}
	if r.Result != ResultPartial {
		t.Errorf("the observation reads %q", r.Result)
	}
}

// Observing the same step twice at one state leaves one entry, or the
// count of what passed would climb with every repetition.
func TestTheSameStepTwiceIsStillOneStep(t *testing.T) {
	st, repo, _ := fixture(t)
	snap := snapshotOf(t, repo)
	for i := 0; i < 3; i++ {
		if _, err := Observe(st, state.RepoHash(repo.Root), "test", snap, plan()[1], plan(), 1, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	r, _, err := Load(st, state.RepoHash(repo.Root))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Steps) != 1 || r.Passed != 1 {
		t.Errorf("three observations of one step left %d steps, passed %d", len(r.Steps), r.Passed)
	}
}

// snapshotOf is the repository state as the evidence package sees it.
func snapshotOf(t *testing.T, repo git.Repo) snapshot.Snapshot {
	t.Helper()
	s, err := snapshot.Compute(context.Background(), repo, "plan")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
