package gonx

import (
	"bytes"
	"math"
	"slices"
	"testing"
)

// edgeKey identifies an undirected edge regardless of endpoint order.
func edgeKey(u, v int) [2]int {
	if u > v {
		u, v = v, u
	}
	return [2]int{u, v}
}

// checkWeights verifies every edge of g against want through both accessors,
// that neighbor lists are sorted with weights aligned, and that g has no edge
// beyond want. On an unweighted g every want value must be 1.
func checkWeights(t *testing.T, g *Graph, want map[[2]int]float64) {
	t.Helper()
	if g.NumEdges() != len(want) {
		t.Fatalf("NumEdges = %d, want %d", g.NumEdges(), len(want))
	}
	for u := 0; u < g.NumNodes(); u++ {
		nbrs, ws := g.Neighbors(u), g.Weights(u)
		if !slices.IsSorted(nbrs) {
			t.Fatalf("Neighbors(%d) = %v is not sorted", u, nbrs)
		}
		if (ws == nil) == g.Weighted() {
			t.Fatalf("Weights(%d) = %v on a graph with Weighted() = %v", u, ws, g.Weighted())
		}
		if ws != nil && len(ws) != len(nbrs) {
			t.Fatalf("Weights(%d) has length %d, Neighbors %d", u, len(ws), len(nbrs))
		}
		for i, v := range nbrs {
			w, ok := want[edgeKey(u, int(v))]
			if !ok {
				t.Fatalf("unexpected edge {%d, %d}", u, v)
			}
			if ws != nil && ws[i] != w {
				t.Errorf("Weights(%d)[%d] (edge to %d) = %v, want %v", u, i, v, ws[i], w)
			}
			if got, ok := g.Weight(u, int(v)); !ok || got != w {
				t.Errorf("Weight(%d, %d) = %v, %v; want %v, true", u, v, got, ok, w)
			}
		}
	}
}

func TestAddEdgeWRejectsBadInput(t *testing.T) {
	b := NewBuilder(3)
	b.AddEdge(0, 1)
	cases := []struct {
		name string
		u, v int
		w    float64
	}{
		{"NaN", 1, 2, math.NaN()},
		{"+Inf", 1, 2, math.Inf(1)},
		{"-Inf", 1, 2, math.Inf(-1)},
		{"self-loop", 1, 1, 1},
		{"out of range", 1, 3, 1},
		{"negative endpoint", -1, 2, 1},
		{"duplicate of an unweighted edge", 0, 1, 2},
	}
	for _, c := range cases {
		if b.AddEdgeW(c.u, c.v, c.w) {
			t.Errorf("%s: AddEdgeW accepted", c.name)
		}
	}
	if b.Weighted() {
		t.Fatal("rejected AddEdgeW calls switched the builder to weighted")
	}
	if b.NumEdges() != 1 {
		t.Fatalf("NumEdges = %d, want 1", b.NumEdges())
	}
	// Zero and negative weights are legal; only NaN and the infinities are not.
	if !b.AddEdgeW(1, 2, 0) || !b.AddEdgeW(0, 2, -3.5) {
		t.Fatal("zero or negative weight rejected")
	}
	if b.AddEdgeW(2, 1, 7) {
		t.Fatal("duplicate of a weighted edge accepted")
	}
}

func TestAddEdgeWBackfillsAndDefaults(t *testing.T) {
	b := NewBuilder(4)
	b.AddEdge(0, 1) // before the switch: backfilled to weight 1
	if !b.AddEdgeW(1, 2, 2.5) {
		t.Fatal("AddEdgeW rejected a valid edge")
	}
	b.AddEdge(2, 3)          // after the switch: weight 1
	b.AddEdgeUnchecked(0, 3) // likewise
	g := b.Build()
	if !g.Weighted() {
		t.Fatal("Weighted = false")
	}
	checkWeights(t, g, map[[2]int]float64{{0, 1}: 1, {1, 2}: 2.5, {2, 3}: 1, {0, 3}: 1})
	if got := g.Weights(1); !slices.Equal(got, []float64{1, 2.5}) {
		t.Errorf("Weights(1) = %v, want [1 2.5] for neighbors %v", got, g.Neighbors(1))
	}
}

func TestUnweightedGraphReportsUnitWeights(t *testing.T) {
	b := NewBuilder(3)
	b.AddEdge(0, 1)
	g := b.Build()
	if g.Weighted() || g.Weights(0) != nil || g.OutWeights(0) != nil {
		t.Fatal("unweighted graph exposes weights")
	}
	if w, ok := g.Weight(0, 1); !ok || w != 1 {
		t.Errorf("Weight(0, 1) = %v, %v; want 1, true", w, ok)
	}
	if w, ok := g.Weight(1, 2); ok || w != 0 {
		t.Errorf("Weight(1, 2) = %v, %v; want 0, false", w, ok)
	}
	if _, ok := g.Weight(0, 5); ok {
		t.Error("Weight with an out-of-range endpoint reported ok")
	}
}

func TestRemoveEdgeKeepsWeightsAligned(t *testing.T) {
	b := NewBuilder(4)
	// A star around 0 with distinct weights, plus one rim edge.
	b.AddEdgeW(0, 1, 10)
	b.AddEdgeW(0, 2, 20)
	b.AddEdgeW(0, 3, 30)
	b.AddEdgeW(1, 2, 12)
	if !b.RemoveEdge(0, 2) {
		t.Fatal("RemoveEdge reported a missing edge")
	}
	b.AddEdgeW(0, 2, 21) // re-added with a new weight; must land aligned
	checkWeights(t, b.Build(), map[[2]int]float64{{0, 1}: 10, {0, 2}: 21, {0, 3}: 30, {1, 2}: 12})
}

func TestBuildWeightsAlignedRandom(t *testing.T) {
	r := NewRand(3)
	const n = 200
	b := NewBuilder(n)
	want := map[[2]int]float64{}
	var added [][2]int
	for len(want) < 1500 {
		u, v := r.IntN(n), r.IntN(n)
		w := r.Float64()*100 - 50
		if b.AddEdgeW(u, v, w) {
			want[edgeKey(u, v)] = w
			added = append(added, edgeKey(u, v))
		}
	}
	// Remove a batch so unlink and moveWeight run on both lists of both endpoints.
	for _, k := range added[:300] {
		if !b.RemoveEdge(k[0], k[1]) {
			t.Fatalf("RemoveEdge%v = false", k)
		}
		delete(want, k)
	}
	g := b.Build()
	checkWeights(t, g, want)

	// EdgeOffset partitions the edge arrays exactly by degree.
	if g.EdgeOffset(0) != 0 {
		t.Errorf("EdgeOffset(0) = %d", g.EdgeOffset(0))
	}
	for u := 0; u+1 < n; u++ {
		if g.EdgeOffset(u+1) != g.EdgeOffset(u)+g.Degree(u) {
			t.Fatalf("EdgeOffset(%d) = %d, want %d", u+1, g.EdgeOffset(u+1), g.EdgeOffset(u)+g.Degree(u))
		}
	}
	if end := g.EdgeOffset(n-1) + g.Degree(n-1); end != 2*g.NumEdges() {
		t.Errorf("edge arrays end at %d, want %d", end, 2*g.NumEdges())
	}
}

func TestWeightedBuildKeepsUnweightedLayout(t *testing.T) {
	r := NewRand(5)
	const n = 300
	plain, weighted := NewBuilder(n), NewBuilder(n)
	for plain.NumEdges() < 2000 {
		u, v := r.IntN(n), r.IntN(n)
		if plain.AddEdge(u, v) {
			weighted.AddEdgeW(u, v, r.Float64())
		}
	}
	pg, wg := plain.Build(), weighted.Build()
	if !slices.Equal(pg.offsets, wg.offsets) || !slices.Equal(pg.data, wg.data) {
		t.Fatal("weighted Build produced a different CSR layout than the unweighted one")
	}
	if pg.weights != nil {
		t.Fatal("unweighted Build allocated a weight array")
	}
	if len(wg.weights) != len(wg.data) {
		t.Fatalf("weight array has length %d, adjacency %d", len(wg.weights), len(wg.data))
	}
}

func TestToBuilderRoundTripKeepsWeights(t *testing.T) {
	b := NewBuilder(3)
	b.AddEdgeW(0, 1, 1.5)
	b.AddEdgeW(1, 2, 2.5)
	g := b.Build()
	b2 := g.ToBuilder()
	if !b2.Weighted() {
		t.Fatal("ToBuilder dropped the weights")
	}
	b2.RemoveEdge(0, 1)
	b2.AddEdgeW(0, 2, 9)
	g2 := b2.Build()
	checkWeights(t, g, map[[2]int]float64{{0, 1}: 1.5, {1, 2}: 2.5}) // the original is untouched
	checkWeights(t, g2, map[[2]int]float64{{1, 2}: 2.5, {0, 2}: 9})
}

func TestAddNodeOnWeightedBuilder(t *testing.T) {
	b := NewBuilder(2)
	b.AddEdgeW(0, 1, 4)
	v := b.AddNode()
	if !b.AddEdgeW(1, v, 5) {
		t.Fatal("edge to a node added after the switch rejected")
	}
	checkWeights(t, b.Build(), map[[2]int]float64{{0, 1}: 4, {1, 2}: 5})
}

// checkDigraphWeights is checkWeights for directed graphs: out-lists, in-lists,
// and Weight must all agree with want.
func checkDigraphWeights(t *testing.T, g *Digraph, want map[[2]int]float64) {
	t.Helper()
	if g.NumEdges() != len(want) {
		t.Fatalf("NumEdges = %d, want %d", g.NumEdges(), len(want))
	}
	inTotal := 0
	for u := 0; u < g.NumNodes(); u++ {
		out, ows := g.OutNeighbors(u), g.OutWeights(u)
		if (ows == nil) == g.Weighted() {
			t.Fatalf("OutWeights(%d) = %v on a graph with Weighted() = %v", u, ows, g.Weighted())
		}
		if ows != nil && len(ows) != len(out) {
			t.Fatalf("OutWeights(%d) has length %d, OutNeighbors %d", u, len(ows), len(out))
		}
		for i, v := range out {
			w, ok := want[[2]int{u, int(v)}]
			if !ok {
				t.Fatalf("unexpected edge %d->%d", u, v)
			}
			if ows != nil && ows[i] != w {
				t.Errorf("OutWeights(%d)[%d] (edge to %d) = %v, want %v", u, i, v, ows[i], w)
			}
			if got, ok := g.Weight(u, int(v)); !ok || got != w {
				t.Errorf("Weight(%d, %d) = %v, %v; want %v, true", u, v, got, ok, w)
			}
		}
		in, iws := g.InNeighbors(u), g.InWeights(u)
		if (iws == nil) == g.Weighted() {
			t.Fatalf("InWeights(%d) = %v on a graph with Weighted() = %v", u, iws, g.Weighted())
		}
		if iws != nil && len(iws) != len(in) {
			t.Fatalf("InWeights(%d) has length %d, InNeighbors %d", u, len(iws), len(in))
		}
		inTotal += len(in)
		for i, src := range in {
			w, ok := want[[2]int{int(src), u}]
			if !ok {
				t.Fatalf("in-list of %d claims an edge from %d that does not exist", u, src)
			}
			if iws != nil && iws[i] != w {
				t.Errorf("InWeights(%d)[%d] (edge from %d) = %v, want %v", u, i, src, iws[i], w)
			}
		}
	}
	if inTotal != g.NumEdges() {
		t.Fatalf("in-lists hold %d entries, NumEdges is %d", inTotal, g.NumEdges())
	}
}

func TestDigraphAddEdgeW(t *testing.T) {
	b := NewDigraphBuilder(4)
	b.AddEdge(0, 1)
	if b.AddEdgeW(1, 2, math.NaN()) || b.AddEdgeW(2, 2, 1) || b.AddEdgeW(0, 1, 5) || b.Weighted() {
		t.Fatal("bad AddEdgeW input accepted, or it switched the builder to weighted")
	}
	b.AddEdgeW(1, 2, 2.5)
	b.AddEdgeW(2, 1, -1) // the reverse edge is distinct and keeps its own weight
	b.AddEdge(2, 3)
	b.RemoveEdge(0, 1)
	b.AddEdgeW(0, 1, 0.25)
	g := b.Build()
	if !g.Weighted() {
		t.Fatal("Weighted = false")
	}
	checkDigraphWeights(t, g, map[[2]int]float64{{1, 2}: 2.5, {2, 1}: -1, {2, 3}: 1, {0, 1}: 0.25})
}

func TestDigraphWeightSearchesBothLists(t *testing.T) {
	// Node 0 fans out to everyone, so Weight(0, v) searches v's short in-list;
	// every other node points at 1, so Weight(u, 1) searches u's short out-list.
	const n = 50
	b := NewDigraphBuilder(n)
	want := map[[2]int]float64{}
	for v := 1; v < n; v++ {
		b.AddEdgeW(0, v, float64(v))
		want[[2]int{0, v}] = float64(v)
		if v != 1 {
			b.AddEdgeW(v, 1, float64(100+v))
			want[[2]int{v, 1}] = float64(100 + v)
		}
	}
	g := b.Build()
	if g.OutDegree(0) <= g.InDegree(n-1) || g.OutDegree(2) >= g.InDegree(1) {
		t.Fatal("test graph does not exercise both search branches")
	}
	checkDigraphWeights(t, g, want)
	if _, ok := g.Weight(1, 0); ok {
		t.Error("Weight reported a reverse edge that does not exist")
	}
}

func TestDigraphBuildWeightsAlignedRandom(t *testing.T) {
	r := NewRand(11)
	const n = 200
	b := NewDigraphBuilder(n)
	want := map[[2]int]float64{}
	var added [][2]int
	for len(want) < 1500 {
		u, v := r.IntN(n), r.IntN(n)
		w := r.Float64()
		if b.AddEdgeW(u, v, w) {
			want[[2]int{u, v}] = w
			added = append(added, [2]int{u, v})
		}
	}
	for _, k := range added[:300] {
		b.RemoveEdge(k[0], k[1])
		delete(want, k)
	}
	g := b.Build()
	checkDigraphWeights(t, g, want)
	for u := 0; u+1 < n; u++ {
		if g.OutEdgeOffset(u+1) != g.OutEdgeOffset(u)+g.OutDegree(u) {
			t.Fatalf("OutEdgeOffset(%d) disagrees with OutDegree(%d)", u+1, u)
		}
		if g.InEdgeOffset(u+1) != g.InEdgeOffset(u)+g.InDegree(u) {
			t.Fatalf("InEdgeOffset(%d) disagrees with InDegree(%d)", u+1, u)
		}
	}
}

func TestDigraphUnweightedAndRoundTrip(t *testing.T) {
	b := NewDigraphBuilder(3)
	b.AddEdge(0, 1)
	g := b.Build()
	if g.Weighted() || g.OutWeights(0) != nil || g.InWeights(1) != nil {
		t.Fatal("unweighted digraph exposes weights")
	}
	if w, ok := g.Weight(0, 1); !ok || w != 1 {
		t.Errorf("Weight(0, 1) = %v, %v; want 1, true", w, ok)
	}
	wb := NewDigraphBuilder(3)
	wb.AddEdgeW(0, 1, 2)
	wb.AddEdgeW(1, 2, 3)
	rt := wb.Build().ToBuilder()
	if !rt.Weighted() {
		t.Fatal("ToBuilder dropped the weights")
	}
	checkDigraphWeights(t, rt.Build(), map[[2]int]float64{{0, 1}: 2, {1, 2}: 3})
}

// outWeightSum is the kind of function Adjacency exists for: one body, both graph
// kinds, unit weights when there are none.
func outWeightSum(g Adjacency) float64 {
	var total float64
	for u := 0; u < g.NumNodes(); u++ {
		if g.Weighted() {
			for _, w := range g.OutWeights(u) {
				total += w
			}
		} else {
			total += float64(len(g.OutNeighbors(u)))
		}
	}
	return total
}

func TestAdjacencyCoversBothGraphKinds(t *testing.T) {
	ub := NewBuilder(3)
	ub.AddEdgeW(0, 1, 2)
	ub.AddEdgeW(1, 2, 3)
	db := NewDigraphBuilder(3)
	db.AddEdgeW(0, 1, 2)
	db.AddEdgeW(1, 2, 3)
	if got := outWeightSum(ub.Build()); got != 10 { // an undirected edge leaves both of its ends
		t.Errorf("undirected sum = %v, want 10", got)
	}
	if got := outWeightSum(db.Build()); got != 5 {
		t.Errorf("directed sum = %v, want 5", got)
	}
	plain := NewBuilder(3)
	plain.AddEdge(0, 1)
	if got := outWeightSum(plain.Build()); got != 2 {
		t.Errorf("unweighted sum = %v, want 2", got)
	}
	g := ub.Build()
	if !slices.Equal(g.OutNeighbors(1), g.Neighbors(1)) || !slices.Equal(g.OutWeights(1), g.Weights(1)) {
		t.Error("Adjacency aliases disagree with Neighbors/Weights")
	}
}

func TestWeightAccessorsPanicOutOfRange(t *testing.T) {
	g := NewBuilder(2).Build()
	mustPanic(t, "Weights", func() { g.Weights(2) })
	mustPanic(t, "EdgeOffset", func() { g.EdgeOffset(-1) })
	d := NewDigraphBuilder(2).Build()
	mustPanic(t, "OutWeights", func() { d.OutWeights(2) })
	mustPanic(t, "InWeights", func() { d.InWeights(2) })
	mustPanic(t, "OutEdgeOffset", func() { d.OutEdgeOffset(2) })
	mustPanic(t, "InEdgeOffset", func() { d.InEdgeOffset(-1) })
}

func TestNewWeightedBuilder(t *testing.T) {
	b := NewWeightedBuilder(3)
	if !b.Weighted() {
		t.Fatal("NewWeightedBuilder is not weighted")
	}
	empty := b.Build()
	if !empty.Weighted() || empty.NumEdges() != 0 {
		t.Fatalf("edgeless build: Weighted = %v, NumEdges = %d", empty.Weighted(), empty.NumEdges())
	}
	if ws := empty.Weights(0); ws == nil || len(ws) != 0 {
		t.Errorf("Weights(0) on an edgeless weighted graph = %v, want empty non-nil", ws)
	}
	b.AddEdge(0, 1) // unweighted add on a weighted builder: weight 1
	b.AddEdgeW(1, 2, 7)
	checkWeights(t, b.Build(), map[[2]int]float64{{0, 1}: 1, {1, 2}: 7})
	if !NewWeightedBuilder(0).Weighted() {
		t.Error("NewWeightedBuilder(0) is not weighted")
	}

	d := NewWeightedDigraphBuilder(3)
	if !d.Weighted() || !d.Build().Weighted() {
		t.Fatal("NewWeightedDigraphBuilder or its edgeless build is not weighted")
	}
	d.AddEdgeUnchecked(0, 1)
	d.AddEdgeW(1, 2, 7)
	checkDigraphWeights(t, d.Build(), map[[2]int]float64{{0, 1}: 1, {1, 2}: 7})
}

func TestDigraphAddNodeAndUncheckedAfterSwitch(t *testing.T) {
	b := NewDigraphBuilder(2)
	b.AddEdgeW(0, 1, 4)
	v := b.AddNode()
	if !b.AddEdgeW(1, v, 5) || !b.AddEdgeUnchecked(v, 0) {
		t.Fatal("edges involving a node added after the switch rejected")
	}
	checkDigraphWeights(t, b.Build(), map[[2]int]float64{{0, 1}: 4, {1, 2}: 5, {2, 0}: 1})
}

func TestEdgeIndex(t *testing.T) {
	b := NewBuilder(4)
	b.AddEdgeW(0, 1, 10)
	b.AddEdgeW(0, 2, 20)
	b.AddEdgeW(2, 3, 23)
	g := b.Build()
	for u := 0; u < g.NumNodes(); u++ {
		for i, v := range g.Neighbors(u) {
			slot, ok := g.EdgeIndex(u, int(v))
			if !ok || slot != g.EdgeOffset(u)+i {
				t.Errorf("EdgeIndex(%d, %d) = %d, %v; want %d, true", u, v, slot, ok, g.EdgeOffset(u)+i)
			}
			if w, _ := g.Weight(u, int(v)); g.Weights(u)[slot-g.EdgeOffset(u)] != w {
				t.Errorf("slot %d does not address the weight of {%d, %d}", slot, u, v)
			}
		}
	}
	for _, e := range [][2]int{{1, 2}, {3, 0}, {0, 0}, {0, 4}, {-1, 1}} {
		if _, ok := g.EdgeIndex(e[0], e[1]); ok {
			t.Errorf("EdgeIndex%v reported an edge", e)
		}
	}

	d := NewDigraphBuilder(4)
	d.AddEdgeW(0, 1, 1)
	d.AddEdgeW(2, 1, 21)
	d.AddEdgeW(1, 3, 13)
	dg := d.Build()
	for u := 0; u < dg.NumNodes(); u++ {
		for i, v := range dg.OutNeighbors(u) {
			if slot, ok := dg.EdgeIndex(u, int(v)); !ok || slot != dg.OutEdgeOffset(u)+i {
				t.Errorf("EdgeIndex(%d, %d) = %d, %v; want %d, true", u, v, slot, ok, dg.OutEdgeOffset(u)+i)
			}
		}
		for i, src := range dg.InNeighbors(u) {
			slot, ok := dg.InEdgeIndex(int(src), u)
			if !ok || slot != dg.InEdgeOffset(u)+i {
				t.Errorf("InEdgeIndex(%d, %d) = %d, %v; want %d, true", src, u, slot, ok, dg.InEdgeOffset(u)+i)
			}
			if w, _ := dg.Weight(int(src), u); dg.InWeights(u)[slot-dg.InEdgeOffset(u)] != w {
				t.Errorf("in slot %d does not address the weight of %d->%d", slot, src, u)
			}
		}
	}
	if _, ok := dg.EdgeIndex(1, 0); ok {
		t.Error("EdgeIndex reported the reverse of an existing edge")
	}
	if _, ok := dg.InEdgeIndex(1, 0); ok {
		t.Error("InEdgeIndex reported the reverse of an existing edge")
	}
}

// TestAccessorSlicesHaveNoSpareCapacity guards against append on a returned
// view silently overwriting the next node's data.
func TestAccessorSlicesHaveNoSpareCapacity(t *testing.T) {
	b := NewBuilder(4)
	b.AddEdgeW(0, 1, 1)
	b.AddEdgeW(1, 2, 2)
	b.AddEdgeW(2, 3, 3)
	g := b.Build()
	for u := 0; u < 3; u++ { // node 3 is last; its view has no successor to clobber
		if nbrs := g.Neighbors(u); cap(nbrs) != len(nbrs) {
			t.Errorf("Neighbors(%d): cap %d, len %d", u, cap(nbrs), len(nbrs))
		}
		if ws := g.Weights(u); cap(ws) != len(ws) {
			t.Errorf("Weights(%d): cap %d, len %d", u, cap(ws), len(ws))
		}
	}
	_ = append(g.Weights(0), 99) // must allocate, not write into node 1's weights
	if w, _ := g.Weight(1, 2); w != 2 {
		t.Errorf("append on a view overwrote a neighbor's weight: Weight(1, 2) = %v", w)
	}

	d := NewDigraphBuilder(4)
	d.AddEdgeW(0, 1, 1)
	d.AddEdgeW(1, 2, 2)
	d.AddEdgeW(2, 3, 3)
	dg := d.Build()
	for u := 0; u < 3; u++ {
		for name, sl := range map[string]int{
			"OutNeighbors": cap(dg.OutNeighbors(u)) - len(dg.OutNeighbors(u)),
			"OutWeights":   cap(dg.OutWeights(u)) - len(dg.OutWeights(u)),
			"InNeighbors":  cap(dg.InNeighbors(u+1)) - len(dg.InNeighbors(u+1)),
			"InWeights":    cap(dg.InWeights(u+1)) - len(dg.InWeights(u+1)),
		} {
			if sl != 0 {
				t.Errorf("%s at node %d has %d spare capacity", name, u, sl)
			}
		}
	}
}

// checkPrefix is the white-box invariant the prefix representation rests on: a
// weight list never outgrows its neighbor list, and a weighted builder has a
// weight list slot for every node.
func checkPrefix(t *testing.T, ub *Builder, db *DigraphBuilder) {
	t.Helper()
	if ub.w != nil {
		if len(ub.w) != len(ub.adj) {
			t.Fatalf("Builder: %d weight lists for %d nodes", len(ub.w), len(ub.adj))
		}
		for u := range ub.adj {
			if len(ub.w[u]) > len(ub.adj[u]) {
				t.Fatalf("Builder node %d: weight list %d longer than neighbor list %d", u, len(ub.w[u]), len(ub.adj[u]))
			}
		}
	}
	if db.w != nil {
		if len(db.w) != len(db.out) {
			t.Fatalf("DigraphBuilder: %d weight lists for %d nodes", len(db.w), len(db.out))
		}
		for u := range db.out {
			if len(db.w[u]) > len(db.out[u]) {
				t.Fatalf("DigraphBuilder node %d: weight list %d longer than out-list %d", u, len(db.w[u]), len(db.out[u]))
			}
		}
	}
}

// FuzzWeightsAligned drives both builders through a script of four-byte
// instructions (opcode, two node bytes, a weight byte): weighted and unweighted
// adds (checked and unchecked), removals, AddNode, Build with continued
// mutation afterwards, ToBuilder round trips, and a restart from the weighted
// constructors. After every step it checks the prefix invariant, and at every
// Build that each weight came out on its edge. Weights avoid 1 on purpose, so a
// slot filled by the implicit default can never pass as a real weight, and
// take 256 distinct values, so two edges in one row rarely share one and a swap
// between them shows.
func FuzzWeightsAligned(f *testing.F) {
	f.Add([]byte{
		0, 1, 2, 10, 4, 2, 3, 0, 6, 3, 0, 0, 10, 0, 0, 0, 9, 0, 0, 0, 0, 4, 1, 200,
		11, 0, 0, 0, 8, 1, 2, 0, 12, 0, 0, 0, 4, 0, 1, 0, 0, 1, 3, 77, 10, 0, 0, 0,
	})
	f.Add(bytes.Repeat([]byte{0x11, 0x87, 0x2a, 0xfe, 0x63, 0x05, 0x40, 0x9c}, 40))
	f.Fuzz(func(t *testing.T, script []byte) {
		if len(script) > 8000 {
			script = script[:8000]
		}
		ub, db := NewBuilder(4), NewDigraphBuilder(4)
		uwant, dwant := map[[2]int]float64{}, map[[2]int]float64{}
		for i := 0; i+3 < len(script); i += 4 {
			op, a, c, wb := script[i]%13, int(script[i+1]), int(script[i+2]), script[i+3]
			n := ub.NumNodes()
			u, v := a%n, c%n
			w := 2 + float64(wb)/32 // 2.0 .. 9.97, never 1
			switch op {
			case 0, 1, 2, 3:
				if ub.AddEdgeW(u, v, w) {
					uwant[edgeKey(u, v)] = w
				}
				if db.AddEdgeW(u, v, w) {
					dwant[[2]int{u, v}] = w
				}
			case 4, 5:
				if ub.AddEdge(u, v) {
					uwant[edgeKey(u, v)] = 1
				}
				if db.AddEdge(u, v) {
					dwant[[2]int{u, v}] = 1
				}
			case 6: // unchecked adds are only legal on fresh pairs
				if u != v && !ub.HasEdge(u, v) && ub.AddEdgeUnchecked(u, v) {
					uwant[edgeKey(u, v)] = 1
				}
				if u != v && !db.HasEdge(u, v) && db.AddEdgeUnchecked(u, v) {
					dwant[[2]int{u, v}] = 1
				}
			case 7, 8:
				if ub.RemoveEdge(u, v) {
					delete(uwant, edgeKey(u, v))
				}
				if db.RemoveEdge(u, v) {
					delete(dwant, [2]int{u, v})
				}
			case 9:
				if n < 64 {
					ub.AddNode()
					db.AddNode()
				}
			case 10: // build, check, and keep mutating the same builders
				checkWeights(t, ub.Build(), uwant)
				checkDigraphWeights(t, db.Build(), dwant)
			case 11: // round trip through the immutable form
				ub = ub.Build().ToBuilder()
				db = db.Build().ToBuilder()
			case 12: // start over from the weighted constructors
				ub, db = NewWeightedBuilder(n), NewWeightedDigraphBuilder(n)
				clear(uwant)
				clear(dwant)
			}
			checkPrefix(t, ub, db)
		}
		checkWeights(t, ub.Build(), uwant)
		checkDigraphWeights(t, db.Build(), dwant)
	})
}
