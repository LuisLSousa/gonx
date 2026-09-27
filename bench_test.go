package gonx_test

import (
	"testing"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/generators"
	"github.com/LuisLSousa/gonx/metrics"
	"github.com/LuisLSousa/gonx/transform"
)

func BenchmarkWattsStrogatz_1000_8(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = generators.WattsStrogatz(1000, 8, 0.1, gonx.NewRand(uint64(i)))
	}
}

func BenchmarkBarabasiAlbert_1000_4(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = generators.BarabasiAlbert(1000, 4, gonx.NewRand(uint64(i)))
	}
}

func BenchmarkDoubleEdgeSwap_1500(b *testing.B) {
	g, _ := generators.WattsStrogatz(1000, 8, 0, gonx.NewRand(1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = transform.DoubleEdgeSwap(g, 1500, 1000*1000, gonx.NewRand(uint64(i)))
	}
}

// BenchmarkNeighborIteration measures the simulation's hot path; it should be
// allocation-free (run with -benchmem).
func BenchmarkNeighborIteration(b *testing.B) {
	g, _ := generators.WattsStrogatz(1000, 8, 0.1, gonx.NewRand(1))
	b.ResetTimer()
	var sum int64
	for i := 0; i < b.N; i++ {
		for u := 0; u < g.NumNodes(); u++ {
			for _, v := range g.Neighbors(u) {
				sum += int64(v)
			}
		}
	}
	_ = sum
}

func BenchmarkAveragePathLength_1000(b *testing.B) {
	g, _ := generators.WattsStrogatz(1000, 8, 0.1, gonx.NewRand(1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = metrics.AveragePathLength(g)
	}
}

func BenchmarkTransitivity_1000(b *testing.B) {
	g, _ := generators.WattsStrogatz(1000, 8, 0.1, gonx.NewRand(1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = metrics.Transitivity(g)
	}
}

// BenchmarkDigraphBuild measures freezing a directed builder into the dual-CSR
// Digraph, the directed counterpart of Builder.Build.
func BenchmarkDigraphBuild_10000(b *testing.B) {
	r := gonx.NewRand(1)
	db := gonx.NewDigraphBuilder(10_000)
	for db.NumEdges() < 50_000 {
		db.AddEdge(r.IntN(10_000), r.IntN(10_000))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.Build()
	}
}

func BenchmarkPageRank_10000(b *testing.B) {
	r := gonx.NewRand(1)
	db := gonx.NewDigraphBuilder(10_000)
	for db.NumEdges() < 50_000 {
		db.AddEdge(r.IntN(10_000), r.IntN(10_000))
	}
	g := db.Build()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = metrics.PageRank(g, 0.85, 1e-6, 100)
	}
}

// randomBuilders returns an unweighted and a weighted Builder holding the same
// 10k-node, 50k-edge random topology, so the two Build benchmarks below differ
// only in the weight path.
func randomBuilders() (plain, weighted *gonx.Builder) {
	const n = 10_000
	r := gonx.NewRand(1)
	plain, weighted = gonx.NewBuilder(n), gonx.NewBuilder(n)
	for plain.NumEdges() < 50_000 {
		u, v := r.IntN(n), r.IntN(n)
		if plain.AddEdge(u, v) {
			weighted.AddEdgeW(u, v, r.Float64())
		}
	}
	return plain, weighted
}

func BenchmarkBuild_10000(b *testing.B) {
	plain, _ := randomBuilders()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = plain.Build()
	}
}

// BenchmarkBuildWeighted measures the price of carrying weights through Build:
// sorting packed (neighbor, position) keys and gathering, instead of sorting
// the neighbor row in place.
func BenchmarkBuildWeighted_10000(b *testing.B) {
	_, weighted := randomBuilders()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = weighted.Build()
	}
}

// BenchmarkNeighborIteration_10000 and BenchmarkWeightedNeighborIteration_10000
// walk the same 10k-node graph, without and with the weight array, so the pair
// is the receipt for what carrying weights costs in the inner loop of a
// traversal. Both should be allocation-free.
func BenchmarkNeighborIteration_10000(b *testing.B) {
	plain, _ := randomBuilders()
	g := plain.Build()
	b.ResetTimer()
	var sum int64
	for i := 0; i < b.N; i++ {
		for u := 0; u < g.NumNodes(); u++ {
			for _, v := range g.Neighbors(u) {
				sum += int64(v)
			}
		}
	}
	_ = sum
}

func BenchmarkWeightedNeighborIteration_10000(b *testing.B) {
	_, weighted := randomBuilders()
	g := weighted.Build()
	b.ResetTimer()
	var sum float64
	for i := 0; i < b.N; i++ {
		for u := 0; u < g.NumNodes(); u++ {
			nbrs := g.Neighbors(u)
			// Reslicing to len(nbrs) tells the compiler the two rows match and drops
			// the per-edge bounds check. The graph here is weighted; on an
			// unweighted one Weights is nil and this line would panic.
			ws := g.Weights(u)[:len(nbrs)]
			for j, v := range nbrs {
				sum += ws[j] * float64(v)
			}
		}
	}
	_ = sum
}
