package cligram

import "fmt"

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
