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
