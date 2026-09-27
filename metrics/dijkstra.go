package metrics

import (
	"errors"

	"github.com/LuisLSousa/gonx"
)

// ErrNegativeWeight is returned by Dijkstra when the part of the graph it can
// reach contains an edge with a negative weight. Dijkstra's algorithm is only
// correct for non-negative weights, and a negative one is almost always a data
// error rather than an intended shortest-path problem, so the call fails
// instead of returning distances that are silently wrong.
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
// Dijkstra returns ErrNegativeWeight if, and only if, an edge with a negative
// weight leaves a node that is reachable from src, src included. dist and prev
// are then partially filled and must not be used. Weights are finite, but a
// path length can still overflow to +Inf; a node reached only by such paths is
// reported as unreachable, with dist +Inf and prev -1.
//
// It panics if src is out of range, or if dist or a non-nil prev does not have
// length g.NumNodes(). It runs in O((n + m) log n) time and allocates O(n)
// scratch per call.
func Dijkstra(g gonx.Forward, src int, dist []float64, prev []int32) error {
	return errors.New("gonx/metrics: Dijkstra is not implemented yet")
}
