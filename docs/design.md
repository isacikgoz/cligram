# cligram design

cligram draws flow diagrams in the terminal, from Go. It takes after
[reladraw](https://github.com/reladraw/reladraw): positions are relative
("right of triage"), so the author can say where things go without
coordinates. Unlike reladraw, placement is optional: anything without a
hint is placed by an automatic layout.

The first user is [ackt](https://github.com/isacikgoz/ackt), whose playbook flows are drawn with it
and updated live while a run moves through them.

## Decisions

| Question | Decision | Why |
|---|---|---|
| Placement required? | Optional hints, automatic layout for the rest | Playbooks are written by people and agents; a hint on every step is friction |
| Where do ackt's hints live? | In the flow YAML, as `at: right of triage` | The diagram is generated from the flow, never kept beside it |
| Text DSL? | Go API first; a DSL later as a thin parser over it | ackt builds diagrams from YAML, not a DSL |
| Dependencies | Only `rivo/uniseg` in the core, for text width; bubbletea only in `cligram/bubble` | Reusable outside Charm apps; measuring CJK and emoji right needs Unicode tables |
| Edge labels | Inline on the line: `──[ Approved ]──▶` | Reads like the labelled pills of a drawn flowchart |
| Opening a sub-diagram | Zoom: it replaces the view, with a breadcrumb | Fits a small terminal; expanding in place can come later |
| Focusable edges | Not in v1, only nodes | Edge details can be shown with the focused node |
| Colors | The diagram names a node's class (`Class("human")`, `class: human` in YAML); the host's theme maps classes and statuses to colors (`Palette`) | A playbook never names a color, so it reads alike in every terminal theme, and renaming a color touches no playbook |
| Long text | Node text wraps at 24 cells, at most 3 lines; labels one line of 20; the rest cut with `…` (`WithLimits`) | One long text never makes the picture unreadable; the host shows the whole text for the focused node |

## Layers

1. **Model.** Nodes (step, decision, terminal), edges, placement hints,
   sub-diagrams. Plain Go.
2. **Layout.** The automatic layout writes the placements the author left
   out, by following the edges: a step's first way on goes beside it,
   centered, and each further one beside it too, below everything the one
   before led to. Then every placement, written or not, becomes a minimum
   distance on its axis, solved as a longest path; in a conflict, centering
   goes first, then automatic placements, then the author's latest (with a
   warning). A node placed left of or above its targets is pulled up to
   them, and nodes that overlap are moved apart along the axis they overlap
   least on. Then edges are routed one at a time on the cell grid with A*,
   shortest first: a route pays for length, corners, zig-zags, crossings,
   and running along a box's side or beside another line. Edges from one
   node share a way out and branch off one trunk. Each label goes on the
   straight stretch nearest its target that only its edge uses, else across
   a vertical stretch, else beside its line. Every drawing must be
   readable from its glyphs alone, which `internal/reader` checks on
   random diagrams: split branches only cross, never rejoin; edges that
   cannot be routed go first on a fresh attempt, then get their nodes
   more room. Runs only when the graph or the terminal size changes
   (about 15ms for the 19-step factory loop).
3. **Paint.** Cell grid to string. A theme gets each run of cells with its
   part (border, text, marker, line, arrow, label), status, focus, and the
   node's kind and class. `Palette` builds one from color names: status
   colors on every kind of box alike, a class color on a box the run has
   not reached. Runs on every state change and never moves a box.
4. **Adapters.** `Render` for static output; `cligram/bubble` for an
   interactive Bubble Tea component.

## API sketch

```go
d := cligram.New()
d.Node("new", "New task or issue", cligram.As(cligram.Terminal))
d.Node("triage", "Triage agent runs", cligram.At("right of new"))
d.Node("outcome", "Triage outcome", cligram.As(cligram.Decision)) // auto-placed
d.Node("draft", "Drafting", cligram.Sub(loadDrafting))             // opens into another diagram
d.Edge("outcome", "impl", cligram.Label("Automat-able"), cligram.From(cligram.Top))

l := d.Layout() // never fails: l.Warnings() lists mistakes and placements left out

out := l.Render(cligram.State{
	Status: map[string]cligram.Status{"triage": cligram.Done, "impl": cligram.Active},
	Taken:  []cligram.EdgeRef{{From: "outcome", To: "impl", Label: "Automat-able"}},
	Focus:  "impl",
}, theme)
```

- `At` takes the placement as a string, parsed by `cligram.ParsePlacement`.
  ackt passes its YAML `at:` through, and a future DSL reuses the grammar.
- `Sub(func(ctx) (*Diagram, error))` is lazy: ackt loads the other
  playbook only when the box is opened.
- The layout exposes every node's rectangle, for focus and hit-testing.

## Interaction (`cligram/bubble`)

Focus is view state, not part of the diagram: `State.Focus` names the node
to highlight when painting, so one diagram can be shown in two places, only
one node is ever in focus, and moving it only repaints. Two kinds of moves:

| Keys | Move | Depends on |
|---|---|---|
| arrows / `hjkl` | to the nearest box that way as drawn (`Layout.Move`): within 45 degrees, closest, sideways offset counting double, ties to the first written | the layout, so a resize can change it |
| `tab` / `shift+tab` | next step (the edge the run took, else the first) / back (history, else a step leading here) | the flow only |
| `]` / `[` | round the other ways on from the step before | the flow only |
| `enter` / `esc` | open the focused node's sub-diagram / close back out | the flow only |
| `f` | follow the run again: the focus goes with the active step | the run |

Until the reader moves it, the focus follows the active step. The host
sends `StateMsg{Path, State}` for the diagram at a path (the node ids
opened from the root), parent boxes included; it hears `FocusMsg{Path, ID}`
to show the focused step's details and `OpenMsg{Path}` when another
diagram is shown. Keys are rebindable (`WithKeys`). `examples/live` runs
the factory loop with a pretend run: `go run ./examples/live`.

## Fitting the terminal

`d.Layout(cligram.Fit(w, h))` tries ways of laying out, cheapest first, and
changes the shape of the flow before cutting any of its words:

1. As it is.
2. Default gaps closed to tight.
3. Wrapped like a snake: where the flow reaches the edge it turns into a
   new band below, reading back the other way, so the edge between bands
   is one step down. A long flow becomes the ring of a drawn flowchart.
4. Turned: top to bottom (or left to right), plain, compact, then wrapped.
5. Narrower text (16, then 12 cells), never splitting a word that fits
   the width the text was given.

A way fits only if it fits the room and every label found a place.
Placing boxes is cheap and routing is not, so a way whose boxes alone
overflow is never routed. If none fits, the one that fits the width in the
fewest rows wins, since scrolling down reads better than across, and the
host shows it through a view: `l.Reveal(view, id)` moves the view the least
it must to show a node (the active step), and `l.RenderView(st, theme,
view)` paints only that window. `l.Fits()` says which happened. About 10ms
for the 19-step factory loop.

## Terminal pitfalls

- `┼` reads as both a junction and a crossing, so it only ever means a
  junction: lines of one group (a decision's ways on) join, and lines of
  different groups cross with the one drawn last passing over unbroken.
  The router avoids crossings where it can.
- A drawn diamond costs about five rows; a decision is a double-bordered
  box instead.
- Glyphs like `▶` are ambiguous-width or turn into emoji in some
  terminals. Arrowheads are `▸ ▴ ▾ ◂` and markers `▸ ✓ ✗ ◔`, all narrow
  everywhere; box drawing is ambiguous-width too, so there is an ASCII
  glyph set.
- Every box keeps a cell for its status marker whatever the status, so a
  box never changes size as a run moves through it.
