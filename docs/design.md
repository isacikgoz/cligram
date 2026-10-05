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
   a vertical stretch, else beside its line, as near each stretch's middle
   as is clear. A label with no room gets its edge routed again on its
   own, then its nodes more room. A labelled loop goes last and round a
   corner of its box, since its shortest way is a hook too short for a
   label; its label may reach past the grid, which no other label may,
   as it would bar the way round the picture. Every drawing must be
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

## Networks

A network is a diagram whose edges all have no way (`Undirected()`,
Mermaid's `---`): services that talk, a cluster's peers. There is no flow
to follow, so the automatic layout is replaced (`layout_network.go`).

| Question | Decision | Why |
|---|---|---|
| How is a link drawn? | No arrowhead: its line ends in a junction on the target's border, as it leaves the source's | Reads as joined at both ends |
| Do links share lines? | Never: each is a line of its own, crossing others only | A branching line with no arrowheads would not say which boxes it links |
| Where do nodes go? | Each connected part spreads out from its middle (fewest links to the farthest node, then most links), in layers by distance along the main axis; parts side by side across it | A hub in the middle, its neighbours round it, reads as a network does |
| Order in a layer | By the average place of each node's links in the layer beside it, a few sweeps each way, then neighbours swapped while that crosses fewer links (or makes a link within a layer pass fewer nodes) | The usual layered-drawing heuristics; cheap for a diagram that fits a terminal |
| Place across | Near the middle of its links in the layers beside it, placed from each side and averaged | Leans neither way |
| Room between layers | A lane per link that must turn, where their runs across overlap | A line turning beside a box would run along its side |
| A busy box | Top to bottom, widened so each link to a layer above or below has its own cell on that side, text kept in the middle; links aimed along the side in the order their other ends lie, and leaving off the facing side costs more | The fan out of a hub then neither goes round nor crosses itself |
| Routing order | Top to bottom, longest first, so a fan's outer links take the outer cells; across, shortest first, as for flows | Across, the side facing the next layer is one cell, and the fan goes out of the top and bottom |
| Through the solver | Positions become minimums, each its own statement | What the author placed and frames kept clear still win, dropping only what they contradict |

## Sequence diagrams (`cligram/sequence`)

A sequence diagram is not a flow: its participants stand in a row in the
order they are named, and what happens between them goes down the page
in the order it happens. So it has its own package, model and layout, and
shares only the glyphs, themes and text wrapping with flowcharts.

| Question | Decision | Why |
|---|---|---|
| Where do participants go? | In the order they are first named, each lifeline as far left as the text between it and the ones before it lets it be (a longest path over "lifeline j at least d right of i") | The order is the author's; the spacing is the least that keeps every word clear |
| Where does a message's text go? | Above its line, centered between the two lifelines, wrapped; a text crossing other lifelines covers them, a cell round it | Reads like Mermaid's; covering keeps a wrapped text one block |
| A message crossing a lifeline | Its line runs over it unbroken | A junction there would read as the message touching that participant |
| Activations | A heavy lifeline (`┃`), `┠` and `┨` (vertical heavy, the line light) where a message leaves one | A bar beside the lifeline would cost a column per level; nesting shows only as active or not |
| Notes | Square corners (`┌┐└┘`), beside a lifeline with one blank and twice as far from the next | Unlike any participant box; the nearer lifeline is its own |
| Blocks (loop, alt, ...) | A frame in the flowchart's dashed frame glyphs, round everything in it with two columns to spare; its title and its sections' titles start right of its first lifeline, cover any other lifeline on their row, and end in a stroke of border | Sides never land on a lifeline, the first lifeline runs on through, and where a title ends is plain |
| Participant boxes at the bottom too? | No | Rows are what a terminal is short of; the boxes are a scroll up |
| Fitting the terminal | Narrower text, 3/4 at a time down to 8 cells; height is never fitted | Wrapping is all that changes the width; a long exchange scrolls |
| Live runs | Not yet | A run on a sequence diagram is a message reached; the model has no state yet |

The harness in `sequence/harness_test.go` reads random drawings back
from their glyphs alone (`sequence/reader_test.go`), as for flowcharts:
every participant, message, note and block in order, every glyph
explained, frames round what they hold, activations where they were
asked for. `FuzzDrawings` in that package searches for more.

## Terminal pitfalls

- `┼` reads as both a junction and a crossing, so it only ever means a
  junction: lines of one group (a decision's ways on) join, and lines of
  different groups cross with the one drawn last passing over unbroken.
  The router avoids crossings where it can.
- A drawn diamond costs about five rows; a decision is a double-bordered
  box instead.
- Glyphs like `▶` turn into emoji in some terminals. Arrowheads are
  `► ▲ ▼ ◄`, which never do, and are large enough to see in common fonts
  (the small `▸ ▴ ▾ ◂` are a few pixels). Markers are `▸ ✓ ✗ ◔`. `▲ ▼`
  are ambiguous-width like box drawing, so there is an ASCII glyph set.
- Dashed (`┄ ┊`, the four-dash vertical: three dashes read as solid in common fonts) and thick (`━ ┃`) edges show their style on straight
  runs only; corners and junctions stay light, since heavy corners are an
  end box's. Edges of different styles from one node never share a trunk,
  or the dashed one would show no dashes.
- A group is a frame, `╭╌ Title ╌╮` and `╎`, unlike any box. It is laid
  out round its nodes after each solve; a node that is not in it but lands
  in it, and frames side by side that meet, are kept apart as boxes are,
  by putting one side's nodes wholly before the other's. Lines cross a
  frame's border straight, never along it, never by a corner or the title.
- Every box keeps a cell for its status marker whatever the status, so a
  box never changes size as a run moves through it.
