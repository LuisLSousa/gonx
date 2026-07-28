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

func TestAddEdgeUnchecked(t *testing.T) {
	b := NewBuilder(4)
	if !b.AddEdgeUnchecked(0, 1) || !b.AddEdgeUnchecked(2, 0) {
		t.Fatal("AddEdgeUnchecked returned false for valid edges")
	}
	// Endpoints are still validated even on the unchecked path.
	if b.AddEdgeUnchecked(1, 1) {
		t.Error("self-loop accepted")
	}
	if b.AddEdgeUnchecked(0, 4) || b.AddEdgeUnchecked(-1, 2) {
		t.Error("out-of-range edge accepted")
	}
	if b.NumEdges() != 2 || !b.HasEdge(0, 1) || !b.HasEdge(0, 2) {
		t.Errorf("edges not recorded: m=%d", b.NumEdges())
	}
	// For unique pairs the result must be identical to the checked path.
	checked := NewBuilder(4)
	checked.AddEdge(0, 1)
	checked.AddEdge(2, 0)
	g1, g2 := b.Build(), checked.Build()
	if !reflect.DeepEqual(g1.offsets, g2.offsets) || !reflect.DeepEqual(g1.data, g2.data) {
		t.Error("AddEdgeUnchecked produced different CSR than AddEdge for unique pairs")
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

func TestNeighborsSeq(t *testing.T) {
	b := NewBuilder(4)
	b.AddEdge(0, 3)
	b.AddEdge(0, 1)
	g := b.Build()
	var got []int
	for v := range g.NeighborsSeq(0) {
		got = append(got, v)
	}
	if !reflect.DeepEqual(got, []int{1, 3}) {
		t.Errorf("NeighborsSeq(0) = %v, want [1 3]", got)
	}
	// Early break must not panic or over-yield.
	count := 0
	for range g.NeighborsSeq(0) {
		count++
		break
	}
	if count != 1 {
		t.Errorf("early break yielded %d values, want 1", count)
	}
}

// mustPanic asserts fn panics; the accessors' contract for out-of-range IDs.
func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic on out-of-range node", name)
		}
	}()
	fn()
}

func TestOutOfRangeAccessorsPanic(t *testing.T) {
	g := NewBuilder(3).Build()
	r := NewRand(1)
	mustPanic(t, "Graph.Degree", func() { g.Degree(3) })
	mustPanic(t, "Graph.Degree(-1)", func() { g.Degree(-1) })
	mustPanic(t, "Graph.Neighbors", func() { g.Neighbors(3) })
	mustPanic(t, "Graph.NeighborsSeq", func() { g.NeighborsSeq(-1) })
	mustPanic(t, "Graph.RandomNeighbor", func() { g.RandomNeighbor(3, r) })
	b := NewBuilder(2)
	mustPanic(t, "Builder.Degree", func() { b.Degree(2) })
	// HasEdge is the documented exception: out-of-range means "no such edge".
	if g.HasEdge(-1, 99) {
		t.Error("HasEdge(-1, 99) = true, want false")
	}
	if b.HasEdge(5, 0) {
		t.Error("Builder.HasEdge(5, 0) = true, want false")
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
