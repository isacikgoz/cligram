package cligram

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
)

// checkRoutes checks what every layout must hold: routes run straight
// between their corners, stay out of every box but leave through their
// source's border and stop beside their target, heading in; and no label
// lies on a line it does not belong to, on a box, or on another label.
func checkRoutes(t *testing.T, l *Layout) {
	t.Helper()
	inBox := func(p Point) int {
		for i, n := range l.nodes {
			r := n.rect
			if p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H {
				return i
			}
		}
		return -1
	}
	owner := map[Point]int{} // cell -> route drawing a line through it
	for ri, rt := range l.routes {
		if rt.path == nil {
			continue
		}
		cells := []Point{rt.path[0]}
		for k := 1; k < len(rt.path); k++ {
			a, b := rt.path[k-1], rt.path[k]
			if a.X != b.X && a.Y != b.Y {
				t.Errorf("%s: %v to %v is not straight", rt.edge.describe(), a, b)
				continue
			}
			sx, sy := sign(b.X-a.X), sign(b.Y-a.Y)
			for p := a; p != b; {
				p = Point{p.X + sx, p.Y + sy}
				cells = append(cells, p)
			}
		}
		from, to := l.index[rt.edge.From], l.index[rt.edge.To]
		if inBox(cells[0]) != from {
			t.Errorf("%s: starts at %v, not on its source's border", rt.edge.describe(), cells[0])
		}
		for _, p := range cells[1:] {
			if i := inBox(p); i >= 0 {
				t.Errorf("%s: runs through %s at %v", rt.edge.describe(), l.nodes[i].node.ID, p)
			}
			owner[p] = ri
		}
		end, before := cells[len(cells)-1], cells[len(cells)-2]
		next := Point{2*end.X - before.X, 2*end.Y - before.Y}
		if inBox(next) != to {
			t.Errorf("%s: its arrowhead at %v points at %v, not into %s", rt.edge.describe(), end, next, rt.edge.To)
		}
	}
	labels := map[Point]string{}
	for _, rt := range l.routes {
		if !rt.placed {
			continue
		}
		w := textWidth(l.glyphs.LabelOpen + rt.label + l.glyphs.LabelClose)
		for k := range w {
			p := Point{rt.labelX + k, rt.labelY}
			if i := inBox(p); i >= 0 {
				t.Errorf("label %q lies on %s at %v", rt.label, l.nodes[i].node.ID, p)
			}
			if o, ok := owner[p]; ok && l.routes[o].group != rt.group {
				t.Errorf("label %q lies on %s at %v", rt.label, l.routes[o].edge.describe(), p)
			}
			if other, ok := labels[p]; ok {
				t.Errorf("label %q lies on label %q at %v", rt.label, other, p)
			}
			labels[p] = rt.label
		}
	}
}

func TestRoutesHoldOnRandomFlows(t *testing.T) {
	for seed := range 60 {
		rng := rand.New(rand.NewPCG(uint64(seed), 7))
		d := New()
		n := 3 + rng.IntN(10)
		for i := range n {
			text := fmt.Sprintf("step %d", i)
			if rng.IntN(3) == 0 {
				text += "\nwith more"
			}
			var opts []NodeOption
			if rng.IntN(4) == 0 {
				opts = append(opts, As(Decision))
			}
			if rng.IntN(8) == 0 {
				opts = append(opts, Sub(func(context.Context) (*Diagram, error) { return New(), nil }))
			}
			d.Node(fmt.Sprint(i), text, opts...)
		}
		for i := 1; i < n; i++ {
			d.Edge(fmt.Sprint(rng.IntN(i)), fmt.Sprint(i))
		}
		for range rng.IntN(n) {
			label := ""
			if rng.IntN(2) == 0 {
				label = []string{"yes", "no", "retry", "needs more work"}[rng.IntN(4)]
			}
			d.Edge(fmt.Sprint(rng.IntN(n)), fmt.Sprint(rng.IntN(n)), Label(label))
		}
		for _, o := range []Orientation{LeftToRight, TopToBottom} {
			t.Run(fmt.Sprintf("seed%d/%d", seed, o), func(t *testing.T) {
				l := d.Layout(WithOrientation(o))
				checkRoutes(t, l)
				for _, w := range l.Warnings() {
					t.Log(w)
				}
			})
		}
	}
}
