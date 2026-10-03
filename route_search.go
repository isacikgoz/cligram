package cligram

import (
	"sync/atomic"
)

// The search runs over states: a cell, the way the line came into it,
// and how many cells it has come straight, up to jogRun. They are numbered
// densely, and the search's tables are kept between edges and cleared by
// stamping a new generation, so routing allocates almost nothing.
const runs = jogRun + 1

// A state also knows whether the line has split off its group's trunk.
// Before, it may run along and turn on the group's lines; after, it may
// only cross them, as it would any other line, so two branches of one
// node never join again and each stays readable on its own.
func (r *router) state(cell, dir, run int, split bool) int {
	b := 0
	if split {
		b = 1
	}
	return ((cell*4+dir)*runs+run)*2 + b
}

func (r *router) unstate(s int) (cell, dir, run int, split bool) {
	split = s%2 == 1
	s /= 2
	run = s % runs
	s /= runs
	return s / 4, s % 4, run, split
}

type search struct {
	gen      uint32
	stamp    []uint32 // states seen this generation
	best     []int32  // their cheapest cost so far
	prev     []int32  // the state before, or -1 for a first step
	start    []int32  // a first step's border cell
	goalGen  []uint32 // per cell and way in: a goal this generation
	goalCost []int32
	landing  []uint32 // per cell: a goal for some way in, this generation
	buckets  [][]item // open states by estimate
	lo, hi   int      // the lowest and highest bucket in use
	open     int      // states waiting
}

type item struct{ f, g, s int32 }

// The open states wait in buckets by their estimate, costs being small
// whole numbers: the cheapest bucket first and, in it, the state put in
// last, which has come furthest, so a grid search does not spread out
// across every tie.
func (q *search) push(it item) {
	f := int(it.f)
	for f >= len(q.buckets) {
		q.buckets = append(q.buckets, nil)
	}
	q.buckets[f] = append(q.buckets[f], it)
	q.lo = min(q.lo, f) // an estimate need not rise along a path
	q.hi = max(q.hi, f)
	q.open++
}

func (q *search) pop() item {
	for len(q.buckets[q.lo]) == 0 {
		q.lo++
	}
	b := q.buckets[q.lo]
	it := b[len(b)-1]
	q.buckets[q.lo] = b[:len(b)-1]
	q.open--
	return it
}

// reset empties the buckets for a new search.
func (q *search) reset() {
	for f := 0; f <= q.hi && f < len(q.buckets); f++ {
		q.buckets[f] = q.buckets[f][:0]
	}
	q.lo, q.hi, q.open = 1<<30, 0, 0
}

// find searches for the cheapest way from e's source to its target, and
// returns its cells: the border cell it leaves through, then every cell
// to the one its arrowhead goes in. It looks near the two boxes first,
// where nearly every edge is, and over the whole grid only if it must.
func (r *router) find(e Edge, group int) (cells []int, ok bool) {
	a := r.grid(r.l.nodes[r.l.index[e.From]].rect)
	b := r.grid(r.l.nodes[r.l.index[e.To]].rect)
	x0, y0 := min(a.X, b.X)-searchSlack, min(a.Y, b.Y)-searchSlack
	x1, y1 := max(a.X+a.W, b.X+b.W)+searchSlack, max(a.Y+a.H, b.Y+b.H)+searchSlack
	cells, ok = r.findIn(e, group, Rect{x0, y0, x1 - x0, y1 - y0})
	if !ok {
		cells, ok = r.findIn(e, group, Rect{0, 0, r.w, r.h})
	}
	// A path that comes back over itself would draw a junction or a loop
	// out of one line: keep it off the cells it repeated, and search again.
	var kept []int
	defer func() {
		for _, c := range kept {
			r.blocked[c] = false
		}
	}()
	for range 3 {
		if !ok {
			return nil, false
		}
		again := repeated(cells)
		if len(again) == 0 {
			return cells, true
		}
		for _, c := range again {
			if !r.blocked[c] {
				r.blocked[c] = true
				kept = append(kept, c)
			}
		}
		cells, ok = r.findIn(e, group, Rect{0, 0, r.w, r.h})
	}
	if ok && len(repeated(cells)) == 0 {
		return cells, true
	}
	return nil, false
}

// repeated are the cells a path visits more than once.
func repeated(cells []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, c := range cells {
		if seen[c] {
			out = append(out, c)
		}
		seen[c] = true
	}
	return out
}

// findIn is find within the window w.
// searched counts the states every search took off its queue: the work
// layout does, for a test to keep from creeping up.
var searched atomic.Int64

func (r *router) findIn(e Edge, group int, w Rect) ([]int, bool) {
	steps := 0
	defer func() { searched.Add(int64(steps)) }()
	from, to := r.l.index[e.From], r.l.index[e.To]
	target := r.grid(r.l.nodes[to].rect)
	inWindow := func(x, y int) bool {
		return r.in(x, y) && x >= w.X && y >= w.Y && x < w.X+w.W && y < w.Y+w.H
	}
	q := &r.search
	if q.stamp == nil {
		n := r.w * r.h * 4 * runs * 2
		q.stamp, q.best, q.prev, q.start = make([]uint32, n), make([]int32, n), make([]int32, n), make([]int32, n)
		q.goalGen, q.goalCost = make([]uint32, r.w*r.h*4), make([]int32, r.w*r.h*4)
		q.landing = make([]uint32, r.w*r.h)
	}
	q.gen++
	q.reset()

	// Arrive in a free cell beside the target, heading in.
	goals := 0
	for _, p := range r.portsOf(to, e.ToSide, false) {
		in := opposite(sideDir(p.side))
		gx, gy := p.x-dx[in], p.y-dy[in]
		if !r.in(gx, gy) {
			continue
		}
		c := r.cellOf(gx, gy)
		if r.box[c] != 0 || r.blocked[c] || r.lines[c] != 0 {
			continue
		}
		q.goalGen[c*4+in], q.goalCost[c*4+in] = q.gen, int32(p.cost)
		q.landing[c] = q.gen
		goals++
	}
	if goals == 0 {
		return nil, false
	}
	goalCost := func(c, d int) (int32, bool) {
		if q.goalGen[c*4+d] != q.gen {
			return 0, false
		}
		return q.goalCost[c*4+d], true
	}
	// The estimate is never more than the way there costs: a cell of new
	// line for each step to the target, or of shared line if the group
	// has drawn any, and a corner if the target is not straight ahead.
	step := costStep
	if r.drawn[group] {
		step = costReuse
	}
	h := func(c int) int32 {
		x, y := c%r.w, c/r.w
		ddx := max(target.X-x, 0, x-(target.X+target.W-1))
		ddy := max(target.Y-y, 0, y-(target.Y+target.H-1))
		est := step * max(ddx+ddy-1, 0)
		if ddx > 0 && ddy > 0 {
			est += costBend
		}
		return int32(est)
	}
	visit := func(s int, g int32, prev int, start int) {
		if q.stamp[s] == q.gen && q.best[s] <= g {
			return
		}
		q.stamp[s], q.best[s], q.prev[s], q.start[s] = q.gen, g, int32(prev), int32(start)
		c, _, _, _ := r.unstate(s)
		q.push(item{g + h(c), g, int32(s)})
	}

	// A group leaves through one cell where it can, so its edges share a
	// trunk.
	shared := false
	for _, p := range r.portsOf(from, Auto, true) {
		if r.ports[r.cellOf(p.x, p.y)] == group {
			shared = true
		}
	}
	for _, p := range r.portsOf(from, e.FromSide, true) {
		pc := r.cellOf(p.x, p.y)
		owner, used := r.ports[pc]
		if used && owner != group {
			continue
		}
		if shared && !used {
			p.cost += costSplit
		}
		out := sideDir(p.side)
		nx, ny := p.x+dx[out], p.y+dy[out]
		if !r.in(nx, ny) {
			continue
		}
		nc := r.cellOf(nx, ny)
		cost, split, ok := r.enter(pc, nc, out, group, false)
		if !ok {
			continue
		}
		gc, _ := goalCost(nc, out)
		visit(r.state(nc, out, jogRun, split), int32(p.cost+cost)+gc, -1, pc)
	}

	for q.open > 0 {
		steps++
		it := q.pop()
		s := int(it.s)
		if it.g > q.best[s] {
			continue
		}
		c, dir, run, split := r.unstate(s)
		if _, ok := goalCost(c, dir); ok {
			return r.cells(s), true
		}
		x, y := c%r.w, c/r.w
		// A line crosses another group's straight through, never turns on it.
		crossing := r.xlines[c] != 0 || (r.lines[c] != 0 && (r.group[c] != group || split)) || r.frame[c] != 0
		for d := range 4 {
			if d == opposite(dir) || (crossing && d != dir) {
				continue
			}
			nx, ny := x+dx[d], y+dy[d]
			if !inWindow(nx, ny) {
				continue
			}
			nc := r.cellOf(nx, ny)
			// A cell beside the target is for landing in, not passing by.
			if _, isGoal := goalCost(nc, d); q.landing[nc] == q.gen && !isGoal {
				continue
			}
			cost, nsplit, ok := r.enter(c, nc, d, group, split)
			if !ok {
				continue
			}
			nrun := min(run+1, jogRun)
			if d != dir {
				cost += costBend
				if run < jogRun {
					cost += costJog
				}
				nrun = 1
			}
			gc, _ := goalCost(nc, d)
			visit(r.state(nc, d, nrun, nsplit), it.g+int32(cost)+gc, s, -1)
		}
	}
	return nil, false
}

// enter is what moving into cell c heading d costs a line of group that
// has split off its trunk or not, whether it has split after, and whether
// it may enter at all.
func (r *router) enter(prev, c, d, group int, split bool) (int, bool, bool) {
	if r.box[c] != 0 || r.blocked[c] || !r.crossesFrame(c, d) {
		return 0, false, false
	}
	x, y := c%r.w, c/r.w
	cost := costStep
	along := dirBit(d) | dirBit(opposite(d))
	if xm := r.xlines[c]; xm != 0 {
		// Two lines cross here already: a line may only go on along one of
		// its own, on its trunk.
		owner := r.group[c]
		if xm&along != 0 {
			owner = r.xgroup[c]
		}
		if owner != group || split || !r.linked(prev, c, d, group) {
			return 0, false, false
		}
		return costReuse, false, true
	}
	if m := r.lines[c]; m != 0 {
		switch {
		case r.sole[c] && m != (north|south|east|west)&^along:
			return 0, false, false
		case r.group[c] == group && !split && m != (north|south|east|west)&^along:
			// Still on the trunk, only where it already leads: stepping onto
			// a branch from beside it would join two branches into a loop.
			if !r.linked(prev, c, d, group) {
				return 0, false, false
			}
			cost = costReuse
		case m == (north|south|east|west)&^along:
			// A crossing, of another group's line or a sibling branch. One
			// near a box is hard to tell from a junction, or breaks the
			// line just before its arrowhead.
			cost += costCross
			if r.nearBox(x, y, 2) {
				cost += 3 * costCross // reads as a break in the line it crosses
			}
			if r.unbroken {
				cost += 3 * costCross
			}
			split = true
		default:
			return 0, false, false
		}
	} else {
		split = true
	}
	// Beside a box: under or over one is how lines cross a gap between
	// rows, but along its side, a line reads as a second border. Squeezed
	// between two boxes, it pays for both.
	for _, k := range []int{(d + 1) % 4, (d + 3) % 4} {
		nx, ny := x+dx[k], y+dy[k]
		if r.in(nx, ny) && r.box[r.cellOf(nx, ny)] != 0 {
			if d == goN || d == goS {
				cost += costHugSide
			} else {
				cost += costHug
			}
		}
	}
	// Right beside a frame's border, along it, a line reads as part of
	// the frame.
	for _, k := range []int{(d + 1) % 4, (d + 3) % 4} {
		nx, ny := x+dx[k], y+dy[k]
		if r.in(nx, ny) && r.frame[r.cellOf(nx, ny)] != 0 {
			cost += costHugSide
			break
		}
	}
	// A line butting against a label's end reads as part of it.
	for _, k := range []int{goE, goW} {
		if nx := x + dx[k]; r.in(nx, y) && r.label[r.cellOf(nx, y)] {
			cost += costTouch
			break
		}
	}
	// Running right beside another line, even a sibling branch, reads as
	// one double line. Running along its own trunk is not beside it.
	for _, k := range []int{(d + 1) % 4, (d + 3) % 4} {
		nx, ny := x+dx[k], y+dy[k]
		if !r.in(nx, ny) {
			continue
		}
		if n := r.cellOf(nx, ny); r.lines[n]&along != 0 && (r.group[n] != group || split || r.lines[c] == 0) {
			cost += costBeside
			break
		}
	}
	return cost, split, true
}

// linked reports whether group's line already leads from prev into c,
// heading d: prev is the group's way out of its box, or its line there
// leads on that way, and c's leads back.
func (r *router) linked(prev, c, d, group int) bool {
	back := dirBit(opposite(d))
	into := r.lines[c]
	if r.group[c] != group {
		into = r.xlines[c]
	}
	if into&back == 0 {
		return false
	}
	if owner, ok := r.ports[prev]; ok && owner == group {
		return true
	}
	from := r.lines[prev]
	if r.group[prev] != group {
		from = 0
		if r.xgroup[prev] == group {
			from = r.xlines[prev]
		}
	}
	return from&dirBit(d) != 0
}

// nearBox reports whether a box is within reach cells of (x, y), corners
// too.
func (r *router) nearBox(x, y, reach int) bool {
	for ny := y - reach; ny <= y+reach; ny++ {
		for nx := x - reach; nx <= x+reach; nx++ {
			if r.in(nx, ny) && r.box[r.cellOf(nx, ny)] != 0 {
				return true
			}
		}
	}
	return false
}

// cells walks back from the goal to the border cell the route left from.
func (r *router) cells(goal int) []int {
	var rev []int
	for s := goal; ; {
		c, _, _, _ := r.unstate(s)
		rev = append(rev, c)
		p := int(r.search.prev[s])
		if p < 0 {
			rev = append(rev, int(r.search.start[s]))
			break
		}
		s = p
	}
	out := make([]int, len(rev))
	for i, c := range rev {
		out[len(rev)-1-i] = c
	}
	return out
}
