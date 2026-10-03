// Package yaml reads a cligram diagram from YAML:
//
//	title: Release
//	orientation: across          # or down; across is the default
//	nodes:
//	  triage: Triage             # an id and its text
//	  ready:
//	    text: Ready to ship?
//	    kind: decision           # step (the default), decision or end
//	  ship: { text: Ship it, class: human, at: right of ready, group: release }
//	groups:                      # frames round some nodes
//	  release: Release           # an id and its title
//	  checks: { title: Checks, in: release }   # inside another
//	edges:
//	  - triage -> ready          # an edge
//	  - ready -> ship: yes       # an edge and its label
//	  - ship -.-> triage         # dashed; ==> is thick
//
// Nodes keep the order they are written in, which is the order a flow is
// laid out in. Every mistake is reported with its line.
package yaml

import (
	"errors"
	"fmt"
	"strings"

	goyaml "gopkg.in/yaml.v3"

	"github.com/isacikgoz/cligram"
)

// Doc is a diagram read from YAML, with what it says about laying it out.
type Doc struct {
	Title   string
	Diagram *cligram.Diagram
	// Options lay the diagram out as the document says.
	Options []cligram.LayoutOption
}

// Parse reads a diagram from data.
func Parse(data []byte) (*Doc, error) {
	var root goyaml.Node
	if err := goyaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	doc := &Doc{Diagram: cligram.New()}
	if len(root.Content) == 0 {
		return nil, errors.New("the document is empty: it needs nodes")
	}
	top := root.Content[0]
	if top.Kind != goyaml.MappingNode {
		return nil, at(top, "a diagram is a mapping of title, orientation, nodes and edges")
	}
	var errs []error
	var nodes, edges, groups *goyaml.Node
	for i := 0; i+1 < len(top.Content); i += 2 {
		key, val := top.Content[i], top.Content[i+1]
		switch key.Value {
		case "title":
			doc.Title = val.Value
		case "orientation":
			switch val.Value {
			case "across":
			case "down":
				doc.Options = append(doc.Options, cligram.WithOrientation(cligram.TopToBottom))
			default:
				errs = append(errs, at(val, "orientation is across or down, not %q", val.Value))
			}
		case "nodes":
			nodes = val
		case "edges":
			edges = val
		case "groups":
			groups = val
		default:
			errs = append(errs, at(key, "%q is not a key of a diagram: use title, orientation, nodes, edges or groups", key.Value))
		}
	}
	if nodes == nil {
		return nil, at(top, "a diagram needs nodes")
	}
	ids := map[string]bool{}
	if groups != nil {
		errs = append(errs, readGroups(doc.Diagram, groups)...)
	}
	errs = append(errs, readNodes(doc.Diagram, nodes, ids)...)
	if edges != nil {
		errs = append(errs, readEdges(doc.Diagram, edges, ids)...)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	// What only the whole diagram can tell: a placement against a node
	// that is not there, an edge written twice.
	if err := doc.Diagram.Check(); err != nil {
		return nil, err
	}
	return doc, nil
}

var kinds = map[string]cligram.Kind{"step": cligram.Step, "decision": cligram.Decision, "end": cligram.Terminal}

func readNodes(d *cligram.Diagram, nodes *goyaml.Node, ids map[string]bool) []error {
	if nodes.Kind != goyaml.MappingNode {
		return []error{at(nodes, "nodes are a mapping of ids to texts or to node fields")}
	}
	var errs []error
	for i := 0; i+1 < len(nodes.Content); i += 2 {
		key, val := nodes.Content[i], nodes.Content[i+1]
		id := key.Value
		if ids[id] {
			errs = append(errs, at(key, "node %q is written twice", id))
			continue
		}
		ids[id] = true
		if val.Kind == goyaml.ScalarNode {
			d.Node(id, val.Value)
			continue
		}
		if val.Kind != goyaml.MappingNode {
			errs = append(errs, at(val, "node %q is a text or a mapping of text, kind, class and at", id))
			continue
		}
		var text string
		var opts []cligram.NodeOption
		for j := 0; j+1 < len(val.Content); j += 2 {
			k, v := val.Content[j], val.Content[j+1]
			switch k.Value {
			case "text":
				text = v.Value
			case "kind":
				kind, ok := kinds[v.Value]
				if !ok {
					errs = append(errs, at(v, "kind is step, decision or end, not %q", v.Value))
					continue
				}
				opts = append(opts, cligram.As(kind))
			case "class":
				opts = append(opts, cligram.Class(v.Value))
			case "group":
				opts = append(opts, cligram.In(v.Value))
			case "at":
				if _, err := cligram.ParsePlacement(v.Value); err != nil {
					errs = append(errs, at(v, "%v", err))
					continue
				}
				opts = append(opts, cligram.At(v.Value))
			default:
				errs = append(errs, at(k, "%q is not a key of a node: use text, kind, class, at or group", k.Value))
			}
		}
		d.Node(id, text, opts...)
	}
	return errs
}

func readGroups(d *cligram.Diagram, groups *goyaml.Node) []error {
	if groups.Kind != goyaml.MappingNode {
		return []error{at(groups, "groups are a mapping of ids to titles or to title and in")}
	}
	var errs []error
	for i := 0; i+1 < len(groups.Content); i += 2 {
		key, val := groups.Content[i], groups.Content[i+1]
		switch val.Kind {
		case goyaml.ScalarNode:
			d.Group(key.Value, val.Value)
		case goyaml.MappingNode:
			var title, parent string
			for j := 0; j+1 < len(val.Content); j += 2 {
				k, v := val.Content[j], val.Content[j+1]
				switch k.Value {
				case "title":
					title = v.Value
				case "in":
					parent = v.Value
				default:
					errs = append(errs, at(k, "%q is not a key of a group: use title or in", k.Value))
				}
			}
			d.Group(key.Value, title, cligram.Inside(parent))
		default:
			errs = append(errs, at(val, "group %q is a title or a mapping of title and in", key.Value))
		}
	}
	return errs
}

func readEdges(d *cligram.Diagram, edges *goyaml.Node, ids map[string]bool) []error {
	if edges.Kind != goyaml.SequenceNode {
		return []error{at(edges, `edges are a list of "a -> b", or "a -> b: label"`)}
	}
	var errs []error
	for _, item := range edges.Content {
		spec, label, where := item.Value, "", item
		if item.Kind == goyaml.MappingNode && len(item.Content) == 2 {
			spec, label = item.Content[0].Value, item.Content[1].Value
		} else if item.Kind != goyaml.ScalarNode {
			errs = append(errs, at(item, `an edge is "a -> b", or "a -> b: label"`))
			continue
		}
		// The arrow says how the line is drawn: -.-> dashed, ==> thick.
		from, to, ok, line := "", "", false, cligram.Solid
		for _, a := range []struct {
			arrow string
			line  cligram.LineStyle
		}{{"-.->", cligram.Dashed}, {"==>", cligram.Thick}, {"->", cligram.Solid}} {
			if from, to, ok = strings.Cut(spec, a.arrow); ok {
				line = a.line
				break
			}
		}
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		if !ok || from == "" || to == "" {
			errs = append(errs, at(where, `%q is not an edge: write "a -> b"`, spec))
			continue
		}
		bad := false
		for _, id := range []string{from, to} {
			if !ids[id] {
				errs = append(errs, at(where, "edge %s: %q is not a node", spec, id))
				bad = true
			}
		}
		if !bad {
			d.Edge(from, to, cligram.Label(label), cligram.Line(line))
		}
	}
	return errs
}

// at is an error at n's line.
func at(n *goyaml.Node, format string, args ...any) error {
	return fmt.Errorf("line %d: %s", n.Line, fmt.Sprintf(format, args...))
}
