package cligram_test

// TestExportPixelCases writes cases for the pixel checks: random drawings
// from the harness, the factory loop and a sequence diagram with every
// glyph one draws, each as the ANSI a terminal is given and the cells it
// should show. It runs only when told where:
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
	"github.com/isacikgoz/cligram/sequence"
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
	writeANSI := func(name, ansi string, w, h int) {
		grid := reader.Parse(ansi)
		c := pixelCase{Name: name, Cols: w, Rows: h, ANSI: ansi, Cells: grid.Cells}
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "cases", name+".json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name string, l *cligram.Layout, st cligram.State) {
		writeANSI(name, l.Render(st, theme), l.W, l.H)
	}
	seq := checkout().Layout()
	writeANSI("sequence", seq.Render(theme), seq.W, seq.H)
	write("factory", factory().Layout(), running)
	write("factory-200x50", factory().Layout(cligram.Fit(200, 50)), running)
	write("factory-down", factory().Layout(cligram.WithOrientation(cligram.TopToBottom)), running)
	for seed := range uint64(n) {
		c := generate(seed)
		write(fmt.Sprintf("seed-%04d", seed), c.d.Layout(c.opts...), c.states[0])
	}
}

// checkout is a sequence diagram with every glyph one is drawn with: both
// kinds of box, every line style and head, a message both ways and one
// to itself, activations, notes and nested blocks, and wide text.
func checkout() *sequence.Diagram {
	d := sequence.New()
	d.Participant("u", "Shopper", sequence.Actor())
	d.Participant("w", "Web app")
	d.Participant("p", "支払い")
	d.Autonumber(1, 1)
	d.Message("u", "w", "Place order 🛒", sequence.Activate())
	d.Message("w", "w", "Check the cart")
	d.Block("loop", "until paid")
	d.Message("w", "p", "Charge card", sequence.Activate())
	d.Block("alt", "accepted")
	d.Message("p", "w", "Paid", sequence.Reply(), sequence.Deactivate())
	d.Section("else", "declined")
	d.Message("p", "w", "Declined", sequence.Reply(), sequence.WithHead(sequence.Cross))
	d.Note("The bank said no", sequence.RightOf("p"))
	d.End()
	d.End()
	d.Message("w", "p", "Sync", sequence.Line(cligram.Thick), sequence.WithHead(sequence.Open))
	d.Message("u", "p", "Chat", sequence.BothWays())
	d.Message("w", "u", "Confirmed", sequence.Reply(), sequence.Deactivate())
	d.Note("An email follows", sequence.Over("u", "w"))
	d.Note("Done", sequence.LeftOf("u"))
	return d
}

func TestThePixelSequenceIsWellMade(t *testing.T) {
	if err := checkout().Check(); err != nil {
		t.Fatal(err)
	}
}
