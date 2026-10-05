// Package text measures, wraps and cuts text by the cells a terminal
// draws it in: what every kind of diagram does to its words.
package text

import (
	"strings"

	"github.com/rivo/uniseg"
)

// Width is how many cells s takes.
func Width(s string) int { return uniseg.StringWidth(s) }

// Wrap breaks s into lines of at most width cells, at spaces where it can
// and inside a word only when the word alone is too wide. A "\n" in s
// always breaks. More than maxLines lines are cut, the last of them
// ending in ell.
func Wrap(s string, width, maxLines int, ell string) []string {
	var lines []string
	for _, hard := range strings.Split(s, "\n") {
		lines = append(lines, wrapLine(hard, width)...)
	}
	if len(lines) <= maxLines {
		return lines
	}
	lines = lines[:maxLines]
	lines[maxLines-1] = Ellipsize(lines[maxLines-1], width, ell)
	return lines
}

func wrapLine(s string, width int) []string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "" && Width(w) <= width:
			line = w
		case line != "" && Width(line)+1+Width(w) <= width:
			line += " " + w
		default:
			if line != "" {
				lines = append(lines, line)
			}
			// A word too wide for a line of its own is broken where lines end.
			for Width(w) > width {
				var head string
				head, w = SplitAt(w, width)
				lines = append(lines, head)
			}
			line = w
		}
	}
	return append(lines, line)
}

// SplitAt splits s after the graphemes that fit in width cells, taking at
// least one so that splitting always moves on.
func SplitAt(s string, width int) (string, string) {
	used := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		from, to := g.Positions()
		if used+g.Width() > width {
			if from == 0 {
				return s[:to], s[to:]
			}
			return s[:from], s[from:]
		}
		used += g.Width()
	}
	return s, ""
}

// Cut shortens s to width cells, ending in ell when anything was cut.
func Cut(s string, width int, ell string) string {
	if Width(s) <= width {
		return s
	}
	return Ellipsize(s, width, ell)
}

// Ellipsize ends s in ell, within width cells, to say there is more.
func Ellipsize(s string, width int, ell string) string {
	head, _ := SplitAt(s, width-Width(ell))
	return strings.TrimRight(head, " ") + ell
}

// LongestWord is how many cells the widest word in s takes.
func LongestWord(s string) int {
	w := 0
	for _, word := range strings.Fields(s) {
		w = max(w, Width(word))
	}
	return w
}
