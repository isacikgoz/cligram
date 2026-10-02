package cligram

import (
	"fmt"
	"strconv"
	"strings"
)

// A placement says one thing about where a node goes, relative to other
// nodes: "right of triage", "below a and b (gap: tight)", "level with x".
// It never names a coordinate, so the picture can reflow to fit the
// terminal and still say what was written.

// Relation is what a placement says about its node and its targets.
type Relation int

const (
	// Directions: the node goes that way from the targets, at least a gap
	// away. On its own, a direction also centers the node on the targets
	// along the other axis.
	Above Relation = iota + 1
	Below
	LeftOf
	RightOf
	AboveLeft
	AboveRight
	BelowLeft
	BelowRight

	// Alignments: the node shares a line with the targets, no gap involved.
	Level       // same horizontal center line
	TopLevel    // same top side
	BottomLevel // same bottom side
	LeftLevel   // same left side
	RightLevel  // same right side
)

// IsAlignment reports whether r shares a line rather than keeping a gap.
func (r Relation) IsAlignment() bool { return r >= Level && r <= RightLevel }

func (r Relation) String() string {
	switch r {
	case Above:
		return "above"
	case Below:
		return "below"
	case LeftOf:
		return "left of"
	case RightOf:
		return "right of"
	case AboveLeft:
		return "above-left of"
	case AboveRight:
		return "above-right of"
	case BelowLeft:
		return "below-left of"
	case BelowRight:
		return "below-right of"
	case Level:
		return "level with"
	case TopLevel:
		return "top level with"
	case BottomLevel:
		return "bottom level with"
	case LeftLevel:
		return "left level with"
	case RightLevel:
		return "right level with"
	}
	return fmt.Sprintf("Relation(%d)", int(r))
}

// directions are the words that start a direction; "of" may follow any.
var directions = map[string]Relation{
	"above":       Above,
	"below":       Below,
	"left":        LeftOf,
	"right":       RightOf,
	"above-left":  AboveLeft,
	"above-right": AboveRight,
	"below-left":  BelowLeft,
	"below-right": BelowRight,
}

// sideLevels are the words that may come before "level with".
var sideLevels = map[string]Relation{
	"top":    TopLevel,
	"bottom": BottomLevel,
	"left":   LeftLevel,
	"right":  RightLevel,
}

// GapSize is how far a direction keeps its node from the targets. The
// names follow the theme, so changing what "tight" means moves every
// tight gap; Cells is for the distance you actually mean.
type GapSize int

const (
	GapDefault GapSize = iota // nothing written: normal, or the layout's choice
	GapNone
	GapTight
	GapNormal
	GapWide
	GapCells
)

// Gap is a placement's minimum distance, in terminal cells when Size is
// GapCells.
type Gap struct {
	Size  GapSize
	Cells int
}

func (g Gap) String() string {
	switch g.Size {
	case GapNone:
		return "none"
	case GapTight:
		return "tight"
	case GapNormal:
		return "normal"
	case GapWide:
		return "wide"
	case GapCells:
		return strconv.Itoa(g.Cells)
	}
	return ""
}

var gapNames = map[string]GapSize{
	"none":   GapNone,
	"tight":  GapTight,
	"normal": GapNormal,
	"wide":   GapWide,
}

// Placement is one relation of a node to one or more targets. With
// several targets the node is placed against the box bounding them all.
type Placement struct {
	Rel     Relation
	Targets []string
	Gap     Gap
}

func (p Placement) String() string {
	var b strings.Builder
	b.WriteString(p.Rel.String())
	b.WriteByte(' ')
	b.WriteString(strings.Join(p.Targets, " and "))
	if p.Gap.Size != GapDefault {
		b.WriteString(" (gap: ")
		b.WriteString(p.Gap.String())
		b.WriteByte(')')
	}
	return b.String()
}

// PlacementError says what is wrong in a placement and where.
type PlacementError struct {
	Input  string
	Column int // 1-based, in runes
	Msg    string
}

func (e *PlacementError) Error() string {
	return fmt.Sprintf("placement %q, column %d: %s", e.Input, e.Column, e.Msg)
}

// ParsePlacement reads one or more placements written one after another,
// as in "right of docker level with docker":
//
//	above X           below X           left of X          right of X
//	above-left of X   above-right of X  below-left of X    below-right of X
//	level with X      top level with X  bottom level with X
//	left level with X right level with X
//
// "of" is optional after a direction. A placement may name several
// targets joined by "and" or commas, and a direction may carry its own
// gap in brackets: "below a and b (gap: tight)", where the gap is one of
// none, tight, normal, wide or a number of cells.
func ParsePlacement(s string) ([]Placement, error) {
	toks, err := lex(s)
	if err != nil {
		return nil, err
	}
	p := &parser{in: s, toks: toks}
	var out []Placement
	for !p.done() {
		pl, err := p.placement()
		if err != nil {
			return nil, err
		}
		out = append(out, pl)
	}
	if len(out) == 0 {
		return nil, &PlacementError{Input: s, Column: 1, Msg: `nothing to place by; write a placement such as "right of x"`}
	}
	return out, nil
}

type tokKind int

const (
	tokWord tokKind = iota
	tokComma
	tokOpen
	tokClose
	tokColon
	tokEnd
)

type token struct {
	kind tokKind
	text string
	col  int // 1-based, in runes
}

func isNameRune(r rune) bool {
	return r == '_' || r == '-' || r == '.' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func lex(s string) ([]token, error) {
	var toks []token
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			i++
		case r == ',':
			toks = append(toks, token{tokComma, ",", i + 1})
			i++
		case r == '(':
			toks = append(toks, token{tokOpen, "(", i + 1})
			i++
		case r == ')':
			toks = append(toks, token{tokClose, ")", i + 1})
			i++
		case r == ':':
			toks = append(toks, token{tokColon, ":", i + 1})
			i++
		case isNameRune(r):
			j := i
			for j < len(rs) && isNameRune(rs[j]) {
				j++
			}
			toks = append(toks, token{tokWord, string(rs[i:j]), i + 1})
			i = j
		default:
			return nil, &PlacementError{Input: s, Column: i + 1, Msg: fmt.Sprintf("%q cannot be in a placement", r)}
		}
	}
	return append(toks, token{kind: tokEnd, col: len(rs) + 1}), nil
}

type parser struct {
	in   string
	toks []token
	pos  int
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) peekAt(n int) token {
	if p.pos+n >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}
	return p.toks[p.pos+n]
}
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }
func (p *parser) done() bool  { return p.peek().kind == tokEnd }

func (p *parser) fail(t token, format string, args ...any) error {
	return &PlacementError{Input: p.in, Column: t.col, Msg: fmt.Sprintf(format, args...)}
}

// isWord reports whether t is the word w.
func isWord(t token, w string) bool { return t.kind == tokWord && t.text == w }

// startsPlacement reports whether the word at t opens a placement, so a
// list of targets ends before it.
func startsPlacement(t token) bool {
	if t.kind != tokWord {
		return false
	}
	_, dir := directions[t.text]
	_, side := sideLevels[t.text]
	return dir || side || t.text == "level"
}

// reserved are the words a target can never be, since they read as the
// start of the next placement or as glue.
func reserved(w string) bool {
	_, dir := directions[w]
	_, side := sideLevels[w]
	return dir || side || w == "level" || w == "with" || w == "of" || w == "and"
}

func (p *parser) placement() (Placement, error) {
	t := p.next()
	if t.kind != tokWord {
		return Placement{}, p.fail(t, "expected a placement such as \"right of x\", found %q", t.text)
	}
	var pl Placement
	switch {
	case t.text == "level":
		pl.Rel = Level
		if err := p.expectWith(t); err != nil {
			return Placement{}, err
		}
	case sideLevels[t.text] != 0 && isWord(p.peek(), "level"):
		pl.Rel = sideLevels[t.text]
		lvl := p.next()
		if err := p.expectWith(lvl); err != nil {
			return Placement{}, err
		}
	case directions[t.text] != 0:
		pl.Rel = directions[t.text]
		if isWord(p.peek(), "of") {
			p.next()
		}
	case t.text == "top" || t.text == "bottom":
		return Placement{}, p.fail(t, "%q aligns a side: write \"%s level with x\"", t.text, t.text)
	default:
		return Placement{}, p.fail(t, "%q does not start a placement; expected one of above, below, left of, right of, level with", t.text)
	}

	targets, err := p.targets(pl.Rel)
	if err != nil {
		return Placement{}, err
	}
	pl.Targets = targets

	if p.peek().kind == tokOpen {
		open := p.peek()
		if pl.Rel.IsAlignment() {
			return Placement{}, p.fail(open, "%q shares a line and keeps no distance, so it takes no gap", pl.Rel)
		}
		gap, err := p.gap()
		if err != nil {
			return Placement{}, err
		}
		pl.Gap = gap
	}
	return pl, nil
}

func (p *parser) expectWith(after token) error {
	if !isWord(p.peek(), "with") {
		return p.fail(p.peek(), "expected \"with\" after %q", after.text)
	}
	p.next()
	return nil
}

func (p *parser) targets(rel Relation) ([]string, error) {
	var out []string
	after := rel.String()
	for {
		t := p.peek()
		switch {
		case t.kind == tokWord && reserved(t.text):
			return nil, p.fail(t, "%q is a placement word, not a node: name a node after %q", t.text, after)
		case t.kind != tokWord:
			return nil, p.fail(t, "name a node after %q", after)
		}
		out = append(out, p.next().text)

		// More targets follow ", x", "and x" or ", and x". A comma before
		// the next placement only separates the two.
		after = ""
		if p.peek().kind == tokComma {
			if startsPlacement(p.peekAt(1)) {
				p.next()
				return out, nil
			}
			p.next()
			after = ","
		}
		if isWord(p.peek(), "and") {
			p.next()
			after = "and"
		}
		if after == "" {
			return out, nil
		}
	}
}

func (p *parser) gap() (Gap, error) {
	p.next() // (
	key := p.next()
	if !isWord(key, "gap") {
		return Gap{}, p.fail(key, "the brackets after a placement take \"gap:\" and nothing else")
	}
	if c := p.next(); c.kind != tokColon {
		return Gap{}, p.fail(c, "expected \":\" after \"gap\"")
	}
	v := p.next()
	if v.kind != tokWord {
		return Gap{}, p.fail(v, "expected a gap: none, tight, normal, wide or a number of cells")
	}
	var g Gap
	if size, ok := gapNames[v.text]; ok {
		g = Gap{Size: size}
	} else if n, err := strconv.Atoi(v.text); err == nil {
		if n < 0 {
			return Gap{}, p.fail(v, "a gap is a distance and cannot be negative")
		}
		g = Gap{Size: GapCells, Cells: n}
	} else if num := strings.TrimRight(v.text, "abcdefghijklmnopqrstuvwxyz"); num != v.text && num != "" {
		if _, err := strconv.Atoi(num); err == nil {
			return Gap{}, p.fail(v, "a gap is a plain number of cells: write %q", num)
		}
		return Gap{}, p.fail(v, "%q is not a gap: use none, tight, normal, wide or a number of cells", v.text)
	} else {
		return Gap{}, p.fail(v, "%q is not a gap: use none, tight, normal, wide or a number of cells", v.text)
	}
	if c := p.next(); c.kind != tokClose {
		return Gap{}, p.fail(c, "expected \")\" to close the gap")
	}
	return g, nil
}
