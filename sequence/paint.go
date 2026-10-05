package sequence

import (
	"strings"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/internal/text"
	"github.com/rivo/uniseg"
)

// grid is the cells a layout is painted on.
type grid struct {
	w, h  int
	cells []cell
	g     *Glyphs
}

type cell struct {
	g      string
	cont   bool // the right half of a wide grapheme on its left
	st     cligram.Style
	styled bool
}

func newGrid(w, h int, g *Glyphs) grid {
	return grid{w: w, h: h, cells: make([]cell, w*h), g: g}
}

// put sets the cell at (x, y) to glyph s of width wd, keeping wide
// graphemes whole: one half overwritten is cleared.
func (gr *grid) put(x, y int, s string, wd int, st cligram.Style) {
	if x < 0 || y < 0 || x+wd > gr.w || y >= gr.h {
		return
	}
	for k := range wd {
		i := y*gr.w + x + k
		if gr.cells[i].cont && x+k > 0 {
			gr.cells[i-1] = cell{}
		}
		if x+k+1 < gr.w && gr.cells[i+1].cont {
			gr.cells[i+1] = cell{}
		}
	}
	gr.cells[y*gr.w+x] = cell{g: s, st: st, styled: true}
	if wd == 2 {
		gr.cells[y*gr.w+x+1] = cell{cont: true}
	}
}

// glyph sets a one-cell glyph.
func (gr *grid) glyph(x, y int, s string, st cligram.Style) { gr.put(x, y, s, 1, st) }

// text writes s from (x, y), grapheme by grapheme.
func (gr *grid) text(x, y int, s string, st cligram.Style) {
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		w := g.Width()
		if w == 0 {
			continue
		}
		gr.put(x, y, g.Str(), w, st)
		x += w
	}
}

// render paints the cells with t, row by row, without the blanks a row
// ends in.
func (gr *grid) render(t cligram.Theme) string {
	var out strings.Builder
	out.Grow(gr.w * gr.h * 3)
	for y := range gr.h {
		row := gr.cells[y*gr.w : (y+1)*gr.w]
		end := len(row) - 1
		for end >= 0 && row[end].g == "" && !row[end].cont {
			end--
		}
		var run strings.Builder
		var runSt cligram.Style
		runStyled := false
		flush := func() {
			if run.Len() == 0 {
				return
			}
			if runStyled {
				out.WriteString(t.Paint(runSt, run.String()))
			} else {
				out.WriteString(run.String())
			}
			run.Reset()
		}
		for x := 0; x <= end; x++ {
			c := row[x]
			if c.cont {
				continue
			}
			g, styled := c.g, c.styled && c.g != "" && c.g != " "
			if g == "" {
				g = " "
			}
			if styled != runStyled || (styled && c.st != runSt) {
				flush()
				runSt, runStyled = c.st, styled
			}
			run.WriteString(g)
		}
		flush()
		if y < gr.h-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// paint draws the layout: frames at the back, lifelines over them, then
// boxes, notes and messages over the lifelines.
func (ly *laying) paint(gr *grid, cx []int, top int) {
	g := ly.o.glyphs
	at := func(s spot) int { return cx[s.p] + s.off }
	frameSt := cligram.Style{Part: cligram.PartFrame}
	titleSt := cligram.Style{Part: cligram.PartFrameTitle}
	fb := g.Frame
	for _, f := range ly.frames {
		l, r := f.wall(at, true), f.wall(at, false)
		for x := l + 1; x < r; x++ {
			gr.glyph(x, f.y0, fb.Horizontal, frameSt)
			gr.glyph(x, f.y1, fb.Horizontal, frameSt)
			for _, s := range f.sections {
				gr.glyph(x, s.y, fb.Horizontal, frameSt)
			}
		}
		gr.glyph(l, f.y0, fb.TopLeft, frameSt)
		gr.glyph(r, f.y0, fb.TopRight, frameSt)
		gr.glyph(l, f.y1, fb.BottomLeft, frameSt)
		gr.glyph(r, f.y1, fb.BottomRight, frameSt)
		for y := f.y0 + 1; y < f.y1; y++ {
			gr.glyph(l, y, fb.Vertical, frameSt)
			gr.glyph(r, y, fb.Vertical, frameSt)
		}
	}

	lifeSt := cligram.Style{Part: cligram.PartLifeline}
	for p, x := range cx {
		for y := top; y < gr.h; y++ {
			s := g.Lines[north|south]
			if ly.isActive(p, y) {
				s = g.Active
			}
			gr.glyph(x, y, s, lifeSt)
		}
	}

	// Titles go over the lifelines they cross, so they can be read whole,
	// and a stroke of border follows each, so where it ends is plain.
	title := func(x, y int, t string) {
		gr.text(x, y, " "+t+" ", titleSt)
		gr.glyph(x+text.Width(t)+2, y, fb.Horizontal, frameSt)
	}
	for _, f := range ly.frames {
		x := cx[f.lo] + 2
		title(x, f.y0, f.title)
		for _, s := range f.sections {
			title(x, s.y, s.title)
		}
	}

	for p, hd := range ly.heads {
		ly.paintHead(gr, ly.d.participants[p], hd, cx[p], top)
	}
	for _, it := range ly.items {
		switch {
		case it.kind == stepNote:
			ly.paintNote(gr, it, cx)
		case it.from == it.to:
			ly.paintSelf(gr, it, cx[it.from])
		default:
			ly.paintMessage(gr, it, cx)
		}
	}
}

func (ly *laying) paintHead(gr *grid, p Participant, hd head, x, top int) {
	g := ly.o.glyphs
	kind := cligram.Step
	if p.Actor {
		kind = cligram.Terminal
	}
	b := g.Boxes[kind]
	border := cligram.Style{Part: cligram.PartBorder, Kind: kind, Class: p.Class}
	textSt := cligram.Style{Part: cligram.PartText, Kind: kind, Class: p.Class}
	left, y0 := x-hd.halfL, top-hd.h
	right, bottom := left+hd.w-1, top-1
	for xx := left + 1; xx < right; xx++ {
		gr.glyph(xx, y0, b.Horizontal, border)
		gr.glyph(xx, bottom, b.Horizontal, border)
	}
	gr.glyph(x, bottom, b.OutBottom, border)
	gr.glyph(left, y0, b.TopLeft, border)
	gr.glyph(right, y0, b.TopRight, border)
	gr.glyph(left, bottom, b.BottomLeft, border)
	gr.glyph(right, bottom, b.BottomRight, border)
	for i, line := range hd.lines {
		y := y0 + 1 + i
		gr.glyph(left, y, b.Vertical, border)
		gr.glyph(right, y, b.Vertical, border)
		w := text.Width(line)
		gr.text(left+2+(hd.w-4-w)/2, y, line, textSt)
	}
	// A shorter box's text is centered down it too: blank rows have sides.
	for y := y0 + 1 + len(hd.lines); y < bottom; y++ {
		gr.glyph(left, y, b.Vertical, border)
		gr.glyph(right, y, b.Vertical, border)
	}
}

// lineGlyphs are a message's straight runs, across and down.
func (ly *laying) lineGlyphs(s cligram.LineStyle) (across, down string) {
	g := ly.o.glyphs
	switch s {
	case cligram.Dashed:
		return g.Dashed[0], g.Dashed[1]
	case cligram.Thick:
		return g.Thick[0], g.Thick[1]
	}
	return g.Lines[east|west], g.Lines[north|south]
}

// headGlyph is what ends a message at its receiver, pointing in dir
// (0 up, 1 right, 2 down, 3 left), and whether it has one.
func (ly *laying) headGlyph(h Head, dir int) (string, bool) {
	switch h {
	case Open:
		return "", false
	case Cross:
		return ly.o.glyphs.Cross, true
	}
	return ly.o.glyphs.Arrows[dir], true
}

// junction is a lifeline where a line leaves it to the right or left.
func (ly *laying) junction(p, y int, right bool) string {
	g := ly.o.glyphs
	switch {
	case ly.isActive(p, y) && right:
		return g.ActiveRight
	case ly.isActive(p, y):
		return g.ActiveLeft
	case right:
		return g.Lines[north|south|east]
	}
	return g.Lines[north|south|west]
}

func (ly *laying) paintMessage(gr *grid, it *item, cx []int) {
	m := it.msg
	lineSt := cligram.Style{Part: cligram.PartLine, Line: m.Line}
	arrowSt := cligram.Style{Part: cligram.PartArrow, Line: m.Line}
	labelSt := cligram.Style{Part: cligram.PartLabel, Line: m.Line}
	lo, hi := min(it.from, it.to), max(it.from, it.to)
	xl, xr := cx[lo], cx[hi]
	// A label crossing lifelines covers them, a cell round it, so it
	// reads as one block of text.
	if len(it.lines) > 0 {
		from := max(xl+1+(xr-xl-1-it.width)/2-1, xl+1)
		to := min(from+it.width+1, xr-1)
		for i := range it.lines {
			for x := from; x <= to; x++ {
				gr.glyph(x, it.y+i, " ", labelSt)
			}
		}
	}
	for i, line := range it.lines {
		w := text.Width(line)
		gr.text(xl+1+(xr-xl-1-w)/2, it.y+i, line, labelSt)
	}
	y := it.y + it.rows - 1
	across, _ := ly.lineGlyphs(m.Line)
	for x := xl + 1; x < xr; x++ {
		gr.glyph(x, y, across, lineSt)
	}
	rightward := it.to == hi
	// The receiver's end.
	recv, inward := xr, -1
	if !rightward {
		recv, inward = xl, 1
	}
	dir := 1
	if !rightward {
		dir = 3
	}
	if h, ok := ly.headGlyph(m.Head, dir); ok {
		gr.glyph(recv+inward, y, h, arrowSt)
	} else {
		gr.glyph(recv, y, ly.junction(it.to, y, !rightward), lineSt)
	}
	// The sender's end.
	send := xl
	if !rightward {
		send = xr
	}
	if m.Both {
		gr.glyph(send-inward, y, ly.o.glyphs.Arrows[(dir+2)%4], arrowSt)
	} else {
		gr.glyph(send, y, ly.junction(it.from, y, rightward), lineSt)
	}
}

func (ly *laying) paintSelf(gr *grid, it *item, x int) {
	m := it.msg
	g := ly.o.glyphs
	lineSt := cligram.Style{Part: cligram.PartLine, Line: m.Line}
	arrowSt := cligram.Style{Part: cligram.PartArrow, Line: m.Line}
	labelSt := cligram.Style{Part: cligram.PartLabel, Line: m.Line}
	across, down := ly.lineGlyphs(m.Line)
	y0, y1 := it.y, it.y+it.rows-1
	if m.Both {
		gr.glyph(x+1, y0, g.Arrows[3], arrowSt)
	} else {
		gr.glyph(x, y0, ly.junction(it.from, y0, true), lineSt)
		gr.glyph(x+1, y0, across, lineSt)
	}
	gr.glyph(x+2, y0, across, lineSt)
	gr.glyph(x+3, y0, g.Lines[south|west], lineSt)
	for y := y0 + 1; y < y1; y++ {
		gr.glyph(x+3, y, down, lineSt)
	}
	gr.glyph(x+3, y1, g.Lines[north|west], lineSt)
	gr.glyph(x+2, y1, across, lineSt)
	if h, ok := ly.headGlyph(m.Head, 3); ok {
		gr.glyph(x+1, y1, h, arrowSt)
	} else {
		gr.glyph(x+1, y1, across, lineSt)
		gr.glyph(x, y1, ly.junction(it.to, y1, true), lineSt)
	}
	for i, line := range it.lines {
		gr.text(x+5, y0+i, line, labelSt)
	}
}

func (ly *laying) paintNote(gr *grid, it *item, cx []int) {
	g := ly.o.glyphs
	border := cligram.Style{Part: cligram.PartNote}
	textSt := cligram.Style{Part: cligram.PartNoteText}
	l, r := it.extent()
	x0, x1 := cx[l[0].p]+l[0].off, cx[r[0].p]+r[0].off
	y0, y1 := it.y, it.y+it.rows-1
	for x := x0 + 1; x < x1; x++ {
		gr.glyph(x, y0, g.Lines[east|west], border)
		gr.glyph(x, y1, g.Lines[east|west], border)
	}
	gr.glyph(x0, y0, g.Lines[east|south], border)
	gr.glyph(x1, y0, g.Lines[south|west], border)
	gr.glyph(x0, y1, g.Lines[north|east], border)
	gr.glyph(x1, y1, g.Lines[north|west], border)
	for i, line := range it.lines {
		y := y0 + 1 + i
		gr.glyph(x0, y, g.Lines[north|south], border)
		gr.glyph(x1, y, g.Lines[north|south], border)
		// The note covers the lifelines under it.
		for x := x0 + 1; x < x1; x++ {
			gr.glyph(x, y, " ", textSt)
		}
		w := text.Width(line)
		gr.text(x0+1+(x1-x0-1-w)/2, y, line, textSt)
	}
}
