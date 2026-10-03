package mermaid_test

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/mermaid"
)

// shape is a diagram as text, to compare: each node as id:kind:class:text
// in order, each edge as from>to:label.
func shape(d *cligram.Diagram) (nodes, edges []string) {
	for _, n := range d.Nodes() {
		nodes = append(nodes, fmt.Sprintf("%s:%v:%s:%s", n.ID, n.Kind, n.Class, strings.ReplaceAll(n.Label(), "\n", "|")))
	}
	for _, e := range d.Edges() {
		edges = append(edges, e.From+">"+e.To+":"+e.Label)
	}
	return nodes, edges
}

func TestAFlowchartIsReadAsWritten(t *testing.T) {
	for _, tc := range []struct {
		name, in     string
		nodes, edges []string
		down         bool
		title        string
	}{
		{"shapes", `flowchart LR
  a[Step] --> b{Decide?}
  b --> c([End]) & d((Stop)) & e(Round) & f[[Sub]] & g[(Store)] & h>Flag] & i[/Lean/] & j{{Hex}}`,
			[]string{"a:step::Step", "b:decision::Decide?", "c:terminal::End", "d:terminal::Stop", "e:step::Round",
				"f:step::Sub", "g:step::Store", "h:step::Flag", "i:step::Lean", "j:decision::Hex"},
			[]string{"a>b:", "b>c:", "b>d:", "b>e:", "b>f:", "b>g:", "b>h:", "b>i:", "b>j:"}, false, ""},
		{"links and labels", `graph TD
  a --> b
  a -->|yes| c
  a -- no --> d
  a == sure ==> e
  a -. maybe .-> f
  a -.-> g
  a --- h
  a ==> i
  a --x j
  a <--> k
  a -->|"quoted, text"| l`,
			[]string{"a:step::a", "b:step::b", "c:step::c", "d:step::d", "e:step::e", "f:step::f", "g:step::g",
				"h:step::h", "i:step::i", "j:step::j", "k:step::k", "l:step::l"},
			[]string{"a>b:", "a>c:yes", "a>d:no", "a>e:sure", "a>f:maybe", "a>g:", "a>h:", "a>i:", "a>j:", "a>k:", "a>l:quoted, text"},
			true, ""},
		{"chains, groups, no spaces", `flowchart LR
  a-->b-->c
  a & b --> d; d-->|done|e`,
			[]string{"a:step::a", "b:step::b", "c:step::c", "d:step::d", "e:step::e"},
			[]string{"a>b:", "b>c:", "a>d:", "b>d:", "d>e:done"}, false, ""},
		{"classes, comments, subgraphs, styles", `---
title: Release
---
%% the release flow
flowchart LR
  subgraph build [Build]
    a[Plan]:::human --> b[Code]
  end
  b --> c["Ship it<br>now"]
  class b,c agent
  classDef agent fill:#0af
  style a fill:#f00
  click a "https://example.com"`,
			[]string{"a:step:human:Plan", "b:step:agent:Code", "c:step:agent:Ship it|now"},
			[]string{"a>b:", "b>c:"}, false, "Release"},
		{"a node named once with text and once without", `flowchart TB
  a[Start]
  a --> b
  b --> a`,
			[]string{"a:step::Start", "b:step::b"}, []string{"a>b:", "b>a:"}, true, ""},
		{"a link written twice is drawn once", "flowchart LR\na --> b\na --> b",
			[]string{"a:step::a", "b:step::b"}, []string{"a>b:"}, false, ""},
		{"dashed ids", "flowchart LR\nmy-step --> other-step",
			[]string{"my-step:step::my-step", "other-step:step::other-step"}, []string{"my-step>other-step:"}, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !mermaid.Is(tc.in) {
				t.Error("not seen as a flowchart")
			}
			doc, err := mermaid.Parse([]byte(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			nodes, edges := shape(doc.Diagram)
			if !reflect.DeepEqual(nodes, tc.nodes) {
				t.Errorf("nodes:\n got %q\nwant %q", nodes, tc.nodes)
			}
			if !reflect.DeepEqual(edges, tc.edges) {
				t.Errorf("edges:\n got %q\nwant %q", edges, tc.edges)
			}
			l := doc.Diagram.Layout(doc.Options...)
			if (l.Orientation() == cligram.TopToBottom) != tc.down || doc.Title != tc.title {
				t.Errorf("down %v title %q", l.Orientation() == cligram.TopToBottom, doc.Title)
			}
		})
	}
}

func TestMistakesAreReportedWithTheirLine(t *testing.T) {
	for _, tc := range []struct{ in, says string }{
		{"", "not a flowchart"},
		{"sequenceDiagram\nA->>B: hi", `line 1: a flowchart starts with "flowchart LR"`},
		{"flowchart LR\n", "has no nodes"},
		{"flowchart LR\na --> b\na ~~> c", `line 3: expected a link such as --> after the node`},
		{"flowchart LR\na[open --> b", `line 2: node "a" has a shape that does not close`},
		{"flowchart LR\na --> ", `line 2: expected a node id`},
		{"flowchart LR\na[One] --> a[Two]", `line 2: node "a" is given a second shape or text`},
	} {
		_, err := mermaid.Parse([]byte(tc.in))
		if err == nil || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%q: got %v, want %q", tc.in, err, tc.says)
		}
	}
}

func TestIsTellsAFlowchartFromOtherText(t *testing.T) {
	for in, want := range map[string]bool{
		"flowchart LR\na-->b":                     true,
		"graph TD;a-->b":                          true,
		"%% hello\n\nflowchart TB\n":              true,
		"---\ntitle: x\n---\nflowchart LR\na-->b": true,
		"nodes: {a: A}":                           false,
		"sequenceDiagram":                         false,
		"":                                        false,
	} {
		if mermaid.Is(in) != want {
			t.Errorf("Is(%q) != %v", in, want)
		}
	}
}

// Random diagrams written as flowcharts, in every style a writer might
// use, read back as the same diagram.
func TestRandomFlowchartsReadBack(t *testing.T) {
	shapes := []struct {
		open, close string
		kind        cligram.Kind
	}{
		{"[", "]", cligram.Step}, {"(", ")", cligram.Step}, {"{", "}", cligram.Decision},
		{"([", "])", cligram.Terminal}, {"((", "))", cligram.Terminal}, {"[[", "]]", cligram.Step},
		{"{{", "}}", cligram.Decision},
	}
	words := []string{"Plan", "Ship it", "Ready?", "審查", "a, b", "x/y", "🚀 go"}
	for seed := range uint64(300) {
		rng := rand.New(rand.NewPCG(seed, 99))
		n := 1 + rng.IntN(10)
		var lines []string
		dir := []string{"LR", "TD"}[rng.IntN(2)]
		lines = append(lines, "flowchart "+dir)
		var want []string
		texts := map[int]string{}
		for i := range n {
			sh := shapes[rng.IntN(len(shapes))]
			text := words[rng.IntN(len(words))]
			class := ""
			suffix := ""
			if rng.IntN(3) == 0 {
				class = []string{"human", "agent"}[rng.IntN(2)]
				suffix = ":::" + class
			}
			lines = append(lines, fmt.Sprintf("  n%d%s\"%s\"%s%s", i, sh.open, text, sh.close, suffix))
			texts[i] = text
			want = append(want, fmt.Sprintf("n%d:%v:%s:%s", i, sh.kind, class, text))
		}
		var wantEdges []string
		seen := map[string]bool{}
		for range rng.IntN(2 * n) {
			a, b := rng.IntN(n), rng.IntN(n)
			label := []string{"", "yes", "no", "needs work"}[rng.IntN(4)]
			var link string
			switch {
			case label == "":
				link = []string{"-->", "---", "==>", "-.->"}[rng.IntN(4)]
			case rng.IntN(2) == 0:
				link = "-->|" + label + "|"
			default:
				link = "-- " + label + " -->"
			}
			sep := []string{" ", ""}[rng.IntN(2)]
			lines = append(lines, fmt.Sprintf("  n%d%s%s%sn%d", a, sep, link, sep, b))
			key := fmt.Sprintf("n%d>n%d:%s", a, b, label)
			if !seen[key] {
				seen[key] = true
				wantEdges = append(wantEdges, key)
			}
		}
		src := strings.Join(lines, "\n")
		doc, err := mermaid.Parse([]byte(src))
		if err != nil {
			t.Fatalf("seed %d: %v\n%s", seed, err, src)
		}
		nodes, edges := shape(doc.Diagram)
		sort.Strings(edges)
		sort.Strings(wantEdges)
		if !reflect.DeepEqual(nodes, want) || !reflect.DeepEqual(edges, wantEdges) {
			t.Fatalf("seed %d:\n%s\nnodes %q\n want %q\nedges %q\n want %q", seed, src, nodes, want, edges, wantEdges)
		}
	}
}

// Whatever it is given, Parse returns a diagram or an error.
func FuzzParse(f *testing.F) {
	for _, s := range []string{"flowchart LR\na-->b", "graph TD\na[x] -->|y| b{z}", "flowchart LR\na -- b --> c & d",
		"---\ntitle: t\n---\nflowchart LR\na:::c-->b", "flowchart LR\na[\"q\"]"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		doc, err := mermaid.Parse([]byte(s))
		if err == nil {
			doc.Diagram.Layout(doc.Options...)
		}
	})
}
