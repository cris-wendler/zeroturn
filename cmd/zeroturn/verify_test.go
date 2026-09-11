package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/trust"
	"github.com/cris-wendler/zeroturn/internal/verify"
)

func verifyRepo(t *testing.T, steps ...config.Step) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh for controlled step output")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Verify.Steps = steps })
	return work
}

func approve(t *testing.T, work string) {
	t.Helper()
	st, _ := state.Open()
	c, err := config.Load(work)
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.Approve(st, work, c); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRefusesUntrustedCommands(t *testing.T) {
	work := verifyRepo(t, config.Step{Name: "touch", Command: []string{"sh", "-c", "touch ran"}})
	r := run(t, work, "", "verify")
	if r.code != output.ExitDeclined || !strings.Contains(r.stdout, `"sh" "-c" "touch ran"`) {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(work, "ran")); err == nil {
		t.Fatal("an unapproved command ran")
	}
}

func TestVerifyPassAndFailExitCodes(t *testing.T) {
	work := verifyRepo(t,
		config.Step{Name: "lint", Command: []string{"sh", "-c", "exit 0"}},
		config.Step{Name: "test", Command: []string{"sh", "-c", "echo boom; exit 2"}},
	)
	approve(t, work)
	r := run(t, work, "", "verify")
	if r.code != output.ExitPolicyFailure {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{"PASS  lint", "FAIL  test", "boom", "Full output:"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, r.stdout)
		}
	}
}

func TestVerifyJSON(t *testing.T) {
	work := verifyRepo(t, config.Step{Name: "ok", Command: []string{"sh", "-c", "exit 0"}})
	approve(t, work)
	r := run(t, work, "", "verify", "--json")
	var res verify.Result
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &res) != nil || res.Passed != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestVerifyChangedConfigNeedsReapproval(t *testing.T) {
	work := verifyRepo(t, config.Step{Name: "ok", Command: []string{"sh", "-c", "exit 0"}})
	approve(t, work)
	c, _ := config.Load(work)
	c.Verify.Steps[0].Command = []string{"sh", "-c", "touch changed"}
	config.Save(work, c)
	if r := run(t, work, "", "verify"); r.code != output.ExitDeclined || !strings.Contains(r.stderr, "changed since approval") {
		t.Fatalf("%+v", r)
	}
}

func TestReportStatesProvenance(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeConfirm })
	statusline(t, work, "s", contextAt(84))
	gate(t, work, "s")
	r := run(t, work, "", "report", "current")
	for _, want := range []string{"Peak context              84%", "Confirmation requests     1", provenance} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("report lacks %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(strings.ToLower(r.stdout), "saving") || strings.Contains(strings.ToLower(r.stdout), "saved") {
		t.Error("report describes counts as savings")
	}
	j := run(t, work, "", "report", "current", "--json")
	var rep reportJSON
	if json.Unmarshal([]byte(j.stdout), &rep) != nil || rep.Provenance != provenance || rep.ConfirmRequests != 1 {
		t.Fatalf("%+v", j)
	}
}

func TestReportWithoutDataShowsUnavailable(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := run(t, work, "", "report", "week")
	if !strings.Contains(r.stdout, "Peak context              unavailable") {
		t.Fatalf("%s", r.stdout)
	}
}

func TestCapabilitiesJSON(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := run(t, work, "", "capabilities", "--json")
	var doc map[string]interface{}
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &doc) != nil {
		t.Fatalf("%+v", r)
	}
	if doc["makesNetworkRequests"] != false || doc["makesModelCalls"] != false {
		t.Fatalf("%v", doc)
	}
	for _, c := range doc["commands"].([]interface{}) {
		if c == "sync" {
			t.Fatal("capabilities lists sync, which is not built")
		}
	}
}
