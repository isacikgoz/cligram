package draw_test

import (
	"strings"
	"testing"

	"github.com/isacikgoz/cligram/internal/draw"
)

func TestMarkdownDrawsMermaidFlowchartsInPlace(t *testing.T) {
	md := "# Title\n\nSome text.\n\n```mermaid\nflowchart LR\n  a[Plan] --> b[Ship]\n```\n\nAfter.\n"
	out, errs := draw.Markdown(md, draw.Request{Width: 80})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	want := "# Title\n\nSome text.\n\n```\n╭────────╮    ╭────────╮\n│   Plan ├───►│   Ship │\n╰────────╯    ╰────────╯\n```\n\nAfter.\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

func TestMarkdownLeavesTheRestAsItWas(t *testing.T) {
	for name, md := range map[string]string{
		"no blocks":         "# Just text\n\nNothing to draw.\n",
		"go":                "```go\nfunc main() {}\n```\n",
		"another diagram":   "```mermaid\nclassDiagram\n  Animal <|-- Duck\n```\n",
		"no trailing break": "text without a final newline",
		"a longer fence":    "````markdown\n```mermaid\nflowchart LR\n a --> b\n```\n````\n",
	} {
		out, errs := draw.Markdown(md, draw.Request{Width: 80})
		if out != md || len(errs) > 0 {
			t.Errorf("%s: changed to\n%q\n%v", name, out, errs)
		}
	}
}

func TestMarkdownKeepsABrokenBlockAndSaysWhy(t *testing.T) {
	md := "Intro.\n\n```mermaid\nflowchart LR\n  a ~~> b\n```\nAfter.\n"
	out, errs := draw.Markdown(md, draw.Request{Width: 80})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "the block at line 3: line 2:") {
		t.Fatalf("errors: %v", errs)
	}
	want := "Intro.\n\n```mermaid\nflowchart LR\n  a ~~> b\n```\n<!-- cligram: line 2: expected a link such as - -> after the node, found \"~~> b\" -->\nAfter.\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

func TestMarkdownReadsFencesAsCommonMarkDoes(t *testing.T) {
	for name, tc := range map[string]struct{ md, has string }{
		"tildes":   {"~~~mermaid\nflowchart LR\n a --> b\n~~~\n", "~~~\n╭"},
		"indented": {"  ```mermaid\n  flowchart LR\n   a --> b\n  ```\n", "  ```\n  ╭"},
		"unclosed": {"```mermaid\nflowchart LR\n a --> b\n", "```\n╭"},
		"info":     {"```mermaid title=x\nflowchart LR\n a --> b\n```\n", "```\n╭"},
	} {
		out, errs := draw.Markdown(tc.md, draw.Request{Width: 80})
		if len(errs) > 0 || !strings.Contains(out, tc.has) || strings.Contains(out, "flowchart") {
			t.Errorf("%s:\n%s\n%v", name, out, errs)
		}
	}
}

// Whatever the Markdown, it comes back whole unless it has a mermaid
// block to draw.
func FuzzMarkdown(f *testing.F) {
	for _, s := range []string{"# t\n```go\nx\n```\n", "~~~\n```\n~~~\n", "```mermaid\nflowchart LR\na-->b\n```", "   ```\n\n````"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, md string) {
		out, _ := draw.Markdown(md, draw.Request{Width: 60})
		if !strings.Contains(md, "mermaid") && out != md {
			t.Errorf("changed:\n%q\n%q", md, out)
		}
	})
}
