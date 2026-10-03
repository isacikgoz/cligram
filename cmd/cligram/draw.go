package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/mermaid"
	"github.com/isacikgoz/cligram/yaml"
)

// request is what to draw, and how: the same for the command and for its
// MCP tool.
type request struct {
	Source string
	// Format is yaml, mermaid, or auto: a Mermaid flowchart if it starts
	// as one, YAML otherwise.
	Format        string
	Width, Height int // 0: as wide or tall as it takes
	ASCII, Color  bool
}

// drawing is what was drawn.
type drawing struct {
	Title    string   `json:"title,omitempty"`
	Text     string   `json:"drawing"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Fits     bool     `json:"fits"`
	Format   string   `json:"format"`
	Warnings []string `json:"warnings"`
}

// draw reads r's source and draws it.
func draw(r request) (*drawing, error) {
	format := r.Format
	switch format {
	case "", "auto":
		format = "yaml"
		if mermaid.Is(r.Source) {
			format = "mermaid"
		}
	case "yaml", "mermaid":
	default:
		return nil, fmt.Errorf("format is auto, yaml or mermaid, not %q", r.Format)
	}

	var title string
	var d *cligram.Diagram
	var opts []cligram.LayoutOption
	switch format {
	case "mermaid":
		doc, err := mermaid.Parse([]byte(r.Source))
		if err != nil {
			return nil, err
		}
		title, d, opts = doc.Title, doc.Diagram, doc.Options
	default:
		doc, err := yaml.Parse([]byte(r.Source))
		if err != nil {
			if r.Format == "" || r.Format == "auto" {
				err = errors.Join(err, errors.New("(read as YAML; a Mermaid flowchart starts with \"flowchart LR\" or \"flowchart TD\")"))
			}
			return nil, err
		}
		title, d, opts = doc.Title, doc.Diagram, doc.Options
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
	l := d.Layout(opts...)
	theme := cligram.Plain
	if r.Color {
		theme = cligram.ANSI
	}
	out := &drawing{
		Title: title, Text: l.Render(cligram.State{}, theme),
		Width: l.W, Height: l.H, Fits: l.Fits(), Format: format, Warnings: []string{},
	}
	for _, w := range l.Warnings() {
		out.Warnings = append(out.Warnings, w.Error())
	}
	return out, nil
}

// String is the drawing as a terminal shows it: its title, a blank line,
// then the drawing.
func (d *drawing) String() string {
	if d.Title == "" {
		return d.Text + "\n"
	}
	return d.Title + "\n\n" + d.Text + "\n"
}

// hint is advice on the warnings, for whoever drew it to act on.
func (d *drawing) hint() string {
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
