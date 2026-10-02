package cligram_test

// The harness draws random diagrams and reads every drawing back with
// internal/reader, which sees only the text a person would, and checks
// the picture says what the diagram says. TestDrawingsReadBack runs a
// fixed set of seeds on every test run; FuzzDrawings searches for more:
//
//	go test -run '^$' -fuzz FuzzDrawings -fuzztime 2m

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/internal/reader"
)

// A drawn case: a diagram, how to lay it out, and a run to paint.
type drawCase struct {
	d      *cligram.Diagram
	nodes  []genNode
	edges  []cligram.Edge // the ones that should be drawn, once each
	opts   []cligram.LayoutOption
	fit    *[2]int
	states [2]cligram.State
	view   cligram.Rect
}

type genNode struct {
	id, text string
	kind     cligram.Kind
	class    string
	sub      bool
}

var (
	words = []string{"triage", "review", "the", "spec", "ship", "agent", "runs", "human", "input", "verify",
		"deploy", "monitor", "issue", "create", "a", "very", "long", "step", "summary", "審查", "検証", "🚀",
		"supercalifragilisticexpialidocious", "x"}
	labelWords = []string{"yes", "no", "approved", "needs", "specs", "retry", "timeout", "answer >= 0.7",
		"else", "done", "failed", "declined", "replied", "ok", "審查", "a rather long outcome name"}
	relations = []string{"right of", "left of", "above", "below", "above-left of", "below-right of",
		"level with", "top level with", "left level with"}
	gaps = []string{"", "", "", " (gap: tight)", " (gap: wide)", " (gap: 2)"}
)

func pick[T any](rng *rand.Rand, s []T) T { return s[rng.IntN(len(s))] }

// generate makes a random case from seed.
func generate(seed uint64) drawCase {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	c := drawCase{d: cligram.New()}
	n := 1 + rng.IntN(14)
	ids := make([]string, n)
	for i := range n {
		ids[i] = fmt.Sprintf("n%d", i)
	}
	for i, id := range ids {
		g := genNode{id: id}
		var text []string
		for range rng.IntN(7) {
			text = append(text, pick(rng, words))
		}
		g.text = id
		if len(text) > 0 {
			sep := " "
			if rng.IntN(6) == 0 {
				sep = "\n"
			}
			g.text += sep + strings.Join(text, " ")
		}
		opts := []cligram.NodeOption{}
		switch rng.IntN(5) {
		case 0:
			g.kind = cligram.Decision
		case 1:
			g.kind = cligram.Terminal
		}
		opts = append(opts, cligram.As(g.kind))
		if rng.IntN(3) == 0 {
			g.class = pick(rng, []string{"human", "agent", "unknown"})
			opts = append(opts, cligram.Class(g.class))
		}
		if rng.IntN(8) == 0 {
			g.sub = true
			opts = append(opts, cligram.Sub(func(context.Context) (*cligram.Diagram, error) { return cligram.New(), nil }))
		}
		// Placements, now and then, some of them broken.
		if i > 0 && rng.IntN(4) == 0 {
			target := ids[rng.IntN(i)]
			switch rng.IntN(10) {
			case 0:
				target = "ghost"
			case 1:
				target = "left"
			}
			opts = append(opts, cligram.At(pick(rng, relations)+" "+target+pick(rng, gaps)))
		}
		c.d.Node(id, g.text, opts...)
		c.nodes = append(c.nodes, g)
	}
	seen := map[cligram.EdgeRef]bool{}
	edge := func(from, to string) {
		label := ""
		if rng.IntN(2) == 0 {
			label = pick(rng, labelWords)
		}
		e := cligram.Edge{From: from, To: to, Label: label}
		c.d.Edge(from, to, cligram.Label(label))
		if !seen[e.Ref()] {
			seen[e.Ref()] = true
			c.edges = append(c.edges, e)
		}
	}
	// Mostly a flow onward, with branches, loops back and the odd self-loop.
	for i := 1; i < n; i++ {
		edge(ids[rng.IntN(i)], ids[i])
	}
	for range rng.IntN(n + 1) {
		edge(pick(rng, ids), pick(rng, ids))
	}

	if rng.IntN(2) == 0 {
		c.opts = append(c.opts, cligram.WithOrientation(cligram.TopToBottom))
	}
	if rng.IntN(2) == 0 {
		c.fit = &[2]int{30 + rng.IntN(190), 10 + rng.IntN(70)}
		c.opts = append(c.opts, cligram.Fit(c.fit[0], c.fit[1]))
	}
	statuses := []cligram.Status{cligram.Idle, cligram.Active, cligram.Done, cligram.Failed, cligram.Waiting}
	for k := range c.states {
		st := cligram.State{Status: map[string]cligram.Status{}}
		for _, id := range ids {
			st.Status[id] = pick(rng, statuses)
		}
		for _, e := range c.edges {
			if rng.IntN(3) == 0 {
				st.Taken = append(st.Taken, e.Ref())
			}
		}
		st.Focus = pick(rng, ids)
		c.states[k] = st
	}
	c.view = cligram.Rect{X: rng.IntN(60) - 5, Y: rng.IntN(30) - 3, W: 1 + rng.IntN(80), H: 1 + rng.IntN(30)}
	return c
}

// The palette drawings are read with, and the escape codes it gives.
var harnessPalette = cligram.Palette{
	Status:  map[cligram.Status]string{cligram.Active: "cyan", cligram.Done: "green", cligram.Failed: "red", cligram.Waiting: "yellow"},
	Classes: map[string]string{"human": "magenta", "agent": "bright-blue"},
	Line:    "gray",
	Label:   "blue",
}

var (
	statusSGR = map[cligram.Status]string{cligram.Active: "36", cligram.Done: "32", cligram.Failed: "31", cligram.Waiting: "33"}
	classSGR  = map[string]string{"human": "35", "agent": "94"}
	markers   = map[cligram.Status]string{cligram.Idle: " ", cligram.Active: "▸", cligram.Done: "✓", cligram.Failed: "✗", cligram.Waiting: "◔"}
	kinds     = map[cligram.Kind]reader.Kind{cligram.Step: reader.Step, cligram.Decision: reader.Decision, cligram.Terminal: reader.Terminal}
)

// checkCase lays c out, paints it, reads it back and checks it.
func checkCase(t *testing.T, c drawCase) {
	t.Helper()
	l := c.d.Layout(c.opts...)
	theme := harnessPalette.Theme()
	text := l.Render(c.states[0], theme)

	// The same diagram lays out the same, every time.
	if again := c.d.Layout(c.opts...).Render(c.states[0], theme); again != text {
		t.Fatal("laying out twice drew two pictures")
	}
	var warnings []string
	for _, w := range l.Warnings() {
		warnings = append(warnings, w.Error())
		if strings.Contains(w.Error(), "no way through") {
			t.Errorf("an edge could not be routed: %v", w)
		}
	}
	if slices.ContainsFunc(warnings, func(w string) bool { return strings.Contains(w, "overlap") }) {
		return // the placements pinned two boxes on each other, and said so
	}

	pic, err := reader.Read(text)
	if err != nil {
		t.Fatalf("the drawing cannot be read: %v\n%s", err, stripped(l, c.states[0]))
	}
	byID := checkBoxes(t, c, pic, l)
	checkEdges(t, c, pic, byID, warnings, l)

	// Painting another run moves nothing.
	other, err := reader.Read(l.Render(c.states[1], theme))
	if err != nil {
		t.Fatalf("the drawing in another state cannot be read: %v", err)
	}
	for i, b := range other.Boxes {
		if a := pic.Boxes[i]; a.X != b.X || a.Y != b.Y || a.W != b.W || a.H != b.H {
			t.Errorf("a box moved when the state changed: %+v to %+v", a, b)
		}
	}

	// Fit is honored when it says it fits.
	if c.fit != nil && l.Fits() {
		if l.W > c.fit[0] || l.H > c.fit[1] {
			t.Errorf("fits %dx%d but is %dx%d", c.fit[0], c.fit[1], l.W, l.H)
		}
		for _, w := range warnings {
			if strings.Contains(w, "no room for its label") {
				t.Errorf("fits, yet lost a label: %s", w)
			}
		}
	}

	// A view is the same cells as the whole picture.
	checkView(t, l, c)
}

// checkBoxes checks every node is drawn once, as what it is, and maps the
// boxes to node ids by the id each text starts with.
func checkBoxes(t *testing.T, c drawCase, pic *reader.Picture, l *cligram.Layout) map[int]string {
	t.Helper()
	byID := map[int]string{}
	drawn := map[string]bool{}
	for i, b := range pic.Boxes {
		id := ""
		if len(b.Lines) > 0 {
			if f := strings.Fields(b.Lines[0]); len(f) > 0 {
				id = strings.TrimSuffix(f[0], "…")
			}
		}
		if drawn[id] {
			t.Errorf("node %s is drawn twice", id)
		}
		drawn[id] = true
		byID[i] = id
	}
	st := c.states[0]
	for _, g := range c.nodes {
		var b *reader.Box
		for i := range pic.Boxes {
			if byID[i] == g.id {
				b = &pic.Boxes[i]
			}
		}
		if b == nil {
			t.Errorf("node %s is not drawn\n%s", g.id, stripped(l, st))
			continue
		}
		if b.Kind != kinds[g.kind] || b.Stacked != g.sub {
			t.Errorf("node %s: drawn as kind %v stacked %v, is %v sub %v", g.id, b.Kind, b.Stacked, g.kind, g.sub)
		}
		if !sameText(b.Text(), g.text) {
			t.Errorf("node %s reads %q, is %q", g.id, b.Text(), g.text)
		}
		status := st.Status[g.id]
		if b.Marker != markers[status] {
			t.Errorf("node %s is %v but shows %q", g.id, status, b.Marker)
		}
		want := statusSGR[status]
		if want == "" {
			want = classSGR[g.class]
		}
		if st.Focus == g.id {
			want = strings.TrimSuffix("1;"+want, ";")
		}
		if b.BorderSGR != want {
			t.Errorf("node %s (%v, class %q, focus %v): border drawn %q, want %q", g.id, status, g.class, st.Focus == g.id, b.BorderSGR, want)
		}
		if b.MarkerSGR != statusSGR[status] {
			t.Errorf("node %s: marker drawn %q, want %q", g.id, b.MarkerSGR, statusSGR[status])
		}
	}
	return byID
}

// sameText reports whether what a box reads is its text, wrapped, and cut
// with an ellipsis where it ran out of room.
func sameText(read, text string) bool {
	squash := func(s string) string { return strings.Join(strings.Fields(s), "") }
	r, full := squash(read), squash(text)
	if cut, ok := strings.CutSuffix(r, "…"); ok {
		return strings.HasPrefix(full, cut)
	}
	return r == full
}

// checkEdges checks every edge is drawn once, from its box to its box,
// with its label on its own line and its arrowhead in the run's color.
func checkEdges(t *testing.T, c drawCase, pic *reader.Picture, byID map[int]string, warnings []string, l *cligram.Layout) {
	t.Helper()
	taken := map[cligram.EdgeRef]bool{}
	for _, ref := range c.states[0].Taken {
		taken[ref] = true
	}
	type key struct{ from, to string }
	want := map[key][]cligram.Edge{}
	for _, e := range c.edges {
		want[key{e.From, e.To}] = append(want[key{e.From, e.To}], e)
	}
	got := map[key][]reader.Edge{}
	for _, e := range pic.Edges {
		k := key{byID[e.From], byID[e.To]}
		got[k] = append(got[k], e)
	}
	for k, es := range want {
		gs := got[k]
		if len(gs) != len(es) {
			t.Errorf("%s -> %s: drawn %d times, is in the diagram %d times\n%s", k.from, k.to, len(gs), len(es), stripped(l, c.states[0]))
			continue
		}
		// Match labels: each read label to an edge whose label it is.
		used := make([]bool, len(gs))
		for _, e := range es {
			lost := slices.ContainsFunc(warnings, func(w string) bool {
				return strings.Contains(w, fmt.Sprintf("edge %s -> %s %q has no room", e.From, e.To, e.Label))
			})
			match := -1
			for i, g := range gs {
				if used[i] {
					continue
				}
				if (g.Label == "" && (e.Label == "" || lost)) || (g.Label != "" && sameLabel(g.Label, e.Label)) {
					match = i
					if g.Label != "" || e.Label == "" {
						break
					}
				}
			}
			if match < 0 {
				t.Errorf("%s -> %s %q: not drawn with its label; drawn with %v", e.From, e.To, e.Label, labels(gs))
				continue
			}
			used[match] = true
			wantSGR := "90"
			if taken[e.Ref()] {
				wantSGR = "32"
			}
			// Edges between the same boxes may have their arrowheads drawn
			// over by each other's color only if they share a cell; they do
			// not, so each is its own.
			if gs[match].ArrowSGR != wantSGR && len(gs) == 1 {
				t.Errorf("%s -> %s %q: arrowhead drawn %q, want %q", e.From, e.To, e.Label, gs[match].ArrowSGR, wantSGR)
			}
		}
	}
	for k, gs := range got {
		if len(want[k]) == 0 {
			t.Errorf("%s -> %s is drawn %d times but is not in the diagram", k.from, k.to, len(gs))
		}
	}
}

func sameLabel(read, label string) bool {
	if cut, ok := strings.CutSuffix(read, "…"); ok {
		return strings.HasPrefix(label, strings.TrimRight(cut, " "))
	}
	return read == label
}

func labels(gs []reader.Edge) []string {
	var out []string
	for _, g := range gs {
		out = append(out, g.Label)
	}
	return out
}

// checkView checks a view shows the cells of the whole picture it covers.
func checkView(t *testing.T, l *cligram.Layout, c drawCase) {
	t.Helper()
	full := reader.Parse(l.Render(c.states[0], cligram.Plain))
	view := l.RenderView(c.states[0], cligram.Plain, c.view)
	rows := strings.Split(view, "\n")
	if len(rows) != c.view.H {
		t.Fatalf("view %+v has %d rows", c.view, len(rows))
	}
	for y := range c.view.H {
		var want strings.Builder
		for x := c.view.X; x < c.view.X+c.view.W; x++ {
			fy := c.view.Y + y
			var cell reader.Cell
			if fy >= 0 && fy < full.H && x >= 0 && x < len(full.Cells[fy]) {
				cell = full.Cells[fy][x]
			} else {
				cell = reader.Cell{G: " "}
			}
			switch {
			case cell.Cont && x == c.view.X:
				want.WriteString(" ") // its left half is outside
			case cell.Cont:
			case uniWide(full, x, fy) && x == c.view.X+c.view.W-1:
				want.WriteString(" ") // its right half is outside
			default:
				want.WriteString(cell.G)
			}
		}
		if got, w := strings.TrimRight(rows[y], " "), strings.TrimRight(want.String(), " "); got != w {
			t.Fatalf("view %+v row %d: %q, the picture has %q", c.view, y, got, w)
		}
	}
}

func uniWide(g *reader.Grid, x, y int) bool {
	return y >= 0 && y < g.H && x+1 >= 0 && x+1 < len(g.Cells[y]) && g.Cells[y][x+1].Cont
}

// stripped is the picture in plain text, for a failure to show.
func stripped(l *cligram.Layout, st cligram.State) string {
	return l.Render(st, cligram.Plain)
}

func TestDrawingsReadBack(t *testing.T) {
	// The race detector slows layout tenfold; a few seeds find what it
	// looks for, and FuzzDrawings covers the breadth.
	seeds := 400
	if testing.Short() || raceEnabled {
		seeds = 60
	}
	for seed := range uint64(seeds) {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			checkCase(t, generate(seed))
		})
	}
}

func FuzzDrawings(f *testing.F) {
	for seed := range uint64(8) {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkCase(t, generate(seed))
	})
}
