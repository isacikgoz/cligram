package cligram

import (
	"reflect"
	"testing"
)

func TestWrapBreaksAtSpacesWithinTheWidth(t *testing.T) {
	for _, tc := range []struct {
		in    string
		width int
		lines int
		want  []string
	}{
		{"Human reviews code and verification output", 24, 3, []string{"Human reviews code and", "verification output"}},
		{"short", 24, 3, []string{"short"}},
		{"", 24, 3, []string{""}},
		{"two\nlines", 24, 3, []string{"two", "lines"}},
		{"  spaces   collapse  ", 24, 3, []string{"spaces collapse"}},
		// A word wider than a line is broken where lines end.
		{"supercalifragilistic", 8, 3, []string{"supercal", "ifragili", "stic"}},
		{"a supercalifragilistic b", 8, 4, []string{"a", "supercal", "ifragili", "stic b"}},
		// More lines than allowed: the last one says there is more.
		{"one two three four five six", 9, 2, []string{"one two", "three…"}},
		{"one\ntwo\nthree", 10, 2, []string{"one", "two…"}},
		{"exactly9x more", 9, 1, []string{"exactly9…"}},
		// Wide graphemes count as two cells.
		{"審查審查審查", 8, 2, []string{"審查審查", "審查"}},
	} {
		if got := wrap(tc.in, tc.width, tc.lines, "…"); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("wrap(%q, %d, %d) = %q, want %q", tc.in, tc.width, tc.lines, got, tc.want)
		}
		for _, l := range wrap(tc.in, tc.width, tc.lines, "…") {
			if textWidth(l) > tc.width {
				t.Errorf("wrap(%q, %d): %q is %d cells", tc.in, tc.width, l, textWidth(l))
			}
		}
	}
}

func TestCutKeepsALabelWithinItsWidth(t *testing.T) {
	for _, tc := range []struct {
		in, ell string
		width   int
		want    string
	}{
		{"Approved", "…", 20, "Approved"},
		{"Needs human clarification", "…", 20, "Needs human clarifi…"},
		{"Needs human clarification", "...", 20, "Needs human clari..."},
		{"Needs human x", "…", 12, "Needs human…"},
		{"審查審查審查", "…", 6, "審查…"},
	} {
		got := cut(tc.in, tc.width, tc.ell)
		if got != tc.want || textWidth(got) > tc.width {
			t.Errorf("cut(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

func TestLimitsBelowTheMinimumAreRaised(t *testing.T) {
	got := Limits{NodeWidth: 1, NodeLines: 0, LabelWidth: -3}.sane()
	if got != (Limits{NodeWidth: minTextWidth, NodeLines: 1, LabelWidth: minTextWidth}) {
		t.Errorf("got %+v", got)
	}
}

func TestLongTextStaysWithinTheLimitsInALayout(t *testing.T) {
	d := New()
	d.Node("a", "An extraordinarily long step summary that an agent wrote without any regard for the width of a terminal")
	d.Node("b", "b")
	d.Edge("a", "b", Label("a label that goes on and on and on"))
	l := d.Layout(WithLimits(Limits{NodeWidth: 20, NodeLines: 2, LabelWidth: 12}))
	// As wide as its widest wrapped line, never wider than the limit.
	if r, _ := l.Rect("a"); r.W > 20+boxChrome || r.H != 2+2 {
		t.Errorf("a is %+v", r)
	}
	if got := l.routes[0].label; got != "a label tha…" {
		t.Errorf("label %q", got)
	}
}
