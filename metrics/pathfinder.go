package metrics

import (
	"fmt"
	"math"

	"github.com/LuisLSousa/gonx"
)

// PathFinder answers repeated shortest-path queries, keeping its scratch from
// one query to the next so that each costs time in proportion to the nodes it
// reaches rather than to the size of the graph. [ShortestPath] allocates and
// initializes O(n) scratch on every call, which dominates when the target is
// near and the graph is large; a PathFinder pays that once.
//
// The zero value is ready to use. A PathFinder is not tied to one graph: it
// sizes itself to the graph of each query, so one serves a graph and any
// number of [gonx.RestrictedView]s of it alike. Its scratch takes about 24
// bytes per node of the largest graph it has searched, and is kept until the
// PathFinder is dropped.
//
// A PathFinder is not safe for concurrent use; give each goroutine its own.
// The graphs it reads may be shared. Copying a PathFinder is safe, but the
// copy does not inherit the scratch: it allocates its own on its first query,
// so a warmed-up PathFinder copied once per goroutine warms up again in each.
type PathFinder struct {
	// self is the PathFinder's address as of its last query. A copy carries
	// the original's, which is how prepare recognizes one: a copy shares the
	// original's arrays but has its own touched list, so each would reset
	// only its own part of what the other dirtied.
	self *PathFinder
	dist []float64
	prev []int32
	heap nodeHeap
	// The nodes the last query gave a finite distance, and so the only
	// entries of dist, prev and the heap's positions that are not in their
	// initial state. The next query resets these and nothing else.
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
	p.prepare(n)
	p.search(g, src, dst)
	return pathTo(p.prev, src, dst, p.dist[dst]), p.dist[dst], nil
}

// search is the loop of dijkstra, plus a record of every node it gives a
// finite distance, src first. Weights are non-negative and the comparison is
// strict, so a node goes from +Inf to finite once and is recorded once. The
// loop is a copy rather than shared with dijkstra because the recording,
// switched off by a flag, still made Dijkstra and ShortestPath 7-9% slower
// in alternating benchmarks.
func (p *PathFinder) search(g gonx.Adjacency, src, target int) {
	dist, prev, h := p.dist, p.prev, &p.heap
	dist[src] = 0
	p.touched = append(p.touched, int32(src))
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
					p.touched = append(p.touched, v32)
				}
				dist[v] = d
				prev[v] = int32(u)
				h.update(v)
			}
		}
	}
}

// prepare undoes the last query and sizes the scratch to n nodes. Every entry
// up to the capacity is kept in its initial state between queries, so
// shrinking and regrowing within it needs no initialization either. On the
// zero value, or on a copy, it first drops whatever scratch is there.
func (p *PathFinder) prepare(n int) {
	if p.self != p {
		*p = PathFinder{self: p}
	}
	for _, v := range p.touched {
		p.dist[v] = math.Inf(1)
		p.prev[v] = -1
		p.heap.pos[v] = -1
	}
	p.touched = p.touched[:0]
	p.heap.nodes = p.heap.nodes[:0]
	if n <= cap(p.dist) {
		p.dist, p.prev, p.heap.pos = p.dist[:n], p.prev[:n], p.heap.pos[:n]
		p.heap.key = p.dist
		return
	}
	p.dist = make([]float64, n)
	p.prev = make([]int32, n)
	for i := range p.dist {
		p.dist[i] = math.Inf(1)
		p.prev[i] = -1
	}
	p.heap = *newNodeHeap(n, p.dist)
	// A query records each node it reaches once, so n entries always suffice
	// and the list never grows mid-query.
	p.touched = make([]int32, 0, n)
}
