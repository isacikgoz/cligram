package cligram

// port is a border cell an edge may leave or arrive through.
type port struct {
	x, y int
	side Side
	cost int
}

// ports are the cells of node i's border an edge may use on its sides:
// the pinned side, or all four, the ones across the flow costing more.
// A stacked box's back border has the right and bottom sides.
func (r *router) portsOf(i int, pinned Side, leaving bool) []port {
	p := r.l.nodes[i]
	b := r.grid(p.rect)
	stacked := p.node.Sub != nil
	var out []port
	add := func(s Side, x0, y0, x1, y1 int) {
		if pinned != Auto && s != pinned {
			return
		}
		side := r.sideCost(s, leaving)
		mx, my := (x0+x1)/2, (y0+y1)/2
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				out = append(out, port{x, y, s, side + costOffset*(abs(x-mx)+abs(y-my))})
			}
		}
	}
	x1, y1 := b.X+b.W-1, b.Y+b.H-1
	if stacked {
		add(Top, b.X+1, b.Y, x1-2, b.Y)
		add(Left, b.X, b.Y+1, b.X, y1-2)
		add(Right, x1, b.Y+2, x1, y1-1)
		add(Bottom, b.X+2, y1, x1-1, y1)
	} else {
		add(Top, b.X+1, b.Y, x1-1, b.Y)
		add(Left, b.X, b.Y+1, b.X, y1-1)
		add(Right, x1, b.Y+1, x1, y1-1)
		add(Bottom, b.X+1, y1, x1-1, y1)
	}
	return out
}

// sideCost prefers leaving the way the flow reads and arriving from where
// it comes.
func (r *router) sideCost(s Side, leaving bool) int {
	want := Right
	if r.orient == TopToBottom {
		want = Bottom
	}
	if !leaving {
		want = map[Side]Side{Right: Left, Bottom: Top}[want]
	}
	turns := abs(sideDir(s) - sideDir(want))
	if turns == 3 {
		turns = 1
	}
	return costSide * turns
}
