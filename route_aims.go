package cligram

import "sort"

// aim is where along one side of a box a link would rather leave or
// arrive: x on the top or bottom, y on the left or right, in grid cells.
type aim struct {
	side Side
	at   int
	ok   bool
}

// facing is the side of box a that faces box b.
func facing(a, b Rect) Side {
	switch {
	case b.Y >= a.Y+a.H:
		return Bottom
	case b.Y+b.H <= a.Y:
		return Top
	case b.X >= a.X+a.W:
		return Right
	}
	return Left
}

// aimLinks spreads each box's links along the side facing their other
// ends, in the order those ends lie: a fan of links out of a box, which
// share no line, then leaves it without crossing itself. Edges with a
// way share a trunk and need none.
func (r *router) aimLinks() {
	r.aims = emptied(r.aims)
	type end struct {
		ref   EdgeRef
		leave bool
		key   int // where the other end lies along the side
	}
	bySide := map[[2]int][]end{} // node, side
	for _, e := range r.l.edges {
		if !e.linked() {
			continue
		}
		from, to := r.l.index[e.From], r.l.index[e.To]
		for _, leave := range []bool{true, false} {
			i, j := from, to
			if !leave {
				i, j = to, from
			}
			a, b := r.grid(r.l.nodes[i].rect), r.grid(r.l.nodes[j].rect)
			s := facing(a, b)
			key := 2*b.X + b.W
			if s == Left || s == Right {
				key = 2*b.Y + b.H
			}
			k := [2]int{i, int(s)}
			bySide[k] = append(bySide[k], end{e.Ref(), leave, key})
		}
	}
	for k, ends := range bySide {
		sort.SliceStable(ends, func(a, b int) bool { return ends[a].key < ends[b].key })
		box, s := r.grid(r.l.nodes[k[0]].rect), Side(k[1])
		start, length := box.X+1, box.W-2
		if s == Left || s == Right {
			start, length = box.Y+1, box.H-2
		}
		for n, e := range ends {
			at := start + (n+1)*length/(len(ends)+1)
			pair := r.aims[e.ref]
			if e.leave {
				pair[0] = aim{s, at, true}
			} else {
				pair[1] = aim{s, at, true}
			}
			r.aims[e.ref] = pair
		}
	}
}
