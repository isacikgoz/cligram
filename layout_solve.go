package cligram

import (
	"fmt"
	"maps"
	"slices"
)

// solve settles both axes, then keeps nodes from sitting on each other:
// two that overlap are moved apart along the axis they overlap least on,
// the way they already lean, and everything is solved again.
func (y *layouter) solve() {
	n := len(y.l.nodes)
	apart := map[[2]int]bool{} // pairs already kept apart, or given up on
	for range 4 * n * n {
		var pos [2][]int
		for ax := range 2 {
			pos[ax] = y.solveAxis(axis(ax))
		}
		for i := range y.l.nodes {
			r := &y.l.nodes[i].rect
			r.X, r.Y = pos[axisX][i]+y.pad[i], pos[axisY][i]+y.pad[i]
		}
		// What a stacked way shares columns with may have changed; and
		// frames are round their nodes as they are now, nothing else in.
		moved := y.restack(pos[y.main])
		y.frameRects()
		moved = y.outsiders() || moved
		for a := range n {
			for b := a + 1; b < n; b++ {
				ra, rb := y.box(a), y.box(b)
				// Boxes keep a cell apart, for the lines and arrowheads
				// between them, unless a placement asked them to touch.
				if apart[[2]int{a, b}] || !overlaps(grow(ra), rb) || y.touching[[2]int{a, b}] {
					continue
				}
				apart[[2]int{a, b}] = true
				moved = y.separate(a, b) || moved
			}
		}
		if !moved {
			break
		}
	}
	y.normalize()
}

// peek is where the nodes would go on ax as things stand, leaving the
// system as it was: what it would drop is dropped and warned of when it
// is solved for real.
func (y *layouter) peek(ax axis) []int {
	s := y.sys[ax]
	was := maps.Clone(s.dropped)
	pos, _ := s.solve()
	s.dropped = was
	return pos
}

// solveAxis solves one axis, raising the soft bounds until they settle.
func (y *layouter) solveAxis(ax axis) []int {
	s := y.sys[ax]
	var pos []int
	for range 16 {
		var dropped []int
		pos, dropped = s.solve()
		y.warnDropped(dropped)
		changed := false
		for _, sf := range y.softs {
			if sf.ax != ax || s.dropped[s.cons[sf.cons].group] {
				continue
			}
			if v := y.softValue(sf, pos); v != s.cons[sf.cons].w {
				s.cons[sf.cons].w = v
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	y.pull(ax, pos)
	return pos
}

// pull moves each node placed before its targets on ax (left of them,
// above them) up to them. The smallest positions keep everything else as
// close as it can be, but leave such a node at the start of the axis; it
// goes as far towards its targets as every constraint on it allows, and
// nothing else moves.
func (y *layouter) pull(ax axis, pos []int) {
	s := y.sys[ax]
	for changed := true; changed; {
		changed = false
		for i := range y.l.nodes {
			if !y.backward[i][ax] {
				continue
			}
			upper, bounded := 0, false
			for _, c := range s.cons {
				if c.from != i || s.dropped[c.group] {
					continue
				}
				if v := pos[c.to] - c.w; !bounded || v < upper {
					upper, bounded = v, true
				}
			}
			if bounded && upper > pos[i] {
				pos[i] = upper
				changed = true
			}
		}
	}
}

func (y *layouter) softValue(sf soft, pos []int) int {
	lo, hi := pos[sf.targets[0]], 0
	for _, t := range sf.targets {
		lo = min(lo, pos[t])
		hi = max(hi, pos[t]+y.size(t, sf.ax))
	}
	size := y.size(sf.node, sf.ax)
	switch sf.rel {
	case TopLevel, LeftLevel:
		return lo
	case BottomLevel, RightLevel:
		return hi - size
	}
	return lo + floorDiv(hi-lo-size, 2)
}

// grow is r with a cell more all round.
func grow(r Rect) Rect { return Rect{r.X - 1, r.Y - 1, r.W + 2, r.H + 2} }

func overlaps(a, b Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

// separate moves a and b apart, trying the axis they overlap least on
// first, and reports whether it could.
func (y *layouter) separate(a, b int) bool {
	ra, rb := y.box(a), y.box(b)
	over := [2]int{
		min(ra.X+ra.W, rb.X+rb.W) - max(ra.X, rb.X),
		min(ra.Y+ra.H, rb.Y+rb.H) - max(ra.Y, rb.Y),
	}
	axes := []axis{y.cross, y.main}
	if over[y.main] < over[y.cross] {
		axes = []axis{y.main, y.cross}
	}
	for _, ax := range axes {
		before, after := a, b
		ca := 2*y.pos(a, ax) + y.size(a, ax)
		cb := 2*y.pos(b, ax) + y.size(b, ax)
		if cb < ca {
			before, after = b, a
		}
		g := y.group()
		s := y.sys[ax]
		s.floor(before, after, y.size(before, ax)+gapCells(Gap{Size: GapTight}, ax), g, prioOverlap)
		_, dropped := s.solve()
		if slices.Contains(dropped, g) {
			continue // this way apart conflicts with what was written
		}
		y.warnDropped(dropped)
		return true
	}
	y.l.warnings = append(y.l.warnings, fmt.Errorf("nodes %q and %q overlap: their placements leave no way apart",
		y.l.nodes[a].node.ID, y.l.nodes[b].node.ID))
	return false
}

// warnDropped warns of the placements the author wrote that were dropped.
func (y *layouter) warnDropped(groups []int) {
	for _, g := range groups {
		if why, ok := y.why[g]; ok {
			y.l.warnings = append(y.l.warnings, fmt.Errorf("%s cannot hold with the other placements and is left out", why))
		}
	}
}

// pos is where node i's room starts on ax.
func (y *layouter) pos(i int, ax axis) int {
	if ax == axisX {
		return y.l.nodes[i].rect.X - y.pad[i]
	}
	return y.l.nodes[i].rect.Y - y.pad[i]
}

// normalize moves the picture to the top left corner and sizes it.
func (y *layouter) normalize() {
	l := y.l
	if len(l.nodes) == 0 {
		return
	}
	l.W, l.H = 0, 0
	y.frameRects()
	minX, minY := l.nodes[0].rect.X, l.nodes[0].rect.Y
	for _, p := range l.nodes {
		minX, minY = min(minX, p.rect.X), min(minY, p.rect.Y)
	}
	for _, f := range l.frames {
		minX, minY = min(minX, f.rect.X), min(minY, f.rect.Y)
	}
	for i := range l.nodes {
		r := &l.nodes[i].rect
		r.X, r.Y = r.X-minX, r.Y-minY
		l.W, l.H = max(l.W, r.X+r.W), max(l.H, r.Y+r.H)
	}
	for i := range l.frames {
		r := &l.frames[i].rect
		r.X, r.Y = r.X-minX, r.Y-minY
		l.W, l.H = max(l.W, r.X+r.W), max(l.H, r.Y+r.H)
	}
}
