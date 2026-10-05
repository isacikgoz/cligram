package cligram

import (
	"slices"
	"sort"
	"strings"
)

// A network is a diagram whose edges all have no way: there is no flow to
// follow, so it is laid out by its links. Each connected part spreads out
// from its middle, the node from which every other is fewest links away,
// in layers by how many links away they are, one layer a step along the
// main axis; each layer is ordered so its links cross least, and each
// node put across from the nodes it links to. Parts sit side by side
// across the main axis, in the order they are first written.

// network reports whether every edge has no way, and there is one.
func (l *Layout) network() bool {
	if len(l.edges) == 0 {
		return false
	}
	for _, e := range l.edges {
		if !e.Undirected {
			return false
		}
	}
	return true
}

// networkSweeps is how many times layers are ordered, and placed, against
// the layers beside them.
const networkSweeps = 4

// network places what the author left unplaced, as a network.
func (y *layouter) network() {
	n := len(y.l.nodes)
	links := make([][]int, n)
	for _, e := range y.l.edges {
		a, b := y.l.index[e.From], y.l.index[e.To]
		if a == b || slices.Contains(links[a], b) {
			continue
		}
		links[a] = append(links[a], b)
		links[b] = append(links[b], a)
	}
	for i := range n {
		y.dir[i], y.band[i], y.parent[i] = 1, i, -1
	}

	seen := make([]bool, n)
	offset := 0 // across the main axis, where the next part starts
	for start := range n {
		if seen[start] {
			continue
		}
		part := reachable(links, start)
		for _, i := range part {
			seen[i] = true
		}
		layers := y.layers(links, part)
		y.widen(links, layers)
		cross := y.across(links, layers)
		gaps := make([]int, len(layers))
		for k := 0; k+1 < len(layers); k++ {
			gaps[k] = y.layerGap(links, layers[k], layers[k+1], cross)
		}
		main := y.along(layers, gaps)
		// Each a statement of its own: one that cannot hold, against a
		// frame kept clear or what the author placed, goes alone.
		end := offset
		for k, layer := range layers {
			for _, i := range layer {
				if y.free(i, y.cross) {
					y.sys[y.cross].floor(src, i, offset+cross[i], y.group(), prioAuto)
				}
				if y.free(i, y.main) {
					y.sys[y.main].floor(src, i, main[i], y.group(), prioAuto)
				}
				end = max(end, offset+cross[i]+y.size(i, y.cross))
				y.order = append(y.order, i)
			}
			// A layer keeps its order, and the next keeps below it, when
			// what the author placed moves some of it.
			for j := 1; j < len(layer); j++ {
				a, b := layer[j-1], layer[j]
				y.sys[y.cross].floor(a, b, y.size(a, y.cross)+y.spacing(a, b, y.cross), y.group(), prioAuto)
			}
			if k+1 < len(layers) {
				for _, a := range layer {
					for _, b := range layers[k+1] {
						y.sys[y.main].floor(a, b, y.size(a, y.main)+gaps[k], y.group(), prioAuto)
					}
				}
			}
		}
		offset = end + 2*y.gap(Gap{}, y.cross)
	}
}

// widen makes a node with many links to a layer above or below it wide
// enough for each to have a cell of its own, and one between, on the side
// facing that layer: links share no line, and a side too narrow sends
// them out of other sides and round. Its text stays in the middle. Across
// the flow a box is three rows tall and a side is one cell: its links
// leave by the top and bottom too, which reads as well.
func (y *layouter) widen(links [][]int, layers [][]int) {
	if y.main != axisY {
		return
	}
	layer := map[int]int{}
	for k, nodes := range layers {
		for _, i := range nodes {
			layer[i] = k
		}
	}
	for i := range layer {
		up, down := 0, 0
		for _, j := range links[i] {
			switch layer[j] - layer[i] {
			case -1:
				up++
			case 1:
				down++
			}
		}
		p := &y.l.nodes[i]
		extra := 2*max(up, down) + 3 - p.rect.W
		if extra <= 0 {
			continue
		}
		p.rect.W += extra
		// In the middle of the box: the marker's room on the left counts.
		lead := strings.Repeat(" ", max(extra-2, 0)/2)
		for k := range p.lines {
			p.lines[k] = lead + p.lines[k]
		}
	}
}

// reachable is every node linked to start, near or far, start first.
func reachable(links [][]int, start int) []int {
	out := []int{start}
	seen := map[int]bool{start: true}
	for k := 0; k < len(out); k++ {
		for _, j := range links[out[k]] {
			if !seen[j] {
				seen[j] = true
				out = append(out, j)
			}
		}
	}
	return out
}

// layers are a part's nodes by how many links they are from its middle,
// each layer in the order that crosses its links with the layers beside
// it least.
func (y *layouter) layers(links [][]int, part []int) [][]int {
	// The middle: fewest links to the farthest node, then the most links,
	// then the first written.
	middle, best := part[0], -1
	for _, i := range part {
		far := len(levels(links, i)) - 1
		better := best < 0 || far < best ||
			(far == best && len(links[i]) > len(links[middle])) ||
			(far == best && len(links[i]) == len(links[middle]) && i < middle)
		if better {
			middle, best = i, far
		}
	}
	layers := levels(links, middle)
	at := make(map[int]int, len(part)) // a node's place in its layer
	place := func(layer []int) {
		for k, i := range layer {
			at[i] = k
		}
	}
	// order sorts layer by where the nodes it links to in other sit.
	order := func(layer, other []int) {
		in := map[int]bool{}
		for _, j := range other {
			in[j] = true
		}
		key := map[int]float64{}
		for _, i := range layer {
			sum, count := 0, 0
			for _, j := range links[i] {
				if in[j] {
					sum += at[j]
					count++
				}
			}
			if count > 0 {
				key[i] = float64(sum) / float64(count)
			} else {
				key[i] = float64(at[i])
			}
		}
		sort.SliceStable(layer, func(a, b int) bool { return key[layer[a]] < key[layer[b]] })
		place(layer)
	}
	for _, layer := range layers {
		place(layer)
	}
	for range networkSweeps {
		for k := 1; k < len(layers); k++ {
			order(layers[k], layers[k-1])
		}
		for k := len(layers) - 2; k >= 0; k-- {
			order(layers[k], layers[k+1])
		}
	}
	untangle(links, layers)
	return layers
}

// untangle swaps nodes side by side in a layer while that crosses fewer
// links: links between two layers that cross, and links within a layer
// that pass nodes between their ends, which they must go round.
func untangle(links [][]int, layers [][]int) {
	layerOf, at := map[int]int{}, map[int]int{}
	for k, layer := range layers {
		for p, i := range layer {
			layerOf[i], at[i] = k, p
		}
	}
	type link struct{ a, b int }
	var between, within []link
	for i := range links {
		for _, j := range links[i] {
			if i >= j {
				continue
			}
			switch layerOf[j] - layerOf[i] {
			case 0:
				within = append(within, link{i, j})
			case 1:
				between = append(between, link{i, j})
			case -1:
				between = append(between, link{j, i})
			}
		}
	}
	tangle := func() int {
		n := 0
		for x, l := range between {
			for _, m := range between[x+1:] {
				if layerOf[l.a] == layerOf[m.a] && (at[l.a]-at[m.a])*(at[l.b]-at[m.b]) < 0 {
					n++
				}
			}
		}
		for _, l := range within {
			n += abs(at[l.a]-at[l.b]) - 1
		}
		return n
	}
	best := tangle()
	for range len(at) {
		better := false
		for _, layer := range layers {
			for p := 0; p+1 < len(layer); p++ {
				swap := func() {
					layer[p], layer[p+1] = layer[p+1], layer[p]
					at[layer[p]], at[layer[p+1]] = p, p+1
				}
				swap()
				if t := tangle(); t < best {
					best, better = t, true
				} else {
					swap()
				}
			}
		}
		if !better {
			return
		}
	}
}

// levels are the nodes linked to start, by how many links away they are.
func levels(links [][]int, start int) [][]int {
	seen := map[int]bool{start: true}
	out := [][]int{{start}}
	for {
		var next []int
		for _, i := range out[len(out)-1] {
			for _, j := range links[i] {
				if !seen[j] {
					seen[j] = true
					next = append(next, j)
				}
			}
		}
		if len(next) == 0 {
			return out
		}
		out = append(out, next)
	}
}

// spacing is the room across between two nodes side by side in a layer:
// a gap, and room for the label of a link between them.
func (y *layouter) spacing(a, b int, ax axis) int {
	return max(y.gap(Gap{}, ax), y.labelRoom(a, b, ax))
}

// layerGap is the room along the main axis between two layers: a gap,
// room for the labels of the links between them, and a lane for each
// link that must turn to reach across, where the stretches across of
// several overlap: a line turning in the row beside a box would run along
// its side.
func (y *layouter) layerGap(links [][]int, a, b []int, cross map[int]int) int {
	gap := y.gap(Gap{}, y.main)
	type run struct{ lo, hi int }
	var runs []run
	for _, i := range a {
		for _, j := range b {
			gap = max(gap, y.labelRoom(i, j, y.main))
			if !slices.Contains(links[i], j) {
				continue
			}
			// Straight, if the boxes share a column inside both.
			lo := max(cross[i], cross[j]) + 1
			hi := min(cross[i]+y.size(i, y.cross), cross[j]+y.size(j, y.cross)) - 2
			if lo <= hi {
				continue
			}
			// Short of both middles: a fan to the left and one to the right
			// of a node turn in the same rows.
			mi, mj := cross[i]+y.size(i, y.cross)/2, cross[j]+y.size(j, y.cross)/2
			runs = append(runs, run{min(mi, mj) + 1, max(mi, mj) - 1})
		}
	}
	deepest := 0
	for _, r := range runs {
		depth := 0
		for _, o := range runs {
			if o.lo <= r.hi && r.lo <= o.hi {
				depth++
			}
		}
		deepest = max(deepest, depth)
	}
	if deepest > 0 {
		gap = max(gap, 2*deepest+1)
	}
	return gap
}

// across places each node across the main axis, near the middle of the
// nodes it links to in the layers beside it, its layer in order: placed
// from each side in turn and the two averaged, so it leans neither way.
func (y *layouter) across(links [][]int, layers [][]int) map[int]int {
	pos := map[int]int{}
	for _, layer := range layers {
		x := 0
		for k, i := range layer {
			if k > 0 {
				prev := layer[k-1]
				x += y.size(prev, y.cross) + y.spacing(prev, i, y.cross)
			}
			pos[i] = x
		}
	}
	mid := func(i int) int { return 2*pos[i] + y.size(i, y.cross) } // twice its middle
	settle := func(layer, other []int) {
		want := map[int]int{}
		for _, i := range layer {
			sum, count := 0, 0
			for _, j := range links[i] {
				if slices.Contains(other, j) {
					sum += mid(j)
					count++
				}
			}
			want[i] = pos[i]
			if count > 0 {
				want[i] = (sum/count - y.size(i, y.cross)) / 2
			}
		}
		// From the left: as near as it wants, right of the one before.
		left := map[int]int{}
		for k, i := range layer {
			left[i] = want[i]
			if k > 0 {
				prev := layer[k-1]
				left[i] = max(left[i], left[prev]+y.size(prev, y.cross)+y.spacing(prev, i, y.cross))
			}
		}
		// From the right: as near as it wants, left of the one after.
		right := map[int]int{}
		for k := len(layer) - 1; k >= 0; k-- {
			i := layer[k]
			right[i] = want[i]
			if k < len(layer)-1 {
				next := layer[k+1]
				right[i] = min(right[i], right[next]-y.size(i, y.cross)-y.spacing(i, next, y.cross))
			}
		}
		for _, i := range layer {
			pos[i] = (left[i] + right[i]) / 2
		}
	}
	for range networkSweeps {
		for k := 1; k < len(layers); k++ {
			settle(layers[k], layers[k-1])
		}
		for k := len(layers) - 2; k >= 0; k-- {
			settle(layers[k], layers[k+1])
		}
	}
	lo := 1 << 30
	for _, x := range pos {
		lo = min(lo, x)
	}
	for i := range pos {
		pos[i] -= lo
	}
	return pos
}

// along places each layer along the main axis, gaps[k] after layer k, its
// nodes centered on the layer's middle line.
func (y *layouter) along(layers [][]int, gaps []int) map[int]int {
	pos := map[int]int{}
	at := 0
	for k, layer := range layers {
		size := 0
		for _, i := range layer {
			size = max(size, y.size(i, y.main))
		}
		for _, i := range layer {
			pos[i] = at + (size-y.size(i, y.main))/2
		}
		at += size + gaps[k]
	}
	return pos
}
