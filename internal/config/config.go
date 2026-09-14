// Package config loads, validates, and writes the per repository
// .zeroturn.json file.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// FileName is part of the public contract. Adapters and documentation
	// refer to this name directly.
	FileName = ".zeroturn.json"
	// Version is the configuration schema version, not the product version.
	Version = 1
)

// Credential guard modes. The guard scans a file the model is about to
// read and can ask before the content reaches the conversation.
// Prompt guard settings. Off is the default, because reading messages
// is a promise this project otherwise does not make, and because a
// blocked message is erased by the harness.
// Projection settings for the five hour window.
const (
	ProjectionOn  = "on"
	ProjectionOff = "off"
)

const (
	PromptsOff   = "off"
	PromptsBlock = "block"
)

const (
	CredentialOff  = "off"
	CredentialAsk  = "ask"
	CredentialDeny = "deny"
)

const (
	ModeObserve = "observe"
	ModeConfirm = "confirm"
	ModeStrict  = "strict"
)

type Config struct {
	Version int    `json:"version"`
	Guard   Guard  `json:"guard"`
	Verify  Verify `json:"verify"`
	Git     Git    `json:"git"`
}

type Guard struct {
	Mode        string      `json:"mode"`
	Context     Context     `json:"context"`
	Limits      Limits      `json:"limits"`
	Session     Session     `json:"session"`
	Credentials Credentials `json:"credentials"`
}

// Credentials controls what happens when the model asks to read a file
// that holds something shaped like a credential.
type Credentials struct {
	Mode string `json:"mode"`
	// Prompts is off unless the developer switches it on. When it is
	// block, ZeroTurn reads each message before it is sent, in memory,
	// and stops one that carries a credential.
	Prompts string `json:"prompts"`
}

type Context struct {
	Warn     int `json:"warn"`
	Confirm  int `json:"confirm"`
	Critical int `json:"critical"`
}

type Limits struct {
	FiveHourWarn int `json:"fiveHourWarn"`
	SevenDayWarn int `json:"sevenDayWarn"`
	// Projection asks when the rate of use will exhaust the five hour
	// window before it resets, whatever the reading is now. A heavy
	// session that will still finish inside the window stays quiet.
	Projection string `json:"projection"`
}

type Session struct {
	DurationWarnMinutes int `json:"durationWarnMinutes"`
	ActiveSubagentsWarn int `json:"activeSubagentsWarn"`
	SubagentStartsWarn  int `json:"subagentStartsWarn"`
}

type Verify struct {
	Steps []Step `json:"steps"`
}

type Step struct {
	Name string `json:"name"`
	// Command is an argument array. A shell string is rejected on load so
	// that repository configuration can never smuggle shell syntax into
	// an execution path.
	Command []string `json:"command"`
}

type Git struct {
	Remote            string   `json:"remote"`
	ProtectedBranches []string `json:"protectedBranches"`
}

// Default returns the operational defaults. These values are starting
// points chosen for a first run. They are not derived from measurement.
func Default() Config {
	return Config{
		Version: Version,
		Guard: Guard{
			Mode:        ModeObserve,
			Context:     Context{Warn: 70, Confirm: 80, Critical: 90},
			Limits:      Limits{FiveHourWarn: 75, SevenDayWarn: 75, Projection: ProjectionOn},
			Session:     Session{DurationWarnMinutes: 240, ActiveSubagentsWarn: 2, SubagentStartsWarn: 4},
			Credentials: Credentials{Mode: CredentialAsk, Prompts: PromptsOff},
		},
		Verify: Verify{Steps: []Step{}},
		Git:    Git{Remote: "origin", ProtectedBranches: []string{"main", "master"}},
	}
}

type ValidationError struct {
	Field  string
	Detail string
	Fix    string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Detail }

func Path(repoRoot string) string { return filepath.Join(repoRoot, FileName) }

func Load(repoRoot string) (Config, error) {
	b, err := ioutil.ReadFile(Path(repoRoot))
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("%s is not valid ZeroTurn configuration: %v", FileName, err)
	}
	// A file written before the credential guard existed has no mode.
	// It reads as the default rather than as a fault.
	if c.Guard.Credentials.Mode == "" {
		c.Guard.Credentials.Mode = CredentialAsk
	}
	if c.Guard.Credentials.Prompts == "" {
		c.Guard.Credentials.Prompts = PromptsOff
	}
	if c.Guard.Limits.Projection == "" {
		c.Guard.Limits.Projection = ProjectionOn
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func Exists(repoRoot string) bool {
	_, err := os.Stat(Path(repoRoot))
	return err == nil
}

// Save writes the file atomically so an interrupted write cannot leave
// a repository with a half written policy.
func Save(repoRoot string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return AtomicWrite(Path(repoRoot), b, 0644)
}

func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := ioutil.TempFile(dir, ".zeroturn-tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func pct(field string, v int) error {
	if v < 1 || v > 100 {
		return ValidationError{field, fmt.Sprintf("value %d is outside the range 1 to 100", v),
			"choose a whole number between 1 and 100"}
	}
	return nil
}

func (c Config) Validate() error {
	if c.Version != Version {
		return ValidationError{"version",
			fmt.Sprintf("file declares version %d, this build supports version %d", c.Version, Version),
			"run zeroturn init to write a supported file, or set version to 1"}
	}
	switch c.Guard.Mode {
	case ModeObserve, ModeConfirm, ModeStrict:
	default:
		return ValidationError{"guard.mode", "value " + quote(c.Guard.Mode) + " is not a guard mode",
			"use observe, confirm, or strict"}
	}
	switch c.Guard.Credentials.Mode {
	case CredentialOff, CredentialAsk, CredentialDeny:
	default:
		return ValidationError{"guard.credentials.mode", "value " + quote(c.Guard.Credentials.Mode) + " is not a credential guard mode",
			"use off, ask, or deny"}
	}
	switch c.Guard.Limits.Projection {
	case ProjectionOn, ProjectionOff:
	default:
		return ValidationError{"guard.limits.projection", "value " + quote(c.Guard.Limits.Projection) + " is not a projection setting",
			"use on, or off"}
	}
	switch c.Guard.Credentials.Prompts {
	case PromptsOff, PromptsBlock:
	default:
		return ValidationError{"guard.credentials.prompts", "value " + quote(c.Guard.Credentials.Prompts) + " is not a prompt guard setting",
			"use off, or block"}
	}
	// The percentages come from the key registry rather than a list
	// repeated here, so a new percentage setting is range checked without
	// anyone remembering to add it.
	for _, k := range keys {
		if k.Kind != KindPercent {
			continue
		}
		if err := pct(k.Name, *k.num(&c.Guard)); err != nil {
			return err
		}
	}
	if c.Guard.Context.Warn >= c.Guard.Context.Confirm {
		return ValidationError{"guard.context.warn",
			fmt.Sprintf("warn %d must stay below confirm %d", c.Guard.Context.Warn, c.Guard.Context.Confirm),
			"lower guard.context.warn or raise guard.context.confirm"}
	}
	if c.Guard.Context.Confirm >= c.Guard.Context.Critical {
		return ValidationError{"guard.context.confirm",
			fmt.Sprintf("confirm %d must stay below critical %d", c.Guard.Context.Confirm, c.Guard.Context.Critical),
			"lower guard.context.confirm or raise guard.context.critical"}
	}
	if c.Guard.Session.DurationWarnMinutes < 1 {
		return ValidationError{"guard.session.durationWarnMinutes", "value must be at least 1",
			"set a whole number of minutes"}
	}
	if c.Guard.Session.ActiveSubagentsWarn < 1 {
		return ValidationError{"guard.session.activeSubagentsWarn", "value must be at least 1",
			"set a whole number of subagents"}
	}
	if c.Guard.Session.SubagentStartsWarn < 1 {
		return ValidationError{"guard.session.subagentStartsWarn", "value must be at least 1",
			"set a whole number of subagent starts"}
	}
	seen := map[string]bool{}
	for i, s := range c.Verify.Steps {
		if strings.TrimSpace(s.Name) == "" {
			return ValidationError{fmt.Sprintf("verify.steps[%d].name", i), "name is empty",
				"give the step a short name such as lint or test"}
		}
		if seen[s.Name] {
			return ValidationError{fmt.Sprintf("verify.steps[%d].name", i), "name " + quote(s.Name) + " is used more than once",
				"give each step a distinct name"}
		}
		seen[s.Name] = true
		if len(s.Command) == 0 {
			return ValidationError{fmt.Sprintf("verify.steps[%d].command", i), "command array is empty",
				"write the command as an argument array, for example [\"npm\", \"test\"]"}
		}
		if strings.TrimSpace(s.Command[0]) == "" {
			return ValidationError{fmt.Sprintf("verify.steps[%d].command[0]", i), "executable name is empty",
				"put the executable in the first array element"}
		}
	}
	if strings.TrimSpace(c.Git.Remote) == "" {
		return ValidationError{"git.remote", "remote name is empty", "set a remote name such as origin"}
	}
	return nil
}

func quote(s string) string { return "\"" + s + "\"" }

// Hash covers the parts of the configuration that decide what ZeroTurn
// will execute. Trust approval is bound to this value, so a change to any
// verify step invalidates a previous approval.
func (c Config) Hash() string {
	h := sha256.New()
	fmt.Fprintf(h, "v%d\n", c.Version)
	steps := make([]Step, len(c.Verify.Steps))
	copy(steps, c.Verify.Steps)
	sort.Slice(steps, func(i, j int) bool { return steps[i].Name < steps[j].Name })
	for _, s := range steps {
		fmt.Fprintf(h, "step:%s\n", s.Name)
		for _, a := range s.Command {
			fmt.Fprintf(h, "arg:%d:%s\n", len(a), a)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c Config) IsProtected(branch string) bool {
	for _, b := range c.Git.ProtectedBranches {
		if b == branch {
			return true
		}
	}
	return false
}

func (c Config) Step(name string) (Step, bool) {
	for _, s := range c.Verify.Steps {
		if s.Name == name {
			return s, true
		}
	}
	return Step{}, false
}
