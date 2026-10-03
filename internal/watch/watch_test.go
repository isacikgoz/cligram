package watch_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/internal/watch"
)

func flow() *cligram.Diagram {
	d := cligram.New()
	d.Node("build", "Build")
	d.Node("test", "Test", cligram.As(cligram.Decision))
	d.Node("fix", "Fix")
	d.Edge("build", "test")
	d.Edge("test", "fix", cligram.Label("no"))
	d.Edge("test", "fix", cligram.Label("flaky"))
	return d
}

func TestEventsAreJSONOrWords(t *testing.T) {
	for line, want := range map[string]watch.Event{
		`{"node":"build","status":"active"}`: {Node: "build", Status: "active"},
		"build active":                       {Node: "build", Status: "active"},
		`{"edge":["build","test"]}`:          {Edge: []string{"build", "test"}},
		"build -> test":                      {Edge: []string{"build", "test"}},
		"build->test":                        {Edge: []string{"build", "test"}},
		"test -> fix: no":                    {Edge: []string{"test", "fix", "no"}},
		`{"edge":["test","fix","no"]}`:       {Edge: []string{"test", "fix", "no"}},
		"reset":                              {Reset: true},
		`{"reset":true}`:                     {Reset: true},
	} {
		ev, ok, err := watch.Parse(line)
		if err != nil || !ok || !reflect.DeepEqual(ev, want) {
			t.Errorf("%q: %+v %v %v", line, ev, ok, err)
		}
	}
	for _, line := range []string{"", "   ", "# a comment"} {
		if _, ok, err := watch.Parse(line); ok || err != nil {
			t.Errorf("%q is read as an event", line)
		}
	}
	for _, line := range []string{"build", "build finished", `{"node":`, `{"nod":"x"}`} {
		if _, _, err := watch.Parse(line); err == nil {
			t.Errorf("%q is taken for an event", line)
		}
	}
}

func TestEventsAddUpToAState(t *testing.T) {
	d := flow()
	var st cligram.State
	for _, line := range []string{"build done", "build -> test", "test failed", "test -> fix: no", "build -> test", "fix active"} {
		ev, _, _ := watch.Parse(line)
		if err := watch.Apply(d, &st, ev); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
	}
	want := cligram.State{
		Status: map[string]cligram.Status{"build": cligram.Done, "test": cligram.Failed, "fix": cligram.Active},
		Taken:  []cligram.EdgeRef{{From: "build", To: "test"}, {From: "test", To: "fix", Label: "no"}},
	}
	if !reflect.DeepEqual(st, want) {
		t.Errorf("got %+v\nwant %+v", st, want)
	}
	ev, _, _ := watch.Parse("reset")
	if err := watch.Apply(d, &st, ev); err != nil || len(st.Status) != 0 || len(st.Taken) != 0 {
		t.Errorf("reset left %+v, %v", st, err)
	}
}

func TestAnEventForSomethingElseIsSaidAndLeftOut(t *testing.T) {
	d := flow()
	for line, says := range map[string]string{
		"deploy done":                      `"deploy" is not a node`,
		"build -> fix":                     "build -> fix is not an edge",
		"test -> fix":                      "is 2 edges: say which by its label",
		`{"node":"build","status":"gone"}`: `status "gone" of build`,
		`{}`:                               "an event sets",
	} {
		ev, _, err := watch.Parse(line)
		if err == nil {
			var st cligram.State
			err = watch.Apply(d, &st, ev)
			if len(st.Status) != 0 || len(st.Taken) != 0 {
				t.Errorf("%q changed the state: %+v", line, st)
			}
		}
		if err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("%q: %v, want %q", line, err, says)
		}
	}
}
