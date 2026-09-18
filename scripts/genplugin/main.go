// Command genplugin writes the plugin's hooks.json from the hook list in
// internal/harness/claude. A test compares the file with what this
// produces, so the two cannot drift.
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cris-wendler/zeroturn/internal/harness/claude"
)

func main() {
	b, err := claude.PluginHooksJSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path := filepath.Join("integrations", "claude-plugin", "hooks", "hooks.json")
	if err := ioutil.WriteFile(path, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var check map[string]interface{}
	if err := json.Unmarshal(b, &check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", path)
}
