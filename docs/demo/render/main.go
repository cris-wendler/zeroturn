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
	fontSize   = 14.0
	charWidth  = 8.45 // close to 0.6em, the advance width of common monospace fonts
	lineHeight = 20.0
	padX       = 20.0
	padTop     = 52.0
	padBottom  = 20.0

	idleBeforeTyping = 350  // ms with the prompt and cursor shown
	typeBase         = 14   // ms per character, before jitter
	typeSpace        = 45   // ms for a space, which reads as a word boundary
	afterTyping      = 320  // ms between the last character and the output
	outputLine       = 45   // ms between ordinary output lines
	outputResult     = 260  // ms before a PASS or FAIL line, which reflects work
	holdScreen       = 2100 // ms a finished screen stays before it clears
)

type span struct {
	text  string
	class string
}

type item struct {
	command bool
	spans   []span
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
	var screens [][]item
	var cur []item
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "---":
			screens = append(screens, cur)
			cur = nil
		case strings.HasPrefix(line, "$ "):
			cur = append(cur, item{command: true, spans: []span{{"$ ", "p"}, {line[2:], "w"}}})
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
	return screens, sc.Err()
}

// colorize keeps ANSI colors when present and otherwise applies the colors
// the CLI itself uses on a terminal.
func colorize(line string) []span {
	if strings.Contains(line, "\x1b[") {
		return fromANSI(line)
	}
	switch {
	case strings.HasPrefix(line, "PASS"):
		return []span{{"PASS", "g"}, {line[4:], ""}}
	case strings.HasPrefix(line, "FAIL"):
		return []span{{"FAIL", "r"}, {line[4:], ""}}
	case line == "READY TO SHIP":
		return []span{{line, "g b"}}
	case strings.HasPrefix(line, "ZEROTURN "):
		return []span{{line, "b"}}
	case strings.HasPrefix(line, "Result: ") && !strings.Contains(line, "failed"):
		return []span{{line, "g"}}
	case strings.Contains(line, `"ask"`):
		i := strings.Index(line, `"ask"`)
		return []span{{line[:i], ""}, {`"ask"`, "y"}, {line[i+5:], ""}}
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

func width(spans []span) int {
	n := 0
	for _, s := range spans {
		n += len([]rune(s.text))
	}
	return n
}

// jitter varies typing speed deterministically so the recording looks
// typed by a person and still renders identically every time.
func jitter(i int) int { return (i * 7919 % 7) * 4 }

func timeline(screens [][]item) ([]element, int, int, int) {
	var els []element
	t, cols, rows := 0, 0, 0
	for _, screen := range screens {
		start := len(els)
		row := 0
		for _, it := range screen {
			if w := width(it.spans); w+2 > cols {
				cols = w + 2
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
			cmd := []rune(it.spans[1].text)
			els = append(els, element{row: row, spans: withCursor(""), from: t, to: t + idleBeforeTyping})
			t += idleBeforeTyping
			for i := 1; i <= len(cmd); i++ {
				d := typeBase + jitter(i)
				if cmd[i-1] == ' ' {
					d = typeSpace
				}
				next := t + d
				if i == len(cmd) {
					next = t + afterTyping
				}
				els = append(els, element{row: row, spans: withCursor(string(cmd[:i])), from: t, to: next})
				t = next
			}
			els = append(els, element{row: row, spans: it.spans, from: t})
			row++
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

func withCursor(typed string) []span {
	return []span{{"$ ", "p"}, {typed, "w"}, {"█", "c"}}
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
	b.WriteString(`<desc id="desc">A terminal types four commands. The ZeroTurn status line shows context at 82 percent, five hour usage at 81 percent, seven day usage at 47 percent, a session of 3 hours 12 minutes, 2 active subagents, and the word ask. The subagent gate then returns an ask decision with the reason: New subagent requires approval. Context is 82% and five hour usage is 81%. zeroturn verify passes two checks, and zeroturn ship with dry run passes the same checks and prints READY TO SHIP. The session values are sample data.</desc>` + "\n")
	b.WriteString("<style>\n")
	fmt.Fprintf(b, `text{font-family:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,"Liberation Mono",monospace;font-size:%.0fpx;fill:#c9d1d9;white-space:pre}`+"\n", fontSize)
	fmt.Fprintf(b, ".a{opacity:0;animation-duration:%dms;animation-iteration-count:infinite;animation-timing-function:step-end}\n", total)
	b.WriteString(".p{fill:#3fb950}.w{fill:#e6edf3}.c{fill:#8b949e}.g{fill:#3fb950}.y{fill:#d29922}.r{fill:#f85149}.m{fill:#8b949e}.b{font-weight:700}\n")
	b.WriteString(".t{font-size:12px;fill:#8b949e}\n")
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
		fmt.Fprintf(b, `<text class="%s" style="animation-name:k%d" x="%.0f" y="%.0f">`, cls, i, padX, padTop+float64(e.row)*lineHeight+fontSize)
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
		b.WriteString("</text>\n")
	}
	b.WriteString("</svg>\n")
}

func isCursor(spans []span) bool {
	return len(spans) > 0 && spans[len(spans)-1].class == "c"
}
