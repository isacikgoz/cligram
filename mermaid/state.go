package mermaid

import (
	"errors"
	"regexp"
	"strings"

	"github.com/isacikgoz/cligram"
)

// A state diagram is read onto the same kinds a flowchart is:
//
//	stateDiagram-v2
//	  [*] --> Draft
//	  Draft --> Review : submit
//	  state Approved <<choice>>
//	  Review --> Approved
//	  Approved --> Done : yes
//	  Approved --> Draft : no
//	  Done --> [*]
//
// A state is a step, its text its name or what "state "text" as id" or
// "id : text" says; a <<choice>> is a decision; [*] is a Start end box as
// the source of a transition and an End one as its target. A composite
// state's states are drawn in a frame titled as it is, the composite's own
// box before it: its inner [*] start is that box, its inner end an End of
// its own. Notes, concurrency (--) and
// styling are left out.

var (
	stateHeader     = regexp.MustCompile(`^stateDiagram(-v2)?\s*$`)
	stateDirection  = regexp.MustCompile(`^direction\s+(TB|TD|BT|LR|RL)$`)
	stateAs         = regexp.MustCompile(`^state\s+"([^"]*)"\s+as\s+([\w.-]+)\s*(\{)?$`)
	stateDecl       = regexp.MustCompile(`^state\s+([\w.-]+)\s*(<<(\w+)>>)?\s*(\{)?$`)
	stateTransition = regexp.MustCompile(`^(\[\*\]|[\w.-]+)(:::[\w-]+)?\s*-->\s*(\[\*\]|[\w.-]+)(:::[\w-]+)?\s*(?::\s*(.*))?$`)
	stateText       = regexp.MustCompile(`^([\w.-]+)\s*:\s*(.+)$`)
	stateNote       = regexp.MustCompile(`^note\s+(left|right)\s+of\s+[\w.-]+\s*(:.*)?$`)
)

// isState reports whether stmt opens a state diagram.
func isState(stmt string) bool { return stateHeader.MatchString(stmt) }

// parseState reads a state diagram, its header already seen on line
// first; text is the whole document, its front matter blanked.
func parseState(title, text string, first int) (*Doc, error) {
	p := &parser{doc: &Doc{Title: title, Diagram: cligram.New()}, nodes: map[string]*node{}, classes: map[string]string{}}
	inNote := false
	lines := strings.Split(text, "\n")
	for i := first; i < len(lines); i++ {
		p.line = i + 1
		for _, stmt := range splitStatements(lines[i]) {
			stmt = strings.TrimSpace(stmt)
			switch {
			case stmt == "" || strings.HasPrefix(stmt, "%%"):
				continue
			case inNote:
				inNote = stmt != "end note"
				continue
			case strings.HasPrefix(stmt, "note "):
				// "note right of a : text" is one line; without the text
				// it runs to "end note".
				inNote = stateNote.MatchString(stmt) && !strings.Contains(stmt, ":")
				continue
			case stmt == "--" || ignored.MatchString(stmt) && !strings.HasPrefix(stmt, "direction"):
				continue
			case stmt == "}":
				if len(p.open) == 0 {
					p.fail("a } closes no composite state")
					continue
				}
				p.open = p.open[:len(p.open)-1]
				continue
			}
			if m := stateDirection.FindStringSubmatch(stmt); m != nil {
				if len(p.open) == 0 && m[1] != "LR" && m[1] != "RL" {
					p.doc.Options = append(p.doc.Options, cligram.WithOrientation(cligram.TopToBottom))
				}
				continue
			}
			if m := stateAs.FindStringSubmatch(stmt); m != nil {
				p.state(m[2], cligram.Step).text, p.nodes[m[2]].defined = unquote(m[1]), true
				if m[3] != "" {
					p.composite(m[2], unquote(m[1]))
				}
				continue
			}
			if m := stateDecl.FindStringSubmatch(stmt); m != nil {
				kind := cligram.Step
				if m[3] == "choice" {
					kind = cligram.Decision
				}
				n := p.state(m[1], kind)
				n.kind = kind
				if m[4] != "" {
					p.composite(m[1], m[1])
				}
				continue
			}
			if m := classLine.FindStringSubmatch(stmt); m != nil {
				for _, id := range strings.Split(m[1], ",") {
					p.classes[strings.TrimSpace(id)] = m[2]
				}
				continue
			}
			if m := stateTransition.FindStringSubmatch(stmt); m != nil {
				scope := ""
				if len(p.open) > 0 {
					scope = p.open[len(p.open)-1]
				}
				from := p.endpoint(m[1], scope, true)
				to := p.endpoint(m[3], scope, false)
				for _, c := range []struct{ id, class string }{{from, m[2]}, {to, m[4]}} {
					if c.class != "" {
						p.nodes[c.id].class = strings.TrimPrefix(c.class, ":::")
					}
				}
				p.edges = append(p.edges, edge{from: from, to: to, label: strings.TrimSpace(m[5]), line: p.line})
				continue
			}
			if m := stateText.FindStringSubmatch(stmt); m != nil {
				// A description goes under the state's name, or its text.
				n := p.state(m[1], cligram.Step)
				text := unquote(strings.TrimSpace(m[2]))
				if n.defined && n.text != "" {
					text = n.text + "\n" + text
				} else {
					text = n.id + "\n" + text
				}
				n.text, n.defined = text, true
				continue
			}
			if id := readID(stmt); id == stmt {
				p.state(id, cligram.Step)
				continue
			}
			p.fail("expected a state, a transition such as a --> b, or a note; found %q", stmt)
		}
	}
	if len(p.open) > 0 {
		p.errs = append(p.errs, errors.New("the composite state "+p.open[len(p.open)-1]+" is not closed with }"))
		p.open = nil
	}
	if len(p.order) == 0 {
		p.errs = append(p.errs, errors.New("the state diagram has no states"))
	}
	return p.finish()
}

// composite opens a composite state: its states are drawn in a frame,
// titled as it is, the composite's own box before it.
func (p *parser) composite(id, title string) {
	g := cligram.Group{ID: id, Title: title}
	if len(p.open) > 0 {
		g.Parent = p.open[len(p.open)-1]
	}
	p.groups = append(p.groups, g)
	p.open = append(p.open, id)
}

// state is the state with id, made a kind of node the first time, in the
// composite state open then.
func (p *parser) state(id string, kind cligram.Kind) *node {
	if n, ok := p.nodes[id]; ok {
		return n
	}
	n := &node{id: id, kind: kind}
	if len(p.open) > 0 {
		n.group = p.open[len(p.open)-1]
	}
	p.nodes[id] = n
	p.order = append(p.order, id)
	return n
}

// endpoint is the node a transition's end names: a state, or for [*]
// the start or end of the scope it is in.
func (p *parser) endpoint(name, scope string, source bool) string {
	if name != "[*]" {
		return p.state(name, cligram.Step).id
	}
	switch {
	case source && scope != "":
		return p.state(scope, cligram.Step).id // entering a composite starts it
	case source:
		n := p.state("[*]start", cligram.Terminal)
		n.text, n.defined = "Start", true
		return n.id
	}
	id := "[*]end"
	if scope != "" {
		id = scope + ".[*]end"
	}
	n := p.state(id, cligram.Terminal)
	n.text, n.defined = "End", true
	return n.id
}
