package main

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/isacikgoz/cligram/internal/draw"
	"github.com/isacikgoz/cligram/internal/watch"
)

// drawInput is what an agent passes the draw tool.
type drawInput struct {
	Source string   `json:"source" jsonschema:"the diagram: a Mermaid flowchart (starting with flowchart LR or flowchart TD) or cligram YAML"`
	Format string   `json:"format,omitempty" jsonschema:"auto (the default), mermaid or yaml"`
	Width  int      `json:"width,omitempty" jsonschema:"columns to fit the drawing to; 0 or unset for 100"`
	Height int      `json:"height,omitempty" jsonschema:"rows to fit the drawing to; 0 or unset for as tall as it takes"`
	ASCII  bool     `json:"ascii,omitempty" jsonschema:"draw with ASCII only, for places that cannot show box drawing"`
	Events []string `json:"events,omitempty" jsonschema:"where a run is, applied in order before drawing: \"node status\" (idle, active, done, failed or waiting), \"a -> b\" for an edge taken, \"reset\""`
}

// drawTool is the tool's description: how and when to use it, read by
// the agent deciding to.
const drawTool = `Draw a flow diagram (steps, decisions and labelled edges) as plain text, laid out to fit a width. Use it to show a process, a pipeline, a state machine, an agent loop or the shape of a plan, instead of drawing boxes by hand or leaving Mermaid unrendered.

Write the diagram as a Mermaid flowchart:
  flowchart LR
    a[Plan] --> b{Ready?}
    b -->|yes| c([Ship])
    b -->|no| a
A state machine may be a stateDiagram-v2 instead. [text] is a step, {text} a decision, ([text]) an end; -->|label| labels an edge; -.-> is dashed and ==> thick; subgraph id [Title] ... end draws a frame round some nodes; node:::class names a kind of node. Use flowchart TD for a tall flow.

The result is the drawing, to put in your reply inside a code block, exactly as returned. If it lists warnings, act on them: shorten labels that found no room, or draw it wider.

To show a plan advancing as you work, draw it again with the same source and events, all of them so far, in order: "plan done", "plan -> act", "act active". Each step then shows a marker: ✓ done, ▸ active, ✗ failed, ◔ waiting.`

// versionOf is the release's version, or the module's for a go install of
// a tagged version, or dev.
func versionOf() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// serveMCP serves the draw tool over MCP on in and out until in closes.
func serveMCP(in io.Reader, out io.Writer) error {
	s := mcp.NewServer(&mcp.Implementation{Name: "cligram", Version: versionOf()}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "draw", Description: drawTool},
		func(_ context.Context, _ *mcp.CallToolRequest, in drawInput) (*mcp.CallToolResult, *draw.Drawing, error) {
			width := in.Width
			if width <= 0 {
				width = 100
			}
			req := draw.Request{Source: in.Source, Format: in.Format, Width: width, Height: in.Height, ASCII: in.ASCII}
			// The events are said back as warnings when they name nothing
			// in the diagram: the drawing is still worth having.
			var notes []string
			if len(in.Events) > 0 {
				if src, err := draw.Read(in.Source, in.Format); err == nil {
					for i, line := range in.Events {
						ev, ok, err := watch.Parse(line)
						if err == nil && ok {
							err = watch.Apply(src.Diagram, &req.State, ev)
						}
						if err != nil {
							notes = append(notes, fmt.Sprintf("event %d: %v", i+1, err))
						}
					}
				}
			}
			d, err := draw.Draw(req)
			if err != nil {
				// A mistake in the diagram is the agent's to fix: said as the
				// tool's result, not as a failure of the server.
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
			}
			d.Warnings = append(d.Warnings, notes...)
			text := d.String()
			if len(d.Warnings) > 0 {
				text += "\nwarnings:\n"
				for _, w := range d.Warnings {
					text += "- " + w + "\n"
				}
				if h := d.Hint(); h != "" {
					text += h + "\n"
				}
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, d, nil
		})
	if err := s.Run(context.Background(), &mcp.IOTransport{Reader: io.NopCloser(in), Writer: nopWriteCloser{out}}); err != nil {
		return fmt.Errorf("serving: %w", err)
	}
	return nil
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
