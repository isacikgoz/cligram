// Package pixels checks cligram drawings as a real terminal shows them.
//
// harness/pixels renders cases in xterm.js, in real fonts, and saves a
// screenshot of each, with an atlas: every glyph and color the cases use,
// each drawn on its own. This package cuts a screenshot into its cells and
// compares each cell with the atlas: a cell that does not look like the
// glyph it should be means the terminal drew something else there, most
// often because it measured a character's width differently and every cell
// after it moved. It also checks each glyph the font draws: that it has
// one at all, and that a line reaches the edges of its cell where lines
// join.
package pixels

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Shot is a screenshot of a terminal, and where its cells are in it.
type Shot struct {
	Image      *image.RGBA
	Cols, Rows int
	// CellW and CellH are a cell's size in pixels; cells lie on a grid
	// from the image's top left.
	CellW, CellH float64
}

type geometry struct {
	Width, Height float64
	Cols, Rows    int
	Scale         float64
}

// LoadShot reads dir/name.png and the geometry in dir/name.json.
func LoadShot(dir, name string) (*Shot, error) {
	b, err := os.ReadFile(filepath.Join(dir, name+".png"))
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	var g geometry
	b, err = os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	// The cells fill the screenshot: its size in pixels is exact where the
	// page's size in points was rounded.
	return &Shot{
		Image: rgba(img), Cols: g.Cols, Rows: g.Rows,
		CellW: float64(img.Bounds().Dx()) / float64(g.Cols),
		CellH: float64(img.Bounds().Dy()) / float64(g.Rows),
	}, nil
}

// Cell is the rectangle of w cells from column x of row y.
func (s *Shot) Cell(x, y, w int) image.Rectangle {
	b := s.Image.Bounds()
	return image.Rect(
		b.Min.X+int(math.Round(float64(x)*s.CellW)), b.Min.Y+int(math.Round(float64(y)*s.CellH)),
		b.Min.X+int(math.Round(float64(x+w)*s.CellW)), b.Min.Y+int(math.Round(float64(y+1)*s.CellH)),
	)
}

func rgba(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out
}

// Diff is how unlike two same-sized patches are: the mean difference of
// their pixels' channels, from 0 for alike to 255. Pixels outside an image
// count as background.
func Diff(a *image.RGBA, ra image.Rectangle, b *image.RGBA, rb image.Rectangle) float64 {
	w, h := min(ra.Dx(), rb.Dx()), min(ra.Dy(), rb.Dy())
	if w <= 0 || h <= 0 {
		return 255
	}
	px := func(img *image.RGBA, x, y int) []uint8 {
		if !(image.Point{x, y}.In(img.Rect)) {
			return []uint8{0, 0, 0}
		}
		i := img.PixOffset(x, y)
		return img.Pix[i : i+3]
	}
	var sum int
	for y := range h {
		for x := range w {
			p, q := px(a, ra.Min.X+x, ra.Min.Y+y), px(b, rb.Min.X+x, rb.Min.Y+y)
			for c := range 3 {
				d := int(p[c]) - int(q[c])
				if d < 0 {
					d = -d
				}
				sum += d
			}
		}
	}
	return float64(sum) / float64(w*h*3)
}

// Ink is whether a pixel is drawn on, against a black background.
func Ink(img *image.RGBA, x, y int) bool {
	if !(image.Point{x, y}.In(img.Rect)) {
		return false
	}
	i := img.PixOffset(x, y)
	return max(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) > 60
}

// Inked reports whether any pixel of r is drawn on.
func Inked(img *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if Ink(img, x, y) {
				return true
			}
		}
	}
	return false
}

// Item is a glyph in the atlas, by its cell.
type Item struct {
	G    string `json:"g"`
	SGR  string `json:"sgr"`
	W    int    `json:"w"` // the cells it takes, as cligram measured
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Tofu bool   `json:"tofu"`
}

// Atlas is every glyph and color the cases use, each drawn on its own.
type Atlas struct {
	*Shot
	Items map[string]Item // by Key
	Tofu  *Item           // a glyph no font has
}

// Key names a glyph in a color.
func Key(g, sgr string) string { return sgr + "\x00" + g }

// LoadAtlas reads the atlas a render left in dir.
func LoadAtlas(dir string) (*Atlas, error) {
	s, err := LoadShot(dir, "_atlas")
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "_atlas.items.json"))
	if err != nil {
		return nil, err
	}
	var items []Item
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	a := &Atlas{Shot: s, Items: map[string]Item{}}
	for _, it := range items {
		if it.Tofu {
			a.Tofu = &it
			continue
		}
		a.Items[Key(it.G, it.SGR)] = it
	}
	return a, nil
}

// Line glyphs, by the edges of their cell their strokes reach.
const (
	N = 1 << iota
	E
	S
	W
)

// Strokes are the edges each line and border glyph reaches.
var Strokes = map[string]int{}

func init() {
	sets := []struct {
		glyphs string
		masks  []int
	}{
		// Order: │ ─ ┌ ┐ └ ┘ ├ ┤ ┬ ┴ ┼
		{"│─┌┐└┘├┤┬┴┼", []int{N | S, E | W, E | S, S | W, N | E, N | W, N | E | S, N | S | W, E | S | W, N | E | W, N | E | S | W}},
		{"║═╔╗╚╝╟╢╤╧", []int{N | S, E | W, E | S, S | W, N | E, N | W, N | E | S, N | S | W, E | S | W, N | E | W}},
		{"┃━┏┓┗┛┠┨┯┷", []int{N | S, E | W, E | S, S | W, N | E, N | W, N | E | S, N | S | W, E | S | W, N | E | W}},
		// Rounded corners reach the edges as square ones do.
		{"╭╮╰╯", []int{E | S, S | W, N | E, N | W}},
	}
	for _, set := range sets {
		for i, g := range strings.Split(set.glyphs, "") {
			Strokes[g] = set.masks[i]
		}
	}
}

// Reaches reports whether a glyph drawn in r reaches the edge of r on
// side, near the middle of that edge, where a joining line would meet it.
func Reaches(img *image.RGBA, r image.Rectangle, side int) bool {
	// The middle third of the edge, the outermost two pixels.
	switch side {
	case N, S:
		x0, x1 := r.Min.X+r.Dx()/3, r.Max.X-r.Dx()/3
		y := r.Min.Y
		if side == S {
			y = r.Max.Y - 2
		}
		return Inked(img, image.Rect(x0, y, x1, y+2))
	default:
		y0, y1 := r.Min.Y+r.Dy()/3, r.Max.Y-r.Dy()/3
		x := r.Min.X
		if side == E {
			x = r.Max.X - 2
		}
		return Inked(img, image.Rect(x, y0, x+2, y1))
	}
}
