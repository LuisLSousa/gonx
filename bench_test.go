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
