package gonx

// Forward is the read-only, forward-traversal view that [Graph] and [Digraph]
// share: how many nodes there are and, for each node, the edges leaving it
// together with their weights when the graph has any. An algorithm that only
// ever walks edges in their natural direction takes a Forward and runs on either
// graph kind from a single implementation. On an undirected Graph the edges
// leaving u are its neighbors, since every edge can be walked from either end.
//
// The contract, which both graph kinds honor and which algorithms rely on:
//
//   - Node IDs are dense in [0, NumNodes).
//   - OutNeighbors(u) lists the targets of u's outgoing edges in ascending
//     order, without duplicates and without u itself.
//   - Weighted is constant for the life of the graph. When it is true,
//     OutWeights(u) has the same length as OutNeighbors(u) and OutWeights(u)[i]
//     is the finite weight of the edge to OutNeighbors(u)[i]; when it is false,
//     OutWeights returns nil and every edge counts as weight 1.
//   - Returned slices are views into the graph's storage: read-only, with no
//     spare capacity, and valid for as long as the graph is. They may be held
//     across calls, and the graph may be read from several goroutines at once.
//
// Forward is sealed: only this package's types, and types embedding them,
// implement it, so the method set can grow with the algorithms that consume it.
// A later release may open it to other representations once that set has
// settled.
type Forward interface {
	NumNodes() int
	OutNeighbors(u int) []int32
	OutWeights(u int) []float64
	Weighted() bool

	forward() // seals the interface to this package
}

func (*Graph) forward()   {}
func (*Digraph) forward() {}

var (
	_ Forward = (*Graph)(nil)
	_ Forward = (*Digraph)(nil)
)
