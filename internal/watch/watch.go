// Package watch reads where a run is, one event a line, and keeps the
// state it adds up to: what cligram watch and the MCP server show a
// diagram in.
//
// An event is JSON, or the same in words, which shell scripts write most
// easily:
//
//	{"node": "build", "status": "active"}   build active
//	{"edge": ["build", "test"]}              build -> test
//	{"edge": ["check", "fix", "no"]}         check -> fix: no
//	{"reset": true}                          reset
//
// A status is idle, active, done, failed or waiting. An edge is taken: it
// is drawn as the way the run went. Blank lines and # comments are left
// out.
package watch

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/isacikgoz/cligram"
)

// Event is one change to where a run is.
type Event struct {
	Node   string   `json:"node,omitempty"`
	Status string   `json:"status,omitempty"`
	Edge   []string `json:"edge,omitempty"` // from, to, and its label if it has one
	Reset  bool     `json:"reset,omitempty"`
}

var (
	words    = regexp.MustCompile(`^(\S+)\s+(idle|active|done|failed|waiting)$`)
	edgeWord = regexp.MustCompile(`^(\S+)\s*->\s*(\S+?)(?:\s*:\s*(.+))?$`)
)

// Parse reads an event from a line; ok is false for a blank line or a
// comment.
func Parse(line string) (ev Event, ok bool, err error) {
	line = strings.TrimSpace(line)
	switch {
	case line == "" || strings.HasPrefix(line, "#"):
		return Event{}, false, nil
	case strings.HasPrefix(line, "{"):
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ev); err != nil {
			return Event{}, false, fmt.Errorf("%q is not an event: %w", line, err)
		}
	case line == "reset":
		ev.Reset = true
	default:
		if m := words.FindStringSubmatch(line); m != nil {
			ev.Node, ev.Status = m[1], m[2]
		} else if m := edgeWord.FindStringSubmatch(line); m != nil {
			ev.Edge = []string{m[1], m[2]}
			if m[3] != "" {
				ev.Edge = append(ev.Edge, strings.TrimSpace(m[3]))
			}
		} else {
			return Event{}, false, fmt.Errorf(`%q is not an event: write "node status", "a -> b" or JSON`, line)
		}
	}
	return ev, true, nil
}

var statuses = map[string]cligram.Status{
	"idle": cligram.Idle, "active": cligram.Active, "done": cligram.Done,
	"failed": cligram.Failed, "waiting": cligram.Waiting,
}

// Apply changes st, for diagram d, by ev. It says what is wrong with an
// event that names no node or edge of d, and leaves st as it was.
func Apply(d *cligram.Diagram, st *cligram.State, ev Event) error {
	if st.Status == nil {
		st.Status = map[string]cligram.Status{}
	}
	switch {
	case ev.Reset:
		*st = cligram.State{Status: map[string]cligram.Status{}}
	case ev.Node != "":
		s, ok := statuses[ev.Status]
		if !ok {
			return fmt.Errorf("status %q of %s is not idle, active, done, failed or waiting", ev.Status, ev.Node)
		}
		if !slices.ContainsFunc(d.Nodes(), func(n cligram.Node) bool { return n.ID == ev.Node }) {
			return fmt.Errorf("%q is not a node of the diagram", ev.Node)
		}
		st.Status[ev.Node] = s
	case len(ev.Edge) == 2 || len(ev.Edge) == 3:
		ref := cligram.EdgeRef{From: ev.Edge[0], To: ev.Edge[1]}
		if len(ev.Edge) == 3 {
			ref.Label = ev.Edge[2]
		}
		// Without a label, the one edge between the two, whatever its label.
		var found []cligram.EdgeRef
		for _, e := range d.Edges() {
			if e.From == ref.From && e.To == ref.To && (len(ev.Edge) == 2 || e.Label == ref.Label) {
				found = append(found, e.Ref())
			}
		}
		switch len(found) {
		case 0:
			return fmt.Errorf("%s -> %s is not an edge of the diagram", ref.From, ref.To)
		case 1:
		default:
			return fmt.Errorf("%s -> %s is %d edges: say which by its label", ref.From, ref.To, len(found))
		}
		if !slices.Contains(st.Taken, found[0]) {
			st.Taken = append(st.Taken, found[0])
		}
	default:
		return errors.New("an event sets a node's status, takes an edge, or resets")
	}
	return nil
}
