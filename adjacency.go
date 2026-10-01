package gonx

// Adjacency is the read-only view of a graph's outgoing edges that [Graph] and
// [Digraph] share, and that a [Restricted] view of either provides too: how
// many nodes there are and, for each node, the edges leaving it together with
// their weights when the graph has any. An algorithm that only ever walks edges
// in their natural direction takes an Adjacency and runs on either graph kind,
// or on a view with parts hidden, from a single implementation. On an undirected
// Graph the edges leaving u are its neighbors, since every edge can be walked
// from either end; on a Digraph only the out-side is part of the view, and the
// in-lists stay on the concrete type.
//
// The contract, which every implementation honors and which algorithms rely on:
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
// Adjacency is sealed: only this package's types implement it, so the method
// set can grow with the algorithms that consume it. A later release may open it
// to other representations once that set has settled. A type that embeds one
// of them, or embeds an Adjacency, satisfies the interface too, and takes on
// the contract for every method it overrides; an override of OutWeights, for
// one, must keep HasNegativeWeight truthful, since Dijkstra relies on it to
// reject negative weights up front. Functions that need a graph's storage
// rather than its methods, such as [RestrictedView], read through such a
// wrapper to the graph it holds.
type Adjacency interface {
	NumNodes() int
	OutNeighbors(u int) []int32
	OutWeights(u int) []float64
	Weighted() bool
	HasNegativeWeight() bool

	// adjacency seals the interface and returns the Graph, Digraph or
	// Restricted that implements it. Through a wrapper, which inherits the
	// method from what it embeds, that is the value wrapped.
	adjacency() Adjacency
}

func (g *Graph) adjacency() Adjacency      { return g }
func (g *Digraph) adjacency() Adjacency    { return g }
func (r *Restricted) adjacency() Adjacency { return r }

var (
	_ Adjacency = (*Graph)(nil)
	_ Adjacency = (*Digraph)(nil)
	_ Adjacency = (*Restricted)(nil)
)
