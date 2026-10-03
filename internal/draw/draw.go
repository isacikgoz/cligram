// Package draw reads a diagram, as a Mermaid flowchart or YAML, and draws
// it: the one way the command, its MCP server and the playground draw.
package draw

import (
	"errors"
	"fmt"
	"strings"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/mermaid"
	"github.com/isacikgoz/cligram/yaml"
)

// Request is what to draw, and how.
type Request struct {
	Source string
	// Format is yaml, mermaid, or auto: a Mermaid flowchart if it starts
	// as one, YAML otherwise.
	Format        string
	Width, Height int // 0: as wide or tall as it takes
	ASCII, Color  bool
	// Orientation is across or down, or empty for what the source says.
	Orientation string
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

// Draw reads r's source and draws it.
func Draw(r Request) (*Drawing, error) {
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

	switch r.Orientation {
	case "":
	case "across":
		opts = append(opts, cligram.WithOrientation(cligram.LeftToRight))
	case "down":
		opts = append(opts, cligram.WithOrientation(cligram.TopToBottom))
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
	l := d.Layout(opts...)
	theme := cligram.Plain
	if r.Color {
		theme = cligram.ANSI
	}
	out := &Drawing{
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
