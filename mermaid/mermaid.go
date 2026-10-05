// Package mermaid reads a cligram diagram from a Mermaid flowchart, the
// diagram language most people, and every coding agent, already write:
//
//	---
//	title: Release
//	---
//	flowchart LR
//	  triage[Triage] --> ready{Ready to ship?}
//	  ready -->|yes| ship([Ship it])
//	  ready -->|no| fix[Fix it]:::agent
//	  fix --> ready
//
// It also reads state diagrams (stateDiagram-v2), see state.go, and
// sequence diagrams (sequenceDiagram) onto package sequence, see
// sequence.go. Of
// flowcharts it reads the subset that maps onto cligram:
//
//   - shapes: [text], (text), [[text]], [(text)], >text], [/text/] and
//     [\text\] are steps; {text} and {{text}} decisions; ([text]) and
//     ((text)) ends. Text may be quoted, and <br> breaks a line.
//   - links: -->, ---, -.->, -.-, ==>, ===, --x, --o and <-->, with a
//     label as -->|label| or -- label -->; chains (a --> b --> c) and
//     groups (a & b --> c). A link with no arrowhead (---, -.-, ===) is
//     undirected; a flowchart of only those is a network, laid out by its
//     links rather than as a flow.
//   - flowchart (or graph) TD, TB, LR, RL or BT; a front matter title;
//     node:::class and "class a,b name" for classes; subgraphs, drawn
//     as titled frames, nested too; %% comments.
//
// Styling (style, classDef, linkStyle) and interaction (click) are
// ignored. Anything else is an error that gives its line.
package mermaid

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/isacikgoz/cligram"
)

// Doc is a diagram read from a flowchart, with what it says about laying
// it out.
type Doc struct {
	Title   string
	Diagram *cligram.Diagram
	// Options lay the diagram out as the flowchart says.
	Options []cligram.LayoutOption
}

// Is reports whether text looks like a Mermaid flowchart or state
// diagram: its first line that is not front matter or a comment starts
// one.
func Is(text string) bool {
	_, rest := frontMatter(text)
	for _, line := range strings.Split(rest, "\n") {
		for _, stmt := range splitStatements(line) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" || strings.HasPrefix(stmt, "%%") {
				continue
			}
			return header.MatchString(stmt) || isState(stmt)
		}
	}
	return false
}

var header = regexp.MustCompile(`^(flowchart|graph)(\s+(TD|TB|LR|RL|BT))?\s*;?\s*$`)

// frontMatter splits off a --- block at the top, giving its title.
func frontMatter(text string) (title, rest string) {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	if !strings.HasPrefix(trimmed, "---") {
		return "", text
	}
	lines := strings.Split(trimmed, "\n")
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			// Keep the lines, blank, so errors still give the right line.
			blank := strings.Repeat("\n", i+1)
			lead := len(text) - len(trimmed)
			return title, strings.Repeat("\n", strings.Count(text[:lead], "\n")) + blank + strings.Join(lines[i+1:], "\n")
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "title" {
			title = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return "", text
}

type node struct {
	id, text string
	kind     cligram.Kind
	class    string
	defined  bool   // given a shape and text, not only named
	group    string // the subgraph it is last named inside
}

type parser struct {
	doc     *Doc
	nodes   map[string]*node
	order   []string
	edges   []edge
	classes map[string]string // node id to class, from class lines
	errs    []error
	line    int
	// groups are the subgraphs, in order; open those not yet ended,
	// innermost last.
	groups []cligram.Group
	open   []string
}

type edge struct {
	from, to, label string
	line            int
	style           cligram.LineStyle
	undirected      bool
}

// Parse reads a diagram from a flowchart.
func Parse(data []byte) (*Doc, error) {
	title, text := frontMatter(string(data))
	p := &parser{doc: &Doc{Title: title, Diagram: cligram.New()}, nodes: map[string]*node{}, classes: map[string]string{}}
	seenHeader := false
	for i, raw := range strings.Split(text, "\n") {
		p.line = i + 1
		for _, stmt := range splitStatements(raw) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" || strings.HasPrefix(stmt, "%%") {
				continue
			}
			if !seenHeader {
				if isState(stmt) {
					return parseState(title, text, i+1)
				}
				m := header.FindStringSubmatch(stmt)
				if m == nil && seqHeader.MatchString(stmt) {
					p.fail("a flowchart starts with \"flowchart LR\" or \"flowchart TD\", not %q: a sequence diagram is read with ParseSequence", stmt)
					return nil, errors.Join(p.errs...)
				}
				if m == nil {
					p.fail("a flowchart starts with \"flowchart LR\" or \"flowchart TD\", not %q", stmt)
					return nil, errors.Join(p.errs...)
				}
				if m[3] == "TD" || m[3] == "TB" || m[3] == "BT" {
					p.doc.Options = append(p.doc.Options, cligram.WithOrientation(cligram.TopToBottom))
				}
				seenHeader = true
				continue
			}
			p.statement(stmt)
		}
	}
	if !seenHeader {
		return nil, errors.New("not a flowchart: it needs a \"flowchart LR\" or \"flowchart TD\" line")
	}
	if len(p.order) == 0 {
		p.errs = append(p.errs, errors.New("the flowchart has no nodes"))
	}
	return p.finish()
}

// finish builds the diagram from what was read, or says what is wrong.
func (p *parser) finish() (*Doc, error) {
	if len(p.open) > 0 {
		p.errs = append(p.errs, fmt.Errorf("subgraph %q is not closed with end", p.open[len(p.open)-1]))
	}
	for _, g := range p.groups {
		p.doc.Diagram.Group(g.ID, g.Title, cligram.Inside(g.Parent))
	}
	if err := errors.Join(p.errs...); err != nil {
		return nil, err
	}
	d := p.doc.Diagram
	for _, id := range p.order {
		n := p.nodes[id]
		text := n.text
		if !n.defined {
			text = id
		}
		opts := []cligram.NodeOption{cligram.As(n.kind)}
		class := n.class
		if c, ok := p.classes[id]; ok {
			class = c
		}
		if class != "" {
			opts = append(opts, cligram.Class(class))
		}
		if n.group != "" {
			opts = append(opts, cligram.In(n.group))
		}
		d.Node(id, text, opts...)
	}
	seen := map[cligram.EdgeRef]bool{}
	for _, e := range p.edges {
		ref := cligram.EdgeRef{From: e.from, To: e.to, Label: e.label}
		if seen[ref] {
			continue // Mermaid draws a link written twice once
		}
		seen[ref] = true
		opts := []cligram.EdgeOption{cligram.Label(e.label), cligram.Line(e.style)}
		if e.undirected {
			opts = append(opts, cligram.Undirected())
		}
		d.Edge(e.from, e.to, opts...)
	}
	if err := d.Check(); err != nil {
		return nil, err
	}
	return p.doc, nil
}

func (p *parser) fail(format string, args ...any) {
	p.errs = append(p.errs, fmt.Errorf("line %d: %s", p.line, fmt.Sprintf(format, args...)))
}

// splitStatements splits a line at semicolons outside quotes and brackets.
func splitStatements(line string) []string {
	var out []string
	depth, quoted, start := 0, false, 0
	for i, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
		case quoted:
		case strings.ContainsRune("[({", r):
			depth++
		case strings.ContainsRune("])}", r):
			depth--
		case r == ';' && depth <= 0:
			out = append(out, line[start:i])
			start = i + 1
		}
	}
	return append(out, line[start:])
}

var (
	ignored    = regexp.MustCompile(`^(style|classDef|linkStyle|click|direction)\b`)
	classLine  = regexp.MustCompile(`^class\s+([\w,\s-]+?)\s+([\w-]+)$`)
	subgraphRe = regexp.MustCompile(`^subgraph\b`)
	// Links with their label inside, as in "-- yes -->", "== yes ==>" or
	// "-. yes .->"; and links alone, which may take a label after them
	// between bars, as in "-->|yes|".
	labelled = []*regexp.Regexp{
		regexp.MustCompile(`^<?--\s+(.+?)\s+-{2,}[>xo]?`),
		regexp.MustCompile(`^<?==\s+(.+?)\s+={2,}>?`),
		regexp.MustCompile(`^<?-\.\s+(.+?)\s+\.-+>?`),
	}
	plain = regexp.MustCompile(`^<?(?:-\.+->|-\.+-|-{2,}[>xo]|-{3,}|={2,}>|={3,})(?:\s*\|([^|]*)\|)?`)
)

// link reads a link from the front of *s, giving its label, how its line
// is drawn (-.-> dashed, ==> thick) and whether it has no arrowhead (---,
// -.-, ===), a link with no way.
func link(s *string) (string, cligram.LineStyle, bool, bool) {
	style := func(m string) cligram.LineStyle {
		switch {
		case strings.HasPrefix(strings.TrimPrefix(m, "<"), "-."):
			return cligram.Dashed
		case strings.HasPrefix(strings.TrimPrefix(m, "<"), "=="):
			return cligram.Thick
		}
		return cligram.Solid
	}
	headless := func(m string) bool {
		if i := strings.Index(m, "|"); i >= 0 {
			m = strings.TrimSpace(m[:i])
		}
		return !strings.HasPrefix(m, "<") && (strings.HasSuffix(m, "-") || strings.HasSuffix(m, "="))
	}
	for _, re := range labelled {
		if m := re.FindStringSubmatch(*s); m != nil {
			*s = (*s)[len(m[0]):]
			return strings.TrimSpace(m[1]), style(m[0]), headless(m[0]), true
		}
	}
	if m := plain.FindStringSubmatch(*s); m != nil {
		*s = (*s)[len(m[0]):]
		return strings.TrimSpace(m[1]), style(m[0]), headless(m[0]), true
	}
	return "", cligram.Solid, false, false
}

// readID reads a node id from the front of s: letters, digits, _ and .,
// and a dash only between them, so that "a-->b" is a link from a.
func readID(s string) string {
	isWord := func(c byte) bool {
		return c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	}
	i := 0
	for i < len(s) {
		switch {
		case isWord(s[i]):
			i++
		case s[i] == '-' && i > 0 && i+1 < len(s) && isWord(s[i+1]) && s[i+1] != '.':
			i++
		default:
			return s[:i]
		}
	}
	return s
}

func (p *parser) statement(s string) {
	switch {
	case subgraphRe.MatchString(s):
		p.subgraph(strings.TrimSpace(strings.TrimPrefix(s, "subgraph")))
		return
	case s == "end":
		if len(p.open) == 0 {
			p.fail("an end closes no subgraph")
			return
		}
		p.open = p.open[:len(p.open)-1]
		return
	case ignored.MatchString(s):
		return
	}
	if m := classLine.FindStringSubmatch(s); m != nil {
		for _, id := range strings.Split(m[1], ",") {
			p.classes[strings.TrimSpace(id)] = m[2]
		}
		return
	}
	rest := s
	prev, ok := p.group(&rest)
	if !ok {
		return
	}
	for {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return
		}
		label, style, undirected, ok := link(&rest)
		if !ok {
			p.fail("expected a link such as --> after the node, found %q", rest)
			return
		}
		label = unquote(label)
		next, ok := p.group(&rest)
		if !ok {
			return
		}
		for _, from := range prev {
			for _, to := range next {
				p.edges = append(p.edges, edge{from, to, label, p.line, style, undirected})
			}
		}
		prev = next
	}
}

// subgraph opens a subgraph: "id [Title]", "id", "Title words" or
// "\"Title\"", inside the one open, if any.
func (p *parser) subgraph(rest string) {
	id, title := rest, rest
	if m := subgraphTitled.FindStringSubmatch(rest); m != nil {
		id, title = m[1], unquote(strings.TrimSpace(m[2]))
	} else if q := unquote(rest); q != rest {
		id, title = q, q
	}
	if id == "" {
		p.fail("a subgraph needs an id or a title")
		return
	}
	g := cligram.Group{ID: id, Title: title}
	if len(p.open) > 0 {
		g.Parent = p.open[len(p.open)-1]
	}
	for _, have := range p.groups {
		if have.ID == id {
			p.fail("subgraph %q is opened twice", id)
			return
		}
	}
	p.groups = append(p.groups, g)
	p.open = append(p.open, id)
}

var subgraphTitled = regexp.MustCompile(`^([\w.-]+)\s*\[(.*)\]$`)

// group reads one node, or several joined by &, from the front of *s.
func (p *parser) group(s *string) ([]string, bool) {
	var ids []string
	for {
		*s = strings.TrimSpace(*s)
		id, ok := p.node(s)
		if !ok {
			return nil, false
		}
		ids = append(ids, id)
		t := strings.TrimSpace(*s)
		if !strings.HasPrefix(t, "&") {
			return ids, true
		}
		*s = t[1:]
	}
}

// Shapes, longest opener first: what each opens, closes and draws as.
var shapes = []struct {
	open, close string
	kind        cligram.Kind
}{
	{"([", "])", cligram.Terminal},
	{"((", "))", cligram.Terminal},
	{"[[", "]]", cligram.Step},
	{"[(", ")]", cligram.Step},
	{"{{", "}}", cligram.Decision},
	{"[/", "/]", cligram.Step},
	{"[\\", "\\]", cligram.Step},
	{"[/", "\\]", cligram.Step},
	{"[\\", "/]", cligram.Step},
	{"[", "]", cligram.Step},
	{"(", ")", cligram.Step},
	{"{", "}", cligram.Decision},
	{">", "]", cligram.Step},
}

// node reads a node from the front of *s: an id, a shape with its text,
// a class.
func (p *parser) node(s *string) (string, bool) {
	id := readID(*s)
	if id == "" {
		p.fail("expected a node id, found %q", *s)
		return "", false
	}
	*s = (*s)[len(id):]
	n, ok := p.nodes[id]
	if !ok {
		n = &node{id: id}
		p.nodes[id] = n
		p.order = append(p.order, id)
	}
	// As in Mermaid, a node is in the subgraph it is last named inside,
	// wherever it was named first.
	if len(p.open) > 0 {
		n.group = p.open[len(p.open)-1]
	}
	for _, sh := range shapes {
		if !strings.HasPrefix(*s, sh.open) {
			continue
		}
		body := (*s)[len(sh.open):]
		end := closing(body, sh.close)
		if end < 0 {
			continue // try a shorter opener, or fail below
		}
		text := unquote(strings.TrimSpace(body[:end]))
		text = brRe.ReplaceAllString(text, "\n")
		if n.defined && (n.text != text || n.kind != sh.kind) {
			p.fail("node %q is given a second shape or text", id)
		}
		n.text, n.kind, n.defined = text, sh.kind, true
		*s = body[end+len(sh.close):]
		break
	}
	if strings.HasPrefix(*s, ":::") {
		class := classRe.FindString((*s)[3:])
		n.class = class
		*s = (*s)[3+len(class):]
	}
	if t := strings.TrimLeft(*s, " \t"); strings.ContainsAny(t[:min(1, len(t))], "[({>") {
		p.fail("node %q has a shape that does not close", id)
		return "", false
	}
	return id, true
}

var (
	brRe    = regexp.MustCompile(`(?i)<br\s*/?>`)
	classRe = regexp.MustCompile(`^[\w-]+`)
)

// closing is where close first appears in body outside quotes, or -1.
func closing(body, close string) int {
	quoted := false
	for i := 0; i < len(body); i++ {
		if body[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted && strings.HasPrefix(body[i:], close) {
			return i
		}
	}
	return -1
}

// unquote takes the quotes off a quoted text.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
