package bubble

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"

	"github.com/isacikgoz/cligram"
)

// triage is the top of the factory loop: a decision with four ways on,
// two of them back again, and a step that opens into its own flow.
func triage(sub cligram.SubFunc) *cligram.Diagram {
	d := cligram.New()
	d.Node("new", "New task")
	d.Node("triage", "Triage\noutcome", cligram.As(cligram.Decision))
	d.Node("impl", "Implement", cligram.Sub(sub))
	d.Node("spec", "Spec")
	d.Node("human", "Ask a human")
	d.Node("park", "Park", cligram.As(cligram.Terminal))
	d.Node("review", "Review")
	d.Edge("new", "triage")
	d.Edge("triage", "impl", cligram.Label("automatable"))
	d.Edge("triage", "spec", cligram.Label("needs specs"))
	d.Edge("triage", "human", cligram.Label("needs human"))
	d.Edge("triage", "park", cligram.Label("park"))
	d.Edge("spec", "impl")
	d.Edge("human", "triage")
	d.Edge("impl", "review")
	return d
}

// drafting is what impl opens into.
func drafting(context.Context) (*cligram.Diagram, error) {
	d := cligram.New()
	d.Node("draft", "Draft")
	d.Node("check", "Check")
	d.Edge("draft", "check")
	return d, nil
}

// run sends msg, runs the commands that come back and feeds what they
// bring that is meant for the model back in, and returns what was meant
// for the host.
func run(t *testing.T, m Model, msg tea.Msg) (Model, []tea.Msg) {
	t.Helper()
	var out []tea.Msg
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		msg, queue = queue[0], queue[1:]
		switch msg := msg.(type) {
		case FocusMsg, OpenMsg:
			out = append(out, msg)
			continue
		case tea.BatchMsg:
			for _, c := range msg {
				if c != nil {
					queue = append(queue, c())
				}
			}
			continue
		}
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd != nil {
			queue = append(queue, cmd())
		}
	}
	return m, out
}

var keys = map[string]tea.KeyMsg{
	"tab": {Type: tea.KeyTab}, "shift+tab": {Type: tea.KeyShiftTab},
	"enter": {Type: tea.KeyEnter}, "esc": {Type: tea.KeyEsc},
	"up": {Type: tea.KeyUp}, "down": {Type: tea.KeyDown},
	"left": {Type: tea.KeyLeft}, "right": {Type: tea.KeyRight},
}

// press presses each key in turn.
func press(t *testing.T, m Model, ks ...string) (Model, []tea.Msg) {
	t.Helper()
	var all []tea.Msg
	for _, k := range ks {
		msg, ok := keys[k]
		if !ok {
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		var out []tea.Msg
		m, out = run(t, m, msg)
		all = append(all, out...)
	}
	return m, all
}

func model(opts ...Option) Model {
	opts = append([]Option{WithTheme(cligram.Plain)}, opts...)
	return New("Factory", triage(drafting), opts...).SetSize(120, 30)
}

func TestTheFocusStartsAtTheFirstStep(t *testing.T) {
	if got := model().Focus(); got != "new" {
		t.Errorf("focus %s", got)
	}
}

func TestTabFollowsTheFlowAndShiftTabComesBack(t *testing.T) {
	m, out := press(t, model(), "tab", "tab", "tab")
	if m.Focus() != "review" {
		t.Errorf("focus %s", m.Focus())
	}
	if want := []tea.Msg{FocusMsg{ID: "triage"}, FocusMsg{ID: "impl"}, FocusMsg{ID: "review"}}; !equalMsgs(out, want) {
		t.Errorf("told the host %v", out)
	}
	m, _ = press(t, m, "shift+tab", "shift+tab")
	if m.Focus() != "triage" {
		t.Errorf("back twice: %s", m.Focus())
	}
}

func TestTabTakesTheWayTheRunWent(t *testing.T) {
	m, _ := run(t, model(), StateMsg{State: cligram.State{
		Taken: []cligram.EdgeRef{{From: "triage", To: "human", Label: "needs human"}},
	}})
	m, _ = press(t, m, "tab", "tab")
	if m.Focus() != "human" {
		t.Errorf("focus %s", m.Focus())
	}
}

func TestShiftTabWithNoHistoryGoesToAStepThatLeadsHere(t *testing.T) {
	m, _ := press(t, model(), "tab", "tab", "]") // impl, then spec: history is new, triage
	m, _ = press(t, m, "shift+tab", "shift+tab", "shift+tab")
	if m.Focus() != "new" {
		t.Errorf("focus %s", m.Focus())
	}
	if m, _ = press(t, m, "shift+tab"); m.Focus() != "new" {
		t.Errorf("nothing leads to new, yet the focus went to %s", m.Focus())
	}
}

func TestBracketsGoRoundTheOtherWaysOn(t *testing.T) {
	m, _ := press(t, model(), "tab", "tab")
	var seen []string
	for range 5 {
		m, _ = press(t, m, "]")
		seen = append(seen, m.Focus())
	}
	if got := strings.Join(seen, " "); got != "spec human park impl spec" {
		t.Errorf("] went %s", got)
	}
	m, _ = press(t, m, "[", "[")
	if m.Focus() != "park" {
		t.Errorf("[ went to %s", m.Focus())
	}
}

func TestArrowsGoWhereTheEyeGoes(t *testing.T) {
	m, _ := press(t, model(), "right")
	if m.Focus() != "triage" {
		t.Errorf("right of new: %s", m.Focus())
	}
	m, _ = press(t, m, "right", "down")
	if m.Focus() != "spec" {
		t.Errorf("right of triage, then down: %s", m.Focus())
	}
	m, _ = press(t, m, "k", "h")
	if m.Focus() != "triage" {
		t.Errorf("k then h: %s", m.Focus())
	}
}

func TestTheFocusFollowsTheRunUntilTheReaderMovesIt(t *testing.T) {
	m, out := run(t, model(), StateMsg{State: cligram.State{Status: map[string]cligram.Status{
		"new": cligram.Done, "triage": cligram.Active,
	}}})
	if m.Focus() != "triage" || !equalMsgs(out, []tea.Msg{FocusMsg{ID: "triage"}}) {
		t.Fatalf("focus %s, told %v", m.Focus(), out)
	}
	m, _ = press(t, m, "left")
	m, _ = run(t, m, StateMsg{State: cligram.State{Status: map[string]cligram.Status{
		"triage": cligram.Done, "impl": cligram.Active,
	}}})
	if m.Focus() != "new" {
		t.Errorf("the run took the focus from the reader: %s", m.Focus())
	}
	m, _ = press(t, m, "f")
	if m.Focus() != "impl" {
		t.Errorf("f did not go back to the run: %s", m.Focus())
	}
}

func TestEnterOpensAStepIntoItsOwnFlowAndEscClosesIt(t *testing.T) {
	m, _ := press(t, model(), "tab", "tab")
	m, out := press(t, m, "enter")
	if !slices.Equal(m.Path(), []string{"impl"}) || m.Focus() != "draft" {
		t.Fatalf("path %v focus %s", m.Path(), m.Focus())
	}
	if want := []tea.Msg{OpenMsg{Path: []string{"impl"}}, FocusMsg{Path: []string{"impl"}, ID: "draft"}}; !equalMsgs(out, want) {
		t.Errorf("told the host %v", out)
	}
	if title := strings.SplitN(m.View(), "\n", 2)[0]; title != "Factory › Implement" {
		t.Errorf("title %q", title)
	}
	m, out = press(t, m, "esc")
	if len(m.Path()) != 0 || m.Focus() != "impl" {
		t.Errorf("after esc: path %v focus %s", m.Path(), m.Focus())
	}
	if want := []tea.Msg{OpenMsg{Path: []string{}}, FocusMsg{Path: []string{}, ID: "impl"}}; !equalMsgs(out, want) {
		t.Errorf("told the host %v", out)
	}
	if m, _ = press(t, m, "esc"); len(m.Path()) != 0 {
		t.Error("esc at the root went somewhere")
	}
}

func TestEnterOnAPlainStepDoesNothing(t *testing.T) {
	m, out := press(t, model(), "enter")
	if len(m.Path()) != 0 || len(out) != 0 {
		t.Errorf("path %v, told %v", m.Path(), out)
	}
}

func TestAStateForAFlowNotOpenYetWaitsForIt(t *testing.T) {
	m, _ := run(t, model(), StateMsg{Path: []string{"impl"}, State: cligram.State{
		Status: map[string]cligram.Status{"draft": cligram.Done, "check": cligram.Active},
	}})
	m, _ = press(t, m, "tab", "tab", "enter")
	if m.Focus() != "check" {
		t.Errorf("focus %s", m.Focus())
	}
	if !strings.Contains(m.View(), "✓ Draft") {
		t.Errorf("the state was not painted:\n%s", m.View())
	}
}

func TestAFlowThatCannotBeOpenedSaysWhy(t *testing.T) {
	m := New("Factory", triage(func(context.Context) (*cligram.Diagram, error) {
		return nil, errors.New("playbook drafting is gone")
	}), WithTheme(cligram.Plain)).SetSize(120, 30)
	m, _ = press(t, m, "tab", "tab", "enter")
	if len(m.Path()) != 0 {
		t.Fatalf("opened %v", m.Path())
	}
	if title := strings.SplitN(m.View(), "\n", 2)[0]; title != "Factory · could not open: playbook drafting is gone" {
		t.Errorf("title %q", title)
	}
}

func TestClosingWhileOpeningLeavesTheFlowClosed(t *testing.T) {
	m, _ := press(t, model(), "tab", "tab")
	m, cmd := m.Update(keys["enter"])
	if !strings.Contains(m.View(), "opening…") {
		t.Errorf("no sign of opening:\n%s", m.View())
	}
	m, _ = press(t, m, "esc")
	m, _ = run(t, m, cmd()) // the diagram arrives after all
	if len(m.Path()) != 0 {
		t.Errorf("opened %v after esc", m.Path())
	}
}

func TestTheViewFillsItsRoomAndKeepsTheFocusInSight(t *testing.T) {
	d := cligram.New()
	var prev string
	for _, id := range strings.Fields("a b c d e f g h i j k l m n o p") {
		d.Node(id, "step "+id)
		if prev != "" {
			d.Edge(prev, id)
		}
		prev = id
	}
	m := New("Long", d, WithTheme(cligram.Plain)).SetSize(40, 12)
	for range 15 {
		m, _ = press(t, m, "tab")
		view := m.View()
		rows := strings.Split(view, "\n")
		if len(rows) != 12 {
			t.Fatalf("%d rows", len(rows))
		}
		for _, r := range rows {
			if w := uniseg.StringWidth(r); w > 40 {
				t.Fatalf("a row is %d cells: %q", w, r)
			}
		}
		if !strings.Contains(view, "step "+m.Focus()) {
			t.Fatalf("focus %s is out of sight:\n%s", m.Focus(), view)
		}
	}
}

func TestATitleTooLongIsCut(t *testing.T) {
	m := New("A factory loop with a long name", triage(drafting), WithTheme(cligram.Plain)).SetSize(12, 10)
	if title := strings.SplitN(m.View(), "\n", 2)[0]; title != "A factory l…" {
		t.Errorf("title %q", title)
	}
}

func TestKeysCanBeChanged(t *testing.T) {
	k := DefaultKeys
	k.Next = []string{"n"}
	m, _ := press(t, model(WithKeys(k)), "tab", "n")
	if m.Focus() != "triage" {
		t.Errorf("focus %s", m.Focus())
	}
}

func equalMsgs(got, want []tea.Msg) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		switch g := got[i].(type) {
		case FocusMsg:
			w, ok := want[i].(FocusMsg)
			if !ok || g.ID != w.ID || !slices.Equal(g.Path, w.Path) && (len(g.Path) != 0 || len(w.Path) != 0) {
				return false
			}
		case OpenMsg:
			w, ok := want[i].(OpenMsg)
			if !ok || !slices.Equal(g.Path, w.Path) && (len(g.Path) != 0 || len(w.Path) != 0) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
