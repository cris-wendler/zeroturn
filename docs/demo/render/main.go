// Command render turns a transcript from docs/demo/record.sh into the
// animated SVG shown at the top of the README.
//
// The transcript is real program output. Lines starting with "$ " are
// typed one character at a time, "---" clears the screen, and every other
// line is output that appears after its command. ANSI color codes in the
// output are kept. Lines without codes get the colors the CLI uses in a
// terminal, because the recording pipes output and so loses them.
//
// The animation uses CSS keyframes only, which GitHub renders in an image
// without scripts. Each visible element owns one keyframe rule that shows
// it for its time window within the loop.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	fontSize   = 16.0
	charWidth  = 9.65 // close to 0.6em, the advance width of common monospace fonts
	lineHeight = 23.0
	padX       = 20.0
	padTop     = 56.0
	padBottom  = 20.0

	idleBeforeTyping = 200  // ms with the prompt and cursor shown
	typeBase         = 11   // ms per character, before jitter
	typeSpace        = 36   // ms for a space, which reads as a word boundary
	afterTyping      = 220  // ms between the last character and the output
	outputLine       = 38   // ms between ordinary output lines
	outputResult     = 260  // ms before a PASS or FAIL line, which reflects work
	holdScreen       = 1250 // ms a finished screen stays before it clears
)

type span struct {
	text  string
	class string
}

type item struct {
	command bool
	spans   []span
	// rows holds a typed command that continues onto another line, the
	// way a shell does with a trailing backslash.
	rows [][]span
}

type element struct {
	row      int
	spans    []span
	from, to int // ms
}

func main() {
	frame := flag.Int("frame", -1, "write a still image of the moment this many milliseconds into the loop, for review")
	flag.Parse()
	screens, err := parse(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "render stopped:", err)
		os.Exit(1)
	}
	elements, total, cols, rows := timeline(screens)
	if *frame >= 0 {
		var still []element
		for _, e := range elements {
			if e.from <= *frame && *frame < e.to {
				e.from, e.to = 0, total
				still = append(still, e)
			}
		}
		elements = still
	}
	fmt.Fprintf(os.Stderr, "loop %.1fs, %d elements, %d columns, %d rows\n", float64(total)/1000, len(elements), cols, rows)
	writeSVG(os.Stdout, elements, total, cols, rows)
}

func parse(r io.Reader) ([][]item, error) {
	var lines []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	var screens [][]item
	var cur []item
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case line == "---":
			screens = append(screens, cur)
			cur = nil
		case strings.HasPrefix(line, "$ "):
			text := line[2:]
			rows := [][]span{append([]span{{"$ ", "p"}}, commandSpans(text)...)}
			// A shell continues a command after a trailing backslash, and
			// prompts for the rest with a second prompt character.
			for strings.HasSuffix(text, "\\") && i+1 < len(lines) {
				i++
				text = strings.TrimLeft(lines[i], " \t")
				rows = append(rows, append([]span{{"> ", "c"}}, commandSpans(text)...))
			}
			cur = append(cur, item{command: true, rows: rows})
		default:
			cur = append(cur, item{spans: colorize(line)})
		}
	}
	if len(cur) > 0 {
		screens = append(screens, cur)
	}
	if len(screens) == 0 {
		return nil, fmt.Errorf("the transcript on standard input is empty")
	}
	return screens, nil
}

// commandSpans colors a typed command: the program, the subcommand, flags,
// quoted text, and shell operators each get their own color.
func commandSpans(cmd string) []span {
	var out []span
	word := 0
	for len(cmd) > 0 {
		if cmd[0] == ' ' {
			out = append(out, span{" ", ""})
			cmd = cmd[1:]
			continue
		}
		end := strings.IndexByte(cmd, ' ')
		if cmd[0] == '"' {
			if q := strings.IndexByte(cmd[1:], '"'); q >= 0 {
				end = q + 2
			}
		}
		if end < 0 {
			end = len(cmd)
		}
		tok := cmd[:end]
		cmd = cmd[end:]
		class := "w"
		switch {
		case word == 0:
			class = "cmd b"
		case word == 1:
			class = "sub b"
		case strings.HasPrefix(tok, "--"):
			class = "flag"
		case strings.HasPrefix(tok, `"`):
			class = "str"
		case tok == "<" || tok == "|" || tok == ">":
			class = "op"
		}
		out = append(out, span{tok, class})
		word++
	}
	return out
}

var fieldNames = map[string]bool{
	"files:": true, "message:": true, "branch:": true, "remote:": true,
	"mode": true, "level": true, "decision": true, "thresholds crossed:": true,
}

// colorize keeps ANSI colors when present and otherwise applies the colors
// the CLI uses on a terminal, plus colors for field names and decisions so
// the recording is easier to scan. Every state is also written as a word,
// so nothing depends on color alone.
func colorize(line string) []span {
	if strings.Contains(line, "\x1b[") {
		return fromANSI(line)
	}
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "# "):
		return []span{{line, "m"}}
	case strings.HasPrefix(line, "Reading this file"), strings.HasPrefix(line, "Your message was not sent"),
		strings.HasPrefix(line, "line ") && strings.Contains(line, "looks like"):
		return []span{{line, "y"}}
	case strings.HasPrefix(line, "PASS"):
		return []span{{"PASS", "g b"}, {line[4:], ""}}
	case strings.HasPrefix(line, "FAIL"):
		return []span{{"FAIL", "r b"}, {line[4:], ""}}
	case line == "READY TO SHIP":
		return []span{{line, "g b hl"}}
	case strings.HasPrefix(line, "ZEROTURN "):
		return []span{{line, "h b"}}
	case strings.HasPrefix(line, "Result: ") && !strings.Contains(line, "failed"):
		return []span{{line, "g"}}
	case fieldNames[trimmed]:
		return []span{{line, "k"}}
	}
	if f := strings.Fields(line); len(f) >= 2 && fieldNames[f[0]] && !strings.HasPrefix(line, " ") {
		key := line[:strings.Index(line, f[0])+len(f[0])]
		rest := line[len(key):]
		value := strings.TrimSpace(rest)
		class := "w b"
		switch value {
		case "ask", "confirm":
			class = "y b"
		case "deny", "critical":
			class = "r b"
		case "allow", "ok":
			class = "g b"
		}
		switch key {
		case "remote:", "files:", "message:", "branch:":
			class = "w"
		case "mode":
			class = "w b"
		}
		pad := rest[:len(rest)-len(strings.TrimLeft(rest, " "))]
		return []span{{key, "k"}, {pad, ""}, {value, class}}
	}
	if strings.HasPrefix(line, "  ") && strings.Contains(line, "(limit ") {
		f := strings.Fields(line)
		name := f[0]
		i := strings.Index(line, name) + len(name)
		j := strings.LastIndex(line, "(limit ")
		return []span{{line[:i], "o"}, {line[i:j], "w"}, {line[j:], "m"}}
	}
	return []span{{line, ""}}
}

func fromANSI(line string) []span {
	var out []span
	class := ""
	for len(line) > 0 {
		i := strings.Index(line, "\x1b[")
		if i < 0 {
			out = append(out, span{line, class})
			break
		}
		if i > 0 {
			out = append(out, span{line[:i], class})
		}
		line = line[i+2:]
		m := strings.IndexByte(line, 'm')
		if m < 0 {
			break
		}
		switch line[:m] {
		case "0":
			class = ""
		case "1":
			class = "b"
		case "2":
			class = "m"
		case "31":
			class = "r"
		case "32":
			class = "g"
		case "33":
			class = "y"
		}
		line = line[m+1:]
	}
	return out
}

func textWidth(spans []span) int {
	n := 0
	for _, s := range spans {
		n += len([]rune(s.text))
	}
	return n
}

// jitter varies typing speed deterministically so the recording looks
// typed by a person and still renders identically every time.
func jitter(i int) int { return (i * 7919 % 7) * 3 }

func timeline(screens [][]item) ([]element, int, int, int) {
	var els []element
	t, cols, rows := 0, 0, 0
	for _, screen := range screens {
		start := len(els)
		row := 0
		for _, it := range screen {
			widest := textWidth(it.spans)
			for _, r := range it.rows {
				if w := textWidth(r); w > widest {
					widest = w
				}
			}
			if widest+2 > cols {
				cols = widest + 2
			}
			if !it.command {
				delay := outputLine
				if len(it.spans) > 0 && (it.spans[0].text == "PASS" || it.spans[0].text == "FAIL") {
					delay = outputResult
				}
				t += delay
				els = append(els, element{row: row, spans: it.spans, from: t})
				row++
				continue
			}
			for r, full := range it.rows {
				prompt := full[:1]
				rest := full[1:]
				cmd := []rune(plain(rest))
				els = append(els, element{row: row, spans: withCursor(prompt, nil), from: t, to: t + idleBeforeTyping})
				t += idleBeforeTyping
				for i := 1; i <= len(cmd); i++ {
					d := typeBase + jitter(i)
					if cmd[i-1] == ' ' {
						d = typeSpace
					}
					next := t + d
					if i == len(cmd) && r == len(it.rows)-1 {
						next = t + afterTyping
					}
					els = append(els, element{row: row, spans: withCursor(prompt, prefix(rest, i)), from: t, to: next})
					t = next
				}
				els = append(els, element{row: row, spans: full, from: t})
				row++
			}
		}
		if row > rows {
			rows = row
		}
		t += holdScreen
		for i := start; i < len(els); i++ {
			if els[i].to == 0 {
				els[i].to = t
			}
		}
	}
	return els, t, cols, rows
}

func withCursor(prompt, typed []span) []span {
	out := append(append([]span{}, prompt...), typed...)
	return append(out, span{"█", "c"})
}

func plain(spans []span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.text)
	}
	return b.String()
}

// prefix returns the first n characters of spans, keeping each color.
func prefix(spans []span, n int) []span {
	var out []span
	for _, s := range spans {
		r := []rune(s.text)
		if n <= 0 {
			break
		}
		if len(r) > n {
			r = r[:n]
		}
		out = append(out, span{string(r), s.class})
		n -= len(r)
	}
	return out
}

func pct(ms, total int) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", float64(ms)*100/float64(total)), "0"), ".")
}

func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func writeSVG(w io.Writer, els []element, total, cols, rows int) {
	width := padX*2 + float64(cols)*charWidth
	height := padTop + float64(rows)*lineHeight + padBottom
	b := bufio.NewWriter(w)
	defer b.Flush()

	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f" role="img" aria-labelledby="title desc" xml:space="preserve">`+"\n",
		width, height, width, height)
	b.WriteString(`<title id="title">ZeroTurn terminal demonstration</title>` + "\n")
	b.WriteString(`<desc id="desc">A terminal types four commands. The ZeroTurn status line shows context at 82 percent, five hour usage at 81 percent, seven day usage at 47 percent, a session of 3 hours 12 minutes, 2 active subagents, and the word ask. zeroturn policy check shows the decision ask, with three thresholds crossed: context 82 percent over a limit of 80, five hour usage 81 percent over 75, and 2 active subagents at a limit of 2. zeroturn verify passes two checks, and zeroturn ship with dry run passes the same checks and prints READY TO SHIP. The session values are sample data.</desc>` + "\n")
	b.WriteString("<style>\n")
	fmt.Fprintf(b, `text{font-family:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,"Liberation Mono",monospace;font-size:%.0fpx;fill:#e6edf3;white-space:pre}`+"\n", fontSize)
	fmt.Fprintf(b, ".a{opacity:0;animation-duration:%dms;animation-iteration-count:infinite;animation-timing-function:step-end}\n", total)
	// Colors are from the GitHub dark palette, chosen for contrast of at
	// least 4.5 to 1 against the background.
	b.WriteString(".p{fill:#56d364}.w{fill:#e6edf3}.c{fill:#9da7b3}.g{fill:#56d364}.y{fill:#e3b341}.r{fill:#ff7b72}.m{fill:#9da7b3}.b{font-weight:700}\n")
	b.WriteString(".cmd{fill:#56d4dd}.sub{fill:#f0f6fc}.flag{fill:#d2a8ff}.str{fill:#ffa657}.op{fill:#ff7b72}.h{fill:#79c0ff}.k{fill:#79c0ff}.o{fill:#ffa657}\n")
	b.WriteString(".t{font-size:13px;fill:#9da7b3}.hlb{fill:#238636;fill-opacity:.28}\n")
	for i, e := range els {
		fmt.Fprintf(b, "@keyframes k%d{", i)
		if e.from == 0 {
			b.WriteString("0%{opacity:1}")
		} else {
			fmt.Fprintf(b, "0%%{opacity:0}%s%%{opacity:1}", pct(e.from, total))
		}
		fmt.Fprintf(b, "%s%%{opacity:0}100%%{opacity:0}}\n", pct(e.to, total))
	}
	b.WriteString("@media (prefers-reduced-motion:reduce){.a{animation:none}.f{opacity:1}}\n")
	b.WriteString("</style>\n")

	fmt.Fprintf(b, `<rect x="0.5" y="0.5" width="%.0f" height="%.0f" rx="10" fill="#0d1117" stroke="#30363d"/>`+"\n", width-1, height-1)
	fmt.Fprintf(b, `<path d="M0.5 32.5H%.1f" stroke="#21262d"/>`+"\n", width-0.5)
	b.WriteString(`<circle cx="20" cy="16.5" r="6" fill="#ff5f57"/><circle cx="40" cy="16.5" r="6" fill="#febc2e"/><circle cx="60" cy="16.5" r="6" fill="#28c840"/>` + "\n")
	fmt.Fprintf(b, `<text class="t" x="%.0f" y="21" text-anchor="middle">zeroturn</text>`+"\n", width/2)
	fmt.Fprintf(b, `<text class="t" x="%.0f" y="21" text-anchor="end">sample session data</text>`+"\n", width-padX)

	// Under reduced motion the finished last screen is shown without
	// animation, so the image still carries the result.
	lastScreenStart := 0
	for i, e := range els {
		if e.to == total && (i == 0 || els[i-1].to != total) {
			lastScreenStart = i
		}
	}
	for i, e := range els {
		cls := "a"
		if i >= lastScreenStart && e.to == total && !isCursor(e.spans) {
			cls = "a f"
		}
		y := padTop + float64(e.row)*lineHeight
		fmt.Fprintf(b, `<g class="%s" style="animation-name:k%d">`, cls, i)
		if hasClass(e.spans, "hl") {
			fmt.Fprintf(b, `<rect class="hlb" x="%.0f" y="%.1f" width="%.1f" height="%.0f" rx="4"/>`,
				padX-6, y-2, float64(textWidth(e.spans))*charWidth+12, lineHeight)
		}
		fmt.Fprintf(b, `<text x="%.0f" y="%.1f">`, padX, y+fontSize)
		for _, s := range e.spans {
			if s.text == "" {
				continue
			}
			if s.class == "" {
				b.WriteString(esc(s.text))
				continue
			}
			fmt.Fprintf(b, `<tspan class="%s">%s</tspan>`, s.class, esc(s.text))
		}
		b.WriteString("</text></g>\n")
	}
	b.WriteString("</svg>\n")
}

func hasClass(spans []span, class string) bool {
	for _, s := range spans {
		for _, c := range strings.Fields(s.class) {
			if c == class {
				return true
			}
		}
	}
	return false
}

func isCursor(spans []span) bool {
	return len(spans) > 0 && spans[len(spans)-1].class == "c"
}
