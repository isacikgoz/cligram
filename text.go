package cligram

import "github.com/isacikgoz/cligram/internal/text"

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

// The text helpers, in the names the package has always used.
var (
	wrap        = text.Wrap
	cut         = text.Cut
	longestWord = text.LongestWord
)
