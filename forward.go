package gonx

// Forward is the read-only, forward-traversal view shared by [Graph] and
// [Digraph]: how many nodes there are and, for each node, the edges leaving it
// together with their weights when the graph has any. Algorithms that only ever
// walk edges in their natural direction, breadth-first search and Dijkstra among
// them, take a Forward and run on either graph kind from a single
// implementation.
//
// On an undirected Graph the edges leaving u are its neighbors, since every edge
// can be walked from either end. Weights follow the alignment rule of the
// concrete types: OutWeights(u)[i] belongs to OutNeighbors(u)[i], and OutWeights
// returns nil when Weighted reports false, in which case every edge counts as
// weight 1.
//
// Other representations may implement Forward to run gonx algorithms over them,
// an implicit grid or a graph streamed from disk for instance. Implementations
// must use dense node IDs in [0, NumNodes) and return each node's targets
// without duplicates or the node itself; the algorithms treat the returned
// slices as read-only and do not hold on to them across calls, so an
// implementation is free to return the same scratch slice every time.
type Forward interface {
	NumNodes() int
	OutNeighbors(u int) []int32
	OutWeights(u int) []float64
	Weighted() bool
}

var (
	_ Forward = (*Graph)(nil)
	_ Forward = (*Digraph)(nil)
)
