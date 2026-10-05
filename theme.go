package cligram

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Status is where a run is with a node, or with an edge: an edge that was
// taken is Done, and one being taken is Active.
type Status int

const (
	Idle Status = iota
	Active
	Done
	Failed
	Waiting
)

func (s Status) String() string {
	switch s {
	case Idle:
		return "idle"
	case Active:
		return "active"
	case Done:
		return "done"
	case Failed:
		return "failed"
	case Waiting:
		return "waiting"
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

// Part is which part of the picture a run of cells belongs to.
type Part int

const (
	PartBorder     Part = iota // a node's border
	PartText                   // a node's text
	PartMarker                 // a node's status marker
	PartLine                   // an edge's line
	PartArrow                  // an edge's arrowhead
	PartLabel                  // an edge's label
	PartFrame                  // a group's frame
	PartFrameTitle             // a group's title, in its frame
	PartLifeline               // a participant's lifeline, in a sequence diagram
	PartNote                   // a note's border, in a sequence diagram
	PartNoteText               // a note's text
)

func (p Part) String() string {
	switch p {
	case PartBorder:
		return "border"
	case PartText:
		return "text"
	case PartMarker:
		return "marker"
	case PartLine:
		return "line"
	case PartArrow:
		return "arrow"
	case PartLabel:
		return "label"
	case PartFrame:
		return "frame"
	case PartFrameTitle:
		return "frame title"
	case PartLifeline:
		return "lifeline"
	case PartNote:
		return "note"
	case PartNoteText:
		return "note text"
	}
	return fmt.Sprintf("Part(%d)", int(p))
}

// Style is everything a theme needs to color a run of cells.
type Style struct {
	Part   Part
	Status Status
	// Focus is set on the focused node's cells.
	Focus bool
	// Kind and Class are the node's, on a node's cells.
	Kind  Kind
	Class string
	// Line is the edge's line style, on its line's cells.
	Line LineStyle
}

// Theme colors the picture. Paint is called with runs of cells that share
// a style, so a lipgloss style's Render fits it as it is.
type Theme interface {
	Paint(st Style, text string) string
}

// ThemeFunc makes a function a Theme.
type ThemeFunc func(st Style, text string) string

// Paint calls f.
func (f ThemeFunc) Paint(st Style, text string) string { return f(st, text) }

// Plain draws without color; the glyphs and markers still say everything.
var Plain Theme = ThemeFunc(func(_ Style, text string) string { return text })

// Palette names the colors a theme paints with. Status colors show where
// a run is, on every kind of box alike; a class color shows on the border
// of a box of that class the run has not reached, so what a node is shows
// until what happened to it takes over. Line and Label color edges no run
// has taken.
//
// A color is a name (black, red, green, yellow, blue, magenta, cyan, white,
// gray, or bright- before any but gray), a number from 0 to 255 for the
// 256-color palette, or #rrggbb. "" leaves the terminal's own color.
type Palette struct {
	Status      map[Status]string
	Classes     map[string]string
	Line, Label string
}

// DefaultPalette follows the terminal's own 16 colors, light or dark.
var DefaultPalette = Palette{
	Status: map[Status]string{Active: "cyan", Done: "green", Failed: "red", Waiting: "yellow"},
	Line:   "gray",
	Label:  "blue",
}

// ANSI is DefaultPalette's theme.
var ANSI = DefaultPalette.Theme()

var colorNames = map[string]int{
	"black": 30, "red": 31, "green": 32, "yellow": 33, "blue": 34, "magenta": 35, "cyan": 36, "white": 37,
	"gray": 90, "grey": 90,
}

// sgr is the escape parameters that set the foreground to color, and
// whether color could be read.
func sgr(color string) (string, bool) {
	c := strings.ToLower(strings.TrimSpace(color))
	if c == "" {
		return "", true
	}
	if n, ok := colorNames[c]; ok {
		return strconv.Itoa(n), true
	}
	if base, ok := strings.CutPrefix(c, "bright-"); ok {
		if n, ok := colorNames[base]; ok && n < 90 {
			return strconv.Itoa(n + 60), true
		}
		return "", false
	}
	if n, err := strconv.Atoi(c); err == nil {
		if n < 0 || n > 255 {
			return "", false
		}
		return "38;5;" + c, true
	}
	if hex, ok := strings.CutPrefix(c, "#"); ok && len(hex) == 6 {
		rgb, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return "", false
		}
		return fmt.Sprintf("38;2;%d;%d;%d", rgb>>16, rgb>>8&0xff, rgb&0xff), true
	}
	return "", false
}

// Check names every color in p that cannot be read; Theme leaves those
// out.
func (p Palette) Check() error {
	var errs []error
	bad := func(where, color string) {
		if _, ok := sgr(color); !ok {
			errs = append(errs, fmt.Errorf("%s: %q is not a color: use a name such as green or bright-blue, 0 to 255, or #rrggbb", where, color))
		}
	}
	for _, st := range []Status{Idle, Active, Done, Failed, Waiting} {
		if c, ok := p.Status[st]; ok {
			bad("status "+st.String(), c)
		}
	}
	classes := make([]string, 0, len(p.Classes))
	for name := range p.Classes {
		classes = append(classes, name)
	}
	slices.Sort(classes)
	for _, name := range classes {
		bad("class "+name, p.Classes[name])
	}
	bad("line", p.Line)
	bad("label", p.Label)
	return errors.Join(errs...)
}

// Theme paints with p.
func (p Palette) Theme() Theme {
	read := func(color string) string { code, _ := sgr(color); return code }
	status := map[Status]string{}
	for st, c := range p.Status {
		status[st] = read(c)
	}
	classes := map[string]string{}
	for name, c := range p.Classes {
		classes[name] = read(c)
	}
	line, label := read(p.Line), read(p.Label)
	return ThemeFunc(func(st Style, text string) string {
		code := paletteSGR(st, status[st.Status], classes[st.Class], line, label)
		if code == "" {
			return text
		}
		return "\x1b[" + code + "m" + text + "\x1b[0m"
	})
}

// paletteSGR is how a theme sets the style of a part, given the color of
// its status, its node's class, and of edges no run has taken.
func paletteSGR(st Style, status, class, line, label string) string {
	bold := func(code string) string {
		if code == "" {
			return "1"
		}
		return "1;" + code
	}
	switch st.Part {
	case PartLine, PartArrow:
		if status == "" {
			return line // what has not happened recedes
		}
		return status
	case PartLabel:
		if status == "" {
			return label // labels say when, so they stand out from lines
		}
		return bold(status)
	case PartMarker:
		return status
	case PartFrame, PartLifeline, PartNote:
		return line // in the background, as lines not taken are
	case PartFrameTitle:
		return "1"
	case PartBorder:
		color := status
		if color == "" {
			color = class
		}
		if st.Focus {
			return bold(color)
		}
		return color
	case PartText:
		if st.Focus || st.Status == Active {
			return "1"
		}
	}
	return ""
}
