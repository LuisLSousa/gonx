package gonx_test

import (
	"fmt"

	"github.com/LuisLSousa/gonx"
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
