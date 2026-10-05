package mermaid_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/mermaid"
	"github.com/isacikgoz/cligram/sequence"
)

// messages are a sequence diagram's messages as text, to compare:
// from>to:text, then what else is said about each.
func messages(d *sequence.Diagram) []string {
	var out []string
	for _, m := range d.Messages() {
		s := fmt.Sprintf("%s>%s:%s", m.From, m.To, strings.ReplaceAll(m.Text, "\n", "|"))
		if m.Line == cligram.Dashed {
			s += " dashed"
		}
		switch m.Head {
		case sequence.Open:
			s += " open"
		case sequence.Cross:
			s += " cross"
		}
		if m.Both {
			s += " both"
		}
		if m.Activate {
			s += " +"
		}
		if m.Deactivate {
			s += " -"
		}
		out = append(out, s)
	}
	return out
}

func participants(d *sequence.Diagram) []string {
	var out []string
	for _, p := range d.Participants() {
		s := p.ID + ":" + p.Text
		if p.Actor {
			s += " actor"
		}
		out = append(out, s)
	}
	return out
}

func TestASequenceDiagramIsReadAsWritten(t *testing.T) {
	for _, tc := range []struct {
		name, in           string
		participants, msgs []string
		title              string
	}{
		{"participants in the order they are named", `sequenceDiagram
  participant B as Bob
  actor A as Alice
  A->>B: hi
  B->>C: and you?`,
			[]string{"B:Bob", "A:Alice actor", "C:"},
			[]string{"A>B:hi", "B>C:and you?"}, ""},
		{"every arrow", `sequenceDiagram
  a->>b: solid
  a-->>b: dashed
  a->b: line
  a-->b: dashed line
  a-xb: cross
  a--xb: dashed cross
  a-)b: async
  a--)b: dashed async
  a<<->>b: both
  a<<-->>b: dashed both`,
			[]string{"a:", "b:"},
			[]string{"a>b:solid", "a>b:dashed dashed", "a>b:line open", "a>b:dashed line dashed open",
				"a>b:cross cross", "a>b:dashed cross dashed cross", "a>b:async", "a>b:dashed async dashed",
				"a>b:both both", "a>b:dashed both dashed both"}, ""},
		{"activations", `sequenceDiagram
  a->>+b: go
  b->>+b: think
  b-->>-b: done
  b-->>-a: back`,
			[]string{"a:", "b:"},
			[]string{"a>b:go +", "b>b:think +", "b>b:done dashed -", "b>a:back dashed -"}, ""},
		{"text", `sequenceDiagram
  title: Orders
  a->>b: one<br>two
  a->>b: semi#59; colon#58; #quot;q#quot;
  a->>b
  a ->> b : spaced out
  Web app->>Database: names with spaces`,
			[]string{"a:", "b:", "Web app:", "Database:"},
			[]string{"a>b:one|two", `a>b:semi; colon: "q"`, "a>b:", "a>b:spaced out", "Web app>Database:names with spaces"},
			"Orders"},
		{"front matter, comments, statements on a line, styling", `---
title: Release
---
%% a comment
sequenceDiagram
  rect rgb(200, 200, 255)
  a->>b: one; b->>a: two
  end
  box Aqua Group
  participant c
  end
  create participant d
  a->>d: hello
  destroy d
  links a: {"Docs": "https://example.com"}`,
			[]string{"a:", "b:", "c:", "d:"},
			[]string{"a>b:one", "b>a:two", "a>d:hello"}, "Release"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := mermaid.ParseSequence([]byte(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			if got := participants(doc.Diagram); !reflect.DeepEqual(got, tc.participants) {
				t.Errorf("participants:\n got %q\nwant %q", got, tc.participants)
			}
			if got := messages(doc.Diagram); !reflect.DeepEqual(got, tc.msgs) {
				t.Errorf("messages:\n got %q\nwant %q", got, tc.msgs)
			}
			if doc.Title != tc.title {
				t.Errorf("title %q, want %q", doc.Title, tc.title)
			}
		})
	}
}

func TestNotesAndBlocksAreDrawn(t *testing.T) {
	doc, err := mermaid.ParseSequence([]byte(`sequenceDiagram
  autonumber
  loop every minute
    a->>b: ping
  end
  alt up
    b-->>a: pong
  else down
    Note right of b: no answer
  end
  par to b
    a->>b: one
  and to c
    a->>c: two
  end
  critical connect
    a->>b: open
  option refused
    a->>c: log it
  end
  opt rarely
    break failed
      Note over a,c: give up
    end
  end
  Note left of a: done
  Note over b: over b`))
	if err != nil {
		t.Fatal(err)
	}
	out := doc.Diagram.Layout().Render(cligram.Plain)
	for _, want := range []string{"loop every minute", "1. ping", "alt up", "2. pong", "else down", "no answer",
		"par to b", "and to c", "critical connect", "option refused", "opt rarely", "break failed", "give up", "done", "over b"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is not drawn:\n%s", want, out)
		}
	}
}

func TestSequenceMistakesGiveTheirLine(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"sequenceDiagram\n  a->>b: hi\n  what is this", "line 3: expected a participant"},
		{"sequenceDiagram\n  else x", "line 2: else belongs in an alt block"},
		{"sequenceDiagram\n  loop x\n  and y\n  end", "line 3: and belongs in a par block"},
		{"sequenceDiagram\n  a->>b: hi\n  end", "line 3: end closes no block"},
		{"sequenceDiagram\n  loop forever\n  a->>b: hi", "loop is not closed with end"},
		{"sequenceDiagram\n  a->>b: hi\n  deactivate b", "line 3: deactivate: b is not active"},
		{"sequenceDiagram\n  a->>b: hi\n  b-->>-a: back", "line 3: b-->>-a ends b's activation, but b is not active"},
		{"sequenceDiagram\n  participant a\n  participant a as Again", "line 3: participant a is declared twice"},
		{"sequenceDiagram\n  Note over a,b,c: three", "line 2: a note is over one or two participants"},
		{"flowchart LR\n  a --> b", "a sequence diagram starts with \"sequenceDiagram\""},
		{"sequenceDiagram", "the diagram has no participants"},
	} {
		_, err := mermaid.ParseSequence([]byte(tc.in))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want %q", tc.in, err, tc.want)
		}
	}
}

func TestIsSequenceTellsASequenceDiagram(t *testing.T) {
	for in, want := range map[string]bool{
		"sequenceDiagram\n  a->>b: hi":                    true,
		"%% a comment\n\nsequenceDiagram":                 true,
		"---\ntitle: T\n---\nsequenceDiagram\n  a->>b: x": true,
		"flowchart LR\n  a --> b":                         false,
		"title: x\nnodes: {}":                             false,
	} {
		if got := mermaid.IsSequence(in); got != want {
			t.Errorf("IsSequence(%q) = %v", in, got)
		}
	}
	if _, err := mermaid.Parse([]byte("sequenceDiagram\n  a->>b: hi")); err == nil || !strings.Contains(err.Error(), "ParseSequence") {
		t.Errorf("Parse of a sequence diagram: %v", err)
	}
}
