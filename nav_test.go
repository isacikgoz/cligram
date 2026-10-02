package cligram_test

import (
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
)

func TestMoveGoesToTheNearestBoxThatWay(t *testing.T) {
	// A plus: c in the middle, n s e w around it, and a far corner box.
	l := diagram("c", "n above c", "s below c", "e right of c", "w left of c",
		"far below-right of e").Layout()
	noWarnings(t, l)
	for _, tc := range []struct {
		from string
		dir  cligram.Side
		want string
	}{
		{"c", cligram.Top, "n"},
		{"c", cligram.Bottom, "s"},
		{"c", cligram.Right, "e"},
		{"c", cligram.Left, "w"},
		{"n", cligram.Bottom, "c"}, // straight on beats nearer but aside
		{"e", cligram.Bottom, "s"}, // s and far lie mirrored below e: written first wins
		{"far", cligram.Top, "e"},
		{"w", cligram.Right, "c"},
		{"n", cligram.Top, "n"}, // nothing that way
		{"far", cligram.Right, "far"},
		{"ghost", cligram.Right, "ghost"},
		{"c", cligram.Auto, "c"},
	} {
		if got := l.Move(tc.from, tc.dir); got != tc.want {
			t.Errorf("Move(%s, %v) = %s, want %s", tc.from, tc.dir, got, tc.want)
		}
	}
}

func TestMoveFallsBackToAnythingThatWay(t *testing.T) {
	// b is far off to the side but is the only thing right of a.
	l := diagram("a", "b below-right of a (gap: 2)").Layout()
	if got := l.Move("a", cligram.Right); got != "b" {
		t.Errorf("got %s", got)
	}
}

func TestMoveFollowsWhatIsDrawn(t *testing.T) {
	// The same flow, laid out two ways: right of a is b across, nothing down.
	d := chain(3)
	if got := d.Layout().Move("a", cligram.Right); got != "b" {
		t.Errorf("across: %s", got)
	}
	if got := d.Layout(cligram.WithOrientation(cligram.TopToBottom)).Move("a", cligram.Bottom); got != "b" {
		t.Errorf("down: %s", got)
	}
	if got := d.Layout(cligram.WithOrientation(cligram.TopToBottom)).Move("a", cligram.Right); got != "a" {
		t.Errorf("down, then right: %s", got)
	}
}

func TestOutAndInFollowTheFlow(t *testing.T) {
	d := factory()
	var to []string
	for _, e := range d.Out("outcome") {
		to = append(to, e.To+":"+e.Label)
	}
	want := "impl:Automat-able spec:Needs specs human:Needs human clarification park:Park for now"
	if got := strings.Join(to, " "); got != want {
		t.Errorf("out: %s", got)
	}
	var from []string
	for _, e := range d.In("impl") {
		from = append(from, e.From)
	}
	if got := strings.Join(from, " "); got != "outcome specreview ready" {
		t.Errorf("in: %s", got)
	}
	if len(d.Out("ghost")) != 0 || len(d.In("ghost")) != 0 {
		t.Error("a node that is not there has edges")
	}
}
