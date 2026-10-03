package main

import (
	"io"
	"os"

	"github.com/charmbracelet/x/term"
)

// What every way of running cligram reads and decides alike.

// validColor reports whether c is a -color: auto, always or never.
func validColor(c string) bool { return c == "auto" || c == "always" || c == "never" }

// colored reports whether to paint in color: always, or auto when writing
// to a terminal.
func colored(c string, tty bool) bool { return c == "always" || (c == "auto" && tty) }

// readSource reads the file named, or stdin for none or "-".
func readSource(name string, stdin io.Reader) ([]byte, error) {
	if name == "" || name == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(name)
}

// terminalSize is the size of out, if it is a terminal.
func terminalSize(out io.Writer) (w, h int, ok bool) {
	f, isFile := out.(*os.File)
	if !isFile || !isTerminal(out) {
		return 0, 0, false
	}
	w, h, err := term.GetSize(f.Fd())
	return w, h, err == nil
}

func isTerminal(f any) bool {
	file, ok := f.(*os.File)
	return ok && term.IsTerminal(file.Fd())
}
