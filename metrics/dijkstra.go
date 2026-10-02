package metrics

import (
	"errors"
	"fmt"
	"math"

	"github.com/LuisLSousa/gonx"
)

// ErrNegativeWeight is returned by Dijkstra, ShortestPath and
// [PathFinder.ShortestPath] when the graph has an edge with a negative weight,
// wherever it is. Dijkstra's algorithm is only
// correct for non-negative weights, and a negative one is almost always a data
// error rather than an intended shortest-path problem, so the call fails
// instead of returning distances that are silently wrong. The check reads
// [gonx.Adjacency.HasNegativeWeight] before any work is done, so it does not
// depend on which part of the graph a search happens to visit.
var ErrNegativeWeight = errors.New("gonx/metrics: negative edge weight")

// Dijkstra fills dist with the length of the shortest path from src to every
// node, following edges in their forward direction and summing their weights.
// dist[v] is +Inf when v is unreachable and 0 for src itself. When prev is
// non-nil it receives the shortest-path tree: prev[v] is the node before v on
// a shortest path from src, or -1 for src and for unreachable nodes, and
// following prev from any reachable node leads back to src. Both slices must
// have length g.NumNodes(); pass nil for prev when the tree is not needed. The
// slices are overwritten in full, so their previous contents do not matter.
//
// An unweighted graph is walked with unit weights, so the result equals
// BreadthFirst with the distances as floats; callers who know their graph is
// unweighted should prefer BreadthFirst, which is cheaper. Where several
// shortest paths tie, which one prev records is not specified.
//
// Dijkstra returns ErrNegativeWeight if, and only if, g.HasNegativeWeight(),
// and leaves dist and prev untouched. Weights are finite, but a path length can
// still overflow to +Inf; a node reached only by such paths is reported as
// unreachable, with dist +Inf and prev -1.
//
// It panics if src is out of range, or if dist or a non-nil prev does not have
// length g.NumNodes(). It runs in O((n + m) log n) time and allocates O(n)
// scratch per call.
func Dijkstra(g gonx.Adjacency, src int, dist []float64, prev []int32) error {
	return dijkstra("Dijkstra", g, src, -1, dist, prev)
}

// dijkstra is the shared core of Dijkstra and ShortestPath. When target is a
// node rather than -1, the search stops as soon as that node is settled, which
// leaves dist and prev correct for every settled node and for target itself
// but not beyond. caller names the exported function in panic messages.
func dijkstra(caller string, g gonx.Adjacency, src, target int, dist []float64, prev []int32) error {
	n := g.NumNodes()
	if src < 0 || src >= n {
		panic(fmt.Sprintf("gonx/metrics: %s: source %d out of range [0, %d)", caller, src, n))
	}
	if len(dist) != n {
		panic(fmt.Sprintf("gonx/metrics: %s: dist has length %d, want %d", caller, len(dist), n))
	}
	if prev != nil && len(prev) != n {
		panic(fmt.Sprintf("gonx/metrics: %s: prev has length %d, want %d", caller, len(prev), n))
	}
	if g.HasNegativeWeight() {
		return ErrNegativeWeight
	}
	for i := range dist {
		dist[i] = math.Inf(1)
	}
	for i := range prev {
		prev[i] = -1
	}
	dist[src] = 0

	h := newNodeHeap(n, dist)
	h.update(src)
	for h.len() > 0 {
		u := h.pop()
		if u == target {
			return nil
		}
		du := dist[u]
		ws := g.OutWeights(u)
		for i, v32 := range g.OutNeighbors(u) {
			w := 1.0
			if ws != nil {
				w = ws[i]
			}
			// Strict comparison: with <= a zero-weight edge between two
			// settled nodes would rewrite prev into a cycle.
			v := int(v32)
			if d := du + w; d < dist[v] {
				dist[v] = d
				if prev != nil {
					prev[v] = int32(u)
				}
				h.update(v)
			}
		}
	}
	return nil
}

// ShortestPath returns the nodes of a shortest path from src to dst, both
// included, and its length. When dst is unreachable, path is nil and length is
// +Inf; that is an answer, not an error. The error is ErrNegativeWeight under
// the same rule as Dijkstra. It panics if either node is out of range.
//
// The search stops as soon as dst is settled, so it expands only nodes no
// farther from src than dst is. Each call still allocates and initializes O(n)
// scratch, however near dst is, which dominates when many point-to-point
// queries run on a large graph: a [PathFinder] keeps that scratch between
// queries instead. For many targets from one source, call Dijkstra once and
// follow its prev tree.
func ShortestPath(g gonx.Adjacency, src, dst int) (path []int, length float64, err error) {
	n := g.NumNodes()
	if dst < 0 || dst >= n {
		panic(fmt.Sprintf("gonx/metrics: ShortestPath: target %d out of range [0, %d)", dst, n))
	}
	// Fail before paying for the scratch; the core repeats both checks.
	if src < 0 || src >= n {
		panic(fmt.Sprintf("gonx/metrics: ShortestPath: source %d out of range [0, %d)", src, n))
	}
	if g.HasNegativeWeight() {
		return nil, 0, ErrNegativeWeight
	}
	dist := make([]float64, n)
	prev := make([]int32, n)
	if err := dijkstra("ShortestPath", g, src, dst, dist, prev); err != nil {
		return nil, 0, err
	}
	return pathTo(prev, src, dst, dist[dst]), dist[dst], nil
}

// pathTo follows prev back from dst to src and returns the nodes in order,
// or nil when length says dst was not reached.
func pathTo(prev []int32, src, dst int, length float64) []int {
	if math.IsInf(length, 1) {
		return nil
	}
	hops := 0
	for v := dst; v != src; v = int(prev[v]) {
		hops++
	}
	path := make([]int, hops+1)
	for v, i := dst, hops; ; v, i = int(prev[v]), i-1 {
		path[i] = v
		if v == src {
			break
		}
	}
	return path
}

// nodeHeap is a binary min-heap of node IDs ordered by their current tentative
// distance, which it reads from the caller's dist slice rather than storing
// keys of its own. A position index lets a node whose distance improves be
// moved up in place (decrease-key) instead of being pushed a second time, so
// the heap never holds more than n entries. Two int32 arrays of length n are
// its whole footprint: that is the "O(n) scratch" in Dijkstra's contract, and
// two allocations per call. container/heap is not used because its interface
// boxes every element and dispatches every comparison dynamically, both of
// which show up on graphs with millions of edges. The index was also measured
// against a lazy heap, which pushes a node again on every improvement and skips
// stale entries on pop: the indexed heap was faster on graphs that fit in
// cache and level with it on larger ones.
type nodeHeap struct {
	key   []float64 // dist, owned by the caller
	nodes []int32   // heap order
	pos   []int32   // pos[v] is v's index in nodes, or -1 when v is not in the heap
}

// newNodeHeap returns the heap by value, so that the struct itself lives in
// its caller's frame whether or not the call is inlined, and the two arrays
// are the only allocations.
func newNodeHeap(n int, key []float64) nodeHeap {
	h := nodeHeap{key: key, nodes: make([]int32, 0, n), pos: make([]int32, n)}
	for i := range h.pos {
		h.pos[i] = -1
	}
	return h
}

func (h *nodeHeap) len() int { return len(h.nodes) }

// update pushes v if it is not in the heap and otherwise restores heap order
// after v's key decreased. Keys only ever decrease in Dijkstra, so sifting up
// is always the right direction.
func (h *nodeHeap) update(v int) {
	i := int(h.pos[v])
	if i < 0 {
		i = len(h.nodes)
		h.nodes = append(h.nodes, int32(v))
		h.pos[v] = int32(i)
	}
	h.siftUp(i)
}

// pop removes and returns the node with the smallest key.
func (h *nodeHeap) pop() int {
	root := h.nodes[0]
	last := len(h.nodes) - 1
	h.nodes[0] = h.nodes[last]
	h.pos[h.nodes[0]] = 0
	h.nodes = h.nodes[:last]
	h.pos[root] = -1
	if last > 0 {
		h.siftDown(0)
	}
	return int(root)
}

func (h *nodeHeap) siftUp(i int) {
	v := h.nodes[i]
	k := h.key[v]
	for i > 0 {
		p := (i - 1) / 2
		pv := h.nodes[p]
		if h.key[pv] <= k {
			break
		}
		h.nodes[i] = pv
		h.pos[pv] = int32(i)
		i = p
	}
	h.nodes[i] = v
	h.pos[v] = int32(i)
}

func (h *nodeHeap) siftDown(i int) {
	v := h.nodes[i]
	k := h.key[v]
	n := len(h.nodes)
	for {
		c := 2*i + 1
		if c >= n {
			break
		}
		if r := c + 1; r < n && h.key[h.nodes[r]] < h.key[h.nodes[c]] {
			c = r
		}
		cv := h.nodes[c]
		if k <= h.key[cv] {
			break
		}
		h.nodes[i] = cv
		h.pos[cv] = int32(i)
		i = c
	}
	h.nodes[i] = v
	h.pos[v] = int32(i)
}
