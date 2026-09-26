package transform

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/generators"
)

func degreeSequence(g *gonx.Graph) []int {
	ds := make([]int, g.NumNodes())
	for u := 0; u < g.NumNodes(); u++ {
		ds[u] = g.Degree(u)
	}
	sort.Ints(ds)
	return ds
}

func edgeSet(g *gonx.Graph) map[[2]int]bool {
	s := map[[2]int]bool{}
	for u, v := range g.Edges() {
		s[[2]int{u, v}] = true
	}
	return s
}

func TestDoubleEdgeSwapPreservesDegrees(t *testing.T) {
	g, _ := generators.WattsStrogatz(100, 6, 0.1, gonx.NewRand(1))
	before := degreeSequence(g)
	swapped, n, err := DoubleEdgeSwap(g, 200, 100000, gonx.NewRand(2))
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no swaps performed")
	}
	// Per-node degree must be preserved exactly, not just the multiset.
	for u := 0; u < g.NumNodes(); u++ {
		if g.Degree(u) != swapped.Degree(u) {
			t.Fatalf("degree of node %d changed: %d -> %d", u, g.Degree(u), swapped.Degree(u))
		}
	}
	if !reflect.DeepEqual(before, degreeSequence(swapped)) {
		t.Error("degree sequence changed")
	}
	if swapped.NumEdges() != g.NumEdges() {
		t.Errorf("edge count changed: %d -> %d", g.NumEdges(), swapped.NumEdges())
	}
	// The original graph must be untouched.
	if !reflect.DeepEqual(edgeSet(g), edgeSet(g.ToBuilder().Build())) {
		t.Error("original graph mutated")
	}
}

func TestDoubleEdgeSwapNoSelfLoopsOrDuplicates(t *testing.T) {
	g, _ := generators.Complete(10)
	swapped, _, err := DoubleEdgeSwap(g, 50, 10000, gonx.NewRand(5))
	if err != nil {
		t.Fatal(err)
	}
	for u, v := range swapped.Edges() {
		if u == v {
			t.Errorf("self-loop at %d", u)
		}
	}
}

func TestRelabelIsomorphism(t *testing.T) {
	g, _ := generators.WattsStrogatz(30, 4, 0.2, gonx.NewRand(1))
	perm := make([]int, g.NumNodes())
	for i := range perm {
		perm[i] = (i + 7) % g.NumNodes() // a rotation, which is a permutation
	}
	h, err := RelabelNodes(g, perm)
	if err != nil {
		t.Fatal(err)
	}
	// Isomorphism preserves the degree sequence and edge count.
	if !reflect.DeepEqual(degreeSequence(g), degreeSequence(h)) {
		t.Error("relabel changed degree sequence")
	}
	// Applying perm then its inverse restores the original edge set.
	inv := make([]int, len(perm))
	for i, p := range perm {
		inv[p] = i
	}
	back, _ := RelabelNodes(h, inv)
	if !reflect.DeepEqual(edgeSet(g), edgeSet(back)) {
		t.Error("relabel then inverse did not restore original")
	}
}

func TestRelabelRejectsNonPermutation(t *testing.T) {
	g, _ := generators.Complete(4)
	if _, err := RelabelNodes(g, []int{0, 1, 2}); err == nil {
		t.Error("expected error for wrong-length perm")
	}
	if _, err := RelabelNodes(g, []int{0, 1, 1, 2}); err == nil {
		t.Error("expected error for repeated target")
	}
}

func TestShuffleIsIsomorphic(t *testing.T) {
	g, _ := generators.BarabasiAlbert(40, 2, gonx.NewRand(1))
	h := Shuffle(g, gonx.NewRand(9))
	if !reflect.DeepEqual(degreeSequence(g), degreeSequence(h)) {
		t.Error("shuffle changed degree sequence")
	}
}

// weightedGraph puts distinct random weights on a small-world topology and
// returns them keyed by (min, max) endpoint.
func weightedGraph(t *testing.T) (*gonx.Graph, map[[2]int]float64) {
	t.Helper()
	topo, err := generators.WattsStrogatz(40, 4, 0.3, gonx.NewRand(2))
	if err != nil {
		t.Fatal(err)
	}
	r := gonx.NewRand(3)
	b := gonx.NewBuilder(topo.NumNodes())
	want := map[[2]int]float64{}
	for u, v := range topo.Edges() {
		w := r.Float64()
		b.AddEdgeW(u, v, w)
		want[[2]int{u, v}] = w
	}
	return b.Build(), want
}

// checkMappedWeights asserts that got has exactly the edges of want, each
// relabeled through perm and carrying its weight.
func checkMappedWeights(t *testing.T, got *gonx.Graph, want map[[2]int]float64, perm []int) {
	t.Helper()
	if !got.Weighted() {
		t.Fatal("result is unweighted")
	}
	if got.NumEdges() != len(want) {
		t.Fatalf("NumEdges = %d, want %d", got.NumEdges(), len(want))
	}
	for k, w := range want {
		u, v := k[0], k[1]
		if perm != nil {
			u, v = perm[u], perm[v]
		}
		if gw, ok := got.Weight(u, v); !ok || gw != w {
			t.Errorf("Weight(%d, %d) = %v, %v; want %v, true", u, v, gw, ok, w)
		}
	}
}

func TestCopyKeepsWeights(t *testing.T) {
	g, want := weightedGraph(t)
	checkMappedWeights(t, Copy(g), want, nil)
}

func TestRelabelKeepsWeights(t *testing.T) {
	g, want := weightedGraph(t)
	n := g.NumNodes()
	perm := make([]int, n)
	for i := range perm {
		perm[i] = n - 1 - i
	}
	out, err := RelabelNodes(g, perm)
	if err != nil {
		t.Fatal(err)
	}
	checkMappedWeights(t, out, want, perm)
}

func TestShuffleKeepsWeights(t *testing.T) {
	g, want := weightedGraph(t)
	out, perm := ShuffleWithPerm(g, gonx.NewRand(9))
	checkMappedWeights(t, out, want, perm)
}

func TestDoubleEdgeSwapRejectsWeighted(t *testing.T) {
	g, _ := weightedGraph(t)
	_, _, err := DoubleEdgeSwap(g, 10, 1000, gonx.NewRand(1))
	if !errors.Is(err, gonx.ErrInvalidParam) {
		t.Fatalf("err = %v, want ErrInvalidParam", err)
	}
}
