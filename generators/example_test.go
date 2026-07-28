package generators_test

import (
	"fmt"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/generators"
)

func ExampleWattsStrogatz() {
	r := gonx.NewRand(42)
	g, _ := generators.WattsStrogatz(1000, 8, 0.1, r)
	// Rewiring preserves the edge count: n*k/2.
	fmt.Println(g.NumNodes(), g.NumEdges())
	// Output:
	// 1000 4000
}

func ExampleBarabasiAlbert() {
	r := gonx.NewRand(7)
	g, _ := generators.BarabasiAlbert(150, 2, r)
	// Each of the n-m arrivals adds exactly m edges.
	fmt.Println(g.NumNodes(), g.NumEdges())
	// Output:
	// 150 296
}
