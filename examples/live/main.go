// Live shows the factory loop being worked through: a pretend run moves
// from step to step while you move around the diagram. Implement opens
// into its own flow.
//
//	go run ./examples/live
//
// Arrows or hjkl move by sight; tab, shift+tab, ] and [ along the flow;
// enter and esc open and close; f follows the run; q quits.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/bubble"
)

func main() {
	if _, err := tea.NewProgram(newApp(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// factory is the factory loop, the way ackt would hand it over: steps and
// the outcomes between them, and no placements.
func factory() *cligram.Diagram {
	d := cligram.New()
	step := func(id, text string, opts ...cligram.NodeOption) { d.Node(id, text, opts...) }
	decision := cligram.As(cligram.Decision)
	agent, human := cligram.Class("agent"), cligram.Class("human")
	step("triage", "Triage agent runs", agent)
	step("outcome", "Triage\noutcome", decision)
	step("impl", "Implementation\nagent runs", agent, cligram.Sub(drafting))
	step("review", "Code review\nagent runs", agent)
	step("verify", "Verification\nagent runs", agent)
	step("ready", "Ready to ship?", decision, human)
	step("ship", "Ship it")
	step("monitor", "Monitoring\nagent runs", agent)
	step("issue", "Issue\ndetected", decision)
	step("create", "Create issue")
	step("spec", "Spec agent runs", agent)
	step("specreview", "Human review specs", decision, human)
	step("human", "Human provides input", human)
	step("park", "Park issue for now", cligram.As(cligram.Terminal))
	step("watching", "Continue\nmonitoring", cligram.As(cligram.Terminal))

	edge := func(from, to, label string) { d.Edge(from, to, cligram.Label(label)) }
	edge("triage", "outcome", "")
	edge("outcome", "impl", "automatable")
	edge("outcome", "spec", "needs specs")
	edge("outcome", "human", "needs human")
	edge("outcome", "park", "park")
	edge("spec", "specreview", "")
	edge("specreview", "impl", "approved")
	edge("specreview", "spec", "needs revision")
	edge("human", "triage", "")
	edge("impl", "review", "")
	edge("review", "verify", "")
	edge("verify", "ready", "")
	edge("ready", "ship", "yes")
	edge("ready", "impl", "not ready")
	edge("ship", "monitor", "")
	edge("monitor", "issue", "")
	edge("issue", "create", "yes")
	edge("issue", "watching", "no")
	edge("create", "triage", "")
	return d
}

// drafting is what Implementation opens into.
func drafting(context.Context) (*cligram.Diagram, error) {
	time.Sleep(200 * time.Millisecond) // as if it came over the wire
	d := cligram.New()
	d.Node("plan", "Plan the change")
	d.Node("edit", "Edit the code")
	d.Node("test", "Run the tests", cligram.As(cligram.Decision))
	d.Node("done", "Done", cligram.As(cligram.Terminal))
	d.Edge("plan", "edit")
	d.Edge("edit", "test")
	d.Edge("test", "done", cligram.Label("pass"))
	d.Edge("test", "edit", cligram.Label("fail"))
	return d, nil
}

// The pretend run: the steps it goes through, by the edges it takes.
var script = []cligram.EdgeRef{
	{From: "triage", To: "outcome"},
	{From: "outcome", To: "impl", Label: "automatable"},
	{From: "impl", To: "review"},
	{From: "review", To: "verify"},
	{From: "verify", To: "ready"},
	{From: "ready", To: "ship", Label: "yes"},
	{From: "ship", To: "monitor"},
	{From: "monitor", To: "issue"},
	{From: "issue", To: "watching", Label: "no"},
}

type tick struct{}

type app struct {
	d       *cligram.Diagram
	diagram bubble.Model
	at      int // how far the run has got along the script
	focus   string
	w, h    int
}

// palette colors agent steps bright blue and steps waiting on a person magenta,
// until the run reaches them.
var palette = cligram.Palette{
	Status:  cligram.DefaultPalette.Status,
	Classes: map[string]string{"agent": "bright-blue", "human": "magenta"},
	Line:    cligram.DefaultPalette.Line,
	Label:   cligram.DefaultPalette.Label,
}

func newApp() app {
	d := factory()
	return app{d: d, diagram: bubble.New("Factory loop", d, bubble.WithTheme(palette.Theme())), focus: "triage"}
}

func (a app) Init() tea.Cmd {
	return tea.Batch(a.state(), next())
}

func next() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return tick{} })
}

// state tells the diagram where the run is.
func (a app) state() tea.Cmd {
	st := cligram.State{Status: map[string]cligram.Status{}}
	current := "triage"
	for _, e := range script[:a.at] {
		st.Status[e.From] = cligram.Done
		st.Taken = append(st.Taken, e)
		current = e.To
	}
	st.Status[current] = cligram.Active
	if a.at == len(script) {
		st.Status[current] = cligram.Done
	}
	return func() tea.Msg { return bubble.StateMsg{State: st} }
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		a.diagram = a.diagram.SetSize(msg.Width, msg.Height-1)
		return a, nil
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	case tick:
		if a.at < len(script) {
			a.at++
			return a, tea.Batch(a.state(), next())
		}
		return a, nil
	case bubble.FocusMsg:
		a.focus = msg.ID
		if len(msg.Path) > 0 {
			a.focus = strings.Join(msg.Path, " › ") + " › " + msg.ID
		}
		return a, nil
	}
	var cmd tea.Cmd
	a.diagram, cmd = a.diagram.Update(msg)
	return a, cmd
}

func (a app) View() string {
	help := "focus: " + a.focus + "  ·  arrows/hjkl look  tab ⇥ next  shift+tab back  ] [ other ways  enter open  esc close  f follow  q quit"
	if w := a.w; w > 0 && len([]rune(help)) > w {
		help = string([]rune(help)[:w])
	}
	return a.diagram.View() + "\n" + help
}
