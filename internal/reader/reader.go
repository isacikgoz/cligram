// Package reader reads a cligram drawing back from what it shows, as a
// person would: boxes by their corners, lines followed from where they
// leave a box, through junctions and crossings, to the arrowhead they end
// in, and labels by the stretch of line they sit on. It knows nothing of
// how the drawing was made, only what its glyphs mean, so it can check
// that a picture says what its diagram says.
//
// It reads the Unicode glyphs, and the colors of ANSI escape codes.
package reader

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/rivo/uniseg"
)

// Kind is the kind of box, by its border.
type Kind int

const (
	Step     Kind = iota // rounded
	Decision             // double
	Terminal             // heavy
)

// Cell is one cell of the picture.
type Cell struct {
	G string `json:"g"` // what it shows; "" for the right half of a wide grapheme
	// SGR is the escape parameters it was drawn with, "" for none.
	SGR  string `json:"sgr,omitempty"`
	Cont bool   `json:"cont,omitempty"` // the right half of a wide grapheme
}

// Grid is the picture as cells.
type Grid struct {
	W, H  int
	Cells [][]Cell
}

func (g *Grid) at(x, y int) Cell {
	if y < 0 || y >= g.H || x < 0 || x >= len(g.Cells[y]) {
		return Cell{G: " "}
	}
	return g.Cells[y][x]
}

var sgrRe = regexp.MustCompile(`^\x1b\[([0-9;]*)m`)

// Parse turns text, with or without ANSI colors, into cells.
func Parse(text string) *Grid {
	g := &Grid{}
	for _, line := range strings.Split(text, "\n") {
		var row []Cell
		sgr := ""
		for len(line) > 0 {
			if m := sgrRe.FindStringSubmatch(line); m != nil {
				sgr = m[1]
				if sgr == "0" {
					sgr = ""
				}
				line = line[len(m[0]):]
				continue
			}
			gr, rest, width, _ := uniseg.FirstGraphemeClusterInString(line, -1)
			line = rest
			row = append(row, Cell{G: gr, SGR: sgr})
			if width == 2 {
				row = append(row, Cell{Cont: true, SGR: sgr})
			}
		}
		g.Cells = append(g.Cells, row)
		g.W = max(g.W, len(row))
	}
	g.H = len(g.Cells)
	return g
}

// Line directions, as bits.
const (
	n uint8 = 1 << iota
	e
	s
	w
)

var dirs = []struct {
	bit    uint8
	dx, dy int
}{{n, 0, -1}, {e, 1, 0}, {s, 0, 1}, {w, -1, 0}}

func opposite(b uint8) uint8 {
	switch b {
	case n:
		return s
	case e:
		return w
	case s:
		return n
	}
	return e
}

// lines are the line glyphs and the ways they lead.
var lines = map[string]uint8{
	"│": n | s, "─": e | w,
	"┊": n | s, "┄": e | w, // dashed
	"┃": n | s, "━": e | w, // thick
	"└": n | e, "┘": n | w, "┌": e | s, "┐": s | w,
	"├": n | e | s, "┤": n | s | w, "┬": e | s | w, "┴": n | e | w, "┼": n | e | s | w,
}

// styles are the glyphs that show a line's style; a line with none of
// them is solid.
var styles = map[string]string{"┊": "dashed", "┄": "dashed", "┃": "thick", "━": "thick"}

// arrows are the arrowheads, by the way they point.
var arrows = map[string]uint8{"▲": n, "►": e, "▼": s, "◄": w}

// family is how one kind of box is drawn.
type family struct {
	kind                   Kind
	tl, tr, bl, br, hz, vt string
	// outs are the border glyphs with a line leaving: by the way it leaves.
	outTop, outBottom, outLeft, outRight string
}

var families = []family{
	{Step, "╭", "╮", "╰", "╯", "─", "│", "┴", "┬", "┤", "├"},
	{Decision, "╔", "╗", "╚", "╝", "═", "║", "╧", "╤", "╢", "╟"},
	{Terminal, "┏", "┓", "┗", "┛", "━", "┃", "┷", "┯", "┨", "┠"},
}

// Markers say a box's status.
var Markers = map[string]string{" ": "idle", "▸": "active", "✓": "done", "✗": "failed", "◔": "waiting"}

// Box is a node as drawn.
type Box struct {
	X, Y, W, H int // its whole cells, a stacked box's back frame too
	Kind       Kind
	Stacked    bool
	Marker     string   // the status marker, " " for none
	Lines      []string // its text, a line a row, trailing blanks off
	BorderSGR  string   // how its top left corner was colored
	MarkerSGR  string
}

// Edge is a line from one box to another, by box index.
type Edge struct {
	From, To int
	Label    string // "" for none
	ArrowSGR string // how its arrowhead was colored
	Style    string // solid, dashed or thick
	ArrowX   int
	ArrowY   int
}

// Picture is what a drawing shows.
type Picture struct {
	Grid   *Grid
	Boxes  []Box
	Edges  []Edge
	Frames []Frame
}

// Text is a box's text as one string, lines joined by spaces.
func (b Box) Text() string { return strings.Join(b.Lines, " ") }

// Read reads text into a picture, and says what in it cannot be read:
// a broken box, a line that ends in nothing, a line from two boxes at
// once, a label that belongs to no edge or to several.
func Read(text string) (*Picture, error) {
	r := &read{g: Parse(text), owner: map[pt]int{}, port: map[pt]uint8{}, frameCell: map[pt]bool{}}
	r.frames()
	r.boxes()
	r.labels()
	r.lines()
	r.trace()
	return &Picture{Grid: r.g, Boxes: r.out, Edges: r.edges, Frames: r.frameList}, errors.Join(r.errs...)
}

type pt struct{ x, y int }

type label struct {
	x, y, w int
	text    string
}

type read struct {
	g     *Grid
	out   []Box
	owner map[pt]int   // cells of boxes, by box index
	port  map[pt]uint8 // border cells a line leaves through, by the way
	label map[pt]int   // cells of labels, by label index
	lbls  []label
	edges []Edge
	errs  []error
	// frameCell are the cells of frames, title included, but not where a
	// line crosses one.
	frameCell map[pt]bool
	frameList []Frame
}

func (r *read) fail(format string, args ...any) { r.errs = append(r.errs, fmt.Errorf(format, args...)) }

func (r *read) is(x, y int, gs ...string) bool {
	c := r.g.at(x, y).G
	for _, g := range gs {
		if c == g {
			return true
		}
	}
	return false
}

// boxes finds every box by its top left corner and checks its border.
func (r *read) boxes() {
	for y := 0; y < r.g.H; y++ {
		for x := 0; x < len(r.g.Cells[y]); x++ {
			for _, f := range families {
				if r.g.at(x, y).G == f.tl && !r.frameCell[pt{x, y}] {
					if _, taken := r.owner[pt{x, y}]; !taken {
						r.box(x, y, f)
					}
				}
			}
		}
	}
}

func (r *read) box(x0, y0 int, f family) {
	// Along the top to the top right corner, down to the bottom.
	x1 := x0 + 1
	for r.is(x1, y0, f.hz, f.outTop) {
		x1++
	}
	if !r.is(x1, y0, f.tr) {
		r.fail("box at (%d,%d): its top does not end in %s", x0, y0, f.tr)
		return
	}
	y1 := y0 + 1
	for r.is(x0, y1, f.vt, f.outLeft) {
		y1++
	}
	if !r.is(x0, y1, f.bl) || x1-x0 < 2 || y1-y0 < 2 {
		r.fail("box at (%d,%d): its left side does not end in %s", x0, y0, f.bl)
		return
	}
	b := Box{X: x0, Y: y0, Kind: f.kind, BorderSGR: r.g.at(x0, y0).SGR}
	for x := x0 + 1; x < x1; x++ {
		if !r.is(x, y1, f.hz, f.outBottom) {
			r.fail("box at (%d,%d): bottom broken at (%d,%d) by %q", x0, y0, x, y1, r.g.at(x, y1).G)
		}
	}
	if !r.is(x1, y1, f.br) {
		r.fail("box at (%d,%d): no bottom right corner", x0, y0)
	}
	// Stacked: a back frame one cell down and right.
	b.Stacked = r.is(x1+1, y0+1, f.tr) && r.is(x0+1, y1+1, f.bl) && r.is(x1+1, y1+1, f.br)
	frontRight := x1
	if !b.Stacked {
		for y := y0 + 1; y < y1; y++ {
			if !r.is(x1, y, f.vt, f.outRight) {
				r.fail("box at (%d,%d): right side broken at (%d,%d)", x0, y0, x1, y)
			}
		}
	} else {
		for y := y0 + 1; y < y1; y++ {
			if !r.is(x1, y, f.vt) {
				r.fail("stacked box at (%d,%d): front right side broken at (%d,%d)", x0, y0, x1, y)
			}
		}
		for y := y0 + 2; y <= y1; y++ {
			if !r.is(x1+1, y, f.vt, f.outRight) {
				r.fail("stacked box at (%d,%d): back right side broken at (%d,%d)", x0, y0, x1+1, y)
			}
		}
		for x := x0 + 2; x <= x1; x++ {
			if !r.is(x, y1+1, f.hz, f.outBottom) {
				r.fail("stacked box at (%d,%d): back bottom broken at (%d,%d)", x0, y0, x, y1+1)
			}
		}
	}
	b.W, b.H = x1-x0+1, y1-y0+1
	if b.Stacked {
		b.W, b.H = b.W+1, b.H+1
	}
	idx := len(r.out)
	for y := y0; y < y0+b.H; y++ {
		for x := x0; x < x0+b.W; x++ {
			if b.Stacked && ((x == x0+b.W-1 && y == y0) || (y == y0+b.H-1 && x == x0)) {
				continue // the corners the back frame leaves empty
			}
			r.owner[pt{x, y}] = idx
		}
	}
	// Ports: where lines leave, on the sides lines can leave by.
	mark := func(x, y int, glyph string, way uint8) {
		if r.is(x, y, glyph) {
			r.port[pt{x, y}] = way
		}
	}
	for x := x0 + 1; x < x1; x++ {
		mark(x, y0, f.outTop, n)
		if !b.Stacked {
			mark(x, y1, f.outBottom, s)
		}
	}
	for y := y0 + 1; y < y1; y++ {
		mark(x0, y, f.outLeft, w)
		if !b.Stacked {
			mark(x1, y, f.outRight, e)
		}
	}
	if b.Stacked {
		for y := y0 + 2; y <= y1; y++ {
			mark(x1+1, y, f.outRight, e)
		}
		for x := x0 + 2; x <= x1; x++ {
			mark(x, y1+1, f.outBottom, s)
		}
	}
	// Inside: a space, the marker, a space, then the text.
	b.Marker = r.g.at(x0+2, y0+1).G
	b.MarkerSGR = r.g.at(x0+2, y0+1).SGR
	if _, ok := Markers[b.Marker]; !ok {
		r.fail("box at (%d,%d): %q is not a status marker", x0, y0, b.Marker)
	}
	for y := y0 + 1; y < y1; y++ {
		var line strings.Builder
		for x := x0 + 4; x < frontRight-1; x++ {
			c := r.g.at(x, y)
			if !c.Cont {
				line.WriteString(c.G)
			}
		}
		b.Lines = append(b.Lines, strings.TrimRight(line.String(), " "))
	}
	for len(b.Lines) > 0 && b.Lines[len(b.Lines)-1] == "" {
		b.Lines = b.Lines[:len(b.Lines)-1]
	}
	r.out = append(r.out, b)
}

var labelRe = regexp.MustCompile(`\[ ([^\[\]]*?) \]`)

// labels finds every label, "[ text ]", outside the boxes.
func (r *read) labels() {
	r.label = map[pt]int{}
	for y := 0; y < r.g.H; y++ {
		// The row as text, with where each byte's cell is.
		var row strings.Builder
		var cellOf []int
		for x, c := range r.g.Cells[y] {
			if c.Cont {
				continue
			}
			if _, inBox := r.owner[pt{x, y}]; inBox || r.frameCell[pt{x, y}] {
				row.WriteString("\x00")
				cellOf = append(cellOf, x)
				continue
			}
			for range len(c.G) {
				cellOf = append(cellOf, x)
			}
			row.WriteString(c.G)
		}
		for _, m := range labelRe.FindAllStringSubmatchIndex(row.String(), -1) {
			x0 := cellOf[m[0]]
			x1 := cellOf[m[1]-1]
			l := label{x: x0, y: y, w: x1 - x0 + 1, text: row.String()[m[2]:m[3]]}
			idx := len(r.lbls)
			for x := x0; x <= x1; x++ {
				r.label[pt{x, y}] = idx
			}
			r.lbls = append(r.lbls, l)
		}
	}
}

// lines checks every cell outside boxes and labels is a line, an
// arrowhead or blank, and that every line leads somewhere.
func (r *read) lines() {
	for y := 0; y < r.g.H; y++ {
		for x, c := range r.g.Cells[y] {
			p := pt{x, y}
			if _, ok := r.owner[p]; ok {
				continue
			}
			if _, ok := r.label[p]; ok {
				continue
			}
			if r.frameCell[p] {
				continue
			}
			if c.Cont || c.G == " " || c.G == "" {
				continue
			}
			if _, ok := lines[c.G]; ok {
				continue
			}
			if _, ok := arrows[c.G]; ok {
				continue
			}
			r.fail("stray %q at (%d,%d)", c.G, x, y)
		}
	}
}

// mask is the ways the line at p leads, 0 for no line.
func (r *read) mask(p pt) uint8 {
	if r.g.at(p.x, p.y).Cont {
		return 0
	}
	if _, inBox := r.owner[p]; inBox {
		return 0
	}
	if _, inLabel := r.label[p]; inLabel {
		return 0
	}
	if r.frameCell[p] {
		return 0
	}
	return lines[r.g.at(p.x, p.y).G]
}

// trace follows every line: it joins the cells a line leads between into
// networks, each of which must leave one box and end in arrowheads.
func (r *read) trace() {
	// A union of everything that carries a line: line cells, ports,
	// arrowheads.
	parent := map[pt]pt{}
	var find func(pt) pt
	find = func(p pt) pt {
		q, ok := parent[p]
		if !ok || q == p {
			parent[p] = p
			return p
		}
		root := find(q)
		parent[p] = root
		return root
	}
	union := func(a, b pt) { parent[find(a)] = find(b) }
	// labelJoins are the joins a label sits on, so its edge can be told.
	type join struct{ a, b pt }
	labelJoins := map[int][]join{}

	// next is where a line at p heading way d leads: the cell it joins,
	// with the label it passes through on the way, if any.
	next := func(p pt, d uint8) (pt, int, bool) {
		var dx, dy int
		for _, dd := range dirs {
			if dd.bit == d {
				dx, dy = dd.dx, dd.dy
			}
		}
		q := pt{p.x + dx, p.y + dy}
		through := -1
		across := e | w
		if d == e || d == w {
			across = n | s
		}
		for {
			if li, ok := r.label[q]; ok {
				// Through a label: across one, along the row; or astride
				// one, on down the column.
				l := r.lbls[li]
				through = li
				if d == e || d == w {
					x := l.x - 1
					if d == e {
						x = l.x + l.w
					}
					q = pt{x, q.y}
				} else {
					q = pt{q.x, q.y + dy}
				}
				continue
			}
			if r.mask(q) == across {
				// A straight line across the way is a crossing: the line
				// passes under it.
				q = pt{q.x + dx, q.y + dy}
				continue
			}
			break
		}
		return q, through, true
	}
	accepts := func(q pt, d uint8) bool {
		// q takes a line arriving at it heading d.
		if m := r.mask(q); m != 0 {
			return m&opposite(d) != 0
		}
		if a, ok := arrows[r.g.at(q.x, q.y).G]; ok && r.mask(q) == 0 {
			if _, inLabel := r.label[q]; !inLabel {
				return a == d
			}
		}
		if way, ok := r.port[q]; ok {
			return way == opposite(d)
		}
		return false
	}
	link := func(p pt, d uint8) {
		q, li, _ := next(p, d)
		if !accepts(q, d) {
			r.fail("line at (%d,%d) leads %s into %q at (%d,%d)", p.x, p.y, name(d), r.g.at(q.x, q.y).G, q.x, q.y)
			return
		}
		union(p, q)
		if li >= 0 {
			labelJoins[li] = append(labelJoins[li], join{p, q})
		}
	}
	for y := 0; y < r.g.H; y++ {
		for x := range r.g.Cells[y] {
			p := pt{x, y}
			if m := r.mask(p); m != 0 {
				for _, d := range dirs {
					if m&d.bit != 0 {
						link(p, d.bit)
					}
				}
			}
		}
	}
	for p, way := range r.port {
		link(p, way)
	}
	// Arrowheads: each points into a box, and a line arrives behind it.
	type sink struct {
		at  pt
		box int
	}
	var sinks []sink
	for y := 0; y < r.g.H; y++ {
		for x, c := range r.g.Cells[y] {
			p := pt{x, y}
			a, ok := arrows[c.G]
			if !ok || r.mask(p) != 0 {
				continue
			}
			if _, inLabel := r.label[p]; inLabel {
				continue
			}
			if _, inBox := r.owner[p]; inBox {
				continue
			}
			q, _, _ := next(p, a)
			b, inBox := r.owner[q]
			if !inBox {
				r.fail("arrowhead at (%d,%d) points at no box", x, y)
				continue
			}
			behind, _, _ := next(p, opposite(a))
			if !accepts(behind, opposite(a)) {
				r.fail("arrowhead at (%d,%d) has no line behind it", x, y)
			}
			sinks = append(sinks, sink{p, b})
			find(p)
		}
	}

	// Networks: each leaves one box and ends in arrowheads.
	sources := map[pt]map[int]bool{}
	for p := range r.port {
		root := find(p)
		if sources[root] == nil {
			sources[root] = map[int]bool{}
		}
		sources[root][r.owner[p]] = true
	}
	ends := map[pt][]sink{}
	for _, sk := range sinks {
		ends[find(sk.at)] = append(ends[find(sk.at)], sk)
	}
	roots := map[pt]bool{}
	for p := range parent {
		roots[find(p)] = true
	}
	// A network's style is in the glyphs of its straight runs.
	style := map[pt]string{}
	for p := range parent {
		if st, ok := styles[r.g.at(p.x, p.y).G]; ok && r.mask(p) != 0 {
			root := find(p)
			if was, ok := style[root]; ok && was != st {
				r.fail("a line at (%d,%d) is both %s and %s", p.x, p.y, was, st)
			}
			style[root] = st
		}
	}
	for root := range roots {
		from := sources[root]
		switch {
		case len(from) == 0:
			r.fail("a line at (%d,%d) leaves no box", root.x, root.y)
			continue
		case len(from) > 1:
			r.fail("a line at (%d,%d) leaves %d boxes at once", root.x, root.y, len(from))
			continue
		case len(ends[root]) == 0:
			r.fail("a line at (%d,%d) ends in no arrowhead", root.x, root.y)
			continue
		}
		var src int
		for b := range from {
			src = b
		}
		for _, sk := range ends[root] {
			c := r.g.at(sk.at.x, sk.at.y)
			st := style[root]
			if st == "" {
				st = "solid"
			}
			r.edges = append(r.edges, Edge{From: src, To: sk.box, ArrowSGR: c.SGR, ArrowX: sk.at.x, ArrowY: sk.at.y, Style: st})
		}
	}

	// Labels: a label belongs to the one arrowhead its line leads to past
	// it, away from the box the line leaves. The joins again, as a graph
	// to cut at a label.
	edgesOf := func() map[pt][]pt {
		g := map[pt][]pt{}
		add := func(a, b pt) { g[a] = append(g[a], b); g[b] = append(g[b], a) }
		for y := 0; y < r.g.H; y++ {
			for x := range r.g.Cells[y] {
				p := pt{x, y}
				if m := r.mask(p); m != 0 {
					for _, d := range dirs {
						if m&d.bit != 0 {
							if q, _, _ := next(p, d.bit); accepts(q, d.bit) {
								add(p, q)
							}
						}
					}
				}
			}
		}
		for p, way := range r.port {
			if q, _, _ := next(p, way); accepts(q, way) {
				add(p, q)
			}
		}
		return g
	}
	adj := edgesOf()
	sinkAt := map[pt]int{}
	for i, e := range r.edges {
		sinkAt[pt{e.ArrowX, e.ArrowY}] = i
	}
	// reach is what can be reached from p without crossing the joins cut.
	reach := func(start pt, cut map[[2]pt]bool) map[pt]bool {
		seen := map[pt]bool{start: true}
		stack := []pt{start}
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, q := range adj[p] {
				if cut[[2]pt{p, q}] || cut[[2]pt{q, p}] || seen[q] {
					continue
				}
				seen[q] = true
				stack = append(stack, q)
			}
		}
		return seen
	}
	for li, l := range r.lbls {
		cut := map[[2]pt]bool{}
		var far []pt
		if js := labelJoins[li]; len(js) > 0 {
			for _, j := range js {
				cut[[2]pt{j.a, j.b}] = true
			}
			far = []pt{js[0].a, js[0].b}
		} else {
			// Beside its line: the line cell next to it.
			at, ok := r.besideOf(l)
			if !ok {
				r.fail("label %q at (%d,%d) is on no line", l.text, l.x, l.y)
				continue
			}
			// Cut every join of that cell, and look from each side.
			for _, q := range adj[at] {
				cut[[2]pt{at, q}] = true
				far = append(far, q)
			}
		}
		// The side holding a box's port is the way back; the arrowheads
		// on the other side are the label's.
		var mine []int
		for _, start := range far {
			side := reach(start, cut)
			hasPort := false
			for p := range side {
				if _, ok := r.port[p]; ok {
					hasPort = true
				}
			}
			if hasPort {
				continue
			}
			for p := range side {
				if i, ok := sinkAt[p]; ok {
					mine = append(mine, i)
				}
			}
		}
		sort.Ints(mine)
		mine = dedupe(mine)
		switch len(mine) {
		case 1:
			if r.edges[mine[0]].Label != "" {
				r.fail("edge to box %d has two labels: %q and %q", r.edges[mine[0]].To, r.edges[mine[0]].Label, l.text)
			}
			r.edges[mine[0]].Label = l.text
		case 0:
			r.fail("label %q at (%d,%d) leads to no arrowhead of its own", l.text, l.x, l.y)
		default:
			r.fail("label %q at (%d,%d) sits on a line %d edges share", l.text, l.x, l.y, len(mine))
		}
	}
}

// besideOf is the line cell a label sits beside, not on: a cell of
// space between them across, or right over or under it.
func (r *read) besideOf(l label) (pt, bool) {
	var found []pt
	for _, x := range []int{l.x - 2, l.x + l.w + 1} {
		if r.mask(pt{x, l.y})&(n|s) != 0 {
			found = append(found, pt{x, l.y})
		}
	}
	for x := l.x; x < l.x+l.w; x++ {
		for _, y := range []int{l.y - 1, l.y + 1} {
			if r.mask(pt{x, y})&(e|w) != 0 && len(found) == 0 {
				found = append(found, pt{x, y})
			}
		}
	}
	if len(found) == 0 {
		return pt{}, false
	}
	return found[0], true
}

func dedupe(s []int) []int {
	var out []int
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func name(d uint8) string {
	switch d {
	case n:
		return "up"
	case e:
		return "right"
	case s:
		return "down"
	}
	return "left"
}
