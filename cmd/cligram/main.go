// Command cligram draws a flow diagram in the terminal, from a Mermaid
// flowchart or from YAML.
//
//	cligram flow.mmd              # or flow.yaml
//	echo 'flowchart LR; a-->b' | cligram
//	cligram -json -width 100 -    # for a program, or an agent, to read
//	cligram md README.md          # Markdown, its mermaid blocks drawn
//	make | cligram watch flow.mmd # live, as events come in on stdin
//	cligram mcp                   # an MCP server, for agents
//
// It fits the drawing to the terminal when it can, colors it when it
// writes to one, and says on stderr what it could not draw. The formats
// are in packages github.com/isacikgoz/cligram/mermaid and .../yaml.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"

	"github.com/isacikgoz/cligram/internal/draw"
)

// version is the release's, set when a release is built.
var version string

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the command, with its streams given.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// What goes wrong is said on stderr; if even that fails, there is no
	// one left to tell.
	complain := func(args ...any) { _, _ = fmt.Fprintln(stderr, args...) }
	if len(args) > 0 && args[0] == "mcp" {
		if err := serveMCP(stdin, stdout); err != nil {
			complain("cligram mcp:", err)
			return 1
		}
		return 0
	}
	if len(args) > 0 && args[0] == "md" {
		return runMarkdown(args[1:], stdin, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "watch" {
		return runWatch(args[1:], stdin, stdout, stderr)
	}

	flags := flag.NewFlagSet("cligram", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		complain("usage: cligram [flags] [file.mmd | file.yaml | -]   (stdin when no file is given)")
		complain("       cligram md [flags] [file.md | -]   (Markdown with its mermaid blocks drawn)")
		complain("       events | cligram watch [flags] flow.mmd   (a diagram, live as a run moves)")
		complain("       cligram mcp   (an MCP server on stdio, with a draw tool)")
		flags.PrintDefaults()
	}
	width := flags.Int("width", 0, "fit the drawing to this many columns (default: the terminal's, if it is one)")
	height := flags.Int("height", 0, "and this many rows")
	ascii := flags.Bool("ascii", false, "draw with ASCII only")
	color := flags.String("color", "auto", "color: auto (when writing to a terminal), always or never")
	format := flags.String("format", "auto", "the input: auto (a Mermaid flowchart if it starts as one, else YAML), mermaid or yaml")
	asJSON := flags.Bool("json", false, "write the drawing, its size, whether it fits, and its warnings as JSON")
	showVersion := flags.Bool("version", false, "say which version this is")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		_, _ = fmt.Fprintln(stdout, "cligram", versionOf())
		return 0
	}
	if flags.NArg() > 1 || (flags.NArg() == 0 && isTerminal(stdin)) {
		flags.Usage()
		return 2
	}
	if *color != "auto" && *color != "always" && *color != "never" {
		complain("cligram: -color is auto, always or never, not", *color)
		return 2
	}

	var data []byte
	var err error
	if name := flags.Arg(0); name == "" || name == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(name)
	}
	if err != nil {
		complain("cligram:", err)
		return 1
	}

	tty := isTerminal(stdout)
	if *width == 0 && tty {
		if f, ok := stdout.(*os.File); ok {
			if w, h, err := term.GetSize(f.Fd()); err == nil {
				*width, *height = w, h
			}
		}
	}
	d, err := draw.Draw(draw.Request{
		Source: string(data), Format: *format, Width: *width, Height: *height,
		ASCII: *ascii, Color: *color == "always" || (*color == "auto" && tty && !*asJSON),
	})
	if err != nil {
		complain("cligram:", err)
		return 1
	}

	out := d.String()
	if *asJSON {
		b, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			complain("cligram:", err)
			return 1
		}
		out = string(b) + "\n"
	} else {
		for _, w := range d.Warnings {
			complain("cligram:", w)
		}
	}
	if _, err := io.WriteString(stdout, out); err != nil {
		complain("cligram:", err)
		return 1
	}
	return 0
}

// runMarkdown is cligram md: the Markdown given, with each mermaid
// flowchart block drawn in its place, for a terminal or a Markdown viewer.
func runMarkdown(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	complain := func(args ...any) { _, _ = fmt.Fprintln(stderr, args...) }
	flags := flag.NewFlagSet("cligram md", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		complain("usage: cligram md [flags] [file.md | -]   (stdin when no file is given)")
		flags.PrintDefaults()
	}
	width := flags.Int("width", 0, "fit each drawing to this many columns (default: the terminal's, if it is one, else 100)")
	ascii := flags.Bool("ascii", false, "draw with ASCII only")
	color := flags.String("color", "never", "color: auto (when writing to a terminal), always or never")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 1 || (flags.NArg() == 0 && isTerminal(stdin)) {
		flags.Usage()
		return 2
	}
	if *color != "auto" && *color != "always" && *color != "never" {
		complain("cligram md: -color is auto, always or never, not", *color)
		return 2
	}
	var data []byte
	var err error
	if name := flags.Arg(0); name == "" || name == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(name)
	}
	if err != nil {
		complain("cligram md:", err)
		return 1
	}
	tty := isTerminal(stdout)
	if *width == 0 {
		*width = 100
		if f, ok := stdout.(*os.File); ok && tty {
			if w, _, err := term.GetSize(f.Fd()); err == nil {
				*width = w
			}
		}
	}
	out, problems := draw.Markdown(string(data), draw.Request{
		Width: *width, ASCII: *ascii, Color: *color == "always" || (*color == "auto" && tty),
	})
	for _, p := range problems {
		complain("cligram md:", p)
	}
	if _, err := io.WriteString(stdout, out); err != nil {
		complain("cligram md:", err)
		return 1
	}
	return 0
}

func isTerminal(f any) bool {
	file, ok := f.(*os.File)
	return ok && term.IsTerminal(file.Fd())
}
