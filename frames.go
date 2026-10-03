package cligram

import (
	"fmt"
	"slices"
)

// A group is drawn as a frame round its nodes: their boxes, and the frames
// of groups inside it, with room for lines between. No other node is in
// it, and frames of groups side by side keep apart, as boxes do.

// frame is a group as laid out.
type frame struct {
	group Group
	// members are the nodes in it, in groups inside it too; depth how
	// many frames each is inside of, counting this one.
	members []int
	depth   map[int]int
	// inner are the frames inside it, as indices of Layout.frames.
	inner []int
	level int // how many frames it is inside of
	rect  Rect
	// wide is how much further right its title takes it than its
	// members and margin do.
	wide int
}

// Room between a frame's border and what is in it: its border and two
// columns across, its border and a row down.
const (
	frameMarginX = 3
	frameMarginY = 2
)

// frameMargin is a frame's margin on ax.
func frameMargin(ax axis) int {
	if ax == axisX {
		return frameMarginX
	}
	return frameMarginY
}

// layFrames finds the groups that have nodes, and what is in each. A
// group whose parents lead nowhere or back to it, which Check reports,
// stands on its own.
func (l *Layout) layFrames(d *Diagram) {
	l.frames = nil
	parent := map[string]string{}
	for _, g := range d.groups {
		parent[g.ID] = g.Parent
	}
	// chain is the groups a node or group is in, innermost first.
	chain := func(id string) []string {
		var out []string
		seen := map[string]bool{}
		for g := id; g != ""; g = parent[g] {
			if _, ok := d.group(g); !ok || seen[g] {
				break
			}
			seen[g] = true
			out = append(out, g)
		}
		return out
	}
	index := map[string]int{}
	for _, g := range d.groups {
		f := frame{group: g, depth: map[int]int{}}
		for i, p := range l.nodes {
			if at := slices.Index(chain(p.node.Group), g.ID); at >= 0 {
				f.members = append(f.members, i)
				f.depth[i] = at + 1
			}
		}
		if len(f.members) == 0 {
			continue
		}
		f.level = len(chain(g.ID)) - 1
		index[g.ID] = len(l.frames)
		l.frames = append(l.frames, f)
	}
	for i, f := range l.frames {
		if len(chain(f.group.ID)) > 1 {
			if p, ok := index[chain(f.group.ID)[1]]; ok {
				l.frames[p].inner = append(l.frames[p].inner, i)
			}
		}
	}
}

// frameRects sizes every frame round what is in it now, the innermost
// first: wide enough for its title, too.
func (y *layouter) frameRects() {
	fs := y.l.frames
	order := make([]int, len(fs))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return fs[b].level - fs[a].level })
	for _, i := range order {
		f := &fs[i]
		x0, y0, x1, y1 := 1<<30, 1<<30, -1<<30, -1<<30
		grow := func(r Rect) {
			x0, y0, x1, y1 = min(x0, r.X), min(y0, r.Y), max(x1, r.X+r.W), max(y1, r.Y+r.H)
		}
		for _, m := range f.members {
			if f.depth[m] == 1 {
				grow(y.l.nodes[m].rect)
			}
		}
		for _, in := range f.inner {
			grow(fs[in].rect)
		}
		r := Rect{x0 - frameMarginX, y0 - frameMarginY, x1 - x0 + 2*frameMarginX, y1 - y0 + 2*frameMarginY}
		w := max(r.W, textWidth(y.l.frameTitle(f.group))+7) // ╭╌ title ╌╮
		f.wide, r.W = w-r.W, w
		// A frame inside reaches as far right as its title does.
		for _, in := range f.inner {
			f.wide = max(f.wide, fs[in].rect.X+fs[in].rect.W+frameMarginX-(r.X+r.W))
		}
		f.wide = max(f.wide, 0)
		f.rect = r
	}
}

// frameTitle is a group's title as its frame shows it, cut like a label.
func (l *Layout) frameTitle(g Group) string {
	t := g.Title
	if t == "" {
		t = g.ID
	}
	return cut(t, l.limits.NodeWidth, l.glyphs.Ellipsis)
}

// outsiders separates every node that is in a frame it is not a member
// of, and every two frames side by side that overlap, and reports whether
// it moved any.
func (y *layouter) outsiders() bool {
	n := len(y.l.nodes)
	fs := y.l.frames
	moved := false
	// A frame grows as what is in it moves: a pair kept apart may meet
	// again, and is kept apart again, a few times.
	again := func(key [2]int) bool {
		y.frameTries[key]++
		return y.frameTries[key] <= frameTries
	}
	// Frames side by side first, each as a whole: a node in one frame that
	// is in another's goes with its frame, never on its own, so the way
	// they go apart is one choice, not many that may not agree.
	for a := range fs {
		for b := a + 1; b < len(fs); b++ {
			key := [2]int{n + a, n + b}
			if y.nested(a, b) || y.nested(b, a) || !overlaps(grow(fs[a].rect), fs[b].rect) || !again(key) {
				continue
			}
			moved = y.separateFrames(a, b) || moved
		}
	}
	for fi, f := range fs {
		for o := range y.l.nodes {
			key := [2]int{o, n + fi}
			if slices.Contains(f.members, o) || y.sibling(o, fi) || !overlaps(grow(f.rect), y.box(o)) || !again(key) {
				continue
			}
			moved = y.separateFrame(o, fi) || moved
		}
	}
	return moved
}

// frameTries is how many times a node and a frame, or two frames, are
// kept apart before they are left as they are.
const frameTries = 3

// sibling reports whether node o is in a frame side by side with frame
// fi, neither inside the other: that frame is kept apart from it whole.
func (y *layouter) sibling(o, fi int) bool {
	for gi, g := range y.l.frames {
		if gi != fi && slices.Contains(g.members, o) && !y.nested(gi, fi) && !y.nested(fi, gi) {
			return true
		}
	}
	return false
}

// nested reports whether frame b is inside frame a.
func (y *layouter) nested(a, b int) bool {
	for _, in := range y.l.frames[a].inner {
		if in == b || y.nested(in, b) {
			return true
		}
	}
	return false
}

// separateFrame puts node o out of frame fi: before or after every member,
// on the axis they overlap least on.
func (y *layouter) separateFrame(o, fi int) bool {
	f := y.l.frames[fi]
	return y.keepApart([]int{o}, map[int]int{o: 0}, 0, f.members, f.depth, f.wide, y.box(o), f.rect,
		fmt.Sprintf("node %q and group %q", y.l.nodes[o].node.ID, f.group.ID))
}

// separateFrames puts two frames side by side, apart.
func (y *layouter) separateFrames(a, b int) bool {
	fa, fb := y.l.frames[a], y.l.frames[b]
	return y.keepApart(fa.members, fa.depth, fa.wide, fb.members, fb.depth, fb.wide, fa.rect, fb.rect,
		fmt.Sprintf("groups %q and %q", fa.group.ID, fb.group.ID))
}

// keepApart puts the nodes of one side wholly before or after the other's
// on one axis, the frames each is inside keeping their margins; a side
// whose title takes it wider keeps that room after it across.
func (y *layouter) keepApart(as []int, da map[int]int, wa int, bs []int, db map[int]int, wb int, ra, rb Rect, what string) bool {
	over := [2]int{
		min(ra.X+ra.W, rb.X+rb.W) - max(ra.X, rb.X),
		min(ra.Y+ra.H, rb.Y+rb.H) - max(ra.Y, rb.Y),
	}
	axes := []axis{y.cross, y.main}
	if over[y.main] < over[y.cross] {
		axes = []axis{y.main, y.cross}
	}
	// Each axis, the way they lean first, then the other way: a placement
	// may hold one of them after the other only.
	for _, ax := range axes {
		ca, cb := 2*[2]int{ra.X, ra.Y}[ax]+[2]int{ra.W, ra.H}[ax], 2*[2]int{rb.X, rb.Y}[ax]+[2]int{rb.W, rb.H}[ax]
		for _, flip := range []bool{cb < ca, cb >= ca} {
			first, firstDepth, firstWide, then, thenDepth := as, da, wa, bs, db
			if flip {
				first, firstDepth, firstWide, then, thenDepth = bs, db, wb, as, da
			}
			if ax != axisX {
				firstWide = 0
			}
			g := y.group()
			s := y.sys[ax]
			for _, p := range first {
				for _, q := range then {
					gap := gapCells(Gap{Size: GapTight}, ax) + frameMargin(ax)*(firstDepth[p]+thenDepth[q]) + firstWide
					s.floor(p, q, y.size(p, ax)+gap, g, prioOverlap)
				}
			}
			_, dropped := s.solve()
			if contains(dropped, g) {
				continue
			}
			y.warnDropped(dropped)
			return true
		}
	}
	y.l.warnings = append(y.l.warnings, fmt.Errorf("%s overlap: their placements leave no way apart", what))
	return false
}
