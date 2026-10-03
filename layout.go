package cligram

import (
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

// route routes the edges and fits the picture around everything.
func (l *Layout) route(cfg layoutConfig) {
	routes, warns := routeAll(l, cfg.orient, cfg.glyphs.Ellipsis, cfg.limits)
	l.routes = routes
	l.warnings = append(l.warnings, warns...)
	l.fitPicture()
}

// fitPicture fits the picture around everything drawn: the boxes, the
// frames, and the edges, which may go round them. Routes are in the router's cells until
// now.
func (l *Layout) fitPicture() {
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
