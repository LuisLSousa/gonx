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

func ExampleBreadthFirst() {
	// Hop counts follow edge direction on a Digraph: 3 is two hops from 0, and
	// nothing leads back to 0.
	b := gonx.NewDigraphBuilder(4)
	b.AddEdge(0, 1)
	b.AddEdge(1, 3)
	b.AddEdge(2, 0)
	g := b.Build()

	hops := make([]int32, g.NumNodes())
	metrics.BreadthFirst(g, 0, hops)
	fmt.Println(hops)
	// Output:
	// [0 1 -1 2]
}

func ExampleDijkstra() {
	// A square 0-1-2-3 with unit sides, a diagonal 0-2 of weight 2.5, and a
	// direct 0-3 of weight 5. Both shortcuts lose to walking the sides.
	b := gonx.NewBuilder(4)
	b.AddEdgeW(0, 1, 1)
	b.AddEdgeW(1, 2, 1)
	b.AddEdgeW(2, 3, 1)
	b.AddEdgeW(0, 2, 2.5)
	b.AddEdgeW(0, 3, 5)
	g := b.Build()

	dist := make([]float64, g.NumNodes())
	prev := make([]int32, g.NumNodes())
	if err := metrics.Dijkstra(g, 0, dist, prev); err != nil {
		panic(err)
	}
	fmt.Println(dist)
	fmt.Println(prev) // the shortest-path tree: -1 marks the source
	// Output:
	// [0 1 2 3]
	// [-1 0 1 2]
}

func ExampleShortestPath() {
	b := gonx.NewBuilder(5) // node 4 stays isolated
	b.AddEdgeW(0, 1, 1)
	b.AddEdgeW(1, 2, 1)
	b.AddEdgeW(2, 3, 1)
	b.AddEdgeW(0, 3, 5)
	g := b.Build()

	path, length, err := metrics.ShortestPath(g, 0, 3)
	if err != nil {
		panic(err)
	}
	fmt.Println(path, length)
	path, length, err = metrics.ShortestPath(g, 0, 4)
	if err != nil {
		panic(err)
	}
	fmt.Println(path == nil, length)
	// Output:
	// [0 1 2 3] 3
	// true +Inf
}

func ExampleBridges() {
	// Two triangles, 0-1-2 and 3-4-5, joined by the single edge 2-3, with
	// node 6 hanging off 5. Each node carries a weight, here a user count.
	b := gonx.NewBuilder(7)
	for _, e := range [][2]int{{0, 1}, {1, 2}, {0, 2}, {2, 3}, {3, 4}, {4, 5}, {3, 5}, {5, 6}} {
		b.AddEdge(e[0], e[1])
	}
	g := b.Build()
	users := []float64{120, 80, 200, 40, 25, 60, 5}

	for _, br := range metrics.Bridges(g, users) {
		fmt.Printf("cutting %d-%d strands nodes: %d, users: %v\n", br.U, br.V, br.Side, br.SideWeight)
	}
	// Output:
	// cutting 2-3 strands nodes: 4, users: 130
	// cutting 5-6 strands nodes: 1, users: 5
}

func ExampleArticulationPoints() {
	b := gonx.NewBuilder(7) // the same graph as in the Bridges example
	for _, e := range [][2]int{{0, 1}, {1, 2}, {0, 2}, {2, 3}, {3, 4}, {4, 5}, {3, 5}, {5, 6}} {
		b.AddEdge(e[0], e[1])
	}
	fmt.Println(metrics.ArticulationPoints(b.Build()))
	// Output:
	// [2 3 5]
}
