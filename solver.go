package cligram

// Positions along one axis are solved as a system of difference
// constraints, pos[to] - pos[from] >= w: every gap is a minimum, so the
// smallest positions that satisfy them all put everything as close
// together as the constraints allow. That is a longest path through the
// constraints, from a floor of 0. A loop of constraints that each demand
// more room than the last cannot hold, and the least important statement
// in it is dropped until the rest can.

// prio is how much a constraint matters: the lowest in a conflict goes.
type prio int

const (
	prioCenter  prio = iota // a lone direction centering on its target
	prioAuto                // placed by the automatic layout
	prioOverlap             // keeping two nodes apart
	prioUser                // what the author wrote
)

// src as a constraint's from makes its w a lower bound on to.
const src = -1

type cons struct {
	from, to int
	w        int
	// group is the statement the constraint comes from; a statement's
	// constraints are dropped together.
	group int
	prio  prio
}

type system struct {
	n       int
	cons    []cons
	dropped map[int]bool
}

func newSystem(n int) *system {
	return &system{n: n, dropped: map[int]bool{}}
}

// floor adds pos[to] - pos[from] >= w, and returns its index.
func (s *system) floor(from, to, w, group int, p prio) int {
	s.cons = append(s.cons, cons{from: from, to: to, w: w, group: group, prio: p})
	return len(s.cons) - 1
}

// equal adds pos[to] - pos[from] == d.
func (s *system) equal(from, to, d, group int, p prio) {
	s.floor(from, to, d, group, p)
	s.floor(to, from, -d, group, p)
}

// solve finds the smallest positions that satisfy every constraint not
// dropped, dropping statements until there is no conflict. It returns the
// groups it dropped this time.
func (s *system) solve() (pos []int, dropped []int) {
	for {
		pos, cycle := s.longestPaths()
		if cycle == nil {
			return pos, dropped
		}
		worst := cycle[0]
		for _, ci := range cycle[1:] {
			c, w := s.cons[ci], s.cons[worst]
			// The least important goes; of equals, the latest written.
			if c.prio < w.prio || (c.prio == w.prio && c.group > w.group) {
				worst = ci
			}
		}
		g := s.cons[worst].group
		s.dropped[g] = true
		dropped = append(dropped, g)
	}
}

// longestPaths runs Bellman-Ford for longest paths from a floor of 0. It
// returns the constraints of a loop that cannot hold, if there is one.
func (s *system) longestPaths() ([]int, []int) {
	pos := make([]int, s.n)
	pred := make([]int, s.n)
	for i := range pred {
		pred[i] = -1
	}
	for ci, c := range s.cons {
		if c.from == src && !s.dropped[c.group] && c.w > pos[c.to] {
			pos[c.to], pred[c.to] = c.w, ci
		}
	}
	last := -1
	for range s.n + 1 {
		last = -1
		for ci, c := range s.cons {
			if c.from == src || s.dropped[c.group] {
				continue
			}
			if v := pos[c.from] + c.w; v > pos[c.to] {
				pos[c.to], pred[c.to] = v, ci
				last = c.to
			}
		}
		if last == -1 {
			return pos, nil
		}
	}
	// Still relaxing after n rounds: walk back far enough to be on the loop,
	// then go round it once.
	v := last
	for range s.n {
		c := s.cons[pred[v]]
		if c.from == src {
			return pos, nil
		}
		v = c.from
	}
	var cycle []int
	for u := v; ; {
		ci := pred[u]
		cycle = append(cycle, ci)
		u = s.cons[ci].from
		if u == v || u == src {
			break
		}
	}
	return pos, cycle
}
