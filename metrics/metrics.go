// Package metrics computes structural graph properties: clustering, connectivity,
// and shortest-path-based measures. The all-pairs measures are parallelized over
// source nodes, which are independent; because the reductions are
// order-independent sums and counts, results are deterministic regardless of the
// degree of parallelism.
package metrics

import (
	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/internal/pool"
)

// Transitivity returns the global clustering coefficient: 3 * (number of
// triangles) / (number of connected triples). It is 0 when the graph has no
// connected triples. This matches networkx.transitivity.
func Transitivity(g *gonx.Graph) float64 {
	n := g.NumNodes()
	w := pool.Workers(n)
	triangles := make([]int64, w) // one slot per worker, written once per chunk
	triads := make([]int64, w)

	pool.ParallelChunks(n, w, func(worker, start, end int) {
		var tri, tpl int64 // accumulate locally; publishing per-item would false-share
		for u := start; u < end; u++ {
			nbrs := g.Neighbors(u)
			d := len(nbrs)
			tpl += int64(d) * int64(d-1) / 2
			// Count edges among u's neighbors. Each such edge closes a triangle at
			// u; summed over all u this counts every triangle exactly 3 times, which
			// the factor of 3 in the transitivity formula cancels against the triple
			// count.
			for i := 0; i < d; i++ {
				for j := i + 1; j < d; j++ {
					if g.HasEdge(int(nbrs[i]), int(nbrs[j])) {
						tri++
					}
				}
			}
		}
		triangles[worker] = tri
		triads[worker] = tpl
	})

	var tri, tpl int64
	for i := 0; i < w; i++ {
		tri += triangles[i]
		tpl += triads[i]
	}
	if tpl == 0 {
		return 0
	}
	// tri already counts each triangle 3 times (once per vertex), so 3*triangles
	// equals tri, and transitivity = tri / triads.
	return float64(tri) / float64(tpl)
}

// AverageClustering returns the mean over all nodes of the local clustering
// coefficient (the fraction of a node's neighbor pairs that are connected). Nodes
// with degree < 2 contribute 0. This differs from Transitivity, matching
// networkx.average_clustering.
func AverageClustering(g *gonx.Graph) float64 {
	n := g.NumNodes()
	if n == 0 {
		return 0
	}
	w := pool.Workers(n)
	sums := make([]float64, w)
	pool.ParallelChunks(n, w, func(worker, start, end int) {
		var sum float64
		for u := start; u < end; u++ {
			nbrs := g.Neighbors(u)
			d := len(nbrs)
			if d < 2 {
				continue
			}
			links := 0
			for i := 0; i < d; i++ {
				for j := i + 1; j < d; j++ {
					if g.HasEdge(int(nbrs[i]), int(nbrs[j])) {
						links++
					}
				}
			}
			sum += 2 * float64(links) / (float64(d) * float64(d-1))
		}
		sums[worker] = sum
	})
	var total float64
	for _, s := range sums {
		total += s
	}
	return total / float64(n)
}

// bfsDistances fills dist with the shortest-path distance from src to every node,
// using the provided queue as scratch. Unreachable nodes are left as -1. Both
// dist and queue must have length >= n; the caller reuses them across sources.
func bfsDistances(g *gonx.Graph, src int, dist []int32, queue []int32) {
	for i := range dist {
		dist[i] = -1
	}
	dist[src] = 0
	queue = queue[:0]
	queue = append(queue, int32(src))
	for head := 0; head < len(queue); head++ {
		u := queue[head]
		du := dist[u]
		for _, v := range g.Neighbors(int(u)) {
			if dist[v] == -1 {
				dist[v] = du + 1
				queue = append(queue, v)
			}
		}
	}
}

// BFS fills dist with shortest-path distances from src; dist[v] == -1 means v is
// unreachable from src. dist must have length g.NumNodes().
func BFS(g *gonx.Graph, src int, dist []int32) {
	bfsDistances(g, src, dist, make([]int32, 0, g.NumNodes()))
}
