// Command cligram draws a diagram file in the terminal.
//
//	cligram flow.yaml
//	cligram - < flow.yaml
//
// It fits the drawing to the terminal when it can, colors it when it
// writes to one, and says on stderr what it could not draw. The file
// format is in package github.com/isacikgoz/cligram/yaml.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/yaml"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the command, with its streams given.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// What goes wrong is said on stderr; if even that fails, there is no
	// one left to tell.
	complain := func(args ...any) { _, _ = fmt.Fprintln(stderr, args...) }
	flags := flag.NewFlagSet("cligram", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		complain("usage: cligram [flags] file.yaml (or - for stdin)")
		flags.PrintDefaults()
	}
	width := flags.Int("width", 0, "fit the drawing to this many columns (default: the terminal's, if it is one)")
	height := flags.Int("height", 0, "and this many rows")
	ascii := flags.Bool("ascii", false, "draw with ASCII only")
	color := flags.String("color", "auto", "color: auto (when writing to a terminal), always or never")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	if *color != "auto" && *color != "always" && *color != "never" {
		complain("cligram: -color is auto, always or never, not", *color)
		return 2
	}

	var data []byte
	var err error
	if name := flags.Arg(0); name == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(name)
	}
	if err != nil {
		complain("cligram:", err)
		return 1
	}
	doc, err := yaml.Parse(data)
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
	opts := doc.Options
	if *ascii {
		opts = append(opts, cligram.WithGlyphs(cligram.ASCII))
	}
	if *width > 0 {
		h := *height
		if h <= 0 {
			h = 1 << 20 // as tall as it takes
		}
		opts = append(opts, cligram.Fit(*width, h))
	}
	l := doc.Diagram.Layout(opts...)
	for _, w := range l.Warnings() {
		complain("cligram:", w)
	}

	theme := cligram.Plain
	if *color == "always" || (*color == "auto" && tty) {
		theme = cligram.ANSI
	}
	drawing := l.Render(cligram.State{}, theme) + "\n"
	if doc.Title != "" {
		drawing = doc.Title + "\n\n" + drawing
	}
	if _, err := io.WriteString(stdout, drawing); err != nil {
		complain("cligram:", err)
		return 1
	}
	return 0
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}
