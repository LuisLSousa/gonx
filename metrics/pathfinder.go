package metrics

import (
	"fmt"
	"math"

	"github.com/LuisLSousa/gonx"
)

// PathFinder answers repeated shortest-path queries, keeping its scratch from
// one query to the next so that each costs time in proportion to the nodes it
// reaches rather than to the size of the graph: a query resets what it
// touched before it returns, so the next starts from clean scratch.
// [ShortestPath] allocates and initializes O(n) scratch on every call, which
// dominates when the target is near and the graph is large; a PathFinder pays
// that once.
//
// The zero value is ready to use. A PathFinder is not tied to one graph: it
// sizes itself to the graph of each query, so one serves a graph and any
// number of [gonx.RestrictedView]s of it alike. Its scratch takes about 24
// bytes per node of the largest graph it has searched, and is kept until the
// PathFinder is dropped.
//
// A PathFinder is not safe for concurrent use; give each goroutine its own.
// The graphs it reads may be shared. Copying a PathFinder, and assigning a
// copy back over the original, are safe. A copy does not inherit the scratch,
// though: it allocates its own on its first query, so a warmed-up PathFinder
// copied once per goroutine warms up again in each.
type PathFinder struct {
	// self is the address of the PathFinder that owns s. A copy carries the
	// original's, which is how prepare recognizes one and gives it scratch
	// of its own rather than share the original's.
	self *PathFinder
	s    *pathScratch
}

// pathScratch is a PathFinder's reusable state. It sits behind a pointer, so
// that a copy holds the same record of what is dirty as the scratch itself:
// assigning a saved copy back over the original then finds the scratch as it
// really is, not as it was when the copy was taken.
type pathScratch struct {
	dist []float64
	prev []int32
	heap nodeHeap
	// The nodes the current query has given a finite distance, and so the
	// only entries of dist, prev and the heap's positions that are not in
	// their initial state. A query resets these, and nothing else, before it
	// returns; the list is left non-empty only by a query cut short by a
	// panic, and the next one resets it first.
	touched []int32
}

// ShortestPath is [ShortestPath] on the PathFinder's scratch: the same path
// and length, the same ErrNegativeWeight rule, and panics in the same cases,
// checked in the same order. The path is newly allocated and stays valid after
// later queries. Apart from that path, a query allocates nothing once the
// PathFinder has searched a graph at least this large.
func (p *PathFinder) ShortestPath(g gonx.Adjacency, src, dst int) (path []int, length float64, err error) {
	n := g.NumNodes()
	if dst < 0 || dst >= n {
		panic(fmt.Sprintf("gonx/metrics: PathFinder.ShortestPath: target %d out of range [0, %d)", dst, n))
	}
	if src < 0 || src >= n {
		panic(fmt.Sprintf("gonx/metrics: PathFinder.ShortestPath: source %d out of range [0, %d)", src, n))
	}
	if g.HasNegativeWeight() {
		return nil, 0, ErrNegativeWeight
	}
	s := p.prepare(n)
	s.search(g, src, dst)
	length = s.dist[dst]
	path = pathTo(s.prev, src, dst, length)
	s.reset()
	return path, length, nil
}

// search is the loop of dijkstra, plus a record of every node it gives a
// finite distance, src first. Weights are non-negative and the comparison is
// strict, so a node goes from +Inf to finite once and is recorded once. The
// loop is a copy rather than shared with dijkstra because the recording,
// switched off by a flag, still made Dijkstra and ShortestPath 7-9% slower
// in alternating benchmarks.
func (s *pathScratch) search(g gonx.Adjacency, src, target int) {
	dist, prev, h := s.dist, s.prev, &s.heap
	dist[src] = 0
	s.touched = append(s.touched, int32(src))
	h.update(src)
	for h.len() > 0 {
		u := h.pop()
		if u == target {
			return
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
				if math.IsInf(dist[v], 1) {
					s.touched = append(s.touched, v32)
				}
				dist[v] = d
				prev[v] = int32(u)
				h.update(v)
			}
		}
	}
}

// reset returns every entry the current query touched to its initial state,
// and empties the heap, which an early exit leaves holding the frontier.
func (s *pathScratch) reset() {
	for _, v := range s.touched {
		s.dist[v] = math.Inf(1)
		s.prev[v] = -1
		s.heap.pos[v] = -1
	}
	s.touched = s.touched[:0]
	s.heap.nodes = s.heap.nodes[:0]
}

// prepare returns the PathFinder's scratch sized to n nodes, giving the zero
// value or a copy scratch of its own first. Every entry up to the capacity is
// kept in its initial state between queries, so shrinking and regrowing within
// it needs no initialization either.
func (p *PathFinder) prepare(n int) *pathScratch {
	if p.self != p || p.s == nil {
		p.self, p.s = p, new(pathScratch)
	}
	s := p.s
	s.reset() // a no-op, unless the last query panicked part way
	if n <= cap(s.dist) {
		s.dist, s.prev, s.heap.pos = s.dist[:n], s.prev[:n], s.heap.pos[:n]
		s.heap.key = s.dist
		return s
	}
	s.dist = make([]float64, n)
	s.prev = make([]int32, n)
	for i := range s.dist {
		s.dist[i] = math.Inf(1)
		s.prev[i] = -1
	}
	s.heap = *newNodeHeap(n, s.dist)
	// A query records each node it reaches once, so n entries always suffice
	// and the list never grows mid-query.
	s.touched = make([]int32, 0, n)
	return s
}
