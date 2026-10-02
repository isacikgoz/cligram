package cligram

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
	// alone do not fit is never routed unless it is the best there is. A
	// way fits only if every label found a place too.
	var best *Layout
	var bestCfg layoutConfig
	better := func(l *Layout) bool {
		if best == nil {
			return true
		}
		lw, bw := l.W <= room.X, best.W <= room.X
		switch {
		case lw != bw:
			return lw
		case l.lost() != best.lost():
			return l.lost() < best.lost()
		case lw:
			return l.H < best.H
		}
		return l.W < best.W
	}
	for _, c := range tries {
		l := d.place(c)
		if l.W <= room.X && l.H <= room.Y {
			l.route(c)
			if l.W <= room.X && l.H <= room.Y && l.lost() == 0 {
				l.fits = true
				return l
			}
		}
		if better(l) {
			best, bestCfg = l, c
		}
	}
	if best.routes == nil && len(best.edges) > 0 {
		best = d.place(bestCfg)
		best.route(bestCfg)
	}
	return best
}

// lost is how many labels found no place; before routing, none have.
func (l *Layout) lost() int {
	n := 0
	for _, rt := range l.routes {
		if rt.label != "" && !rt.placed {
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
