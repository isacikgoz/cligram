package sequence

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/internal/crash"
	"github.com/rivo/uniseg"
)

// The harness makes random sequence diagrams, draws each, reads the
// drawing back with the reader, and checks it says what was built: every
// participant, message, note and block, in order, and nothing else.

var (
	names = []string{"Alice", "Bob", "API", "Database", "Web app", "Payments service", "審查", "🚀 Launcher",
		"a participant with a very long name indeed", "x"}
	saying = []string{"GET /orders", "200 OK", "ok", "", "Reserve items", "retry with backoff after a timeout",
		"審查を依頼", "🚀 go", "supercalifragilisticexpialidocious", "a", "check stock and price"}
	blockWords = map[string]string{"loop": "", "alt": "else", "opt": "", "par": "and", "critical": "option", "break": ""}
	blockKinds = []string{"loop", "alt", "opt", "par", "critical", "break"}
)

func pick[T any](rng *rand.Rand, s []T) T { return s[rng.IntN(len(s))] }

// want is what a drawing should be read as.
type want struct {
	heads  []seenHead
	events []seenEvent
	// never are the participants never active.
	never map[int]bool
}

type harnessCase struct {
	d    *Diagram
	want want
	opts []Option
	desc string
}

func generate(seed uint64) harnessCase {
	rng := rand.New(rand.NewPCG(seed, 0x5e9))
	d := New()
	var w want
	n := 1 + rng.IntN(6)
	ids := make([]string, n)
	for i := range n {
		ids[i] = fmt.Sprintf("p%d", i)
		text := pick(rng, names)
		actor := rng.IntN(4) == 0
		var opts []ParticipantOption
		if actor {
			opts = append(opts, Actor())
		}
		if rng.IntN(5) == 0 {
			text = "" // shows its id
		}
		d.Participant(ids[i], text, opts...)
		if text == "" {
			text = ids[i]
		}
		w.heads = append(w.heads, seenHead{text: text, actor: actor})
	}
	depth := make([]int, n)
	everActive := map[int]bool{}
	number, by := 0, 0
	if rng.IntN(4) == 0 {
		number, by = 1+rng.IntN(5), 1+rng.IntN(2)
		d.Autonumber(number, by)
	}
	var open []string
	steps := rng.IntN(22)
	for range steps {
		switch r := rng.IntN(20); {
		case r < 11:
			from, to := rng.IntN(n), rng.IntN(n)
			if rng.IntN(5) == 0 {
				to = from
			}
			text := pick(rng, saying)
			var opts []MessageOption
			ev := seenEvent{kind: "message", from: from, to: to, line: "solid", head: "arrow"}
			switch rng.IntN(4) {
			case 1:
				opts, ev.line = append(opts, Reply()), "dashed"
			case 2:
				opts, ev.line = append(opts, Line(cligram.Thick)), "thick"
			}
			switch rng.IntN(6) {
			case 1:
				opts, ev.head = append(opts, WithHead(Open)), "open"
			case 2:
				opts, ev.head = append(opts, WithHead(Cross)), "cross"
			case 3:
				opts, ev.both, ev.head = append(opts, BothWays()), true, "arrow"
			}
			// The sender is active as it sends if it was before, even when
			// this ends it; the receiver as it is reached, counting this.
			ev.leaveActive = depth[from] > 0
			if depth[from] > 0 && rng.IntN(3) == 0 {
				opts = append(opts, Deactivate())
				depth[from]--
			}
			if rng.IntN(4) == 0 {
				opts = append(opts, Activate())
				depth[to]++
				everActive[to] = true
			}
			ev.backActive = depth[to] > 0
			d.Message(ids[from], ids[to], text, opts...)
			if by != 0 {
				text = strings.TrimSpace(fmt.Sprintf("%d. %s", number, text))
				number += by
			}
			ev.text = text
			w.events = append(w.events, ev)
		case r < 14:
			text := pick(rng, saying)
			if text == "" {
				text = "note"
			}
			a := rng.IntN(n)
			ev := seenEvent{kind: "note", text: text, from: a, to: a}
			switch rng.IntN(4) {
			case 0:
				d.Note(text, RightOf(ids[a]))
				ev.side = "right"
			case 1:
				d.Note(text, LeftOf(ids[a]))
				ev.side = "left"
			case 2:
				d.Note(text, Over(ids[a]))
				ev.side = "over"
			default:
				b := rng.IntN(n)
				d.Note(text, Over(ids[a], ids[b]))
				ev.side, ev.from, ev.to = "over", min(a, b), max(a, b)
			}
			w.events = append(w.events, ev)
		case r < 16 && len(open) < 3:
			kind := pick(rng, blockKinds)
			text := pick(rng, saying)
			d.Block(kind, text)
			open = append(open, kind)
			w.events = append(w.events, seenEvent{kind: "open", text: strings.TrimSpace(kind + " " + text)})
		case r < 17 && len(open) > 0 && blockWords[open[len(open)-1]] != "":
			word := blockWords[open[len(open)-1]]
			text := pick(rng, saying)
			d.Section(word, text)
			w.events = append(w.events, seenEvent{kind: "section", text: strings.TrimSpace(word + " " + text)})
		case r < 18 && len(open) > 0:
			d.End()
			open = open[:len(open)-1]
			w.events = append(w.events, seenEvent{kind: "close"})
		case r < 19:
			p := rng.IntN(n)
			d.Activate(ids[p])
			depth[p]++
			everActive[p] = true
		default:
			p := rng.IntN(n)
			if depth[p] > 0 {
				d.Deactivate(ids[p])
				depth[p]--
			}
		}
	}
	// Blocks left open are a mistake, drawn closed; most are closed.
	for len(open) > 0 {
		if rng.IntN(5) > 0 {
			d.End()
		}
		open = open[:len(open)-1]
		w.events = append(w.events, seenEvent{kind: "close"})
	}
	w.never = map[int]bool{}
	for i := range n {
		w.never[i] = !everActive[i]
	}
	c := harnessCase{d: d, want: w}
	if rng.IntN(2) == 0 {
		lim := Limits{Width: 6 + rng.IntN(40), Lines: 1 + rng.IntN(5)}
		c.opts = append(c.opts, WithLimits(lim))
		c.desc += fmt.Sprintf(" limits %+v", lim)
	}
	if rng.IntN(3) > 0 {
		fw, fh := 20+rng.IntN(180), 10+rng.IntN(60)
		c.opts = append(c.opts, Fit(fw, fh))
		c.desc += fmt.Sprintf(" fit %dx%d", fw, fh)
	}
	return c
}

// sameText reports whether text read from a drawing says want: the same
// words, wrapped anywhere, or their start cut with an ellipsis.
func sameText(read, want string) bool {
	squash := func(s string) string { return strings.Join(strings.Fields(s), "") }
	r, w := squash(read), squash(want)
	if cut, ok := strings.CutSuffix(r, "…"); ok {
		return strings.HasPrefix(w, cut)
	}
	return r == w
}

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func checkCase(t *testing.T, c harnessCase) {
	t.Helper()
	l := c.d.Layout(c.opts...)
	plain := l.Render(cligram.Plain)
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("%s\n%s\n%s", c.desc, fmt.Sprintf(format, args...), plain)
	}
	if got := sgrRe.ReplaceAllString(l.Render(cligram.ANSI), ""); got != plain {
		fail("in color it says something else:\n%s", got)
	}
	if again := c.d.Layout(c.opts...).Render(cligram.Plain); again != plain {
		fail("laid out again, it is drawn differently:\n%s", again)
	}
	rows := strings.Split(plain, "\n")
	if len(rows) != l.H {
		fail("%d rows drawn, H is %d", len(rows), l.H)
	}
	for y, row := range rows {
		if w := uniseg.StringWidth(row); w > l.W {
			fail("row %d is %d wide, W is %d", y, w, l.W)
		}
	}
	if l.Fits() {
		for _, o := range c.opts {
			var opt options
			o(&opt)
			if opt.fit && l.W > opt.fitW {
				fail("fits, but is %d wide for %d", l.W, opt.fitW)
			}
		}
	}

	s, err := read(plain)
	if err != nil {
		fail("unreadable: %v", err)
	}
	if len(s.heads) != len(c.want.heads) {
		fail("%d participants read, %d drawn", len(s.heads), len(c.want.heads))
	}
	for i, h := range s.heads {
		wh := c.want.heads[i]
		if !sameText(h.text, wh.text) || h.actor != wh.actor {
			fail("participant %d reads %q (actor %v), not %q (actor %v)", i, h.text, h.actor, wh.text, wh.actor)
		}
	}
	if len(s.events) != len(c.want.events) {
		var got []string
		for _, e := range s.events {
			got = append(got, fmt.Sprintf("%s %q", e.kind, e.text))
		}
		fail("%d things read, %d drawn: %s", len(s.events), len(c.want.events), strings.Join(got, ", "))
	}
	var frames []seenEvent
	for k, e := range s.events {
		we := c.want.events[k]
		if e.kind != we.kind || !sameText(e.text, we.text) {
			fail("thing %d reads as %s %q, not %s %q", k, e.kind, e.text, we.kind, we.text)
		}
		switch e.kind {
		case "message":
			from, to, wfrom, wto := e.from, e.to, we.from, we.to
			if e.head == "open" || e.both {
				// A line with no head, or a head at each end, has no way.
				from, to, wfrom, wto = min(from, to), max(from, to), min(wfrom, wto), max(wfrom, wto)
			}
			if from != wfrom || to != wto || e.line != we.line || e.head != we.head || e.both != we.both {
				fail("message %d reads %d->%d %s %s both %v, not %d->%d %s %s both %v", k,
					e.from, e.to, e.line, e.head, e.both, we.from, we.to, we.line, we.head, we.both)
			}
		case "note":
			if e.side != we.side || e.from != we.from || e.to != we.to {
				fail("note %d reads %s %d..%d, not %s %d..%d", k, e.side, e.from, e.to, we.side, we.from, we.to)
			}
		case "open":
			frames = append(frames, e)
		case "close":
			frames = frames[:len(frames)-1]
		}
		// Everything in a frame is inside it.
		if e.kind == "message" || e.kind == "note" {
			for _, f := range frames {
				if e.x0 <= f.x0 || e.x1 >= f.x1 {
					fail("thing %d, columns %d..%d, is not inside its frame, %d..%d", k, e.x0, e.x1, f.x0, f.x1)
				}
			}
		}
	}
	// Every message leaves and reaches its lifelines active or not as the
	// activations so far say. Which way it went is the one written: one
	// with no way reads either way.
	for k, e := range s.events {
		we := c.want.events[k]
		if e.kind != "message" {
			continue
		}
		leave, back := e.leaveActive, e.backActive
		if we.from != we.to {
			leave, back = e.active[we.from], e.active[we.to]
		}
		if leave != we.leaveActive || back != we.backActive {
			fail("message %d leaves %d active %v and reaches %d active %v, not %v and %v",
				k, we.from, leave, we.to, back, we.leaveActive, we.backActive)
		}
	}
	for p, never := range c.want.never {
		if never && len(s.active[p]) > 0 {
			fail("participant %d is never activated, but drawn active in rows %v", p, s.active[p])
		}
	}
}

func TestDrawingsReadBack(t *testing.T) {
	seeds := 600
	if testing.Short() {
		seeds = 100
	}
	for seed := range uint64(seeds) {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			checkCase(t, generate(seed))
		})
	}
}

func FuzzDrawings(f *testing.F) {
	crash.KeepTrace(f)
	for seed := range uint64(8) {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkCase(t, generate(seed))
	})
}
