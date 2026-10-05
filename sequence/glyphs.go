package sequence

import "github.com/isacikgoz/cligram"

// Glyphs are what a sequence diagram is drawn with: a flowchart's glyphs,
// and a few only a sequence diagram needs.
type Glyphs struct {
	*cligram.Glyphs
	// Active is an active participant's lifeline; ActiveRight and
	// ActiveLeft are it where a message leaves it to that side.
	Active, ActiveRight, ActiveLeft string
	// Cross ends a message that is lost or refused.
	Cross string
}

// Unicode draws with box drawing characters, as cligram.Unicode does.
var Unicode = &Glyphs{Glyphs: cligram.Unicode, Active: "┃", ActiveRight: "┠", ActiveLeft: "┨", Cross: "╳"}

// ASCII draws with nothing outside ASCII, as cligram.ASCII does.
var ASCII = &Glyphs{Glyphs: cligram.ASCII, Active: "#", ActiveRight: "#", ActiveLeft: "#", Cross: "x"}

// Line directions, as cligram.Glyphs.Lines indexes them.
const (
	north = 1 << iota
	east
	south
	west
)
