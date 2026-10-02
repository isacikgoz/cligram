package cligram_test

// TestExportPixelCases writes cases for the pixel checks: random drawings
// from the harness and the factory loop, each as the ANSI a terminal is
// given and the cells it should show. It runs only when told where:
//
//	CLIGRAM_PIXELS_OUT=harness/pixels/out go test -run TestExportPixelCases .

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/internal/reader"
)

// pixelCase is one drawing for a terminal to render.
type pixelCase struct {
	Name  string          `json:"name"`
	Cols  int             `json:"cols"`
	Rows  int             `json:"rows"`
	ANSI  string          `json:"ansi"`
	Cells [][]reader.Cell `json:"cells"`
}

func TestExportPixelCases(t *testing.T) {
	out := os.Getenv("CLIGRAM_PIXELS_OUT")
	if out == "" {
		t.Skip("CLIGRAM_PIXELS_OUT is not set")
	}
	n := 40
	if v := os.Getenv("CLIGRAM_PIXELS_CASES"); v != "" {
		_, _ = fmt.Sscan(v, &n)
	}
	if err := os.MkdirAll(filepath.Join(out, "cases"), 0o755); err != nil {
		t.Fatal(err)
	}
	theme := harnessPalette.Theme()
	write := func(name string, l *cligram.Layout, st cligram.State) {
		ansi := l.Render(st, theme)
		grid := reader.Parse(ansi)
		c := pixelCase{Name: name, Cols: l.W, Rows: l.H, ANSI: ansi, Cells: grid.Cells}
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "cases", name+".json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("factory", factory().Layout(), running)
	write("factory-200x50", factory().Layout(cligram.Fit(200, 50)), running)
	write("factory-down", factory().Layout(cligram.WithOrientation(cligram.TopToBottom)), running)
	for seed := range uint64(n) {
		c := generate(seed)
		write(fmt.Sprintf("seed-%04d", seed), c.d.Layout(c.opts...), c.states[0])
	}
}
