package cligram

import "slices"

// placeLabel finds where a label sits on its route: on the straight
// stretch across nearest the target that only this edge uses, or else
// astride such a stretch down, or beside one. It returns the label's
// first cell, in grid cells, and keeps the cells it covers for the label.
// With corners, it may sit by its own line where it turns. With outside,
// it may reach off the grid, where no line goes: for a loop, whose label
// is by its own box; another edge's label there would bar the way round
// the picture to the edges routed after it.
func (r *router) placeLabel(cells []int, label string, corners, outside bool) (int, int, bool) {
	g := r.l.glyphs
	lb := labelling{r: r, cells: cells, corners: corners, outside: outside,
		w: textWidth(g.LabelOpen) + textWidth(label) + textWidth(g.LabelClose)}
	if x, y, ok := lb.along(); ok {
		return x, y, true
	}
	if x, y, ok := lb.across(); ok {
		return x, y, true
	}
	return lb.beside()
}

// labelling is a label being placed on a route, cells, w cells wide.
type labelling struct {
	r                *router
	cells            []int
	w                int
	corners, outside bool
}

// stretch is a straight run of a route's cells: from its index, n long.
type stretch struct{ from, n int }

// own reports whether cell c is the route's own line, running m, and
// nothing else's.
func (lb labelling) own(c int, m uint8) bool {
	r := lb.r
	return r.uses[c] == 1 && r.lines[c] == m && !r.blocked[c] && r.frame[c] == 0
}

// stretches are the stretches of the route's own straight line running m,
// cells step apart, the ends left out: the border cell and the
// arrowhead's.
func (lb labelling) stretches(m uint8, step int) []stretch {
	var out []stretch
	inner := lb.cells[1 : len(lb.cells)-1]
	for i := 0; i < len(inner); {
		j := i
		for j < len(inner) && lb.own(inner[j], m) && (j == i || abs(inner[j]-inner[j-1]) == step) {
			j++
		}
		if j > i {
			out = append(out, stretch{i + 1, j - i})
			i = j
		} else {
			i++
		}
	}
	return out
}

// lastFitting is the last stretch that fits, the one nearest the target:
// a stretch near the source is where trunks are.
func lastFitting(ss []stretch, fits func(stretch) bool) (stretch, bool) {
	for i := len(ss) - 1; i >= 0; i-- {
		if fits(ss[i]) {
			return ss[i], true
		}
	}
	return stretch{}, false
}

// along puts the label on a stretch across, in its line.
func (lb labelling) along() (int, int, bool) {
	r, cells, lw := lb.r, lb.cells, lb.w
	s, ok := lastFitting(lb.stretches(east|west, 1), func(s stretch) bool { return s.n >= lw+2*labelPadding })
	if !ok {
		return 0, 0, false
	}
	a, b := cells[s.from], cells[s.from+s.n-1]
	// Near the start of the stretch, where a trunk would branch, so the
	// branches keep their length and labels in a column line up.
	x := min(a%r.w, b%r.w) + min(labelLead, s.n-lw-labelPadding)
	y := a / r.w
	// A cell of line each side stays the edge's own, so another edge's
	// branch never butts against the label.
	r.keep(x, y, lw, labelPadding)
	return x, y, true
}

// across puts the label across a stretch down, astride its line.
func (lb labelling) across() (int, int, bool) {
	s, ok := lastFitting(lb.stretches(north|south, lb.r.w), func(s stretch) bool {
		_, _, ok := lb.astride(s)
		return s.n >= 1+2*labelPadding && ok
	})
	if !ok {
		return 0, 0, false
	}
	x, y, _ := lb.astride(s)
	lb.r.keep(x, y, lb.w, 0)
	return x, y, true
}

// astride is where a label across stretch s sits, on one of its rows: the
// middle one if it is clear, else the next clear one out from the middle.
func (lb labelling) astride(s stretch) (int, int, bool) {
	r, lw := lb.r, lb.w
rows:
	for _, i := range fromMiddle(s.n, labelPadding) {
		at := lb.cells[s.from+i]
		x, y := at%r.w-lw/2, at/r.w
		// The label and a cell each side of it, clear of everything but its
		// own line.
		for k := -labelPadding; k < lw+labelPadding; k++ {
			if !r.in(x+k, y) {
				if !lb.outside || r.off[Point{x + k, y}] {
					continue rows
				}
				continue
			}
			c := r.cellOf(x+k, y)
			if c != at && (r.box[c] != 0 || r.lines[c] != 0 || r.blocked[c]) || r.frame[c] != 0 {
				continue rows
			}
		}
		return x, y, true
	}
	return 0, 0, false
}

// beside puts the label beside its own line where there is room: to the
// side of a stretch down, or over or under one across.
func (lb labelling) beside() (int, int, bool) {
	r, cells, lw := lb.r, lb.cells, lb.w
	vertical, across := lb.stretches(north|south, r.w), lb.stretches(east|west, 1)
	for i := len(vertical) - 1; i >= 0; i-- {
		v := vertical[i]
		for _, k := range fromMiddle(v.n, 0) {
			at := cells[v.from+k]
			lx, ly := at%r.w, at/r.w
			for _, x := range []int{lx + 1 + labelPadding, lx - labelPadding - lw} {
				if lb.claim(x, ly, cells[v.from:v.from+v.n], k) {
					return x, ly, true
				}
			}
		}
	}
	for i := len(across) - 1; i >= 0; i-- {
		a := across[i]
		first, last := cells[a.from], cells[a.from+a.n-1]
		x := min(first%r.w, last%r.w) + min(labelLead, max(a.n-lw, 0))
		stretch := cells[a.from : a.from+a.n]
		anchor := slices.IndexFunc(stretch, func(c int) bool { return c%r.w >= x && c%r.w < x+lw })
		for _, y := range []int{first/r.w - 1, first/r.w + 1} {
			if lb.claim(x, y, stretch, max(anchor, 0)) {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// claim puts the label at (x, y), off its line. A label off its line is
// known by what lies next to it, so it claims a ring a cell wide: no line
// but its own may be in it, now or later, and its own stretch is its
// edge's alone from now on.
func (lb labelling) claim(x, y int, stretch []int, anchor int) bool {
	r, lw := lb.r, lb.w
	mine := map[int]bool{}
	for _, c := range stretch {
		mine[c] = true
	}
	// By a corner, its own line comes round next to the label: the same
	// edge either way. Not where another edge shares or crosses it.
	for _, c := range lb.cells {
		if lb.corners && r.uses[c] == 1 && r.xlines[c] == 0 {
			mine[c] = true
		}
	}
	var ring []int
	for yy := y - 1; yy <= y+1; yy++ {
		for xx := x - 2; xx < x+lw+2; xx++ {
			// Off the grid no line goes, only another label.
			if !r.in(xx, yy) {
				if !lb.outside || r.off[Point{xx, yy}] {
					return false
				}
				continue
			}
			c := r.cellOf(xx, yy)
			onLabel := yy == y && xx >= x-labelPadding && xx < x+lw+labelPadding
			switch {
			case onLabel && (r.box[c] != 0 || r.lines[c] != 0 || r.blocked[c] || r.frame[c] != 0):
				return false
			case r.lines[c] != 0 && !mine[c]:
				return false
			}
			if !onLabel && r.box[c] == 0 && r.lines[c] == 0 {
				ring = append(ring, c)
			}
		}
	}
	r.keep(x, y, lw, labelPadding)
	for _, c := range ring {
		r.blocked[c] = true
	}
	// From just before the label on to the target the line is its edge's
	// alone; before that, a sibling may share it and branch off.
	for _, c := range stretch[max(anchor-1, 0):] {
		r.sole[c] = true
	}
	return true
}

// fromMiddle lists the places along a stretch of n cells, keeping margin
// cells clear at each end, from its middle outward: n/2, then one before,
// one after, and so on.
func fromMiddle(n, margin int) []int {
	var out []int
	add := func(i int) {
		if i >= margin && i < n-margin {
			out = append(out, i)
		}
	}
	mid := n / 2
	add(mid)
	for d := 1; d <= mid; d++ {
		add(mid - d)
		add(mid + d)
	}
	return out
}

// keep keeps the cells of a label from everything else, and pad cells of
// its own line each side of it.
func (r *router) keep(x, y, w, pad int) {
	for k := -pad; k < w+pad; k++ {
		if !r.in(x+k, y) {
			r.off[Point{x + k, y}] = true
			continue
		}
		c := r.cellOf(x+k, y)
		r.blocked[c] = true
		r.label[c] = k >= 0 && k < w
	}
}
