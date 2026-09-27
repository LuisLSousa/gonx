package gonx

// Adjacency is the read-only view of a graph's outgoing edges that [Graph] and
// [Digraph] share: how many nodes there are and, for each node, the edges
// leaving it together with their weights when the graph has any. An algorithm
// that only ever walks edges in their natural direction takes an Adjacency and
// runs on either graph kind from a single implementation. On an undirected
// Graph the edges leaving u are its neighbors, since every edge can be walked
// from either end; on a Digraph only the out-side is part of the view, and the
// in-lists stay on the concrete type.
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
//   - HasNegativeWeight is true if, and only if, some OutWeights entry is
//     below zero, and like Weighted it costs O(1).
//   - Returned slices are views into the graph's storage: read-only, with no
//     spare capacity, and valid for as long as the graph is. They may be held
//     across calls, and the graph may be read from several goroutines at once.
//
// Adjacency is sealed: only this package's types, and types embedding them,
// implement it, so the method set can grow with the algorithms that consume it.
// A later release may open it to other representations once that set has
// settled.
type Adjacency interface {
	NumNodes() int
	OutNeighbors(u int) []int32
	OutWeights(u int) []float64
	Weighted() bool
	HasNegativeWeight() bool

	adjacency() // seals the interface to this package
}

func (*Graph) adjacency()   {}
func (*Digraph) adjacency() {}

var (
	_ Adjacency = (*Graph)(nil)
	_ Adjacency = (*Digraph)(nil)
)
