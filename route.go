package cligram

import (
	"fmt"
	"maps"
	"slices"
	"sort"
)

// Edges are routed one at a time on the cell grid, around the boxes, with
// A*: a path pays for its length, more for each bend, more again for each
// line of another edge it crosses and for running alongside a box. Edges
// from the same node are one group and share their way out, so a
// decision's ways on leave on one trunk and branch off it. A label sits on
// a straight stretch of its own edge that no other edge shares.

// What a route pays.
const (
	costStep     = 2  // a cell of new line
	costReuse    = 1  // a cell of line the group already drew
	costBend     = 6  // turning a corner
	costJog      = 10 // turning again within jogRun cells of the last turn
	jogRun       = 3
	costCross    = 12 // crossing another group's line
	costHug      = 1  // a cell under or over a box
	costHugSide  = 4  // a cell along a box's side
	costBeside   = 3  // a cell beside another group's line, running with it
	costTouch    = 12 // a cell against a label's end
	costSide     = 3  // leaving or arriving across the flow, per quarter turn
	costOffset   = 3  // a cell off the middle of a side: more than the line it saves
	costSplit    = 12 // a second way out of a side for one group
	routeMargin  = 4  // room around the picture for edges to go round it
	searchSlack  = 8  // room around two boxes searched first
	labelPadding = 1  // line kept each side of a label
	labelLead    = 3  // line before a label, where there is room
)

// Directions of travel, indexing dx and dy.
const (
	goN = iota
	goE
	goS
	goW
)

var (
	dx = [4]int{0, 1, 0, -1}
	dy = [4]int{-1, 0, 1, 0}
)

func dirBit(d int) uint8 { return [4]uint8{north, east, south, west}[d] }

func opposite(d int) int { return (d + 2) % 4 }

// sideDir is the way out of a box through side s.
func sideDir(s Side) int {
	switch s {
	case Top:
		return goN
	case Right:
		return goE
	case Bottom:
		return goS
	}
	return goW
}

// route is an edge as drawn: a line from a box's border to the cell
// before the box it arrives at, and where its label sits.
type route struct {
	edge   Edge
	group  int
	label  string
	path   []Point // corners, from the border cell to the arrowhead's cell
	labelX int
	labelY int
	placed bool // the label found a place
}

type router struct {
	l      *Layout
	orient Orientation
	w, h   int // the grid: the picture and a margin all round
	// Per cell: the node whose box covers it, plus one; the lines through
	// it, the group that drew them and how many edges share it; whether a
	// label or an arrowhead keeps everything else out.
	box     []int
	lines   []uint8
	group   []int
	uses    []int
	blocked []bool
	label   []bool
	// sole are line cells only their own edge may run along; another
	// line may cross them, but not join or share them.
	sole []bool
	// At a crossing, the line passing the other way, and its group: a
	// trunk that passes a crossing goes on beyond it.
	xlines []uint8
	xgroup []int
	// unbroken makes crossings dearer still, for a route that needs a long
	// stretch for its label.
	unbroken bool
	// ports are the border cells edges leave through, by group.
	ports  map[int]int
	search search
}

func newRouter(l *Layout, o Orientation) *router {
	w, h := l.W+2*routeMargin, l.H+2*routeMargin
	r := &router{l: l, orient: o, w: w, h: h,
		box: make([]int, w*h), lines: make([]uint8, w*h), group: make([]int, w*h),
		uses: make([]int, w*h), blocked: make([]bool, w*h), label: make([]bool, w*h), sole: make([]bool, w*h),
		xlines: make([]uint8, w*h), xgroup: make([]int, w*h), ports: map[int]int{}}
	for i, p := range l.nodes {
		b := r.grid(p.rect)
		for y := b.Y; y < b.Y+b.H; y++ {
			for x := b.X; x < b.X+b.W; x++ {
				r.box[y*w+x] = i + 1
			}
		}
	}
	return r
}

// grid is a picture rect in grid cells.
func (r *router) grid(rc Rect) Rect {
	return Rect{rc.X + routeMargin, rc.Y + routeMargin, rc.W, rc.H}
}

func (r *router) in(x, y int) bool { return x >= 0 && y >= 0 && x < r.w && y < r.h }

// routeAll routes every edge, shortest first so the plain ones get the
// plain lines, and returns them in the order they were written.
// routeAll routes every edge, shortest first so the plain ones get the
// plain lines, and returns them in the order they were written. An edge
// that finds no way through is often only walled in by edges routed
// before it, so the edges that failed go first and everything is routed
// again, a few times, keeping the attempt that drew the most.
func routeAll(l *Layout, o Orientation, ell string, lim Limits) ([]route, []error) {
	groups := map[string]int{}
	for _, e := range l.edges {
		if _, ok := groups[e.From]; !ok {
			groups[e.From] = len(groups) + 1
		}
	}
	order := make([]int, len(l.edges))
	for i := range order {
		order[i] = i
	}
	length := func(i int) int {
		e := l.edges[i]
		a, b := l.nodes[l.index[e.From]].rect, l.nodes[l.index[e.To]].rect
		return abs(2*a.X+a.W-2*b.X-b.W) + abs(2*a.Y+a.H-2*b.Y-b.H)
	}
	sort.SliceStable(order, func(a, b int) bool { return length(order[a]) < length(order[b]) })

	var best []route
	var bestWarns []error
	bestFailed, bestLost := -1, 0
	for range routeAttempts {
		routes := make([]route, len(l.edges))
		for i, e := range l.edges {
			routes[i] = route{edge: e, group: groups[e.From], label: cut(e.Label, lim.LabelWidth, ell)}
		}
		failed, lost, warns := newRouter(l, o).routeIn(routes, order)
		improved := bestFailed < 0 || len(failed) < bestFailed
		if improved || (len(failed) == bestFailed && lost < bestLost) {
			best, bestWarns, bestFailed, bestLost = routes, warns, len(failed), lost
		}
		// Done when all are drawn, or when starting over drew no more.
		if len(failed) == 0 || !improved {
			break
		}
		// The failed first, the rest as they were.
		next := append([]int(nil), failed...)
		for _, i := range order {
			if !slices.Contains(failed, i) {
				next = append(next, i)
			}
		}
		order = next
	}
	return best, bestWarns
}

// routeAttempts is how many times routing may start over.
const routeAttempts = 4

// routeIn routes routes in order, and says which found no way through and
// how many labels found no room.
func (r *router) routeIn(routes []route, order []int) (failed []int, lost int, warns []error) {
	for _, i := range order {
		rt := &routes[i]
		cells, ok := r.find(rt.edge, rt.group)
		if !ok {
			failed = append(failed, i)
			warns = append(warns, fmt.Errorf("edge %s has no way through and is not drawn", rt.edge.describe()))
			continue
		}
		var before *snapshot
		if rt.label != "" {
			before = r.save()
		}
		r.commit(cells, rt.group)
		rt.path = r.corners(cells)
		if rt.label == "" {
			continue
		}
		// The label goes on now, so edges routed after keep clear of it:
		// one of the same group branches off before it, not under it.
		rt.labelX, rt.labelY, rt.placed = r.placeLabel(cells, rt.label)
		if !rt.placed {
			// Sharing a trunk can leave a branch too short for its label:
			// route the edge again on its own, and keep that if the label
			// fits there.
			// Failing that, avoid crossing lines too, which cut the
			// stretches a label needs.
			after := r.save()
			placed := false
			for _, unbroken := range []bool{false, true} {
				r.restore(before)
				r.unbroken = unbroken
				alone, ok := r.find(rt.edge, -1-i)
				r.unbroken = false
				if !ok {
					continue
				}
				r.commit(alone, rt.group)
				// It is no trunk: an edge routed later may cross it but
				// not join it, or its label would lead two ways.
				for _, c := range alone[1:] {
					r.sole[c] = true
				}
				if x, y, ok := r.placeLabel(alone, rt.label); ok {
					rt.path, rt.labelX, rt.labelY, rt.placed = r.corners(alone), x, y, true
					placed = true
					break
				}
			}
			if placed {
				continue
			}
			r.restore(after)
			lost++
			warns = append(warns, fmt.Errorf("edge %s has no room for its label", rt.edge.describe()))
		}
	}
	return failed, lost, warns
}

// snapshot is the router's grid as it was, to go back to.
type snapshot struct {
	lines, xlines        []uint8
	group, uses, xgroup  []int
	blocked, label, sole []bool
	ports                map[int]int
}

func (r *router) save() *snapshot {
	return &snapshot{
		lines: slices.Clone(r.lines), xlines: slices.Clone(r.xlines),
		group: slices.Clone(r.group), uses: slices.Clone(r.uses), xgroup: slices.Clone(r.xgroup),
		blocked: slices.Clone(r.blocked), label: slices.Clone(r.label), sole: slices.Clone(r.sole),
		ports: maps.Clone(r.ports),
	}
}

func (r *router) restore(s *snapshot) {
	copy(r.lines, s.lines)
	copy(r.xlines, s.xlines)
	copy(r.group, s.group)
	copy(r.uses, s.uses)
	copy(r.xgroup, s.xgroup)
	copy(r.blocked, s.blocked)
	copy(r.label, s.label)
	copy(r.sole, s.sole)
	r.ports = maps.Clone(s.ports)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// port is a border cell an edge may leave or arrive through.
type port struct {
	x, y int
	side Side
	cost int
}

// ports are the cells of node i's border an edge may use on its sides:
// the pinned side, or all four, the ones across the flow costing more.
// A stacked box's back frame has the right and bottom sides.
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
	heap     []item
}

type item struct{ f, g, s int32 }

// before orders the heap: cheapest estimate first and, of equals, the one
// that has come further, which keeps a grid search from spreading out
// across every tie.
func (a item) before(b item) bool { return a.f < b.f }

func (q *search) push(it item) {
	q.heap = append(q.heap, it)
	h := q.heap
	for i := len(h) - 1; i > 0; {
		p := (i - 1) / 2
		if !h[i].before(h[p]) {
			break
		}
		h[p], h[i] = h[i], h[p]
		i = p
	}
}

func (q *search) pop() item {
	h := q.heap
	top := h[0]
	last := len(h) - 1
	h[0] = h[last]
	q.heap = h[:last]
	h = q.heap
	for i := 0; ; {
		l, small := 2*i+1, i
		if l < len(h) && h[l].before(h[small]) {
			small = l
		}
		if l+1 < len(h) && h[l+1].before(h[small]) {
			small = l + 1
		}
		if small == i {
			break
		}
		h[i], h[small] = h[small], h[i]
		i = small
	}
	return top
}

func (r *router) cellOf(x, y int) int { return y*r.w + x }

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
func (r *router) findIn(e Edge, group int, w Rect) ([]int, bool) {
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
	q.heap = q.heap[:0]

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
	// The estimate is a cell of shared line for each step to the target:
	// never more than the way there costs.
	h := func(c int) int32 {
		x, y := c%r.w, c/r.w
		ddx := max(target.X-x, 0, x-(target.X+target.W-1))
		ddy := max(target.Y-y, 0, y-(target.Y+target.H-1))
		return int32(costReuse * max(ddx+ddy-1, 0))
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

	for len(q.heap) > 0 {
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
		crossing := r.xlines[c] != 0 || (r.lines[c] != 0 && (r.group[c] != group || split))
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
	if r.box[c] != 0 || r.blocked[c] {
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
			// beside a box is hard to tell from a junction or an
			// arrowhead's line.
			cost += costCross
			if r.nearBox(x, y) {
				cost += 3 * costCross // reads as a break in the line it crosses
			}
			if r.unbroken {
				cost += costCross
			}
			if r.unbroken {
				cost += 2 * costCross
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

// nearBox reports whether a box is within a cell of (x, y), corners too.
func (r *router) nearBox(x, y int) bool {
	for ny := y - 1; ny <= y+1; ny++ {
		for nx := x - 1; nx <= x+1; nx++ {
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

// commit marks a route's cells taken: its lines, its border cell as its
// group's way out, its arrowhead's cell as no one else's.
func (r *router) commit(cells []int, group int) {
	r.ports[cells[0]] = group
	for i := 1; i < len(cells); i++ {
		c := cells[i]
		m := r.toward(c, cells[i-1])
		if i < len(cells)-1 {
			m |= r.toward(c, cells[i+1])
		} else {
			r.blocked[c] = true
		}
		if r.lines[c] != 0 && (r.group[c] != group || crosses(r.lines[c], m)) {
			// A crossing: the line there keeps the cell, and the one
			// passing it is remembered.
			if crosses(r.lines[c], m) && r.xlines[c] == 0 {
				r.xlines[c], r.xgroup[c] = m, group
			}
			continue
		}
		r.lines[c] |= m
		r.group[c] = group
		r.uses[c]++
	}
}

// toward is the direction bit from cell a to its neighbor b.
func (r *router) toward(a, b int) uint8 {
	switch b - a {
	case -r.w:
		return north
	case 1:
		return east
	case r.w:
		return south
	}
	return west
}

// corners are the points where a route turns, with its two ends, in grid
// cells.
func (r *router) corners(cells []int) []Point {
	pt := func(c int) Point { return Point{c % r.w, c / r.w} }
	out := []Point{pt(cells[0])}
	for i := 1; i < len(cells)-1; i++ {
		if cells[i]-cells[i-1] != cells[i+1]-cells[i] {
			out = append(out, pt(cells[i]))
		}
	}
	return append(out, pt(cells[len(cells)-1]))
}

// placeLabel finds where a label sits on its route: on the straight
// stretch across nearest the target that only this edge uses, or else
// astride such a stretch down. It returns the label's first cell, in grid cells, and
// keeps the cells it covers for the label.
func (r *router) placeLabel(cells []int, label string) (int, int, bool) {
	g := r.l.glyphs
	lw := textWidth(g.LabelOpen) + textWidth(label) + textWidth(g.LabelClose)
	own := func(c int, m uint8) bool { return r.uses[c] == 1 && r.lines[c] == m && !r.blocked[c] }

	// Stretches of the route's own straight line, the ends left out: the
	// border cell and the arrowhead's.
	type stretch struct{ from, n int }
	runs := func(m uint8, step int) []stretch {
		var out []stretch
		inner := cells[1 : len(cells)-1]
		for i := 0; i < len(inner); {
			j := i
			for j < len(inner) && own(inner[j], m) && (j == i || abs(inner[j]-inner[j-1]) == step) {
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
	// The last stretch that fits, the one nearest the target: a stretch
	// near the source is where trunks are.
	last := func(ss []stretch, fits func(stretch) bool) (stretch, bool) {
		for i := len(ss) - 1; i >= 0; i-- {
			if fits(ss[i]) {
				return ss[i], true
			}
		}
		return stretch{}, false
	}

	if s, ok := last(runs(east|west, 1), func(s stretch) bool { return s.n >= lw+2*labelPadding }); ok {
		a, b := cells[s.from], cells[s.from+s.n-1]
		// Near the start of the stretch, where a trunk would branch, so the
		// branches keep their length and labels in a column line up.
		x := min(a%r.w, b%r.w) + min(labelLead, s.n-lw-labelPadding)
		y := a / r.w
		// A cell of line each side stays the edge's own, so another
		// edge's branch never butts against the label.
		r.keep(x, y, lw, labelPadding)
		return x, y, true
	}
	astride := func(s stretch) (int, int, bool) {
		mid := cells[s.from+s.n/2]
		x, y := mid%r.w-lw/2, mid/r.w
		// The label and a cell each side of it, clear of everything but
		// its own line.
		for k := -labelPadding; k < lw+labelPadding; k++ {
			if !r.in(x+k, y) {
				return 0, 0, false
			}
			c := r.cellOf(x+k, y)
			if c != mid && (r.box[c] != 0 || r.lines[c] != 0 || r.blocked[c]) {
				return 0, 0, false
			}
		}
		return x, y, true
	}
	if s, ok := last(runs(north|south, r.w), func(s stretch) bool {
		_, _, ok := astride(s)
		return s.n >= 1+2*labelPadding && ok
	}); ok {
		x, y, _ := astride(s)
		r.keep(x, y, lw, 0)
		return x, y, true
	}
	// Last, beside its own line where there is room: to the side of a
	// stretch down, or over or under one across. Off its line, a label is
	// known by what lies next to it, so it claims a ring a cell wide: no
	// line but its own may be in it, now or later, and its own stretch is
	// its edge's alone from now on.
	claim := func(x, y int, stretch []int, anchor int) bool {
		mine := map[int]bool{}
		for _, c := range stretch {
			mine[c] = true
		}
		var ring []int
		for yy := y - 1; yy <= y+1; yy++ {
			for xx := x - 2; xx < x+lw+2; xx++ {
				if !r.in(xx, yy) {
					return false
				}
				c := r.cellOf(xx, yy)
				onLabel := yy == y && xx >= x-labelPadding && xx < x+lw+labelPadding
				switch {
				case onLabel && (r.box[c] != 0 || r.lines[c] != 0 || r.blocked[c]):
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
		// From just before the label on to the target the line is its
		// edge's alone; before that, a sibling may share it and branch off.
		for _, c := range stretch[max(anchor-1, 0):] {
			r.sole[c] = true
		}
		return true
	}
	vertical, across := runs(north|south, r.w), runs(east|west, 1)
	for i := len(vertical) - 1; i >= 0; i-- {
		v := vertical[i]
		mid := cells[v.from+v.n/2]
		lx, ly := mid%r.w, mid/r.w
		for _, x := range []int{lx + 1 + labelPadding, lx - labelPadding - lw} {
			if claim(x, ly, cells[v.from:v.from+v.n], v.n/2) {
				return x, ly, true
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
			if claim(x, y, stretch, max(anchor, 0)) {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// keep keeps the cells of a label from everything else, and pad cells of
// its own line each side of it.
func (r *router) keep(x, y, w, pad int) {
	for k := -pad; k < w+pad; k++ {
		c := r.cellOf(x+k, y)
		r.blocked[c] = true
		r.label[c] = k >= 0 && k < w
	}
}
