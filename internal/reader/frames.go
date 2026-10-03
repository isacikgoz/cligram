package reader

import "strings"

// Frame is a group's frame, drawn round some boxes: ╭╌ Title ╌╌╮, its
// sides ╎, its corners rounded.
type Frame struct {
	Title      string
	X, Y, W, H int
}

// Contains reports whether the cell (x, y) is inside the frame's border.
func (f Frame) Contains(x, y int) bool {
	return x > f.X && x < f.X+f.W-1 && y > f.Y && y < f.Y+f.H-1
}

// Where a line crosses a frame's border, the border cell shows the line,
// straight across: these are the lines that may.
var (
	crossDown   = map[string]bool{"│": true, "┊": true, "┃": true}
	crossAcross = map[string]bool{"─": true, "┄": true, "━": true}
)

// frames finds every frame by its top left corner, ╭ then ╌, reads its
// title and checks its border, and keeps its cells from being read as
// anything else, but for lines crossing it.
func (r *read) frames() {
	for y := 0; y < r.g.H; y++ {
		for x := 0; x+1 < len(r.g.Cells[y]); x++ {
			if r.is(x, y, "╭") && r.is(x+1, y, "╌") && !r.frameCell[pt{x, y}] {
				r.frame(x, y)
			}
		}
	}
}

func (r *read) frame(x0, y0 int) {
	// The top: ╭╌, a space, the title, a space, then ╌ (or lines crossing)
	// to ╮.
	row := r.g.Cells[y0]
	if x0+3 >= len(row) || !r.is(x0+2, y0, " ") {
		r.fail("frame at (%d,%d): no title after ╭╌", x0, y0)
		return
	}
	var title strings.Builder
	x := x0 + 3
	titleEnds := func(x int) bool { return r.is(x, y0, " ") && r.is(x+1, y0, "╌", "╮") }
	for ; x+1 < len(row) && !titleEnds(x); x++ {
		if !row[x].Cont {
			title.WriteString(row[x].G)
		}
	}
	titleEnd := x // the space after the title
	x++
	for r.is(x, y0, "╌") || crossDown[r.g.at(x, y0).G] {
		x++
	}
	if !r.is(x, y0, "╮") {
		r.fail("frame at (%d,%d): its top does not end in ╮", x0, y0)
		return
	}
	x1 := x
	y := y0 + 1
	for r.is(x0, y, "╎") || crossAcross[r.g.at(x0, y).G] {
		y++
	}
	if !r.is(x0, y, "╰") {
		r.fail("frame at (%d,%d): its left side does not end in ╰", x0, y0)
		return
	}
	y1 := y
	for x := x0 + 1; x < x1; x++ {
		if !r.is(x, y1, "╌") && !crossDown[r.g.at(x, y1).G] {
			r.fail("frame at (%d,%d): bottom broken at (%d,%d) by %q", x0, y0, x, y1, r.g.at(x, y1).G)
		}
	}
	for y := y0 + 1; y < y1; y++ {
		if !r.is(x1, y, "╎") && !crossAcross[r.g.at(x1, y).G] {
			r.fail("frame at (%d,%d): right side broken at (%d,%d) by %q", x0, y0, x1, y, r.g.at(x1, y).G)
		}
	}
	if !r.is(x1, y1, "╯") {
		r.fail("frame at (%d,%d): no bottom right corner", x0, y0)
	}
	// Its own cells: the border but where lines cross it, and the title.
	claim := func(x, y int) {
		if g := r.g.at(x, y).G; !crossDown[g] && !crossAcross[g] {
			r.frameCell[pt{x, y}] = true
		}
	}
	for x := x0; x <= x1; x++ {
		claim(x, y0)
		claim(x, y1)
	}
	for y := y0; y <= y1; y++ {
		claim(x0, y)
		claim(x1, y)
	}
	for x := x0 + 2; x <= titleEnd; x++ {
		r.frameCell[pt{x, y0}] = true
	}
	r.frameList = append(r.frameList, Frame{Title: title.String(), X: x0, Y: y0, W: x1 - x0 + 1, H: y1 - y0 + 1})
}
