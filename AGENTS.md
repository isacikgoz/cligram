# cligram

cligram draws flow diagrams in the terminal, from Go: boxes, decisions and
labelled edges, laid out to fit the terminal, repainted live as a run moves
through them, and navigable from the keyboard. Think Mermaid for the
terminal, with [reladraw](https://github.com/reladraw/reladraw)-style
relative placement when you want to say where things go.

- `github.com/isacikgoz/cligram`: the diagram, layout, routing and painting.
  Its only dependency is `rivo/uniseg` (text width).
- `github.com/isacikgoz/cligram/bubble`: a Bubble Tea component on top.
- `github.com/isacikgoz/cligram/yaml`: reads a diagram from YAML, with
  errors that give a line; `cmd/cligram` draws a YAML file in the
  terminal. The README's examples are checked by `cmd/cligram`'s tests.
- `docs/design.md`: every design decision and why. Read it before changing
  behaviour.
- `examples/live`: the factory loop with a pretend run. `go run ./examples/live`.

## Using it

```go
d := cligram.New()
d.Node("triage", "Triage")
d.Node("ready", "Ready to ship?", cligram.As(cligram.Decision))
d.Node("ship", "Ship it")
d.Node("fix", "Fix it", cligram.Class("agent"))
d.Edge("triage", "ready")
d.Edge("ready", "ship", cligram.Label("yes"))
d.Edge("ready", "fix", cligram.Label("no"))
d.Edge("fix", "ready")

l := d.Layout(cligram.Fit(80, 24))          // once per diagram and size
fmt.Println(l.Render(cligram.State{         // as often as the run moves
    Status: map[string]cligram.Status{"triage": cligram.Done, "ready": cligram.Active},
    Taken:  []cligram.EdgeRef{{From: "triage", To: "ready"}},
}, cligram.ANSI))
```

```
╭──────────╮    ╔══════════════════╗              ╭───────────╮
│ ✓ Triage ├───▸║ ▸ Ready to ship? ╟─┬─[ yes ]───▸│   Ship it │
╰──────────╯    ╚══════════════════╝ │            ╰───────────╯
                                  ▴  │
                                  │  │
                                  │  │            ╭──────────╮
                                  │  └───[ no ]──▸│   Fix it │
                                  │               ╰┬─────────╯
                                  └────────────────┘
```

This is `Example` in `example_test.go`; `go test` checks the drawing. The
snippets below are compiled there and in `bubble/example_test.go` too.

### The pieces

| Piece | What it is |
|---|---|
| `Diagram` | Nodes and edges, built in any order. Mistakes are collected, not returned per call: `d.Check()` lists them all. |
| `Node(id, text, opts...)` | `As(Decision)` / `As(Terminal)` (default `Step`), `At("right of x")`, `Class("human")`, `Sub(loader)`. Text breaks on `"\n"`; empty text shows the id. |
| `Edge(from, to, opts...)` | `Label("yes")`, `From(cligram.Right)`, `To(cligram.Top)`. Edges between the same nodes are told apart by label (`EdgeRef{From, To, Label}`). |
| `Layout(opts...)` | Places every node and routes every edge. Never fails: `l.Warnings()` lists mistakes and placements it had to leave out. |
| `Render(State, Theme)` | Paints the layout. Never moves a box, so repaint freely. |
| `State` | `Status` per node (`Idle`, `Active`, `Done`, `Failed`, `Waiting`), the edges `Taken`, the node in `Focus`. Focus is view state, not part of the diagram. |

### Placement

Optional. A node without a placement is put beside the step that leads to
it, and further ways on stack below one another. A placement pins one:

```
above x   below x   left of x   right of x
above-left of x   above-right of x   below-left of x   below-right of x
level with x   top|bottom|left|right level with x
```

- Several targets: `below a and b` centers under the pair.
- Several placements: `right of a level with b`, or `right of a, below b`.
- A gap on a direction: `right of a (gap: none|tight|normal|wide|3)`.
- Gaps are minimums, so a node squeezed between two pushes them apart.
- Conflicts never break the layout. Centering goes first, then automatic
  placements, then the author's latest, with a warning naming it.
- `ParsePlacement` gives errors with a column and a fix, for validating
  user input such as a YAML `at:` field.

### Fitting the terminal

`Fit(w, h)` tries cheapest first and changes the shape before cutting
words: as is, compact gaps, wrapped like a snake into bands that read back
and forth, turned top to bottom, then narrower text. A layout fits only if
every label found a place. If nothing fits, it keeps the width and you
scroll:

```go
view = l.Reveal(view, "impl")               // move a window the least to show a node
out := l.RenderView(st, theme, view)        // paint only that window
l.Fits()                                    // did it fit?
```

Text is capped (`DefaultLimits`: 24 cells by 3 lines per node, 20 per
label; `WithLimits` changes it) and cut with `…`, so one long text can
never make the picture unreadable. Show the full text elsewhere.

### Colors

A diagram never names a color. It names what a node is, `Class("human")`,
and the host's theme decides the look:

```go
palette := cligram.Palette{
    Status:  cligram.DefaultPalette.Status,     // done green, active cyan, failed red, waiting yellow
    Classes: map[string]string{"human": "magenta", "agent": "bright-blue"},
    Line:    "gray",
    Label:   "blue",
}
if err := palette.Check(); err != nil {         // names every color it cannot read
    return err
}
theme := palette.Theme()
```

Colors are names (`green`, `bright-blue`, `gray`), `0` to `255`, or
`#rrggbb`. A class colors a box's border until the run reaches it; then
the status color takes over, the same on every kind of box. `ANSI` is the
default palette, `Plain` paints nothing. Any `func(Style, string) string`
is a `ThemeFunc`; `Style` carries the part, status, focus, kind and class.

### Navigation

- `l.Move(id, cligram.Right)`: the nearest box that way **as drawn**. It
  can change when a resize redraws the picture.
- `d.Out(id)`, `d.In(id)`: the edges, in written order. The same at any
  size.

### The Bubble Tea component

```go
m := bubble.New("Factory loop", d, bubble.WithTheme(theme))
m = m.SetSize(width, height)                    // it fits the diagram itself

// as the run moves (Path: node ids opened from the root; nil is the root)
m, cmd = m.Update(bubble.StateMsg{State: st})

// what it tells you
case bubble.FocusMsg:  // msg.Path, msg.ID: show that step's details
case bubble.OpenMsg:   // msg.Path: another diagram is shown
```

Keys (`DefaultKeys`, rebind with `WithKeys`): arrows or `hjkl` move by
sight; `tab`, `shift+tab`, `]`, `[` move along the flow (next, back, other
ways on); `enter` opens a node's `Sub` diagram, `esc` closes it; `f`
follows the run again. The focus follows the active step until the reader
moves it. `View()` is exactly the rows and no more than the columns given.

A node with `Sub(func(ctx) (*Diagram, error))` is drawn stacked and loads
only when opened. State for a diagram that is not open yet is kept for
when it is. Showing a parent box's status is the host's call: send it in
the parent's `StateMsg`.

## Pitfalls

- **Placement words are not node ids.** `top`, `bottom`, `left`, `right`,
  `above`, `below`, `level`, `with`, `of`, `and` and the diagonals cannot be
  placement targets. `Check` and `Layout().Warnings()` say so, and the
  layout places that node automatically, which is easy to miss if you do
  not read the warnings.
- **Layout is the slow part** (about 10 to 15ms for 20 steps, routing
  dominates). Lay out on size or graph changes; repaint (under 1ms) on
  state changes.
- **Glyphs are single-width.** Box drawing is East Asian "ambiguous" width;
  for terminals that draw it double, use `WithGlyphs(cligram.ASCII)`.
- **A label may find no room** in a very tight layout, and in a very dense
  one an edge may find no way through. Both are warnings, and `Fit`
  counts such a layout as not fitting; an undrawn edge counts as worse
  than any number of lost labels.
- **Bubble Tea asks the terminal** for its background color and cursor
  position at start. A pseudo terminal in tests must answer, or the program
  waits.

## Working on cligram

```sh
make test     # go test -race ./...
make lint     # pinned golangci-lint built with this module's Go
make golden   # rewrite testdata/*.golden; read the diff before committing
make fuzz     # random diagrams, every drawing read back (FUZZTIME=2m)
make pixels   # render in a real terminal engine, check the pixels (needs Node)
```

CI (`.github/workflows/ci.yml`) runs all of it on every push: tests on the
oldest Go `go.mod` allows and the newest, with and without `-race`; lint;
3 minutes of fuzzing, 20 every night.

### The harness: every drawing read back

`internal/reader` reads a drawing from its text alone, as a person would:
boxes by their corners, lines followed from where they leave a box,
through junctions, crossings and labels, to an arrowhead; a label belongs
to the one arrowhead past it. It knows only what the Unicode glyphs mean,
never cligram's tables, so it cannot share their bugs. It refuses a
drawing that is ambiguous: a line that leads nowhere, a line leaving two
boxes, a label on a line two edges share.

`harness_test.go` generates random diagrams (kinds, classes,
sub-diagrams, cycles, self-loops, long, CJK and emoji text, valid and
broken placements, both orientations, random `Fit` sizes and run states)
and checks the reader's picture against the diagram: every node once,
with its text; every edge from the right box to the right box, its label
on its own line; colors and markers as the state says; no box moving
when the state changes; views equal to crops; `Fit` honored; layout
deterministic. `TestDrawingsReadBack` runs 400 seeds (60 under `-race`);
`FuzzDrawings` searches for more.

When it fails:

- The seed is in the test name, or in `testdata/fuzz/FuzzDrawings/` for
  the fuzzer. Commit that file: it then runs in every `go test`.
- `cligram.Routes(l)` (export_test.go) prints a layout's routes.
- Decide first whether the reader or the drawing is wrong. The reader is
  wrong only if a person could read the drawing unambiguously.

Rules the harness forced, all in route.go:

- Branches of one node share a trunk, but once a branch splits off it
  may only cross its siblings, never join them again.
- A crossing cell remembers both lines, so a trunk goes on past it.
- A label beside its line claims a clear ring and its stretch from there
  to the target.
- A path never passes its target's landing cells, nor its own cells.
- Edges that find no way through are routed first and everything again;
  if still walled in, their nodes get room all round and are laid out
  again. Boxes keep a cell apart unless a placement says `gap: none`.

### The pixel checks: drawings as a terminal shows them

`make pixels` renders cases (random ones, and the factory loop) the way a
person would see them, and checks the pixels:

1. `TestExportPixelCases` writes each case: the ANSI a terminal is given,
   and the cells it should show.
2. `harness/pixels/render.mjs` draws them with xterm.js's WebGL renderer
   in headless Chromium (glyphs drawn cell by cell, as GPU terminals do;
   box drawing from the font, not drawn by xterm.js; Unicode 11 widths),
   in JetBrains Mono and DejaVu Sans Mono with Noto fallbacks for wide
   text and emoji, and screenshots each, plus an atlas of every glyph and
   color on its own. Fonts are pinned by checksum (`fonts.sh`), packages
   by `package-lock.json`.
3. `internal/pixels` cuts each screenshot into cells and checks each looks
   like its glyph in its color, more than like any other; a terminal that
   measures a character's width differently from cligram shifts a row a
   whole cell and fails. It also checks every glyph has ink, is not drawn
   as a missing glyph, and reaches its cell's edges where lines join.
   `TestTheChecksSeeWhatIsWrong` proves the checks fail on a row a cell
   out, a wrong color and a missing glyph.

When it fails in CI, the screenshots are uploaded as the `pixels`
artifact. Look at them: the failure names each cell, what it should be
and what it looks most like.

`checkRoutes` (route_test.go) is the older, inside view: lines stay out
of boxes, arrowheads point into their target, labels never sit on
another edge's line.
- When showing a drawing to a person, crop it by script from real output;
  never trim it by hand.
- Check behaviour end to end in a real pseudo terminal (`examples/live`),
  not only in unit tests: colors and focus show only there.
- Write prose and comments in plain words; no em dashes.
