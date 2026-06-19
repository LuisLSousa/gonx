package metrics

import (
	"errors"
	"math"
	"testing"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/generators"
)

func build(n int, edges [][2]int) *gonx.Graph {
	b := gonx.NewBuilder(n)
	for _, e := range edges {
		b.AddEdge(e[0], e[1])
	}
	return b.Build()
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestTransitivityKnownGraphs(t *testing.T) {
	// Triangle: every triple is closed -> transitivity 1.
	tri := build(3, [][2]int{{0, 1}, {1, 2}, {0, 2}})
	if got := Transitivity(tri); !approx(got, 1.0) {
		t.Errorf("triangle transitivity = %g, want 1", got)
	}
	// Path P3 (0-1-2): one open triple, no triangle -> 0.
	path := build(3, [][2]int{{0, 1}, {1, 2}})
	if got := Transitivity(path); !approx(got, 0) {
		t.Errorf("path transitivity = %g, want 0", got)
	}
	// Empty graph -> 0, no panic.
	if got := Transitivity(build(4, nil)); !approx(got, 0) {
		t.Errorf("empty transitivity = %g, want 0", got)
	}
}

func TestAveragePathLengthKnownGraphs(t *testing.T) {
	// Triangle: all pairs at distance 1.
	tri := build(3, [][2]int{{0, 1}, {1, 2}, {0, 2}})
	if apl, err := AveragePathLength(tri); err != nil || !approx(apl, 1.0) {
		t.Errorf("triangle APL = %g, err=%v, want 1", apl, err)
	}
	// Path P4 (0-1-2-3): distances sum over ordered pairs = 2*(1+2+3+1+2+1)=20,
	// over 12 ordered pairs -> 20/12.
	p4 := build(4, [][2]int{{0, 1}, {1, 2}, {2, 3}})
	apl, err := AveragePathLength(p4)
	if err != nil {
		t.Fatal(err)
	}
	if !approx(apl, 20.0/12.0) {
		t.Errorf("P4 APL = %g, want %g", apl, 20.0/12.0)
	}
}

func TestAveragePathLengthDisconnected(t *testing.T) {
	g := build(4, [][2]int{{0, 1}, {2, 3}}) // two components
	if _, err := AveragePathLength(g); !errors.Is(err, ErrDisconnectedGraph) {
		t.Errorf("expected ErrDisconnectedGraph, got %v", err)
	}
	// LCC variant must not error; each component has 2 nodes at distance 1.
	if apl := AveragePathLengthLCC(g); !approx(apl, 1.0) {
		t.Errorf("LCC APL = %g, want 1", apl)
	}
}

func TestConnectivity(t *testing.T) {
	if !IsConnected(build(3, [][2]int{{0, 1}, {1, 2}})) {
		t.Error("path P3 reported disconnected")
	}
	if IsConnected(build(3, [][2]int{{0, 1}})) {
		t.Error("graph with isolated node reported connected")
	}
	comps := ConnectedComponents(build(5, [][2]int{{0, 1}, {2, 3}}))
	if len(comps) != 3 { // {0,1}, {2,3}, {4}
		t.Errorf("got %d components, want 3", len(comps))
	}
}

// petersen builds the Petersen graph: a well-known 3-regular graph with 10 nodes,
// transitivity 0, diameter 2, and average path length 5/3.
func petersen() *gonx.Graph {
	b := gonx.NewBuilder(10)
	// Outer 5-cycle 0..4, inner pentagram 5..9, spokes i--i+5.
	for i := 0; i < 5; i++ {
		b.AddEdge(i, (i+1)%5)     // outer cycle
		b.AddEdge(5+i, 5+(i+2)%5) // inner pentagram
		b.AddEdge(i, i+5)         // spoke
	}
	return b.Build()
}

func TestPetersenGraph(t *testing.T) {
	g := petersen()
	if g.NumEdges() != 15 {
		t.Fatalf("Petersen edges = %d, want 15", g.NumEdges())
	}
	for u := 0; u < 10; u++ {
		if g.Degree(u) != 3 {
			t.Fatalf("Petersen degree(%d) = %d, want 3", u, g.Degree(u))
		}
	}
	if got := Transitivity(g); !approx(got, 0) {
		t.Errorf("Petersen transitivity = %g, want 0 (triangle-free)", got)
	}
	apl, err := AveragePathLength(g)
	if err != nil || !approx(apl, 5.0/3.0) {
		t.Errorf("Petersen APL = %g, err=%v, want 5/3", apl, err)
	}
	if d, err := Diameter(g); err != nil || d != 2 {
		t.Errorf("Petersen diameter = %d, err=%v, want 2", d, err)
	}
}

// TestParallelDeterminism ensures the parallel reductions give identical results
// regardless of how the work is partitioned, on a larger graph that actually uses
// multiple workers.
func TestParallelDeterminism(t *testing.T) {
	g, _ := generators.WattsStrogatz(500, 6, 0.1, gonx.NewRand(1))
	apl1, err1 := AveragePathLength(g)
	apl2, err2 := AveragePathLength(g)
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v %v", err1, err2)
	}
	if apl1 != apl2 {
		t.Errorf("APL not reproducible: %v vs %v", apl1, apl2)
	}
	tr1, tr2 := Transitivity(g), Transitivity(g)
	if tr1 != tr2 {
		t.Errorf("transitivity not reproducible: %v vs %v", tr1, tr2)
	}
}
