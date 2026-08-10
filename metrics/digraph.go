package metrics

import (
	"errors"
	"fmt"
	"math"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/internal/pool"
)

// ErrNoConvergence is returned by PageRank when the power iteration has not
// reached the requested tolerance within the iteration budget. The ranks
// computed so far are still returned alongside it.
var ErrNoConvergence = errors.New("gonx/metrics: power iteration did not converge")

// PageRank returns the PageRank score of every node under the damped
// random-surfer model; the typical call is PageRank(g, 0.85, 1e-6, 100).
// With probability damping the surfer follows a uniformly random outgoing
// edge; otherwise it jumps to a uniformly random node. Nodes with no outgoing
// edges (dangling nodes) contribute their rank uniformly to all nodes, the
// standard correction. Scores sum to 1. This matches networkx.pagerank; the
// conventional damping factor is 0.85.
//
// Iteration stops when the total absolute change in one pass drops below
// n * tolerance (networkx's criterion; a tolerance of 1e-6 is a sensible
// default), or after maxIterations passes, whichever comes first. In the
// latter case the current scores are returned together with
// [ErrNoConvergence].
//
// The update is formulated as a pull: each node gathers rank from its
// in-neighbors, which is why [gonx.Digraph] carrying both edge directions
// matters here. Pulling makes every node's new score independent of the
// others', so passes parallelize across nodes and the result is deterministic
// regardless of worker count.
//
// PageRank returns an error wrapping [gonx.ErrInvalidParam] when damping is
// outside [0, 1), tolerance is not positive, or maxIterations is less than 1.
// The negated comparisons in the validation are deliberate: they also reject
// NaN.
func PageRank(g *gonx.Digraph, damping, tolerance float64, maxIterations int) ([]float64, error) {
	if !(damping >= 0 && damping < 1) {
		return nil, fmt.Errorf("%w: damping %v outside [0, 1)", gonx.ErrInvalidParam, damping)
	}
	if !(tolerance > 0) {
		return nil, fmt.Errorf("%w: tolerance %v must be positive", gonx.ErrInvalidParam, tolerance)
	}
	if maxIterations < 1 {
		return nil, fmt.Errorf("%w: maxIterations %d must be at least 1", gonx.ErrInvalidParam, maxIterations)
	}
	n := g.NumNodes()
	if n <= 0 {
		return nil, nil
	}

	rank := make([]float64, n)
	next := make([]float64, n)
	// invOut[u] is 1/outdeg(u), or 0 for dangling nodes; precomputing it
	// replaces a divide per edge with a multiply.
	invOut := make([]float64, n)
	for u := range n {
		rank[u] = 1 / float64(n)
		if d := g.OutDegree(u); d > 0 {
			invOut[u] = 1 / float64(d)
		}
	}

	w := pool.Workers(n)
	for range maxIterations {
		var dangling float64
		for u := range n {
			if invOut[u] == 0 {
				dangling += rank[u]
			}
		}
		base := (1-damping)/float64(n) + damping*dangling/float64(n)

		pool.ParallelChunks(n, w, func(_, start, end int) {
			for v := start; v < end; v++ {
				var sum float64
				for _, u := range g.InNeighbors(v) {
					sum += rank[u] * invOut[u]
				}
				next[v] = base + damping*sum
			}
		})

		// The convergence delta is summed serially, in index order: float
		// addition is not associative, so a per-worker reduction could flip
		// the stopping test between machines with different GOMAXPROCS and
		// break the determinism promise above. O(n) is negligible next to
		// the O(m) pull pass.
		var delta float64
		for v := range n {
			delta += math.Abs(next[v] - rank[v])
		}
		rank, next = next, rank
		if delta < float64(n)*tolerance {
			return rank, nil
		}
	}
	return rank, ErrNoConvergence
}

// WeaklyConnectedComponents returns the weakly connected components (the
// components of the graph once edge directions are ignored) as slices of node
// IDs, in ascending order of their smallest member. This matches
// networkx.weakly_connected_components up to ordering, which networkx leaves
// unspecified.
func WeaklyConnectedComponents(g *gonx.Digraph) [][]int {
	n := g.NumNodes()
	if n <= 0 {
		return nil
	}
	comp := make([]int, n)
	for i := range comp {
		comp[i] = -1
	}
	var out [][]int
	queue := make([]int32, 0, n)
	visit := func(v int32, id int, members *[]int) {
		if comp[v] == -1 {
			comp[v] = id
			*members = append(*members, int(v))
			queue = append(queue, v)
		}
	}
	for s := range n {
		if comp[s] != -1 {
			continue
		}
		id := len(out)
		members := []int{s}
		comp[s] = id
		queue = queue[:0]
		queue = append(queue, int32(s))
		for head := 0; head < len(queue); head++ {
			u := int(queue[head])
			for _, v := range g.OutNeighbors(u) {
				visit(v, id, &members)
			}
			for _, v := range g.InNeighbors(u) {
				visit(v, id, &members)
			}
		}
		out = append(out, members)
	}
	return out
}
