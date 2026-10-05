package mermaid

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/sequence"
)

// A sequence diagram is read onto package sequence:
//
//	sequenceDiagram
//	  actor Alice
//	  participant API as Orders API
//	  Alice->>+API: GET /orders
//	  alt found
//	    API-->>Alice: 200
//	  else missing
//	    API--xAlice: 404
//	  end
//	  deactivate API
//
// It reads participants and actors (with "as" text), every message arrow
// (->>, -->>, ->, -->, -x, --x, -), --), <<->> and <<-->>) with + and -
// to activate and deactivate, activate and deactivate lines, notes left
// of, right of and over one or two participants, the loop, alt/else,
// opt, par/and, critical/option and break blocks, autonumber, and a
// title. rect and box are read and left out, as styling; create and
// destroy declare and leave the participant as it is. Text may use <br>
// and entities such as #59; for a semicolon. Anything else is an error
// that gives its line.

// SequenceDoc is a sequence diagram read from Mermaid.
type SequenceDoc struct {
	Title   string
	Diagram *sequence.Diagram
}

var (
	seqHeader      = regexp.MustCompile(`^sequenceDiagram\s*$`)
	seqTitle       = regexp.MustCompile(`^title\s*:?\s*(.*)$`)
	seqParticipant = regexp.MustCompile(`^(?:create\s+)?(participant|actor)\s+(.+?)(?:\s+as\s+(.+))?$`)
	seqMessage     = regexp.MustCompile(`^([^-<>:;,+]+?)\s*(<<-->>|<<->>|-->>|->>|--x|-x|--\)|-\)|-->|->)\s*([+-]?)\s*([^-<>:;,+]+?)\s*(?::(.*))?$`)
	seqNote        = regexp.MustCompile(`(?i)^note\s+(right\s+of|left\s+of|over)\s+([^:]+?)\s*:(.*)$`)
	seqActivate    = regexp.MustCompile(`^(activate|deactivate)\s+(.+)$`)
	seqAutonumber  = regexp.MustCompile(`^autonumber(?:\s+(off|\d+)(?:\s+(\d+))?)?$`)
	seqBlock       = regexp.MustCompile(`^(loop|alt|opt|par|critical|break)\b\s*(.*)$`)
	seqSection     = regexp.MustCompile(`^(else|and|option)\b\s*(.*)$`)
	seqStyling     = regexp.MustCompile(`^(rect|box)\b`)
	seqIgnored     = regexp.MustCompile(`^(destroy|links?|properties|details)\s`)
	seqProperties  = regexp.MustCompile(`@\{.*\}$`)
	seqEntity      = regexp.MustCompile(`#(\d+|[a-zA-Z]+);`)
	seqEntityEnd   = regexp.MustCompile(`#(\d+|[a-zA-Z]+);$`)
)

// IsSequence reports whether text is a Mermaid sequence diagram: its
// first line that is not front matter or a comment is sequenceDiagram.
func IsSequence(text string) bool {
	_, rest := frontMatter(text)
	for _, line := range strings.Split(rest, "\n") {
		stmt := strings.TrimSpace(line)
		if stmt == "" || strings.HasPrefix(stmt, "%%") {
			continue
		}
		return seqHeader.MatchString(stmt)
	}
	return false
}

// sections are the words a block's further parts start with.
var sections = map[string]string{"alt": "else", "par": "and", "critical": "option"}

// ParseSequence reads a sequence diagram.
func ParseSequence(data []byte) (*SequenceDoc, error) {
	title, text := frontMatter(string(data))
	doc := &SequenceDoc{Title: title, Diagram: sequence.New()}
	d := doc.Diagram
	var errs []error
	line := 0
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("line %d: %s", line, fmt.Sprintf(format, args...)))
	}
	seenHeader := false
	// The blocks open, innermost last: a block word, or "" for rect and
	// box, which are styling and only need their end.
	var open []string
	// Mistakes the diagram would find, found here to give their line.
	declared, depth := map[string]bool{}, map[string]int{}
	for i, raw := range strings.Split(text, "\n") {
		line = i + 1
		// A semicolon ends a statement; in text it is written #59;.
		for _, stmt := range seqStatements(raw) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" || strings.HasPrefix(stmt, "%%") {
				continue
			}
			if !seenHeader {
				if !seqHeader.MatchString(stmt) {
					fail("a sequence diagram starts with \"sequenceDiagram\", not %q", stmt)
					return nil, errors.Join(errs...)
				}
				seenHeader = true
				continue
			}
			switch m := match(stmt); {
			case m.is(seqParticipant):
				id := strings.TrimSpace(seqProperties.ReplaceAllString(m.m[2], ""))
				if declared[id] {
					fail("participant %s is declared twice", id)
					continue
				}
				declared[id] = true
				var opts []sequence.ParticipantOption
				if m.m[1] == "actor" {
					opts = append(opts, sequence.Actor())
				}
				d.Participant(id, seqText(m.m[3]), opts...)
			case m.is(seqMessage):
				from, to := strings.TrimSpace(m.m[1]), strings.TrimSpace(m.m[4])
				opts := arrowOptions(m.m[2])
				switch m.m[3] {
				case "+":
					opts = append(opts, sequence.Activate())
					depth[to]++
				case "-":
					if depth[from] == 0 {
						fail("%s%s-%s ends %s's activation, but %s is not active", from, m.m[2], to, from, from)
						continue
					}
					opts = append(opts, sequence.Deactivate())
					depth[from]--
				}
				d.Message(from, to, seqText(m.m[5]), opts...)
			case m.is(seqNote):
				ids := strings.Split(m.m[2], ",")
				for k := range ids {
					ids[k] = strings.TrimSpace(ids[k])
				}
				where := strings.Join(strings.Fields(strings.ToLower(m.m[1])), " ")
				switch {
				case len(ids) > 2 || (len(ids) == 2 && where != "over"):
					fail("a note is over one or two participants, or beside one: %q", stmt)
				case where == "right of":
					d.Note(seqText(m.m[3]), sequence.RightOf(ids[0]))
				case where == "left of":
					d.Note(seqText(m.m[3]), sequence.LeftOf(ids[0]))
				case len(ids) == 2:
					d.Note(seqText(m.m[3]), sequence.Over(ids[0], ids[1]))
				default:
					d.Note(seqText(m.m[3]), sequence.Over(ids[0]))
				}
			case m.is(seqActivate):
				id := strings.TrimSpace(m.m[2])
				switch {
				case m.m[1] == "activate":
					depth[id]++
					d.Activate(id)
				case depth[id] == 0:
					fail("deactivate: %s is not active", id)
				default:
					depth[id]--
					d.Deactivate(id)
				}
			case m.is(seqAutonumber):
				switch m.m[1] {
				case "off":
					d.Autonumber(0, 0)
				case "":
					d.Autonumber(1, 1)
				default:
					start, _ := strconv.Atoi(m.m[1])
					by := 1
					if m.m[2] != "" {
						by, _ = strconv.Atoi(m.m[2])
					}
					d.Autonumber(start, by)
				}
			case m.is(seqBlock):
				open = append(open, m.m[1])
				d.Block(m.m[1], seqText(m.m[2]))
			case m.is(seqSection):
				innermost := ""
				if len(open) > 0 {
					innermost = open[len(open)-1]
				}
				if want, ok := sections[innermost]; !ok || want != m.m[1] {
					fail("%s belongs in %s, not here", m.m[1], blockOf(m.m[1]))
					continue
				}
				d.Section(m.m[1], seqText(m.m[2]))
			case stmt == "end":
				if len(open) == 0 {
					fail("end closes no block")
					continue
				}
				if open[len(open)-1] != "" {
					d.End()
				}
				open = open[:len(open)-1]
			case seqStyling.MatchString(stmt):
				open = append(open, "")
			case seqTitle.MatchString(stmt):
				doc.Title = strings.TrimSpace(seqTitle.FindStringSubmatch(stmt)[1])
			case seqIgnored.MatchString(stmt):
			default:
				fail("expected a participant, a message such as A->>B: text, a note, or a block; found %q", stmt)
			}
		}
	}
	if !seenHeader {
		return nil, errors.New("not a sequence diagram: it needs a \"sequenceDiagram\" line")
	}
	if len(open) > 0 {
		word := open[len(open)-1]
		if word == "" {
			word = "rect or box"
		}
		errs = append(errs, fmt.Errorf("%s is not closed with end", word))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	if err := d.Check(); err != nil {
		return nil, err
	}
	return doc, nil
}

// match matches a statement against one pattern after another.
func match(stmt string) *matcher { return &matcher{stmt: stmt} }

// matcher is a statement, and its submatches against the pattern last
// tried.
type matcher struct {
	stmt string
	m    []string
}

func (m *matcher) is(re *regexp.Regexp) bool {
	m.m = re.FindStringSubmatch(m.stmt)
	return m.m != nil
}

func blockOf(section string) string {
	for block, s := range sections {
		if s == section {
			if strings.ContainsAny(block[:1], "aeiou") {
				return "an " + block + " block"
			}
			return "a " + block + " block"
		}
	}
	return "a block"
}

// seqStatements splits a line at the semicolons that end statements, not
// those ending an entity such as #59;.
func seqStatements(line string) []string {
	var out []string
	start := 0
	for i := 0; i < len(line); i++ {
		if line[i] != ';' {
			continue
		}
		if seqEntityEnd.MatchString(line[start : i+1]) {
			continue
		}
		out = append(out, line[start:i])
		start = i + 1
	}
	return append(out, line[start:])
}

// arrowOptions are what a message's arrow says about it.
func arrowOptions(arrow string) []sequence.MessageOption {
	var opts []sequence.MessageOption
	if strings.Contains(arrow, "--") {
		opts = append(opts, sequence.Line(cligram.Dashed))
	}
	switch {
	case strings.HasPrefix(arrow, "<<"):
		opts = append(opts, sequence.BothWays())
	case strings.HasSuffix(arrow, "x"):
		opts = append(opts, sequence.WithHead(sequence.Cross))
	case strings.HasSuffix(arrow, ">>"), strings.HasSuffix(arrow, ")"):
	default: // -> and -->: a line with no head
		opts = append(opts, sequence.WithHead(sequence.Open))
	}
	return opts
}

// seqText is a text as Mermaid writes it: <br> breaks a line, and an
// entity such as #59; or #quot; is its character.
func seqText(s string) string {
	s = strings.TrimSpace(s)
	s = brRe.ReplaceAllString(s, "\n")
	return seqEntity.ReplaceAllStringFunc(s, func(e string) string {
		name := e[1 : len(e)-1]
		if n, err := strconv.Atoi(name); err == nil {
			return string(rune(n))
		}
		if u := html.UnescapeString("&" + name + ";"); u != "&"+name+";" {
			return u
		}
		return e
	})
}
