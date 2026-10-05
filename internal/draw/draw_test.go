package draw_test

import (
	"strings"
	"testing"

	"github.com/isacikgoz/cligram/internal/draw"
)

const chain = "flowchart LR\n  a --> b --> c"

// Orientation turns the flow whatever the source says; empty keeps it.
func TestOrientationTurnsTheFlow(t *testing.T) {
	for _, tc := range []struct {
		orientation string
		rows        int // the boxes of a chain of three side by side: 3 rows
	}{
		{"", 3},
		{"across", 3},
		{"down", 13},
	} {
		d, err := draw.Draw(draw.Request{Source: chain, Orientation: tc.orientation})
		if err != nil {
			t.Fatal(err)
		}
		if d.Height != tc.rows {
			t.Errorf("%q: %d rows, want %d:\n%s", tc.orientation, d.Height, tc.rows, d.Text)
		}
	}
	if _, err := draw.Draw(draw.Request{Source: chain, Orientation: "sideways"}); err == nil || !strings.Contains(err.Error(), "across or down") {
		t.Errorf("a bad orientation: %v", err)
	}
}

const checkout = "---\ntitle: Checkout\n---\nsequenceDiagram\n  actor U as Shopper\n  U->>+W: Place order\n  alt paid\n    W-->>-U: Confirmed\n  end"

// A sequence diagram is read as Mermaid without being told, and drawn to
// the width asked, in ASCII when asked.
func TestASequenceDiagramDraws(t *testing.T) {
	d, err := draw.Draw(draw.Request{Source: checkout, Width: 40})
	if err != nil {
		t.Fatal(err)
	}
	if d.Format != "mermaid" || d.Title != "Checkout" || !d.Fits || d.Width > 40 || len(d.Warnings) != 0 {
		t.Errorf("%+v", d)
	}
	for _, want := range []string{"Shopper", "Place order", "►", "◄", "alt paid", "┃"} {
		if !strings.Contains(d.Text, want) {
			t.Errorf("no %q in:\n%s", want, d.Text)
		}
	}
	a, err := draw.Draw(draw.Request{Source: checkout, ASCII: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range a.Text {
		if r > 127 {
			t.Fatalf("%q in an ASCII drawing:\n%s", r, a.Text)
		}
	}
	src, err := draw.Read(checkout, "auto")
	if err != nil || src.Sequence == nil || src.Diagram != nil {
		t.Errorf("read as %+v, %v", src, err)
	}
	md, errs := draw.Markdown("# Flow\n\n```mermaid\n"+checkout+"\n```\n", draw.Request{Width: 60})
	if len(errs) > 0 || !strings.Contains(md, "Place order") {
		t.Errorf("Markdown: %v\n%s", errs, md)
	}
}
