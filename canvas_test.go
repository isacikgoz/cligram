package cligram

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// golden compares got with testdata/name, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to write it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs; got:\n%s\nwant:\n%s", name, got, want)
	}
}

type sceneNode struct {
	at      Point
	kind    Kind
	stacked bool
	lines   []string
	status  Status
	focus   bool
}

// paintScene paints the top of the factory loop by hand, as the layout
// will: a run has gone through triage and is implementing.
func paintScene(g *Glyphs) *canvas {
	nodes := map[string]sceneNode{
		"new":     {at: Point{0, 0}, kind: Terminal, lines: []string{"New task or issue"}, status: Done},
		"triage":  {at: Point{27, 0}, lines: []string{"Triage", "agent runs"}, status: Done},
		"outcome": {at: Point{47, 0}, kind: Decision, lines: []string{"Triage", "outcome"}, status: Done},
		"impl":    {at: Point{82, 0}, lines: []string{"Implement", "agent runs"}, status: Active, focus: true},
		"human":   {at: Point{82, 5}, lines: []string{"Human input"}},
		"park":    {at: Point{82, 9}, lines: []string{"Park"}, stacked: true},
	}
	c := newCanvas(100, 13, g)
	for _, n := range nodes {
		w, h := boxSize(n.lines, n.stacked)
		c.box(Rect{n.at.X, n.at.Y, w, h}, n.stacked, n.lines, Style{Kind: n.kind, Status: n.status, Focus: n.focus})
	}
	taken, idle := Style{Status: Done}, Style{}
	c.path([]Point{{22, 1}, {26, 1}}, 1, taken, true)
	c.path([]Point{{42, 1}, {46, 1}}, 2, taken, true)
	c.path([]Point{{59, 1}, {81, 1}}, 3, taken, true)
	c.path([]Point{{53, 3}, {53, 6}, {81, 6}}, 3, idle, true)
	c.path([]Point{{53, 3}, {53, 10}, {81, 10}}, 3, idle, true)
	// Back to triage, crossing the decision's trunk.
	c.path([]Point{{90, 7}, {90, 8}, {34, 8}, {34, 4}}, 4, idle, true)
	c.label(63, 1, "Automat-able", taken)
	c.label(57, 6, "Needs human", idle)
	c.label(57, 10, "Park for now", idle)
	return c
}

func TestPaintTheFactoryLoop(t *testing.T) {
	golden(t, "scene.unicode.golden", paintScene(Unicode).render(Plain)+"\n")
	golden(t, "scene.ascii.golden", paintScene(ASCII).render(Plain)+"\n")
	golden(t, "scene.ansi.golden", paintScene(Unicode).render(ANSI)+"\n")
}

func TestEveryGlyphIsOneCellWide(t *testing.T) {
	for name, g := range map[string]*Glyphs{"unicode": Unicode, "ascii": ASCII} {
		all := append([]string{g.LabelOpen, g.LabelClose}, g.Arrows[:]...)
		all = append(all, g.Lines[1:]...)
		for _, m := range g.Markers {
			all = append(all, m)
		}
		for _, b := range g.Boxes {
			all = append(all, b.TopLeft, b.TopRight, b.BottomLeft, b.BottomRight, b.Horizontal, b.Vertical,
				b.OutTop, b.OutBottom, b.OutLeft, b.OutRight)
		}
		for _, s := range all {
			if s == g.LabelOpen || s == g.LabelClose {
				continue
			}
			if w := textWidth(s); w != 1 {
				t.Errorf("%s: %q is %d cells wide", name, s, w)
			}
		}
	}
}

func TestWideTextKeepsTheBoxAligned(t *testing.T) {
	lines := []string{"審查 code", "👩‍💻 by hand"}
	w, h := boxSize(lines, false)
	c := newCanvas(w+2, h, Unicode)
	c.box(Rect{0, 0, w, h}, false, lines, Style{})
	// A wide grapheme cut by the edge of the canvas is left out, not halved.
	c.text(w, 1, "審", Style{}, 2)
	c.text(w+1, 2, "審", Style{}, 2)
	got := c.render(Plain)
	for i, row := range strings.Split(got, "\n") {
		if row == "" {
			continue
		}
		if tw := textWidth(strings.TrimRight(row, " ")); i != 2 && tw != w && tw != w+2 {
			t.Errorf("row %d is %d cells wide, want %d:\n%s", i, tw, w, got)
		}
	}
	golden(t, "wide.golden", got+"\n")
}

func TestOverwritingHalfAWideGraphemeClearsIt(t *testing.T) {
	c := newCanvas(4, 1, Unicode)
	c.text(0, 0, "審審", Style{}, 4)
	c.put(1, 0, "x", 1, Style{})
	c.put(2, 0, "y", 1, Style{})
	if got := c.render(Plain); got != " xy" {
		t.Errorf("got %q", got)
	}
}

func TestLinesOfOneGroupJoinAndOthersCross(t *testing.T) {
	c := newCanvas(7, 5, Unicode)
	c.path([]Point{{3, 0}, {3, 4}}, 1, Style{}, false)
	c.path([]Point{{3, 2}, {6, 2}}, 1, Style{}, false)
	c.path([]Point{{0, 3}, {6, 3}}, 2, Style{}, false)
	want := strings.Join([]string{
		"   │",
		"   │",
		"   ├───",
		"───────",
		"   │",
	}, "\n")
	if got := c.render(Plain); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
