package trust

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
)

func setup(t *testing.T) (*state.Store, string, config.Config) {
	t.Helper()
	t.Setenv("ZEROTURN_STATE_DIR", t.TempDir())
	st, err := state.Open()
	if err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Verify.Steps = []config.Step{{Name: "test", Command: []string{"go", "test", "./..."}}}
	return st, "/repo/one", c
}

func TestUntrustedByDefault(t *testing.T) {
	st, root, c := setup(t)
	if s := Check(st, root, c); s.Trusted || s.Reason == "" {
		t.Fatalf("got %+v", s)
	}
}

func TestApproveThenTrusted(t *testing.T) {
	st, root, c := setup(t)
	if err := Approve(st, root, c); err != nil {
		t.Fatal(err)
	}
	if s := Check(st, root, c); !s.Trusted {
		t.Fatalf("got %+v", s)
	}
}

func TestCommandChangeWithdrawsTrust(t *testing.T) {
	st, root, c := setup(t)
	Approve(st, root, c)
	c.Verify.Steps[0].Command = append(c.Verify.Steps[0].Command, "-run", "X")
	if s := Check(st, root, c); s.Trusted {
		t.Fatal("changed command is still trusted")
	}
	c2 := c
	c2.Verify.Steps = append(c2.Verify.Steps, config.Step{Name: "extra", Command: []string{"curl"}})
	if s := Check(st, root, c2); s.Trusted {
		t.Fatal("added step is still trusted")
	}
}

func TestGuardChangeKeepsTrust(t *testing.T) {
	st, root, c := setup(t)
	Approve(st, root, c)
	c.Guard.Mode = config.ModeConfirm
	if s := Check(st, root, c); !s.Trusted {
		t.Fatalf("guard change withdrew trust: %+v", s)
	}
}

func TestApprovalIsBoundToRepository(t *testing.T) {
	st, root, c := setup(t)
	Approve(st, root, c)
	if s := Check(st, "/repo/two", c); s.Trusted {
		t.Fatal("approval carried over to another repository")
	}
}

func TestRevoke(t *testing.T) {
	st, root, c := setup(t)
	Approve(st, root, c)
	if err := Revoke(st, root); err != nil {
		t.Fatal(err)
	}
	if s := Check(st, root, c); s.Trusted {
		t.Fatal("revoked approval still trusted")
	}
	if err := Revoke(st, root); err != nil {
		t.Fatal("second revoke failed")
	}
}

func TestUnreadableRecordIsUntrusted(t *testing.T) {
	st, root, c := setup(t)
	ioutil.WriteFile(filepath.Join(st.TrustDir(), state.RepoHash(root)+".json"), []byte("{bad"), 0600)
	if s := Check(st, root, c); s.Trusted {
		t.Fatal("corrupt record trusted")
	}
}

// Discarding the read error meant a record that was never written passed:
// no file, no bytes, no path in them.
func TestRecordStoresNoPath(t *testing.T) {
	st, root, c := setup(t)
	Approve(st, root, c)

	b, err := ioutil.ReadFile(filepath.Join(st.TrustDir(), state.RepoHash(root)+".json"))
	if err != nil {
		t.Fatalf("approval wrote no record: %v", err)
	}
	if strings.Contains(string(b), root) {
		t.Fatalf("the trust record contains the repository path: %s", b)
	}
	// It has to hold the thing it is for, or it is not a record.
	if !strings.Contains(string(b), state.RepoHash(root)) {
		t.Fatalf("the trust record does not name the repository it is for: %s", b)
	}
}

// A record this build cannot read is refused, and so is one that reads
// as JSON but was written by a build that recorded something else. The
// two are told apart so the second does not read as corruption.
func TestARecordFromAnotherBuildIsUntrusted(t *testing.T) {
	st, root, c := setup(t)
	raw, err := json.Marshal(Record{SchemaVersion: SchemaVersion + 1, RepoHash: state.RepoHash(root)})
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(st.TrustDir(), state.RepoHash(root)+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	s := Check(st, root, c)
	if s.Trusted {
		t.Fatal("a record from a later schema version was trusted")
	}
	if !strings.Contains(s.Reason, "not readable by this build") {
		t.Errorf("the reason is %q", s.Reason)
	}
}

// Strict is approved per repository and per machine, and nothing else
// grants it. Both answers are asserted, because a function that always
// said yes would pass a test that only ever approved first.
func TestStrictIsApprovedOnlyWhereItWasApproved(t *testing.T) {
	st, root, _ := setup(t)
	if StrictApproved(st, root) {
		t.Fatal("strict was approved before anybody approved it")
	}
	if err := ApproveStrict(st, root); err != nil {
		t.Fatal(err)
	}
	if !StrictApproved(st, root) {
		t.Fatal("strict was approved and does not read as approved")
	}
	if StrictApproved(st, filepath.Join(root, "elsewhere")) {
		t.Fatal("approving one repository approved another")
	}
}
