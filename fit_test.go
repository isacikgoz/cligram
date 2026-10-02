package cligram_test

import (
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
)

// chain is n one-letter steps, a to the nth letter, each leading to the
// next: every box 7 by 3.
func chain(n int) *cligram.Diagram {
	var lines []string
	for i := range n {
		id := string(rune('a' + i))
		lines = append(lines, id)
		if i > 0 {
			lines = append(lines, string(rune('a'+i-1))+" -> "+id)
		}
	}
	return diagram(lines...)
}

func TestFitKeepsALayoutThatFits(t *testing.T) {
	plain := chain(3).Layout()
	l := chain(3).Layout(cligram.Fit(100, 100))
	if !l.Fits() || l.W != plain.W || l.H != plain.H || l.Orientation() != cligram.LeftToRight {
		t.Errorf("fitted %dx%d, plain %dx%d", l.W, l.H, plain.W, plain.H)
	}
}

func TestFitClosesTheGapsFirst(t *testing.T) {
	l := chain(3).Layout(cligram.Fit(25, 10))
	noWarnings(t, l)
	if !l.Fits() || l.Orientation() != cligram.LeftToRight {
		t.Fatalf("%dx%d fits %v, orientation %v", l.W, l.H, l.Fits(), l.Orientation())
	}
	at(t, l, map[string]cligram.Rect{"a": {0, 0, 7, 3}, "b": {9, 0, 7, 3}, "c": {18, 0, 7, 3}})
}

func TestFitWrapsALongFlowLikeASnake(t *testing.T) {
	l := chain(6).Layout(cligram.Fit(40, 30))
	noWarnings(t, l)
	if !l.Fits() || l.W > 40 || l.Orientation() != cligram.LeftToRight {
		t.Fatalf("%dx%d fits %v, orientation %v\n%s", l.W, l.H, l.Fits(), l.Orientation(), l.Render(cligram.State{}, cligram.Plain))
	}
	d, e, f := rect(t, l, "d"), rect(t, l, "e"), rect(t, l, "f")
	// e starts a band under d and the band reads back right to left.
	if e.Y <= d.Y+d.H || e.X+e.W != d.X+d.W || f.Y != e.Y || f.X+f.W >= e.X {
		t.Errorf("d %+v e %+v f %+v\n%s", d, e, f, l.Render(cligram.State{}, cligram.Plain))
	}
}

func TestFitTurnsTheFlowWhenItCannotWrap(t *testing.T) {
	l := chain(4).Layout(cligram.Fit(10, 40))
	noWarnings(t, l)
	if !l.Fits() || l.Orientation() != cligram.TopToBottom || l.W > 10 {
		t.Errorf("%dx%d fits %v, orientation %v", l.W, l.H, l.Fits(), l.Orientation())
	}
}

func TestFitCutsTextOnlyWhenNothingElseFits(t *testing.T) {
	d := cligram.New()
	d.Node("a", "a step whose summary runs on far longer than it needs to")
	if l := d.Layout(cligram.Fit(40, 10)); !l.Fits() || l.W <= 20 {
		t.Errorf("text cut although it fit at its usual width: %dx%d", l.W, l.H)
	}
	if l := d.Layout(cligram.Fit(20, 10)); !l.Fits() || l.W > 20 {
		t.Errorf("%dx%d does not fit 20x10", l.W, l.H)
	}
}

func TestWhenNothingFitsTheWidthIsKept(t *testing.T) {
	l := chain(12).Layout(cligram.Fit(20, 5))
	if l.Fits() || l.W > 20 {
		t.Errorf("%dx%d fits %v; want too tall but within 20 columns", l.W, l.H, l.Fits())
	}
}

func TestRevealMovesTheViewTheLeastItMust(t *testing.T) {
	l := chain(6).Layout() // 62x3, boxes 11 apart
	for _, tc := range []struct {
		view cligram.Rect
		id   string
		want cligram.Rect
	}{
		{cligram.Rect{0, 0, 20, 3}, "a", cligram.Rect{0, 0, 20, 3}},  // already in view
		{cligram.Rect{0, 0, 20, 3}, "c", cligram.Rect{10, 0, 20, 3}}, // c at 22..28, a cell to spare
		{cligram.Rect{40, 0, 20, 3}, "b", cligram.Rect{10, 0, 20, 3}},
		{cligram.Rect{0, 0, 20, 3}, "f", cligram.Rect{42, 0, 20, 3}},   // kept on the picture
		{cligram.Rect{30, 0, 100, 9}, "c", cligram.Rect{0, 0, 100, 9}}, // bigger than the picture
		{cligram.Rect{5, 0, 20, 3}, "ghost", cligram.Rect{5, 0, 20, 3}},
		{cligram.Rect{0, 0, 5, 3}, "d", cligram.Rect{33, 0, 5, 3}}, // too narrow: show where it starts
	} {
		if got := l.Reveal(tc.view, tc.id); got != tc.want {
			t.Errorf("Reveal(%+v, %s) = %+v, want %+v", tc.view, tc.id, got, tc.want)
		}
	}
}

func TestRenderViewShowsAWindow(t *testing.T) {
	l := chain(3).Layout()
	full := strings.Split(l.Render(cligram.State{}, cligram.Plain), "\n")
	got := l.RenderView(cligram.State{}, cligram.Plain, cligram.Rect{X: 5, Y: 1, W: 10, H: 3})
	// The same cells as the whole picture, the box's own blanks kept.
	var want []string
	for _, row := range full[1:] {
		r := []rune(row + strings.Repeat(" ", 40))
		want = append(want, string(r[5:15]))
	}
	want = append(want, "") // a row below the picture is blank
	rows := strings.Split(got, "\n")
	for i := range want {
		if i >= len(rows) || strings.TrimRight(rows[i], " ") != strings.TrimRight(want[i], " ") {
			t.Fatalf("got:\n%q\nwant:\n%q", got, strings.Join(want, "\n"))
		}
	}
}

func TestRenderViewBlanksAWideGraphemeItCuts(t *testing.T) {
	d := cligram.New()
	d.Node("a", "審查")
	l := d.Layout()
	// The box is "│   審查 │": 審 takes cells 4 and 5, 查 6 and 7.
	got := l.RenderView(cligram.State{}, cligram.Plain, cligram.Rect{X: 5, Y: 1, W: 4, H: 1})
	if got != " 查 " {
		t.Errorf("got %q", got)
	}
	got = l.RenderView(cligram.State{}, cligram.Plain, cligram.Rect{X: 0, Y: 1, W: 5, H: 1})
	if got != "│   " {
		t.Errorf("got %q", got)
	}
}

func TestTheFactoryLoopFitsATerminal(t *testing.T) {
	for _, sz := range []struct {
		name string
		w, h int
		fits bool
	}{
		{"200x50", 200, 50, true},
		{"160x50", 160, 50, true},
		{"80x24", 80, 24, false},
	} {
		l := factory().Layout(cligram.Fit(sz.w, sz.h))
		if sz.fits {
			noWarnings(t, l)
		} else {
			// Too small for the flow: the routes still hold, though a
			// label may find no room.
			cligram.CheckRoutes(t, l)
		}
		if l.Fits() != sz.fits {
			t.Errorf("%s: fits %v (%dx%d)", sz.name, l.Fits(), l.W, l.H)
		}
		out := l.Render(running, cligram.Plain)
		if !sz.fits {
			out = l.RenderView(running, cligram.Plain, l.Reveal(cligram.Rect{W: sz.w, H: sz.h}, "impl"))
		}
		cligram.Golden(t, "factory.fit"+sz.name+".golden", out+"\n")
	}
}

func BenchmarkFitFactoryLoopNothingFits(b *testing.B) {
	d := factory()
	for b.Loop() {
		d.Layout(cligram.Fit(80, 24))
	}
}

func BenchmarkFitFactoryLoopWraps(b *testing.B) {
	d := factory()
	for b.Loop() {
		d.Layout(cligram.Fit(200, 50))
	}
}
