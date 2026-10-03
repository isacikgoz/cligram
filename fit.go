package cligram

import "sort"

// A terminal is only so big. Fit lays a diagram out to fit it, trying the
// cheapest changes first and changing the shape of the flow before cutting
// any of its words; what still does not fit is shown through a view that
// moves to keep a node in sight.

// Fit lays the diagram out to fit w columns by h rows if any way of laying
// it out does: as it is; with the gaps nothing asked for closed up; with
// the flow wrapped, like text, onto a new band where it reaches the edge;
// read the other way, top to bottom or left to right; then with its text
// wrapped narrower, and narrower still. If none fits, it takes the one
// that fits the width in the fewest rows, since scrolling down reads
// better than scrolling across, or failing that the narrowest; Fits says
// which happened, and Reveal and RenderView show it a window at a time.
func Fit(w, h int) LayoutOption {
	return func(c *layoutConfig) { c.fit = &Point{max(w, 1), max(h, 1)} }
}

// wrapSlack is the room wrapping leaves at the edge for lines.
const wrapSlack = 3

// The narrower text Fit tries, in turn, when nothing else fits.
var narrower = []Limits{
	{NodeWidth: 16, NodeLines: 4, LabelWidth: 14},
	{NodeWidth: 12, NodeLines: 5, LabelWidth: 12},
}

// Fits reports whether the layout fits the room Fit was given; without
// Fit, it always does.
func (l *Layout) Fits() bool { return l.fits }

// Orientation is the way the layout reads, which Fit may have turned.
func (l *Layout) Orientation() Orientation { return l.orient }

func (d *Diagram) fitted(cfg layoutConfig) *Layout {
	room := *cfg.fit
	other := TopToBottom
	if cfg.orient == TopToBottom {
		other = LeftToRight
	}
	tries := []layoutConfig{}
	try := func(o Orientation, compact, wrap bool, lim Limits) {
		if o != cfg.orient && cfg.keepOrient {
			return // the reader chose which way it reads
		}
		c := cfg
		c.orient, c.compact = o, compact
		if wrap {
			// Short of the edge, so lines going round a band still fit.
			c.wrap = room.X - wrapSlack
			if o == TopToBottom {
				c.wrap = room.Y - wrapSlack
			}
		}
		c.limits = Limits{
			NodeWidth:  min(cfg.limits.NodeWidth, lim.NodeWidth),
			NodeLines:  max(cfg.limits.NodeLines, lim.NodeLines),
			LabelWidth: min(cfg.limits.LabelWidth, lim.LabelWidth),
		}.sane()
		tries = append(tries, c)
	}
	as := cfg.limits
	try(cfg.orient, false, false, as)
	try(cfg.orient, true, false, as)
	try(cfg.orient, true, true, as)
	try(other, false, false, as)
	try(other, true, false, as)
	try(other, true, true, as)
	for _, lim := range narrower {
		try(cfg.orient, true, true, lim)
		try(other, true, true, lim)
		try(other, true, false, lim)
	}

	// Placing the boxes is cheap and routing is not, so a way whose boxes
	// alone do not fit is not routed while another might fit. A way fits
	// only if it drew every edge and every label too.
	type candidate struct {
		l   *Layout
		cfg layoutConfig
	}
	var all []candidate
	nearly := 0 // ways that fit and drew every edge, but lost a label
	for _, c := range tries {
		l := d.place(c)
		if l.W <= room.X && l.H <= room.Y {
			// Routed as it is: giving nodes more room is for the way that
			// is kept in the end, not for every way tried.
			l.route(c)
			if l.W <= room.X && l.H <= room.Y && l.lost() == 0 {
				l.fits = true
				return l
			}
			// It fits but lost a label: room round the label's edge, if
			// that still fits, keeps it.
			if l.W <= room.X && l.H <= room.Y && l.lost() < 1000 {
				if r := d.placeAndRoute(c); r.W <= room.X && r.H <= room.Y && r.lost() == 0 {
					r.fits = true
					return r
				}
			}
			if l.lost() < 1000 {
				nearly++
			}
		}
		all = append(all, candidate{l, c})
		// A label lost in every way tried is likely lost in all: after a
		// few, take the best of them rather than route every way there is.
		if nearly >= fallbackTries {
			break
		}
	}
	// Nothing fits: take the one that draws the most, then keeps the width
	// (scrolling down reads better than across), then loses the fewest
	// labels, then is smallest. They are routed most promising first, by
	// the size of their boxes, until one draws everything.
	sort.SliceStable(all, func(i, j int) bool { return betterFallback(all[i].l, all[j].l, room) })
	// Of the ways routed already, the best; if it draws every edge, the
	// rest are not worth routing to compare labels.
	var best *Layout
	var bestCfg layoutConfig
	for _, c := range all {
		if c.l.routes != nil || len(c.l.edges) == 0 {
			if best == nil || betterFallback(c.l, best, room) {
				best, bestCfg = c.l, c.cfg
			}
		}
	}
	if best == nil || best.lost() >= 1000 {
		for _, c := range all {
			if c.l.routes != nil || len(c.l.edges) == 0 {
				continue
			}
			l := d.place(c.cfg)
			l.route(c.cfg)
			if best == nil || betterFallback(l, best, room) {
				best, bestCfg = l, c.cfg
			}
			if best.lost() < 1000 {
				break
			}
		}
	}
	// An edge walled in, in the best there is: give its nodes room.
	if best.lost() >= 1000 {
		if roomier := d.placeAndRoute(bestCfg); roomier.lost() < best.lost() {
			best = roomier
		}
	}
	return best
}

// fallbackTries is how many ways that draw every edge but lose a label
// are routed before the best of them is taken.
const fallbackTries = 4

// betterFallback reports whether a is a better layout than b for room
// when neither fits it.
func betterFallback(a, b *Layout, room Point) bool {
	undrawn := func(l *Layout) int { return l.lost() / 1000 }
	aw, bw := a.W <= room.X, b.W <= room.X
	switch {
	case undrawn(a) != undrawn(b):
		return undrawn(a) < undrawn(b)
	case aw != bw:
		return aw
	case a.lost() != b.lost():
		return a.lost() < b.lost()
	case aw:
		return a.H < b.H
	}
	return a.W < b.W
}

// lost is what a layout failed to draw: each edge with no way through
// counts as worse than any number of lost labels.
func (l *Layout) lost() int {
	n := 0
	for _, rt := range l.routes {
		switch {
		case rt.path == nil:
			n += 1000
		case rt.label != "" && !rt.placed:
			n++
		}
	}
	return n
}

// Reveal moves view, a window on the picture, the least it must for the
// node with id to show whole, with a cell to spare where there is room. The
// view keeps its size and stays on the picture; a picture smaller than the
// view sits at its top left.
func (l *Layout) Reveal(view Rect, id string) Rect {
	if r, ok := l.Rect(id); ok {
		view.X = reveal(view.X, view.W, r.X, r.W)
		view.Y = reveal(view.Y, view.H, r.Y, r.H)
	}
	view.X = max(min(view.X, l.W-view.W), 0)
	view.Y = max(min(view.Y, l.H-view.H), 0)
	return view
}

// reveal moves a span at pos of length n the least it must to hold the
// span at at of length m, with a cell to spare each side if it fits.
func reveal(pos, n, at, m int) int {
	spare := 0
	if m+2 <= n {
		spare = 1
	}
	lo, hi := at-spare, at+m+spare
	switch {
	case m > n:
		return at // too big to show whole: show where it starts
	case lo < pos:
		return lo
	case hi > pos+n:
		return hi - n
	}
	return pos
}

// RenderView paints the cells of the picture in view, as Render would.
func (l *Layout) RenderView(st State, t Theme, view Rect) string {
	return l.paint(st).renderRect(t, view)
}
