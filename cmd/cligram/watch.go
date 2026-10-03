package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/bubble"
	"github.com/isacikgoz/cligram/internal/draw"
	"github.com/isacikgoz/cligram/internal/watch"
)

// runWatch is cligram watch: a diagram, and where a run is in it as events
// come in on stdin. In a terminal it is shown full screen and repainted as
// each event comes, with the keyboard to look around; when the events end
// the final picture is printed, to stay in the scrollback. Elsewhere, the
// final picture only.
func runWatch(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	complain := func(args ...any) { _, _ = fmt.Fprintln(stderr, args...) }
	flags := flag.NewFlagSet("cligram watch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		complain("usage: events | cligram watch [flags] flow.mmd")
		complain(`  events, one a line: "build active", "build -> test", "reset", or JSON`)
		complain(`  such as {"node":"build","status":"done"}; statuses are idle, active,`)
		complain("  done, failed and waiting")
		flags.PrintDefaults()
	}
	hold := flags.Bool("hold", false, "in a terminal, stay open when the events end, until q")
	width := flags.Int("width", 0, "fit the final picture to this many columns (default: the terminal's, if it is one)")
	ascii := flags.Bool("ascii", false, "draw with ASCII only")
	color := flags.String("color", "auto", "color: auto (when writing to a terminal), always or never")
	format := flags.String("format", "auto", "the diagram: auto, mermaid or yaml")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	if !validColor(*color) {
		complain("cligram watch: -color is auto, always or never, not", *color)
		return 2
	}
	source, err := os.ReadFile(flags.Arg(0))
	if err != nil {
		complain("cligram watch:", err)
		return 1
	}
	src, err := draw.Read(string(source), *format)
	if err != nil {
		complain("cligram watch:", err)
		return 1
	}
	tty := isTerminal(stdout)
	req := draw.Request{ASCII: *ascii, Color: colored(*color, tty)}
	title := src.Title
	if title == "" {
		title = flags.Arg(0)
	}

	events := make(chan event)
	go read(stdin, src.Diagram, events)

	var st cligram.State
	shown := false
	if tty {
		var problems []error
		st, problems, shown = show(title, src, req, events, stdout, *hold)
		for _, p := range problems {
			complain("cligram watch:", p)
		}
	}
	if !shown {
		// Not a terminal, or none could be opened: the events add up to
		// the final picture alone.
		for ev := range events {
			if ev.err != nil {
				complain("cligram watch:", ev.err)
				continue
			}
			st = ev.state
		}
	}

	if w, _, ok := terminalSize(stdout); ok && *width == 0 {
		*width = w
	}
	req.Width, req.State = *width, st
	opts, err := req.Options(src)
	if err != nil {
		complain("cligram watch:", err)
		return 1
	}
	theme := cligram.Plain
	if req.Color {
		theme = cligram.ANSI
	}
	l := src.Diagram.Layout(opts...)
	out := l.Render(st, theme) + "\n"
	if src.Title != "" {
		out = src.Title + "\n\n" + out
	}
	if _, err := io.WriteString(stdout, out); err != nil {
		complain("cligram watch:", err)
		return 1
	}
	return 0
}

// event is the state after one more line of events, or what was wrong
// with the line.
type event struct {
	state cligram.State
	line  string
	err   error
}

// read reads events from r until it ends, sending the state each adds up
// to, and closes out.
func read(r io.Reader, d *cligram.Diagram, out chan<- event) {
	defer close(out)
	var st cligram.State
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		ev, ok, err := watch.Parse(sc.Text())
		if err == nil && ok {
			next := clone(st)
			if err = watch.Apply(d, &next, ev); err == nil {
				st = next
			}
		}
		if err != nil {
			out <- event{state: st, line: sc.Text(), err: fmt.Errorf("line %d: %w", n, err)}
			continue
		}
		if ok {
			out <- event{state: clone(st), line: sc.Text()}
		}
	}
	if err := sc.Err(); err != nil {
		out <- event{state: st, err: err}
	}
}

func clone(st cligram.State) cligram.State {
	c := cligram.State{Status: map[string]cligram.Status{}, Focus: st.Focus}
	for k, v := range st.Status {
		c.Status[k] = v
	}
	c.Taken = append(c.Taken, st.Taken...)
	return c
}

// show runs the full-screen view until the events end (or, holding, until
// q), and gives the final state and the problems met. ok is false if no
// terminal could be opened for it.
func show(title string, src *draw.Diagram, req draw.Request, events <-chan event, out io.Writer, hold bool) (cligram.State, []error, bool) {
	opts, err := req.Options(src)
	if err != nil {
		return cligram.State{}, []error{err}, false
	}
	theme := cligram.Plain
	if req.Color {
		theme = cligram.ANSI
	}
	m := watching{
		view:   bubble.New(title, src.Diagram, bubble.WithTheme(theme), bubble.WithLayout(opts...)),
		events: events,
		hold:   hold,
	}
	// Keys come from the terminal itself: stdin is the events.
	topts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithOutput(out)}
	if tty, err := os.Open("/dev/tty"); err == nil {
		defer func() { _ = tty.Close() }()
		topts = append(topts, tea.WithInput(tty))
	} else {
		topts = append(topts, tea.WithInput(nil))
	}
	final, err := tea.NewProgram(m, topts...).Run()
	if err != nil {
		return cligram.State{}, []error{err}, false
	}
	w := final.(watching)
	if !w.ended {
		// Left before the events ended: the rest are read, so a writer
		// into the pipe is not stopped short.
		for ev := range events {
			if ev.err == nil {
				w.state = ev.state
			}
		}
	}
	return w.state, w.problems, true
}

// watching is the full-screen view: the diagram, and a line saying what
// came last.
type watching struct {
	view     bubble.Model
	events   <-chan event
	state    cligram.State
	last     string
	problems []error
	count    int
	ended    bool
	hold     bool
	w        int
}

type eventMsg event
type endMsg struct{}

func (m watching) next() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.events
		if !ok {
			return endMsg{}
		}
		return eventMsg(ev)
	}
}

func (m watching) Init() tea.Cmd { return m.next() }

func (m watching) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w = msg.Width
		var cmd tea.Cmd
		m.view, cmd = m.view.Resize(msg.Width, msg.Height-1)
		return m, cmd
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case eventMsg:
		if msg.err != nil {
			m.problems = append(m.problems, msg.err)
			m.last = msg.err.Error()
			return m, m.next()
		}
		m.state, m.last = msg.state, msg.line
		m.count++
		var cmd tea.Cmd
		m.view, cmd = m.view.Update(bubble.StateMsg{State: msg.state})
		return m, tea.Batch(cmd, m.next())
	case endMsg:
		m.ended = true
		if !m.hold {
			return m, tea.Quit
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.view, cmd = m.view.Update(msg)
	return m, cmd
}

func (m watching) View() string {
	status := fmt.Sprintf("%d events", m.count)
	if m.last != "" {
		status += " · last: " + m.last
	}
	if len(m.problems) > 0 {
		status += fmt.Sprintf(" · %d not understood", len(m.problems))
	}
	if m.ended {
		status += " · the events ended · q quits"
	} else {
		status += " · q quits"
	}
	for m.w > 1 && uniseg.StringWidth(status) > m.w {
		g := uniseg.NewGraphemes(status)
		var cut []string
		for g.Next() {
			cut = append(cut, g.Str())
		}
		status = strings.Join(cut[:len(cut)-2], "") + "…"
	}
	return m.view.View() + "\n" + status
}
