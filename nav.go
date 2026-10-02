package cligram

// Moving the focus works two ways. Move goes where the eye goes: to the
// nearest box in a direction as the layout drew it, so it changes when a
// resize redraws the picture. Out and In follow the flow itself, the same
// at any size.

// Move is the node nearest the node id in direction dir, as drawn: among
// the boxes within 45 degrees of that way, the closest, a cell of offset to
// the side counting twice a cell straight on; or, if none is that close to
// the line, the closest anywhere that way. A row is about twice as tall as
// a column is wide, so distances down count double. Ties go to the node
// written first. With no node that way, or no node id, it is id.
func (l *Layout) Move(id string, dir Side) string {
	from, ok := l.Rect(id)
	if !ok || dir == Auto {
		return id
	}
	// Centers, doubled to stay whole, and rows doubled again for their height.
	cx, cy := 2*from.X+from.W, 2*(2*from.Y+from.H)
	best, bestScore, bestCone := id, 0, false
	for _, p := range l.nodes {
		if p.node.ID == id {
			continue
		}
		r := p.rect
		dx, dy := 2*r.X+r.W-cx, 2*(2*r.Y+r.H)-cy
		ahead, side := 0, 0
		switch dir {
		case Right:
			ahead, side = dx, dy
		case Left:
			ahead, side = -dx, dy
		case Bottom:
			ahead, side = dy, dx
		case Top:
			ahead, side = -dy, dx
		}
		if ahead <= 0 {
			continue
		}
		side = abs(side)
		cone := side <= ahead
		score := ahead + 2*side
		if best == id || (cone && !bestCone) || (cone == bestCone && score < bestScore) {
			best, bestScore, bestCone = p.node.ID, score, cone
		}
	}
	return best
}

// Out are the edges leaving the node id, in the order they were written:
// its ways on.
func (d *Diagram) Out(id string) []Edge {
	var out []Edge
	for _, e := range d.edges {
		if e.From == id {
			out = append(out, e)
		}
	}
	return out
}

// In are the edges arriving at the node id, in the order they were
// written: the ways it is reached.
func (d *Diagram) In(id string) []Edge {
	var in []Edge
	for _, e := range d.edges {
		if e.To == id {
			in = append(in, e)
		}
	}
	return in
}
