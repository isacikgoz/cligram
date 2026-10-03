// Fuzzview draws random diagrams one after another in your terminal, reads
// each drawing back and says whether it says what its diagram says. A
// throwaway look at what the fuzz harness checks.
//
//	go run ./fuzzview                 # 500 cases, a second each
//	go run ./fuzzview -n 1000 -delay 300ms -seed 2000
//	go run ./fuzzview -only-failures  # show only cases that fail
//
// Ctrl+C stops and prints the summary.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/internal/reader"
)

var (
	words = []string{"triage", "review", "the", "spec", "ship", "agent", "runs", "human", "input", "verify",
		"deploy", "monitor", "issue", "create", "a", "very", "long", "step", "summary", "審查", "検証", "🚀",
		"supercalifragilisticexpialidocious", "x"}
	labelWords = []string{"yes", "no", "approved", "needs", "specs", "retry", "timeout", "answer >= 0.7",
		"else", "done", "failed", "declined", "replied", "ok", "審查", "a rather long outcome name"}
	relations = []string{"right of", "left of", "above", "below", "above-left of", "below-right of",
		"level with", "top level with", "left level with"}
	gaps = []string{"", "", "", " (gap: tight)", " (gap: wide)", " (gap: 2)"}
)

func pick[T any](rng *rand.Rand, s []T) T { return s[rng.IntN(len(s))] }

// diagram is a random diagram from seed, its edges as written (once
// each), and a random run state; as the harness makes them.
func diagram(seed uint64) (*cligram.Diagram, []cligram.Edge, cligram.State, string) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	d := cligram.New()
	n := 1 + rng.IntN(14)
	ids := make([]string, n)
	for i := range n {
		ids[i] = fmt.Sprintf("n%d", i)
	}
	// Groups in about half, some inside others; and lines dashed or thick.
	extra := rand.New(rand.NewPCG(seed, 0x6a0))
	var groups []string
	if extra.IntN(2) == 0 {
		for k := range 1 + extra.IntN(3) {
			id, parent := fmt.Sprintf("g%d", k), ""
			if k > 0 && extra.IntN(2) == 0 {
				parent = groups[extra.IntN(k)]
			}
			d.Group(id, pick(extra, []string{"Build", "Review loop", "Release", "a rather long group title here", "CI"}), cligram.Inside(parent))
			groups = append(groups, id)
		}
	}
	for i, id := range ids {
		text := id
		var more []string
		for range rng.IntN(7) {
			more = append(more, pick(rng, words))
		}
		if len(more) > 0 {
			sep := " "
			if rng.IntN(6) == 0 {
				sep = "\n"
			}
			text += sep + strings.Join(more, " ")
		}
		var opts []cligram.NodeOption
		switch rng.IntN(5) {
		case 0:
			opts = append(opts, cligram.As(cligram.Decision))
		case 1:
			opts = append(opts, cligram.As(cligram.Terminal))
		}
		if rng.IntN(3) == 0 {
			opts = append(opts, cligram.Class(pick(rng, []string{"human", "agent"})))
		}
		if rng.IntN(8) == 0 {
			opts = append(opts, cligram.Sub(func(context.Context) (*cligram.Diagram, error) { return cligram.New(), nil }))
		}
		if i > 0 && rng.IntN(4) == 0 {
			opts = append(opts, cligram.At(pick(rng, relations)+" "+ids[rng.IntN(i)]+pick(rng, gaps)))
		}
		if len(groups) > 0 && extra.IntN(3) > 0 {
			opts = append(opts, cligram.In(pick(extra, groups)))
		}
		d.Node(id, text, opts...)
	}
	seen := map[cligram.EdgeRef]bool{}
	var edges []cligram.Edge
	edge := func(from, to string) {
		label := ""
		if rng.IntN(2) == 0 {
			label = pick(rng, labelWords)
		}
		e := cligram.Edge{From: from, To: to, Label: label}
		if seen[e.Ref()] {
			return
		}
		e.Line = pick(extra, []cligram.LineStyle{cligram.Solid, cligram.Solid, cligram.Dashed, cligram.Thick})
		d.Edge(from, to, cligram.Label(label), cligram.Line(e.Line))
		if !seen[e.Ref()] {
			seen[e.Ref()] = true
			edges = append(edges, e)
		}
	}
	for i := 1; i < n; i++ {
		edge(ids[rng.IntN(i)], ids[i])
	}
	for range rng.IntN(n + 1) {
		edge(pick(rng, ids), pick(rng, ids))
	}
	orient := "across"
	if rng.IntN(2) == 0 {
		orient = "down"
	}
	st := cligram.State{Status: map[string]cligram.Status{}}
	statuses := []cligram.Status{cligram.Idle, cligram.Active, cligram.Done, cligram.Failed, cligram.Waiting}
	for _, id := range ids {
		st.Status[id] = pick(rng, statuses)
	}
	for _, e := range edges {
		if rng.IntN(3) == 0 {
			st.Taken = append(st.Taken, e.Ref())
		}
	}
	return d, edges, st, orient
}

// check reads the drawing back and lists where it does not say what the
// diagram says.
func check(text string, d *cligram.Diagram, edges []cligram.Edge, warnings []string) []string {
	pic, err := reader.Read(text)
	if err != nil {
		return strings.Split(err.Error(), "\n")
	}
	var problems []string
	byBox := map[int]string{}
	drawn := map[string]int{}
	for i, b := range pic.Boxes {
		id := ""
		if len(b.Lines) > 0 {
			if f := strings.Fields(b.Lines[0]); len(f) > 0 {
				id = strings.TrimSuffix(f[0], "…")
			}
		}
		byBox[i] = id
		drawn[id]++
	}
	for _, n := range d.Nodes() {
		switch drawn[n.ID] {
		case 0:
			problems = append(problems, "node "+n.ID+" is not drawn")
		case 1:
		default:
			problems = append(problems, fmt.Sprintf("node %s is drawn %d times", n.ID, drawn[n.ID]))
		}
	}
	type key struct{ from, to string }
	want, got := map[key][]string{}, map[key][]string{}
	for _, e := range edges {
		want[key{e.From, e.To}] = append(want[key{e.From, e.To}], e.Label)
	}
	for _, e := range pic.Edges {
		k := key{byBox[e.From], byBox[e.To]}
		got[k] = append(got[k], e.Label)
	}
	for k, labels := range want {
		if len(got[k]) != len(labels) {
			problems = append(problems, fmt.Sprintf("%s -> %s is drawn %d times, wanted %d", k.from, k.to, len(got[k]), len(labels)))
			continue
		}
		for _, l := range labels {
			if l == "" || slicesContainsPrefix(got[k], l) {
				continue
			}
			lost := false
			for _, w := range warnings {
				lost = lost || strings.Contains(w, fmt.Sprintf("edge %s -> %s %q has no room", k.from, k.to, l))
			}
			if !lost {
				problems = append(problems, fmt.Sprintf("%s -> %s lost its label %q", k.from, k.to, l))
			}
		}
	}
	for k := range got {
		if len(want[k]) == 0 {
			problems = append(problems, fmt.Sprintf("%s -> %s is drawn but not in the diagram", k.from, k.to))
		}
	}
	// Frames: each group's round exactly its nodes, those inside too.
	parent := map[string]string{}
	title := map[string]string{}
	for _, g := range d.Groups() {
		parent[g.ID], title[g.ID] = g.Parent, g.Title
	}
	in := func(group, of string) bool {
		for g := group; g != ""; g = parent[g] {
			if g == of {
				return true
			}
		}
		return false
	}
	groupOf := map[string]string{}
	for _, n := range d.Nodes() {
		groupOf[n.ID] = n.Group
	}
	depth := func(id string) int {
		n := 0
		for g := parent[id]; g != ""; g = parent[g] {
			n++
		}
		return n
	}
	groupsInner := d.Groups()
	sort.SliceStable(groupsInner, func(a, b int) bool { return depth(groupsInner[a].ID) > depth(groupsInner[b].ID) })
	used := map[int]bool{}
	for _, g := range groupsInner {
		var members []string
		for _, n := range d.Nodes() {
			if in(n.Group, g.ID) {
				members = append(members, n.ID)
			}
		}
		if len(members) == 0 {
			continue
		}
		// The smallest frame not taken yet, titled as the group is, round
		// all its nodes: groups may share a title.
		var frame *reader.Frame
		at := -1
		for i, f := range pic.Frames {
			if used[i] || !strings.HasPrefix(title[g.ID], strings.TrimSuffix(f.Title, "…")) {
				continue
			}
			all := true
			for _, m := range members {
				found := false
				for bi, b := range pic.Boxes {
					found = found || (byBox[bi] == m && f.Contains(b.X, b.Y))
				}
				all = all && found
			}
			if all && (frame == nil || f.W*f.H < frame.W*frame.H) {
				frame, at = &pic.Frames[i], i
			}
		}
		if frame != nil {
			used[at] = true
		}
		if frame == nil {
			problems = append(problems, fmt.Sprintf("group %s %q has no frame round %v", g.ID, title[g.ID], members))
			continue
		}
		for bi, b := range pic.Boxes {
			inside := frame.Contains(b.X, b.Y) && frame.Contains(b.X+b.W-1, b.Y+b.H-1)
			member := in(groupOf[byBox[bi]], g.ID)
			if member && !inside {
				problems = append(problems, fmt.Sprintf("%s is in group %s but out of its frame", byBox[bi], g.ID))
			}
			// Placements may leave no way apart: the layout says so.
			said := false
			for _, w := range warnings {
				said = said || strings.Contains(w, fmt.Sprintf("node %q and group %q overlap", byBox[bi], g.ID))
			}
			if !member && !said && (frame.Contains(b.X, b.Y) || frame.Contains(b.X+b.W-1, b.Y+b.H-1)) {
				problems = append(problems, fmt.Sprintf("%s is in group %s's frame but not in the group", byBox[bi], g.ID))
			}
		}
	}
	return problems
}

// slicesContainsPrefix reports whether some read label is label, or label
// cut short with an ellipsis.
func slicesContainsPrefix(read []string, label string) bool {
	for _, r := range read {
		if r == label {
			return true
		}
		if cut, ok := strings.CutSuffix(r, "…"); ok && strings.HasPrefix(label, strings.TrimRight(cut, " ")) {
			return true
		}
	}
	return false
}

func main() {
	n := flag.Int("n", 500, "how many cases")
	delay := flag.Duration("delay", time.Second, "how long each case shows")
	start := flag.Uint64("seed", 0, "the first seed")
	onlyFailures := flag.Bool("only-failures", false, "show only cases that fail")
	flag.Parse()

	w, h := 120, 40
	if tw, th, err := term.GetSize(os.Stdout.Fd()); err == nil {
		w, h = tw, th
	}
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)

	type result struct {
		seed     uint64
		problems []string
	}
	var failures []result
	var lost, unfit, done int
	var slowest time.Duration
	var slowSeed uint64
	defer func() {
		fmt.Print("\x1b[?25h") // the cursor back
	}()
	fmt.Print("\x1b[?25l")

loop:
	for i := range *n {
		seed := *start + uint64(i)
		d, edges, st, orient := diagram(seed)
		opts := []cligram.LayoutOption{cligram.Fit(w, h-3)}
		if orient == "down" {
			opts = append(opts, cligram.WithOrientation(cligram.TopToBottom))
		}
		began := time.Now()
		l := d.Layout(opts...)
		took := time.Since(began)
		if took > slowest {
			slowest, slowSeed = took, seed
		}
		var warnings []string
		lostHere := 0
		for _, w := range l.Warnings() {
			warnings = append(warnings, w.Error())
			if strings.Contains(w.Error(), "no room for its label") {
				lostHere++
			}
		}
		lost += lostHere
		// Fits is false when a label is lost too; the size is what scrolls.
		tooBig := l.W > w || l.H > h-3
		if tooBig {
			unfit++
		}
		problems := check(l.Render(st, cligram.Plain), d, edges, warnings)
		done++
		if len(problems) > 0 {
			failures = append(failures, result{seed, problems})
		}
		if *onlyFailures && len(problems) == 0 {
			continue
		}

		verdict := "\x1b[32m✓ reads back as its diagram\x1b[0m"
		if len(problems) > 0 {
			verdict = fmt.Sprintf("\x1b[31m✗ %d problems: %s\x1b[0m", len(problems), problems[0])
		}
		fits := fmt.Sprintf("%dx%d, fits", l.W, l.H)
		if tooBig {
			fits = fmt.Sprintf("%dx%d, too big: showing the top left", l.W, l.H)
		}
		if lostHere > 0 {
			fits += fmt.Sprintf(" · %d label(s) found no room", lostHere)
		}
		view := l.RenderView(st, cligram.ANSI, cligram.Rect{W: w, H: h - 3})
		fmt.Print("\x1b[H\x1b[2J")
		// One line, never wrapped: a wrapped line would push the drawing down.
		header := fmt.Sprintf("case %d/%d  seed %d · %d nodes, %d edges · %s · %s · %v",
			i+1, *n, seed, len(d.Nodes()), len(edges), orient, fits, took.Round(time.Millisecond))
		if r := []rune(header); len(r) > w {
			header = string(r[:w-1]) + "…"
		}
		fmt.Printf("\x1b[1m%s\x1b[0m\n", header)
		fmt.Printf("%s   \x1b[90m(%d failed so far)\x1b[0m\n\n", verdict, len(failures))
		fmt.Print(view)

		select {
		case <-interrupted:
			break loop
		case <-time.After(*delay):
		}
	}

	fmt.Print("\x1b[H\x1b[2J")
	fmt.Printf("\x1b[1m%d cases\x1b[0m from seed %d: %d read back as their diagram, %d did not.\n",
		done, *start, done-len(failures), len(failures))
	fmt.Printf("%d were too big for %dx%d and scroll; %d labels found no room; slowest layout %v (seed %d).\n",
		unfit, w, h-3, lost, slowest.Round(time.Millisecond), slowSeed)
	sort.Slice(failures, func(i, j int) bool { return failures[i].seed < failures[j].seed })
	for _, f := range failures {
		fmt.Printf("\n\x1b[31mseed %d\x1b[0m\n", f.seed)
		for _, p := range f.problems[:min(5, len(f.problems))] {
			fmt.Println("  " + p)
		}
	}
	if len(failures) > 0 {
		fmt.Printf("\nSee one again: go run ./fuzzview -seed N -n 1 -delay 1h\n")
	}
}
