package cligram

// Every glyph here is one cell wide. Box drawing characters are East Asian
// "ambiguous" width, which no terminal draws two cells wide unless told
// to; for one that is, ASCII is the fallback. The arrowheads and markers
// are chosen from glyphs that are narrow everywhere and are never drawn as
// emoji: ▸ rather than ▶, which some terminals turn into an emoji.

// Line directions, as bits: a cell's lines are the directions they leave it in.
const (
	north uint8 = 1 << iota
	east
	south
	west
)

// Borders are the glyphs one kind of box is drawn with.
type Borders struct {
	TopLeft, TopRight, BottomLeft, BottomRight string
	Horizontal, Vertical                       string
	// Where an edge leaves through a side: the border with a line out of it.
	OutTop, OutBottom, OutLeft, OutRight string
}

// Glyphs are the characters a diagram is drawn with.
type Glyphs struct {
	// Lines are edge glyphs by the directions they join, a mask of
	// north, east, south and west.
	Lines [16]string
	// Arrows are arrowheads pointing up, right, down and left.
	Arrows [4]string
	// Boxes are the borders of each kind of node.
	Boxes map[Kind]Borders
	// Markers show a node's status in front of its text, so status reads
	// without color too. Every marker is as wide as every other.
	Markers map[Status]string
	// LabelOpen and LabelClose bracket a label sitting on a line.
	LabelOpen, LabelClose string
	// Ellipsis ends a text cut to fit.
	Ellipsis string
}

// Unicode draws with box drawing characters: rounded steps, double-bordered
// decisions, heavy terminals.
var Unicode = &Glyphs{
	Lines: [16]string{
		"",
		north:                       "│",
		east:                        "─",
		north | east:                "└",
		south:                       "│",
		north | south:               "│",
		east | south:                "┌",
		north | east | south:        "├",
		west:                        "─",
		north | west:                "┘",
		east | west:                 "─",
		north | east | west:         "┴",
		south | west:                "┐",
		north | south | west:        "┤",
		east | south | west:         "┬",
		north | east | south | west: "┼",
	},
	Arrows: [4]string{"▴", "▸", "▾", "◂"},
	Boxes: map[Kind]Borders{
		Step: {
			TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯",
			Horizontal: "─", Vertical: "│",
			OutTop: "┴", OutBottom: "┬", OutLeft: "┤", OutRight: "├",
		},
		Decision: {
			TopLeft: "╔", TopRight: "╗", BottomLeft: "╚", BottomRight: "╝",
			Horizontal: "═", Vertical: "║",
			OutTop: "╧", OutBottom: "╤", OutLeft: "╢", OutRight: "╟",
		},
		Terminal: {
			TopLeft: "┏", TopRight: "┓", BottomLeft: "┗", BottomRight: "┛",
			Horizontal: "━", Vertical: "┃",
			OutTop: "┷", OutBottom: "┯", OutLeft: "┨", OutRight: "┠",
		},
	},
	Markers: map[Status]string{
		Idle:    " ",
		Active:  "▸",
		Done:    "✓",
		Failed:  "✗",
		Waiting: "◔",
	},
	LabelOpen:  "[ ",
	LabelClose: " ]",
	Ellipsis:   "…",
}

// ASCII draws with nothing outside ASCII, for terminals and logs that
// cannot be trusted with anything else.
var ASCII = &Glyphs{
	Lines: [16]string{
		"",
		north:                       "|",
		east:                        "-",
		north | east:                "+",
		south:                       "|",
		north | south:               "|",
		east | south:                "+",
		north | east | south:        "+",
		west:                        "-",
		north | west:                "+",
		east | west:                 "-",
		north | east | west:         "+",
		south | west:                "+",
		north | south | west:        "+",
		east | south | west:         "+",
		north | east | south | west: "+",
	},
	Arrows: [4]string{"^", ">", "v", "<"},
	Boxes: map[Kind]Borders{
		Step: {
			TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
			Horizontal: "-", Vertical: "|",
			OutTop: "+", OutBottom: "+", OutLeft: "+", OutRight: "+",
		},
		Decision: {
			TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
			Horizontal: "=", Vertical: "|",
			OutTop: "+", OutBottom: "+", OutLeft: "+", OutRight: "+",
		},
		Terminal: {
			TopLeft: "#", TopRight: "#", BottomLeft: "#", BottomRight: "#",
			Horizontal: "-", Vertical: "|",
			OutTop: "+", OutBottom: "+", OutLeft: "+", OutRight: "+",
		},
	},
	Markers: map[Status]string{
		Idle:    " ",
		Active:  ">",
		Done:    "*",
		Failed:  "x",
		Waiting: "~",
	},
	LabelOpen:  "[ ",
	LabelClose: " ]",
	Ellipsis:   "...",
}
