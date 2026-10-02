package cligram_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
)

// factory is the whole factory loop, written as ackt would: steps in the
// order a reader follows them, edges by outcome, no placements.
func factory() *cligram.Diagram {
	d := cligram.New()
	node := func(id, text string, k cligram.Kind) { d.Node(id, text, cligram.As(k)) }
	node("new", "New task or issue", cligram.Terminal)
	node("triage", "Triage agent runs", cligram.Step)
	node("outcome", "Triage\noutcome", cligram.Decision)
	node("impl", "Implementation\nagent runs", cligram.Step)
	node("review", "Code review\nagent runs", cligram.Step)
	node("verify", "Verification\nagent runs", cligram.Step)
	node("humanreview", "Human reviews code\nand verification output", cligram.Step)
	node("ready", "Ready to ship?", cligram.Decision)
	node("cicd", "CI/CD", cligram.Step)
	node("ship", "Ship it", cligram.Step)
	node("monitor", "Monitoring\nagent runs", cligram.Step)
	node("issue", "Issue\ndetected", cligram.Decision)
	node("create", "Create issue", cligram.Step)
	node("loop", "Factory loop\ncontinues", cligram.Step)
	node("continue", "Continue\nmonitoring", cligram.Terminal)
	node("spec", "Spec agent runs", cligram.Step)
	node("specreview", "Human review specs", cligram.Step)
	node("human", "Human provides input", cligram.Step)
	node("park", "Park issue for now", cligram.Terminal)

	edge := func(from, to, label string) { d.Edge(from, to, cligram.Label(label)) }
	edge("new", "triage", "")
	edge("triage", "outcome", "")
	edge("outcome", "impl", "Automat-able")
	edge("outcome", "spec", "Needs specs")
	edge("outcome", "human", "Needs human clarification")
	edge("outcome", "park", "Park for now")
	edge("spec", "specreview", "")
	edge("specreview", "spec", "Needs revision")
	edge("specreview", "impl", "Approved")
	edge("human", "triage", "")
	edge("impl", "review", "")
	edge("review", "verify", "")
	edge("verify", "humanreview", "")
	edge("humanreview", "ready", "")
	edge("ready", "impl", "Not ready")
	edge("ready", "cicd", "Approved")
	edge("cicd", "ship", "")
	edge("ship", "monitor", "")
	edge("monitor", "issue", "")
	edge("issue", "create", "Yes - create issue")
	edge("issue", "continue", "No")
	edge("create", "loop", "")
	edge("loop", "new", "")
	return d
}

func TestFactoryLoopLaysOutUnplaced(t *testing.T) {
	d := factory()
	l := d.Layout()
	noWarnings(t, l)
	noOverlaps(t, d, l)
	// The flow reads left to right along its first ways on.
	row := []string{"new", "triage", "outcome", "impl", "review", "verify", "humanreview", "ready", "cicd"}
	for i := 1; i < len(row); i++ {
		a, b := rect(t, l, row[i-1]), rect(t, l, row[i])
		if b.X <= a.X+a.W || centerY(a) != centerY(b) && b.H == a.H {
			t.Errorf("%s %+v does not follow %s %+v along the row", row[i], b, row[i-1], a)
		}
	}
	// The other ways on stack in a column below the first.
	impl := rect(t, l, "impl")
	for _, id := range []string{"spec", "human", "park"} {
		if r := rect(t, l, id); r.X != impl.X || r.Y <= impl.Y {
			t.Errorf("%s %+v is not in impl's column below it %+v", id, r, impl)
		}
	}
}

// ring is the factory loop placed by hand into the ring of the original
// drawing: across the top, down the right, back along the bottom.
func ring() *cligram.Diagram {
	d := factory()
	hints := map[string]string{
		"review":      "right of impl",
		"verify":      "below review",
		"humanreview": "below verify",
		"ready":       "below humanreview",
		"cicd":        "below ready",
		"ship":        "left of cicd",
		"monitor":     "left of ship",
		"issue":       "left of monitor",
		"create":      "above-left of issue",
		"continue":    "below-left of issue",
		"loop":        "left of create, below new",
	}
	r := cligram.New()
	for _, n := range d.Nodes() {
		opts := []cligram.NodeOption{cligram.As(n.Kind)}
		if h, ok := hints[n.ID]; ok {
			opts = append(opts, cligram.At(h))
		}
		r.Node(n.ID, n.Text, opts...)
	}
	for _, e := range d.Edges() {
		r.Edge(e.From, e.To, cligram.Label(e.Label))
	}
	return r
}

func TestFactoryLoopLaysOutAsARing(t *testing.T) {
	d := ring()
	l := d.Layout()
	noWarnings(t, l)
	noOverlaps(t, d, l)
	review, ready, cicd := rect(t, l, "review"), rect(t, l, "ready"), rect(t, l, "cicd")
	ship, monitor, issue := rect(t, l, "ship"), rect(t, l, "monitor"), rect(t, l, "issue")
	create, loop, nw := rect(t, l, "create"), rect(t, l, "loop"), rect(t, l, "new")
	down := ready.Y > review.Y && cicd.Y > ready.Y
	back := ship.X+ship.W+4 == cicd.X && monitor.X+monitor.W+4 == ship.X && issue.X+issue.W+4 == monitor.X
	if !down {
		t.Errorf("the right side does not go down: %+v %+v %+v", review, ready, cicd)
	}
	if !back {
		t.Errorf("the bottom does not come back right to left, a gap apart: %+v %+v %+v %+v", issue, monitor, ship, cicd)
	}
	if create.X+create.W >= issue.X || create.Y+create.H >= issue.Y {
		t.Errorf("create %+v is not above-left of issue %+v", create, issue)
	}
	if loop.X+loop.W+4 != create.X || loop.Y <= nw.Y+nw.H {
		t.Errorf("loop %+v is not left of create %+v and below new %+v", loop, create, nw)
	}
}

func noWarnings(t *testing.T, l *cligram.Layout) {
	t.Helper()
	cligram.CheckRoutes(t, l)
	for _, w := range l.Warnings() {
		t.Error(w)
	}
}

func noOverlaps(t *testing.T, d *cligram.Diagram, l *cligram.Layout) {
	t.Helper()
	nodes := d.Nodes()
	for i, a := range nodes {
		for _, b := range nodes[i+1:] {
			ra, rb := rect(t, l, a.ID), rect(t, l, b.ID)
			if ra.X < rb.X+rb.W && rb.X < ra.X+ra.W && ra.Y < rb.Y+rb.H && rb.Y < ra.Y+ra.H {
				t.Errorf("%s %+v overlaps %s %+v", a.ID, ra, b.ID, rb)
			}
		}
	}
	for _, n := range nodes {
		if r := rect(t, l, n.ID); r.X < 0 || r.Y < 0 || r.X+r.W > l.W || r.Y+r.H > l.H {
			t.Errorf("%s %+v is outside the %dx%d picture", n.ID, r, l.W, l.H)
		}
	}
}

func rect(t *testing.T, l *cligram.Layout, id string) cligram.Rect {
	t.Helper()
	r, ok := l.Rect(id)
	if !ok {
		t.Fatalf("no node %q", id)
	}
	return r
}

func centerY(r cligram.Rect) int { return 2*r.Y + r.H }

// diagram builds a diagram from lines of "id placement...", every node
// one letter of text so each box is 7 by 3, and edges as "a -> b label".
func diagram(lines ...string) *cligram.Diagram {
	d := cligram.New()
	for _, line := range lines {
		if from, rest, ok := strings.Cut(line, " -> "); ok {
			to, label, _ := strings.Cut(rest, " ")
			d.Edge(from, to, cligram.Label(label))
			continue
		}
		id, place, _ := strings.Cut(line, " ")
		var opts []cligram.NodeOption
		if place != "" {
			opts = append(opts, cligram.At(place))
		}
		d.Node(id, "X", opts...)
	}
	return d
}

func at(t *testing.T, l *cligram.Layout, want map[string]cligram.Rect) {
	t.Helper()
	var got []string
	ok := true
	for id, w := range want {
		r := rect(t, l, id)
		got = append(got, fmt.Sprintf("%s %+v", id, r))
		ok = ok && r == w
	}
	if !ok {
		t.Errorf("got %s\nwant %+v", strings.Join(got, ", "), want)
	}
}

func TestAChainReadsAcross(t *testing.T) {
	l := diagram("a", "b", "c", "a -> b", "b -> c").Layout()
	noWarnings(t, l)
	at(t, l, map[string]cligram.Rect{"a": {0, 0, 7, 3}, "b": {11, 0, 7, 3}, "c": {22, 0, 7, 3}})
	if l.W != 29 || l.H != 3 {
		t.Errorf("picture %dx%d", l.W, l.H)
	}
}

func TestAChainReadsDownTopToBottom(t *testing.T) {
	l := diagram("a", "b", "a -> b").Layout(cligram.WithOrientation(cligram.TopToBottom))
	at(t, l, map[string]cligram.Rect{"a": {0, 0, 7, 3}, "b": {0, 5, 7, 3}})
}

func TestWaysOnStackBesideTheirStep(t *testing.T) {
	l := diagram("p", "a", "b", "c", "p -> a", "p -> b", "p -> c").Layout()
	noWarnings(t, l)
	at(t, l, map[string]cligram.Rect{
		"p": {0, 0, 7, 3}, "a": {11, 0, 7, 3}, "b": {11, 5, 7, 3}, "c": {11, 10, 7, 3},
	})
}

func TestASiblingClearsWhatTheOneBeforeItLedTo(t *testing.T) {
	l := diagram("p", "a", "a2", "a3", "b", "p -> a", "a -> a2", "a -> a3", "p -> b").Layout()
	noWarnings(t, l)
	if a3, b := rect(t, l, "a3"), rect(t, l, "b"); b.Y < a3.Y+a3.H+2 {
		t.Errorf("b %+v is not below a's subtree, which reaches %+v", b, a3)
	}
}

func TestALabelWidensTheGapItSitsIn(t *testing.T) {
	l := diagram("a", "b", "a -> b Approved").Layout()
	// Room for a trunk to branch, three cells of line, "[ Approved ]", two
	// more and the arrowhead.
	at(t, l, map[string]cligram.Rect{"a": {0, 0, 7, 3}, "b": {7 + 2 + 3 + 12 + 2, 0, 7, 3}})
}

func TestAnotherRootGoesBelowEverythingBefore(t *testing.T) {
	l := diagram("a", "b", "c", "a -> b").Layout()
	at(t, l, map[string]cligram.Rect{"a": {0, 0, 7, 3}, "b": {11, 0, 7, 3}, "c": {0, 5, 7, 3}})
}

func TestPlacementsPutNodesWhereTheySay(t *testing.T) {
	l := diagram("a", "r right of a", "l left of a", "u above a", "d below a").Layout()
	noWarnings(t, l)
	at(t, l, map[string]cligram.Rect{
		"l": {0, 5, 7, 3}, "u": {11, 0, 7, 3}, "a": {11, 5, 7, 3}, "r": {22, 5, 7, 3}, "d": {11, 10, 7, 3},
	})
}

func TestADiagonalSitsAtItsCorner(t *testing.T) {
	l := diagram("a", "b above-left of a", "c below-right of a (gap: tight)").Layout()
	noWarnings(t, l)
	at(t, l, map[string]cligram.Rect{"b": {0, 0, 7, 3}, "a": {11, 5, 7, 3}, "c": {20, 9, 7, 3}})
}

func TestANodeWedgedBetweenTwoPushesThemApart(t *testing.T) {
	l := diagram("hub", "side left of hub", "wedge right of side left of hub level with hub").Layout()
	noWarnings(t, l)
	at(t, l, map[string]cligram.Rect{"side": {0, 0, 7, 3}, "wedge": {11, 0, 7, 3}, "hub": {22, 0, 7, 3}})
}

func TestSeveralTargetsCenterOnTheirRegion(t *testing.T) {
	l := diagram("a", "b right of a", "c right of b", "d below a and b", "e below b and c").Layout()
	noWarnings(t, l)
	at(t, l, map[string]cligram.Rect{"d": {5, 5, 7, 3}, "e": {16, 5, 7, 3}})
}

func TestAlignmentsShareALine(t *testing.T) {
	d := cligram.New()
	d.Node("tall", "1\n2\n3")
	d.Node("up", "", cligram.At("right of tall top level with tall"))
	d.Node("down", "", cligram.At("right of up bottom level with tall"))
	d.Node("wide", "a wide one", cligram.At("below tall left level with tall"))
	d.Node("flush", "", cligram.At("below wide right level with wide"))
	l := d.Layout()
	noWarnings(t, l)
	tall, up, down := rect(t, l, "tall"), rect(t, l, "up"), rect(t, l, "down")
	wide, flush := rect(t, l, "wide"), rect(t, l, "flush")
	if up.Y != tall.Y || down.Y+down.H != tall.Y+tall.H || wide.X != tall.X || flush.X+flush.W != wide.X+wide.W {
		t.Errorf("tall %+v up %+v down %+v wide %+v flush %+v", tall, up, down, wide, flush)
	}
}

func TestNodesPlacedAlikeAreKeptApart(t *testing.T) {
	d := diagram("a", "b right of a", "c right of a")
	l := d.Layout()
	noWarnings(t, l)
	noOverlaps(t, d, l)
	if b, c := rect(t, l, "b"), rect(t, l, "c"); b.X != c.X {
		t.Errorf("b %+v and c %+v are not stacked", b, c)
	}
}

func TestPlacementsThatCannotHoldAreLeftOutWithAWarning(t *testing.T) {
	d := diagram("a", "b right of a", "c right of b", "a2 left of a level with c", "c2 right of c (gap: none) left of a")
	l := d.Layout()
	noOverlaps(t, d, l)
	var said []string
	for _, w := range l.Warnings() {
		said = append(said, w.Error())
	}
	if len(said) != 1 || !strings.Contains(said[0], `node "c2": "left of a" cannot hold`) {
		t.Errorf("warnings: %q", said)
	}
}

func TestMistakesWarnAndTheRestLaysOut(t *testing.T) {
	d := diagram("a", "b right of ghost", "c right of", "a -> b", "a -> c")
	l := d.Layout()
	if len(l.Warnings()) != 2 {
		t.Errorf("warnings: %v", l.Warnings())
	}
	noOverlaps(t, d, l)
	// With no placement left, they are placed as ways on from a.
	at(t, l, map[string]cligram.Rect{"a": {0, 0, 7, 3}, "b": {11, 0, 7, 3}, "c": {11, 5, 7, 3}})
}

func TestRenderPaintsTheStateWithoutMovingABox(t *testing.T) {
	l := diagram("a", "b", "a -> b").Layout()
	idle := l.Render(cligram.State{}, cligram.Plain)
	running := l.Render(cligram.State{
		Status: map[string]cligram.Status{"a": cligram.Done, "b": cligram.Active},
		Taken:  []cligram.EdgeRef{{From: "a", To: "b"}},
		Focus:  "b",
	}, cligram.Plain)
	want := []string{
		"╭─────╮    ╭─────╮",
		"│ %s X ├───▸│ %s X │",
		"╰─────╯    ╰─────╯",
	}
	if w := fmt.Sprintf(strings.Join(want, "\n"), " ", " "); idle != w {
		t.Errorf("idle:\n%s\nwant:\n%s", idle, w)
	}
	if w := fmt.Sprintf(strings.Join(want, "\n"), "✓", "▸"); running != w {
		t.Errorf("running:\n%s\nwant:\n%s", running, w)
	}
}

func TestAnAlignmentWithSeveralTargetsUsesTheirRegion(t *testing.T) {
	d := cligram.New()
	d.Node("a", "")
	d.Node("b", "1\n2\n3", cligram.At("right of a (gap: tight)"))
	d.Node("c", "", cligram.At("right of b top level with a and b"))
	d.Node("e", "", cligram.At("right of c bottom level with a and b"))
	d.Node("f", "", cligram.At("right of e level with a and b"))
	l := d.Layout()
	noWarnings(t, l)
	a, b := rect(t, l, "a"), rect(t, l, "b")
	top, bottom := min(a.Y, b.Y), max(a.Y+a.H, b.Y+b.H)
	if c := rect(t, l, "c"); c.Y != top {
		t.Errorf("c %+v is not level with the top of a %+v and b %+v", c, a, b)
	}
	if e := rect(t, l, "e"); e.Y+e.H != bottom {
		t.Errorf("e %+v is not level with the bottom of a %+v and b %+v", e, a, b)
	}
	if f := rect(t, l, "f"); centerY(f) != top+bottom {
		t.Errorf("f %+v is not centered on a %+v and b %+v", f, a, b)
	}
}

func TestNodesPinnedOnEachOtherOverlapWithAWarning(t *testing.T) {
	d := diagram("a", "b level with a left level with a")
	l := d.Layout()
	if w := l.Warnings(); len(w) != 1 || !strings.Contains(w[0].Error(), `nodes "a" and "b" overlap`) {
		t.Errorf("warnings: %v", w)
	}
}

func TestGlyphsChooseHowItIsDrawn(t *testing.T) {
	got := diagram("a").Layout(cligram.WithGlyphs(cligram.ASCII)).Render(cligram.State{}, cligram.Plain)
	if want := "+-----+\n|   X |\n+-----+"; got != want {
		t.Errorf("got:\n%s", got)
	}
}

func BenchmarkLayoutFactoryLoop(b *testing.B) {
	d := factory()
	for b.Loop() {
		d.Layout()
	}
}

func BenchmarkRenderFactoryLoop(b *testing.B) {
	l := factory().Layout()
	st := cligram.State{Status: map[string]cligram.Status{"impl": cligram.Active}}
	for b.Loop() {
		l.Render(st, cligram.ANSI)
	}
}

// running is a run that went through triage and is implementing.
var running = cligram.State{
	Status: map[string]cligram.Status{
		"new": cligram.Done, "triage": cligram.Done, "outcome": cligram.Done, "impl": cligram.Active,
	},
	Taken: []cligram.EdgeRef{
		{From: "new", To: "triage"}, {From: "triage", To: "outcome"},
		{From: "outcome", To: "impl", Label: "Automat-able"},
	},
	Focus: "impl",
}

func TestTheFactoryLoopDraws(t *testing.T) {
	for name, d := range map[string]*cligram.Diagram{"factory": factory(), "ring": ring()} {
		l := d.Layout()
		noWarnings(t, l)
		cligram.Golden(t, name+".golden", l.Render(running, cligram.Plain)+"\n")
		cligram.Golden(t, name+".ansi.golden", l.Render(running, cligram.ANSI)+"\n")
	}
	l := factory().Layout(cligram.WithGlyphs(cligram.ASCII))
	cligram.Golden(t, "factory.ascii.golden", l.Render(running, cligram.Plain)+"\n")
	l = factory().Layout(cligram.WithOrientation(cligram.TopToBottom))
	noWarnings(t, l)
	cligram.Golden(t, "factory.down.golden", l.Render(running, cligram.Plain)+"\n")
}
