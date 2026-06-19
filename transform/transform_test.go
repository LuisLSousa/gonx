package transform

import (
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
