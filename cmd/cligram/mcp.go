package main

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// drawInput is what an agent passes the draw tool.
type drawInput struct {
	Source string `json:"source" jsonschema:"the diagram: a Mermaid flowchart (starting with flowchart LR or flowchart TD) or cligram YAML"`
	Format string `json:"format,omitempty" jsonschema:"auto (the default), mermaid or yaml"`
	Width  int    `json:"width,omitempty" jsonschema:"columns to fit the drawing to; 0 or unset for 100"`
	Height int    `json:"height,omitempty" jsonschema:"rows to fit the drawing to; 0 or unset for as tall as it takes"`
	ASCII  bool   `json:"ascii,omitempty" jsonschema:"draw with ASCII only, for places that cannot show box drawing"`
}

// drawTool is the tool's description: how and when to use it, read by
// the agent deciding to.
const drawTool = `Draw a flow diagram (steps, decisions and labelled edges) as plain text, laid out to fit a width. Use it to show a process, a pipeline, a state machine, an agent loop or the shape of a plan, instead of drawing boxes by hand or leaving Mermaid unrendered.

Write the diagram as a Mermaid flowchart:
  flowchart LR
    a[Plan] --> b{Ready?}
    b -->|yes| c([Ship])
    b -->|no| a
[text] is a step, {text} a decision, ([text]) an end; -->|label| labels an edge; node:::class names a kind of node. Use flowchart TD for a tall flow.

The result is the drawing, to put in your reply inside a code block, exactly as returned. If it lists warnings, act on them: shorten labels that found no room, or draw it wider.`

// serveMCP serves the draw tool over MCP on in and out until in closes.
func serveMCP(in io.Reader, out io.Writer) error {
	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "cligram", Version: version}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "draw", Description: drawTool},
		func(_ context.Context, _ *mcp.CallToolRequest, in drawInput) (*mcp.CallToolResult, *drawing, error) {
			width := in.Width
			if width <= 0 {
				width = 100
			}
			d, err := draw(request{Source: in.Source, Format: in.Format, Width: width, Height: in.Height, ASCII: in.ASCII})
			if err != nil {
				// A mistake in the diagram is the agent's to fix: said as the
				// tool's result, not as a failure of the server.
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
			}
			text := d.String()
			if len(d.Warnings) > 0 {
				text += "\nwarnings:\n"
				for _, w := range d.Warnings {
					text += "- " + w + "\n"
				}
				if h := d.hint(); h != "" {
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
