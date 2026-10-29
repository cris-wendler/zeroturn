package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

func linesOf(n int) string {
	return strings.Repeat("x\n", n)
}

func rangedReadGate(t *testing.T, work, path string, input map[string]interface{}) result {
	t.Helper()
	input["file_path"] = path
	b, err := json.Marshal(map[string]interface{}{
		"session_id": "s", "hook_event_name": "PreToolUse", "cwd": work,
		"tool_name": "Read", "tool_input": input,
	})
	if err != nil {
		t.Fatal(err)
	}
	return run(t, work, string(b), "event", "--harness", "claude", "--event", "PreToolUse")
}

func TestAWholeReadOfALongFileIsTurnedBackWithWhatToDoInstead(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "big.go", linesOf(config.DefaultMaxReadLines+1))
	d, reason := decision(t, readGate(t, work, filepath.Join(work, "big.go")))
	if d != "deny" {
		t.Fatalf("decision %q, want deny", d)
	}
	for _, want := range []string{"big.go", "1001 lines", "1000 lines", "Grep", "offset and limit"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q lacks %q", reason, want)
		}
	}

	// A last line with no newline after it is still a line.
	testutil.Write(t, work, "tail.go", linesOf(config.DefaultMaxReadLines)+"x")
	if d, _ := decision(t, readGate(t, work, filepath.Join(work, "tail.go"))); d != "deny" {
		t.Errorf("a file one line over the limit, ending without a newline: decision %q, want deny", d)
	}

	st, err := state.Open()
	if err != nil {
		t.Fatal(err)
	}
	s, ok, err := st.Load("s")
	if err != nil || !ok {
		t.Fatalf("no record: %v", err)
	}
	if s.SteeredReads != 2 {
		t.Errorf("steeredReads %d, want 2", s.SteeredReads)
	}
}

func TestALongReadIsLeftAloneWhenItNeedNotBeTurnedBack(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "big.go", linesOf(config.DefaultMaxReadLines+1))
	testutil.Write(t, work, "edge.go", linesOf(config.DefaultMaxReadLines))
	testutil.Write(t, work, "image.bin", linesOf(2000)+"\x00")
	testutil.Write(t, work, "book.ipynb", linesOf(2000))
	big := filepath.Join(work, "big.go")

	cases := map[string]result{
		"a read naming a limit":           rangedReadGate(t, work, big, map[string]interface{}{"limit": 200}),
		"a read naming an offset":         rangedReadGate(t, work, big, map[string]interface{}{"offset": 500}),
		"a file exactly at the limit":     readGate(t, work, filepath.Join(work, "edge.go")),
		"a file holding a zero byte":      readGate(t, work, filepath.Join(work, "image.bin")),
		"a notebook, shown by cell":       readGate(t, work, filepath.Join(work, "book.ipynb")),
		"a limit given as the wrong type": rangedReadGate(t, work, big, map[string]interface{}{"limit": "200"}),
	}
	for name, r := range cases {
		if r.code != 0 || r.stdout != "" {
			t.Errorf("%s: %+v, want silence", name, r)
		}
	}

	off, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Reads.MaxLines = 0 })
	testutil.Write(t, off, "big.go", linesOf(5000))
	if r := readGate(t, off, filepath.Join(off, "big.go")); r.stdout != "" {
		t.Errorf("switched off and still answered: %q", r.stdout)
	}
}

func TestACredentialInALongFileIsReportedAsACredential(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "deploy.env", "key="+awsKey+"\n"+linesOf(3000))
	d, reason := decision(t, readGate(t, work, filepath.Join(work, "deploy.env")))
	if d != "ask" || !strings.Contains(reason, "aws access key id") {
		t.Fatalf("decision %q reason %q, want the credential guard's ask", d, reason)
	}
}

func TestTheCredentialGuardStillRunsWhenTheRangeIsMalformed(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "deploy.env", "key="+awsKey+"\n")
	r := rangedReadGate(t, work, filepath.Join(work, "deploy.env"), map[string]interface{}{"offset": map[string]int{"x": 1}})
	if d, _ := decision(t, r); d != "ask" {
		t.Fatalf("decision %q: a range of the wrong type switched the credential guard off", d)
	}
}
