// Package output defines ZeroTurn's public exit codes and the shared
// text and JSON writers used by every command.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Exit codes are part of the public contract. Changing the meaning of an
// existing code requires a major contract version.
const (
	ExitOK              = 0
	ExitPolicyFailure   = 1
	ExitInvalidUsage    = 2
	ExitUnsafeGit       = 3
	ExitMissingExec     = 4
	ExitInternal        = 5
	ExitDeclined        = 6
	ExitContractVersion = 7
	ExitNoIntegration   = 8
)

type Error struct {
	Code    int
	Stopped string
	Because string
	Next    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Stopped, e.Because)
}

// Errorf builds an error carrying the three parts every ZeroTurn failure
// message must state: what stopped, why, and the smallest safe next action.
func Errorf(code int, stopped, because, next string) *Error {
	return &Error{Code: code, Stopped: stopped, Because: because, Next: next}
}

func (e *Error) Print(w io.Writer) {
	fmt.Fprintf(w, "%s\n", e.Stopped)
	fmt.Fprintf(w, "  reason: %s\n", e.Because)
	if e.Next != "" {
		fmt.Fprintf(w, "  next:   %s\n", e.Next)
	}
}

func JSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type Color struct{ enabled bool }

// NewColor enables ANSI sequences only when the destination can show them.
// NO_COLOR and TERM=dumb always win, and a redirected stream stays plain
// unless a harness is rendering the output itself.
func NewColor(f *os.File, harness string) Color {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return Color{false}
	}
	if harness != "" {
		return Color{true}
	}
	info, err := f.Stat()
	if err != nil {
		return Color{false}
	}
	return Color{info.Mode()&os.ModeCharDevice != 0}
}

func (c Color) wrap(code, s string) string {
	if !c.enabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Escapes reports whether this destination is one ZeroTurn already
// writes escape sequences to. Anything beyond colour, such as moving the
// cursor or erasing a row, belongs behind the same answer rather than
// behind a second guess at what the terminal can do.
func (c Color) Escapes() bool { return c.enabled }

func (c Color) Dim(s string) string    { return c.wrap("2", s) }
func (c Color) Green(s string) string  { return c.wrap("32", s) }
func (c Color) Yellow(s string) string { return c.wrap("33", s) }
func (c Color) Red(s string) string    { return c.wrap("31", s) }
func (c Color) Bold(s string) string   { return c.wrap("1", s) }

func Table(rows [][2]string) string {
	width := 0
	for _, r := range rows {
		if len(r[0]) > width {
			width = len(r[0])
		}
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r[0])
		b.WriteString(strings.Repeat(" ", width-len(r[0])+2))
		b.WriteString(r[1])
		b.WriteString("\n")
	}
	return b.String()
}
