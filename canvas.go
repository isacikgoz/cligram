package cligram

import (
	"strings"

	"github.com/rivo/uniseg"
)

// Point is a cell: X counts columns from the left, Y rows from the top.
type Point struct{ X, Y int }

// Rect is a block of cells.
type Rect struct{ X, Y, W, H int }

// canvas is a grid of cells the picture is painted on, then turned into
// text.
type canvas struct {
	w, h   int
	cells  []cell
	glyphs *Glyphs
}

type cell struct {
	// g is what the cell shows; empty, it shows its lines, or nothing.
	g string
	// cont marks the right half of a two-cell-wide grapheme on its left.
	cont   bool
	style  Style
	styled bool
	// lines are the edge lines through the cell, as direction bits, and
	// group is the edge group that drew them: lines of one group join,
	// lines of different groups cross.
	lines uint8
	group int
	// line is the style of the lines through it: solid where lines of
	// different styles meet.
	line LineStyle
	// frame is the frame border glyph the cell shows when nothing else
	// is drawn in it.
	frame string
	// border is the side of a box this cell is the border of, if any, and
	// kind the kind of that box. Corners have no side.
	border Side
	kind   Kind
}

func newCanvas(w, h int, g *Glyphs) *canvas {
	return &canvas{w: w, h: h, cells: make([]cell, w*h), glyphs: g}
}

func (c *canvas) in(x, y int) bool { return x >= 0 && y >= 0 && x < c.w && y < c.h }

func (c *canvas) at(x, y int) *cell { return &c.cells[y*c.w+x] }

// put sets one cell to a glyph g of width wd (1 or 2), keeping wide
// graphemes whole: a grapheme half overwritten is cleared.
func (c *canvas) put(x, y int, g string, wd int, st Style) {
	if !c.in(x, y) || (wd == 2 && !c.in(x+1, y)) {
		return
	}
	c.clearHalf(x, y)
	if wd == 2 {
		c.clearHalf(x+1, y)
	}
	*c.at(x, y) = cell{g: g, style: st, styled: true}
	if wd == 2 {
		*c.at(x+1, y) = cell{cont: true, style: st, styled: true}
	}
}

// clearHalf blanks the other half of a wide grapheme at x, if x is one.
func (c *canvas) clearHalf(x, y int) {
	switch here := c.at(x, y); {
	case here.cont && x > 0:
		*c.at(x-1, y) = cell{}
	case !here.cont && x+1 < c.w && c.at(x+1, y).cont:
		*c.at(x+1, y) = cell{}
	}
}

// text writes s from x, at most maxW cells of it, and says how many cells
// it took. A wide grapheme that would not fit whole is left out.
func (c *canvas) text(x, y int, s string, st Style, maxW int) int {
	used := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		wd := g.Width()
		if wd == 0 {
			continue
		}
		if used+wd > maxW {
			break
		}
		c.put(x+used, y, g.Str(), wd, st)
		used += wd
	}
	return used
}

// textWidth is how many cells s takes.
func textWidth(s string) int { return uniseg.StringWidth(s) }

// Box geometry: a border, a space, the marker, a space, the text, a space,
// a border. The marker's place is kept whatever the status, so a box never
// changes size when the run moves.
const boxChrome = 6

// boxSize is the cells a node with these lines of text takes. A node that
// opens into another diagram is drawn stacked, one cell wider and taller.
func boxSize(lines []string, stacked bool) (w, h int) {
	for _, l := range lines {
		w = max(w, textWidth(l))
	}
	w, h = w+boxChrome, len(lines)+2
	if stacked {
		w, h = w+1, h+1
	}
	return w, h
}

// box draws a node in r, which boxSize sized, in style st: its kind,
// class, status and focus, the part set for each piece.
func (c *canvas) box(r Rect, stacked bool, lines []string, st Style) {
	b := c.glyphs.Boxes[st.Kind]
	part := func(p Part) Style { st.Part = p; return st }
	front := r
	if stacked {
		front.W, front.H = r.W-1, r.H-1
		c.frame(Rect{X: r.X + 1, Y: r.Y + 1, W: front.W, H: front.H}, st.Kind, b, part(PartBorder))
	}
	c.frame(front, st.Kind, b, part(PartBorder))
	for y := front.Y + 1; y < front.Y+front.H-1; y++ {
		for x := front.X + 1; x < front.X+front.W-1; x++ {
			c.put(x, y, " ", 1, part(PartText))
		}
	}
	c.text(front.X+2, front.Y+1, c.glyphs.Markers[st.Status], part(PartMarker), 1)
	for i, l := range lines {
		c.text(front.X+4, front.Y+1+i, l, part(PartText), front.W-boxChrome)
	}
}

// frame draws a box's border. Drawn second, a stacked box's front frame
// covers the back one wherever they meet.
func (c *canvas) frame(r Rect, kind Kind, b Borders, st Style) {
	x0, y0, x1, y1 := r.X, r.Y, r.X+r.W-1, r.Y+r.H-1
	side := func(x, y int, g string, s Side) {
		c.put(x, y, g, 1, st)
		if c.in(x, y) {
			c.at(x, y).border, c.at(x, y).kind = s, kind
		}
	}
	for x := x0 + 1; x < x1; x++ {
		side(x, y0, b.Horizontal, Top)
		side(x, y1, b.Horizontal, Bottom)
	}
	for y := y0 + 1; y < y1; y++ {
		side(x0, y, b.Vertical, Left)
		side(x1, y, b.Vertical, Right)
	}
	side(x0, y0, b.TopLeft, Auto)
	side(x1, y0, b.TopRight, Auto)
	side(x0, y1, b.BottomLeft, Auto)
	side(x1, y1, b.BottomRight, Auto)
}

// path draws an edge along pts, a line of horizontal and vertical runs.
// It may start on a box's border, which then shows the line leaving it,
// and ends on the cell before the box it arrives at, with an arrowhead
// when arrow is set. Lines of the same group join where they meet, as a
// decision's ways on share a trunk; lines of different groups cross, and
// the one drawn last passes over unbroken.
func (c *canvas) path(pts []Point, group int, st Style, arrow bool) {
	if len(pts) < 2 {
		return
	}
	masks := map[Point]uint8{}
	var order []Point
	add := func(p Point, m uint8) {
		if _, ok := masks[p]; !ok {
			order = append(order, p)
		}
		masks[p] |= m
	}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
		if dx != 0 && dy != 0 {
			panic("cligram: a path runs only horizontally and vertically")
		}
		fwd, back := dirOf(dx, dy), dirOf(-dx, -dy)
		for p := a; p != b; p = (Point{p.X + dx, p.Y + dy}) {
			add(p, fwd)
			add(Point{p.X + dx, p.Y + dy}, back)
		}
	}

	line := Style{Part: PartLine, Status: st.Status, Line: st.Line}
	for _, p := range order {
		if !c.in(p.X, p.Y) {
			continue
		}
		here := c.at(p.X, p.Y)
		m := masks[p]
		if here.border != Auto {
			c.leave(here)
			continue
		}
		if here.g != "" || here.cont {
			continue // a box or a label is never drawn over
		}
		switch {
		case crosses(here.lines, m):
			// A straight line across another, even one of its own group's
			// branches, crosses it: the one drawn last passes over.
			here.lines, here.line = m, st.Line
		case here.lines == 0:
			here.lines, here.line = m, st.Line
		default:
			here.lines |= m
			if here.line != st.Line {
				here.line = Solid // a trunk shared by different styles
			}
		}
		here.group, here.style, here.styled = group, line, true
	}

	if arrow {
		end, prev := pts[len(pts)-1], pts[len(pts)-2]
		head := c.glyphs.Arrows[arrowIndex(sign(end.X-prev.X), sign(end.Y-prev.Y))]
		if c.in(end.X, end.Y) && c.at(end.X, end.Y).border == Auto {
			c.put(end.X, end.Y, head, 1, Style{Part: PartArrow, Status: st.Status})
		}
	}
}

// lineGlyph is the glyph for a cell's lines: a dashed or thick line's
// straight runs in their own glyphs, everything else a solid line's.
func (c *canvas) lineGlyph(cl cell) string {
	runs := map[LineStyle][2]string{Dashed: c.glyphs.Dashed, Thick: c.glyphs.Thick}[cl.line]
	switch {
	case cl.line == Solid || cl.lines == 0:
	case cl.lines&^(east|west) == 0 && runs[0] != "":
		return runs[0]
	case cl.lines&^(north|south) == 0 && runs[1] != "":
		return runs[1]
	}
	return c.glyphs.Lines[cl.lines]
}

// groupFrame draws a group's frame round r, its title in the top border.
func (c *canvas) groupFrame(r Rect, title string) {
	b := c.glyphs.Frame
	set := func(x, y int, g string) {
		if c.in(x, y) {
			c.at(x, y).frame = g
		}
	}
	for x := r.X + 1; x < r.X+r.W-1; x++ {
		set(x, r.Y, b.Horizontal)
		set(x, r.Y+r.H-1, b.Horizontal)
	}
	for y := r.Y + 1; y < r.Y+r.H-1; y++ {
		set(r.X, y, b.Vertical)
		set(r.X+r.W-1, y, b.Vertical)
	}
	set(r.X, r.Y, b.TopLeft)
	set(r.X+r.W-1, r.Y, b.TopRight)
	set(r.X, r.Y+r.H-1, b.BottomLeft)
	set(r.X+r.W-1, r.Y+r.H-1, b.BottomRight)
	c.text(r.X+2, r.Y, " "+title+" ", Style{Part: PartFrameTitle}, r.W-4)
}

// leave shows a line leaving a box through its border.
func (c *canvas) leave(here *cell) {
	b := c.glyphs.Boxes[here.kind]
	g := map[Side]string{Top: b.OutTop, Bottom: b.OutBottom, Left: b.OutLeft, Right: b.OutRight}[here.border]
	here.g = g
}

// crosses reports whether two straight lines are perpendicular, so one can
// pass over the other.
func crosses(a, b uint8) bool {
	const vertical, horizontal = north | south, east | west
	return (a == vertical && b == horizontal) || (a == horizontal && b == vertical)
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

func dirOf(dx, dy int) uint8 {
	switch {
	case dy < 0:
		return north
	case dx > 0:
		return east
	case dy > 0:
		return south
	case dx < 0:
		return west
	}
	return 0
}

// arrowIndex is the arrowhead pointing the way (dx, dy) goes.
func arrowIndex(dx, dy int) int {
	switch {
	case dy < 0:
		return 0
	case dx > 0:
		return 1
	case dy > 0:
		return 2
	}
	return 3
}

// label writes an edge's label from x, over any line there.
func (c *canvas) label(x, y int, text string, st Style) {
	st.Part = PartLabel
	full := c.glyphs.LabelOpen + text + c.glyphs.LabelClose
	c.text(x, y, full, st, c.w-x)
}

// render turns the canvas into lines of text, colored by t, with no
// trailing blanks.
func (c *canvas) render(t Theme) string { return c.renderRect(t, Rect{0, 0, c.w, c.h}) }

// renderRect renders the cells in r, as many rows as r is tall. A wide
// grapheme cut in half by r's sides shows as a blank.
func (c *canvas) renderRect(t Theme, r Rect) string {
	var out strings.Builder
	for y := r.Y; y < r.Y+r.H; y++ {
		var row []cell
		if y >= 0 && y < c.h {
			from, to := max(r.X, 0), min(r.X+r.W, c.w)
			if from < to {
				row = append([]cell(nil), c.cells[y*c.w+from:y*c.w+to]...)
				if row[0].cont {
					row[0] = cell{}
				}
				if last := len(row) - 1; !row[last].cont && to < c.w && c.cells[y*c.w+to].cont {
					row[last] = cell{}
				}
			}
			// Cells left of the picture shift the row; none are drawn.
			if r.X < 0 {
				row = append(make([]cell, min(-r.X, r.W)), row...)
			}
		}
		end := len(row) - 1
		for end >= 0 && row[end].g == "" && row[end].lines == 0 && row[end].frame == "" && !row[end].cont {
			end--
		}
		var run strings.Builder
		var runStyle Style
		var runStyled bool
		flush := func() {
			if run.Len() == 0 {
				return
			}
			if runStyled {
				out.WriteString(t.Paint(runStyle, run.String()))
			} else {
				out.WriteString(run.String())
			}
			run.Reset()
		}
		for x := 0; x <= end; x++ {
			cl := row[x]
			if cl.cont {
				continue
			}
			g := cl.g
			if g == "" {
				g = c.lineGlyph(cl)
			}
			if g == "" && cl.frame != "" {
				g, cl.style, cl.styled = cl.frame, Style{Part: PartFrame}, true
			}
			styled := cl.styled && g != ""
			if g == "" {
				g = " "
			}
			if styled != runStyled || (styled && cl.style != runStyle) {
				flush()
				runStyle, runStyled = cl.style, styled
			}
			run.WriteString(g)
		}
		flush()
		if y < r.Y+r.H-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
