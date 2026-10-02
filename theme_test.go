package cligram

import (
	"strings"
	"testing"
)

func TestColorsAreReadByNameNumberOrHex(t *testing.T) {
	for in, want := range map[string]string{
		"green": "32", "Green ": "32", "gray": "90", "grey": "90",
		"bright-blue": "94", "bright-black": "90", "": "",
		"0": "38;5;0", "208": "38;5;208", "#ff8800": "38;2;255;136;0", "#FFFFFF": "38;2;255;255;255",
	} {
		if got, ok := sgr(in); !ok || got != want {
			t.Errorf("sgr(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"teal", "bright-gray", "256", "-1", "#ff88", "#gg0000", "rgb(1,2,3)"} {
		if _, ok := sgr(in); ok {
			t.Errorf("sgr(%q) read a color", in)
		}
	}
}

func TestPaletteCheckNamesWhatItCannotRead(t *testing.T) {
	if err := DefaultPalette.Check(); err != nil {
		t.Error(err)
	}
	p := Palette{
		Status:  map[Status]string{Done: "teal"},
		Classes: map[string]string{"human": "magenta", "agent": "#12345"},
		Line:    "300",
	}
	err := p.Check()
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{`status done: "teal"`, `class agent: "#12345"`, `line: "300"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %s in:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "human") {
		t.Errorf("magenta is a color:\n%v", err)
	}
}

func TestAClassColorsABoxUntilTheRunReachesIt(t *testing.T) {
	theme := Palette{
		Status:  map[Status]string{Done: "green"},
		Classes: map[string]string{"human": "magenta"},
	}.Theme()
	paint := func(st Style) string { return theme.Paint(st, "x") }
	if got := paint(Style{Part: PartBorder, Class: "human"}); got != "\x1b[35mx\x1b[0m" {
		t.Errorf("idle human border: %q", got)
	}
	if got := paint(Style{Part: PartBorder, Class: "human", Status: Done}); got != "\x1b[32mx\x1b[0m" {
		t.Errorf("done human border: %q", got)
	}
	if got := paint(Style{Part: PartBorder, Class: "human", Focus: true}); got != "\x1b[1;35mx\x1b[0m" {
		t.Errorf("focused human border: %q", got)
	}
	if got := paint(Style{Part: PartText, Class: "human"}); got != "x" {
		t.Errorf("a class colors the border only: %q", got)
	}
	if got := paint(Style{Part: PartBorder, Class: "other"}); got != "x" {
		t.Errorf("a class with no color: %q", got)
	}
}

func TestEveryKindOfBoxShowsItsStatusAlike(t *testing.T) {
	for _, k := range []Kind{Step, Decision, Terminal} {
		got := ANSI.Paint(Style{Part: PartBorder, Kind: k, Status: Done}, "x")
		if got != "\x1b[32mx\x1b[0m" {
			t.Errorf("%v: %q", k, got)
		}
	}
}

func TestThemesAreToldWhatKindAndClassABoxIs(t *testing.T) {
	d := New()
	d.Node("a", "a", As(Decision), Class("human"))
	seen := map[Style]bool{}
	d.Layout().Render(State{}, ThemeFunc(func(st Style, text string) string {
		seen[st] = true
		return text
	}))
	if !seen[Style{Part: PartBorder, Kind: Decision, Class: "human"}] {
		t.Errorf("styles seen: %v", seen)
	}
}
