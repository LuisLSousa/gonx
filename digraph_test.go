package gonx

import (
	"reflect"
	"slices"
	"testing"
)

func TestDigraphBuilderBasics(t *testing.T) {
	b := NewDigraphBuilder(4)
	if b.NumNodes() != 4 {
		t.Fatalf("NumNodes = %d, want 4", b.NumNodes())
	}
	if !b.AddEdge(0, 1) || !b.AddEdge(1, 2) {
		t.Fatal("AddEdge returned false for valid edges")
	}
	// The reverse of an existing edge is a distinct directed edge.
	if !b.AddEdge(1, 0) {
		t.Error("reverse edge rejected")
	}
	// Self-loop, duplicate, and out-of-range edges are rejected.
	if b.AddEdge(0, 0) {
		t.Error("self-loop accepted")
	}
	if b.AddEdge(0, 1) {
		t.Error("duplicate edge accepted")
	}
	if b.AddEdge(0, 99) || b.AddEdge(-1, 2) {
		t.Error("out-of-range edge accepted")
	}
	if b.NumEdges() != 3 {
		t.Errorf("NumEdges = %d, want 3", b.NumEdges())
	}
	if b.OutDegree(1) != 2 || b.InDegree(1) != 1 {
		t.Errorf("degrees of 1 = out %d in %d, want out 2 in 1", b.OutDegree(1), b.InDegree(1))
	}
	// Removing u->v leaves v->u in place.
	if !b.RemoveEdge(0, 1) || b.HasEdge(0, 1) {
		t.Error("RemoveEdge failed")
	}
	if !b.HasEdge(1, 0) {
		t.Error("RemoveEdge(0, 1) also removed the reverse edge")
	}
	if b.NumEdges() != 2 || b.InDegree(1) != 0 {
		t.Errorf("after remove: m = %d, InDegree(1) = %d, want 2 and 0", b.NumEdges(), b.InDegree(1))
	}
}

func TestDigraphBuildCSR(t *testing.T) {
	// 0 -> 1, 0 -> 2, 2 -> 1, 1 -> 0: a small graph with a 2-cycle and a
	// node (1) whose in-list crosses sources.
	b := NewDigraphBuilder(3)
	for _, e := range [][2]int{{0, 1}, {0, 2}, {2, 1}, {1, 0}} {
		if !b.AddEdge(e[0], e[1]) {
			t.Fatalf("AddEdge(%d, %d) rejected", e[0], e[1])
		}
	}
	g := b.Build()

	if g.NumNodes() != 3 || g.NumEdges() != 4 {
		t.Fatalf("size = (%d, %d), want (3, 4)", g.NumNodes(), g.NumEdges())
	}
	wantOut := [][]int32{{1, 2}, {0}, {1}}
	wantIn := [][]int32{{1}, {0, 2}, {0}}
	for u := range 3 {
		if !slices.Equal(g.OutNeighbors(u), wantOut[u]) {
			t.Errorf("OutNeighbors(%d) = %v, want %v", u, g.OutNeighbors(u), wantOut[u])
		}
		if !slices.Equal(g.InNeighbors(u), wantIn[u]) {
			t.Errorf("InNeighbors(%d) = %v, want %v", u, g.InNeighbors(u), wantIn[u])
		}
		if g.Degree(u) != g.InDegree(u)+g.OutDegree(u) {
			t.Errorf("Degree(%d) = %d, want in+out = %d", u, g.Degree(u), g.InDegree(u)+g.OutDegree(u))
		}
	}
	// Direction matters: 0->2 exists, 2->0 does not.
	if !g.HasEdge(0, 2) || g.HasEdge(2, 0) {
		t.Error("HasEdge ignores direction")
	}
	if g.HasEdge(-1, 0) || g.HasEdge(0, 99) {
		t.Error("HasEdge accepted out-of-range endpoints")
	}
}

func TestDigraphAddEdgeUnchecked(t *testing.T) {
	b := NewDigraphBuilder(4)
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
	// For unique ordered pairs the result must be identical to the checked path.
	checked := NewDigraphBuilder(4)
	checked.AddEdge(0, 1)
	checked.AddEdge(2, 0)
	g1, g2 := b.Build(), checked.Build()
	if !reflect.DeepEqual(g1, g2) {
		t.Error("unchecked and checked builds differ for unique pairs")
	}
}

func TestDigraphEdgesAndNodes(t *testing.T) {
	b := NewDigraphBuilder(3)
	b.AddEdge(1, 0)
	b.AddEdge(0, 2)
	b.AddEdge(1, 2)
	g := b.Build()

	var got [][2]int
	for u, v := range g.Edges() {
		got = append(got, [2]int{u, v})
	}
	want := [][2]int{{0, 2}, {1, 0}, {1, 2}} // by source, then target
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Edges() = %v, want %v", got, want)
	}

	var nodes []int
	for u := range g.Nodes() {
		nodes = append(nodes, u)
	}
	if !reflect.DeepEqual(nodes, []int{0, 1, 2}) {
		t.Errorf("Nodes() = %v", nodes)
	}
}

func TestDigraphToBuilderRoundTrip(t *testing.T) {
	r := NewRand(5)
	b := NewDigraphBuilder(50)
	for range 300 {
		b.AddEdge(r.IntN(50), r.IntN(50))
	}
	g := b.Build()
	back := g.ToBuilder()
	if back.NumEdges() != g.NumEdges() {
		t.Fatalf("round-trip edge count = %d, want %d", back.NumEdges(), g.NumEdges())
	}
	for u := range 50 {
		if back.InDegree(u) != g.InDegree(u) || back.OutDegree(u) != g.OutDegree(u) {
			t.Fatalf("round-trip degrees differ at node %d", u)
		}
	}
	if !reflect.DeepEqual(back.Build(), g) {
		t.Error("round-trip Build differs from original")
	}
}

func TestDigraphRandomOutNeighbor(t *testing.T) {
	b := NewDigraphBuilder(4)
	b.AddEdge(0, 1)
	b.AddEdge(0, 2)
	b.AddEdge(0, 3)
	b.AddEdge(1, 0)
	g := b.Build()
	r := NewRand(9)

	// Node 2 has an in-edge but no out-edges; RandomOutNeighbor must say so.
	if _, ok := g.RandomOutNeighbor(2, r); ok {
		t.Error("RandomOutNeighbor reported ok for a node with no out-edges")
	}
	seen := map[int]bool{}
	for range 200 {
		v, ok := g.RandomOutNeighbor(0, r)
		if !ok {
			t.Fatal("RandomOutNeighbor failed for a node with out-edges")
		}
		seen[v] = true
	}
	if len(seen) != 3 {
		t.Errorf("200 draws from 3 out-neighbors hit %d distinct values", len(seen))
	}
}

func TestDigraphOutOfRangeAccessorsPanic(t *testing.T) {
	b := NewDigraphBuilder(2)
	b.AddEdge(0, 1)
	g := b.Build()
	r := NewRand(1)
	mustPanic(t, "Digraph.OutDegree", func() { g.OutDegree(2) })
	mustPanic(t, "Digraph.InDegree", func() { g.InDegree(-1) })
	mustPanic(t, "Digraph.Degree", func() { g.Degree(2) })
	mustPanic(t, "Digraph.OutNeighbors", func() { g.OutNeighbors(2) })
	mustPanic(t, "Digraph.InNeighbors", func() { g.InNeighbors(-1) })
	mustPanic(t, "Digraph.OutNeighborsSeq", func() { g.OutNeighborsSeq(2) })
	mustPanic(t, "Digraph.InNeighborsSeq", func() { g.InNeighborsSeq(2) })
	mustPanic(t, "Digraph.RandomOutNeighbor", func() { g.RandomOutNeighbor(2, r) })
	mustPanic(t, "Digraph.RandomInNeighbor", func() { g.RandomInNeighbor(-1, r) })
	mustPanic(t, "DigraphBuilder.OutDegree", func() { b.OutDegree(2) })
	mustPanic(t, "DigraphBuilder.InDegree", func() { b.InDegree(-1) })
}

func TestBuilderHasEdgeHugeEndpoint(t *testing.T) {
	// Regression: int32 truncation of a huge v used to collide with a
	// real neighbor (1<<32|1 truncates to 1), so HasEdge reported true
	// and RemoveEdge corrupted the builder.
	huge := 1<<32 | 1
	db := NewDigraphBuilder(4)
	db.AddEdge(0, 1)
	if db.HasEdge(0, huge) {
		t.Error("DigraphBuilder.HasEdge(0, 1<<32|1) = true, want false")
	}
	if db.RemoveEdge(0, huge) || !db.HasEdge(0, 1) || db.NumEdges() != 1 {
		t.Error("RemoveEdge with huge endpoint disturbed the builder")
	}
	ub := NewBuilder(4)
	ub.AddEdge(0, 1)
	if ub.HasEdge(0, huge) {
		t.Error("Builder.HasEdge(0, 1<<32|1) = true, want false")
	}
	if ub.RemoveEdge(0, huge) || !ub.HasEdge(0, 1) || ub.NumEdges() != 1 {
		t.Error("RemoveEdge with huge endpoint disturbed the builder")
	}
}

func TestDigraphBuilderAddNode(t *testing.T) {
	// AddNode must grow the out-lists and the in-degree counters in
	// lockstep; if it didn't, the first edge into the new node would
	// panic on the indeg update.
	b := NewDigraphBuilder(2)
	b.AddEdge(0, 1)
	w := b.AddNode()
	if w != 2 || b.NumNodes() != 3 {
		t.Fatalf("AddNode returned %d with NumNodes %d, want 2 and 3", w, b.NumNodes())
	}
	if !b.AddEdge(0, w) || !b.AddEdge(w, 1) {
		t.Fatal("edges to/from the added node rejected")
	}
	if b.InDegree(w) != 1 || b.OutDegree(w) != 1 {
		t.Errorf("added node degrees = in %d out %d, want 1 and 1", b.InDegree(w), b.OutDegree(w))
	}
	g := b.Build()
	if g.InDegree(w) != 1 || g.OutDegree(w) != 1 {
		t.Errorf("built degrees = in %d out %d, want 1 and 1", g.InDegree(w), g.OutDegree(w))
	}
}

func TestDigraphNeighborSeqValues(t *testing.T) {
	// The Seq iterators must yield the same values as the slice
	// accessors, in the same order, and honor early termination.
	b := NewDigraphBuilder(5)
	b.AddEdge(2, 0)
	b.AddEdge(2, 4)
	b.AddEdge(2, 1)
	b.AddEdge(3, 2)
	g := b.Build()

	var outs []int
	for v := range g.OutNeighborsSeq(2) {
		outs = append(outs, v)
	}
	if !reflect.DeepEqual(outs, []int{0, 1, 4}) {
		t.Errorf("OutNeighborsSeq(2) yielded %v, want [0 1 4]", outs)
	}
	var ins []int
	for v := range g.InNeighborsSeq(2) {
		ins = append(ins, v)
	}
	if !reflect.DeepEqual(ins, []int{3}) {
		t.Errorf("InNeighborsSeq(2) yielded %v, want [3]", ins)
	}
	var first int
	for v := range g.OutNeighborsSeq(2) {
		first = v
		break // early termination must not panic or over-yield
	}
	if first != 0 {
		t.Errorf("first yielded out-neighbor = %d, want 0", first)
	}
}

func TestDigraphRandomInNeighbor(t *testing.T) {
	b := NewDigraphBuilder(4)
	b.AddEdge(1, 0)
	b.AddEdge(2, 0)
	b.AddEdge(3, 0)
	g := b.Build()
	r := NewRand(13)

	// Node 1 has an out-edge but no in-edges.
	if _, ok := g.RandomInNeighbor(1, r); ok {
		t.Error("RandomInNeighbor reported ok for a node with no in-edges")
	}
	seen := map[int]bool{}
	for range 200 {
		v, ok := g.RandomInNeighbor(0, r)
		if !ok {
			t.Fatal("RandomInNeighbor failed for a node with in-edges")
		}
		seen[v] = true
	}
	if len(seen) != 3 {
		t.Errorf("200 draws from 3 in-neighbors hit %d distinct values", len(seen))
	}
}
