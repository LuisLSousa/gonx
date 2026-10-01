package gonx

import (
	"fmt"
	"reflect"
	"slices"
)

// Restricted is a read-only view of a graph with some nodes and edges hidden,
// made by [RestrictedView]. It implements [Adjacency], so the traversals that
// take one, such as metrics.BreadthFirst, metrics.Dijkstra and
// metrics.ShortestPath, run on it unchanged and see only what is left.
//
// A view shares the graph's storage and copies only the out-lists that lose an
// entry. Every other node's OutNeighbors and OutWeights are the graph's own
// slices, returned as they are. Telling the two apart takes one bit test for
// almost every node, and O(log t) when the test is inconclusive, where t is
// the number of nodes whose lists changed. A view is immutable and, like the
// graph, safe to read from several goroutines at once. The zero value is not a
// valid view.
type Restricted struct {
	g        restrictable
	negative bool

	// g's out-lists in CSR form, so that an untouched node's slices come
	// straight from g's storage without a second dynamic call.
	baseOffsets []int32
	baseNbrs    []int32
	baseWs      []float64

	// The nodes whose out-lists differ from g's, ascending, and their lists
	// in CSR form: touched[i]'s out-list is nbrs[offsets[i]:offsets[i+1]],
	// with its weights at the same positions of ws (nil when unweighted).
	touched []int32
	offsets []int32
	nbrs    []int32
	ws      []float64

	// A bit per touched node, at a hash of its ID: a clear bit proves a node
	// untouched in one test, which keeps find's search off the path of almost
	// every node. It has at least 16 bits per touched node, so about one
	// untouched node in 16 needs the search, however many lists changed. Up
	// to 64 bits it is the word small, the common case of a few hidden edges;
	// beyond, it is large, and small has every bit set so that find passes
	// every node on to lookup.
	small uint64
	large []uint64
	shift uint // a hash's top 64-shift bits pick its filter bit

	// What was asked to be hidden, so that a view of this view can be built
	// directly over g instead of stacking one lookup on another.
	hiddenNodes []int
	hiddenEdges [][2]int
}

// restrictable is what RestrictedView needs from the graph under a view,
// beyond Adjacency: whether each edge is one-way, which nodes have an edge
// into a given node, so that hiding the node can take it out of their lists,
// the out-lists' storage, and how many out-list entries weigh less than zero,
// so that the view's HasNegativeWeight follows from the entries it drops.
// Graph and Digraph implement it.
type restrictable interface {
	Adjacency
	directed() bool
	inNeighbors(u int) []int32
	outCSR() (offsets, nbrs []int32, ws []float64)
	negativeEntries() int
}

func (g *Graph) negativeEntries() int   { return g.negatives }
func (g *Digraph) negativeEntries() int { return g.negatives }

func (*Graph) directed() bool                          { return false }
func (g *Graph) inNeighbors(u int) []int32             { return g.Neighbors(u) }
func (g *Graph) outCSR() ([]int32, []int32, []float64) { return g.offsets, g.data, g.weights }
func (*Digraph) directed() bool                        { return true }
func (g *Digraph) inNeighbors(u int) []int32           { return g.InNeighbors(u) }
func (g *Digraph) outCSR() ([]int32, []int32, []float64) {
	return g.outOffsets, g.outData, g.outWeights
}

// RestrictedView returns a view of g with the given nodes and edges hidden,
// after networkx's restricted_view. Neither g nor the arguments are modified,
// and the view does not hold on to the argument slices.
//
// Node IDs do not change. A hidden node stays in the view, since Adjacency
// numbers nodes densely from 0, but it has no edges in either direction: no
// traversal can enter it, and one that starts there goes nowhere. networkx
// drops the node instead; distances between the other nodes come out the same.
//
// On an undirected Graph an edge is hidden as a whole, so {u, v} and {v, u}
// name the same edge and both of its directions disappear. On a Digraph, {u, v}
// hides only the edge from u to v. Edges between nodes in range that g does
// not have are ignored, as networkx ignores them; a node out of range, in
// either argument, panics.
//
// The view keeps the weights of the edges it keeps. Weighted is the same as
// g's. HasNegativeWeight counts only the edges left, so hiding a graph's only
// negative edge makes the view usable for Dijkstra.
//
// When g is itself a Restricted view, the result hides the union of both sets
// over the same underlying graph. When g is a wrapper, a type that embeds a
// Graph, a Digraph, a view or an Adjacency holding one of them, the view is of
// the graph or view it wraps, and methods the wrapper overrides are not
// consulted: the view reads one graph throughout, rather than the wrapper's
// answers for some nodes and the storage beneath them for others.
//
// Building a view takes O(k log k + d) time, where k is the number of edges
// hidden, counting those taken out with hidden nodes, and d is the combined
// degree of the nodes whose lists change. A view of a view is built afresh
// from the union, so hiding edges one at a time through a chain of views costs
// O(k²) over k steps; pass them to one call instead.
func RestrictedView(g Adjacency, nodes []int, edges [][2]int) *Restricted {
	var base restrictable
	switch b := underlying(g).(type) {
	case *Restricted:
		base = b.g
		nodes = append(slices.Clip(b.hiddenNodes), nodes...)
		edges = append(slices.Clip(b.hiddenEdges), edges...)
	case restrictable:
		base = b
	default:
		// Unreachable: adjacency is defined on the three types above alone,
		// and returns its receiver.
		panic(fmt.Sprintf("gonx: RestrictedView: unsupported Adjacency %T", g))
	}
	n := base.NumNodes()
	for _, x := range nodes {
		if x < 0 || x >= n {
			panicNode("RestrictedView", x, n)
		}
	}
	for _, e := range edges {
		for _, x := range e {
			if x < 0 || x >= n {
				panicNode("RestrictedView", x, n)
			}
		}
	}
	r := &Restricted{g: base, hiddenNodes: slices.Clone(nodes), hiddenEdges: slices.Clone(edges)}
	r.baseOffsets, r.baseNbrs, r.baseWs = base.outCSR()

	// Every hidden direction as a (from, to) key, so that sorting groups them
	// by the list they come out of and orders each group like that list.
	// Hiding a node hides its in-edges here; its own list is emptied below.
	key := func(u, v int) uint64 { return uint64(u)<<32 | uint64(v) }
	var cut []uint64
	for _, e := range edges {
		cut = append(cut, key(e[0], e[1]))
		if !base.directed() {
			cut = append(cut, key(e[1], e[0]))
		}
	}
	hidden := slices.Clone(nodes)
	slices.Sort(hidden)
	hidden = slices.Compact(hidden)
	for _, x := range hidden {
		for _, y := range base.inNeighbors(x) {
			cut = append(cut, key(int(y), x))
		}
	}
	slices.Sort(cut)
	cut = slices.Compact(cut)

	weighted := base.Weighted()
	// The negative entries among those the view drops, counted only when g
	// has any: whatever g has beyond them is still visible.
	countNegative := base.negativeEntries() > 0
	droppedNegative := 0
	r.offsets = []int32{0}
	if weighted {
		// Non-nil even if every changed list ends up empty, since a nil
		// OutWeights means an unweighted graph.
		r.ws = []float64{}
	}
	// touch records that u's list is the one just appended to r.nbrs.
	touch := func(u int) {
		r.touched = append(r.touched, int32(u))
		r.offsets = append(r.offsets, int32(len(r.nbrs)))
	}
	keep := func(u int, drop []uint64) {
		out := base.OutNeighbors(u)
		var ws []float64
		if weighted {
			ws = base.OutWeights(u)
		}
		start := len(r.nbrs)
		for i, v := range out {
			// Both lists ascend, so one pass over each finds the matches.
			for len(drop) > 0 && drop[0] < key(u, int(v)) {
				drop = drop[1:]
			}
			if len(drop) > 0 && drop[0] == key(u, int(v)) {
				if countNegative && ws[i] < 0 {
					droppedNegative++
				}
				continue
			}
			r.nbrs = append(r.nbrs, v)
			if weighted {
				r.ws = append(r.ws, ws[i])
			}
		}
		if len(r.nbrs)-start == len(out) {
			// Nothing hidden was there, so g's own list stands.
			r.nbrs = r.nbrs[:start]
			if weighted {
				r.ws = r.ws[:start]
			}
			return
		}
		touch(u)
	}
	for len(cut) > 0 || len(hidden) > 0 {
		// The next list to change, in node order, from either source.
		u := -1
		if len(cut) > 0 {
			u = int(cut[0] >> 32)
		}
		if len(hidden) > 0 && (u < 0 || hidden[0] <= u) {
			u = hidden[0]
		}
		j := 0
		for j < len(cut) && int(cut[j]>>32) == u {
			j++
		}
		if len(hidden) > 0 && hidden[0] == u {
			hidden = hidden[1:]
			if len(base.OutNeighbors(u)) > 0 {
				touch(u) // with nothing appended, an empty list
			}
			if countNegative {
				for _, w := range base.OutWeights(u) {
					if w < 0 {
						droppedNegative++
					}
				}
			}
		} else {
			keep(u, cut[:j])
		}
		cut = cut[j:]
	}

	bits := uint(6) // log2 of the filter's size, one word at least
	for 1<<bits < 16*len(r.touched) {
		bits++
	}
	r.shift = 64 - bits
	if bits > 6 {
		r.large = make([]uint64, 1<<(bits-6))
		r.small = ^uint64(0)
	}
	for _, u := range r.touched {
		h := r.hash(int(u))
		if r.large == nil {
			r.small |= 1 << h
		} else {
			r.large[h>>6] |= 1 << (h & 63)
		}
	}

	r.negative = base.negativeEntries() > droppedNegative
	return r
}

// underlying returns the Graph, Digraph or Restricted that g is or wraps. When
// there is none, because g is nil, a nil pointer, or a wrapper holding either,
// it panics with a message naming RestrictedView and the types involved;
// otherwise the first method call on the missing graph would fail with a bare
// nil dereference.
func underlying(g Adjacency) Adjacency {
	if g == nil {
		panic("gonx: RestrictedView: nil Adjacency")
	}
	// A nil pointer, to a graph, a view or a wrapper, is caught before any
	// method runs on it. That names it exactly, and avoids a fault inside a
	// promoted method, which Go 1.27's race detector turns from a panic into
	// a fatal error when it unwinds through a deferred call.
	if v := reflect.ValueOf(g); v.Kind() == reflect.Pointer && v.IsNil() {
		panic(fmt.Sprintf("gonx: RestrictedView: nil %T", g))
	}
	a := adjacencyOf(g)
	missing := false
	switch b := a.(type) {
	case *Graph:
		missing = b == nil
	case *Digraph:
		missing = b == nil
	case *Restricted:
		missing = b == nil
	}
	if missing {
		panic(fmt.Sprintf("gonx: RestrictedView: %T wraps a nil %T", g, a))
	}
	return a
}

// adjacencyOf calls g.adjacency. On a wrapper holding a nil Adjacency, or a
// nil pointer to another wrapper, that call panics inside the promoted method,
// before there is a value to check, so the panic is replaced with one that
// names the wrapper; the original stays in the trace, marked recovered.
func adjacencyOf(g Adjacency) Adjacency {
	defer func() {
		if recover() != nil {
			panic(fmt.Sprintf("gonx: RestrictedView: %T wraps a nil Adjacency or a nil pointer", g))
		}
	}()
	return g.adjacency()
}

// hash maps a node to a filter bit. The multiplier (2^64 over the golden
// ratio) spreads nearby IDs, which are often touched together, across the
// filter.
func (r *Restricted) hash(u int) uint64 { return uint64(u) * golden >> r.shift }

const golden = 0x9e3779b97f4a7c15

// find returns the index of u in r.touched, or -1 when u's list is g's own.
// It is kept small enough to inline into OutNeighbors and OutWeights, since
// it runs on every call: a small view's one-word filter is tested here, which
// rules out nearly every node, and everything else is left to lookup.
func (r *Restricted) find(u int) int {
	// The same hash as r.hash, with the shift a constant.
	if r.small&(1<<(uint64(u)*golden>>58)) == 0 {
		return -1
	}
	return r.lookup(u)
}

// lookup is find's slow path: a large view's filter, then the search.
func (r *Restricted) lookup(u int) int {
	if h := r.hash(u); r.large != nil && r.large[h>>6]&(1<<(h&63)) == 0 {
		return -1
	}
	return r.search(u)
}

// search finds u in r.touched by binary search, or returns -1.
func (r *Restricted) search(u int) int {
	lo, hi := 0, len(r.touched)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if int(r.touched[mid]) < u {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(r.touched) && int(r.touched[lo]) == u {
		return lo
	}
	return -1
}

// NumNodes reports the number of nodes, hidden ones included.
func (r *Restricted) NumNodes() int { return r.g.NumNodes() }

// OutNeighbors returns the targets of u's visible outgoing edges, ascending:
// the underlying graph's own slice when the view changes nothing at u, and
// otherwise a filtered copy the view holds. It is empty for a hidden node.
// Callers MUST NOT modify the returned slice. It panics if u is out of range.
func (r *Restricted) OutNeighbors(u int) []int32 {
	if u < 0 || u >= len(r.baseOffsets)-1 {
		panicNode("OutNeighbors", u, len(r.baseOffsets)-1)
	}
	if i := r.find(u); i >= 0 {
		lo, hi := r.offsets[i], r.offsets[i+1]
		return r.nbrs[lo:hi:hi]
	}
	lo, hi := r.baseOffsets[u], r.baseOffsets[u+1]
	return r.baseNbrs[lo:hi:hi]
}

// OutWeights returns the weights of u's visible outgoing edges, aligned with
// [Restricted.OutNeighbors], or nil when the graph is unweighted. Callers MUST
// NOT modify the returned slice. It panics if u is out of range.
func (r *Restricted) OutWeights(u int) []float64 {
	if u < 0 || u >= len(r.baseOffsets)-1 {
		panicNode("OutWeights", u, len(r.baseOffsets)-1)
	}
	if r.baseWs == nil {
		return nil
	}
	if i := r.find(u); i >= 0 {
		lo, hi := r.offsets[i], r.offsets[i+1]
		return r.ws[lo:hi:hi]
	}
	lo, hi := r.baseOffsets[u], r.baseOffsets[u+1]
	return r.baseWs[lo:hi:hi]
}

// Weighted reports whether the underlying graph carries edge weights.
func (r *Restricted) Weighted() bool { return r.g.Weighted() }

// HasNegativeWeight reports whether any edge left visible weighs less than
// zero. RestrictedView records the answer, so the call is O(1).
func (r *Restricted) HasNegativeWeight() bool { return r.negative }
