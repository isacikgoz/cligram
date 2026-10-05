// Package draw reads a diagram, as Mermaid (a flowchart, state or
// sequence diagram) or YAML, and draws it: the one way the command, its
// MCP server and the playground draw.
package draw

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/mermaid"
	"github.com/isacikgoz/cligram/sequence"
	"github.com/isacikgoz/cligram/yaml"
)

// Request is what to draw, and how.
type Request struct {
	Source string
	// Format is yaml, mermaid, or auto: Mermaid if it starts as a Mermaid
	// diagram, YAML otherwise.
	Format        string
	Width, Height int // 0: as wide or tall as it takes
	ASCII, Color  bool
	// Orientation is across or down, kept even where turning the flow
	// would fit better; or empty for what the source says. A sequence
	// diagram is always drawn down.
	Orientation string
	// State is where a run is: the drawing shows it. A sequence diagram
	// has no run to show.
	State cligram.State
}

// Drawing is what was drawn.
type Drawing struct {
	Title    string   `json:"title,omitempty"`
	Text     string   `json:"drawing"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Fits     bool     `json:"fits"`
	Format   string   `json:"format"`
	Warnings []string `json:"warnings"`
}

// Diagram is a diagram read from its source: a flow diagram, or a
// sequence diagram.
type Diagram struct {
	Title    string
	Diagram  *cligram.Diagram       // nil for a sequence diagram
	Sequence *sequence.Diagram      // nil for a flow diagram
	Options  []cligram.LayoutOption // what the source says about its layout
	Format   string                 // mermaid or yaml
}

// IsMermaid reports whether source starts as a Mermaid diagram drawn
// here: a flowchart, a state diagram or a sequence diagram.
func IsMermaid(source string) bool { return mermaid.Is(source) || mermaid.IsSequence(source) }

// Read reads a diagram from source, written as format: yaml, mermaid, or
// auto, Mermaid if it starts as a Mermaid diagram and YAML otherwise.
func Read(source, format string) (*Diagram, error) {
	auto := format == "" || format == "auto"
	switch {
	case auto:
		format = "yaml"
		if IsMermaid(source) {
			format = "mermaid"
		}
	case format != "yaml" && format != "mermaid":
		return nil, fmt.Errorf("format is auto, yaml or mermaid, not %q", format)
	}
	if format == "mermaid" && mermaid.IsSequence(source) {
		doc, err := mermaid.ParseSequence([]byte(source))
		if err != nil {
			return nil, err
		}
		return &Diagram{Title: doc.Title, Sequence: doc.Diagram, Format: format}, nil
	}
	if format == "mermaid" {
		doc, err := mermaid.Parse([]byte(source))
		if err != nil {
			return nil, err
		}
		return &Diagram{Title: doc.Title, Diagram: doc.Diagram, Options: doc.Options, Format: format}, nil
	}
	doc, err := yaml.Parse([]byte(source))
	if err != nil {
		if auto {
			err = errors.Join(err, errors.New("(read as YAML; a Mermaid flowchart starts with \"flowchart LR\" or \"flowchart TD\")"))
		}
		return nil, err
	}
	return &Diagram{Title: doc.Title, Diagram: doc.Diagram, Options: doc.Options, Format: format}, nil
}

// Options are the layout options r asks for, after the source's own.
func (r Request) Options(src *Diagram) ([]cligram.LayoutOption, error) {
	opts := slices.Clone(src.Options)
	switch r.Orientation {
	case "":
	case "across":
		opts = append(opts, cligram.WithOrientation(cligram.LeftToRight), cligram.KeepOrientation())
	case "down":
		opts = append(opts, cligram.WithOrientation(cligram.TopToBottom), cligram.KeepOrientation())
	default:
		return nil, fmt.Errorf("orientation is across or down, not %q", r.Orientation)
	}
	if r.ASCII {
		opts = append(opts, cligram.WithGlyphs(cligram.ASCII))
	}
	if r.Width > 0 {
		h := r.Height
		if h <= 0 {
			h = 1 << 20 // as tall as it takes
		}
		opts = append(opts, cligram.Fit(r.Width, h))
	}
	return opts, nil
}

// Draw reads r's source and draws it.
func Draw(r Request) (*Drawing, error) {
	src, err := Read(r.Source, r.Format)
	if err != nil {
		return nil, err
	}
	if src.Sequence != nil {
		return r.drawSequence(src)
	}
	opts, err := r.Options(src)
	if err != nil {
		return nil, err
	}
	title, d, format := src.Title, src.Diagram, src.Format
	l := d.Layout(opts...)
	theme := cligram.Plain
	if r.Color {
		theme = cligram.ANSI
	}
	out := &Drawing{
		Title: title, Text: l.Render(r.State, theme),
		Width: l.W, Height: l.H, Fits: l.Fits(), Format: format, Warnings: []string{},
	}
	for _, w := range l.Warnings() {
		out.Warnings = append(out.Warnings, w.Error())
	}
	return out, nil
}

// drawSequence draws a sequence diagram.
func (r Request) drawSequence(src *Diagram) (*Drawing, error) {
	var opts []sequence.Option
	if r.ASCII {
		opts = append(opts, sequence.WithGlyphs(sequence.ASCII))
	}
	if r.Width > 0 {
		h := r.Height
		if h <= 0 {
			h = 1 << 20 // as tall as it takes
		}
		opts = append(opts, sequence.Fit(r.Width, h))
	}
	l := src.Sequence.Layout(opts...)
	theme := cligram.Plain
	if r.Color {
		theme = cligram.ANSI
	}
	out := &Drawing{
		Title: src.Title, Text: l.Render(theme),
		Width: l.W, Height: l.H, Fits: l.Fits(), Format: src.Format, Warnings: []string{},
	}
	for _, w := range l.Warnings() {
		out.Warnings = append(out.Warnings, w.Error())
	}
	return out, nil
}

// String is the drawing as a terminal shows it: its title, a blank line,
// then the drawing.
func (d *Drawing) String() string {
	if d.Title == "" {
		return d.Text + "\n"
	}
	return d.Title + "\n\n" + d.Text + "\n"
}

// Hint is advice on the warnings, for whoever drew it to act on.
func (d *Drawing) Hint() string {
	var lost, undrawn bool
	for _, w := range d.Warnings {
		lost = lost || strings.Contains(w, "no room for its label")
		undrawn = undrawn || strings.Contains(w, "no way through")
	}
	switch {
	case undrawn:
		return "Some edges could not be drawn: give it more width, or fewer edges into the crowded nodes."
	case lost:
		return "Some labels found no room: shorten them, or give it more width."
	}
	return ""
}
