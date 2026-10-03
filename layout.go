package cligram

import (
	"fmt"
	"maps"
	"slices"
)

// Orientation is the way a flow reads: each step's ways on go that way
// from it, and steps that share a parent stack across it.
type Orientation int

const (
	LeftToRight Orientation = iota
	TopToBottom
)

// LayoutOption sets how a diagram is laid out.
type LayoutOption func(*layoutConfig)

type layoutConfig struct {
	glyphs *Glyphs
	orient Orientation
	// keepOrient keeps Fit from turning the flow.
	keepOrient bool
	limits     Limits
	// compact closes the gaps nothing asked to be wider down to tight.
	compact bool
	// fit, when set, is the room there is.
	fit *Point
	// wrap, when set, is how far the flow may read before it wraps.
	wrap int
	// wordWidth is the widest a word may make a line: the width the text
	// was given before any narrowing.
	wordWidth int
	// pad is room kept clear round a node, by id, for edges that could
	// not find a way to it without.
	pad map[string]int
}

// WithGlyphs draws with g instead of Unicode.
func WithGlyphs(g *Glyphs) LayoutOption {
	return func(c *layoutConfig) { c.glyphs = g }
}

// WithLimits keeps text within lim instead of DefaultLimits.
func WithLimits(lim Limits) LayoutOption {
	return func(c *layoutConfig) { c.limits = lim }
}

// WithOrientation lays the flow out reading o; the default is LeftToRight.
func WithOrientation(o Orientation) LayoutOption {
	return func(c *layoutConfig) { c.orient = o }
}

// KeepOrientation keeps Fit from turning the flow the other way, for a
// reader who chose which way it reads: it compacts, wraps and narrows
// text only.
func KeepOrientation() LayoutOption {
	return func(c *layoutConfig) { c.keepOrient = true }
}

// Layout is a diagram with every node given a place. It is computed once
// for a diagram and a size, and painted as often as the state changes:
// painting never moves a box.
type Layout struct {
	W, H     int
	glyphs   *Glyphs
	limits   Limits
	orient   Orientation
	fits     bool
	nodes    []placed
	index    map[string]int
	edges    []Edge
	routes   []route
	warnings []error
	frames   []frame // the groups, drawn round their nodes
}

type placed struct {
	node  Node
	lines []string
	rect  Rect
}

// Rect is where the node with this id is drawn.
func (l *Layout) Rect(id string) (Rect, bool) {
	i, ok := l.index[id]
	if !ok {
		return Rect{}, false
	}
	return l.nodes[i].rect, true
}

// Warnings are the diagram's mistakes, and the placements that could not
// hold and were left out. The layout is whole either way.
func (l *Layout) Warnings() []error { return l.warnings }

// State is where a run is: each node's status, the edges it took, and the
// node in focus.
type State struct {
	Status map[string]Status
	Taken  []EdgeRef
	Focus  string
}

// Render paints the layout in state st, colored by t.
func (l *Layout) Render(st State, t Theme) string {
	return l.paint(st).render(t)
}

// paint paints the picture in state st.
func (l *Layout) paint(st State) *canvas {
	c := newCanvas(l.W, l.H, l.glyphs)
	for _, f := range l.frames {
		c.groupFrame(f.rect, l.frameTitle(f.group))
	}
	for _, p := range l.nodes {
		c.box(p.rect, p.node.Sub != nil, p.lines, Style{
			Kind: p.node.Kind, Class: p.node.Class, Status: st.Status[p.node.ID], Focus: st.Focus == p.node.ID,
		})
	}
	taken := map[EdgeRef]bool{}
	for _, ref := range st.Taken {
		taken[ref] = true
	}
	// The edges a run took are drawn last, so they pass over at crossings.
	for _, last := range []bool{false, true} {
		for _, rt := range l.routes {
			if rt.path == nil || taken[rt.edge.Ref()] != last {
				continue
			}
			st := edgeStyle(last)
			st.Line = rt.edge.Line
			c.path(rt.path, rt.group, st, true)
		}
	}
	for _, rt := range l.routes {
		if rt.placed {
			c.label(rt.labelX, rt.labelY, rt.label, edgeStyle(taken[rt.edge.Ref()]))
		}
	}
	return c
}

func edgeStyle(taken bool) Style {
	if taken {
		return Style{Status: Done}
	}
	return Style{}
}

// Layout places every node: where it was placed relative to others, by its
// placements, and everything else by following the edges, each step's
// ways on beside it and stacked one under the other. Then it routes the
// edges. Mistakes and conflicts become warnings; a diagram always lays
// out.
func (d *Diagram) Layout(opts ...LayoutOption) *Layout {
	cfg := layoutConfig{glyphs: Unicode, limits: DefaultLimits}
	for _, o := range opts {
		o(&cfg)
	}
	cfg.limits = cfg.limits.sane()
	cfg.wordWidth = cfg.limits.NodeWidth
	if cfg.fit != nil {
		return d.fitted(cfg)
	}
	l := d.placeAndRoute(cfg)
	l.fits = true
	return l
}

// placeAndRoute places the nodes and routes the edges. An edge with no way
// through, or no room for its label, is most often hemmed in where it
// ends, so its two nodes are given room all round and everything is laid
// out again, a few times, keeping the layout that drew the most.
func (d *Diagram) placeAndRoute(cfg layoutConfig) *Layout {
	l := d.place(cfg)
	l.route(cfg)
	pad := map[string]int{}
	for range roomAttempts {
		if l.lost() == 0 {
			break
		}
		for _, rt := range l.routes {
			if rt.path == nil || (rt.label != "" && !rt.placed) {
				pad[rt.edge.From] += roomPad
				pad[rt.edge.To] += roomPad
			}
		}
		c := cfg
		c.pad = maps.Clone(pad)
		next := d.place(c)
		next.route(c)
		if next.lost() < l.lost() {
			l = next
		}
	}
	return l
}

// roomAttempts is how many times nodes get more room for edges to reach
// them; roomPad is how much more, each time, in cells all round.
const (
	roomAttempts = 3
	roomPad      = 2
)

// place gives every node its place, and the picture the size of the boxes.
func (d *Diagram) place(cfg layoutConfig) *Layout {
	l := &Layout{glyphs: cfg.glyphs, limits: cfg.limits, orient: cfg.orient, index: map[string]int{}}
	if err := d.Check(); err != nil {
		if many, ok := err.(interface{ Unwrap() []error }); ok {
			l.warnings = append(l.warnings, many.Unwrap()...)
		} else {
			l.warnings = append(l.warnings, err)
		}
	}
	for _, n := range d.nodes {
		// Narrower text keeps its words whole, up to the width asked for.
		width := min(max(cfg.limits.NodeWidth, longestWord(n.Label())), cfg.wordWidth)
		lines := wrap(n.Label(), width, cfg.limits.NodeLines, cfg.glyphs.Ellipsis)
		w, h := boxSize(lines, n.Sub != nil)
		l.index[n.ID] = len(l.nodes)
		l.nodes = append(l.nodes, placed{node: n, lines: lines, rect: Rect{W: w, H: h}})
	}
	for _, e := range d.edges {
		if d.has(e.From) && d.has(e.To) {
			l.edges = append(l.edges, e)
		}
	}
	l.layFrames(d)
	// Wrapping: while a step reaches past the room there is, the flow
	// wraps at it and is laid out again. Each round wraps one more step,
	// so it ends.
	base := l.warnings
	breaks := map[int]bool{}
	for {
		l.warnings = append([]error(nil), base...)
		y := newLayouter(l, cfg)
		y.breaks = breaks
		y.run()
		if cfg.wrap <= 0 {
			break
		}
		i := y.overflow(cfg.wrap)
		if i < 0 {
			break
		}
		breaks[i] = true
	}
	// A wrap made while a later band still pushed the flow out may not be
	// needed once that band wrapped too: drop each that is not, latest
	// first.
	if len(breaks) > 1 {
		for _, i := range slices.Backward(slices.Sorted(maps.Keys(breaks))) {
			fewer := maps.Clone(breaks)
			delete(fewer, i)
			l.warnings = append([]error(nil), base...)
			y := newLayouter(l, cfg)
			y.breaks = fewer
			y.run()
			if y.overflow(cfg.wrap) < 0 {
				breaks = fewer
			}
		}
		l.warnings = append([]error(nil), base...)
		y := newLayouter(l, cfg)
		y.breaks = breaks
		y.run()
	}
	return l
}

// route routes the edges and frames the picture around everything.
func (l *Layout) route(cfg layoutConfig) {
	routes, warns := routeAll(l, cfg.orient, cfg.glyphs.Ellipsis, cfg.limits)
	l.routes = routes
	l.warnings = append(l.warnings, warns...)
	l.frame()
}

// frame fits the picture around everything drawn: the boxes, and the
// edges, which may go round them. Routes are in the router's cells until
// now.
func (l *Layout) frame() {
	minX, minY, maxX, maxY := 1<<30, 1<<30, -1<<30, -1<<30
	grow := func(x0, y0, x1, y1 int) {
		minX, minY, maxX, maxY = min(minX, x0), min(minY, y0), max(maxX, x1), max(maxY, y1)
	}
	for i := range l.nodes {
		r := &l.nodes[i].rect
		r.X, r.Y = r.X+routeMargin, r.Y+routeMargin
		grow(r.X, r.Y, r.X+r.W-1, r.Y+r.H-1)
	}
	for i := range l.frames {
		r := &l.frames[i].rect
		r.X, r.Y = r.X+routeMargin, r.Y+routeMargin
		grow(r.X, r.Y, r.X+r.W-1, r.Y+r.H-1)
	}
	for _, rt := range l.routes {
		for _, p := range rt.path {
			grow(p.X, p.Y, p.X, p.Y)
		}
		if rt.placed {
			w := textWidth(l.glyphs.LabelOpen) + textWidth(rt.label) + textWidth(l.glyphs.LabelClose)
			grow(rt.labelX, rt.labelY, rt.labelX+w-1, rt.labelY)
		}
	}
	if minX > maxX {
		l.W, l.H = 0, 0
		return
	}
	for i := range l.nodes {
		r := &l.nodes[i].rect
		r.X, r.Y = r.X-minX, r.Y-minY
	}
	for i := range l.frames {
		r := &l.frames[i].rect
		r.X, r.Y = r.X-minX, r.Y-minY
	}
	for i := range l.routes {
		rt := &l.routes[i]
		for k := range rt.path {
			rt.path[k].X -= minX
			rt.path[k].Y -= minY
		}
		rt.labelX, rt.labelY = rt.labelX-minX, rt.labelY-minY
	}
	l.W, l.H = maxX-minX+1, maxY-minY+1
}

type axis int

const (
	axisX axis = iota
	axisY
)

// Gaps in cells, across and down. A row is about twice as tall as a
// column is wide, so the vertical gaps are about half.
func gapCells(g Gap, ax axis) int {
	switch g.Size {
	case GapNone:
		return 0
	case GapTight:
		return [2]int{2, 1}[ax]
	case GapWide:
		return [2]int{8, 4}[ax]
	case GapCells:
		return g.Cells
	}
	return [2]int{4, 2}[ax]
}

type layouter struct {
	l     *Layout
	main  axis // the way a flow reads
	cross axis // the way siblings stack
	sys   [2]*system
	softs []soft
	// bound marks the axes a node's own placements decide; auto fills in
	// only the rest.
	bound [][2]bool
	// backward marks the axes a node is placed before its targets on.
	backward [][2]bool
	groups   int
	// why says, for each group the author wrote, which placement it is.
	why map[int]string
	// compact closes default gaps down to tight.
	compact bool
	// touching are the pairs a placement put flat against each other.
	touching map[[2]int]bool
	// pad is the room kept clear round each node.
	pad []int
	// breaks are the nodes the flow wraps at; beside, the nodes auto put
	// beside their step; order, the order auto followed them in.
	breaks map[int]bool
	beside []bool
	order  []int
	// dir is the way a node's band reads, 1 or -1, and band the node that
	// starts it.
	dir    []int
	band   []int
	parent []int
	// frameTries counts how many times each node and frame, or two
	// frames, were kept apart.
	frameTries map[[2]int]int
	// stacks are the further ways on auto stacks below the ones before,
	// and stacked the pairs (t, c) it has put c below t for.
	stacks  []stack
	stacked map[[2]int]bool
}

// stack is a further way on, c, to go below what the way before it led
// to, prev, where the two share columns: sub is what c leads to, c too.
// A wrapped band is one too, below everything before it; across is then
// the gap it keeps, rather than one sized by the edges between.
type stack struct {
	c         int
	prev, sub []int
	g         int
	across    int
}

// gap is gapCells, with default gaps tight when the layout is compact.
func (y *layouter) gap(g Gap, ax axis) int {
	if y.compact && (g.Size == GapDefault || g.Size == GapNormal) {
		g.Size = GapTight
	}
	return gapCells(g, ax)
}

// soft is a placement against several targets at once on an axis where it
// needs an exact position: centered on them, or aligned with their
// region. It is held as a lower bound, raised to the region each round.
type soft struct {
	node    int
	ax      axis
	rel     Relation // Level, an alignment of a side, or 0 for centered
	targets []int
	cons    int
}

func newLayouter(l *Layout, cfg layoutConfig) *layouter {
	y := &layouter{l: l, main: axisX, cross: axisY, why: map[int]string{}, compact: cfg.compact}
	if cfg.orient == TopToBottom {
		y.main, y.cross = axisY, axisX
	}
	n := len(l.nodes)
	y.sys = [2]*system{newSystem(n), newSystem(n)}
	y.bound = make([][2]bool, n)
	y.backward = make([][2]bool, n)
	y.beside = make([]bool, n)
	y.dir = make([]int, n)
	y.band = make([]int, n)
	y.parent = make([]int, n)
	y.breaks = map[int]bool{}
	y.touching = map[[2]int]bool{}
	y.frameTries = map[[2]int]int{}
	y.pad = make([]int, n)
	for i, p := range l.nodes {
		y.pad[i] = cfg.pad[p.node.ID]
	}
	return y
}

// size is node i's size on ax, with the room kept clear round it.
func (y *layouter) size(i int, ax axis) int {
	if ax == axisX {
		return y.l.nodes[i].rect.W + 2*y.pad[i]
	}
	return y.l.nodes[i].rect.H + 2*y.pad[i]
}

// box is node i's rect with the room kept clear round it.
func (y *layouter) box(i int) Rect {
	r, p := y.l.nodes[i].rect, y.pad[i]
	return Rect{r.X - p, r.Y - p, r.W + 2*p, r.H + 2*p}
}

func (y *layouter) group() int { y.groups++; return y.groups }

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func (y *layouter) run() {
	y.placements()
	y.auto()
	y.stack()
	y.solve()
}

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

// placements turns what the author wrote into constraints.
func (y *layouter) placements() {
	for i, p := range y.l.nodes {
		n := p.node
		// The first direction deciding one axis only, and its targets.
		var lone Relation
		var loneTargets []int
		for k := range n.Place {
			pl := n.Place[k]
			var targets []int
			for _, t := range pl.Targets {
				if ti, ok := y.l.index[t]; ok && ti != i {
					targets = append(targets, ti)
				}
			}
			if len(targets) == 0 {
				continue // Check reported it
			}
			g := y.group()
			y.why[g] = fmt.Sprintf("node %q: %q", n.ID, pl.String())
			if pl.Rel.IsAlignment() {
				y.align(i, pl.Rel, targets, g, prioUser)
				continue
			}
			for _, ax := range directionAxes(pl.Rel) {
				y.direction(i, pl.Rel, ax, targets, pl.Gap, g, prioUser)
				y.bound[i][ax] = true
				y.backward[i][ax] = y.backward[i][ax] || !forward(pl.Rel, ax)
			}
			if lone == 0 && len(directionAxes(pl.Rel)) == 1 {
				lone, loneTargets = pl.Rel, targets
			}
		}
		// A lone direction also centers the node on its targets along the
		// axis nothing else decides.
		if lone != 0 {
			if ax := 1 - directionAxes(lone)[0]; !y.bound[i][ax] {
				y.center(i, ax, loneTargets, y.group())
				y.bound[i][ax] = true
			}
		}
	}
}

// directionAxes are the axes a direction decides.
func directionAxes(r Relation) []axis {
	switch r {
	case LeftOf, RightOf:
		return []axis{axisX}
	case Above, Below:
		return []axis{axisY}
	}
	return []axis{axisX, axisY}
}

// forward reports whether r puts the node after its targets on ax.
func forward(r Relation, ax axis) bool {
	if ax == axisX {
		return r == RightOf || r == AboveRight || r == BelowRight
	}
	return r == Below || r == BelowLeft || r == BelowRight
}

// direction keeps node i at least a gap from every target, on ax.
func (y *layouter) direction(i int, r Relation, ax axis, targets []int, gap Gap, g int, p prio) {
	for _, t := range targets {
		if gap.Size == GapNone || (gap.Size == GapCells && gap.Cells == 0) {
			y.touching[[2]int{min(i, t), max(i, t)}] = true
		}
		cells := y.gap(gap, ax)
		if gap.Size == GapDefault {
			cells = max(cells, y.labelRoom(i, t, ax))
		}
		if forward(r, ax) {
			y.sys[ax].floor(t, i, y.size(t, ax)+cells, g, p)
		} else {
			y.sys[ax].floor(i, t, y.size(i, ax)+cells, g, p)
		}
	}
}

// labelRoom is the room the labels on edges between two nodes need, so a
// line with its label fits between them. An edge back the other way
// counts too: it may go round, but it may also need the gap for its label.
func (y *layouter) labelRoom(a, b int, ax axis) int {
	ida, idb := y.l.nodes[a].node.ID, y.l.nodes[b].node.ID
	room := 0
	for _, e := range y.l.edges {
		between := (e.From == ida && e.To == idb) || (e.From == idb && e.To == ida)
		if e.Label == "" || !between {
			continue
		}
		room = max(room, y.labelGap(e.Label, ax))
	}
	return room
}

// labelGap is the gap a labelled edge needs: across, room for a trunk to
// branch, the line before the label, the label, a run of line and the
// arrowhead; down, a row each for line, label, line and arrowhead.
func (y *layouter) labelGap(label string, ax axis) int {
	if ax == axisY {
		return 4
	}
	g := y.l.glyphs
	label = cut(label, y.l.limits.LabelWidth, g.Ellipsis)
	return textWidth(g.LabelOpen) + textWidth(label) + textWidth(g.LabelClose) + 2 + labelLead + 2
}

// align shares a line with the targets on the axis r decides.
func (y *layouter) align(i int, r Relation, targets []int, g int, p prio) {
	ax := axisY
	if r == LeftLevel || r == RightLevel {
		ax = axisX
	}
	y.bound[i][ax] = true
	if len(targets) > 1 {
		y.addSoft(i, ax, r, targets, g, p)
		return
	}
	t := targets[0]
	var d int
	switch r {
	case Level:
		d = floorDiv(y.size(t, ax)-y.size(i, ax), 2)
	case BottomLevel, RightLevel:
		d = y.size(t, ax) - y.size(i, ax)
	}
	y.sys[ax].equal(t, i, d, g, p)
}

// center centers node i on its targets along ax.
func (y *layouter) center(i int, ax axis, targets []int, g int) {
	if len(targets) > 1 {
		y.addSoft(i, ax, 0, targets, g, prioCenter)
		return
	}
	t := targets[0]
	y.sys[ax].equal(t, i, floorDiv(y.size(t, ax)-y.size(i, ax), 2), g, prioCenter)
}

func (y *layouter) addSoft(i int, ax axis, r Relation, targets []int, g int, p prio) {
	c := y.sys[ax].floor(src, i, 0, g, p)
	y.softs = append(y.softs, soft{node: i, ax: ax, rel: r, targets: targets, cons: c})
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
		if from != to && !contains(kids[from], to) {
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

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

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
		if contains(dropped, g) {
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
