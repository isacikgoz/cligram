package cligram

import "slices"

// stack puts each further way on below what the way before it led to,
// but only below what reaches into the columns its own way leads through
// (rows, top to bottom): a long way on that turns far off does not push
// the next one down past all of it. Where the flow goes along the main
// axis is settled first; it does not depend on how siblings stack.
//
// Edges between the two ways run in the gap between them, so it keeps a
// lane for each, and room for its label.
func (y *layouter) stack() {
	if len(y.stacks) == 0 {
		return
	}
	y.stacked = map[[2]int]bool{}
	y.restack(y.peek(y.main))
}

// restack puts each stacked way below what shares its columns at pos,
// along the main axis, where it is not below it already, and reports
// whether it did. Solving moves nodes, so what shares columns is checked
// again once solved.
func (y *layouter) restack(pos []int) bool {
	gap := y.gap(Gap{}, y.main)
	added := false
	for _, st := range y.stacks {
		lo, hi := 1<<30, -1<<30
		for _, n := range st.sub {
			lo, hi = min(lo, pos[n]), max(hi, pos[n]+y.size(n, y.main))
		}
		across := st.across
		if across == 0 {
			across = y.gap(Gap{}, y.cross) + y.lanes(st.prev, st.sub)
		}
		for _, t := range st.prev {
			if y.stacked[[2]int{t, st.c}] || pos[t] >= hi+gap || pos[t]+y.size(t, y.main)+gap <= lo {
				continue
			}
			y.stacked[[2]int{t, st.c}] = true
			y.sys[y.cross].floor(t, st.c, y.size(t, y.cross)+across, st.g, prioAuto)
			added = true
		}
	}
	return added
}

// lanes is the room across the flow that the edges between nodes in a
// and nodes in b take: a line and a cell beside it each, or its label's
// room.
func (y *layouter) lanes(a, b []int) int {
	in := func(s []int, id string) bool { return slices.Contains(s, y.l.index[id]) }
	room := 0
	for _, e := range y.l.edges {
		between := in(a, e.From) && in(b, e.To) || in(b, e.From) && in(a, e.To)
		if e.From == e.To || !between {
			continue
		}
		if e.Label != "" {
			room += y.labelGap(e.Label, y.cross)
		} else {
			room += 2
		}
	}
	return room
}

// auto places what the author left unplaced by following the edges from
// each root in written order: a step's first way on goes beside it,
// centered on it, and each further one beside it too, stacked below
// everything the one before it led to.
func (y *layouter) auto() {
	n := len(y.l.nodes)
	visited := make([]bool, n)
	var order []int
	kids := make([][]int, n)
	for _, e := range y.l.edges {
		from, to := y.l.index[e.From], y.l.index[e.To]
		if from != to && !slices.Contains(kids[from], to) {
			kids[from] = append(kids[from], to)
		}
	}

	var visit func(int)
	visit = func(i int) {
		visited[i] = true
		order = append(order, i)
		var fresh []int
		for _, c := range kids[i] {
			if !visited[c] {
				fresh = append(fresh, c)
			}
		}
		// Siblings line up, so they all keep the gap the widest label needs.
		gap := y.gap(Gap{}, y.main)
		for _, c := range fresh {
			gap = max(gap, y.labelRoom(i, c, y.main))
		}
		var prev []int
		for _, c := range fresh {
			if visited[c] {
				continue // reached through an earlier sibling
			}
			g := y.group()
			y.dir[c], y.band[c], y.parent[c] = y.dir[i], y.band[i], i
			if y.breaks[c] && y.free(c, y.main) && y.free(c, y.cross) {
				// Wrapped: the flow turns, as a snake does, into a new band
				// below everything before it in its columns, starting under
				// the step it comes from and reading back the other way.
				y.dir[c], y.band[c] = -y.dir[i], c
				across := max(y.gap(Gap{}, y.cross), y.labelRoom(i, c, y.cross))
				if y.dir[c] < 0 {
					y.sys[y.main].equal(i, c, y.size(i, y.main)-y.size(c, y.main), g, prioAuto)
				} else {
					y.sys[y.main].equal(i, c, 0, g, prioAuto)
				}
				wrapped := len(y.stacks)
				y.stacks = append(y.stacks, stack{c: c, prev: slices.Clone(order), g: g, across: across})
				start := len(order)
				visit(c)
				prev = append([]int(nil), order[start:]...)
				y.stacks[wrapped].sub = prev
				continue
			}
			if y.free(c, y.main) {
				if y.dir[c] > 0 {
					y.sys[y.main].floor(i, c, y.size(i, y.main)+gap, g, prioAuto)
				} else {
					y.sys[y.main].floor(c, i, y.size(c, y.main)+gap, g, prioAuto)
					y.backward[c][y.main] = true
				}
				y.beside[c] = true
			}
			stacked := -1
			if y.free(c, y.cross) {
				if prev == nil {
					y.center(c, y.cross, []int{i}, g)
				} else {
					stacked = len(y.stacks)
					y.stacks = append(y.stacks, stack{c: c, prev: prev, g: g})
				}
			}
			start := len(order)
			visit(c)
			prev = append([]int(nil), order[start:]...)
			if stacked >= 0 {
				y.stacks[stacked].sub = prev
			}
		}
	}

	for i := range n {
		if visited[i] {
			continue
		}
		y.dir[i], y.band[i], y.parent[i] = 1, i, -1
		// A further root goes below everything before it.
		if len(order) > 0 && y.free(i, y.cross) {
			g := y.group()
			for _, t := range order {
				y.sys[y.cross].floor(t, i, y.size(t, y.cross)+y.gap(Gap{}, y.cross), g, prioAuto)
			}
		}
		visit(i)
	}
	y.order = order
}

// overflow is the node to wrap the flow at, or -1: the first, in the
// order the flow is followed, whose band reaches past limit at it, along
// the way the flow reads, and that was put there by following an edge.
// Of several bands that reach too far, the latest goes first: a band too
// long for the room pushes the one before it, which then only seems too
// long itself.
func (y *layouter) overflow(limit int) int {
	at, last := -1, -1 // the node to wrap at, and where its band starts in the order
	where := map[int]int{}
	for k, i := range y.order {
		where[i] = k
	}
	for _, i := range y.order {
		// A step right after a band's start wraps into a band of one: a
		// column, which reading the other way draws better.
		if !y.beside[i] || y.breaks[i] || y.parent[i] == y.band[i] {
			continue
		}
		b := y.band[i]
		span := y.pos(i, y.main) + y.size(i, y.main) - y.pos(b, y.main)
		if y.dir[i] < 0 {
			span = y.pos(b, y.main) + y.size(b, y.main) - y.pos(i, y.main)
		}
		if span > limit && where[b] > last {
			at, last = i, where[b]
		}
	}
	return at
}

// free reports whether nothing the author wrote decides node i on ax.
func (y *layouter) free(i int, ax axis) bool { return !y.bound[i][ax] }
