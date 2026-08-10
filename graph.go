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
// Node IDs are dense integers in the range [0, N). Graphs are unweighted and
// always simple (no self-loops or duplicate edges). All randomized operations
// take an explicit *math/rand/v2.Rand so results are fully reproducible; the
// package never touches a global RNG.
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
type Builder struct {
	adj [][]int32 // adj[u] holds u's neighbors; undirected edges appear in both lists
	m   int       // number of undirected edges
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

// AddNode appends a new isolated node and returns its ID. It panics if the node
// count would exceed 2^31-1.
func (b *Builder) AddNode() int {
	if len(b.adj) >= maxNodes {
		panic("gonx: AddNode: node count would exceed 2^31-1")
	}
	b.adj = append(b.adj, nil)
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
	remove(&b.adj[u], int32(v))
	remove(&b.adj[v], int32(u))
	b.m--
	return true
}

// remove deletes the first occurrence of x from *s (order not preserved).
func remove(s *[]int32, x int32) {
	a := *s
	for i, w := range a {
		if w == x {
			a[i] = a[len(a)-1]
			*s = a[:len(a)-1]
			return
		}
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
// canonical, deterministic layout. The Builder may be reused afterwards.
//
// Build panics if the total adjacency size (2 * edges) exceeds 2^31-1, the
// capacity of the int32 CSR offsets.
func (b *Builder) Build() *Graph {
	n := len(b.adj)
	total := 0
	for _, nbrs := range b.adj {
		total += len(nbrs)
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
	for u := 0; u < n; u++ {
		row := data[offsets[u]:offsets[u+1]]
		copy(row, b.adj[u])
		slices.Sort(row)
	}
	return &Graph{offsets: offsets, data: data, m: b.m}
}

// Graph is an immutable, undirected, unweighted graph stored in Compressed Sparse
// Row form. The neighbors of node u occupy data[offsets[u]:offsets[u+1]] and are
// sorted ascending. A Graph is safe for concurrent reads. The zero value is not
// a valid Graph; obtain one from [Builder.Build].
//
// Accessors that take a node ID (Degree, Neighbors, NeighborsSeq, RandomNeighbor)
// panic with a descriptive message when the ID is outside [0, N); HasEdge is the
// exception and reports false for out-of-range endpoints.
type Graph struct {
	offsets []int32 // length n+1
	data    []int32 // length 2*m; concatenated sorted neighbor lists
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

// ToBuilder returns a mutable copy of the graph.
func (g *Graph) ToBuilder() *Builder {
	n := g.NumNodes()
	b := &Builder{adj: make([][]int32, n), m: g.m}
	for u := 0; u < n; u++ {
		nbrs := g.Neighbors(u)
		b.adj[u] = append(make([]int32, 0, len(nbrs)), nbrs...)
	}
	return b
}
