// Package transform contains structure-preserving and structure-randomizing graph
// transformations: copying, relabeling, and degree-preserving edge swaps.
package transform

import (
	"fmt"
	"math/rand/v2"

	"github.com/LuisLSousa/gonx"
)

// Copy returns an independent deep copy of g.
func Copy(g *gonx.Graph) *gonx.Graph {
	return g.ToBuilder().Build()
}

// RelabelNodes returns a new graph in which node i becomes node perm[i]. perm must
// be a permutation of [0, N). The transformation is a graph isomorphism: it
// preserves the degree sequence and all structural metrics, changing only the
// identities attached to each position.
func RelabelNodes(g *gonx.Graph, perm []int) (*gonx.Graph, error) {
	n := g.NumNodes()
	if len(perm) != n {
		return nil, fmt.Errorf("%w: perm has length %d, want %d", gonx.ErrNotPermutation, len(perm), n)
	}
	seen := make([]bool, n)
	for _, p := range perm {
		if p < 0 || p >= n || seen[p] {
			return nil, fmt.Errorf("%w: invalid or repeated target %d", gonx.ErrNotPermutation, p)
		}
		seen[p] = true
	}
	b := gonx.NewBuilder(n)
	for u, v := range g.Edges() {
		b.AddEdge(perm[u], perm[v])
	}
	return b.Build(), nil
}

// Shuffle returns g with its node labels randomly permuted. The result is
// isomorphic to g (same structure, relabeled nodes).
func Shuffle(g *gonx.Graph, r *rand.Rand) *gonx.Graph {
	out, _ := ShuffleWithPerm(g, r)
	return out
}

// ShuffleWithPerm is like Shuffle but also returns the permutation applied, where
// perm[old] = new.
func ShuffleWithPerm(g *gonx.Graph, r *rand.Rand) (*gonx.Graph, []int) {
	n := g.NumNodes()
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}
	r.Shuffle(n, func(i, j int) { perm[i], perm[j] = perm[j], perm[i] })
	out, err := RelabelNodes(g, perm)
	if err != nil { // perm is a valid permutation by construction
		panic(err)
	}
	return out, perm
}

// DoubleEdgeSwap randomizes a graph while exactly preserving every node's degree.
// It repeatedly picks two edges {a,b} and {c,d} and rewires them to {a,d} and
// {c,b}, rejecting any swap that would create a self-loop or a duplicate edge. It
// performs up to nswap successful swaps, giving up after maxTries attempts. The
// returned int is the number of swaps actually performed.
//
// The operation works on a copy; g is left unchanged. This mirrors
// networkx.double_edge_swap and is the standard way to build degree-preserving
// null models: graphs with the same degree sequence as the original but
// otherwise randomized wiring.
func DoubleEdgeSwap(g *gonx.Graph, nswap, maxTries int, r *rand.Rand) (*gonx.Graph, int, error) {
	if nswap < 0 {
		return nil, 0, fmt.Errorf("%w: nswap must be >= 0, got %d", gonx.ErrInvalidParam, nswap)
	}
	if g.NumEdges() < 2 {
		return Copy(g), 0, nil
	}
	// Maintain an explicit edge list alongside an adjacency builder so we can pick
	// edges uniformly at random and test/update membership cheaply.
	type edge struct{ u, v int32 }
	edges := make([]edge, 0, g.NumEdges())
	for u, v := range g.Edges() {
		edges = append(edges, edge{int32(u), int32(v)})
	}
	b := g.ToBuilder()

	swaps, tries := 0, 0
	for swaps < nswap && tries < maxTries {
		tries++
		i := r.IntN(len(edges))
		j := r.IntN(len(edges))
		if i == j {
			continue
		}
		e1, e2 := edges[i], edges[j]
		a, bb, c, d := e1.u, e1.v, e2.u, e2.v
		// New edges {a,d} and {c,b}. Reject degenerate or colliding results.
		if a == d || c == bb || a == c || bb == d {
			continue
		}
		if b.HasEdge(int(a), int(d)) || b.HasEdge(int(c), int(bb)) {
			continue
		}
		b.RemoveEdge(int(a), int(bb))
		b.RemoveEdge(int(c), int(d))
		b.AddEdge(int(a), int(d))
		b.AddEdge(int(c), int(bb))
		edges[i] = edge{a, d}
		edges[j] = edge{c, bb}
		swaps++
	}
	return b.Build(), swaps, nil
}
