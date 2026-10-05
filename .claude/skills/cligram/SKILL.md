---
name: cligram
description: Draw flow diagrams as text in the terminal with cligram, from a Mermaid flowchart, state or sequence diagram. Use when explaining or planning a process, a pipeline, a workflow, a state machine, an agent loop, a request's path through services, the messages between services or people, a decision tree or the steps of a change, and a picture would say it faster than prose; when the user asks to "draw", "diagram", "sketch" or "show the flow" in a terminal or chat; or when you would otherwise draw boxes and arrows by hand or leave Mermaid source unrendered. NOT for charts of data, and NOT for diagrams that are not steps or messages joined by arrows.
---

# cligram

cligram turns a Mermaid flowchart into a diagram drawn in text: boxes,
decisions and labelled arrows, laid out to fit a width, routed so lines do
not cross where they can avoid it, and readable in any terminal or chat
that shows a code block. You already know Mermaid; cligram makes it
visible where Mermaid would stay source.

## Draw

If an MCP tool named `draw` from cligram is available, call it with the
flowchart as `source` (and `width` if the reader's space is not about 100
columns). Otherwise run the command:

```sh
cligram -width 100 <<'EOF'
flowchart LR
  plan[Plan the change] --> review{Tests pass?}
  review -->|yes| ship([Ship])
  review -->|no| fix[Fix it]
  fix --> review
EOF
```

If `cligram` is not found, install it with
`brew install isacikgoz/tap/cligram`, a binary from
https://github.com/isacikgoz/cligram/releases, or
`go install github.com/isacikgoz/cligram/cmd/cligram@latest`.

Then **put the output in your reply inside a code block, exactly as it came
back**. The user may not see a command's output in the transcript, and the
drawing only lines up in a monospace block. Do not edit it by hand: every
character's place matters.

## Write the flowchart

- `flowchart LR` reads left to right; `flowchart TD` top to bottom, better
  for long chains in a narrow space. cligram also wraps a long flow into
  bands on its own.
- Shapes say what a node is: `a[text]` a step, `a{text?}` a decision,
  `a([text])` a start or an end. Other Mermaid shapes draw as steps.
- `a -->|label| b` labels an edge, `a --> b --> c` chains, `a & b --> c`
  joins several. `a -.-> b` draws dashed (optional, a fallback), `a ==> b`
  thick (the main way).
- `a[text]:::human` gives a node a class; a host may color classes.
- `a --- b` is a link with no way, drawn without an arrowhead. A graph of
  only those (services that talk, a cluster's peers) is a network, laid
  out from its best-linked node outward rather than as a flow.
- `subgraph id [Title]` ... `end` draws a titled frame round its nodes:
  a stage, a team's part, a loop's inside. They nest.
- A state machine can be a `stateDiagram-v2`: `[*] --> a`, `a --> b : event`,
  `state c <<choice>>`; `[*]` draws as Start and End.
- Messages between services or people, in order, are a `sequenceDiagram`:
  `participant api as Orders API`, `actor u`, `u->>+api: GET /orders`
  (`+` and `-` activate), `api-->>-u: 200` (dashed, a reply), `--x` lost;
  `alt cond` / `else` / `end`, `loop`, `opt`, `par`/`and`;
  `Note over u,api: text`; `autonumber`. It is always drawn top down.
- Keep node text short (it wraps at 24 columns, three lines at most) and
  labels shorter (20 columns, one line): cligram cuts longer text with `…`.
- Name nodes with short ids and give the words in brackets: `ci[CI runs]`.

## Show a plan advancing

Draw the plan once, then again as you work, with events saying where you
are: `plan done`, `plan -> act` (an edge taken), `act active`. Steps then
carry a marker: ✓ done, ▸ active, ✗ failed, ◔ waiting. With the MCP tool,
pass the same `source` and every event so far as `events`. With the
command, save the flowchart once and pipe the events in:

```sh
printf 'plan done\nplan -> act\nact active\n' | cligram watch -width 100 plan.mmd
```

## Read what it says

Mistakes in the flowchart come back as errors naming the line: fix the
line and draw again. Warnings come after a drawing that worked:

- "has no room for its label": the drawing is fine but an edge lost its
  label. Shorten the label, or draw wider.
- "has no way through and is not drawn": a node has more edges than room
  around it. Draw wider, or split the flow.

`cligram -json` gives the drawing, its width and height, whether it fits,
and the warnings as JSON, when you want to check before showing it.

## Also

- YAML works too (`nodes:` and `edges:`); see examples/release.yaml in the repository.
- `cligram flow.mmd` draws a file; `-ascii` draws with ASCII only, for
  places that mangle box drawing characters.
- Go programs use the library directly, and package `bubble` shows a
  diagram live in a Bubble Tea TUI as a run moves through it.
