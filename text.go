package cligram

import (
	"strings"

	"github.com/rivo/uniseg"
)

// Limits keep text within what a terminal can show: a node's text wraps
// at NodeWidth cells and stops after NodeLines lines, and an edge's label
// is one line of at most LabelWidth cells. Whatever does not fit is cut
// with an ellipsis, so one long text can never make the picture
// unreadable; a host shows the whole text elsewhere, for the node in
// focus.
type Limits struct {
	NodeWidth, NodeLines, LabelWidth int
}

// DefaultLimits fit a dozen boxes across a wide terminal.
var DefaultLimits = Limits{NodeWidth: 24, NodeLines: 3, LabelWidth: 20}

// The narrowest limits honored: room for a letter and the ellipsis.
const minTextWidth = 4

func (lim Limits) sane() Limits {
	if lim.NodeWidth < minTextWidth {
		lim.NodeWidth = minTextWidth
	}
	if lim.NodeLines < 1 {
		lim.NodeLines = 1
	}
	if lim.LabelWidth < minTextWidth {
		lim.LabelWidth = minTextWidth
	}
	return lim
}

// wrap breaks s into lines of at most width cells, at spaces where it can
// and inside a word only when the word alone is too wide. A "\n" in s
// always breaks. More than maxLines lines are cut, the last of them
// ending in ell.
func wrap(s string, width, maxLines int, ell string) []string {
	var lines []string
	for _, hard := range strings.Split(s, "\n") {
		lines = append(lines, wrapLine(hard, width)...)
	}
	if len(lines) <= maxLines {
		return lines
	}
	lines = lines[:maxLines]
	lines[maxLines-1] = ellipsize(lines[maxLines-1], width, ell)
	return lines
}

func wrapLine(s string, width int) []string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "" && textWidth(w) <= width:
			line = w
		case line != "" && textWidth(line)+1+textWidth(w) <= width:
			line += " " + w
		default:
			if line != "" {
				lines = append(lines, line)
			}
			// A word too wide for a line of its own is broken where lines end.
			for textWidth(w) > width {
				var head string
				head, w = splitAt(w, width)
				lines = append(lines, head)
			}
			line = w
		}
	}
	return append(lines, line)
}

// splitAt splits s after the graphemes that fit in width cells, taking at
// least one so that splitting always moves on.
func splitAt(s string, width int) (string, string) {
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

// cut shortens s to width cells, ending in ell when anything was cut.
func cut(s string, width int, ell string) string {
	if textWidth(s) <= width {
		return s
	}
	return ellipsize(s, width, ell)
}

// ellipsize ends s in ell, within width cells, to say there is more.
func ellipsize(s string, width int, ell string) string {
	head, _ := splitAt(s, width-textWidth(ell))
	return strings.TrimRight(head, " ") + ell
}

// longestWord is how many cells the widest word in s takes.
func longestWord(s string) int {
	w := 0
	for _, word := range strings.Fields(s) {
		w = max(w, textWidth(word))
	}
	return w
}
