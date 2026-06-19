package generators

import (
	"reflect"
	"sort"
	"testing"

	"github.com/LuisLSousa/gonx"
)

func degreeSequence(g *gonx.Graph) []int {
	ds := make([]int, g.NumNodes())
	for u := 0; u < g.NumNodes(); u++ {
		ds[u] = g.Degree(u)
	}
	sort.Ints(ds)
	return ds
}

func TestComplete(t *testing.T) {
	g, err := Complete(6)
	if err != nil {
		t.Fatal(err)
	}
	if g.NumEdges() != 6*5/2 {
		t.Errorf("edges = %d, want 15", g.NumEdges())
	}
	for u := 0; u < 6; u++ {
		if g.Degree(u) != 5 {
			t.Errorf("Degree(%d) = %d, want 5", u, g.Degree(u))
		}
	}
}

func TestWattsStrogatzRingLattice(t *testing.T) {
	const n, k = 20, 4
	g, err := WattsStrogatz(n, k, 0, gonx.NewRand(1))
	if err != nil {
		t.Fatal(err)
	}
	if g.NumEdges() != n*k/2 {
		t.Errorf("edges = %d, want %d", g.NumEdges(), n*k/2)
	}
	// p=0: pure ring lattice, every node has degree exactly k and is joined to its
	// k/2 nearest neighbors on each side.
	for u := 0; u < n; u++ {
		if g.Degree(u) != k {
			t.Errorf("Degree(%d) = %d, want %d", u, g.Degree(u), k)
		}
		for j := 1; j <= k/2; j++ {
			if !g.HasEdge(u, (u+j)%n) {
				t.Errorf("ring edge %d-%d missing", u, (u+j)%n)
			}
		}
	}
}

func TestWattsStrogatzPreservesEdgeCount(t *testing.T) {
	const n, k = 100, 6
	for _, p := range []float64{0, 0.1, 0.5, 1.0} {
		g, err := WattsStrogatz(n, k, p, gonx.NewRand(7))
		if err != nil {
			t.Fatal(err)
		}
		if g.NumEdges() != n*k/2 {
			t.Errorf("p=%g: edges = %d, want %d", p, g.NumEdges(), n*k/2)
		}
	}
}

func TestWattsStrogatzInvalidParams(t *testing.T) {
	cases := []struct {
		n, k int
		p    float64
	}{
		{10, 3, 0.1},  // odd k
		{10, 10, 0.1}, // k >= n
		{10, 4, 1.5},  // p out of range
	}
	for _, c := range cases {
		if _, err := WattsStrogatz(c.n, c.k, c.p, gonx.NewRand(1)); err == nil {
			t.Errorf("expected error for n=%d k=%d p=%g", c.n, c.k, c.p)
		}
	}
}

func TestBarabasiAlbert(t *testing.T) {
	const n, m = 50, 3
	g, err := BarabasiAlbert(n, m, gonx.NewRand(1))
	if err != nil {
		t.Fatal(err)
	}
	// Each of the n-m new nodes contributes exactly m edges.
	if g.NumEdges() != m*(n-m) {
		t.Errorf("edges = %d, want %d", g.NumEdges(), m*(n-m))
	}
	// Degree sum is twice the edge count.
	sum := 0
	for u := 0; u < n; u++ {
		sum += g.Degree(u)
	}
	if sum != 2*g.NumEdges() {
		t.Errorf("degree sum = %d, want %d", sum, 2*g.NumEdges())
	}
	// Every node added after the seed has degree >= m.
	for u := m; u < n; u++ {
		if g.Degree(u) < m {
			t.Errorf("Degree(%d) = %d, want >= %d", u, g.Degree(u), m)
		}
	}
}

func TestRandomAvgDegree(t *testing.T) {
	const n = 200
	g, err := RandomAvgDegree(n, 8, gonx.NewRand(3))
	if err != nil {
		t.Fatal(err)
	}
	avg := float64(2*g.NumEdges()) / float64(n)
	if avg < 7.8 || avg > 8.2 {
		t.Errorf("avg degree = %g, want ~8", avg)
	}
}

func TestDeterminismSameSeedSameGraph(t *testing.T) {
	gen := func(seed uint64) *gonx.Graph {
		g, _ := WattsStrogatz(80, 6, 0.3, gonx.NewRand(seed))
		return g
	}
	g1, g2 := gen(99), gen(99)
	if !reflect.DeepEqual(degreeSequence(g1), degreeSequence(g2)) {
		t.Fatal("same seed produced different degree sequences")
	}
	// Full structural equality via edge sets.
	e1, e2 := map[[2]int]bool{}, map[[2]int]bool{}
	for u, v := range g1.Edges() {
		e1[[2]int{u, v}] = true
	}
	for u, v := range g2.Edges() {
		e2[[2]int{u, v}] = true
	}
	if !reflect.DeepEqual(e1, e2) {
		t.Error("same seed produced different edge sets")
	}
}
