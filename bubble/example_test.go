package bubble_test

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/bubble"
)

// host is a Bubble Tea program that shows a flow and the focused step.
type host struct {
	diagram bubble.Model
	details string
}

func (h host) Init() tea.Cmd { return nil }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.diagram = h.diagram.SetSize(msg.Width, msg.Height-1) // a row for details
		return h, nil
	case bubble.FocusMsg: // show that step's details
		h.details = fmt.Sprintf("%v %s", msg.Path, msg.ID)
		return h, nil
	case bubble.OpenMsg: // another diagram is shown
		return h, nil
	}
	var cmd tea.Cmd
	h.diagram, cmd = h.diagram.Update(msg)
	return h, cmd
}

func (h host) View() string { return h.diagram.View() + "\n" + h.details }

func Example() {
	d := cligram.New()
	d.Node("triage", "Triage")
	d.Node("draft", "Draft a reply", cligram.Sub(func(context.Context) (*cligram.Diagram, error) {
		inner := cligram.New()
		inner.Node("write", "Write")
		return inner, nil
	}))
	d.Edge("triage", "draft")

	h := host{diagram: bubble.New("Inbox", d, bubble.WithTheme(cligram.Plain))}
	m, _ := h.Update(tea.WindowSizeMsg{Width: 60, Height: 8})

	// As the run moves: Path nil is the root diagram.
	m, cmd := m.Update(bubble.StateMsg{State: cligram.State{
		Status: map[string]cligram.Status{"triage": cligram.Done, "draft": cligram.Active},
	}})
	m, _ = m.Update(cmd()) // the component tells the host the focus moved
	fmt.Println(m.(host).details)
	// Output:
	// [] draft
}
