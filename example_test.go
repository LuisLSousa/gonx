package gonx_test

import (
	"fmt"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/metrics"
)

func Example() {
	// Assemble a topology with Builder, then freeze it into an immutable CSR
	// Graph for reading.
	b := gonx.NewBuilder(4)
	b.AddEdge(0, 1)
	b.AddEdge(0, 2)
	b.AddEdge(2, 3)
	g := b.Build()

	fmt.Println(g.NumNodes(), g.NumEdges())
	fmt.Println(g.Neighbors(0))
	fmt.Println(g.HasEdge(1, 2))
	// Output:
	// 4 3
	// [1 2]
	// false
}

func ExampleGraph_Edges() {
	b := gonx.NewBuilder(3)
	b.AddEdge(0, 1)
	b.AddEdge(1, 2)
	g := b.Build()
	for u, v := range g.Edges() {
		fmt.Println(u, v)
	}
	// Output:
	// 0 1
	// 1 2
}

func ExampleGraph_NeighborsSeq() {
	b := gonx.NewBuilder(3)
	b.AddEdge(0, 2)
	b.AddEdge(0, 1)
	g := b.Build()
	for v := range g.NeighborsSeq(0) {
		fmt.Println(v)
	}
	// Output:
	// 1
	// 2
}

func ExampleDigraph() {
	// A tiny citation graph: later papers cite earlier ones. Direction
	// matters, and a Digraph keeps both adjacency directions, so asking
	// "whom does 3 cite?" and "who cites 0?" are equally cheap.
	b := gonx.NewDigraphBuilder(4)
	b.AddEdge(1, 0)
	b.AddEdge(2, 0)
	b.AddEdge(3, 0)
	b.AddEdge(3, 2)
	g := b.Build()

	fmt.Println(g.OutNeighbors(3)) // what 3 cites
	fmt.Println(g.InNeighbors(0))  // who cites 0
	fmt.Println(g.HasEdge(3, 2), g.HasEdge(2, 3))
	// Output:
	// [0 2]
	// [1 2 3]
	// true false
}

func ExampleDigraph_OutNeighborsSeq() {
	b := gonx.NewDigraphBuilder(4)
	b.AddEdge(0, 3)
	b.AddEdge(0, 1)
	sum := 0
	g := b.Build()
	for v := range g.OutNeighborsSeq(0) {
		sum += v
	}
	fmt.Println(sum)
	// Output:
	// 4
}

func ExampleBuilder_AddEdgeW() {
	// A weighted triangle. Weights ride along with their edges through Build and
	// come back aligned with Neighbors.
	b := gonx.NewBuilder(3)
	b.AddEdgeW(0, 1, 2.5)
	b.AddEdgeW(1, 2, 0.5)
	b.AddEdge(0, 2) // an unweighted add on a weighted builder means weight 1
	g := b.Build()

	fmt.Println(g.Neighbors(1), g.Weights(1))
	fmt.Println(g.Weight(0, 2))
	fmt.Println(g.Weight(1, 1))
	// Output:
	// [0 2] [2.5 0.5]
	// 1 true
	// 0 false
}

func ExampleGraph_EdgeOffset() {
	// Per-edge data of your own lives in a slice laid out like the adjacency:
	// slot EdgeOffset(u)+i belongs to the edge from u to Neighbors(u)[i].
	// Compute it once per half-edge, then read it in the hot loop with no map
	// lookup. Here the value is a resistance of 1/(deg(u)+deg(v)).
	b := gonx.NewBuilder(4)
	b.AddEdge(0, 1)
	b.AddEdge(1, 2)
	b.AddEdge(1, 3)
	g := b.Build()

	resistance := make([]float64, 2*g.NumEdges())
	for u := range g.Nodes() {
		off := g.EdgeOffset(u)
		for i, v := range g.Neighbors(u) {
			resistance[off+i] = 1 / float64(g.Degree(u)+g.Degree(int(v)))
		}
	}

	// Total resistance around node 1: three edges of 1/(3+1) each.
	var total float64
	off := g.EdgeOffset(1)
	for i := range g.Neighbors(1) {
		total += resistance[off+i]
	}
	fmt.Println(total)

	// EdgeIndex finds a single edge's slot without walking the row.
	slot, _ := g.EdgeIndex(2, 1)
	fmt.Println(resistance[slot])
	// Output:
	// 0.75
	// 0.25
}

func ExampleAdjacency() {
	// One function for both graph kinds: the total weight leaving each node.
	outWeight := func(g gonx.Adjacency) []float64 {
		out := make([]float64, g.NumNodes())
		for u := range out {
			if g.Weighted() {
				for _, w := range g.OutWeights(u) {
					out[u] += w
				}
			} else {
				out[u] = float64(len(g.OutNeighbors(u)))
			}
		}
		return out
	}

	ub := gonx.NewBuilder(3)
	ub.AddEdgeW(0, 1, 2)
	ub.AddEdgeW(1, 2, 3)
	db := gonx.NewDigraphBuilder(3)
	db.AddEdgeW(0, 1, 2)
	db.AddEdgeW(1, 2, 3)

	fmt.Println(outWeight(ub.Build())) // an undirected edge leaves both ends
	fmt.Println(outWeight(db.Build()))
	// Output:
	// [2 5 3]
	// [2 3 0]
}

func ExampleRestrictedView() {
	// A ring 0-1-2-3 with a short cut from 0 to 2. Hiding the short cut
	// answers "what is the best path if this edge were gone?" without
	// copying the graph, and without changing it for anyone else reading it.
	b := gonx.NewWeightedBuilder(4)
	b.AddEdgeW(0, 1, 2)
	b.AddEdgeW(1, 2, 2)
	b.AddEdgeW(2, 3, 1)
	b.AddEdgeW(3, 0, 1)
	b.AddEdgeW(0, 2, 1)
	g := b.Build()

	path, length, _ := metrics.ShortestPath(g, 0, 2)
	fmt.Println(path, length)
	view := gonx.RestrictedView(g, nil, [][2]int{{0, 2}})
	path, length, _ = metrics.ShortestPath(view, 0, 2)
	fmt.Println(path, length)
	// Hiding node 3 as well leaves the long way round.
	path, length, _ = metrics.ShortestPath(gonx.RestrictedView(view, []int{3}, nil), 0, 2)
	fmt.Println(path, length)
	// Output:
	// [0 2] 1
	// [0 3 2] 2
	// [0 1 2] 4
}
