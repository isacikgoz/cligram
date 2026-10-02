// Package bubble is a Bubble Tea component that shows a cligram diagram.
//
// It fits the diagram to the room the host gives it, paints a run's state
// as it changes, and keeps a focus the reader moves: by sight with the
// arrow keys, to the nearest box that way as drawn, or along the flow,
// which works the same at any size. A node that leads into another diagram
// opens into it, and closes back out.
//
// The host owns what the diagram means. It sends StateMsg as a run moves,
// for the diagram it is about, the parent's box included; it hears
// FocusMsg to show the focused step's details, and OpenMsg when another
// diagram is shown.
package bubble

import (
	"context"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"

	"github.com/isacikgoz/cligram"
)

// KeyMap is the keys for each move, as Bubble Tea names them.
type KeyMap struct {
	// By sight: to the nearest box that way, as drawn.
	Up, Down, Left, Right []string
	// Along the flow, the same at any size: the next step, the step before,
	// and the other ways on from the step before.
	Next, Back, NextAlt, PrevAlt []string
	// Open the focused node's diagram; close back out to the one it is in.
	Open, Close []string
	// Follow the run again: the focus goes with the active step.
	Follow []string
}

// DefaultKeys are arrows or hjkl by sight; tab, shift+tab, ] and [ along
// the flow; enter and esc in and out; f to follow the run.
var DefaultKeys = KeyMap{
	Up: []string{"up", "k"}, Down: []string{"down", "j"},
	Left: []string{"left", "h"}, Right: []string{"right", "l"},
	Next: []string{"tab"}, Back: []string{"shift+tab"},
	NextAlt: []string{"]"}, PrevAlt: []string{"["},
	Open: []string{"enter"}, Close: []string{"esc"},
	Follow: []string{"f"},
}

// StateMsg sets where a run is in the diagram at Path: the ids of the
// nodes opened from the root to reach it, none for the root. A diagram not
// open yet keeps it for when it is. State.Focus is ignored: the focus is
// the reader's.
type StateMsg struct {
	Path  []string
	State cligram.State
}

// FocusMsg says the focus moved: to the node ID of the diagram at Path.
type FocusMsg struct {
	Path []string
	ID   string
}

// OpenMsg says which diagram is shown now: the one at Path.
type OpenMsg struct {
	Path []string
}

// loadedMsg brings a diagram a node opens into.
type loadedMsg struct {
	path []string
	d    *cligram.Diagram
	err  error
}

// Option sets up a Model.
type Option func(*Model)

// WithKeys moves the focus with k instead of DefaultKeys.
func WithKeys(k KeyMap) Option { return func(m *Model) { m.keys = k } }

// WithTheme colors with t instead of cligram.ANSI.
func WithTheme(t cligram.Theme) Option { return func(m *Model) { m.theme = t } }

// WithLayout lays every diagram out with opts as well, such as glyphs or
// text limits; the component fits them to its size itself.
func WithLayout(opts ...cligram.LayoutOption) Option {
	return func(m *Model) { m.layout = append(m.layout, opts...) }
}

// Model is the component. Like other Bubble Tea components it is a value:
// keep the one Update returns.
type Model struct {
	frames []*frame
	states map[string]cligram.State
	w, h   int
	keys   KeyMap
	theme  cligram.Theme
	layout []cligram.LayoutOption
	// opening is the path being opened, and failed what went wrong last.
	opening []string
	failed  string
}

// frame is a diagram on the stack of those opened.
type frame struct {
	path  []string
	title string
	d     *cligram.Diagram
	l     *cligram.Layout
	view  cligram.Rect
	focus string
	// follow keeps the focus on the active step until the reader moves it.
	follow bool
	// history is where Next came from, for Back.
	history []string
}

// New shows d, titled title, with nothing yet known about a run.
func New(title string, d *cligram.Diagram, opts ...Option) Model {
	m := Model{states: map[string]cligram.State{}, keys: DefaultKeys, theme: cligram.ANSI}
	for _, o := range opts {
		o(&m)
	}
	m.frames = []*frame{m.newFrame(nil, title, d)}
	return m
}

func (m Model) newFrame(path []string, title string, d *cligram.Diagram) *frame {
	f := &frame{path: path, title: title, d: d, follow: true}
	if nodes := d.Nodes(); len(nodes) > 0 {
		f.focus = nodes[0].ID
	}
	if id := active(m.states[key(path)]); id != "" {
		f.focus = id
	}
	m.lay(f)
	return f
}

func key(path []string) string { return strings.Join(path, "\x00") }

// active is a node the run is at, if any: the first, in no set order, that
// is Active, else one Waiting.
func active(st cligram.State) string {
	var waiting string
	ids := make([]string, 0, len(st.Status))
	for id := range st.Status {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		switch st.Status[id] {
		case cligram.Active:
			return id
		case cligram.Waiting:
			if waiting == "" {
				waiting = id
			}
		}
	}
	return waiting
}

// lay lays f out for the room there is, below the title line.
func (m Model) lay(f *frame) {
	w, h := m.room()
	opts := append(slices.Clone(m.layout), cligram.Fit(w, h))
	f.l = f.d.Layout(opts...)
	f.view = f.l.Reveal(cligram.Rect{X: f.view.X, Y: f.view.Y, W: w, H: h}, f.focus)
}

// room is the size of the window on the diagram: all but the title line.
func (m Model) room() (int, int) { return max(m.w, 1), max(m.h-1, 1) }

func (m Model) top() *frame { return m.frames[len(m.frames)-1] }

// SetSize gives the component w columns and h rows, and lays out again.
func (m Model) SetSize(w, h int) Model {
	if w == m.w && h == m.h {
		return m
	}
	m.w, m.h = w, h
	for _, f := range m.frames {
		m.lay(f)
	}
	return m
}

// Path is the diagram shown: the nodes opened from the root to reach it.
func (m Model) Path() []string { return slices.Clone(m.top().path) }

// Focus is the node in focus in the diagram shown.
func (m Model) Focus() string { return m.top().focus }

// Init does nothing; the host sends the size and the state.
func (m Model) Init() tea.Cmd { return nil }

// Update handles keys, StateMsg and diagrams that finished loading.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.key(msg.String())
	case StateMsg:
		return m.setState(msg)
	case loadedMsg:
		return m.loaded(msg)
	}
	return m, nil
}

func (m Model) setState(msg StateMsg) (Model, tea.Cmd) {
	k := key(msg.Path)
	m.states[k] = msg.State
	for _, f := range m.frames {
		if key(f.path) != k || !f.follow {
			continue
		}
		if id := active(msg.State); id != "" && id != f.focus {
			if f == m.top() {
				return m.focus(id)
			}
			f.focus = id
		}
	}
	return m, nil
}

func (m Model) key(k string) (Model, tea.Cmd) {
	f := m.top()
	is := func(keys []string) bool { return slices.Contains(keys, k) }
	sight := map[cligram.Side]bool{
		cligram.Top: is(m.keys.Up), cligram.Bottom: is(m.keys.Down),
		cligram.Left: is(m.keys.Left), cligram.Right: is(m.keys.Right),
	}
	for side, pressed := range sight {
		if pressed {
			f.follow = false
			return m.focus(f.l.Move(f.focus, side))
		}
	}
	switch {
	case is(m.keys.Next):
		f.follow = false
		if next := m.next(f); next != "" {
			f.history = append(f.history, f.focus)
			return m.focus(next)
		}
	case is(m.keys.Back):
		f.follow = false
		if n := len(f.history); n > 0 {
			back := f.history[n-1]
			f.history = f.history[:n-1]
			return m.focus(back)
		}
		if in := f.d.In(f.focus); len(in) > 0 {
			return m.focus(in[0].From)
		}
	case is(m.keys.NextAlt), is(m.keys.PrevAlt):
		f.follow = false
		step := 1
		if is(m.keys.PrevAlt) {
			step = -1
		}
		return m.focus(m.alternative(f, step))
	case is(m.keys.Follow):
		f.follow = true
		if id := active(m.states[key(f.path)]); id != "" {
			return m.focus(id)
		}
	case is(m.keys.Open):
		return m.open()
	case is(m.keys.Close):
		return m.close()
	}
	return m, nil
}

// next is where the run went from the focus, or else its first way on.
func (m Model) next(f *frame) string {
	out := f.d.Out(f.focus)
	if len(out) == 0 {
		return ""
	}
	st := m.states[key(f.path)]
	for _, e := range out {
		if slices.Contains(st.Taken, e.Ref()) {
			return e.To
		}
	}
	return out[0].To
}

// alternative is the step step places on among the other ways on from the
// step before the focus, round and round.
func (m Model) alternative(f *frame, step int) string {
	parent := ""
	if n := len(f.history); n > 0 {
		parent = f.history[n-1]
	}
	if parent == "" || !leadsTo(f.d, parent, f.focus) {
		in := f.d.In(f.focus)
		if len(in) == 0 {
			return f.focus
		}
		parent = in[0].From
	}
	var ways []string
	for _, e := range f.d.Out(parent) {
		if !slices.Contains(ways, e.To) {
			ways = append(ways, e.To)
		}
	}
	i := slices.Index(ways, f.focus)
	if i < 0 || len(ways) < 2 {
		return f.focus
	}
	return ways[(i+step+len(ways))%len(ways)]
}

func leadsTo(d *cligram.Diagram, from, to string) bool {
	return slices.ContainsFunc(d.Out(from), func(e cligram.Edge) bool { return e.To == to })
}

// focus moves the focus to id in the diagram shown, scrolls it into view,
// and tells the host.
func (m Model) focus(id string) (Model, tea.Cmd) {
	f := m.top()
	if id == f.focus {
		return m, nil
	}
	f.focus = id
	f.view = f.l.Reveal(f.view, id)
	path := slices.Clone(f.path)
	return m, func() tea.Msg { return FocusMsg{Path: path, ID: id} }
}

func (m Model) open() (Model, tea.Cmd) {
	f := m.top()
	n, ok := f.d.Lookup(f.focus)
	if !ok || n.Sub == nil || m.opening != nil {
		return m, nil
	}
	path := append(slices.Clone(f.path), n.ID)
	m.opening, m.failed = path, ""
	sub := n.Sub
	return m, func() tea.Msg {
		d, err := sub(context.Background())
		return loadedMsg{path: path, d: d, err: err}
	}
}

func (m Model) loaded(msg loadedMsg) (Model, tea.Cmd) {
	if !slices.Equal(msg.path, m.opening) {
		return m, nil // opened something else since, or closed
	}
	m.opening = nil
	if msg.err != nil || msg.d == nil {
		m.failed = "could not open: no diagram"
		if msg.err != nil {
			m.failed = "could not open: " + msg.err.Error()
		}
		return m, nil
	}
	parent := m.top()
	n, _ := parent.d.Lookup(msg.path[len(msg.path)-1])
	title := strings.Join(strings.Fields(n.Label()), " ")
	m.frames = append(slices.Clone(m.frames), m.newFrame(msg.path, title, msg.d))
	return m.shown()
}

func (m Model) close() (Model, tea.Cmd) {
	if m.opening != nil {
		m.opening = nil
		return m, nil
	}
	if len(m.frames) < 2 {
		return m, nil
	}
	m.frames = m.frames[:len(m.frames)-1]
	m.failed = ""
	return m.shown()
}

// shown tells the host which diagram is shown, and what is in focus there.
func (m Model) shown() (Model, tea.Cmd) {
	f := m.top()
	path, id := slices.Clone(f.path), f.focus
	return m, tea.Batch(
		func() tea.Msg { return OpenMsg{Path: path} },
		func() tea.Msg { return FocusMsg{Path: path, ID: id} },
	)
}

// View is a title line, the diagrams opened to reach this one, then as
// much of the diagram as fits, the focus in sight: exactly the rows and no
// more than the columns SetSize gave.
func (m Model) View() string {
	f := m.top()
	var titles []string
	for _, fr := range m.frames {
		titles = append(titles, fr.title)
	}
	title := strings.Join(titles, " › ")
	switch {
	case m.opening != nil:
		title += " › opening…"
	case m.failed != "":
		title += " · " + m.failed
	}
	w, h := m.room()
	st := m.states[key(f.path)]
	st.Focus = f.focus
	body := f.l.RenderView(st, m.theme, cligram.Rect{X: f.view.X, Y: f.view.Y, W: w, H: h})
	return clip(title, w) + "\n" + body
}

// clip shortens a title line to w cells, ending in an ellipsis when cut.
func clip(s string, w int) string {
	if uniseg.StringWidth(s) <= w {
		return s
	}
	var b strings.Builder
	used := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() && used+g.Width() <= w-1 {
		b.WriteString(g.Str())
		used += g.Width()
	}
	return b.String() + "…"
}
