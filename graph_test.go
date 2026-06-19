package gonx

import (
	"reflect"
	"slices"
	"testing"
)

func TestBuilderBasics(t *testing.T) {
	b := NewBuilder(4)
	if b.NumNodes() != 4 {
		t.Fatalf("NumNodes = %d, want 4", b.NumNodes())
	}
	if !b.AddEdge(0, 1) || !b.AddEdge(1, 2) {
		t.Fatal("AddEdge returned false for valid edges")
	}
	// Self-loop, duplicate, and out-of-range edges are rejected.
	if b.AddEdge(0, 0) {
		t.Error("self-loop accepted")
	}
	if b.AddEdge(1, 0) {
		t.Error("duplicate edge (reversed) accepted")
	}
	if b.AddEdge(0, 99) {
		t.Error("out-of-range edge accepted")
	}
	if b.NumEdges() != 2 {
		t.Errorf("NumEdges = %d, want 2", b.NumEdges())
	}
	if b.Degree(1) != 2 {
		t.Errorf("Degree(1) = %d, want 2", b.Degree(1))
	}
	if !b.RemoveEdge(0, 1) || b.HasEdge(0, 1) {
		t.Error("RemoveEdge failed")
	}
	if b.NumEdges() != 1 {
		t.Errorf("NumEdges after remove = %d, want 1", b.NumEdges())
	}
}

func TestBuildCSRSortedAndCorrect(t *testing.T) {
	b := NewBuilder(5)
	// Insert neighbors out of order to confirm CSR sorts them.
	b.AddEdge(0, 4)
	b.AddEdge(0, 2)
	b.AddEdge(0, 1)
	g := b.Build()

	got := slices.Clone(g.Neighbors(0))
	want := []int32{1, 2, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Neighbors(0) = %v, want %v (must be sorted)", got, want)
	}
	if g.NumEdges() != 3 || g.Degree(0) != 3 {
		t.Errorf("edges=%d deg0=%d, want 3,3", g.NumEdges(), g.Degree(0))
	}
	for _, v := range []int{1, 2, 4} {
		if !g.HasEdge(0, v) || !g.HasEdge(v, 0) {
			t.Errorf("HasEdge missing for 0-%d", v)
		}
	}
	if g.HasEdge(0, 3) {
		t.Error("HasEdge reported nonexistent edge 0-3")
	}
}

func TestEdgesIteratorVisitsEachOnce(t *testing.T) {
	b := NewBuilder(4)
	b.AddEdge(0, 1)
	b.AddEdge(1, 2)
	b.AddEdge(2, 3)
	b.AddEdge(3, 0)
	g := b.Build()

	count := 0
	for u, v := range g.Edges() {
		if u >= v {
			t.Errorf("Edges yielded (%d,%d) with u >= v", u, v)
		}
		count++
	}
	if count != 4 {
		t.Errorf("Edges visited %d times, want 4", count)
	}
}

func TestNodesIterator(t *testing.T) {
	g := NewBuilder(3).Build()
	var got []int
	for u := range g.Nodes() {
		got = append(got, u)
	}
	if !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Errorf("Nodes = %v, want [0 1 2]", got)
	}
}

func TestRandomNeighbor(t *testing.T) {
	b := NewBuilder(3)
	b.AddEdge(0, 1)
	b.AddEdge(0, 2)
	g := b.Build()
	r := NewRand(1)
	seen := map[int]bool{}
	for i := 0; i < 50; i++ {
		v, ok := g.RandomNeighbor(0, r)
		if !ok {
			t.Fatal("RandomNeighbor(0) returned ok=false on non-isolated node")
		}
		seen[v] = true
	}
	if !seen[1] || !seen[2] {
		t.Errorf("RandomNeighbor never returned one of the neighbors: %v", seen)
	}
	// Isolated node.
	if _, ok := g.RandomNeighbor(0, r); !ok {
		// sanity: node 0 is not isolated
	}
	iso := NewBuilder(1).Build()
	if _, ok := iso.RandomNeighbor(0, r); ok {
		t.Error("RandomNeighbor on isolated node returned ok=true")
	}
}

func TestToBuilderRoundTrip(t *testing.T) {
	b := NewBuilder(5)
	b.AddEdge(0, 1)
	b.AddEdge(2, 3)
	b.AddEdge(1, 4)
	g1 := b.Build()
	g2 := g1.ToBuilder().Build()
	if !reflect.DeepEqual(g1.offsets, g2.offsets) || !reflect.DeepEqual(g1.data, g2.data) {
		t.Error("ToBuilder().Build() did not reproduce identical CSR")
	}
}

func TestNewRandDeterministic(t *testing.T) {
	a, b := NewRand(42), NewRand(42)
	for i := 0; i < 100; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatalf("NewRand streams diverged at draw %d", i)
		}
	}
}
