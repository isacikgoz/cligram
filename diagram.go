// Package cligram draws flow diagrams in the terminal.
//
// A diagram is nodes and the edges between them. Where a node goes may be
// said relative to other nodes ("right of triage"), as in reladraw;
// anything left unsaid is placed automatically. The layout fits the
// terminal it is drawn in, and the picture can be repainted as a run moves
// through it without any box moving.
package cligram

import (
	"context"
	"errors"
	"fmt"
)

// Kind is what a node is in the flow, and so how it is drawn.
type Kind int

const (
	Step     Kind = iota // something done
	Decision             // a choice between ways on
	Terminal             // where a flow starts or ends
)

func (k Kind) String() string {
	switch k {
	case Step:
		return "step"
	case Decision:
		return "decision"
	case Terminal:
		return "terminal"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Side is a side of a node an edge leaves or arrives on.
type Side int

const (
	Auto Side = iota // whichever side the layout finds best
	Top
	Bottom
	Left
	Right
)

func (s Side) String() string {
	switch s {
	case Auto:
		return "auto"
	case Top:
		return "top"
	case Bottom:
		return "bottom"
	case Left:
		return "left"
	case Right:
		return "right"
	}
	return fmt.Sprintf("Side(%d)", int(s))
}

// SubFunc loads the diagram a node opens into. It is called only when the
// node is opened, so a host can fetch it on demand.
type SubFunc func(context.Context) (*Diagram, error)

// Node is a box in the diagram.
type Node struct {
	ID string
	// Text is what the box reads; "\n" breaks a line. Empty, the box
	// reads its ID.
	Text string
	Kind Kind
	// Class says what sort of node it is to whoever colors it, such as
	// "human" or "agent": a theme maps it to a look, so a diagram never
	// names a color.
	Class string
	Place []Placement
	// Sub, when set, is the diagram the node opens into.
	Sub SubFunc
	// Group is the group the node is in, drawn in its frame; empty for
	// none.
	Group string
}

// Group is a frame drawn round some nodes, with a title: a stage of a
// pipeline, one team's part of a flow. A group may be inside another.
type Group struct {
	ID, Title string
	// Parent is the group this one is inside; empty for none.
	Parent string
}

// GroupOption sets something about a group.
type GroupOption func(*Group)

// Inside puts the group inside another, its frame within the other's.
func Inside(parent string) GroupOption {
	return func(g *Group) { g.Parent = parent }
}

// Label is what the box reads.
func (n Node) Label() string {
	if n.Text == "" {
		return n.ID
	}
	return n.Text
}

// Edge is a way on from one node to another.
type Edge struct {
	From, To string
	// Label says when the edge is taken, drawn inline on its line.
	Label string
	// FromSide and ToSide pin where the edge leaves and arrives.
	FromSide, ToSide Side
	// Line is how the edge's line is drawn: solid, dashed or thick.
	Line LineStyle
	// Undirected is a link with no way: drawn with no arrowhead, and
	// never sharing a line with another edge, so where it goes is plain.
	// A diagram whose edges are all undirected is a network, laid out by
	// its links rather than as a flow. A loop is drawn as directed.
	Undirected bool
}

// LineStyle is how an edge's line is drawn. Its straight runs show it;
// corners and junctions are drawn as a solid line's, and a trunk that
// edges of different styles share is solid.
type LineStyle int

const (
	Solid LineStyle = iota
	// Dashed is for an edge taken less: optional, or a fallback.
	Dashed
	// Thick is for an edge that matters more: the main way.
	Thick
)

// EdgeRef names an edge: two edges between the same nodes are told apart
// by their labels.
type EdgeRef struct {
	From, To, Label string
}

// Ref names e.
func (e Edge) Ref() EdgeRef { return EdgeRef{From: e.From, To: e.To, Label: e.Label} }

// Diagram is a graph of nodes and edges, built up in any order: a node may
// be placed against, or linked to, one declared after it. Mistakes are
// collected rather than returned at each call, and Check reports them.
type Diagram struct {
	nodes    []Node
	index    map[string]int
	edges    []Edge
	groups   []Group
	problems []error
}

// New starts an empty diagram.
func New() *Diagram {
	return &Diagram{index: map[string]int{}}
}

// NodeOption sets something about a node.
type NodeOption func(*nodeSpec)

type nodeSpec struct {
	node Node
	errs []error
}

// As makes the node a step, a decision or a terminal.
func As(k Kind) NodeOption {
	return func(s *nodeSpec) { s.node.Kind = k }
}

// Class gives the node a class for themes to color it by.
func Class(name string) NodeOption {
	return func(s *nodeSpec) { s.node.Class = name }
}

// At places the node relative to others, in the placement language of
// ParsePlacement: "right of triage", "below a and b (gap: tight)". It may
// be given more than once.
func At(placement string) NodeOption {
	return func(s *nodeSpec) {
		pl, err := ParsePlacement(placement)
		if err != nil {
			s.errs = append(s.errs, fmt.Errorf("node %q: %w", s.node.ID, err))
			return
		}
		s.node.Place = append(s.node.Place, pl...)
	}
}

// In puts the node in a group, drawn inside its frame.
func In(group string) NodeOption {
	return func(s *nodeSpec) { s.node.Group = group }
}

// Sub makes the node open into the diagram f loads.
func Sub(f SubFunc) NodeOption {
	return func(s *nodeSpec) { s.node.Sub = f }
}

// Node adds a node. A second node with the same id is a problem Check
// reports, and the first one stands.
func (d *Diagram) Node(id, text string, opts ...NodeOption) {
	s := nodeSpec{node: Node{ID: id, Text: text}}
	for _, o := range opts {
		o(&s)
	}
	d.problems = append(d.problems, s.errs...)
	switch {
	case id == "":
		d.problems = append(d.problems, fmt.Errorf("a node with text %q has no id", text))
		return
	case d.has(id):
		d.problems = append(d.problems, fmt.Errorf("node %q is declared twice", id))
		return
	}
	d.index[id] = len(d.nodes)
	d.nodes = append(d.nodes, s.node)
}

// Group adds a group: a frame, titled, round the nodes put In it. A
// second group with the same id is a problem Check reports.
func (d *Diagram) Group(id, title string, opts ...GroupOption) {
	g := Group{ID: id, Title: title}
	for _, o := range opts {
		o(&g)
	}
	for _, have := range d.groups {
		if have.ID == id {
			d.problems = append(d.problems, fmt.Errorf("group %q is declared twice", id))
			return
		}
	}
	if id == "" {
		d.problems = append(d.problems, fmt.Errorf("a group titled %q has no id", title))
		return
	}
	d.groups = append(d.groups, g)
}

// Groups are the groups in the order they were added.
func (d *Diagram) Groups() []Group { return append([]Group(nil), d.groups...) }

// group finds a group by id.
func (d *Diagram) group(id string) (Group, bool) {
	for _, g := range d.groups {
		if g.ID == id {
			return g, true
		}
	}
	return Group{}, false
}

// EdgeOption sets something about an edge.
type EdgeOption func(*Edge)

// Label says when the edge is taken: an outcome, a condition, "else".
func Label(text string) EdgeOption {
	return func(e *Edge) { e.Label = text }
}

// Line draws the edge's line dashed or thick.
func Line(s LineStyle) EdgeOption {
	return func(e *Edge) { e.Line = s }
}

// Undirected makes the edge a link with no way, as in a network: a db
// and a cache that talk both ways, a cluster's peers.
func Undirected() EdgeOption {
	return func(e *Edge) { e.Undirected = true }
}

// From pins the side the edge leaves its first node on.
func From(s Side) EdgeOption {
	return func(e *Edge) { e.FromSide = s }
}

// To pins the side the edge arrives at its second node on.
func To(s Side) EdgeOption {
	return func(e *Edge) { e.ToSide = s }
}

// Edge adds a way on from one node to another.
func (d *Diagram) Edge(from, to string, opts ...EdgeOption) {
	e := Edge{From: from, To: to}
	for _, o := range opts {
		o(&e)
	}
	for _, have := range d.edges {
		if have.Ref() == e.Ref() {
			d.problems = append(d.problems, fmt.Errorf("edge %s is declared twice", e.describe()))
			return
		}
	}
	d.edges = append(d.edges, e)
}

func (e Edge) describe() string {
	way := "->"
	if e.linked() {
		way = "--"
	}
	if e.Label == "" {
		return fmt.Sprintf("%s %s %s", e.From, way, e.To)
	}
	return fmt.Sprintf("%s %s %s %q", e.From, way, e.To, e.Label)
}

// linked reports whether e is drawn as a link with no way: undirected, and
// not a loop, which has no other end to link.
func (e Edge) linked() bool { return e.Undirected && e.From != e.To }

func (d *Diagram) has(id string) bool { _, ok := d.index[id]; return ok }

// Lookup finds a node by id.
func (d *Diagram) Lookup(id string) (Node, bool) {
	i, ok := d.index[id]
	if !ok {
		return Node{}, false
	}
	return d.nodes[i], true
}

// Nodes are the nodes in the order they were added.
func (d *Diagram) Nodes() []Node { return append([]Node(nil), d.nodes...) }

// Edges are the edges in the order they were added.
func (d *Diagram) Edges() []Edge { return append([]Edge(nil), d.edges...) }

// Check reports every mistake in the diagram, or nil. A diagram with
// mistakes still draws: what cannot be honored is left out or placed
// automatically.
func (d *Diagram) Check() error {
	errs := append([]error(nil), d.problems...)
	for _, n := range d.nodes {
		for _, pl := range n.Place {
			for _, t := range pl.Targets {
				switch {
				case t == n.ID:
					errs = append(errs, fmt.Errorf("node %q is placed %s itself", n.ID, pl.Rel))
				case !d.has(t):
					errs = append(errs, fmt.Errorf("node %q is placed %s %q, which is not a node", n.ID, pl.Rel, t))
				}
			}
		}
	}
	for _, e := range d.edges {
		for _, end := range []string{e.From, e.To} {
			if !d.has(end) {
				errs = append(errs, fmt.Errorf("edge %s: %q is not a node", e.describe(), end))
			}
		}
	}
	for _, n := range d.nodes {
		if _, ok := d.group(n.Group); n.Group != "" && !ok {
			errs = append(errs, fmt.Errorf("node %q is in group %q, which is not a group", n.ID, n.Group))
		}
	}
	for _, g := range d.groups {
		// A group's parents lead out, never back to it.
		seen := map[string]bool{g.ID: true}
		for p := g.Parent; p != ""; {
			parent, ok := d.group(p)
			if !ok {
				errs = append(errs, fmt.Errorf("group %q is inside %q, which is not a group", g.ID, p))
				break
			}
			if seen[p] {
				errs = append(errs, fmt.Errorf("group %q is inside itself, through %q", g.ID, p))
				break
			}
			seen[p] = true
			p = parent.Parent
		}
	}
	return errors.Join(errs...)
}
