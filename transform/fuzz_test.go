package transform

import (
	"testing"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/generators"
)

// FuzzDoubleEdgeSwap asserts the core invariant — degree preservation — holds for
// arbitrary seeds and swap counts.
func FuzzDoubleEdgeSwap(f *testing.F) {
	f.Add(uint64(1), 10)
	f.Add(uint64(42), 200)
	f.Add(uint64(7), 0)
	f.Fuzz(func(t *testing.T, seed uint64, nswap int) {
		if nswap < 0 || nswap > 5000 {
			t.Skip()
		}
		g, _ := generators.WattsStrogatz(60, 4, 0.2, gonx.NewRand(seed))
		swapped, _, err := DoubleEdgeSwap(g, nswap, 100000, gonx.NewRand(seed+1))
		if err != nil {
			t.Fatal(err)
		}
		for u := 0; u < g.NumNodes(); u++ {
			if g.Degree(u) != swapped.Degree(u) {
				t.Fatalf("degree of %d changed: %d -> %d", u, g.Degree(u), swapped.Degree(u))
			}
		}
		if swapped.NumEdges() != g.NumEdges() {
			t.Fatalf("edge count changed: %d -> %d", g.NumEdges(), swapped.NumEdges())
		}
	})
}
