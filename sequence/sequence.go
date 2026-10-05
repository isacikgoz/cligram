// Package sequence draws sequence diagrams in the terminal: participants
// side by side, their lifelines down the page, and the messages between
// them in the order they are sent.
//
//	d := sequence.New()
//	d.Participant("alice", "Alice", sequence.Actor())
//	d.Participant("api", "API")
//	d.Message("alice", "api", "GET /orders", sequence.Activate())
//	d.Block("alt", "found")
//	d.Message("api", "alice", "200 orders", sequence.Reply(), sequence.Deactivate())
//	d.Section("else", "missing")
//	d.Message("api", "alice", "404", sequence.Reply(), sequence.Deactivate())
//	d.End()
//	fmt.Println(d.Layout(sequence.Fit(80, 24)).Render(cligram.ANSI))
//
// It draws with cligram's glyphs and themes, so a sequence diagram looks
// like a flowchart drawn beside it. Package mermaid reads one from a
// Mermaid sequenceDiagram.
package sequence

import (
	"errors"
	"fmt"
	"strings"

	"github.com/isacikgoz/cligram"
)

// Diagram is a sequence diagram: participants, and what happens between
// them in order. Like cligram.Diagram, it collects mistakes instead of
// returning them per call: Check lists them all.
type Diagram struct {
	participants []Participant
	index        map[string]int
	declared     map[string]bool
	steps        []step
	open         []int // the blocks not yet ended, innermost last: their steps
	active       map[string]int
	errs         []error
}

// Participant is one of a diagram's columns.
type Participant struct {
	ID, Text string
	// Actor marks a person rather than a system: drawn in a heavy box.
	Actor bool
	Class string
}

// Head is how a message ends at its receiver.
type Head int

const (
	Arrow Head = iota // ──►
	Open              // a line with no head: ──┤
	Cross             // ──╳, a message lost or refused
)

// Message is a message from one participant to another, or to itself.
type Message struct {
	From, To, Text string
	Line           cligram.LineStyle
	Head           Head
	// Both draws an arrowhead at each end.
	Both bool
	// Activate activates To once the message arrives; Deactivate ends
	// From's activation once it is sent, as Mermaid's + and - do.
	Activate, Deactivate bool
}

// MessageOption sets something about a message.
type MessageOption func(*Message)

// Reply draws the message dashed, as an answer is.
func Reply() MessageOption { return func(m *Message) { m.Line = cligram.Dashed } }

// Line sets the message's line style.
func Line(s cligram.LineStyle) MessageOption { return func(m *Message) { m.Line = s } }

// WithHead sets how the message ends.
func WithHead(h Head) MessageOption { return func(m *Message) { m.Head = h } }

// BothWays draws an arrowhead at each end of the message, whatever head
// it was given.
func BothWays() MessageOption { return func(m *Message) { m.Both = true } }

// Activate activates the receiver once the message arrives.
func Activate() MessageOption { return func(m *Message) { m.Activate = true } }

// Deactivate ends the sender's activation once the message is sent.
func Deactivate() MessageOption { return func(m *Message) { m.Deactivate = true } }

// ParticipantOption sets something about a participant.
type ParticipantOption func(*Participant)

// Actor marks the participant as a person.
func Actor() ParticipantOption { return func(p *Participant) { p.Actor = true } }

// Class names what the participant is, for the theme to color.
func Class(name string) ParticipantOption { return func(p *Participant) { p.Class = name } }

// NotePlace is where a note goes: beside a participant's lifeline, or
// over one or two of them.
type NotePlace struct {
	side     noteSide
	from, to string
}

type noteSide int

const (
	noteRight noteSide = iota
	noteLeft
	noteOver
)

// RightOf puts a note right of a participant's lifeline.
func RightOf(id string) NotePlace { return NotePlace{noteRight, id, id} }

// LeftOf puts a note left of a participant's lifeline.
func LeftOf(id string) NotePlace { return NotePlace{noteLeft, id, id} }

// Over puts a note over a participant's lifeline, or over the lifelines
// from one participant to another.
func Over(id string, to ...string) NotePlace {
	p := NotePlace{noteOver, id, id}
	if len(to) > 0 {
		p.to = to[0]
	}
	return p
}

type stepKind int

const (
	stepMessage stepKind = iota
	stepNote
	stepActivate
	stepDeactivate
	stepBlock   // a block opens: loop, alt, opt, par, critical, break
	stepSection // a block's next part: else, and, option
	stepEnd     // a block closes
	stepNumber  // numbering starts, or stops
)

// step is one thing in the diagram's order.
type step struct {
	kind stepKind
	msg  Message
	note NotePlace
	id   string // the participant activated or deactivated
	// word and text are a block's or section's: "loop", "every minute".
	word, text string
	start, by  int // numbering: from start, by; by 0 stops it
}

// New makes an empty diagram.
func New() *Diagram {
	return &Diagram{index: map[string]int{}, declared: map[string]bool{}, active: map[string]int{}}
}

func (d *Diagram) fail(format string, args ...any) {
	d.errs = append(d.errs, fmt.Errorf(format, args...))
}

// Participant declares a participant: its column comes in the order
// participants are first named, declared or not. Text is what its box
// says; empty, it shows the id.
func (d *Diagram) Participant(id, text string, opts ...ParticipantOption) {
	if strings.TrimSpace(id) == "" {
		d.fail("a participant needs an id")
		return
	}
	if d.declared[id] {
		d.fail("participant %q is declared twice", id)
		return
	}
	d.declared[id] = true
	p := d.participant(id)
	p.Text = text
	for _, o := range opts {
		o(p)
	}
}

// participant is the participant id, named for the first time if it is.
func (d *Diagram) participant(id string) *Participant {
	i, ok := d.index[id]
	if !ok {
		i = len(d.participants)
		d.index[id] = i
		d.participants = append(d.participants, Participant{ID: id})
	}
	return &d.participants[i]
}

// named names a participant a step mentions, and says whether it can.
func (d *Diagram) named(what, id string) bool {
	if strings.TrimSpace(id) == "" {
		d.fail("%s names no participant", what)
		return false
	}
	d.participant(id)
	return true
}

// Message sends a message from one participant to another; from and to
// may be the same. A participant not declared yet is named by it.
func (d *Diagram) Message(from, to, text string, opts ...MessageOption) {
	m := Message{From: from, To: to, Text: text}
	for _, o := range opts {
		o(&m)
	}
	if m.Both {
		m.Head = Arrow
	}
	what := fmt.Sprintf("message %q", text)
	if !d.named(what, from) || !d.named(what, to) {
		return
	}
	if m.Deactivate && !d.deactivate(from, what) {
		m.Deactivate = false
	}
	if m.Activate {
		d.active[to]++
	}
	d.steps = append(d.steps, step{kind: stepMessage, msg: m})
}

// Note adds a note at place.
func (d *Diagram) Note(text string, at NotePlace) {
	what := fmt.Sprintf("note %q", text)
	if !d.named(what, at.from) || !d.named(what, at.to) {
		return
	}
	d.steps = append(d.steps, step{kind: stepNote, note: at, text: text})
}

// Activate activates a participant: its lifeline is drawn heavy until it
// is deactivated as often as it was activated.
func (d *Diagram) Activate(id string) {
	if !d.named("activate", id) {
		return
	}
	d.active[id]++
	d.steps = append(d.steps, step{kind: stepActivate, id: id})
}

// Deactivate ends a participant's latest activation.
func (d *Diagram) Deactivate(id string) {
	if !d.named("deactivate", id) || !d.deactivate(id, "deactivate") {
		return
	}
	d.steps = append(d.steps, step{kind: stepDeactivate, id: id})
}

func (d *Diagram) deactivate(id, what string) bool {
	if d.active[id] == 0 {
		d.fail("%s: %q is not active", what, id)
		return false
	}
	d.active[id]--
	return true
}

// Block opens a framed block of what follows, until End: word says what
// it is (loop, alt, opt, par, critical, break) and text when or why.
func (d *Diagram) Block(word, text string) {
	if strings.TrimSpace(word) == "" {
		d.fail("a block needs a word, such as loop or alt")
		return
	}
	d.open = append(d.open, len(d.steps))
	d.steps = append(d.steps, step{kind: stepBlock, word: word, text: text})
}

// Section starts the open block's next part: word is else, and, or
// option, as the block is alt, par, or critical.
func (d *Diagram) Section(word, text string) {
	if len(d.open) == 0 {
		d.fail("%s %q is outside any block", word, text)
		return
	}
	d.steps = append(d.steps, step{kind: stepSection, word: word, text: text})
}

// End closes the innermost open block.
func (d *Diagram) End() {
	if len(d.open) == 0 {
		d.fail("end closes no block")
		return
	}
	d.open = d.open[:len(d.open)-1]
	d.steps = append(d.steps, step{kind: stepEnd})
}

// Autonumber numbers the messages from here, from start up by by; by 0
// stops numbering.
func (d *Diagram) Autonumber(start, by int) {
	d.steps = append(d.steps, step{kind: stepNumber, start: start, by: by})
}

// Check lists every mistake made building the diagram, or nil. A block
// left open is a mistake; it is drawn closed at the end.
func (d *Diagram) Check() error {
	errs := d.errs
	for _, i := range d.open {
		s := d.steps[i]
		errs = append(errs, fmt.Errorf("%s %q is not closed with end", s.word, s.text))
	}
	if len(d.participants) == 0 {
		errs = append(errs, errors.New("the diagram has no participants"))
	}
	return errors.Join(errs...)
}

// Participants are the diagram's participants, left to right.
func (d *Diagram) Participants() []Participant {
	return append([]Participant(nil), d.participants...)
}

// Messages are the diagram's messages, in the order they are sent.
func (d *Diagram) Messages() []Message {
	var out []Message
	for _, s := range d.steps {
		if s.kind == stepMessage {
			out = append(out, s.msg)
		}
	}
	return out
}
