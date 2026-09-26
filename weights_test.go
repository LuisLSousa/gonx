package gonx

import (
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
	// Remove a batch to exercise swapDelete on both lists of both endpoints.
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
		if iws != nil && len(iws) != len(in) {
			t.Fatalf("InWeights(%d) has length %d, InNeighbors %d", u, len(iws), len(in))
		}
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

// outWeightSum is the kind of function Forward exists for: one body, both graph
// kinds, unit weights when there are none.
func outWeightSum(g Forward) float64 {
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

func TestForwardCoversBothGraphKinds(t *testing.T) {
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
		t.Error("Forward aliases disagree with Neighbors/Weights")
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

// FuzzWeightsAligned drives both builders through a random script of weighted
// adds, unweighted adds, and removals, then checks that Build kept every weight
// with its edge in every list it appears in.
func FuzzWeightsAligned(f *testing.F) {
	f.Add(uint64(1), uint(40))
	f.Add(uint64(42), uint(300))
	f.Add(uint64(7), uint(0))
	f.Fuzz(func(t *testing.T, seed uint64, steps uint) {
		if steps > 2000 {
			t.Skip()
		}
		r := NewRand(seed)
		const n = 16
		ub, db := NewBuilder(n), NewDigraphBuilder(n)
		uwant, dwant := map[[2]int]float64{}, map[[2]int]float64{}
		for range steps {
			u, v := r.IntN(n), r.IntN(n)
			w := float64(r.IntN(1000)) / 8
			switch r.IntN(3) {
			case 0:
				if ub.AddEdgeW(u, v, w) {
					uwant[edgeKey(u, v)] = w
				}
				if db.AddEdgeW(u, v, w) {
					dwant[[2]int{u, v}] = w
				}
			case 1: // unweighted add: weight 1 whether or not the builder has switched
				if ub.AddEdge(u, v) {
					uwant[edgeKey(u, v)] = 1
				}
				if db.AddEdge(u, v) {
					dwant[[2]int{u, v}] = 1
				}
			case 2:
				if ub.RemoveEdge(u, v) {
					delete(uwant, edgeKey(u, v))
				}
				if db.RemoveEdge(u, v) {
					delete(dwant, [2]int{u, v})
				}
			}
		}
		checkWeights(t, ub.Build(), uwant)
		checkDigraphWeights(t, db.Build(), dwant)
	})
}
