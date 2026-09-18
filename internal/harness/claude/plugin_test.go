package claude

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

func pluginRoot() string {
	return filepath.Join("..", "..", "..", "integrations", "claude-plugin")
}

// The committed file is what a plugin installs. Generating it and
// comparing bytes is the only thing that stops it drifting from the hook
// list, which is the same guarantee the hand install file gets.
func TestTheCommittedPluginHooksAreTheGeneratedOnes(t *testing.T) {
	want, err := PluginHooksJSON()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pluginRoot(), "hooks", "hooks.json")
	got, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s is not what the hook list produces. Run go run ./scripts/genplugin", path)
	}
}

// A plugin cannot contribute a status line, so it must not appear to.
// Every measurement the gate reads arrives on one, and a file claiming
// otherwise would promise a working guard.
func TestThePluginInstallsNoStatusLine(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join(pluginRoot(), "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "statusLine") || strings.Contains(string(b), "status --stdin") {
		t.Error("the plugin hooks name a status line, and a plugin cannot install one")
	}
}

// The plugin does not carry the binary, so a hook naming an absolute
// path would point at whatever machine built the file.
func TestThePluginCallsTheExecutableByName(t *testing.T) {
	for event, entries := range PluginHooks() {
		for _, e := range entries {
			for _, in := range e.Hooks {
				if strings.ContainsAny(in.Command, `/\`) {
					t.Errorf("%s runs %q, which names a path", event, in.Command)
				}
				if !strings.HasPrefix(in.Command, PluginExecutable+" ") {
					t.Errorf("%s runs %q, which does not start with the executable name", event, in.Command)
				}
				if !Owned(in.Command) {
					t.Errorf("%s runs %q, which ZeroTurn would not recognise as its own", event, in.Command)
				}
			}
		}
	}
}

// The plugin and the installer have to install the same hooks, or which
// one you used changes what the guard sees.
func TestThePluginInstallsTheSameHooksAsIntegrate(t *testing.T) {
	plugin := PluginHooks()
	count := 0
	for _, h := range Hooks {
		found := false
		for _, e := range plugin[h.Event] {
			if e.Matcher == h.Matcher {
				found = true
			}
		}
		if !found {
			t.Errorf("integrate writes %s with matcher %q and the plugin does not, which is %s",
				h.Event, h.Matcher, h.Purpose)
		}
		count++
	}
	got := 0
	for _, entries := range plugin {
		got += len(entries)
	}
	if got != count {
		t.Errorf("the plugin installs %d entries and integrate writes %d", got, count)
	}
}

// The prompt guard is off by default and reads what a person types, so a
// plugin must not switch it on by being installed.
func TestThePluginDoesNotInstallThePromptGuard(t *testing.T) {
	if _, ok := PluginHooks()["UserPromptSubmit"]; ok {
		t.Error("the plugin installs the prompt guard, which is opt in")
	}
}

func TestThePluginManifestsAreValid(t *testing.T) {
	var manifest struct {
		Name  string `json:"name"`
		Hooks string `json:"hooks"`
	}
	b, err := ioutil.ReadFile(filepath.Join(pluginRoot(), ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatalf("plugin.json is not valid JSON: %v", err)
	}
	if manifest.Name == "" {
		t.Error("plugin.json has no name, which is the one required field")
	}
	// The path it names has to be there, or the plugin installs nothing.
	if manifest.Hooks == "" {
		t.Fatal("plugin.json declares no hooks file")
	}
	if _, err := ioutil.ReadFile(filepath.Join(pluginRoot(), filepath.FromSlash(strings.TrimPrefix(manifest.Hooks, "./")))); err != nil {
		t.Errorf("plugin.json points at %s, which cannot be read: %v", manifest.Hooks, err)
	}

	var market struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	mb, err := ioutil.ReadFile(filepath.Join("..", "..", "..", ".claude-plugin", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mb, &market); err != nil {
		t.Fatalf("marketplace.json is not valid JSON: %v", err)
	}
	if len(market.Plugins) == 0 {
		t.Fatal("the marketplace lists no plugins")
	}
	for _, p := range market.Plugins {
		if p.Name != manifest.Name {
			t.Errorf("the marketplace lists %q and the manifest says %q", p.Name, manifest.Name)
		}
		if _, err := ioutil.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(strings.TrimPrefix(p.Source, "./")), ".claude-plugin", "plugin.json")); err != nil {
			t.Errorf("the marketplace points at %s, which has no manifest: %v", p.Source, err)
		}
	}
}
