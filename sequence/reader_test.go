package sequence

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rivo/uniseg"
)

// The reader reads a sequence diagram from its text alone, as a person
// would: participants by their boxes, lifelines down from them, messages
// by the line from one lifeline to another and the words above it,
// notes, and frames round blocks. It knows only what the glyphs mean,
// never the layout's tables, and every glyph must be explained by
// something it read.

// seen is a sequence diagram as read from a drawing.
type seen struct {
	heads  []seenHead
	events []seenEvent // top to bottom
	active map[int][]int
}

type seenHead struct {
	text  string
	actor bool
	x     int
}

type seenEvent struct {
	kind     string // message, note, open, section, close
	row      int    // its first row
	from, to int    // participants; a note's lowest and highest
	text     string
	line     string // solid, dashed, thick
	head     string // arrow, open, cross
	both     bool
	side     string // a note's: right, left, over
	x0, x1   int    // the columns it covers
	arrowRow int
	// active is whether each lifeline the message leaves or reaches is
	// drawn active where it does; a message to itself leaves and comes
	// back in different rows.
	active                  map[int]bool
	leaveActive, backActive bool
}

// page is a drawing as cells, wide graphemes one cell with an empty one
// after them.
type page struct {
	cells [][]string
	used  [][]bool
}

func newPage(drawing string) *page {
	p := &page{}
	for _, line := range strings.Split(drawing, "\n") {
		var row []string
		g := uniseg.NewGraphemes(line)
		for g.Next() {
			row = append(row, g.Str())
			for range g.Width() - 1 {
				row = append(row, "")
			}
		}
		p.cells = append(p.cells, row)
		p.used = append(p.used, make([]bool, len(row)))
	}
	return p
}

func (p *page) at(x, y int) string {
	if y < 0 || y >= len(p.cells) || x < 0 || x >= len(p.cells[y]) {
		return " "
	}
	return p.cells[y][x]
}

func (p *page) isUsed(x, y int) bool {
	return y >= 0 && y < len(p.used) && x >= 0 && x < len(p.used[y]) && p.used[y][x]
}

func (p *page) use(x, y int) {
	if y >= 0 && y < len(p.used) && x >= 0 && x < len(p.used[y]) {
		p.used[y][x] = true
	}
}

func oneOf(s string, set string) bool { return s != "" && s != " " && strings.Contains(set, s) }

const (
	lifeGlyphs   = "│┃"
	acrossGlyphs = "─┄━"
	structural   = "─┄━►◄╳├┤┠┨┐┘┌└╭╮╰╯╌╎┊┬┯┴┷┏┓┗┛━│┃"
)

// heavy reports whether a lifeline glyph is an active one: its stroke
// down is heavy.
func heavy(g string) bool { return oneOf(g, "┃┠┨") }

func lineOf(g string) string {
	switch g {
	case "┄", "┊":
		return "dashed"
	case "━", "┃":
		return "thick"
	}
	return "solid"
}

// read reads a drawing, or says why it cannot.
func read(drawing string) (*seen, error) {
	p := newPage(drawing)
	s := &seen{active: map[int][]int{}}
	// The heads' bottom row is the first with a lifeline leaving a box.
	top := -1
	for y := range p.cells {
		for x := range p.cells[y] {
			if oneOf(p.at(x, y), "┬┯") {
				top = y
				break
			}
		}
		if top >= 0 {
			break
		}
	}
	if top < 0 {
		return nil, fmt.Errorf("no lifeline leaves a box")
	}
	for x := range p.cells[top] {
		if oneOf(p.at(x, top), "┬┯") {
			h, err := p.readHead(x, top)
			if err != nil {
				return nil, err
			}
			s.heads = append(s.heads, h)
		}
	}
	lifeAt := map[int]int{}
	for i, h := range s.heads {
		lifeAt[h.x] = i
	}
	body := top + 1
	for y := body; y < len(p.cells); y++ {
		for x := 0; x < len(p.cells[y]); x++ {
			g := p.at(x, y)
			if p.isUsed(x, y) && g != "╎" {
				continue
			}
			switch {
			case g == "┌":
				ev, err := p.readNote(x, y, s.heads)
				if err != nil {
					return nil, err
				}
				s.events = append(s.events, ev)
			case (g == "╭" || g == "╎" || g == "╰") && p.at(x+1, y) == "╌" && !p.isUsed(x+1, y):
				ev, err := p.readFrameRow(x, y)
				if err != nil {
					return nil, err
				}
				s.events = append(s.events, ev)
			}
		}
		for i, h := range s.heads {
			x := h.x
			if p.isUsed(x, y) {
				continue
			}
			g := p.at(x, y)
			switch {
			case oneOf(g, "├┠┤┨") || (oneOf(g, lifeGlyphs) && p.at(x+1, y) == "◄" && !p.isUsed(x+1, y)):
				ev, err := p.readMessage(i, y, s.heads, lifeAt)
				if err != nil {
					return nil, err
				}
				if ev != nil {
					s.events = append(s.events, *ev)
				}
			}
		}
	}
	// What is left on the lifelines is lifeline; anything else left is a
	// glyph nothing explains.
	for i, h := range s.heads {
		for y := body; y < len(p.cells); y++ {
			g := p.at(h.x, y)
			if oneOf(g, "┃┠┨") {
				s.active[i] = append(s.active[i], y)
			}
			if oneOf(g, lifeGlyphs) {
				p.use(h.x, y)
			}
		}
	}
	for y := range p.cells {
		for x, g := range p.cells[y] {
			if g != "" && g != " " && !p.used[y][x] {
				return nil, fmt.Errorf("(%d,%d) %q is part of nothing read", x, y, g)
			}
		}
	}
	slices.SortStableFunc(s.events, func(a, b seenEvent) int { return a.row - b.row })
	return s, nil
}

// readHead reads the participant box a lifeline leaves at (x, y).
func (p *page) readHead(x, y int) (seenHead, error) {
	actor := p.at(x, y) == "┯"
	l, r := x, x
	for l >= 0 && !oneOf(p.at(l, y), "╰┗") {
		l--
	}
	for r < len(p.cells[y]) && !oneOf(p.at(r, y), "╯┛") {
		r++
	}
	if l < 0 || r >= len(p.cells[y]) {
		return seenHead{}, fmt.Errorf("the box over the lifeline at column %d has no bottom corners", x)
	}
	t := y - 1
	for t >= 0 && !oneOf(p.at(l, t), "╭┏") {
		t--
	}
	if t < 0 || !oneOf(p.at(r, t), "╮┓") {
		return seenHead{}, fmt.Errorf("the box over the lifeline at column %d has no top", x)
	}
	var words []string
	for yy := t; yy <= y; yy++ {
		for xx := l; xx <= r; xx++ {
			p.use(xx, yy)
		}
		if yy > t && yy < y {
			words = append(words, p.span(l+1, r-1, yy))
		}
	}
	return seenHead{text: strings.Join(words, " "), actor: actor, x: x}, nil
}

// span is the text in row y from x0 to x1, trimmed.
func (p *page) span(x0, x1, y int) string {
	var b strings.Builder
	for x := x0; x <= x1; x++ {
		g := p.at(x, y)
		if g == "" {
			continue
		}
		b.WriteString(g)
	}
	return strings.TrimSpace(b.String())
}

// readNote reads the note whose top left corner is at (x, y).
func (p *page) readNote(x, y int, heads []seenHead) (seenEvent, error) {
	r := x + 1
	for oneOf(p.at(r, y), "─") {
		r++
	}
	if p.at(r, y) != "┐" {
		return seenEvent{}, fmt.Errorf("(%d,%d): a note's top has no right corner", x, y)
	}
	b := y + 1
	for p.at(x, b) == "│" && p.at(r, b) == "│" {
		b++
	}
	if p.at(x, b) != "└" || p.at(r, b) != "┘" {
		return seenEvent{}, fmt.Errorf("(%d,%d): a note's sides end without corners", x, y)
	}
	var words []string
	for yy := y; yy <= b; yy++ {
		for xx := x; xx <= r; xx++ {
			if (yy == y || yy == b) && xx > x && xx < r && p.at(xx, yy) != "─" {
				return seenEvent{}, fmt.Errorf("(%d,%d): a note's border is broken", xx, yy)
			}
			p.use(xx, yy)
		}
		if yy > y && yy < b {
			words = append(words, p.span(x+1, r-1, yy))
		}
	}
	ev := seenEvent{kind: "note", row: y, text: strings.Join(words, " "), x0: x, x1: r}
	var inside []int
	for i, h := range heads {
		if h.x > x && h.x < r {
			inside = append(inside, i)
		}
	}
	switch {
	case len(inside) > 0:
		ev.side, ev.from, ev.to = "over", inside[0], inside[len(inside)-1]
	default:
		// Beside the lifeline nearer it; as near both, it is anyone's.
		left, right := -1, -1
		for i, h := range heads {
			if h.x < x {
				left = i
			}
			if h.x > r && right < 0 {
				right = i
			}
		}
		dl, dr := 1<<30, 1<<30
		if left >= 0 {
			dl = x - heads[left].x
		}
		if right >= 0 {
			dr = heads[right].x - r
		}
		switch {
		case dl == dr:
			return seenEvent{}, fmt.Errorf("(%d,%d): a note as near the lifelines either side", x, y)
		case dl < dr:
			ev.side, ev.from, ev.to = "right", left, left
		default:
			ev.side, ev.from, ev.to = "left", right, right
		}
	}
	return ev, nil
}

// readFrameRow reads a frame's top, section or bottom row, starting at
// its left side (x, y).
func (p *page) readFrameRow(x, y int) (seenEvent, error) {
	kinds := map[string]string{"╭": "open", "╎": "section", "╰": "close"}
	ends := map[string]string{"╭": "╮", "╎": "╎", "╰": "╯"}
	kind, end := kinds[p.at(x, y)], ends[p.at(x, y)]
	r := x + 1
	var title strings.Builder
	inTitle := false
	for ; r < len(p.cells[y]); r++ {
		g := p.at(r, y)
		if g == end && !inTitle {
			break
		}
		switch {
		case g == "╌" && !inTitle:
		case oneOf(g, lifeGlyphs) && !inTitle:
		case g == " " && !inTitle && p.at(r-1, y) == "╌" && title.Len() == 0:
			inTitle = true
		case g == " " && inTitle && (p.at(r+1, y) == "╌" || p.at(r+1, y) == end):
			inTitle = false
		case inTitle:
			title.WriteString(g)
		default:
			return seenEvent{}, fmt.Errorf("(%d,%d): %q in a frame's border", r, y, g)
		}
	}
	if r >= len(p.cells[y]) {
		return seenEvent{}, fmt.Errorf("(%d,%d): a frame's row has no right side", x, y)
	}
	for xx := x; xx <= r; xx++ {
		p.use(xx, y)
	}
	ev := seenEvent{kind: kind, row: y, text: title.String(), x0: x, x1: r}
	if kind == "open" {
		// Its sides go down to its bottom.
		b := y + 1
		for ; b < len(p.cells); b++ {
			if p.at(x, b) == "╰" && p.at(r, b) == "╯" {
				break
			}
			if p.at(x, b) != "╎" || p.at(r, b) != "╎" {
				return seenEvent{}, fmt.Errorf("(%d,%d): a frame's side is broken", x, b)
			}
			p.use(x, b)
			p.use(r, b)
		}
		if b == len(p.cells) {
			return seenEvent{}, fmt.Errorf("(%d,%d): a frame never closes", x, y)
		}
	}
	return ev, nil
}

// readMessage reads the message whose line touches lifeline i in row y,
// with its label above it.
func (p *page) readMessage(i, y int, heads []seenHead, lifeAt map[int]int) (*seenEvent, error) {
	x := heads[i].x
	g := p.at(x, y)
	ev := &seenEvent{kind: "message", from: i, arrowRow: y, head: "arrow"}
	step := 1
	if oneOf(g, "┤┨") {
		step = -1
	}
	cx := x + step
	pointsHere := p.at(cx, y) == "◄" && step == 1
	if pointsHere {
		// An arrowhead at this lifeline: the message comes here, or goes
		// both ways.
		p.use(cx, y)
		cx += step
	} else {
		p.use(x, y)
	}
	// Along the line.
	for oneOf(p.at(cx, y), acrossGlyphs) {
		if ev.line == "" {
			ev.line = lineOf(p.at(cx, y))
		} else if ev.line != lineOf(p.at(cx, y)) {
			return nil, fmt.Errorf("(%d,%d): a message's line changes style", cx, y)
		}
		p.use(cx, y)
		cx += step
	}
	if ev.line == "" {
		return nil, fmt.Errorf("(%d,%d): a message with no line", x, y)
	}
	g = p.at(cx, y)
	switch {
	case g == "┐" && step == 1:
		ev.both = pointsHere
		return p.readSelf(i, y, cx, ev)
	case pointsHere && oneOf(g, "┤┨"):
		// Sent from the lifeline at the line's other end.
		j, ok := lifeAt[cx]
		if !ok {
			return nil, fmt.Errorf("(%d,%d): a line ends in a junction on no lifeline", cx, y)
		}
		p.use(cx, y)
		ev.from, ev.to = j, i
	case oneOf(g, "►╳") && step == 1, g == "╳" && step == -1, g == "►" && pointsHere:
		ev.both = pointsHere
		if g == "╳" {
			ev.head = "cross"
		}
		p.use(cx, y)
		cx += step
		j, ok := lifeAt[cx]
		if !ok || !oneOf(p.at(cx, y), lifeGlyphs) {
			return nil, fmt.Errorf("(%d,%d): an arrowhead points at no lifeline", cx-step, y)
		}
		ev.to = j
	case !pointsHere && oneOf(g, "┤┨├┠"):
		j, ok := lifeAt[cx]
		if !ok {
			return nil, fmt.Errorf("(%d,%d): a line ends in a junction on no lifeline", cx, y)
		}
		ev.head = "open"
		ev.to = j
		p.use(cx, y)
	default:
		return nil, fmt.Errorf("(%d,%d): a line from lifeline %d ends in %q", cx, y, i, g)
	}
	lo, hi := min(ev.from, ev.to), max(ev.from, ev.to)
	ev.x0, ev.x1 = heads[lo].x, heads[hi].x
	ev.active = map[int]bool{lo: heavy(p.at(ev.x0, y)), hi: heavy(p.at(ev.x1, y))}
	// The label: the rows of words right above the line, between the
	// lifelines.
	ev.row = y
	var lines []string
	for yy := y - 1; yy >= 0; yy-- {
		var cells []int
		structure := false
		for xx := ev.x0 + 1; xx < ev.x1; xx++ {
			c := p.at(xx, yy)
			_, life := lifeAt[xx]
			switch {
			case c == " " || c == "":
			case life && oneOf(c, lifeGlyphs):
			case oneOf(c, structural) || p.isUsed(xx, yy):
				structure = true
			default:
				cells = append(cells, xx)
			}
		}
		if structure || len(cells) == 0 {
			break
		}
		l, r := cells[0], cells[len(cells)-1]
		for xx := l; xx <= r; xx++ {
			p.use(xx, yy)
		}
		lines = append([]string{p.span(l, r, yy)}, lines...)
		ev.row = yy
	}
	ev.text = strings.Join(lines, " ")
	return ev, nil
}

// readSelf reads a message to its own lifeline, from the corner at
// (cx, y): down, back left to the lifeline, and its label beside it.
func (p *page) readSelf(i, y, cx int, ev *seenEvent) (*seenEvent, error) {
	x := cx - 3
	p.use(cx, y)
	b := y + 1
	for oneOf(p.at(cx, b), "│┊┃") {
		if lineOf(p.at(cx, b)) != ev.line {
			return nil, fmt.Errorf("(%d,%d): a message's line changes style", cx, b)
		}
		p.use(cx, b)
		b++
	}
	if p.at(cx, b) != "┘" {
		return nil, fmt.Errorf("(%d,%d): a message to itself does not come back", cx, b)
	}
	p.use(cx, b)
	for xx := cx - 1; xx > x+1; xx-- {
		if !oneOf(p.at(xx, b), acrossGlyphs) || lineOf(p.at(xx, b)) != ev.line {
			return nil, fmt.Errorf("(%d,%d): a message to itself comes back broken", xx, b)
		}
		p.use(xx, b)
	}
	switch g := p.at(x+1, b); {
	case g == "◄":
	case g == "╳":
		ev.head = "cross"
	case oneOf(g, acrossGlyphs) && oneOf(p.at(x, b), "├┠"):
		ev.head = "open"
		p.use(x, b)
	default:
		return nil, fmt.Errorf("(%d,%d): a message to itself ends in %q", x+1, b, g)
	}
	p.use(x+1, b)
	ev.to = i
	ev.row = y
	ev.leaveActive, ev.backActive = heavy(p.at(x, y)), heavy(p.at(x, b))
	ev.x0, ev.x1 = x, cx
	var lines []string
	for yy := y; yy <= b; yy++ {
		l := cx + 2
		r := l
		for r < len(p.cells[yy]) && (p.at(r, yy) != " " || p.at(r+1, yy) != " ") && !oneOf(p.at(r, yy), structural) {
			r++
		}
		if r == l {
			continue
		}
		for xx := l; xx < r; xx++ {
			p.use(xx, yy)
		}
		lines = append(lines, p.span(l, r-1, yy))
		ev.x1 = max(ev.x1, r-1)
	}
	ev.text = strings.Join(lines, " ")
	return ev, nil
}
