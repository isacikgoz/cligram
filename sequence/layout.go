package sequence

import (
	"errors"
	"strconv"
	"strings"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/internal/text"
)

// Limits keep text within what a terminal can show: every text, of a
// participant, message, note or block, wraps at Width cells and stops
// after Lines lines, cut with an ellipsis.
type Limits struct{ Width, Lines int }

// DefaultLimits leave room for a sentence on each message.
var DefaultLimits = Limits{Width: 32, Lines: 4}

// minWidth is the narrowest text Fit wraps to.
const minWidth = 8

// Option sets something about laying a diagram out.
type Option func(*options)

type options struct {
	fit        bool
	fitW, fitH int
	glyphs     *Glyphs
	limits     Limits
}

// Fit lays the diagram out to fit w columns, wrapping its text narrower
// as it must; a diagram is as tall as its messages make it, so one
// taller than h is scrolled.
func Fit(w, h int) Option {
	return func(o *options) { o.fit, o.fitW, o.fitH = true, w, h }
}

// WithGlyphs draws with g: Unicode, or ASCII.
func WithGlyphs(g *Glyphs) Option { return func(o *options) { o.glyphs = g } }

// WithLimits caps text at lim.
func WithLimits(lim Limits) Option { return func(o *options) { o.limits = lim } }

// Layout is a diagram laid out: where every box, line and word goes.
type Layout struct {
	W, H     int
	fits     bool
	warnings []error
	grid     grid
}

// Fits reports whether the layout fits what Fit asked for; without Fit,
// it does.
func (l *Layout) Fits() bool { return l.fits }

// Warnings are the mistakes the diagram was built with: it is drawn as
// best it can be anyway.
func (l *Layout) Warnings() []error { return l.warnings }

// Render paints the layout with theme t.
func (l *Layout) Render(t cligram.Theme) string { return l.grid.render(t) }

// Layout lays the diagram out. It never fails: Warnings lists what was
// wrong with it.
func (d *Diagram) Layout(opts ...Option) *Layout {
	o := options{glyphs: Unicode, limits: DefaultLimits}
	for _, opt := range opts {
		opt(&o)
	}
	if o.glyphs == nil {
		o.glyphs = Unicode
	}
	o.limits.Width = max(o.limits.Width, minWidth)
	o.limits.Lines = max(o.limits.Lines, 1)

	// Narrower text first changes nothing but the words' wrapping, so the
	// widest that fits is the one to draw.
	widths := []int{o.limits.Width}
	if o.fit {
		for w := o.limits.Width * 3 / 4; w > minWidth; w = w * 3 / 4 {
			widths = append(widths, w)
		}
		if o.limits.Width > minWidth {
			widths = append(widths, minWidth)
		}
	}
	var best *Layout
	for _, w := range widths {
		l := d.lay(w, o)
		if best == nil || l.W < best.W {
			best = l
		}
		if !o.fit || l.W <= o.fitW {
			best = l
			break
		}
	}
	best.fits = !o.fit || (best.W <= o.fitW && best.H <= o.fitH)
	if err := d.Check(); err != nil {
		var joined interface{ Unwrap() []error }
		if errors.As(err, &joined) {
			best.warnings = joined.Unwrap()
		} else {
			best.warnings = []error{err}
		}
	}
	return best
}

// spot is a cell off from a participant's lifeline, by off columns.
type spot struct{ p, off int }

// head is a participant's box at the top.
type head struct {
	lines        []string
	w, h         int
	halfL, halfR int // columns from the lifeline to the box's sides
}

// item is a step drawn in rows of its own: a message, a note.
type item struct {
	step
	from, to int // participants: a message's, or a note's lowest and highest
	lines    []string
	width    int // the widest line
	y        int // its first row
	rows     int
	noteW    int // a note's box, at its narrowest
}

// frame is a block drawn round its steps.
type frame struct {
	title       string
	y0, y1      int // its top and bottom rows
	sections    []section
	lo, hi      int    // the participants in it, -1 while none are
	left, right []spot // where its sides may be, the outermost counting
}

type section struct {
	y     int
	title string
}

// span is the rows a participant is active.
type span struct{ from, to int }

// laying is a diagram being laid out with text wrapped at width.
type laying struct {
	d       *Diagram
	o       options
	width   int
	heads   []head
	items   []*item
	frames  []*frame
	need    map[[2]int]int // need[{i, j}]: lifeline j this far right of i at least
	active  [][]span
	depth   []int
	since   []int
	y       int
	number  int
	numStep int
}

func (d *Diagram) lay(width int, o options) *Layout {
	n := len(d.participants)
	ly := &laying{d: d, o: o, width: width, need: map[[2]int]int{},
		active: make([][]span, n), depth: make([]int, n), since: make([]int, n)}
	g := o.glyphs
	top := 0
	for _, p := range d.participants {
		s := p.Text
		if s == "" {
			s = p.ID
		}
		lines := ly.wrap(s)
		tw := widest(lines)
		w := tw + 4
		if w%2 == 0 {
			w++ // odd, so the lifeline leaves from the middle
		}
		h := len(lines) + 2
		ly.heads = append(ly.heads, head{lines: lines, w: w, h: h, halfL: w / 2, halfR: w - 1 - w/2})
		top = max(top, h)
	}
	for i := 0; i+1 < n; i++ {
		ly.require(i, i+1, ly.heads[i].halfR+ly.heads[i+1].halfL+2)
	}
	ly.y = top
	ly.steps()
	ly.y++ // the lifelines go on a row past the last step
	h := ly.y
	for p := range ly.depth {
		if ly.depth[p] > 0 {
			ly.active[p] = append(ly.active[p], span{ly.since[p], h - 1})
		}
	}

	// Each lifeline as far left as what is between it and those before
	// lets it be.
	cx := make([]int, n)
	for j := 1; j < n; j++ {
		for i := 0; i < j; i++ {
			if v, ok := ly.need[[2]int{i, j}]; ok {
				cx[j] = max(cx[j], cx[i]+v)
			}
		}
	}
	at := func(s spot) int { return cx[s.p] + s.off }
	minX, maxX := 0, 0
	if n > 0 {
		minX, maxX = cx[0]-ly.heads[0].halfL, cx[0]+ly.heads[0].halfR
	}
	for i, hd := range ly.heads {
		minX, maxX = min(minX, cx[i]-hd.halfL), max(maxX, cx[i]+hd.halfR)
	}
	for _, it := range ly.items {
		l, r := it.extent()
		for _, s := range l {
			minX = min(minX, at(s))
		}
		for _, s := range r {
			maxX = max(maxX, at(s))
		}
	}
	for _, f := range ly.frames {
		minX, maxX = min(minX, f.wall(at, true)), max(maxX, f.wall(at, false))
	}
	for i := range cx {
		cx[i] -= minX
	}
	l := &Layout{W: maxX - minX + 1, H: h}
	l.grid = newGrid(l.W, l.H, g)
	ly.paint(&l.grid, cx, top)
	return l
}

func (ly *laying) wrap(s string) []string {
	if s == "" {
		return nil
	}
	return text.Wrap(s, ly.width, ly.o.limits.Lines, ly.o.glyphs.Ellipsis)
}

func widest(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, text.Width(l))
	}
	return w
}

// require keeps lifeline j at least d columns right of lifeline i, i < j.
func (ly *laying) require(i, j, d int) {
	if i < 0 || j >= len(ly.heads) || i >= j {
		return
	}
	k := [2]int{i, j}
	ly.need[k] = max(ly.need[k], d)
}

// steps gives every step its rows, and its room between the lifelines.
func (ly *laying) steps() {
	d := ly.d
	var open []*frame
	for _, s := range d.steps {
		switch s.kind {
		case stepNumber:
			ly.number, ly.numStep = s.start, s.by
		case stepActivate:
			ly.activate(d.index[s.id], ly.y)
		case stepDeactivate:
			ly.deactivate(d.index[s.id], ly.y-1)
		case stepBlock:
			f := &frame{title: ly.title(s), y0: ly.y, lo: -1, hi: -1}
			ly.frames = append(ly.frames, f)
			open = append(open, f)
			ly.y++
		case stepSection:
			if len(open) > 0 {
				f := open[len(open)-1]
				f.sections = append(f.sections, section{ly.y, ly.title(s)})
				ly.y++
			}
		case stepEnd:
			if len(open) > 0 {
				ly.close(open)
				open = open[:len(open)-1]
			}
		case stepMessage, stepNote:
			it := ly.item(s)
			ly.items = append(ly.items, it)
			l, r := it.extent()
			// A note beside a lifeline keeps further from the next one than
			// from its own, so it reads as its own lifeline's.
			gap := 2
			if it.kind == stepNote && it.note.side != noteOver {
				gap = 3
			}
			ly.clear(l, r, gap)
			if len(open) > 0 {
				open[len(open)-1].hold(min(it.from, it.to), max(it.from, it.to), l, r)
			}
		}
	}
	for len(open) > 0 {
		ly.close(open)
		open = open[:len(open)-1]
	}
}

// title is a block's or section's title: its word and its text, the
// text cut to one line.
func (ly *laying) title(s step) string {
	t := s.word
	if s.text != "" {
		t += " " + text.Cut(s.text, ly.width, ly.o.glyphs.Ellipsis)
	}
	return t
}

// close ends the innermost open frame: its sides go round what it holds,
// and it is held by the frame round it.
func (ly *laying) close(open []*frame) {
	f := open[len(open)-1]
	f.y1 = ly.y
	ly.y++
	if f.lo < 0 {
		f.lo, f.hi = 0, 0
	}
	// Its sides clear its own lifelines, whatever it holds.
	f.left = append(f.left, spot{f.lo, 0})
	f.right = append(f.right, spot{f.hi, 0})
	f.left = shift(f.left, -2)
	f.right = shift(f.right, 2)
	// Room for its titles, which start right of its first lifeline so the
	// lifeline goes on through: "╭╌╌│╌ title ╌╮".
	tw := text.Width(f.title)
	for _, s := range f.sections {
		tw = max(tw, text.Width(s.title))
	}
	if f.lo < f.hi {
		ly.require(f.lo, f.hi, tw+3)
	} else {
		f.right = append(f.right, spot{f.lo, tw + 5})
	}
	// Its sides clear the lifelines outside it.
	for _, s := range f.left {
		ly.require(f.lo-1, s.p, 2-s.off)
	}
	for _, s := range f.right {
		ly.require(s.p, f.hi+1, s.off+2)
	}
	if len(open) > 1 {
		open[len(open)-2].hold(f.lo, f.hi, f.left, f.right)
	}
}

func shift(ss []spot, by int) []spot {
	out := make([]spot, len(ss))
	for i, s := range ss {
		out[i] = spot{s.p, s.off + by}
	}
	return out
}

// hold takes in something from participant lo to hi reaching l and r.
func (f *frame) hold(lo, hi int, l, r []spot) {
	if f.lo < 0 || lo < f.lo {
		f.lo = lo
	}
	if hi > f.hi {
		f.hi = hi
	}
	f.left = append(f.left, l...)
	f.right = append(f.right, r...)
}

// wall is the column of a frame's left or right side.
func (f *frame) wall(at func(spot) int, left bool) int {
	ss := f.right
	if left {
		ss = f.left
	}
	x := at(ss[0])
	for _, s := range ss[1:] {
		if left {
			x = min(x, at(s))
		} else {
			x = max(x, at(s))
		}
	}
	return x
}

// clear keeps the lifelines beside what reaches past its own gap columns
// clear of it.
func (ly *laying) clear(l, r []spot, gap int) {
	for _, s := range l {
		if s.off < 0 {
			ly.require(s.p-1, s.p, gap-s.off)
		}
	}
	for _, s := range r {
		if s.off > 0 {
			ly.require(s.p, s.p+1, s.off+gap)
		}
	}
}

// item lays out a message or a note, in the rows from ly.y.
func (ly *laying) item(s step) *item {
	d := ly.d
	it := &item{step: s, y: ly.y}
	switch s.kind {
	case stepMessage:
		m := s.msg
		it.from, it.to = d.index[m.From], d.index[m.To]
		if ly.numStep != 0 {
			// The number stays on the first line, with the words it numbers.
			num := strconv.Itoa(ly.number) + "."
			ly.number += ly.numStep
			it.lines = ly.wrap(strings.TrimSpace(num + " " + m.Text))
			if len(it.lines) > 1 && it.lines[0] == num {
				it.lines = append([]string{num + " " + it.lines[1]}, it.lines[2:]...)
			}
		} else {
			it.lines = ly.wrap(m.Text)
		}
		it.width = widest(it.lines)
		if it.from == it.to {
			// ├──┐ text
			// │◄─┘
			it.rows = max(2, len(it.lines))
			if m.Deactivate {
				ly.deactivate(it.from, it.y)
			}
			if m.Activate {
				ly.activate(it.to, it.y+it.rows-1)
			}
		} else {
			it.rows = len(it.lines) + 1
			lo, hi := min(it.from, it.to), max(it.from, it.to)
			room := 3
			if m.Both {
				room = 4
			}
			ly.require(lo, hi, max(room, it.width+3))
			row := it.y + it.rows - 1
			if m.Deactivate {
				ly.deactivate(it.from, row)
			}
			if m.Activate {
				ly.activate(it.to, row)
			}
		}
	case stepNote:
		a, b := d.index[s.note.from], d.index[s.note.to]
		it.from, it.to = min(a, b), max(a, b)
		it.lines = ly.wrap(s.text)
		it.width = widest(it.lines)
		it.rows = len(it.lines) + 2
		it.noteW = it.width + 4
		if s.note.side == noteOver && it.from == it.to && it.noteW%2 == 0 {
			it.noteW++
		}
		if s.note.side == noteOver && it.from < it.to {
			ly.require(it.from, it.to, it.noteW-5)
		}
	}
	ly.y += it.rows
	return it
}

// extent is the cells an item reaches to on the left and the right, off
// the lifelines.
func (it *item) extent() (l, r []spot) {
	if it.kind == stepMessage {
		if it.from == it.to {
			reach := 3
			if it.width > 0 {
				reach = 4 + it.width
			}
			return []spot{{it.from, 0}}, []spot{{it.from, reach}}
		}
		return []spot{{min(it.from, it.to), 0}}, []spot{{max(it.from, it.to), 0}}
	}
	switch it.note.side {
	case noteRight:
		return []spot{{it.from, 2}}, []spot{{it.from, 1 + it.noteW}}
	case noteLeft:
		return []spot{{it.from, -1 - it.noteW}}, []spot{{it.from, -2}}
	}
	if it.from == it.to {
		return []spot{{it.from, -it.noteW / 2}}, []spot{{it.from, it.noteW / 2}}
	}
	return []spot{{it.from, -2}}, []spot{{it.to, 2}}
}

func (ly *laying) activate(p, row int) {
	if ly.depth[p] == 0 {
		ly.since[p] = row
	}
	ly.depth[p]++
}

func (ly *laying) deactivate(p, row int) {
	if ly.depth[p] == 0 {
		return
	}
	ly.depth[p]--
	if ly.depth[p] == 0 && row >= ly.since[p] {
		ly.active[p] = append(ly.active[p], span{ly.since[p], row})
	}
}

// isActive reports whether participant p is active in row y.
func (ly *laying) isActive(p, y int) bool {
	for _, s := range ly.active[p] {
		if y >= s.from && y <= s.to {
			return true
		}
	}
	return false
}
