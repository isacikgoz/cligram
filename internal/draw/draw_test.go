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
