package yaml_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/yaml"
)

const release = `
title: Release
nodes:
  triage: Triage
  ready:
    text: Ready to ship?
    kind: decision
  ship: { text: Ship it, class: human }
  fix: { text: Fix it, kind: end, at: below ready }
edges:
  - triage -> ready
  - ready -> ship: yes
  - ready -> fix: no
  - fix -> ready
`

func TestADiagramIsReadAsWritten(t *testing.T) {
	doc, err := yaml.Parse([]byte(release))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "Release" || len(doc.Options) != 0 {
		t.Errorf("title %q, %d options", doc.Title, len(doc.Options))
	}
	var ids []string
	for _, n := range doc.Diagram.Nodes() {
		ids = append(ids, n.ID)
	}
	if got := strings.Join(ids, " "); got != "triage ready ship fix" {
		t.Errorf("nodes in order: %s", got)
	}
	ready, _ := doc.Diagram.Lookup("ready")
	ship, _ := doc.Diagram.Lookup("ship")
	fix, _ := doc.Diagram.Lookup("fix")
	if ready.Kind != cligram.Decision || ready.Text != "Ready to ship?" || ship.Class != "human" ||
		fix.Kind != cligram.Terminal || len(fix.Place) != 1 || fix.Place[0].String() != "below ready" {
		t.Errorf("ready %+v ship %+v fix %+v", ready, ship, fix)
	}
	var edges []string
	for _, e := range doc.Diagram.Edges() {
		edges = append(edges, e.From+">"+e.To+":"+e.Label)
	}
	if got := strings.Join(edges, " "); got != "triage>ready: ready>ship:yes ready>fix:no fix>ready:" {
		t.Errorf("edges: %s", got)
	}
}

func TestDownTurnsTheFlow(t *testing.T) {
	doc, err := yaml.Parse([]byte("orientation: down\nnodes: {a: A, b: B}\nedges: [a -> b]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Diagram.Layout(doc.Options...).Orientation() != cligram.TopToBottom {
		t.Error("not turned")
	}
}

func TestMistakesAreReportedWithTheirLine(t *testing.T) {
	for _, tc := range []struct{ in, says string }{
		{"", "empty"},
		{"- a\n", "line 1: a diagram is a mapping"},
		{"title: x\n", "needs nodes"},
		{"nodes: {a: A}\nshape: round\n", `line 2: "shape" is not a key of a diagram`},
		{"orientation: sideways\nnodes: {a: A}\n", `line 1: orientation is across or down, not "sideways"`},
		{"nodes:\n  a: A\n  a: B\n", `line 3: node "a" is written twice`},
		{"nodes:\n  a: {text: A, kind: circle}\n", `line 2: kind is step, decision or end, not "circle"`},
		{"nodes:\n  a: {text: A, colour: red}\n", `line 2: "colour" is not a key of a node`},
		{"nodes:\n  a: A\n  b: {text: B, at: next to a}\n", `line 3: placement "next to a"`},
		{"nodes:\n  a: A\nedges:\n  - a -> ghost\n", `line 4: edge a -> ghost: "ghost" is not a node`},
		{"nodes:\n  a: A\nedges:\n  - a to a\n", `line 4: "a to a" is not an edge`},
		{"nodes:\n  a: A\nedges: a -> a\n", "line 3: edges are a list"},
		{"nodes:\n  a: A\n  b: {text: B, at: below ghost}\n", `node "b" is placed below "ghost", which is not a node`},
		{"nodes: {a: A}\nedges: [a -> a, a -> a]\n", "edge a -> a is declared twice"},
		{"nodes: [a, b]\n", "nodes are a mapping"},
	} {
		_, err := yaml.Parse([]byte(tc.in))
		if err == nil || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%q: got %v, want %q", tc.in, err, tc.says)
		}
	}
}

func TestTheArrowSaysHowTheLineIsDrawn(t *testing.T) {
	doc, err := yaml.Parse([]byte("nodes: {a: A, b: B, c: C}\nedges:\n  - a -> b\n  - b -.-> c: maybe\n  - c ==> a\n"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range doc.Diagram.Edges() {
		got = append(got, fmt.Sprintf("%s>%s:%s:%d", e.From, e.To, e.Label, e.Line))
	}
	if want := "a>b::0 b>c:maybe:1 c>a::2"; strings.Join(got, " ") != want {
		t.Errorf("got %s, want %s", strings.Join(got, " "), want)
	}
}

func TestGroupsAreFramesRoundTheirNodes(t *testing.T) {
	doc, err := yaml.Parse([]byte(`nodes:
  a: { text: A, group: build }
  b: { text: B, group: checks }
  c: C
groups:
  build: Build
  checks: { title: Checks, in: build }
edges: [a -> b, b -> c]
`))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range doc.Diagram.Groups() {
		got = append(got, g.ID+":"+g.Title+":"+g.Parent)
	}
	for _, n := range doc.Diagram.Nodes() {
		got = append(got, n.ID+"@"+n.Group)
	}
	if want := "build:Build: checks:Checks:build a@build b@checks c@"; strings.Join(got, " ") != want {
		t.Errorf("got %s", strings.Join(got, " "))
	}
	for _, bad := range []struct{ in, says string }{
		{"nodes: {a: {text: A, group: ghost}}", `node "a" is in group "ghost", which is not a group`},
		{"nodes: {a: A}\ngroups: {g: {name: x}}", `line 2: "name" is not a key of a group`},
		{"nodes: {a: A}\ngroups: [g]", "line 2: groups are a mapping"},
	} {
		if _, err := yaml.Parse([]byte(bad.in)); err == nil || !strings.Contains(err.Error(), bad.says) {
			t.Errorf("%q: %v, want %q", bad.in, err, bad.says)
		}
	}
}
