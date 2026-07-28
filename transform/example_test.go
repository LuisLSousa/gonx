package transform_test

import (
	"fmt"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/generators"
	"github.com/LuisLSousa/gonx/transform"
)

func ExampleDoubleEdgeSwap() {
	g, _ := generators.WattsStrogatz(100, 6, 0, gonx.NewRand(1))
	swapped, n, _ := transform.DoubleEdgeSwap(g, 50, 10000, gonx.NewRand(2))
	// Swaps rewire edges but every node keeps its exact degree.
	fmt.Println(n, swapped.NumEdges() == g.NumEdges(), swapped.Degree(0) == g.Degree(0))
	// Output:
	// 50 true true
}
