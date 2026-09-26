package gonx

import (
	"fmt"
	"iter"
	"math"
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
//
// A DigraphBuilder starts unweighted. The first [DigraphBuilder.AddEdgeW] call
// switches it to weighted for good, with the same rules as [Builder.AddEdgeW].
type DigraphBuilder struct {
	out       [][]int32   // out[u] holds the targets of u's outgoing edges
	w         [][]float64 // w[u] weights a prefix of out[u] (see Builder.AddEdgeW); nil until the first AddEdgeW
	inDegrees []int32     // per-node counts, kept current on Add/Remove so InDegree is O(1)
	m         int         // number of directed edges
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

// Weighted reports whether the Builder carries edge weights, which is the case
// from the first [DigraphBuilder.AddEdgeW] call on.
func (b *DigraphBuilder) Weighted() bool { return b.w != nil }

// AddNode appends a new isolated node and returns its ID. It panics if the node
// count would exceed 2^31-1.
func (b *DigraphBuilder) AddNode() int {
	if len(b.out) >= maxNodes {
		panic("gonx: AddNode: node count would exceed 2^31-1")
	}
	b.out = append(b.out, nil)
	if b.w != nil {
		b.w = append(b.w, nil)
	}
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

// AddEdgeW inserts the directed edge u->v with weight w. It rejects everything
// AddEdge rejects, plus NaN and infinite weights, returning false in each case.
// The first call makes the Builder weighted: edges already present receive
// weight 1, as do edges added later without an explicit weight. See
// [Builder.AddEdgeW] for the reasoning.
func (b *DigraphBuilder) AddEdgeW(u, v int, w float64) bool {
	if math.IsNaN(w) || math.IsInf(w, 0) {
		return false
	}
	n := len(b.out)
	if u == v || u < 0 || v < 0 || u >= n || v >= n || b.HasEdge(u, v) {
		return false
	}
	if b.w == nil {
		b.w = make([][]float64, n)
	}
	b.out[u] = append(b.out[u], int32(v))
	b.w[u] = appendWeight(b.w[u], len(b.out[u])-1, w)
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
	i := unlink(&b.out[u], int32(v))
	if b.w != nil {
		b.w[u] = moveWeight(b.w[u], i, len(b.out[u]))
	}
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
// binary-search edge tests and has a canonical, deterministic layout. On a
// weighted Builder each weight travels with its edge into both directions. The
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
	maxOut := 0
	for u := range n {
		outOffsets[u] = off
		off += int32(len(b.out[u]))
		maxOut = max(maxOut, len(b.out[u]))
	}
	outOffsets[n] = off
	outData := make([]int32, b.m)
	var outWeights []float64
	if b.w == nil {
		for u := range n {
			row := outData[outOffsets[u]:outOffsets[u+1]]
			copy(row, b.out[u])
			slices.Sort(row)
		}
	} else {
		outWeights = make([]float64, b.m)
		keys := make([]uint64, maxOut)
		row := make([]float64, maxOut)
		for u := range n {
			lo, hi := outOffsets[u], outOffsets[u+1]
			ws := denseWeights(b.w[u], row[:len(b.out[u])])
			copySorted(b.out[u], ws, outData[lo:hi], outWeights[lo:hi], keys)
		}
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
	var inWeights []float64
	if outWeights == nil {
		for u := range n {
			for _, v := range outData[outOffsets[u]:outOffsets[u+1]] {
				inData[cursor[v]] = int32(u)
				cursor[v]++
			}
		}
	} else {
		// The same pass carries each edge's weight to its in-list slot.
		inWeights = make([]float64, b.m)
		for u := range n {
			lo, hi := outOffsets[u], outOffsets[u+1]
			for i, v := range outData[lo:hi] {
				inData[cursor[v]] = int32(u)
				inWeights[cursor[v]] = outWeights[int(lo)+i]
				cursor[v]++
			}
		}
	}

	return &Digraph{
		outOffsets: outOffsets, outData: outData, outWeights: outWeights,
		inOffsets: inOffsets, inData: inData, inWeights: inWeights,
		m: b.m,
	}
}

// Digraph is an immutable, directed graph stored in Compressed Sparse Row
// form, twice: the out-neighbors of node u occupy
// outData[outOffsets[u]:outOffsets[u+1]] and its in-neighbors mirror that in
// a second CSR, both sorted ascending. Storing both directions costs 2x the edge memory
// and buys O(1) access from either end, which is what reverse-flow algorithms
// (PageRank pulls rank from in-neighbors) and "who links here" queries need.
// On a weighted Digraph each direction has a weight array with the same
// indices as its adjacency. A Digraph is safe for concurrent reads.
//
// In networkx terms, OutNeighbors are a node's successors and InNeighbors its
// predecessors.
//
// Accessors that take a node ID panic with a descriptive message when the ID is
// outside [0, N); HasEdge and Weight are the exceptions and report false for
// out-of-range endpoints. The zero value is not a valid Digraph; obtain one from
// [DigraphBuilder.Build].
type Digraph struct {
	outOffsets []int32   // length n+1
	outData    []int32   // length m; concatenated sorted out-neighbor lists
	outWeights []float64 // length m, outWeights[i] belongs to outData[i]; nil when unweighted
	inOffsets  []int32   // length n+1
	inData     []int32   // length m; concatenated sorted in-neighbor lists
	inWeights  []float64 // length m, aligned with inData; nil when unweighted
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

// Weighted reports whether the graph carries edge weights. An unweighted
// Digraph stores no weight arrays: OutWeights and InWeights return nil for every
// node and Weight reports 1 for every edge.
func (g *Digraph) Weighted() bool { return g.outWeights != nil }

// OutWeights returns the weights of u's outgoing edges, aligned index for index
// with [Digraph.OutNeighbors]. The slice is zero-copy and MUST NOT be modified.
// It is nil when the graph is unweighted. It panics if u is out of range.
func (g *Digraph) OutWeights(u int) []float64 {
	if u < 0 || u >= g.NumNodes() {
		panicNode("OutWeights", u, g.NumNodes())
	}
	if g.outWeights == nil {
		return nil
	}
	return g.outWeights[g.outOffsets[u]:g.outOffsets[u+1]]
}

// InWeights returns the weights of u's incoming edges, aligned index for index
// with [Digraph.InNeighbors]. The slice is zero-copy and MUST NOT be modified.
// It is nil when the graph is unweighted. It panics if u is out of range.
func (g *Digraph) InWeights(u int) []float64 {
	if u < 0 || u >= g.NumNodes() {
		panicNode("InWeights", u, g.NumNodes())
	}
	if g.inWeights == nil {
		return nil
	}
	return g.inWeights[g.inOffsets[u]:g.inOffsets[u+1]]
}

// Weight returns the weight of the directed edge u->v. ok is false when the
// edge does not exist or an endpoint is out of range. On an unweighted graph
// every existing edge has weight 1. Like [Digraph.HasEdge] it binary-searches
// the shorter of u's out-list and v's in-list, so it runs in O(log deg).
func (g *Digraph) Weight(u, v int) (w float64, ok bool) {
	n := g.NumNodes()
	if u < 0 || v < 0 || u >= n || v >= n {
		return 0, false
	}
	if g.OutDegree(u) <= g.InDegree(v) {
		i, found := slices.BinarySearch(g.OutNeighbors(u), int32(v))
		if !found {
			return 0, false
		}
		if g.outWeights == nil {
			return 1, true
		}
		return g.outWeights[int(g.outOffsets[u])+i], true
	}
	i, found := slices.BinarySearch(g.InNeighbors(v), int32(u))
	if !found {
		return 0, false
	}
	if g.inWeights == nil {
		return 1, true
	}
	return g.inWeights[int(g.inOffsets[v])+i], true
}

// OutEdgeOffset returns the index at which u's outgoing edges start in the
// graph's out-edge arrays: entry i of OutNeighbors(u), and of OutWeights(u),
// sits at position OutEdgeOffset(u)+i in an array of length NumEdges. See
// [Graph.EdgeOffset] for what this is for. It panics if u is out of range.
func (g *Digraph) OutEdgeOffset(u int) int {
	if u < 0 || u >= g.NumNodes() {
		panicNode("OutEdgeOffset", u, g.NumNodes())
	}
	return int(g.outOffsets[u])
}

// InEdgeOffset is the in-edge counterpart of [Digraph.OutEdgeOffset]: entry i of
// InNeighbors(u) sits at position InEdgeOffset(u)+i in an array of length
// NumEdges indexed like the in-adjacency. The two indexings are unrelated; an
// edge's out slot and in slot differ. It panics if u is out of range.
func (g *Digraph) InEdgeOffset(u int) int {
	if u < 0 || u >= g.NumNodes() {
		panicNode("InEdgeOffset", u, g.NumNodes())
	}
	return int(g.inOffsets[u])
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

// ToBuilder returns a mutable copy of the graph, weights included.
func (g *Digraph) ToBuilder() *DigraphBuilder {
	n := g.NumNodes()
	b := &DigraphBuilder{out: make([][]int32, n), inDegrees: make([]int32, n), m: g.m}
	if g.outWeights != nil {
		b.w = make([][]float64, n)
	}
	for u := range n {
		nbrs := g.OutNeighbors(u)
		b.out[u] = append(make([]int32, 0, len(nbrs)), nbrs...)
		if b.w != nil {
			b.w[u] = slices.Clone(g.OutWeights(u))
		}
		b.inDegrees[u] = g.inOffsets[u+1] - g.inOffsets[u]
	}
	return b
}
