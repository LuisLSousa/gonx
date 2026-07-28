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
