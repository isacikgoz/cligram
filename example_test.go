package cligram_test

import (
	"fmt"

	"github.com/isacikgoz/cligram"
)

// A flow from steps and outcomes, no placements: laid out, routed, and
// painted with a run part way through it.
func Example() {
	d := cligram.New()
	d.Node("triage", "Triage")
	d.Node("ready", "Ready to ship?", cligram.As(cligram.Decision))
	d.Node("ship", "Ship it")
	d.Node("fix", "Fix it", cligram.Class("agent"))
	d.Edge("triage", "ready")
	d.Edge("ready", "ship", cligram.Label("yes"))
	d.Edge("ready", "fix", cligram.Label("no"))
	d.Edge("fix", "ready")

	l := d.Layout(cligram.Fit(80, 24))
	fmt.Println(l.Render(cligram.State{
		Status: map[string]cligram.Status{"triage": cligram.Done, "ready": cligram.Active},
		Taken:  []cligram.EdgeRef{{From: "triage", To: "ready"}},
	}, cligram.Plain))
	// Output:
	// ╭──────────╮    ╔══════════════════╗              ╭───────────╮
	// │ ✓ Triage ├───►║ ▸ Ready to ship? ╟─┬─[ yes ]───►│   Ship it │
	// ╰──────────╯    ╚══════════════════╝ │            ╰───────────╯
	//                          ▲           │
	//                          │           │
	//                          │           │            ╭──────────╮
	//                          │           └───[ no ]──►│   Fix it │
	//                          │                        ╰────┬─────╯
	//                          │                             │
	//                          └─────────────────────────────┘
}

// Placements say where a node goes relative to others; anything without
// one is placed by following the edges.
func Example_placements() {
	d := cligram.New()
	d.Node("hub", "Hub")
	d.Node("side", "Side", cligram.At("left of hub"))
	d.Node("below", "Below", cligram.At("below side and hub (gap: tight)"))
	fmt.Println(d.Layout().Render(cligram.State{}, cligram.Plain))
	// Output:
	// ╭────────╮    ╭───────╮
	// │   Side │    │   Hub │
	// ╰────────╯    ╰───────╯
	//
	//       ╭─────────╮
	//       │   Below │
	//       ╰─────────╯
}

// A host's theme: status colors for every kind of box, and a color for each
// class a diagram names, until the run reaches the box.
func ExamplePalette() {
	palette := cligram.Palette{
		Status:  cligram.DefaultPalette.Status, // done green, active cyan, failed red, waiting yellow
		Classes: map[string]string{"human": "magenta", "agent": "bright-blue"},
		Line:    "gray",
		Label:   "blue",
	}
	if err := palette.Check(); err != nil { // names every color it cannot read
		fmt.Println(err)
		return
	}
	theme := palette.Theme()
	fmt.Printf("%q\n", theme.Paint(cligram.Style{Part: cligram.PartBorder, Class: "human"}, "│"))
	fmt.Printf("%q\n", theme.Paint(cligram.Style{Part: cligram.PartBorder, Class: "human", Status: cligram.Done}, "│"))
	// Output:
	// "\x1b[35m│\x1b[0m"
	// "\x1b[32m│\x1b[0m"
}

// A layout too big for its room, shown a window at a time.
func ExampleLayout_RenderView() {
	d := cligram.New()
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		d.Node(id, "step "+id)
	}
	for _, e := range [][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}, {"d", "e"}, {"e", "f"}} {
		d.Edge(e[0], e[1])
	}
	l := d.Layout(cligram.Fit(30, 5))
	view := l.Reveal(cligram.Rect{W: 30, H: 5}, "f")
	fmt.Println(l.Fits(), view.Y > 0)
	// Output:
	// false true
}
