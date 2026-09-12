package main

import (
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// steps are proposed from what the project contains, so this checks the
// proposal itself rather than what happens to be installed on the
// machine running the tests.
func proposalFor(t *testing.T, files map[string]string) (string, string, []string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		testutil.Write(t, dir, name, content)
	}
	language, manager, steps := propose(dir)
	var described []string
	for _, s := range steps {
		described = append(described, s.Name+": "+strings.Join(s.Command, " "))
	}
	return language, manager, described
}

func TestProjectDetection(t *testing.T) {
	cases := []struct {
		name     string
		files    map[string]string
		language string
		manager  string
		want     []string
	}{
		{
			name:     "go",
			files:    map[string]string{"go.mod": "module example.com/app\n"},
			language: "go", manager: "go modules",
			want: []string{"vet: go vet ./...", "test: go test ./..."},
		},
		{
			name: "npm with scripts",
			files: map[string]string{"package.json": `{"scripts":{"lint":"eslint .","test":"jest","build":"tsc"}}`,
				"package-lock.json": "{}"},
			language: "javascript", manager: "npm",
			want: []string{"lint: npm run lint", "test: npm test", "build: npm run build"},
		},
		{
			name:     "python with ruff and mypy",
			files:    map[string]string{"pyproject.toml": "[tool.ruff]\nline-length = 100\n\n[tool.mypy]\nstrict = true\n"},
			language: "python", manager: "pip",
			want: []string{"lint: ruff check .", "typecheck: mypy .", "test: pytest"},
		},
		{
			name:     "python with poetry",
			files:    map[string]string{"pyproject.toml": "[tool.poetry]\n", "poetry.lock": ""},
			language: "python", manager: "poetry",
			want: []string{"test: poetry run pytest"},
		},
		{
			name:     "python with uv",
			files:    map[string]string{"pyproject.toml": "[project]\n", "uv.lock": ""},
			language: "python", manager: "uv",
			want: []string{"test: uv run pytest"},
		},
		{
			name:     "rust",
			files:    map[string]string{"Cargo.toml": "[package]\nname = \"app\"\n"},
			language: "rust", manager: "cargo",
			want: []string{"test: cargo test"},
		},
		{
			name:     "maven",
			files:    map[string]string{"pom.xml": "<project></project>"},
			language: "java", manager: "maven",
			want: []string{"test: mvn --batch-mode test"},
		},
		{
			name:     "gradle with a wrapper",
			files:    map[string]string{"build.gradle.kts": "plugins {}\n", "gradlew": "#!/bin/sh\n"},
			language: "java", manager: "gradle",
			want: []string{"test: gradlew test"},
		},
		{
			name:     "dotnet",
			files:    map[string]string{"App.csproj": "<Project></Project>"},
			language: "dotnet", manager: "dotnet",
			want: []string{"test: dotnet test"},
		},
		{
			name:     "ruby with rspec",
			files:    map[string]string{"Gemfile": "source 'https://rubygems.org'\n", "spec/app_spec.rb": ""},
			language: "ruby", manager: "bundler",
			want: []string{"test: bundle exec rspec"},
		},
		{
			name:  "a makefile, only for targets it declares",
			files: map[string]string{"Makefile": "help:\n\techo help\n\nlint:\n\techo lint\n\ntest:\n\techo test\n\nVAR := value\n"},
			want:  []string{"lint: make lint", "test: make test"},
		},
		{
			name: "a project with both go and javascript",
			files: map[string]string{"go.mod": "module example.com/app\n",
				"package.json": `{"scripts":{"lint":"eslint ."}}`},
			language: "javascript, go",
			want:     []string{"lint: npm run lint", "vet: go vet ./...", "test: go test ./..."},
		},
		{
			name:  "nothing recognisable",
			files: map[string]string{"README.md": "# app\n"},
			want:  nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			language, manager, got := proposalFor(t, c.files)
			if strings.Join(got, " | ") != strings.Join(c.want, " | ") {
				t.Errorf("steps\n got %v\nwant %v", got, c.want)
			}
			if c.language != "" && language != c.language {
				t.Errorf("language %q, want %q", language, c.language)
			}
			if c.manager != "" && manager != c.manager {
				t.Errorf("package manager %q, want %q", manager, c.manager)
			}
		})
	}
}

// A Makefile is a fallback. A project that already declares its own
// steps does not get make targets as well.
func TestMakefileIsOnlyAFallback(t *testing.T) {
	_, _, steps := proposalFor(t, map[string]string{
		"go.mod":   "module example.com/app\n",
		"Makefile": "test:\n\techo test\n",
	})
	for _, s := range steps {
		if strings.Contains(s, "make") {
			t.Fatalf("make was proposed alongside a Go project: %v", steps)
		}
	}
}

// Whatever is proposed must be a valid configuration.
func TestProposalsAreValidConfiguration(t *testing.T) {
	_, _, steps := proposalFor(t, map[string]string{
		"pyproject.toml": "[tool.ruff]\n[tool.mypy]\n",
	})
	if len(steps) == 0 {
		t.Fatal("nothing proposed")
	}
	dir := t.TempDir()
	testutil.Write(t, dir, "pyproject.toml", "[tool.ruff]\n[tool.mypy]\n")
	_, _, proposed := propose(dir)
	c := config.Default()
	c.Verify.Steps = proposed
	if err := c.Validate(); err != nil {
		t.Fatalf("a proposed configuration does not validate: %v", err)
	}
}
