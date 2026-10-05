package reader_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/internal/reader"
)

// fanout is a decision with three ways on, one of them back, drawn by
// cligram: the good picture the reader's tests break.
func fanout() string {
	d := cligram.New()
	d.Node("q", "q", cligram.As(cligram.Decision))
	d.Node("a", "a")
	d.Node("b", "b", cligram.Sub(func(context.Context) (*cligram.Diagram, error) { return cligram.New(), nil }))
	d.Node("c", "c", cligram.As(cligram.Terminal))
	d.Edge("q", "a", cligram.Label("yes"))
	d.Edge("q", "b", cligram.Label("no"))
	d.Edge("q", "c")
	d.Edge("b", "q")
	return d.Layout().Render(cligram.State{}, cligram.Plain)
}

func TestAGoodPictureReadsAsItsDiagram(t *testing.T) {
	text := fanout()
	pic, err := reader.Read(text)
	if err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	byText := map[string]int{}
	for i, b := range pic.Boxes {
		byText[b.Text()] = i
	}
	if len(pic.Boxes) != 4 || pic.Boxes[byText["q"]].Kind != reader.Decision ||
		pic.Boxes[byText["c"]].Kind != reader.Terminal {
		t.Fatalf("boxes %+v", pic.Boxes)
	}
	got := map[string]bool{}
	for _, e := range pic.Edges {
		got[pic.Boxes[e.From].Text()+">"+pic.Boxes[e.To].Text()+":"+e.Label] = true
	}
	for _, want := range []string{"q>a:yes", "q>b:no", "q>c:", "b>q:"} {
		if !got[want] {
			t.Errorf("missing %s in %v\n%s", want, got, text)
		}
	}
	if len(got) != 4 {
		t.Errorf("edges %v", got)
	}
}

// cellAt finds the first cell showing g in text and gives its row and
// column, by cells.
func cellAt(t *testing.T, text, g string) (int, int) {
	t.Helper()
	grid := reader.Parse(text)
	for y, row := range grid.Cells {
		for x, c := range row {
			if c.G == g {
				return x, y
			}
		}
	}
	t.Fatalf("no %q in\n%s", g, text)
	return 0, 0
}

// set replaces the cell at x, y with g.
func set(text string, x, y int, g string) string {
	grid := reader.Parse(text)
	var out []string
	for ry, row := range grid.Cells {
		var b strings.Builder
		for rx, c := range row {
			switch {
			case rx == x && ry == y:
				b.WriteString(g)
			case !c.Cont:
				b.WriteString(c.G)
			}
		}
		out = append(out, b.String())
	}
	return strings.Join(out, "\n")
}

func TestABrokenPictureSaysWhatIsWrong(t *testing.T) {
	good := fanout()
	for _, tc := range []struct {
		name    string
		breakIt func(t *testing.T) string
		says    string
	}{
		{"a gap in a line", func(t *testing.T) string {
			x, y := cellAt(t, good, "┬")
			return set(good, x, y+1, " ")
		}, "leads"},
		{"an arrowhead the wrong way", func(t *testing.T) string {
			x, y := cellAt(t, good, "►")
			return set(good, x, y, "◄")
		}, "arrowhead"},
		{"a broken border", func(t *testing.T) string {
			x, y := cellAt(t, good, "╔")
			return set(good, x+2, y, " ")
		}, "box at"},
		{"a stray character", func(t *testing.T) string {
			return set(good, 0, len(strings.Split(good, "\n"))-1, "x")
		}, "stray"},
		{"a label with no line", func(t *testing.T) string {
			return good + "\n\n          [ lost ]"
		}, "on no line"},
		{"a line from nowhere", func(t *testing.T) string {
			return good + "\n\n  ────►"
		}, "leads"},
		{"two boxes' lines joined", func(t *testing.T) string {
			// A box whose line runs into another's trunk.
			x, y := cellAt(t, good, "┬")
			return set(good, x, y, "┼")
		}, "leads"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := tc.breakIt(t)
			if _, err := reader.Read(bad); err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("read without the complaint %q: %v\n%s", tc.says, err, bad)
			}
		})
	}
}

func TestALabelOnASharedTrunkIsAmbiguous(t *testing.T) {
	// One box, a trunk with the label on it, branching to two boxes.
	text := strings.Join([]string{
		"╭───╮              ╭───╮",
		"│   ├──[ x ]──┬───►│   │",
		"╰───╯         │    ╰───╯",
		"              │    ╭───╮",
		"              └───►│   │",
		"                   ╰───╯",
	}, "\n")
	if _, err := reader.Read(text); err == nil || !strings.Contains(err.Error(), "2 edges share") {
		t.Errorf("err %v", err)
	}
}

func TestALineLeavingTwoBoxesIsAmbiguous(t *testing.T) {
	text := strings.Join([]string{
		"╭───╮     ╭───╮",
		"│   ├──┬─►│   │",
		"╰───╯  │  ╰───╯",
		"╭───╮  │",
		"│   ├──┘",
		"╰───╯",
	}, "\n")
	if _, err := reader.Read(text); err == nil || !strings.Contains(err.Error(), "leaves 2 boxes") {
		t.Errorf("err %v", err)
	}
}

func TestACrossingIsPassedUnder(t *testing.T) {
	// a's line goes right, under b's line going down to c.
	text := strings.Join([]string{
		"       ╭─────╮",
		"       │   b │",
		"       ╰──┬──╯",
		"╭─────╮   │    ╭─────╮",
		"│   a ├───│───►│   d │",
		"╰─────╯   │    ╰─────╯",
		"          ▼",
		"       ╭─────╮",
		"       │   c │",
		"       ╰─────╯",
	}, "\n")
	pic, err := reader.Read(text)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range pic.Edges {
		got = append(got, pic.Boxes[e.From].Text()+">"+pic.Boxes[e.To].Text())
	}
	if strings.Join(got, " ") != "a>d b>c" && strings.Join(got, " ") != "b>c a>d" {
		t.Errorf("edges %v", got)
	}
}

func TestColorsAreRead(t *testing.T) {
	d := cligram.New()
	d.Node("a", "a")
	d.Node("b", "b")
	d.Edge("a", "b")
	text := d.Layout().Render(cligram.State{
		Status: map[string]cligram.Status{"a": cligram.Done},
		Taken:  []cligram.EdgeRef{{From: "a", To: "b"}},
	}, cligram.ANSI)
	pic, err := reader.Read(text)
	if err != nil {
		t.Fatal(err)
	}
	if pic.Boxes[0].BorderSGR != "32" || pic.Boxes[0].Marker != "✓" || pic.Boxes[1].BorderSGR != "" {
		t.Errorf("boxes %+v", pic.Boxes)
	}
	if pic.Edges[0].ArrowSGR != "32" {
		t.Errorf("edge %+v", pic.Edges[0])
	}
}

func TestEveryWholeGoldenReadsBack(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/*.golden")
	for _, f := range files {
		name := filepath.Base(f)
		if strings.HasPrefix(name, "scene.") || strings.Contains(name, "ascii") || name == "wide.golden" ||
			name == "factory.fit80x24.golden" {
			continue // hand-placed scenes, ASCII, a lone box, a cropped view
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Read(string(b)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestFramesAreReadWithTheirTitleAndLinesAcrossThem(t *testing.T) {
	drawing := strings.Join([]string{
		"           ╭╌ Build [x] ╌╌╌╌╌╌╌╌╌╌╌╌╮",
		"           ╎                        ╎",
		"╭───────╮  ╎   ╭───────────╮        ╎",
		"│   Out ├─────►│   Compile │        ╎",
		"╰───────╯  ╎   ╰───┬───────╯        ╎",
		"           ╎       │                ╎",
		"           ╰╌╌╌╌╌╌╌│╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╯",
		"                   ▼",
		"              ╭─────────╮",
		"              │   After │",
		"              ╰─────────╯",
	}, "\n")
	pic, err := reader.Read(drawing)
	if err != nil {
		t.Fatal(err)
	}
	if len(pic.Frames) != 1 || pic.Frames[0].Title != "Build [x]" {
		t.Fatalf("frames: %+v", pic.Frames)
	}
	f := pic.Frames[0]
	inside := 0
	for _, b := range pic.Boxes {
		if f.Contains(b.X, b.Y) {
			inside++
		}
	}
	if len(pic.Boxes) != 3 || inside != 1 || len(pic.Edges) != 2 {
		t.Errorf("boxes %d, %d inside; edges %+v", len(pic.Boxes), inside, pic.Edges)
	}
}

// A line joining two boxes with no arrowhead is a link between them, its
// label its own; one joining three is no link anyone can read.
func TestALinkJoinsTwoBoxesWithNoWay(t *testing.T) {
	text := `╭─────╮          ╭─────╮
│   a ├─[ peers ]┤   b │
╰─────╯          ╰─────╯`
	pic, err := reader.Read(text)
	if err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	if len(pic.Edges) != 1 {
		t.Fatalf("edges %+v", pic.Edges)
	}
	e := pic.Edges[0]
	if !e.Undirected || pic.Boxes[e.From].Text() != "a" || pic.Boxes[e.To].Text() != "b" || e.Label != "peers" {
		t.Errorf("read %+v", e)
	}

	three := `╭─────╮    ╭─────╮
│   a ├──┬─┤   b │
╰─────╯  │ ╰─────╯
      ╭──┴──╮
      │   c │
      ╰─────╯`
	if _, err := reader.Read(three); err == nil || !strings.Contains(err.Error(), "leaves 3 boxes") {
		t.Errorf("a line joining three boxes: %v", err)
	}
}
