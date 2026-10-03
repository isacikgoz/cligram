package draw

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/isacikgoz/cligram/mermaid"
)

// fence opens or closes a fenced code block, as CommonMark has it: up to
// three spaces, then three or more backticks or tildes; an opening one may
// name the block's language after it.
var fence = regexp.MustCompile("^( {0,3})(`{3,}|~{3,})[ \t]*([^`]*?)[ \t]*$")

// Markdown draws each Mermaid flowchart block in md, written as
// ```mermaid, in its place, as a plain code block, with r's width, glyphs
// and colors. Everything else is as it was, other Mermaid diagrams too. A
// block that has mistakes stays as it was, with an HTML comment after it
// saying what they are. The mistakes, and anything a drawing could not
// show, are returned too, with the line of the Markdown its block starts
// on.
func Markdown(md string, r Request) (string, []error) {
	lines := strings.SplitAfter(md, "\n")
	var out strings.Builder
	var errs []error
	for i := 0; i < len(lines); i++ {
		open := fence.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n"))
		if open == nil {
			out.WriteString(lines[i])
			continue
		}
		indent, marks, info := open[1], open[2], open[3]
		// The block runs to a fence of the same marks, at least as long,
		// or to the end of the document.
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			c := fence.FindStringSubmatch(strings.TrimRight(lines[j], "\r\n"))
			if c != nil && c[3] == "" && c[2][0] == marks[0] && len(c[2]) >= len(marks) {
				end = j
				break
			}
		}
		var body strings.Builder
		for _, l := range lines[i+1 : end] {
			body.WriteString(strings.TrimPrefix(l, indent))
		}
		block := strings.Join(lines[i:min(end+1, len(lines))], "")
		if lang, _, _ := strings.Cut(info, " "); lang != "mermaid" || !mermaid.Is(body.String()) {
			out.WriteString(block)
			i = end
			continue
		}
		req := r
		req.Source, req.Format = body.String(), "mermaid"
		d, err := Draw(req)
		if err != nil {
			errs = append(errs, fmt.Errorf("the block at line %d: %w", i+1, err))
			out.WriteString(block)
			if !strings.HasSuffix(block, "\n") {
				out.WriteString("\n")
			}
			fmt.Fprintf(&out, "%s<!-- cligram: %s -->\n", indent, strings.ReplaceAll(err.Error(), "--", "- -"))
			i = end
			continue
		}
		for _, w := range d.Warnings {
			errs = append(errs, fmt.Errorf("the block at line %d: %s", i+1, w))
		}
		fmt.Fprintf(&out, "%s%s\n", indent, marks)
		for _, l := range strings.Split(strings.TrimRight(d.String(), "\n"), "\n") {
			fmt.Fprintf(&out, "%s%s\n", indent, l)
		}
		fmt.Fprintf(&out, "%s%s\n", indent, marks)
		i = end
	}
	return out.String(), errs
}
