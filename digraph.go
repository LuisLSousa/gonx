package gonx

import (
	"fmt"
	"iter"
	"math/rand/v2"
	"slices"
)

// DigraphBuilder is a mutable directed graph used to assemble a topology before
// freezing it into an immutable [Digraph]. It is not safe for concurrent use.
//
// Like [Builder], it keeps the graph simple: self-loops and duplicate edges are
// rejected. The directed edge u->v and its reverse v->u are distinct edges and
// may coexist.
//
// Edge methods (AddEdge, RemoveEdge, HasEdge) treat out-of-range endpoints as
// absent edges and report false; the degree accessors panic on an out-of-range
// node.
type DigraphBuilder struct {
	out       [][]int32 // out[u] holds the targets of u's outgoing edges
	inDegrees []int32   // per-node counts, kept current on Add/Remove so InDegree is O(1)
	m         int       // number of directed edges
}

// NewDigraphBuilder returns a DigraphBuilder with n isolated nodes (IDs 0..n-1).
// It panics if n exceeds 2^31-1, the maximum node count supported by the int32
// CSR layout.
func NewDigraphBuilder(n int) *DigraphBuilder {
	if n < 0 {
		n = 0
	}
	if n > maxNodes {
		panic(fmt.Sprintf("gonx: NewDigraphBuilder: node count %d exceeds max %d", n, maxNodes))
	}
	return &DigraphBuilder{out: make([][]int32, n), inDegrees: make([]int32, n)}
}

// NumNodes reports the number of nodes.
func (b *DigraphBuilder) NumNodes() int { return len(b.out) }

// NumEdges reports the number of directed edges.
func (b *DigraphBuilder) NumEdges() int { return b.m }

// AddNode appends a new isolated node and returns its ID. It panics if the node
// count would exceed 2^31-1.
func (b *DigraphBuilder) AddNode() int {
	if len(b.out) >= maxNodes {
		panic("gonx: AddNode: node count would exceed 2^31-1")
	}
	b.out = append(b.out, nil)
	b.inDegrees = append(b.inDegrees, 0)
	return len(b.out) - 1
}

// HasEdge reports whether the directed edge u->v exists. O(outdeg(u)).
func (b *DigraphBuilder) HasEdge(u, v int) bool {
	if u < 0 || u >= len(b.out) || v < 0 || v >= len(b.out) {
		return false
	}
	vv := int32(v)
	return slices.Contains(b.out[u], vv)
}

// AddEdge inserts the directed edge u->v. It returns false (and does nothing)
// for self-loops, out-of-range endpoints, or edges that already exist, so the
// resulting graph is always simple. Inserting v->u afterwards is a distinct
// edge and succeeds.
func (b *DigraphBuilder) AddEdge(u, v int) bool {
	n := len(b.out)
	if u == v || u < 0 || v < 0 || u >= n || v >= n {
		return false
	}
	if b.HasEdge(u, v) {
		return false
	}
	b.out[u] = append(b.out[u], int32(v))
	b.inDegrees[v]++
	b.m++
	return true
}

// AddEdgeUnchecked inserts the directed edge u->v without checking whether it
// already exists. Endpoints are still validated and self-loops rejected
// (returning false), but inserting an edge that is already present corrupts the
// Builder: the graph silently becomes a multigraph with a double-counted
// NumEdges. Use it only when each ordered pair is known to be produced at most
// once, where skipping the duplicate scan turns dense O(n*m) builds into O(m).
func (b *DigraphBuilder) AddEdgeUnchecked(u, v int) bool {
	n := len(b.out)
	if u == v || u < 0 || v < 0 || u >= n || v >= n {
		return false
	}
	b.out[u] = append(b.out[u], int32(v))
	b.inDegrees[v]++
	b.m++
	return true
}

// RemoveEdge deletes the directed edge u->v, returning whether it existed. The
// reverse edge v->u, if present, is unaffected.
func (b *DigraphBuilder) RemoveEdge(u, v int) bool {
	if !b.HasEdge(u, v) {
		return false
	}
	remove(&b.out[u], int32(v))
	b.inDegrees[v]--
	b.m--
	return true
}

// OutDegree returns the number of outgoing edges of u. It panics if u is out of
// range.
func (b *DigraphBuilder) OutDegree(u int) int {
	if u < 0 || u >= len(b.out) {
		panicNode("OutDegree", u, len(b.out))
	}
	return len(b.out[u])
}

// InDegree returns the number of incoming edges of u. It panics if u is out of
// range.
func (b *DigraphBuilder) InDegree(u int) int {
	if u < 0 || u >= len(b.out) {
		panicNode("InDegree", u, len(b.out))
	}
	return int(b.inDegrees[u])
}

// Build freezes the Builder into an immutable CSR [Digraph], materializing
// both adjacency directions: the out-lists directly, the in-lists by a counting
// pass over them. Both are sorted, so the resulting Digraph supports
// binary-search edge tests and has a canonical, deterministic layout. The
// Builder may be reused afterwards.
//
// Build panics if the edge count exceeds 2^31-1, the capacity of the int32 CSR
// offsets.
func (b *DigraphBuilder) Build() *Digraph {
	n := len(b.out)
	if b.m > maxNodes {
		panic(fmt.Sprintf("gonx: Build: edge count %d exceeds int32 CSR capacity %d", b.m, maxNodes))
	}

	outOffsets := make([]int32, n+1)
	var off int32
	for u := range n {
		outOffsets[u] = off
		off += int32(len(b.out[u]))
	}
	outOffsets[n] = off
	outData := make([]int32, b.m)
	for u := range n {
		row := outData[outOffsets[u]:outOffsets[u+1]]
		copy(row, b.out[u])
		slices.Sort(row)
	}

	inOffsets := make([]int32, n+1)
	off = 0
	for u := range n {
		inOffsets[u] = off
		off += b.inDegrees[u]
	}
	inOffsets[n] = off
	inData := make([]int32, b.m)
	cursor := make([]int32, n)
	copy(cursor, inOffsets[:n])
	// Filling in ascending source order writes each in-list already sorted.
	for u := range n {
		for _, v := range outData[outOffsets[u]:outOffsets[u+1]] {
			inData[cursor[v]] = int32(u)
			cursor[v]++
		}
	}

	return &Digraph{outOffsets: outOffsets, outData: outData, inOffsets: inOffsets, inData: inData, m: b.m}
}

// Digraph is an immutable, directed, unweighted graph stored in Compressed
// Sparse Row form, twice: the out-neighbors of node u occupy
// outData[outOffsets[u]:outOffsets[u+1]] and its in-neighbors mirror that in
// a second CSR, both sorted ascending. Storing both directions costs 2x the edge memory
// and buys O(1) access from either end, which is what reverse-flow algorithms
// (PageRank pulls rank from in-neighbors) and "who links here" queries need.
// A Digraph is safe for concurrent reads.
//
// In networkx terms, OutNeighbors are a node's successors and InNeighbors its
// predecessors.
//
// Accessors that take a node ID panic with a descriptive message when the ID is
// outside [0, N); HasEdge is the exception and reports false for out-of-range
// endpoints. The zero value is not a valid Digraph; obtain one from
// [DigraphBuilder.Build].
type Digraph struct {
	outOffsets []int32 // length n+1
	outData    []int32 // length m; concatenated sorted out-neighbor lists
	inOffsets  []int32 // length n+1
	inData     []int32 // length m; concatenated sorted in-neighbor lists
	m          int
}

// NumNodes reports the number of nodes.
func (g *Digraph) NumNodes() int { return len(g.outOffsets) - 1 }

// NumEdges reports the number of directed edges.
func (g *Digraph) NumEdges() int { return g.m }

// OutDegree returns the number of outgoing edges of u. It panics if u is out of
// range.
func (g *Digraph) OutDegree(u int) int {
	if u < 0 || u >= g.NumNodes() {
		panicNode("OutDegree", u, g.NumNodes())
	}
	return int(g.outOffsets[u+1] - g.outOffsets[u])
}

// InDegree returns the number of incoming edges of u. It panics if u is out of
// range.
func (g *Digraph) InDegree(u int) int {
	if u < 0 || u >= g.NumNodes() {
		panicNode("InDegree", u, g.NumNodes())
	}
	return int(g.inOffsets[u+1] - g.inOffsets[u])
}

// Degree returns InDegree(u) + OutDegree(u), matching networkx's
// DiGraph.degree. It panics if u is out of range.
func (g *Digraph) Degree(u int) int {
	if u < 0 || u >= g.NumNodes() {
		panicNode("Degree", u, g.NumNodes())
	}
	return int(g.outOffsets[u+1] - g.outOffsets[u] + g.inOffsets[u+1] - g.inOffsets[u])
}

// OutNeighbors returns the targets of u's outgoing edges as a sorted, zero-copy
// slice into the graph's backing storage. Callers MUST NOT modify the returned
// slice. It panics if u is out of range.
func (g *Digraph) OutNeighbors(u int) []int32 {
	if u < 0 || u >= g.NumNodes() {
		panicNode("OutNeighbors", u, g.NumNodes())
	}
	return g.outData[g.outOffsets[u]:g.outOffsets[u+1]]
}

// InNeighbors returns the sources of u's incoming edges as a sorted, zero-copy
// slice into the graph's backing storage. Callers MUST NOT modify the returned
// slice. It panics if u is out of range.
func (g *Digraph) InNeighbors(u int) []int32 {
	if u < 0 || u >= g.NumNodes() {
		panicNode("InNeighbors", u, g.NumNodes())
	}
	return g.inData[g.inOffsets[u]:g.inOffsets[u+1]]
}

// OutNeighborsSeq iterates over the targets of u's outgoing edges in ascending
// order as ints. Hot paths should prefer [Digraph.OutNeighbors], which exposes
// the backing slice with no per-element call overhead. It panics if u is out of
// range.
func (g *Digraph) OutNeighborsSeq(u int) iter.Seq[int] {
	return intSeq(g.OutNeighbors(u))
}

// InNeighborsSeq iterates over the sources of u's incoming edges in ascending
// order as ints. Hot paths should prefer [Digraph.InNeighbors], which exposes
// the backing slice with no per-element call overhead. It panics if u is out of
// range.
func (g *Digraph) InNeighborsSeq(u int) iter.Seq[int] {
	return intSeq(g.InNeighbors(u))
}

func intSeq(nbrs []int32) iter.Seq[int] {
	return func(yield func(int) bool) {
		for _, v := range nbrs {
			if !yield(int(v)) {
				return
			}
		}
	}
}

// HasEdge reports whether the directed edge u->v exists. It binary-searches the
// shorter of u's out-list and v's in-list, so it runs in O(log deg).
func (g *Digraph) HasEdge(u, v int) bool {
	n := g.NumNodes()
	if u < 0 || v < 0 || u >= n || v >= n {
		return false
	}
	if g.OutDegree(u) <= g.InDegree(v) {
		_, found := slices.BinarySearch(g.OutNeighbors(u), int32(v))
		return found
	}
	_, found := slices.BinarySearch(g.InNeighbors(v), int32(u))
	return found
}

// RandomOutNeighbor returns a uniformly random target of u's outgoing edges. ok
// is false when u has none. It panics if u is out of range.
func (g *Digraph) RandomOutNeighbor(u int, r *rand.Rand) (v int, ok bool) {
	if u < 0 || u >= g.NumNodes() {
		panicNode("RandomOutNeighbor", u, g.NumNodes())
	}
	d := g.OutDegree(u)
	if d == 0 {
		return 0, false
	}
	return int(g.outData[int(g.outOffsets[u])+r.IntN(d)]), true
}

// RandomInNeighbor returns a uniformly random source of u's incoming edges. ok
// is false when u has none. It panics if u is out of range.
func (g *Digraph) RandomInNeighbor(u int, r *rand.Rand) (v int, ok bool) {
	if u < 0 || u >= g.NumNodes() {
		panicNode("RandomInNeighbor", u, g.NumNodes())
	}
	d := g.InDegree(u)
	if d == 0 {
		return 0, false
	}
	return int(g.inData[int(g.inOffsets[u])+r.IntN(d)]), true
}

// Nodes iterates over all node IDs in ascending order.
func (g *Digraph) Nodes() iter.Seq[int] {
	return func(yield func(int) bool) {
		for u := 0; u < g.NumNodes(); u++ {
			if !yield(u) {
				return
			}
		}
	}
}

// Edges iterates over each directed edge exactly once as (u, v), ordered by
// source and then by target.
func (g *Digraph) Edges() iter.Seq2[int, int] {
	return func(yield func(int, int) bool) {
		for u := 0; u < g.NumNodes(); u++ {
			for _, v := range g.OutNeighbors(u) {
				if !yield(u, int(v)) {
					return
				}
			}
		}
	}
}

// ToBuilder returns a mutable copy of the graph.
func (g *Digraph) ToBuilder() *DigraphBuilder {
	n := g.NumNodes()
	b := &DigraphBuilder{out: make([][]int32, n), inDegrees: make([]int32, n), m: g.m}
	for u := range n {
		nbrs := g.OutNeighbors(u)
		b.out[u] = append(make([]int32, 0, len(nbrs)), nbrs...)
		b.inDegrees[u] = g.inOffsets[u+1] - g.inOffsets[u]
	}
	return b
}
