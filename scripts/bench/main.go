// Command bench measures how long the commands that run inside a coding
// session take. It reports the median, minimum, and maximum of repeated
// runs rather than a single figure, because process start dominates and
// varies.
//
// Usage: go run ./scripts/bench [path to zeroturn] [runs]
package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"time"
)

type measurement struct {
	name     string
	note     string
	args     []string
	stdin    []byte
	duration []time.Duration
}

func main() {
	bin := "zeroturn"
	if len(os.Args) > 1 {
		bin = os.Args[1]
	}
	runs := 50
	if len(os.Args) > 2 {
		n, err := strconv.Atoi(os.Args[2])
		if err != nil || n < 1 {
			fmt.Fprintln(os.Stderr, "the number of runs must be a whole number of at least 1")
			os.Exit(2)
		}
		runs = n
	}
	abs, err := filepath.Abs(bin)
	if err == nil {
		bin = abs
	}

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "the working directory could not be read")
		os.Exit(1)
	}
	payload, err := ioutil.ReadFile(filepath.Join(root, "fixtures", "claude", "statusline-full.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "run this from the repository root:", err)
		os.Exit(1)
	}

	// The state directory is temporary so a measurement never touches the
	// records of a real session.
	stateDir, err := ioutil.TempDir("", "zeroturn-bench-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(stateDir)
	env := append(os.Environ(), "ZEROTURN_STATE_DIR="+stateDir, "NO_COLOR=1")

	list := []*measurement{
		{name: "startup", note: "zeroturn version, process start only", args: []string{"version"}},
		{name: "status line", note: "one status line payload on standard input", args: []string{"status", "--stdin", "--harness", "claude"}, stdin: payload},
		{name: "subagent gate", note: "one PreToolUse event, decision written", args: []string{"event", "--harness", "claude", "--event", "PreToolUse"},
			stdin: []byte(`{"session_id":"bench","hook_event_name":"PreToolUse","tool_name":"Agent","cwd":"` + root + `","tool_input":{"prompt":"x"}}`)},
		{name: "policy check", note: "reads the repository configuration and records", args: []string{"policy", "check"}},
		{name: "repository status", note: "reads the branch, so it starts git", args: []string{"status"}},
	}

	for _, m := range list {
		// One warm run first, so the measurements are not dominated by the
		// first read of the executable from disk.
		run(bin, m, env, root)
		for i := 0; i < runs; i++ {
			m.duration = append(m.duration, run(bin, m, env, root))
		}
	}

	info, err := os.Stat(bin)
	size := "unknown"
	if err == nil {
		size = fmt.Sprintf("%.1f MB", float64(info.Size())/(1024*1024))
	}

	fmt.Printf("executable   %s\n", bin)
	fmt.Printf("size         %s\n", size)
	fmt.Printf("platform     %s/%s, %d processors, Go %s\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())
	fmt.Printf("runs         %d for each command, after one warm up run\n\n", runs)
	fmt.Printf("| Command | Median | Minimum | Maximum | Notes |\n| --- | --- | --- | --- | --- |\n")
	for _, m := range list {
		sort.Slice(m.duration, func(i, j int) bool { return m.duration[i] < m.duration[j] })
		fmt.Printf("| %s | %s | %s | %s | %s |\n", m.name,
			ms(m.duration[len(m.duration)/2]), ms(m.duration[0]), ms(m.duration[len(m.duration)-1]), m.note)
	}
}

func ms(d time.Duration) string {
	return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
}

func run(bin string, m *measurement, env []string, dir string) time.Duration {
	cmd := exec.Command(bin, m.args...)
	cmd.Dir = dir
	cmd.Env = env
	if m.stdin != nil {
		cmd.Stdin = newReader(m.stdin)
	}
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = ioutil.Discard
	start := time.Now()
	if err := cmd.Run(); err != nil {
		// A command that reports a policy failure still measures the work.
		if _, ok := err.(*exec.ExitError); !ok {
			fmt.Fprintln(os.Stderr, m.name, "did not run:", err)
			os.Exit(1)
		}
	}
	return time.Since(start)
}

func newReader(b []byte) io.Reader { return bytes.NewReader(b) }
