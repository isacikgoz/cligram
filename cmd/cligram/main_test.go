package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// blocks are the README's fenced blocks of language lang, in order.
func blocks(t *testing.T, readme, lang string) []string {
	t.Helper()
	re := regexp.MustCompile("(?s)```" + lang + "\n(.*?)```")
	var out []string
	for _, m := range re.FindAllStringSubmatch(readme, -1) {
		out = append(out, m[1])
	}
	return out
}

func readme(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The README points to examples/release.yaml: it draws, whole.
func TestTheReleaseExampleDraws(t *testing.T) {
	var out, errs bytes.Buffer
	if code := run([]string{"-width", "80", "../../examples/release.yaml"}, nil, &out, &errs); code != 0 || errs.Len() > 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "Ready to ship?") {
		t.Errorf("drew:\n%s", out.String())
	}
}

// The README's Go example is Example in example_test.go, which go test
// runs and checks.
func TestTheReadmeGoExampleIsTheTestedExample(t *testing.T) {
	md := readme(t)
	src, err := os.ReadFile("../../example_test.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)func Example\(\) \{\n(.*?)\n\t// Output:\n`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("no Example in example_test.go")
	}
	body := strings.ReplaceAll(strings.TrimSpace(m[1]), "\n\t", "\n")
	if g := blocks(t, md, "go"); len(g) == 0 || strings.TrimSpace(g[0]) != body {
		t.Errorf("the README's Go is not Example's body:\n%s", body)
	}
}

func TestTheCommandSaysWhatIsWrong(t *testing.T) {
	for _, tc := range []struct {
		args []string
		in   string
		code int
		says string
	}{
		{[]string{"a.yaml", "b.yaml"}, "", 2, "usage: cligram"},
		{nil, "", 1, "the document is empty"},
		{[]string{"-format", "dot", "-"}, "nodes: {a: A}\n", 1, "format is auto, yaml or mermaid"},
		{[]string{"-"}, "flowchart LR\na ~~> b\n", 1, "line 2: expected a link"},
		{[]string{"-"}, "graph: what\n", 1, "a Mermaid flowchart starts with"},
		{[]string{"missing.yaml"}, "", 1, "no such file"},
		{[]string{"-"}, "nodes: {a: A}\nedges: [a -> ghost]\n", 1, `"ghost" is not a node`},
		{[]string{"-color", "always", "-"}, "nodes: {a: A}\n", 0, ""},
		{[]string{"-color", "purple", "-"}, "nodes: {a: A}\n", 2, "-color is auto, always or never"},
	} {
		var out, errs bytes.Buffer
		code := run(tc.args, strings.NewReader(tc.in), &out, &errs)
		if code != tc.code || !strings.Contains(errs.String(), tc.says) {
			t.Errorf("%v: exit %d, %q", tc.args, code, errs.String())
		}
	}
}

func TestColorIsForTerminalsOrWhenAsked(t *testing.T) {
	in := "nodes: {a: A, b: B}\nedges: [a -> b]\n"
	var plain, colored, errs bytes.Buffer
	run([]string{"-"}, strings.NewReader(in), &plain, &errs)
	run([]string{"-color", "always", "-"}, strings.NewReader(in), &colored, &errs)
	if strings.Contains(plain.String(), "\x1b[") || !strings.Contains(colored.String(), "\x1b[") {
		t.Errorf("plain %q\ncolored %q", plain.String(), colored.String())
	}
}

func TestASCIIDrawsWithASCIIOnly(t *testing.T) {
	var out, errs bytes.Buffer
	run([]string{"-ascii", "-"}, strings.NewReader("nodes: {a: A, b: B}\nedges: [a -> b]\n"), &out, &errs)
	for _, r := range out.String() {
		if r > 127 {
			t.Fatalf("%q in %q", r, out.String())
		}
	}
}

// The README pipes a flowchart in: what it shows is what the command
// draws from it.
func TestTheReadmeMermaidExampleIsWhatItDraws(t *testing.T) {
	md := readme(t)
	m := regexp.MustCompile("(?s)```sh\ncligram -width 80 <<'EOF'\n(.*?)EOF\n```").FindStringSubmatch(md)
	if m == nil {
		t.Fatal("the README does not pipe a flowchart in")
	}
	var out, errs bytes.Buffer
	if code := run([]string{"-width", "80"}, strings.NewReader(m[1]), &out, &errs); code != 0 || errs.Len() > 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	if text := blocks(t, md, "text"); len(text) == 0 || text[0] != out.String() {
		t.Errorf("the README's Mermaid drawing is not what the command draws:\n%s", out.String())
	}
}

func TestMermaidComesInOnStdin(t *testing.T) {
	var out, errs bytes.Buffer
	in := "flowchart LR\n  a[Plan] --> b{Ready?}\n  b -->|yes| c([Ship])\n  b -->|no| a\n"
	if code := run(nil, strings.NewReader(in), &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	for _, want := range []string{"Plan", "Ready?", "Ship", "[ yes ]", "[ no ]", "╔", "┏"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("no %q in:\n%s", want, out.String())
		}
	}
}

func TestJSONIsForProgramsToRead(t *testing.T) {
	var out, errs bytes.Buffer
	in := "flowchart LR\n  a --> b\n  a -->|a label far too long to fit in any gap at all| b\n"
	if code := run([]string{"-json", "-width", "40", "-"}, strings.NewReader(in), &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	var got struct {
		Drawing       string
		Width, Height int
		Fits          bool
		Format        string
		Warnings      []string
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v:\n%s", err, out.String())
	}
	if got.Format != "mermaid" || got.Width == 0 || !strings.Contains(got.Drawing, "a") || got.Warnings == nil {
		t.Errorf("%+v", got)
	}
	if strings.Contains(out.String(), "\x1b[") || errs.Len() != 0 {
		t.Errorf("JSON carries no colors and says its warnings itself: %q %q", out.String(), errs.String())
	}
}

func TestTheMCPServerDraws(t *testing.T) {
	clientIn, serverOut := io.Pipe()
	serverIn, clientOut := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- serveMCP(serverIn, serverOut) }()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientIn, Writer: clientOut}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 1 || tools.Tools[0].Name != "draw" {
		t.Fatalf("tools %+v, %v", tools, err)
	}
	if !strings.Contains(tools.Tools[0].Description, "Mermaid") {
		t.Errorf("the description does not say how to write a diagram")
	}
	text := func(r *mcp.CallToolResult) string {
		var b strings.Builder
		for _, c := range r.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		return b.String()
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "draw", Arguments: map[string]any{
		"source": "flowchart LR\n  a[Plan] --> b{Ready?}\n  b -->|yes| c([Ship])",
	}})
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if got := text(res); !strings.Contains(got, "Ready?") || !strings.Contains(got, "[ yes ]") {
		t.Errorf("drawing:\n%s", got)
	}
	if res.StructuredContent == nil {
		t.Error("no structured result")
	}

	// The plan advancing: the same source, with the events so far.
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "draw", Arguments: map[string]any{
		"source": "flowchart LR\n  a[Plan] --> b{Ready?}\n  b -->|yes| c([Ship])",
		"events": []string{"a done", "a -> b", "b active", "c shipped"},
	}})
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if got := text(res); !strings.Contains(got, "✓ Plan") || !strings.Contains(got, "▸ Ready?") || !strings.Contains(got, "event 4:") {
		t.Errorf("drawing with events:\n%s", got)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "draw", Arguments: map[string]any{
		"source": "flowchart LR\n  a ~~> b",
	}})
	if err != nil || !res.IsError || !strings.Contains(text(res), "line 2") {
		t.Errorf("a mistake in the diagram: %v %+v", err, res)
	}

	_ = session.Close()
	_ = clientOut.Close()
	if err := <-done; err != nil {
		t.Logf("server ended: %v", err)
	}
}

func TestMarkdownComesBackWithItsDiagramsDrawn(t *testing.T) {
	var out, errs bytes.Buffer
	in := "Steps:\n\n```mermaid\nflowchart LR\n  a[Plan] --> b[Ship]\n```\n\n```mermaid\nflowchart LR\n  a ~~> b\n```\n"
	if code := run([]string{"md", "-width", "60"}, strings.NewReader(in), &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "│   Plan ├───►│   Ship │") || !strings.Contains(out.String(), "<!-- cligram: line 2:") {
		t.Errorf("out:\n%s", out.String())
	}
	if !strings.Contains(errs.String(), "cligram md: the block at line 8: line 2:") {
		t.Errorf("stderr: %s", errs.String())
	}
}

// Without a terminal, cligram watch prints where the events left the run,
// and says which lines it did not understand.
func TestWatchPrintsWhereTheRunEnded(t *testing.T) {
	dir := t.TempDir()
	flow := dir + "/flow.mmd"
	if err := os.WriteFile(flow, []byte("flowchart LR\n  build[Build] --> test{Pass?}\n  test -->|no| fix[Fix]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	events := "build done\nbuild -> test\n{\"node\":\"test\",\"status\":\"failed\"}\nnot an event\nfix active\n"
	if code := run([]string{"watch", flow}, strings.NewReader(events), &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	for _, want := range []string{"✓ Build", "✗ Pass?", "▸ Fix"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("no %q in:\n%s", want, out.String())
		}
	}
	if !strings.Contains(errs.String(), `line 4: "not an event" is not an event`) {
		t.Errorf("stderr: %s", errs.String())
	}
	if code := run([]string{"watch"}, strings.NewReader(""), &out, &errs); code != 2 {
		t.Errorf("no diagram: exit %d", code)
	}
}

func TestVersionSaysWhichBuildThisIs(t *testing.T) {
	var out, errs bytes.Buffer
	if code := run([]string{"-version"}, nil, &out, &errs); code != 0 || out.String() != "cligram dev\n" {
		t.Errorf("exit %d, %q %q", code, out.String(), errs.String())
	}
}
