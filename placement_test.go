package cligram_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
)

func TestParsePlacementReadsEveryRelation(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want cligram.Placement
	}{
		{"above x", cligram.Placement{Rel: cligram.Above, Targets: []string{"x"}}},
		{"above of x", cligram.Placement{Rel: cligram.Above, Targets: []string{"x"}}},
		{"below x", cligram.Placement{Rel: cligram.Below, Targets: []string{"x"}}},
		{"left of x", cligram.Placement{Rel: cligram.LeftOf, Targets: []string{"x"}}},
		{"right x", cligram.Placement{Rel: cligram.RightOf, Targets: []string{"x"}}},
		{"above-left of x", cligram.Placement{Rel: cligram.AboveLeft, Targets: []string{"x"}}},
		{"above-right of x", cligram.Placement{Rel: cligram.AboveRight, Targets: []string{"x"}}},
		{"below-left of x", cligram.Placement{Rel: cligram.BelowLeft, Targets: []string{"x"}}},
		{"below-right x", cligram.Placement{Rel: cligram.BelowRight, Targets: []string{"x"}}},
		{"level with x", cligram.Placement{Rel: cligram.Level, Targets: []string{"x"}}},
		{"top level with x", cligram.Placement{Rel: cligram.TopLevel, Targets: []string{"x"}}},
		{"bottom level with x", cligram.Placement{Rel: cligram.BottomLevel, Targets: []string{"x"}}},
		{"left level with x", cligram.Placement{Rel: cligram.LeftLevel, Targets: []string{"x"}}},
		{"right level with x", cligram.Placement{Rel: cligram.RightLevel, Targets: []string{"x"}}},
		{"right of server.docker", cligram.Placement{Rel: cligram.RightOf, Targets: []string{"server.docker"}}},
		{"below needs_reply-2", cligram.Placement{Rel: cligram.Below, Targets: []string{"needs_reply-2"}}},
	} {
		got, err := cligram.ParsePlacement(tc.in)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if want := []cligram.Placement{tc.want}; !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %+v, want %+v", tc.in, got, want)
		}
	}
}

func TestParsePlacementReadsSeveralTargetsAndGaps(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []cligram.Placement
	}{
		{"below a and b", []cligram.Placement{{Rel: cligram.Below, Targets: []string{"a", "b"}}}},
		{"level with a, b and c", []cligram.Placement{{Rel: cligram.Level, Targets: []string{"a", "b", "c"}}}},
		{"level with a, b, and c", []cligram.Placement{{Rel: cligram.Level, Targets: []string{"a", "b", "c"}}}},
		{"right of a (gap: tight)", []cligram.Placement{{Rel: cligram.RightOf, Targets: []string{"a"},
			Gap: cligram.Gap{Size: cligram.GapTight}}}},
		{"below a and b (gap: 3)", []cligram.Placement{{Rel: cligram.Below, Targets: []string{"a", "b"},
			Gap: cligram.Gap{Size: cligram.GapCells, Cells: 3}}}},
		{"below a (gap:none)", []cligram.Placement{{Rel: cligram.Below, Targets: []string{"a"},
			Gap: cligram.Gap{Size: cligram.GapNone}}}},
		{"right of docker level with docker", []cligram.Placement{
			{Rel: cligram.RightOf, Targets: []string{"docker"}},
			{Rel: cligram.Level, Targets: []string{"docker"}},
		}},
		// A comma before the next placement only separates the two.
		{"right of a, below b (gap: wide)", []cligram.Placement{
			{Rel: cligram.RightOf, Targets: []string{"a"}},
			{Rel: cligram.Below, Targets: []string{"b"}, Gap: cligram.Gap{Size: cligram.GapWide}},
		}},
		{"right of x left of y level with z", []cligram.Placement{
			{Rel: cligram.RightOf, Targets: []string{"x"}},
			{Rel: cligram.LeftOf, Targets: []string{"y"}},
			{Rel: cligram.Level, Targets: []string{"z"}},
		}},
		{"  right   of\ta  ", []cligram.Placement{{Rel: cligram.RightOf, Targets: []string{"a"}}}},
	} {
		got, err := cligram.ParsePlacement(tc.in)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q:\n got %+v\nwant %+v", tc.in, got, tc.want)
		}
	}
}

func TestParsePlacementSaysWhatIsWrongAndWhere(t *testing.T) {
	for _, tc := range []struct {
		in     string
		column int
		says   string
	}{
		{"", 1, "nothing to place by"},
		{"right of", 9, `name a node after "right of"`},
		{"next to x", 1, `"next" does not start a placement`},
		{"top of x", 1, `write "top level with x"`},
		{"level x", 7, `expected "with" after "level"`},
		{"left level x", 12, `expected "with" after "level"`},
		{"right of below", 10, `"below" is a placement word`},
		{"below a and", 12, `name a node after "and"`},
		{"below a, and level", 14, `"level" is a placement word`},
		{"level with x (gap: tight)", 14, "takes no gap"},
		{"below x (size: 3)", 10, `take "gap:" and nothing else`},
		{"below x (gap: 12px)", 15, `write "12"`},
		{"below x (gap: huge)", 15, `"huge" is not a gap`},
		{"below x (gap: -1)", 15, "cannot be negative"},
		{"below x (gap: tight", 20, `expected ")"`},
		{"below x (gap tight)", 14, `expected ":" after "gap"`},
		{"below x (gap: )", 15, "expected a gap"},
		{"below x; right of y", 8, `';' cannot be in a placement`},
		{"(gap: tight)", 1, "expected a placement"},
	} {
		_, err := cligram.ParsePlacement(tc.in)
		var pe *cligram.PlacementError
		if !errors.As(err, &pe) {
			t.Errorf("%q: got %v, want a PlacementError", tc.in, err)
			continue
		}
		if pe.Column != tc.column || !strings.Contains(pe.Msg, tc.says) {
			t.Errorf("%q: got column %d %q, want column %d saying %q", tc.in, pe.Column, pe.Msg, tc.column, tc.says)
		}
	}
}

func TestPlacementStringReadsBack(t *testing.T) {
	for _, in := range []string{
		"above x",
		"left of x",
		"above-right of x",
		"level with a and b and c",
		"bottom level with x",
		"below a and b (gap: tight)",
		"right of a (gap: 4)",
		"above x (gap: none)",
		"above x (gap: normal)",
		"above x (gap: wide)",
	} {
		got, err := cligram.ParsePlacement(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if s := got[0].String(); s != in {
			t.Errorf("%q reads back as %q", in, s)
		}
		again, err := cligram.ParsePlacement(got[0].String())
		if err != nil || !reflect.DeepEqual(again, got) {
			t.Errorf("%q does not parse back to itself: %+v, %v", in, again, err)
		}
	}
}
