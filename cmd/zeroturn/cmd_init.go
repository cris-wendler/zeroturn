package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
)

type detected struct {
	Root        string
	Branch      string
	Upstream    string
	Remotes     []string
	Language    string
	PkgManager  string
	Steps       []config.Step
	Harnesses   []string
	MissingExec []string
}

func cmdInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	force := fs.Bool("force", false, "replace an existing "+config.FileName)
	yes := fs.Bool("yes", false, "write the proposed configuration without asking")
	if err := fs.Parse(args); err != nil {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn could not read the flags", err.Error(), "run zeroturn init --help")
	}

	repo, err := openRepo(ctx)
	if err != nil {
		return err
	}
	if config.Exists(repo.Root) && !*force {
		return output.Errorf(output.ExitInvalidUsage,
			"zeroturn init wrote nothing",
			config.FileName+" already exists in this repository",
			"edit it directly, or run zeroturn init --force to replace it")
	}

	d := detect(ctx, repo.Root)
	d.Branch, _ = repo.Branch(ctx)
	d.Upstream = repo.Upstream(ctx)
	d.Remotes = repo.Remotes(ctx)

	c := config.Default()
	c.Verify.Steps = d.Steps
	if len(d.Remotes) > 0 {
		c.Git.Remote = d.Remotes[0]
		for _, r := range d.Remotes {
			if r == "origin" {
				c.Git.Remote = "origin"
			}
		}
	}

	fmt.Println("ZEROTURN INIT")
	fmt.Print(output.Table([][2]string{
		{"repository", d.Root},
		{"branch", orUnavailable(d.Branch)},
		{"upstream", orUnavailable(d.Upstream)},
		{"remote", orUnavailable(c.Git.Remote)},
		{"language", orUnavailable(d.Language)},
		{"package manager", orUnavailable(d.PkgManager)},
		{"coding harnesses", joinOrUnavailable(d.Harnesses)},
	}))

	fmt.Println("proposed validation steps:")
	if len(c.Verify.Steps) == 0 {
		fmt.Println("  none found, add them to verify.steps yourself")
	}
	for _, s := range c.Verify.Steps {
		fmt.Printf("  %-10s %v\n", s.Name, s.Command)
	}
	fmt.Println()

	b, _ := json.MarshalIndent(c, "", "  ")
	fmt.Printf("proposed %s:\n%s\n\n", config.FileName, string(b))

	if !*yes {
		ok, cerr := confirm("Write this configuration?")
		if cerr != nil {
			return cerr
		}
		if !ok {
			return output.Errorf(output.ExitDeclined, "zeroturn init wrote nothing",
				"the proposal was declined", "run zeroturn init again when you are ready")
		}
	}
	if err := config.Save(repo.Root, c); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn init could not write the file",
			err.Error(), "check that the repository root is writable")
	}
	fmt.Printf("Wrote %s\n", config.Path(repo.Root))
	fmt.Println("Guard mode is observe, which displays session condition and never blocks a tool.")
	fmt.Println("Next: zeroturn integrate claude --plan")
	return nil
}

func orUnavailable(s string) string {
	if s == "" {
		return "unavailable"
	}
	return s
}

func joinOrUnavailable(s []string) string {
	if len(s) == 0 {
		return "none detected"
	}
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

// detect proposes validation steps only for scripts that already exist in
// the project, so init never writes a command that cannot run.
func detect(ctx context.Context, root string) detected {
	d := detected{Root: root}

	if b, err := ioutil.ReadFile(filepath.Join(root, "package.json")); err == nil {
		d.Language = "javascript"
		d.PkgManager = nodePackageManager(root)
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(b, &pkg) == nil {
			runner := d.PkgManager
			if runner == "" {
				runner = "npm"
			}
			for _, name := range []string{"lint", "test", "build", "typecheck"} {
				if _, ok := pkg.Scripts[name]; !ok {
					continue
				}
				cmd := []string{runner, "run", name}
				if runner == "npm" && name == "test" {
					cmd = []string{"npm", "test"}
				}
				d.Steps = append(d.Steps, config.Step{Name: name, Command: cmd})
			}
		}
	}

	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		d.Language = appendLang(d.Language, "go")
		if d.PkgManager == "" {
			d.PkgManager = "go modules"
		}
		d.Steps = append(d.Steps,
			config.Step{Name: "vet", Command: []string{"go", "vet", "./..."}},
			config.Step{Name: "test", Command: []string{"go", "test", "./..."}},
		)
	}

	if _, err := os.Stat(filepath.Join(root, "Cargo.toml")); err == nil {
		d.Language = appendLang(d.Language, "rust")
		d.Steps = append(d.Steps,
			config.Step{Name: "test", Command: []string{"cargo", "test"}},
		)
	}

	if _, err := os.Stat(filepath.Join(root, "Makefile")); err == nil && len(d.Steps) == 0 {
		d.Steps = append(d.Steps, config.Step{Name: "make", Command: []string{"make"}})
	}

	var kept []config.Step
	for _, s := range d.Steps {
		if _, err := exec.LookPath(s.Command[0]); err != nil {
			d.MissingExec = append(d.MissingExec, s.Command[0])
			continue
		}
		kept = append(kept, s)
	}
	d.Steps = kept
	if d.Steps == nil {
		d.Steps = []config.Step{}
	}

	for name, bin := range map[string]string{"Claude Code": "claude", "GitHub Copilot CLI": "copilot"} {
		if _, err := exec.LookPath(bin); err == nil {
			d.Harnesses = append(d.Harnesses, name)
		}
	}
	return d
}

func appendLang(current, add string) string {
	if current == "" {
		return add
	}
	return current + ", " + add
}

func nodePackageManager(root string) string {
	for file, mgr := range map[string]string{
		"pnpm-lock.yaml":    "pnpm",
		"yarn.lock":         "yarn",
		"package-lock.json": "npm",
		"bun.lockb":         "bun",
	} {
		if _, err := os.Stat(filepath.Join(root, file)); err == nil {
			if _, err := exec.LookPath(mgr); err == nil {
				return mgr
			}
		}
	}
	return "npm"
}
