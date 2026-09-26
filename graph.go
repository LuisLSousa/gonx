// Package gonx is a performance-oriented graph library for Go, in the spirit of
// Python's networkx but built around dense integer node IDs and a compact,
// cache-friendly representation.
//
// The library separates mutation from reading. A [Builder] accumulates nodes and
// edges, and [Builder.Build] freezes it into an immutable [Graph] stored in
// Compressed Sparse Row (CSR) form. The CSR layout gives zero-copy, O(1)
// neighbor iteration, which is the dominant access pattern for the simulations
// and graph metrics this library targets. [Digraph] and [DigraphBuilder] are the
// directed counterparts; a Digraph stores both edge directions in CSR form, so
// out-neighbors and in-neighbors are equally cheap to walk.
//
// Node IDs are dense integers in the range [0, N). Graphs are always simple (no
// self-loops or duplicate edges). Edges are unweighted unless the builder was
// given weights through [Builder.AddEdgeW]; a weighted graph stores one float64
// per edge in an array laid out exactly like the adjacency, so the weight of
// Neighbors(u)[i] is Weights(u)[i] and an unweighted graph pays nothing for the
// feature. [Forward] is the traversal view shared by Graph and Digraph; it is
// what algorithms that only walk edges in their natural direction take.
//
// All randomized operations take an explicit *math/rand/v2.Rand so results are
// fully reproducible; the package never touches a global RNG.
package gonx

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"math/rand/v2"
	"slices"
)

// Sentinel errors returned by constructors and algorithms. Wrap-friendly: callers
// may test with errors.Is.
var (
	// ErrInvalidParam indicates a generator, transform, or metric was given
	// parameters it cannot work with (e.g. odd degree for Watts-Strogatz).
	ErrInvalidParam = errors.New("gonx: invalid parameter")
	// ErrNotPermutation indicates a relabeling slice is not a permutation of [0, N).
	ErrNotPermutation = errors.New("gonx: not a permutation of node ids")
)

// NewRand returns a deterministic PCG-based RNG seeded from a single value.
// Identical seeds yield identical streams across runs and platforms, which is the
// basis for reproducible graph generation.
func NewRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}

// maxNodes is the largest supported node count: IDs and CSR offsets are stored
// as int32, so both the node count and the total adjacency size (2 * edges)
// must fit in an int32.
const maxNodes = math.MaxInt32

// panicNode reports an out-of-range node ID. Kept out of line so the accessors
// that call it stay small enough to inline.
func panicNode(op string, u, n int) {
	panic(fmt.Sprintf("gonx: %s: node %d out of range [0, %d)", op, u, n))
}

// Builder is a mutable undirected graph used to assemble a topology before
// freezing it into an immutable [Graph]. It is not safe for concurrent use.
//
// Edge methods (AddEdge, RemoveEdge, HasEdge) treat out-of-range endpoints as
// absent edges and report false; Degree panics on an out-of-range node.
//
// A Builder starts unweighted. The first [Builder.AddEdgeW] call switches it to
// weighted for good; see that method for how edges without an explicit weight
// are treated.
type Builder struct {
	adj [][]int32   // adj[u] holds u's neighbors; undirected edges appear in both lists
	w   [][]float64 // w[u] weights a prefix of adj[u] (see AddEdgeW); nil until the first AddEdgeW
	m   int         // number of undirected edges
}

// NewBuilder returns a Builder with n isolated nodes (IDs 0..n-1). It panics if
// n exceeds 2^31-1, the maximum node count supported by the int32 CSR layout.
func NewBuilder(n int) *Builder {
	if n < 0 {
		n = 0
	}
	if n > maxNodes {
		panic(fmt.Sprintf("gonx: NewBuilder: node count %d exceeds max %d", n, maxNodes))
	}
	return &Builder{adj: make([][]int32, n)}
}

// NumNodes reports the number of nodes.
func (b *Builder) NumNodes() int { return len(b.adj) }

// NumEdges reports the number of undirected edges.
func (b *Builder) NumEdges() int { return b.m }

// Weighted reports whether the Builder carries edge weights, which is the case
// from the first [Builder.AddEdgeW] call on.
func (b *Builder) Weighted() bool { return b.w != nil }

// AddNode appends a new isolated node and returns its ID. It panics if the node
// count would exceed 2^31-1.
func (b *Builder) AddNode() int {
	if len(b.adj) >= maxNodes {
		panic("gonx: AddNode: node count would exceed 2^31-1")
	}
	b.adj = append(b.adj, nil)
	if b.w != nil {
		b.w = append(b.w, nil)
	}
	return len(b.adj) - 1
}

// HasEdge reports whether the undirected edge {u, v} exists. O(deg(u)).
func (b *Builder) HasEdge(u, v int) bool {
	if u < 0 || u >= len(b.adj) || v < 0 || v >= len(b.adj) {
		return false
	}
	vv := int32(v)
	for _, w := range b.adj[u] {
		if w == vv {
			return true
		}
	}
	return false
}

// AddEdge inserts the undirected edge {u, v}. It returns false (and does nothing)
// for self-loops, out-of-range endpoints, or edges that already exist, so the
// resulting graph is always simple.
func (b *Builder) AddEdge(u, v int) bool {
	n := len(b.adj)
	if u == v || u < 0 || v < 0 || u >= n || v >= n {
		return false
	}
	if b.HasEdge(u, v) {
		return false
	}
	b.adj[u] = append(b.adj[u], int32(v))
	b.adj[v] = append(b.adj[v], int32(u))
	b.m++
	return true
}

// AddEdgeW inserts the undirected edge {u, v} with weight w. It rejects
// everything AddEdge rejects, plus NaN and infinite weights, returning false in
// each case. Any finite weight is accepted, zero and negative values included;
// algorithms that need non-negative weights say so and check for themselves.
//
// The first AddEdgeW call makes the Builder weighted. Every edge already present
// then has weight 1, and edges added afterwards with AddEdge or AddEdgeUnchecked
// get weight 1 too, so a weighted graph has a weight for every edge. This is the
// networkx convention for a missing weight.
//
// Internally a weight list covers only a prefix of its neighbor list; positions
// past its end hold the implicit weight 1. That is what lets AddEdge and
// AddEdgeUnchecked stay free of weight bookkeeping, and therefore exactly as
// small and as inlinable as they were before weights existed: the unweighted
// path pays nothing for the feature, in code as well as in memory.
func (b *Builder) AddEdgeW(u, v int, w float64) bool {
	if math.IsNaN(w) || math.IsInf(w, 0) {
		return false
	}
	n := len(b.adj)
	if u == v || u < 0 || v < 0 || u >= n || v >= n || b.HasEdge(u, v) {
		return false
	}
	if b.w == nil {
		b.w = make([][]float64, n)
	}
	b.adj[u] = append(b.adj[u], int32(v))
	b.w[u] = appendWeight(b.w[u], len(b.adj[u])-1, w)
	b.adj[v] = append(b.adj[v], int32(u))
	b.w[v] = appendWeight(b.w[v], len(b.adj[v])-1, w)
	b.m++
	return true
}

// appendWeight records w as the weight of the neighbor at position pos,
// padding the list with the implicit weight 1 for any earlier positions it did
// not yet cover.
func appendWeight(ws []float64, pos int, w float64) []float64 {
	for len(ws) < pos {
		ws = append(ws, 1)
	}
	return append(ws, w)
}

// AddEdgeUnchecked inserts the undirected edge {u, v} without checking whether it
// already exists. Endpoints are still validated and self-loops rejected (returning
// false), but inserting an edge that is already present corrupts the Builder: the
// graph silently becomes a multigraph with a double-counted NumEdges. Use it only
// when each pair is known to be produced at most once — e.g. generators that
// enumerate pairs with u < v — where skipping the duplicate scan turns dense
// O(n*m) builds into O(m).
func (b *Builder) AddEdgeUnchecked(u, v int) bool {
	n := len(b.adj)
	if u == v || u < 0 || v < 0 || u >= n || v >= n {
		return false
	}
	b.adj[u] = append(b.adj[u], int32(v))
	b.adj[v] = append(b.adj[v], int32(u))
	b.m++
	return true
}

// RemoveEdge deletes the undirected edge {u, v}, returning whether it existed.
func (b *Builder) RemoveEdge(u, v int) bool {
	if !b.HasEdge(u, v) {
		return false
	}
	i := unlink(&b.adj[u], int32(v))
	j := unlink(&b.adj[v], int32(u))
	if b.w != nil {
		b.w[u] = moveWeight(b.w[u], i, len(b.adj[u]))
		b.w[v] = moveWeight(b.w[v], j, len(b.adj[v]))
	}
	b.m--
	return true
}

// unlink deletes the first occurrence of v from *adj by moving the last entry
// into its slot, and returns the position it freed. Order within a builder
// list is not preserved, which is fine: Build sorts every list. The caller
// guarantees v is present. It is deliberately just the loop, so that it stays
// inlinable; the weight bookkeeping happens in the callers.
func unlink(adj *[]int32, v int32) int {
	a := *adj
	last := len(a) - 1
	for i, x := range a {
		if x == v {
			a[i] = a[last]
			*adj = a[:last]
			return i
		}
	}
	return -1 // not reached: the caller has checked that v is present
}

// moveWeight applies to a prefix weight list the swap-delete that unlink just
// performed on its neighbor list, whose length is now last: the weight at
// position last (implicitly 1 when the list does not reach it) moves to
// position i, and the list shrinks to at most last entries. A position i beyond
// the list already reads as 1 and needs no write, because the only weight that
// could land there is the one past the end, which is 1 as well.
func moveWeight(ws []float64, i, last int) []float64 {
	if i < len(ws) {
		if last < len(ws) {
			ws[i] = ws[last]
		} else {
			ws[i] = 1
		}
	}
	if len(ws) > last {
		ws = ws[:last]
	}
	return ws
}

// denseWeights expands a prefix weight list to the full length of its neighbor
// list, writing into dst and padding the tail with 1.
func denseWeights(ws []float64, dst []float64) []float64 {
	n := copy(dst, ws)
	for i := n; i < len(dst); i++ {
		dst[i] = 1
	}
	return dst
}

// copySorted writes nbrs into dst in ascending order and carries each entry's
// weight along into dstW. Rather than sorting two arrays in lockstep it sorts
// packed (neighbor, position) keys, a plain slices.Sort over uint64s, and then
// gathers. Neighbors within a list are distinct and non-negative, so the high
// word orders the keys by neighbor and the low word says where each weight came
// from. keys is caller-provided scratch of at least len(nbrs).
func copySorted(nbrs []int32, ws []float64, dst []int32, dstW []float64, keys []uint64) {
	keys = keys[:len(nbrs)]
	for i, v := range nbrs {
		keys[i] = uint64(v)<<32 | uint64(i)
	}
	slices.Sort(keys)
	for i, k := range keys {
		dst[i] = int32(k >> 32)
		dstW[i] = ws[uint32(k)]
	}
}

// Degree returns the number of neighbors of u. It panics if u is out of range.
func (b *Builder) Degree(u int) int {
	if u < 0 || u >= len(b.adj) {
		panicNode("Degree", u, len(b.adj))
	}
	return len(b.adj[u])
}

// Build freezes the Builder into an immutable CSR [Graph]. Neighbor lists are
// sorted so the resulting Graph supports binary-search edge tests and has a
// canonical, deterministic layout; on a weighted Builder each weight travels
// with its edge. The Builder may be reused afterwards.
//
// Build panics if the total adjacency size (2 * edges) exceeds 2^31-1, the
// capacity of the int32 CSR offsets.
func (b *Builder) Build() *Graph {
	n := len(b.adj)
	total, maxDeg := 0, 0
	for _, nbrs := range b.adj {
		total += len(nbrs)
		maxDeg = max(maxDeg, len(nbrs))
	}
	if total > maxNodes {
		panic(fmt.Sprintf("gonx: Build: adjacency size %d (2 * edges) exceeds int32 CSR capacity %d", total, maxNodes))
	}

	offsets := make([]int32, n+1)
	var off int32
	for u := 0; u < n; u++ {
		offsets[u] = off
		off += int32(len(b.adj[u]))
	}
	offsets[n] = off

	data := make([]int32, total)
	if b.w == nil {
		for u := 0; u < n; u++ {
			row := data[offsets[u]:offsets[u+1]]
			copy(row, b.adj[u])
			slices.Sort(row)
		}
		return &Graph{offsets: offsets, data: data, m: b.m}
	}

	weights := make([]float64, total)
	keys := make([]uint64, maxDeg)
	row := make([]float64, maxDeg)
	for u := 0; u < n; u++ {
		lo, hi := offsets[u], offsets[u+1]
		ws := denseWeights(b.w[u], row[:len(b.adj[u])])
		copySorted(b.adj[u], ws, data[lo:hi], weights[lo:hi], keys)
	}
	return &Graph{offsets: offsets, data: data, weights: weights, m: b.m}
}

// Graph is an immutable, undirected graph stored in Compressed Sparse Row form.
// The neighbors of node u occupy data[offsets[u]:offsets[u+1]] and are sorted
// ascending; on a weighted graph, weights uses the same indices. A Graph is safe
// for concurrent reads. The zero value is not a valid Graph; obtain one from
// [Builder.Build].
//
// Accessors that take a node ID (Degree, Neighbors, Weights, EdgeOffset,
// NeighborsSeq, RandomNeighbor, and the [Forward] aliases) panic with a
// descriptive message when the ID is outside [0, N); HasEdge and Weight are the
// exceptions and report false for out-of-range endpoints.
type Graph struct {
	offsets []int32   // length n+1
	data    []int32   // length 2*m; concatenated sorted neighbor lists
	weights []float64 // length 2*m, weights[i] belongs to data[i]; nil when unweighted
	m       int
}

// NumNodes reports the number of nodes.
func (g *Graph) NumNodes() int { return len(g.offsets) - 1 }

// NumEdges reports the number of undirected edges.
func (g *Graph) NumEdges() int { return g.m }

// Degree returns the number of neighbors of u. It panics if u is out of range.
func (g *Graph) Degree(u int) int {
	if u < 0 || u >= g.NumNodes() {
		panicNode("Degree", u, g.NumNodes())
	}
	return int(g.offsets[u+1] - g.offsets[u])
}

// Neighbors returns u's neighbor IDs as a sorted, zero-copy slice into the
// graph's backing storage. Callers MUST NOT modify the returned slice. It panics
// if u is out of range.
func (g *Graph) Neighbors(u int) []int32 {
	if u < 0 || u >= g.NumNodes() {
		panicNode("Neighbors", u, g.NumNodes())
	}
	return g.data[g.offsets[u]:g.offsets[u+1]]
}

// NeighborsSeq iterates over u's neighbors in ascending order as ints. It is a
// convenience wrapper over [Graph.Neighbors] for callers who want int node IDs
// end-to-end; hot paths should prefer Neighbors, which exposes the backing slice
// with no per-element call overhead. It panics if u is out of range.
func (g *Graph) NeighborsSeq(u int) iter.Seq[int] {
	return intSeq(g.Neighbors(u))
}

// HasEdge reports whether the undirected edge {u, v} exists. It uses binary
// search over the smaller-degree endpoint, so it runs in O(log deg).
func (g *Graph) HasEdge(u, v int) bool {
	n := g.NumNodes()
	if u < 0 || v < 0 || u >= n || v >= n {
		return false
	}
	if g.Degree(u) > g.Degree(v) {
		u, v = v, u
	}
	nbrs := g.Neighbors(u)
	target := int32(v)
	lo, hi := 0, len(nbrs)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if nbrs[mid] < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(nbrs) && nbrs[lo] == target
}

// Weighted reports whether the graph carries edge weights. An unweighted graph
// stores no weight array at all: Weights returns nil for every node and Weight
// reports 1 for every edge.
func (g *Graph) Weighted() bool { return g.weights != nil }

// Weights returns the weights of u's edges, aligned index for index with
// [Graph.Neighbors]: Weights(u)[i] is the weight of the edge to Neighbors(u)[i].
// The slice is zero-copy and MUST NOT be modified. It is nil when the graph is
// unweighted. It panics if u is out of range.
func (g *Graph) Weights(u int) []float64 {
	if u < 0 || u >= g.NumNodes() {
		panicNode("Weights", u, g.NumNodes())
	}
	if g.weights == nil {
		return nil
	}
	return g.weights[g.offsets[u]:g.offsets[u+1]]
}

// Weight returns the weight of the edge {u, v}. ok is false when the edge does
// not exist or an endpoint is out of range. On an unweighted graph every
// existing edge has weight 1. Like [Graph.HasEdge] it binary-searches the
// smaller-degree endpoint's list, so it runs in O(log deg).
func (g *Graph) Weight(u, v int) (w float64, ok bool) {
	n := g.NumNodes()
	if u < 0 || v < 0 || u >= n || v >= n {
		return 0, false
	}
	if g.Degree(u) > g.Degree(v) {
		u, v = v, u
	}
	i, found := slices.BinarySearch(g.Neighbors(u), int32(v))
	if !found {
		return 0, false
	}
	if g.weights == nil {
		return 1, true
	}
	return g.weights[int(g.offsets[u])+i], true
}

// EdgeOffset returns the index at which u's edges start in the graph's
// edge-indexed arrays: entry i of Neighbors(u), and of Weights(u), sits at
// position EdgeOffset(u)+i in an array of length 2*NumEdges, one slot per
// half-edge. It exists so callers can keep their own per-edge attributes in a
// plain slice laid out exactly like the adjacency, and so metrics that return
// per-edge values can use the same indexing. Because an undirected edge has a
// slot at each end, store a per-edge value in both slots or agree on the u < v
// one. It panics if u is out of range.
func (g *Graph) EdgeOffset(u int) int {
	if u < 0 || u >= g.NumNodes() {
		panicNode("EdgeOffset", u, g.NumNodes())
	}
	return int(g.offsets[u])
}

// OutNeighbors is [Graph.Neighbors] under the name [Forward] uses. An undirected
// edge can be walked from either end, so every neighbor is a successor.
func (g *Graph) OutNeighbors(u int) []int32 { return g.Neighbors(u) }

// OutWeights is [Graph.Weights] under the name [Forward] uses.
func (g *Graph) OutWeights(u int) []float64 { return g.Weights(u) }

// RandomNeighbor returns a uniformly random neighbor of u. ok is false when u is
// isolated. It panics if u is out of range.
func (g *Graph) RandomNeighbor(u int, r *rand.Rand) (v int, ok bool) {
	if u < 0 || u >= g.NumNodes() {
		panicNode("RandomNeighbor", u, g.NumNodes())
	}
	d := g.Degree(u)
	if d == 0 {
		return 0, false
	}
	return int(g.data[int(g.offsets[u])+r.IntN(d)]), true
}

// Nodes iterates over all node IDs in ascending order.
func (g *Graph) Nodes() iter.Seq[int] {
	return func(yield func(int) bool) {
		for u := 0; u < g.NumNodes(); u++ {
			if !yield(u) {
				return
			}
		}
	}
}

// Edges iterates over each undirected edge exactly once as (u, v) with u < v.
func (g *Graph) Edges() iter.Seq2[int, int] {
	return func(yield func(int, int) bool) {
		for u := 0; u < g.NumNodes(); u++ {
			for _, v := range g.Neighbors(u) {
				if int(v) > u {
					if !yield(u, int(v)) {
						return
					}
				}
			}
		}
	}
}

// ToBuilder returns a mutable copy of the graph, weights included.
func (g *Graph) ToBuilder() *Builder {
	n := g.NumNodes()
	b := &Builder{adj: make([][]int32, n), m: g.m}
	if g.weights != nil {
		b.w = make([][]float64, n)
	}
	for u := 0; u < n; u++ {
		nbrs := g.Neighbors(u)
		b.adj[u] = append(make([]int32, 0, len(nbrs)), nbrs...)
		if b.w != nil {
			b.w[u] = slices.Clone(g.Weights(u))
		}
	}
	return b
}
