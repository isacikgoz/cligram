package pixels

// The tests here look at the screenshots harness/pixels made, so run that
// first; without them they skip:
//
//	make pixels

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram/internal/reader"
)

// How unlike its template a cell may look, out of 255, once lined up. A
// cell drawn as its glyph comes within 1 or 2, and the nearest other glyph
// is 5 or more away; but bold text is thickened by the browser and spills
// into its neighbors, so a cell need only look more like its own glyph
// than any other (within margin, for glyphs only a weight apart), and
// not unlike all of them.
const (
	sameGlyph = 4.0
	looksLike = 20.0
	margin    = 1.0
	// spill is the pixels at a cell's sides left out of comparing it.
	spill = 2
	// slack is how far, in pixels, a glyph may sit from where its cell
	// says; a cell the terminal measured differently is a whole cell,
	// near 20 pixels, off.
	slack = 1
)

// out is where harness/pixels left its renders.
func out(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("CLIGRAM_PIXELS_OUT")
	if dir == "" {
		dir = filepath.Join("..", "..", "harness", "pixels", "out")
	}
	if _, err := os.Stat(filepath.Join(dir, "shots")); err != nil {
		t.Skip("no screenshots: run make pixels")
	}
	return dir
}

type pixelCase struct {
	Name  string          `json:"name"`
	Cells [][]reader.Cell `json:"cells"`
}

func fonts(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "shots"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func cases(t *testing.T, dir string) []pixelCase {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "cases", "*.json"))
	var out []pixelCase
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var c pixelCase
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		c.Name = strings.TrimSuffix(filepath.Base(f), ".json")
		out = append(out, c)
	}
	return out
}

// width is how many cells the glyph at x takes in row.
func width(row []reader.Cell, x int) int {
	if x+1 < len(row) && row[x+1].Cont {
		return 2
	}
	return 1
}

// TestEveryGlyphIsDrawn checks each glyph the cases use, as each font
// draws it on its own: that it has ink, that it is not the box a missing
// glyph is drawn as, and that a line reaches its cell's edges wherever it
// meets another, so lines join with no gap.
func TestEveryGlyphIsDrawn(t *testing.T) {
	dir := out(t)
	for _, font := range fonts(t, dir) {
		t.Run(font, func(t *testing.T) {
			a, err := LoadAtlas(filepath.Join(dir, "shots", font))
			if err != nil {
				t.Fatal(err)
			}
			var tofu image.Rectangle
			if a.Tofu != nil {
				tofu = a.Cell(a.Tofu.X, a.Tofu.Y, 1)
			}
			for _, it := range a.Items {
				w := it.W
				r := a.Cell(it.X, it.Y, w)
				if !Inked(a.Image, r) {
					t.Errorf("%q is drawn blank", it.G)
					continue
				}
				if a.Tofu != nil && w == 1 && Inked(a.Image, tofu) && Diff(a.Image, r, a.Image, tofu) < sameGlyph {
					t.Errorf("%q is drawn as a missing glyph", it.G)
				}
				for _, side := range []int{N, E, S, W} {
					if Strokes[it.G]&side != 0 && !Reaches(a.Image, r, side) {
						t.Errorf("%q (color %q) stops short of its cell's %s edge, so a line meeting it there shows a gap",
							it.G, it.SGR, map[int]string{N: "top", E: "right", S: "bottom", W: "left"}[side])
					}
				}
			}
		})
	}
}

// TestEveryCellLooksAsItShould cuts each screenshot into cells and checks
// each looks like the glyph, in the color, it should show. A terminal that
// took a character for wider or narrower than cligram did moves every cell
// after it a whole cell, and none of them match.
func TestEveryCellLooksAsItShould(t *testing.T) {
	dir := out(t)
	cs := cases(t, dir)
	for _, font := range fonts(t, dir) {
		t.Run(font, func(t *testing.T) {
			a, err := LoadAtlas(filepath.Join(dir, "shots", font))
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range cs {
				s, err := LoadShot(filepath.Join(dir, "shots", font), c.Name)
				if err != nil {
					t.Fatal(err)
				}
				if bad := mismatches(a, s, c); len(bad) > 0 {
					t.Errorf("%s: %d cells do not look as they should, first:\n  %s",
						c.Name, len(bad), strings.Join(bad[:min(8, len(bad))], "\n  "))
				}
			}
		})
	}
}

// mismatches are the cells of c that s does not show as they should be:
// those that look more like another glyph than their own, or like none.
func mismatches(a *Atlas, s *Shot, c pixelCase) []string {
	var bad []string
	for y, row := range c.Cells {
		for x, cell := range row {
			if cell.Cont || cell.G == " " || cell.G == "" {
				continue
			}
			w := width(row, x)
			it, ok := a.Items[Key(cell.G, cell.SGR)]
			if !ok {
				bad = append(bad, fmt.Sprintf("(%d,%d) %q is not in the atlas", x, y, cell.G))
				continue
			}
			r, tr := inside(s.Cell(x, y, w)), inside(a.Cell(it.X, it.Y, w))
			own := aligned(s.Image, r, a.Image, tr)
			if own <= sameGlyph {
				continue // drawn as its template, near enough pixel for pixel
			}
			guesses := likest(a, s.Image, r, w)
			if own <= looksLike && own <= guesses[0].d+margin {
				continue
			}
			var names []string
			for _, g := range guesses[:min(3, len(guesses))] {
				sgr, glyph, _ := strings.Cut(g.key, "\x00")
				names = append(names, fmt.Sprintf("%q in %q (%.1f)", glyph, sgr, g.d))
			}
			bad = append(bad, fmt.Sprintf("(%d,%d) should be %q in %q (%.1f); it looks most like %s",
				x, y, cell.G, cell.SGR, own, strings.Join(names, ", ")))
		}
	}
	return bad
}

// inside is a cell less the pixels at its sides, where a bold neighbor's
// ink spills over.
func inside(r image.Rectangle) image.Rectangle {
	return image.Rect(r.Min.X+spill, r.Min.Y, r.Max.X-spill, r.Max.Y)
}

// aligned is how unlike r in a is tr in b, at the best shift within slack.
func aligned(a *image.RGBA, r image.Rectangle, b *image.RGBA, tr image.Rectangle) float64 {
	best := 255.0
	for dy := -slack; dy <= slack; dy++ {
		for dx := -slack; dx <= slack; dx++ {
			best = min(best, Diff(a, r.Add(image.Pt(dx, dy)), b, tr))
		}
	}
	return best
}

type guess struct {
	key string
	d   float64
}

// likest ranks the glyphs of the atlas as wide as w by how like them r
// looks, the likest first, a blank cell among them.
func likest(a *Atlas, img *image.RGBA, r image.Rectangle, w int) []guess {
	var gs []guess
	for k, it := range a.Items {
		if it.W != w {
			continue
		}
		gs = append(gs, guess{k, aligned(img, r, a.Image, inside(a.Cell(it.X, it.Y, w)))})
	}
	gs = append(gs, guess{Key(" ", ""), Diff(img, r, a.Image, inside(a.Cell(0, 0, w)))})
	sort.Slice(gs, func(i, j int) bool { return gs[i].d < gs[j].d })
	return gs
}

// TestTheChecksSeeWhatIsWrong feeds the checks screenshots with the wrong
// expectations, to show they fail: a row a cell out, as a terminal that
// measured a character differently would draw it; a color that is not the
// one drawn; and the glyph a font does not have.
func TestTheChecksSeeWhatIsWrong(t *testing.T) {
	dir := out(t)
	font := fonts(t, dir)[0]
	a, err := LoadAtlas(filepath.Join(dir, "shots", font))
	if err != nil {
		t.Fatal(err)
	}
	var c pixelCase
	for _, x := range cases(t, dir) {
		if x.Name == "factory" {
			c = x
		}
	}
	s, err := LoadShot(filepath.Join(dir, "shots", font), c.Name)
	if err != nil {
		t.Fatal(err)
	}
	if bad := mismatches(a, s, c); len(bad) > 0 {
		t.Fatalf("the factory loop itself does not match: %v", bad[:min(3, len(bad))])
	}
	// A row whose glyphs sit a cell to the right of where they are drawn.
	y := 4
	shifted := c
	shifted.Cells = append([][]reader.Cell(nil), c.Cells...)
	shifted.Cells[y] = append([]reader.Cell{{G: " "}}, c.Cells[y]...)
	if bad := mismatches(a, s, shifted); len(bad) < 10 {
		t.Errorf("a row a cell out: only %d cells seen wrong", len(bad))
	}
	// A border in the wrong color.
	recolored := c
	recolored.Cells = append([][]reader.Cell(nil), c.Cells...)
	row := append([]reader.Cell(nil), c.Cells[y]...)
	for x := range row {
		if row[x].G == "║" {
			row[x].SGR = "31"
			break
		}
	}
	recolored.Cells[y] = row
	if bad := mismatches(a, s, recolored); len(bad) != 1 || !strings.Contains(bad[0], `"║" in "31"`) {
		t.Errorf("a border in the wrong color: %v", bad)
	}
	// The glyph no font has is told from every glyph the cases use.
	if a.Tofu == nil || !Inked(a.Image, a.Cell(a.Tofu.X, a.Tofu.Y, 1)) {
		t.Skip("this browser draws a missing glyph blank: the ink check covers it")
	}
	tofu := a.Cell(a.Tofu.X, a.Tofu.Y, 1)
	for _, it := range a.Items {
		if it.W == 1 && Diff(a.Image, a.Cell(it.X, it.Y, 1), a.Image, tofu) < sameGlyph {
			t.Errorf("%q looks like a missing glyph", it.G)
		}
	}
}
