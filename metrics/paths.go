package metrics

import (
	"errors"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/internal/pool"
)

// ErrDisconnectedGraph is returned by AveragePathLength when the graph is not
// connected, since the average shortest path length is undefined across
// components. This matches networkx.average_shortest_path_length, which raises in
// that case.
var ErrDisconnectedGraph = errors.New("gonx/metrics: graph is not connected")

// IsConnected reports whether every node is reachable from node 0. The empty
// graph and the single-node graph are considered connected.
func IsConnected(g *gonx.Graph) bool {
	n := g.NumNodes()
	if n <= 1 {
		return true
	}
	dist := make([]int32, n)
	BFS(g, 0, dist)
	for _, d := range dist {
		if d == -1 {
			return false
		}
	}
	return true
}

// ConnectedComponents returns the connected components as slices of node IDs, in
// ascending order of their smallest member.
func ConnectedComponents(g *gonx.Graph) [][]int {
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
	for s := 0; s < n; s++ {
		if comp[s] != -1 {
			continue
		}
		id := len(out)
		members := []int{s}
		comp[s] = id
		queue = queue[:0]
		queue = append(queue, int32(s))
		for head := 0; head < len(queue); head++ {
			u := queue[head]
			for _, v := range g.Neighbors(int(u)) {
				if comp[v] == -1 {
					comp[v] = id
					members = append(members, int(v))
					queue = append(queue, v)
				}
			}
		}
		out = append(out, members)
	}
	return out
}

// AveragePathLength returns the mean shortest-path distance over all ordered pairs
// of distinct nodes. It returns ErrDisconnectedGraph if the graph is not
// connected, and 0 for graphs with fewer than two nodes. The all-pairs BFS is
// parallelized over source nodes.
func AveragePathLength(g *gonx.Graph) (float64, error) {
	n := g.NumNodes()
	if n < 2 {
		return 0, nil
	}
	sum, reachablePairs := allPairsSum(g)
	if reachablePairs != int64(n)*int64(n-1) {
		return 0, ErrDisconnectedGraph
	}
	return float64(sum) / float64(reachablePairs), nil
}

// AveragePathLengthLCC returns the average shortest path length restricted to the
// largest connected component. Unlike AveragePathLength it never errors, making
// it convenient for disconnected graphs. It returns 0 if the largest component
// has fewer than two nodes.
func AveragePathLengthLCC(g *gonx.Graph) float64 {
	comps := ConnectedComponents(g)
	if len(comps) == 0 {
		return 0
	}
	largest := comps[0]
	for _, c := range comps[1:] {
		if len(c) > len(largest) {
			largest = c
		}
	}
	if len(largest) < 2 {
		return 0
	}
	// Restrict BFS to the component by relabeling it into a dense subgraph.
	idx := make(map[int]int, len(largest))
	for i, v := range largest {
		idx[v] = i
	}
	b := gonx.NewBuilder(len(largest))
	for _, v := range largest {
		for _, w := range g.Neighbors(v) {
			if j, ok := idx[int(w)]; ok && int(w) > v {
				b.AddEdge(idx[v], j)
			}
		}
	}
	sum, pairs := allPairsSum(b.Build())
	if pairs == 0 {
		return 0
	}
	return float64(sum) / float64(pairs)
}

// allPairsSum runs one BFS per source (in parallel) and returns the total of all
// finite distances and the number of reachable ordered pairs.
func allPairsSum(g *gonx.Graph) (sum, reachablePairs int64) {
	n := g.NumNodes()
	w := pool.Workers(n)
	sums := make([]int64, w)
	counts := make([]int64, w)
	pool.ParallelChunks(n, w, func(worker, start, end int) {
		// Per-worker reusable BFS scratch keeps allocation at O(workers * n)
		// rather than O(n^2).
		dist := make([]int32, n)
		queue := make([]int32, 0, n)
		var s, c int64
		for src := start; src < end; src++ {
			bfsDistances(g, src, dist, queue)
			for _, d := range dist {
				if d > 0 {
					s += int64(d)
					c++
				}
			}
		}
		sums[worker] = s
		counts[worker] = c
	})
	for i := 0; i < w; i++ {
		sum += sums[i]
		reachablePairs += counts[i]
	}
	return sum, reachablePairs
}

// Diameter returns the longest shortest-path distance between any two nodes. It
// returns ErrDisconnectedGraph if the graph is not connected. Provided for
// completeness alongside AveragePathLength.
func Diameter(g *gonx.Graph) (int, error) {
	n := g.NumNodes()
	if n < 2 {
		return 0, nil
	}
	if !IsConnected(g) {
		return 0, ErrDisconnectedGraph
	}
	w := pool.Workers(n)
	maxes := make([]int32, w)
	pool.ParallelChunks(n, w, func(worker, start, end int) {
		dist := make([]int32, n)
		queue := make([]int32, 0, n)
		var maxDist int32
		for src := start; src < end; src++ {
			bfsDistances(g, src, dist, queue)
			for _, d := range dist {
				if d > maxDist {
					maxDist = d
				}
			}
		}
		maxes[worker] = maxDist
	})
	var m int32
	for _, x := range maxes {
		if x > m {
			m = x
		}
	}
	return int(m), nil
}
