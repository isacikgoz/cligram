package cligram_test

import (
	"context"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
)

// factoryLoop is the top of the factory loop: triage and its outcomes.
func factoryLoop() *cligram.Diagram {
	d := cligram.New()
	d.Node("new", "New task or issue", cligram.As(cligram.Terminal))
	d.Node("triage", "Triage agent runs", cligram.At("right of new"))
	d.Node("outcome", "Triage\noutcome", cligram.As(cligram.Decision), cligram.At("right of triage"))
	d.Node("impl", "Implementation\nagent runs")
	d.Node("human", "Human provides input", cligram.At("below impl"))
	d.Node("park", "Park issue for now", cligram.At("below human"))
	d.Edge("new", "triage")
	d.Edge("triage", "outcome")
	d.Edge("outcome", "impl", cligram.Label("Automat-able"), cligram.From(cligram.Top))
	d.Edge("outcome", "human", cligram.Label("Needs human clarification"))
	d.Edge("outcome", "park", cligram.Label("Park for now"), cligram.From(cligram.Bottom))
	d.Edge("human", "triage", cligram.To(cligram.Bottom))
	return d
}

func TestADiagramKeepsWhatWasWritten(t *testing.T) {
	d := factoryLoop()
	if err := d.Check(); err != nil {
		t.Fatal(err)
	}

	var ids []string
	for _, n := range d.Nodes() {
		ids = append(ids, n.ID)
	}
	if got := strings.Join(ids, " "); got != "new triage outcome impl human park" {
		t.Errorf("nodes in order: %s", got)
	}

	n, ok := d.Lookup("outcome")
	if !ok || n.Kind != cligram.Decision || n.Label() != "Triage\noutcome" {
		t.Errorf("outcome: %+v", n)
	}
	if want := "right of triage"; len(n.Place) != 1 || n.Place[0].String() != want {
		t.Errorf("outcome placed %v, want %s", n.Place, want)
	}
	if n, _ := d.Lookup("impl"); n.Place != nil {
		t.Errorf("impl has no hint and is placed automatically, got %v", n.Place)
	}

	edges := d.Edges()
	if len(edges) != 6 {
		t.Fatalf("%d edges", len(edges))
	}
	e := edges[2]
	if e.Ref() != (cligram.EdgeRef{From: "outcome", To: "impl", Label: "Automat-able"}) || e.FromSide != cligram.Top || e.ToSide != cligram.Auto {
		t.Errorf("edge: %+v", e)
	}
}

func TestANodeWithoutTextReadsItsID(t *testing.T) {
	d := cligram.New()
	d.Node("parser", "")
	if n, _ := d.Lookup("parser"); n.Label() != "parser" {
		t.Errorf("label %q", n.Label())
	}
}

func TestAtMayBeGivenMoreThanOnce(t *testing.T) {
	d := cligram.New()
	d.Node("a", "")
	d.Node("b", "")
	d.Node("c", "", cligram.At("right of a"), cligram.At("level with b"))
	n, _ := d.Lookup("c")
	if len(n.Place) != 2 || n.Place[1].Rel != cligram.Level {
		t.Errorf("placed %v", n.Place)
	}
}

func TestANodeMayOpenIntoAnotherDiagram(t *testing.T) {
	d := cligram.New()
	d.Node("draft", "Drafting", cligram.Sub(func(context.Context) (*cligram.Diagram, error) {
		return factoryLoop(), nil
	}))
	n, _ := d.Lookup("draft")
	if n.Sub == nil {
		t.Fatal("no sub-diagram")
	}
	sub, err := n.Sub(context.Background())
	if err != nil || len(sub.Nodes()) != 6 {
		t.Errorf("sub-diagram: %v, %v", sub, err)
	}
}

func TestCheckReportsEveryMistake(t *testing.T) {
	d := cligram.New()
	d.Node("a", "A")
	d.Node("a", "again")
	d.Node("", "no id")
	d.Node("b", "", cligram.At("right of"))
	d.Node("c", "", cligram.At("below ghost and a"))
	d.Node("d", "", cligram.At("level with d"))
	d.Edge("a", "b")
	d.Edge("a", "b")
	d.Edge("a", "b", cligram.Label("other"))
	d.Edge("a", "nowhere", cligram.Label("x"))

	err := d.Check()
	if err == nil {
		t.Fatal("no mistakes found")
	}
	got := err.Error()
	for _, want := range []string{
		`node "a" is declared twice`,
		`a node with text "no id" has no id`,
		`node "b": placement "right of", column 9: name a node after "right of"`,
		`node "c" is placed below "ghost", which is not a node`,
		`node "d" is placed level with itself`,
		`edge a -> b is declared twice`,
		`edge a -> nowhere "x": "nowhere" is not a node`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"other"`) {
		t.Errorf("edges told apart by their labels are not duplicates:\n%s", got)
	}
	if n, _ := d.Lookup("a"); n.Text != "A" {
		t.Errorf("the first node declared stands, got %q", n.Text)
	}
}
