package cligram

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
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
	ports map[int]int
	// frame marks the cells of frames' borders: a line crosses a border
	// straight, never runs along it or turns on it, and never touches a
	// corner or a title.
	frame []uint8
	// drawn are the groups that have drawn a line, kept through restores:
	// a group that never drew has no line of its own to share.
	drawn map[int]bool
	// off are the cells of labels off the grid, where no line goes but a
	// loop's label may.
	off    map[Point]bool
	search *search
	// before and after are the grid as it was, kept to go back to: a
	// label's retries need two, and reuse their room.
	before, after snapshot
	// portBuf is room for portsOf's ports, reused.
	portBuf []port
}

// routers keeps routers between routings: Fit routes a diagram many
// times, each on a grid of its own, and grids and search tables made
// afresh each time cost more than the searches do. A router reused is
// cleared; its search tables hold entries of searches before, which their
// generation stamps tell apart.
var routers = sync.Pool{New: func() any { return &router{search: new(search)} }}

// cleared is s, n long and all zero, reusing s's room.
func cleared[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	s = s[:n]
	clear(s)
	return s
}

// emptied is m, empty, reusing it.
func emptied[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return map[K]V{}
	}
	clear(m)
	return m
}

func newRouter(l *Layout, o Orientation) *router {
	w, h := l.W+2*routeMargin, l.H+2*routeMargin
	n := w * h
	r := routers.Get().(*router)
	r.l, r.orient, r.w, r.h, r.unbroken = l, o, w, h, false
	r.box, r.lines, r.group, r.uses = cleared(r.box, n), cleared(r.lines, n), cleared(r.group, n), cleared(r.uses, n)
	r.blocked, r.label, r.sole = cleared(r.blocked, n), cleared(r.label, n), cleared(r.sole, n)
	r.xlines, r.xgroup, r.frame = cleared(r.xlines, n), cleared(r.xgroup, n), cleared(r.frame, n)
	r.ports, r.off, r.drawn = emptied(r.ports), emptied(r.off), emptied(r.drawn)
	r.search.tables(n)
	for i, p := range l.nodes {
		b := r.grid(p.rect)
		for y := b.Y; y < b.Y+b.H; y++ {
			for x := b.X; x < b.X+b.W; x++ {
				r.box[y*w+x] = i + 1
			}
		}
	}
	for _, f := range l.frames {
		b := r.grid(f.rect)
		mark := func(x, y int, m uint8) {
			if r.in(x, y) {
				r.frame[r.cellOf(x, y)] |= m
			}
		}
		for x := b.X; x < b.X+b.W; x++ {
			mark(x, b.Y, frameAcross)
			mark(x, b.Y+b.H-1, frameAcross)
		}
		for y := b.Y; y < b.Y+b.H; y++ {
			mark(b.X, y, frameDown)
			mark(b.X+b.W-1, y, frameDown)
		}
		// Its title, the spaces round it and a dash after, in the top
		// border: the title reads ╭╌ Title ╌.
		for x := b.X + 2; x < b.X+5+textWidth(l.frameTitle(f.group)); x++ {
			mark(x, b.Y, frameFixed)
		}
		// Its corners and the cells beside them, so a corner always reads
		// as one.
		x0, y0, x1, y1 := b.X, b.Y, b.X+b.W-1, b.Y+b.H-1
		for _, c := range []Point{
			{x0, y0}, {x0 + 1, y0}, {x0, y0 + 1}, {x1, y0}, {x1 - 1, y0}, {x1, y0 + 1},
			{x0, y1}, {x0 + 1, y1}, {x0, y1 - 1}, {x1, y1}, {x1 - 1, y1}, {x1, y1 - 1},
		} {
			mark(c.X, c.Y, frameFixed)
		}
	}
	return r
}

// Frame border cells: across the top or bottom, down a side, or fixed: a
// corner or the title.
const (
	frameAcross uint8 = 1 << iota
	frameDown
	frameFixed
)

// crossesFrame reports whether a line may enter frame cell c heading d:
// across a border, never along one, never onto a corner or a title.
func (r *router) crossesFrame(c, d int) bool {
	switch m := r.frame[c]; {
	case m == 0:
		return true
	case m&frameFixed != 0 || m == frameAcross|frameDown:
		return false
	case m == frameAcross:
		return d == goN || d == goS
	}
	return d == goE || d == goW
}

// grid is a picture rect in grid cells.
func (r *router) grid(rc Rect) Rect {
	return Rect{rc.X + routeMargin, rc.Y + routeMargin, rc.W, rc.H}
}

func (r *router) in(x, y int) bool { return x >= 0 && y >= 0 && x < r.w && y < r.h }

func (r *router) cellOf(x, y int) int { return y*r.w + x }

// routeAll routes every edge, shortest first so the plain ones get the
// plain lines, and returns them in the order they were written.
// routeAll routes every edge, shortest first so the plain ones get the
// plain lines, and returns them in the order they were written. An edge
// that finds no way through is often only walled in by edges routed
// before it, so the edges that failed go first and everything is routed
// again, a few times, keeping the attempt that drew the most.
func routeAll(l *Layout, o Orientation, ell string, lim Limits) ([]route, []error) {
	// Edges from one node share a trunk, those of one line style only: a
	// dashed way on shared with a solid one would show no dashes.
	type key struct {
		from string
		line LineStyle
	}
	groups := map[key]int{}
	for _, e := range l.edges {
		if _, ok := groups[key{e.From, e.Line}]; !ok {
			groups[key{e.From, e.Line}] = len(groups) + 1
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
	// A labelled loop goes round a corner of its box, which takes sides
	// the flow would rather have: it goes last, where there is room left.
	last := func(i int) bool { return loop(l.edges[i]) && l.edges[i].Label != "" }
	sort.SliceStable(order, func(a, b int) bool {
		if la, lb := last(order[a]), last(order[b]); la != lb {
			return lb
		}
		return length(order[a]) < length(order[b])
	})

	var best []route
	var bestWarns []error
	bestFailed, bestLost := -1, 0
	for range routeAttempts {
		routes := make([]route, len(l.edges))
		for i, e := range l.edges {
			routes[i] = route{edge: e, group: groups[key{e.From, e.Line}], label: cut(e.Label, lim.LabelWidth, ell)}
		}
		r := newRouter(l, o)
		failed, lost, warns := r.routeIn(routes, order)
		r.done()
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
			before = r.save(&r.before)
		}
		r.commit(cells, rt.group)
		rt.path = r.corners(cells)
		if rt.label == "" {
			continue
		}
		// The label goes on now, so edges routed after keep clear of it:
		// one of the same group branches off before it, not under it.
		// A loop's shortest way is a hook with its arrowhead beside where
		// it leaves: its label goes on a loop round a corner instead.
		if !loop(rt.edge) {
			rt.labelX, rt.labelY, rt.placed = r.placeLabel(cells, rt.label, false, false)
		}
		if !rt.placed {
			// Sharing a trunk can leave a branch too short for its label:
			// route the edge again on its own, and keep that if the label
			// fits there.
			// Failing that, avoid crossing lines too, which cut the
			// stretches a label needs.
			after := r.save(&r.after)
			placed := false
			for _, try := range retries(rt.edge) {
				r.restore(before)
				r.unbroken = try.unbroken
				alone, ok := r.find(try.edge, -1-i)
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
				if x, y, ok := r.placeLabel(alone, rt.label, try.corners, rt.edge.From == rt.edge.To); ok {
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

// retry is a way to route an edge again, on its own, for its label.
type retry struct {
	edge     Edge
	unbroken bool
	// corners lets the label sit by its own line where it turns.
	corners bool
}

// retries are the ways to route e again when its label found no room:
// as it was, then without crossing lines, then with the label by a corner
// of its own line, which reads less plainly. The shortest loop from a
// node back to itself is a hook too short for a label, so a loop first
// goes round a corner of its box, out one side and in the next, where one
// leg is long enough to carry its label beside it; a loop's label is
// always by its corners.
func retries(e Edge) []retry {
	if !loop(e) {
		return []retry{{e, false, false}, {e, true, false}, {e, false, true}, {e, true, true}}
	}
	again := []retry{{e, false, true}, {e, true, true}}
	var out []retry
	for _, sides := range [][2]Side{
		{Right, Bottom}, {Bottom, Right}, {Right, Top}, {Top, Right},
		{Left, Bottom}, {Bottom, Left}, {Left, Top}, {Top, Left},
	} {
		round := e
		round.FromSide, round.ToSide = sides[0], sides[1]
		out = append(out, retry{round, false, true})
	}
	return append(out, again...)
}

// loop reports whether e goes from a node back to itself by whichever
// sides the router finds best.
func loop(e Edge) bool {
	return e.From == e.To && e.FromSide == Auto && e.ToSide == Auto
}

// snapshot is the router's grid as it was, to go back to.
type snapshot struct {
	lines, xlines        []uint8
	group, uses, xgroup  []int
	blocked, label, sole []bool
	ports                map[int]int
	off                  map[Point]bool
}

// save keeps the grid as it is in s, reusing s's room, and gives s.
func (r *router) save(s *snapshot) *snapshot {
	s.lines, s.xlines = append(s.lines[:0], r.lines...), append(s.xlines[:0], r.xlines...)
	s.group, s.uses, s.xgroup = append(s.group[:0], r.group...), append(s.uses[:0], r.uses...), append(s.xgroup[:0], r.xgroup...)
	s.blocked, s.label, s.sole = append(s.blocked[:0], r.blocked...), append(s.label[:0], r.label...), append(s.sole[:0], r.sole...)
	s.ports, s.off = refill(s.ports, r.ports), refill(s.off, r.off)
	return s
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
	r.ports, r.off = refill(r.ports, s.ports), refill(r.off, s.off)
}

// refill makes dst hold what src does, reusing dst.
func refill[K comparable, V any](dst, src map[K]V) map[K]V {
	if dst == nil {
		return maps.Clone(src)
	}
	clear(dst)
	maps.Copy(dst, src)
	return dst
}

// done gives the router back, for the next routing to reuse.
func (r *router) done() {
	r.l = nil
	routers.Put(r)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// commit marks a route's cells taken: its lines, its border cell as its
// group's way out, its arrowhead's cell as no one else's.
func (r *router) commit(cells []int, group int) {
	r.ports[cells[0]] = group
	r.drawn[group] = true
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
