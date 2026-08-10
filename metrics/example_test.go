package metrics_test

import (
	"fmt"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/metrics"
)

func ExampleAveragePathLength() {
	// Path graph 0-1-2-3.
	b := gonx.NewBuilder(4)
	b.AddEdge(0, 1)
	b.AddEdge(1, 2)
	b.AddEdge(2, 3)
	apl, err := metrics.AveragePathLength(b.Build())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%.3f\n", apl)
	// Output:
	// 1.667
}

func ExampleTransitivity() {
	// A triangle with a pendant node hanging off it.
	b := gonx.NewBuilder(4)
	b.AddEdge(0, 1)
	b.AddEdge(1, 2)
	b.AddEdge(0, 2)
	b.AddEdge(2, 3)
	fmt.Printf("%.3f\n", metrics.Transitivity(b.Build()))
	// Output:
	// 0.600
}

func ExamplePageRank() {
	// Pages 0 and 1 both link to 2, which links back to 0: rank pools at
	// 2 and flows on to 0. Nothing links to 1, so it ends up with exactly
	// the teleport share, (1 - 0.85) / 3 = 0.05.
	b := gonx.NewDigraphBuilder(3)
	b.AddEdge(0, 2)
	b.AddEdge(1, 2)
	b.AddEdge(2, 0)
	ranks, err := metrics.PageRank(b.Build(), 0.85, 1e-6, 100)
	if err != nil {
		fmt.Println(err)
		return
	}
	for v, r := range ranks {
		fmt.Printf("%d: %.3f\n", v, r)
	}
	// Output:
	// 0: 0.464
	// 1: 0.050
	// 2: 0.486
}

func ExampleWeaklyConnectedComponents() {
	// 1 -> 0 and 1 -> 4 hang together once direction is ignored; 2 -> 3
	// is a separate island.
	b := gonx.NewDigraphBuilder(5)
	b.AddEdge(1, 0)
	b.AddEdge(1, 4)
	b.AddEdge(2, 3)
	for _, comp := range metrics.WeaklyConnectedComponents(b.Build()) {
		fmt.Println(comp)
	}
	// Output:
	// [0 1 4]
	// [2 3]
}
