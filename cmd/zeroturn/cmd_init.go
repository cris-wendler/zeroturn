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
	"runtime"
	"strings"

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

const initUsage = `zeroturn init [--force] [--yes]

Write .zeroturn.json for this repository. The validation steps are
proposed from what the project declares, never guessed, and the file is
shown before it is written.
`

func cmdInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "replace an existing "+config.FileName)
	yes := fs.Bool("yes", false, "write the proposed configuration without asking")
	if err := parseFlags(fs, args, initUsage, "init"); err != nil {
		return err
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

	// The proposed file is what a person is being asked to approve, so it
	// is printed for them to read. With --yes there is no question, and
	// fifty lines of JSON is then the answer to one nobody asked.
	if !*yes {
		b, _ := json.MarshalIndent(c, "", "  ")
		fmt.Printf("proposed %s:\n%s\n\n", config.FileName, string(b))

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
	d.Language, d.PkgManager, d.Steps = propose(root)

	// A step whose executable is not installed is reported rather than
	// proposed, because a configuration that cannot run is worse than an
	// empty one.
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
	d.Harnesses = detectHarnesses()
	return d
}

// detectHarnesses reports which coding harnesses are installed, so init
// can say what the integration would connect to.
func detectHarnesses() []string {
	var out []string
	for _, h := range []struct{ name, bin string }{
		{"Claude Code", "claude"},
		{"GitHub Copilot CLI", "copilot"},
	} {
		if _, err := exec.LookPath(h.bin); err == nil {
			out = append(out, h.name)
		}
	}
	return out
}

// propose reads the project and suggests validation steps. It is
// separate from detect so it can be tested without the toolchains it
// names being installed.
func propose(root string) (language, pkgManager string, steps []config.Step) {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	}
	add := func(name string, command ...string) {
		steps = append(steps, config.Step{Name: name, Command: command})
	}

	if b, err := ioutil.ReadFile(filepath.Join(root, "package.json")); err == nil {
		language = "javascript"
		pkgManager = nodePackageManager(root)
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(b, &pkg) == nil {
			runner := pkgManager
			if runner == "" {
				runner = "npm"
			}
			for _, name := range []string{"lint", "typecheck", "test", "build"} {
				if _, ok := pkg.Scripts[name]; !ok {
					continue
				}
				cmd := []string{runner, "run", name}
				if runner == "npm" && name == "test" {
					cmd = []string{"npm", "test"}
				}
				add(name, cmd...)
			}
		}
	}

	if has("go.mod") {
		language = appendLang(language, "go")
		if pkgManager == "" {
			pkgManager = "go modules"
		}
		add("vet", "go", "vet", "./...")
		add("test", "go", "test", "./...")
	}

	if has("Cargo.toml") {
		language = appendLang(language, "rust")
		if pkgManager == "" {
			pkgManager = "cargo"
		}
		add("test", "cargo", "test")
	}

	if python, runner := pythonProject(root); python {
		language = appendLang(language, "python")
		if pkgManager == "" {
			pkgManager = runner
		}
		prefix := []string{}
		switch runner {
		case "poetry":
			prefix = []string{"poetry", "run"}
		case "uv":
			prefix = []string{"uv", "run"}
		}
		content, _ := ioutil.ReadFile(filepath.Join(root, "pyproject.toml"))
		text := string(content)
		if strings.Contains(text, "[tool.ruff") {
			add("lint", append(append([]string{}, prefix...), "ruff", "check", ".")...)
		}
		if strings.Contains(text, "[tool.mypy") {
			add("typecheck", append(append([]string{}, prefix...), "mypy", ".")...)
		}
		add("test", append(append([]string{}, prefix...), "pytest")...)
	}

	if has("pom.xml") {
		language = appendLang(language, "java")
		if pkgManager == "" {
			pkgManager = "maven"
		}
		runner := "mvn"
		if w, ok := wrapper(root, "mvnw"); ok {
			runner = w
		}
		add("test", runner, "--batch-mode", "test")
	}
	if has("build.gradle") || has("build.gradle.kts") {
		language = appendLang(language, "java")
		if pkgManager == "" {
			pkgManager = "gradle"
		}
		runner := "gradle"
		if w, ok := wrapper(root, "gradlew"); ok {
			runner = w
		}
		add("test", runner, "test")
	}

	if projects, _ := filepath.Glob(filepath.Join(root, "*.csproj")); len(projects) > 0 {
		language = appendLang(language, "dotnet")
		if pkgManager == "" {
			pkgManager = "dotnet"
		}
		add("test", "dotnet", "test")
	}

	if has("Gemfile") {
		language = appendLang(language, "ruby")
		if pkgManager == "" {
			pkgManager = "bundler"
		}
		switch {
		case has("spec"):
			add("test", "bundle", "exec", "rspec")
		case has("Rakefile"):
			add("test", "bundle", "exec", "rake", "test")
		}
	}

	// A Makefile is used only when nothing else was found, and only for
	// targets it actually declares.
	if len(steps) == 0 {
		for _, target := range makeTargets(root) {
			add(target, "make", target)
		}
	}
	return language, pkgManager, steps
}

// pythonProject reports whether this is a Python project and which
// runner its lock file implies.
func pythonProject(root string) (bool, string) {
	found := false
	for _, name := range []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt", "tox.ini"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			found = true
			break
		}
	}
	if !found {
		return false, ""
	}
	if _, err := os.Stat(filepath.Join(root, "poetry.lock")); err == nil {
		return true, "poetry"
	}
	if _, err := os.Stat(filepath.Join(root, "uv.lock")); err == nil {
		return true, "uv"
	}
	return true, "pip"
}

// makeTargets reads the names of the targets a Makefile declares, so a
// proposal names one that exists rather than a guess.
func makeTargets(root string) []string {
	b, err := ioutil.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return nil
	}
	declared := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		if len(line) == 0 || line[0] == '\t' || line[0] == '#' || line[0] == ' ' {
			continue
		}
		colon := strings.Index(line, ":")
		if colon <= 0 || strings.Contains(line[:colon], "=") {
			continue
		}
		declared[strings.TrimSpace(line[:colon])] = true
	}
	var out []string
	for _, name := range []string{"lint", "check", "test", "build"} {
		if declared[name] {
			out = append(out, name)
		}
	}
	return out
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

// wrapper returns the absolute path of a build wrapper a project ships
// with it, such as gradlew or mvnw.
//
// The absolute path matters twice. exec.LookPath searches PATH for a bare
// name, so "gradlew" was never found in the repository and the step was
// dropped every time; and filepath.Join(".", "gradlew") cleans the "."
// away, which is how that bare name arose. On Windows the wrapper that
// runs is the .bat, because the extensionless file beside it is a POSIX
// shell script that CreateProcess cannot start.
func wrapper(root, name string) (string, bool) {
	candidates := []string{name}
	if runtime.GOOS == "windows" {
		candidates = []string{name + ".bat", name + ".cmd"}
	}
	for _, c := range candidates {
		p := filepath.Join(root, c)
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		if abs, aerr := filepath.Abs(p); aerr == nil {
			return abs, true
		}
		return p, true
	}
	return "", false
}
